package homebrew

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

type Engine struct {
	Collector *Collector
	Streams   Streams
}

func (e Engine) Prepare(ctx context.Context, request ports.Request, policy domain.Policy, overrides []ports.AgeOverride, now int64) (ports.Prepared, ports.ExecutionSession, error) {
	if e.Collector == nil || !domain.ValidRequest(request.Operation, request.Targets) {
		return ports.Prepared{}, nil, errors.New("native planner unavailable")
	}
	if e.Collector.LegacyState != "" {
		_, err := os.Lstat(e.Collector.LegacyState)
		if err == nil || !errors.Is(err, os.ErrNotExist) {
			return ports.Prepared{}, nil, errors.New("legacy attempt journal requires the matching older build for recovery before this version can run; archive it only after confirming no owned process remains")
		}
	}
	seen := map[string]bool{}
	for _, override := range overrides {
		if !domain.ValidRequest("install", []string{override.Name}) || !domain.ValidAgeReason(override.Reason) || seen[override.Name] {
			return ports.Prepared{}, nil, errors.New("invalid or duplicate age exception")
		}
		seen[override.Name] = true
	}
	if len(request.Targets) == 0 {
		entries, err := os.ReadDir("/opt/homebrew/Cellar")
		if err != nil {
			return ports.Prepared{}, nil, errors.New("cannot inspect installed formulae")
		}
		if len(entries) > 128 {
			return ports.Prepared{}, nil, errors.New("upgrade inventory exceeds supported closure")
		}
		for _, entry := range entries {
			if !entry.IsDir() || !domain.ValidRequest("install", []string{entry.Name()}) {
				return ports.Prepared{}, nil, errors.New("unsupported installed inventory")
			}
			request.Targets = append(request.Targets, entry.Name())
		}
		if len(request.Targets) == 0 {
			return ports.Prepared{}, nil, ports.ErrNothingToDo
		}
	}
	if err := os.MkdirAll(e.Collector.Directory, 0700); err != nil {
		return ports.Prepared{}, nil, err
	}
	// Refuse an owned active child before running a new Homebrew preflight.
	// Prepare checks again under its own lock before reserving a new plan.
	lock, err := acquireOperationLock(e.Collector.Directory)
	if err != nil {
		return ports.Prepared{}, nil, err
	}
	guardErr := clearStoppedInFlight(e.Collector.Directory)
	closeErr := lock.Close()
	if guardErr != nil || closeErr != nil {
		return ports.Prepared{}, nil, errors.Join(guardErr, closeErr)
	}
	collection, err := e.Collector.Collect(ctx, request, now)
	if err != nil {
		return ports.Prepared{}, nil, err
	}
	waivers := []domain.AgeWaiver{}
	for _, override := range overrides {
		found := false
		for _, node := range collection.nodes {
			if node.Artifact.Name == override.Name {
				waivers = append(waivers, domain.AgeWaiver{Artifact: node.Artifact, Reason: override.Reason})
				found = true
			}
		}
		if !found {
			_ = removeCollection(e.Collector.Directory, filepath.Base(collection.root))
			return ports.Prepared{}, nil, errors.New("age exception is outside the complete candidate plan")
		}
	}
	return collection.Prepare(ctx, policy, waivers, now, e.Streams)
}
func (e Engine) Check(ctx context.Context) error {
	if e.Collector == nil || e.Collector.BottleVerifier == nil || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return errors.New("runtime requires Apple Silicon macOS Tahoe")
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(bounded, "/usr/bin/sw_vers", "-productVersion")
	command.Env = []string{"PATH=/usr/bin:/bin"}
	version, err := command.Output()
	if err != nil || !strings.HasPrefix(string(version), "26.") {
		return errors.New("macOS version is unsupported")
	}
	prefix, err := filepath.EvalSymlinks("/opt/homebrew")
	if err != nil || prefix != "/opt/homebrew" {
		return errors.New("standard Homebrew prefix is unavailable")
	}
	brew, err := os.Lstat(filepath.Join(prefix, "bin/brew"))
	if err != nil || !brew.Mode().IsRegular() {
		return errors.New("standard Homebrew installation is unavailable")
	}
	temporary, err := os.MkdirTemp("", "brewwarden-runtime-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	temporary, err = filepath.EvalSymlinks(temporary)
	if err != nil {
		return err
	}
	_, err = e.Collector.Runtime.materialize(filepath.Join(temporary, "runtime"))
	if err != nil {
		return err
	}
	// Runtime inventory copying can exceed the OS probe's short deadline;
	// gh has its own independent 10-second command deadline.
	return e.Collector.BottleVerifier.Check(ctx)
}

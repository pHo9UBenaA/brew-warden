package homebrew

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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
	if err := checkLegacyState(e.Collector.LegacyState); err != nil {
		return ports.Prepared{}, nil, err
	}
	if err := validateAgeOverrides(overrides); err != nil {
		return ports.Prepared{}, nil, err
	}
	targets, err := requestedTargets(request.Targets)
	if err != nil {
		return ports.Prepared{}, nil, err
	}
	request.Targets = targets
	if err := os.MkdirAll(e.Collector.Directory, 0o700); err != nil {
		return ports.Prepared{}, nil, err
	}
	// Refuse an owned active child before running a new Homebrew preflight.
	// Collection.Prepare repeats the guard while retaining the execution lock.
	if err := checkNoActiveExecution(e.Collector.Directory); err != nil {
		return ports.Prepared{}, nil, err
	}
	collection, err := e.Collector.Collect(ctx, request, now)
	if err != nil {
		return ports.Prepared{}, nil, err
	}
	waivers, err := planWaivers(collection, overrides)
	if err != nil {
		_ = removeCollection(e.Collector.Directory, filepath.Base(collection.root))
		return ports.Prepared{}, nil, err
	}
	return collection.Prepare(ctx, policy, waivers, now, e.Streams)
}

func checkLegacyState(path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return errors.New(
			"legacy attempt journal requires the matching older build for recovery before this version can run; " +
				"archive it only after confirming no owned process remains",
		)
	}
	return nil
}

func validateAgeOverrides(overrides []ports.AgeOverride) error {
	seen := map[string]bool{}
	for _, override := range overrides {
		if !domain.ValidFormulaName(override.Name) || !domain.ValidAgeReason(override.Reason) || seen[override.Name] {
			return errors.New("invalid or duplicate age exception")
		}
		seen[override.Name] = true
	}
	return nil
}

func requestedTargets(targets []string) ([]string, error) {
	if len(targets) != 0 {
		return targets, nil
	}
	entries, err := os.ReadDir("/opt/homebrew/Cellar")
	if err != nil {
		return nil, errors.New("cannot inspect installed formulae")
	}
	if len(entries) > 128 {
		return nil, errors.New("upgrade inventory exceeds supported closure")
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !domain.ValidFormulaName(entry.Name()) {
			return nil, errors.New("unsupported installed inventory")
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		return nil, ports.ErrNothingToDo
	}
	return names, nil
}

func checkNoActiveExecution(directory string) error {
	lock, err := acquireOperationLock(directory)
	if err != nil {
		return err
	}
	guardErr := clearStoppedInFlight(directory)
	closeErr := lock.Close()
	return errors.Join(guardErr, closeErr)
}

func planWaivers(collection *Collection, overrides []ports.AgeOverride) ([]domain.AgeWaiver, error) {
	waivers := []domain.AgeWaiver{}
	for _, override := range overrides {
		candidateIndex := slices.IndexFunc(collection.nodes, func(node domain.Node) bool {
			return node.Artifact.Name == override.Name
		})
		if candidateIndex == -1 {
			return nil, errors.New("age exception is outside the complete candidate plan")
		}
		artifact := collection.nodes[candidateIndex].Artifact
		waivers = append(waivers, domain.AgeWaiver{Artifact: artifact, Reason: override.Reason})
	}
	return waivers, nil
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
	if err != nil || !strings.HasPrefix(string(version), supportedOSVersionPrefix) {
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
	_, err = e.Collector.Runtime.materialize(ctx, filepath.Join(temporary, "runtime"))
	if err != nil {
		return err
	}
	// Runtime inventory copying can exceed the OS probe's short deadline;
	// gh has its own independent 10-second command deadline.
	return e.Collector.BottleVerifier.Check(ctx)
}

package homebrew

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func (e Engine) Snapshot(ctx context.Context, binding domain.Binding) (domain.Digest, error) {
	if e.Collector == nil || !binding.Valid() {
		return "", errors.New("invalid reconciliation request")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	plan, err := e.savedPlan(binding)
	if err != nil {
		return "", err
	}
	root, err := os.MkdirTemp(e.Collector.Directory, "recovery-")
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	w := workspace{root}
	if err := w.initialize(); err != nil {
		return "", err
	}
	if _, err := e.Collector.Runtime.materialize(filepath.Join(root, "runtime")); err != nil {
		return "", err
	}
	// Reconciliation only writes cooperative Homebrew lock files, never packages.
	profile, err := w.sandbox("reconcile", false, false, []string{filepath.Join(root, "runtime/brew/Library")})
	if err != nil {
		return "", err
	}
	raw, err := readRegular(profile, maxManifest)
	if err != nil {
		return "", err
	}
	raw = append(raw, []byte("\n(allow file-write* (subpath \"/opt/homebrew/var/homebrew/locks\"))\n")...)
	if err := writeNew(profile+".locks", raw, 0600); err != nil {
		return "", err
	}
	names := []string{}
	for _, node := range plan.Nodes {
		if !node.Artifact.Valid() {
			return "", errors.New("invalid saved candidate")
		}
		names = append(names, node.Artifact.Name)
	}
	args := append([]string{"ruby", filepath.Join(root, "bootstrap.rb"), "/opt/homebrew", filepath.Join(root, "reconcile.rb"), root}, names...)
	command := w.command(ctx, profile+".locks", args...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	output, diagnostics := &processOutput{}, &processOutput{}
	command.Stdout = output
	command.Stderr = diagnostics
	if err := command.Run(); err != nil || output.overflow || diagnostics.overflow {
		return "", errors.New("native reconciliation could not acquire locks or observe state")
	}
	var event struct {
		Schema     int           `json:"schema" required:"true"`
		AfterState domain.Digest `json:"afterState" required:"true"`
	}
	if err := decodeStrict(output.Bytes(), &event); err != nil || event.Schema != 1 || !event.AfterState.Valid() {
		return "", errors.New("invalid reconciliation observation")
	}
	snapshot, err := readRegular(filepath.Join(root, "states", string(event.AfterState)+".json"), maxManifest)
	if err != nil || digestBytes(snapshot) != event.AfterState {
		return "", errors.New("reconciliation state was not retained")
	}
	if err := writeRecord(filepath.Join(root, "observation.json"), output.Bytes()); err != nil {
		return "", err
	}
	return event.AfterState, nil
}
func (e Engine) savedPlan(binding domain.Binding) (executionPlan, error) {
	directory := e.Collector.Directory
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return executionPlan{}, errors.New("private plan store unavailable")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) > 10000 {
		return executionPlan{}, errors.New("plan inventory unavailable or oversized")
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "collection-") {
			continue
		}
		if !entry.IsDir() {
			return executionPlan{}, errors.New("invalid plan directory")
		}
		file := filepath.Join(directory, entry.Name(), "plan.json")
		if _, err := os.Lstat(file); errors.Is(err, os.ErrNotExist) {
			continue
		}
		raw, err := readRegular(file, maxManifest)
		if err != nil {
			return executionPlan{}, err
		}
		if digestBytes(raw) != binding.Plan {
			continue
		}
		var plan executionPlan
		if err := decodeStrict(raw, &plan); err != nil {
			return executionPlan{}, err
		}
		prepared, err := plan.prepared(binding.Plan)
		if err != nil || prepared.Assessment.Binding != binding {
			return executionPlan{}, errors.New("attempt and saved plan binding differ")
		}
		return plan, nil
	}
	return executionPlan{}, errors.New("saved attempt plan not found")
}

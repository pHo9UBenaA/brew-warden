package homebrew

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func (e Engine) Snapshot(ctx context.Context, binding domain.Binding) (domain.Digest, error) {
	if e.Collector == nil || !binding.Valid() {
		return "", errors.New("invalid reconciliation request")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	plan, attemptRoot, err := e.savedPlan(binding)
	if err != nil {
		return "", err
	}
	if plan.Schema != 3 {
		return "", errors.New("legacy execution attempt requires the matching older build for reconciliation")
	}
	return e.snapshotPublic(ctx, plan, attemptRoot)
}

func (e Engine) savedPlan(binding domain.Binding) (executionPlan, string, error) {
	directory := e.Collector.Directory
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return executionPlan{}, "", errors.New("private plan store unavailable")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) > 10000 {
		return executionPlan{}, "", errors.New("plan inventory unavailable or oversized")
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "collection-") {
			continue
		}
		if !entry.IsDir() {
			return executionPlan{}, "", errors.New("invalid plan directory")
		}
		file := filepath.Join(directory, entry.Name(), "plan.json")
		if _, err := os.Lstat(file); errors.Is(err, os.ErrNotExist) {
			continue
		}
		raw, err := readRegular(file, maxManifest)
		if err != nil {
			return executionPlan{}, "", err
		}
		if digestBytes(raw) != binding.Plan {
			continue
		}
		var plan executionPlan
		if err := decodeStrict(raw, &plan); err != nil {
			return executionPlan{}, "", err
		}
		prepared, err := plan.prepared(binding.Plan)
		if err != nil || prepared.Assessment.Binding != binding {
			return executionPlan{}, "", errors.New("attempt and saved plan binding differ")
		}
		return plan, filepath.Dir(file), nil
	}
	return executionPlan{}, "", errors.New("saved attempt plan not found")
}

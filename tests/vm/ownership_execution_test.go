//go:build vmacceptance

package vm

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Start after a fresh packaged jq install: oniguruma must be dependency-only.
// Never provision a receipt by hand; exercise Homebrew's public ownership change.
func TestLiveDistributionPromotesExplicitInstallOwnership(t *testing.T) {
	binary := os.Getenv("BREWWARDEN_VM_DISTRIBUTION_BINARY")
	if binary == "" {
		t.Skip("requires packaged binary and fresh jq dependency in a disposable VM")
	}
	requireDisposableMac(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	observe := func() (bool, int64) {
		t.Helper()
		command := exec.CommandContext(ctx, "/opt/homebrew/bin/brew", "info", "--json=v2", "--formula", "oniguruma")
		command.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1")
		raw, err := command.Output()
		if err != nil {
			t.Fatal("cannot observe ownership", err)
		}
		var info struct {
			Formulae []struct {
				Installed []struct {
					OnRequest *bool `json:"installed_on_request"`
					Time      int64 `json:"time"`
				} `json:"installed"`
			} `json:"formulae"`
		}
		if err := json.Unmarshal(raw, &info); err != nil || len(info.Formulae) != 1 || len(info.Formulae[0].Installed) != 1 {
			t.Fatalf("invalid ownership observation: %s: %v", raw, err)
		}
		installed := info.Formulae[0].Installed[0]
		if installed.OnRequest == nil || installed.Time <= 0 {
			t.Fatal("ownership or installation time missing")
		}
		return *installed.OnRequest, installed.Time
	}
	run := func(name string) {
		t.Helper()
		command := exec.CommandContext(ctx, binary, "brew", "install", name)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("packaged install %s: %s: %v", name, out, err)
		}
	}
	onRequest, installedAt := observe()
	if onRequest {
		t.Fatal("requires current dependency-only oniguruma")
	}
	rack := "/opt/homebrew/Cellar/oniguruma"
	before, err := kegSnapshot(rack)
	if err != nil {
		t.Fatal(err)
	}
	run("jq")
	if after, err := kegSnapshot(rack); err != nil || before != after {
		t.Fatalf("non-root dependency changed: before=%s after=%s error=%v", before, after, err)
	}
	if requested, _ := observe(); requested {
		t.Fatal("non-root dependency was promoted")
	}

	keg, err := filepath.EvalSymlinks("/opt/homebrew/opt/oniguruma")
	if err != nil {
		t.Fatal(err)
	}
	payload := func() map[string]string {
		t.Helper()
		entries, err := os.ReadDir(keg)
		if err != nil {
			t.Fatal(err)
		}
		snapshot := map[string]string{}
		for _, entry := range entries {
			if entry.Name() == "INSTALL_RECEIPT.json" {
				continue
			}
			snapshot[entry.Name()], err = kegSnapshot(filepath.Join(keg, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
		}
		return snapshot
	}
	originalPayload := payload()
	run("oniguruma")
	requested, afterTime := observe()
	if !requested || afterTime != installedAt {
		t.Fatalf("want promotion without reinstall: requested=%t installed_at=%d -> %d", requested, installedAt, afterTime)
	}
	if after := payload(); !maps.Equal(originalPayload, after) {
		t.Fatal("ownership promotion changed payload", originalPayload, after)
	}
}

package homebrew

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func runtimeFixture(t *testing.T) (Runtime, string, string) {
	t.Helper()
	prefix := t.TempDir()
	if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(prefix, "Library/Homebrew"), 0700); err != nil {
		t.Fatal(err)
	}
	brew := []byte("#!/bin/sh\nexit 0\n")
	library := []byte("ruby fixture\n")
	if err := os.WriteFile(filepath.Join(prefix, "bin/brew"), brew, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prefix, "Library/Homebrew/fixture.rb"), library, 0644); err != nil {
		t.Fatal(err)
	}
	manifest := runtimeManifest{Schema: 2, BrewRevision: brewRevision, Files: []runtimeEntry{
		{Path: "brew/Library/Homebrew/fixture.rb", Mode: 0644, SHA256: digestBytes(library)},
		{Path: "brew/bin/brew", Mode: 0755, SHA256: digestBytes(brew)},
	}}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return Runtime{ExpectedSHA256: digestBytes(append(raw, '\n'))}, prefix, filepath.Join(t.TempDir(), "runtime")
}

func TestInstalledRuntimeIsCopiedAndBoundToExactVersion(t *testing.T) {
	r, prefix, destination := runtimeFixture(t)
	actual, err := r.materializeFrom(destination, prefix)
	if err != nil || actual != r.ExpectedSHA256 {
		t.Fatal("runtime fingerprint mismatch", actual, err)
	}
	copied, err := os.ReadFile(filepath.Join(destination, "brew/Library/Homebrew/fixture.rb"))
	if err != nil || string(copied) != "ruby fixture\n" {
		t.Fatal("installed library was not copied", err)
	}
	if err := os.WriteFile(filepath.Join(prefix, "Library/Homebrew/fixture.rb"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.materializeFrom(filepath.Join(t.TempDir(), "changed"), prefix); err == nil {
		t.Fatal("changed installed code accepted as supported version")
	}
}

// Explicit probe of a previously reviewed, disposable Homebrew tree. Never
// reads or mutates the maintainer's installed prefix during the baseline suite.
func TestLiveInstalledRuntimeFingerprint(t *testing.T) {
	prefix := os.Getenv("BREWWARDEN_LIVE_PREFIX")
	if prefix == "" {
		t.Skip("requires a disposable reviewed Homebrew tree")
	}
	got, err := (Runtime{}).materializeFrom(filepath.Join(t.TempDir(), "inspection"), prefix)
	if err != nil {
		t.Fatal("installed runtime source differs from reviewed versions", got, err)
	}
	if got != supportedRuntimeDigest {
		if _, err := reviewedBrewSource(prefix); err != nil {
			t.Fatal("installed runtime has no reviewed release identity", got, err)
		}
	}
}

func TestInstalledRuntimeDoesNotTrustAVersionBannerOrUnreviewedGitCommit(t *testing.T) {
	_, prefix, destination := runtimeFixture(t)
	brew := filepath.Join(prefix, "bin/brew")
	if err := os.WriteFile(brew, []byte("#!/bin/sh\necho 'Homebrew 7.0.6'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	commands := [][]string{{"init", "-q"}, {"add", "bin/brew", "Library/Homebrew/fixture.rb"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.org", "commit", "-q", "-m", "unreviewed"}, {"tag", "7.0.6"}}
	for _, args := range commands {
		command := exec.Command("git", args...)
		command.Dir = prefix
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("isolated Git fixture %v: %v: %s", args, err, out)
		}
	}
	if _, err := (Runtime{}).materializeFrom(destination, prefix); err == nil {
		t.Fatal("unreviewed implementation authorized by version banner or Git tag")
	}
}

func TestInstalledRuntimeRejectsUnreviewedInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, string)
	}{
		{"extra library code", func(t *testing.T, prefix string) {
			if err := os.WriteFile(filepath.Join(prefix, "Library/Homebrew/injected.rb"), []byte("unexpected"), 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing executable", func(t *testing.T, prefix string) {
			if err := os.Remove(filepath.Join(prefix, "bin/brew")); err != nil {
				t.Fatal(err)
			}
		}},
		{"unsafe symlink", func(t *testing.T, prefix string) {
			if err := os.Symlink("/etc/passwd", filepath.Join(prefix, "Library/Homebrew/injected.rb")); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink parent", func(t *testing.T, prefix string) {
			if err := os.Rename(filepath.Join(prefix, "Library/Homebrew"), filepath.Join(prefix, "other")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("../other", filepath.Join(prefix, "Library/Homebrew")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, prefix, destination := runtimeFixture(t)
			tc.change(t, prefix)
			if _, err := r.materializeFrom(destination, prefix); err == nil {
				t.Fatal("unsupported installed runtime accepted")
			}
		})
	}
	r, prefix, destination := runtimeFixture(t)
	r.ExpectedSHA256 = domain.Digest(strings.Repeat("b", 64))
	if _, err := r.materializeFrom(destination, prefix); err == nil {
		t.Fatal("different pinned version accepted")
	}
}

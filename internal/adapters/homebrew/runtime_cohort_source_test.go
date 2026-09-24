package homebrew

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Optional offline source-matrix test: supply a local upstream Git object store
// containing all reviewed commits. The isolated shared clone is mutated; the
// input repository and the host Homebrew installation are never mutated.
func TestReviewedReleaseGitSourceMatrix(t *testing.T) {
	upstream := os.Getenv("BREWWARDEN_REVIEWED_BREW_GIT")
	if upstream == "" {
		t.Skip("set BREWWARDEN_REVIEWED_BREW_GIT to an offline upstream checkout")
	}
	resolved, err := filepath.EvalSymlinks(upstream)
	if err != nil || resolved == "/opt/homebrew" || strings.HasPrefix(resolved, "/opt/homebrew/") {
		t.Fatal("requires an explicit non-installed upstream source tree", err)
	}
	prefix := filepath.Join(t.TempDir(), "release")
	command := exec.Command("/usr/bin/git", "clone", "--quiet", "--shared", "--no-checkout", resolved, prefix)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("cannot clone isolated upstream fixture: %v: %s", err, output)
	}
	revisions := make([]string, 0, len(reviewedBrewRevisions))
	for revision := range reviewedBrewRevisions {
		revisions = append(revisions, revision)
	}
	sort.Strings(revisions)
	for _, revision := range revisions {
		t.Run(reviewedBrewRevisions[revision], func(t *testing.T) {
			command := exec.Command("/usr/bin/git", "-C", prefix, "checkout", "--quiet", "--force", "--detach", revision)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("reviewed upstream commit unavailable: %v: %s", err, output)
			}
			if got, err := reviewedBrewSource(prefix); err != nil || got != revision {
				t.Fatalf("real upstream source not selected: %s %v", got, err)
			}
			destination := filepath.Join(t.TempDir(), "inspection")
			if digest, err := (Runtime{}).materializeFrom(destination, prefix); err != nil || !digest.Valid() {
				t.Fatalf("real release source cannot be copied and bound: %s %v", digest, err)
			}
			modified := filepath.Join(prefix, "Library/Homebrew/unreviewed.rb")
			if err := os.WriteFile(modified, []byte("# unreviewed executable source\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := reviewedBrewSource(prefix); err == nil {
				t.Fatal("untracked executable Homebrew source accepted")
			}
			if err := os.Remove(modified); err != nil {
				t.Fatal(err)
			}
		})
	}
}

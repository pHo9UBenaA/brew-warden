package homebrew

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func TestExecutionMetadataMatchesArtifactAndDependencySet(t *testing.T) {
	candidates := metadataFixture().Formulae
	candidate := candidates[0]
	dependency := candidates[1].artifact()
	node := domain.Node{Artifact: candidate.artifact(), Dependencies: []domain.Artifact{dependency}}
	for _, tc := range []struct {
		name   string
		mutate func(*formulaMetadata)
		want   string
	}{
		{"matching", func(*formulaMetadata) {}, ""},
		{"changed bottle", func(f *formulaMetadata) { f.Rebuild++ }, "execution metadata differs"},
		{"missing dependency", func(f *formulaMetadata) { f.Dependencies = nil }, "execution dependency graph changed"},
		{"extra dependency", func(f *formulaMetadata) { f.Dependencies = append(f.Dependencies, "xz") }, "execution dependency graph changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			formula := candidate
			formula.Dependencies = append([]string{}, candidate.Dependencies...)
			tc.mutate(&formula)
			err := matchExecutionMetadata([]formulaMetadata{formula}, []string{candidate.Name}, []domain.Node{node})
			if tc.want == "" {
				if err != nil {
					t.Fatalf("matching execution metadata rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestPublicOperationLockExcludesConcurrentMutation(t *testing.T) {
	directory := t.TempDir()
	lock, err := acquireOperationLock(directory)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := acquireOperationLock(directory); err == nil {
		second.Close()
		t.Fatal("second mutation acquired the active operation lock")
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	fresh, err := acquireOperationLock(directory)
	if err != nil {
		t.Fatal("stopped operation prevented fresh retry", err)
	}
	defer fresh.Close()
}

func TestPublicSessionCloseRemovesCurrentWorkspace(t *testing.T) {
	directory := t.TempDir()
	root := filepath.Join(directory, "collection-12345")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireOperationLock(directory)
	if err != nil {
		t.Fatal(err)
	}
	s := &publicSession{w: workspace{root}, lock: lock}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("completed session retained execution inputs", err)
	}
}

func TestInstalledLinkObservesPartialPourAndRejectsMismatchedRecords(t *testing.T) {
	prefix, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"Cellar/jq/1.8.2", "Cellar/jq/old", "opt", "var/homebrew/linked"} {
		if err := os.MkdirAll(filepath.Join(prefix, path), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	assertLink := func(kegOnly, wantIncomplete bool) {
		t.Helper()
		version, incomplete, err := installedLink(prefix, "jq", kegOnly)
		if err != nil || version == nil {
			t.Fatalf("want active 1.8.2 for kegOnly=%t: version_missing=%t error=%v", kegOnly, version == nil, err)
		}
		if *version != "1.8.2" || incomplete != wantIncomplete {
			t.Fatalf("kegOnly=%t: want version=1.8.2 incomplete=%t, got version=%q incomplete=%t", kegOnly, wantIncomplete, *version, incomplete)
		}
	}
	opt := filepath.Join(prefix, "opt/jq")
	if err := os.Symlink("../Cellar/jq/1.8.2", opt); err != nil {
		t.Fatal(err)
	}
	// An opt-only normal formula is incomplete; an opt-only keg-only formula is complete.
	assertLink(false, true)
	assertLink(true, false)
	record := filepath.Join(prefix, "var/homebrew/linked/jq")
	if err := os.Symlink("../../../Cellar/jq/1.8.2", record); err != nil {
		t.Fatal(err)
	}
	// The matching linked-keg record completes a normal formula's link step.
	assertLink(false, false)
	if err := os.Remove(record); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../Cellar/jq/old", record); err != nil {
		t.Fatal(err)
	}
	if _, _, err := installedLink(prefix, "jq", false); err == nil {
		t.Fatal("linked record for a different keg was accepted")
	}
	if err := os.Remove(record); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, []byte("not a Homebrew link"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := installedLink(prefix, "jq", false); err == nil {
		t.Fatal("non-symlink linked record was accepted")
	}
}

func TestPublicActionsUseInstalledAndCandidateVersions(t *testing.T) {
	node := domain.Node{Artifact: metadataFixture().Formulae[0].artifact()}
	active := node.Artifact.Version
	normal := installedFormula{
		Name: node.Artifact.Name, ActiveVersion: &active,
		Installed: []installedVersion{{Version: active, OnRequest: true, Poured: true, Built: true, Options: []string{}}},
	}
	for _, tc := range []struct {
		name   string
		mutate func(*installedFormula)
		want   string
	}{
		{"current requested root", func(*installedFormula) {}, "keep"},
		{"current dependency promoted to root", func(s *installedFormula) { s.Installed[0].OnRequest = false }, "install"},
		{"absent", func(s *installedFormula) {
			s.Installed = nil
			s.ActiveVersion = nil
		}, "install"},
		{"outdated", func(s *installedFormula) {
			old := "1.0"
			s.ActiveVersion = &old
			s.Installed[0].Version = old
			s.Outdated = true
		}, "upgrade"},
		{"newer installed", func(s *installedFormula) {
			newer := "999.0"
			s.ActiveVersion = &newer
			s.Installed[0].Version = newer
		}, ""},
		{"active version missing", func(s *installedFormula) {
			missing := "missing"
			s.ActiveVersion = &missing
		}, ""},
		{"duplicate active version with unsupported flags", func(s *installedFormula) {
			unsupported := s.Installed[0]
			unsupported.Poured = false
			s.Installed = append(s.Installed, unsupported)
		}, ""},
		{"pinned", func(s *installedFormula) { s.Pinned = true }, ""},
		{"unlinked", func(s *installedFormula) { s.ActiveVersion = nil }, ""},
		{"partial pour", func(s *installedFormula) { s.LinkIncomplete = true }, ""},
		{"source installation", func(s *installedFormula) { s.Installed[0].Poured = false }, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := normal
			state.Installed = append([]installedVersion{}, normal.Installed...)
			tc.mutate(&state)
			got, err := publicActions([]installedFormula{state}, []domain.Node{node}, collectionInputs{Operation: "install", Targets: []string{node.Artifact.Name}})
			if tc.want == "" {
				if err == nil {
					t.Fatal("unsupported state accepted")
				}
			} else if err != nil || len(got) != 1 || got[0].Operation != tc.want {
				t.Fatalf("want one %q action, got %+v: error=%v", tc.want, got, err)
			}
		})
	}
	normal.Installed[0].OnRequest = false
	for _, request := range []collectionInputs{{Operation: "install", Targets: []string{"another-root"}}, {Operation: "upgrade", Targets: []string{node.Artifact.Name}}} {
		actions, err := publicActions([]installedFormula{normal}, []domain.Node{node}, request)
		if err != nil || len(actions) != 1 || actions[0].Operation != "keep" {
			t.Fatalf("non-install-root must retain dependency ownership: request=%+v actions=%+v err=%v", request, actions, err)
		}
	}
	absent := installedFormula{Name: node.Artifact.Name, Installed: []installedVersion{}}
	if _, err := publicActions([]installedFormula{absent}, []domain.Node{node}, collectionInputs{Operation: "upgrade", Targets: []string{node.Artifact.Name}}); err == nil {
		t.Fatal("upgrade of absent root accepted")
	}
}

func TestPublicOperationLockRejectsUnsafeFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "shared file"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "execution.lock")
			if kind == "symlink" {
				target := filepath.Join(directory, "unrelated")
				if err := os.WriteFile(target, []byte("preserve"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if lock, err := acquireOperationLock(directory); err == nil {
				lock.Close()
				t.Fatal("unsafe lock accepted")
			}
		})
	}
}

package homebrew

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func TestPublicOperationLockExcludesConcurrentMutation(t *testing.T) {
	directory := t.TempDir()
	lock, err := acquireOperationLock(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireOperationLock(directory); err == nil {
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
	if err := os.Mkdir(root, 0700); err != nil {
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

func TestInstalledLinkRejectsPartialPourAndMismatchedRecords(t *testing.T) {
	prefix, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"Cellar/jq/1.8.2", "Cellar/jq/old", "opt", "var/homebrew/linked"} {
		if err := os.MkdirAll(filepath.Join(prefix, path), 0700); err != nil {
			t.Fatal(err)
		}
	}
	opt := filepath.Join(prefix, "opt/jq")
	if err := os.Symlink("../Cellar/jq/1.8.2", opt); err != nil {
		t.Fatal(err)
	}
	if version, incomplete, err := installedLink(prefix, "jq", false); err != nil || !incomplete || version == nil || *version != "1.8.2" {
		t.Fatal("partially poured keg was not marked incomplete for the after-state", version, incomplete, err)
	}
	if version, incomplete, err := installedLink(prefix, "jq", true); err != nil || incomplete || version == nil || *version != "1.8.2" {
		t.Fatal("keg-only formula without shared-prefix link was rejected", version, incomplete, err)
	}
	record := filepath.Join(prefix, "var/homebrew/linked/jq")
	if err := os.Symlink("../../../Cellar/jq/1.8.2", record); err != nil {
		t.Fatal(err)
	}
	if version, incomplete, err := installedLink(prefix, "jq", false); err != nil || incomplete || version == nil || *version != "1.8.2" {
		t.Fatal("fully linked keg was not recognized", version, incomplete, err)
	}
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
	if err := os.WriteFile(record, []byte("not a Homebrew link"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := installedLink(prefix, "jq", false); err == nil {
		t.Fatal("non-symlink linked record was accepted")
	}
}

func TestPublicActionsUseInstalledAndCandidateVersions(t *testing.T) {
	node := domain.Node{Artifact: metadataFixture().Formulae[0].artifact()}
	active := node.Artifact.Version
	normal := installedFormula{Name: node.Artifact.Name, LinkedKeg: &active, Installed: []installedVersion{{Version: active, Poured: true, Built: true, Options: []string{}}}}
	for _, tc := range []struct {
		name   string
		mutate func(*installedFormula)
		want   string
	}{
		{"current", func(*installedFormula) {}, "keep"},
		{"absent", func(s *installedFormula) { s.Installed = nil; s.LinkedKeg = nil }, "install"},
		{"outdated", func(s *installedFormula) {
			old := "1.0"
			s.LinkedKeg = &old
			s.Installed[0].Version = old
			s.Outdated = true
		}, "upgrade"},
		{"newer installed", func(s *installedFormula) { old := "999.0"; s.LinkedKeg = &old; s.Installed[0].Version = old }, ""},
		{"pinned", func(s *installedFormula) { s.Pinned = true }, ""},
		{"unlinked", func(s *installedFormula) { s.LinkedKeg = nil }, ""},
		{"partial pour", func(s *installedFormula) { s.LinkIncomplete = true }, ""},
		{"source installation", func(s *installedFormula) { s.Installed[0].Poured = false }, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := normal
			state.Installed = append([]installedVersion{}, normal.Installed...)
			tc.mutate(&state)
			got, err := publicActions([]installedFormula{state}, []domain.Node{node}, collectionInputs{Operation: "install"})
			if tc.want == "" {
				if err == nil {
					t.Fatal("unsupported state accepted")
				}
			} else if err != nil || got[0].Operation != tc.want {
				t.Fatal(got, err)
			}
		})
	}
	absent := installedFormula{Name: node.Artifact.Name, Installed: []installedVersion{}}
	if _, err := publicActions([]installedFormula{absent}, []domain.Node{node}, collectionInputs{Operation: "upgrade", Targets: []string{node.Artifact.Name}}); err == nil {
		t.Fatal("upgrade of absent root accepted")
	}
}
func TestPublicOperationLock(t *testing.T) {
	directory := t.TempDir()
	first, err := acquireOperationLock(directory)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := acquireOperationLock(directory); err == nil {
		second.Close()
		t.Fatal("concurrent operation accepted")
	}
	first.Close()
	second, err := acquireOperationLock(directory)
	if err != nil {
		t.Fatal(err)
	}
	second.Close()
}

func TestPublicOperationLockRejectsUnsafeFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "shared file"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "execution.lock")
			if kind == "symlink" {
				target := filepath.Join(directory, "unrelated")
				if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0644); err != nil {
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

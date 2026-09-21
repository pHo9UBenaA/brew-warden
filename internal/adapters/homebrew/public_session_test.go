package homebrew

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

func TestPublicPlanCannotUseNativeRecovery(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(directory, "collection-public")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	plan := planFixture()
	plan.Schema = 2
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeRecord(filepath.Join(root, "plan.json"), raw); err != nil {
		t.Fatal(err)
	}
	prepared, err := plan.prepared(digestBytes(raw))
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{Collector: &Collector{Directory: directory}}
	if _, err := engine.savedPlan(prepared.Assessment.Binding); err != nil {
		t.Fatal("fixture must resolve to a valid saved plan", err)
	}
	if _, err := engine.Snapshot(context.Background(), prepared.Assessment.Binding); err == nil || err.Error() != "public execution recovery has not passed acceptance" {
		t.Fatal("native recovery accepted public attempt", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatal("recovery launched before capability rejection", err)
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
		{"source installation", func(s *installedFormula) { s.Installed[0].Poured = false }, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := normal
			state.Installed = append([]installedVersion{}, normal.Installed...)
			tc.mutate(&state)
			got, err := publicActions([]installedFormula{state}, []domain.Node{node}, nativeInputs{Operation: "install"})
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
	if _, err := publicActions([]installedFormula{absent}, []domain.Node{node}, nativeInputs{Operation: "upgrade", Targets: []string{node.Artifact.Name}}); err == nil {
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

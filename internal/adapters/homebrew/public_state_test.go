package homebrew

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledObservationReusesExactBytesAndSeparatesChangedState(t *testing.T) {
	w := workspace{t.TempDir()}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	states := []installedFormula{{Name: "jq", Installed: []installedVersion{}}}
	first, err := w.saveInstalledState(states)
	if err != nil || !first.Valid() {
		t.Fatalf("cannot save installed observation: digest=%s error=%v", first, err)
	}
	path := filepath.Join(w.root, "states", string(first)+".json")
	raw, err := os.ReadFile(path)
	if err != nil || digestBytes(raw) != first {
		t.Fatalf("stored bytes do not identify observation: raw=%q digest=%s error=%v", raw, first, err)
	}
	if second, err := w.saveInstalledState(states); err != nil || second != first {
		t.Fatalf("unchanged observation must reuse identity: first=%s second=%s error=%v", first, second, err)
	}
	states[0].Installed = []installedVersion{{Version: "1.8.2", Poured: true, Built: true}}
	if changed, err := w.saveInstalledState(states); err != nil || changed == first {
		t.Fatalf("installed version must change observation identity: before=%s after=%s error=%v", first, changed, err)
	}
	if retained, err := os.ReadFile(path); err != nil || !bytes.Equal(retained, raw) {
		t.Fatalf("new state overwrote earlier observation: before=%q after=%q error=%v", raw, retained, err)
	}
}

func TestInstalledObservationRejectsSubstitutionAtExistingDigestPath(t *testing.T) {
	for _, kind := range []string{"changed bytes", "symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			w := workspace{t.TempDir()}
			if err := w.initialize(); err != nil {
				t.Fatal(err)
			}
			states := []installedFormula{{Name: "jq", Installed: []installedVersion{}}}
			digest, err := w.saveInstalledState(states)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(w.root, "states", string(digest)+".json")
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "changed bytes":
				err = os.WriteFile(path, []byte("substituted observation"), 0600)
			case "symlink":
				target := filepath.Join(t.TempDir(), "original.json")
				if err := os.WriteFile(target, original, 0600); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(target, path)
			case "directory":
				err = os.Mkdir(path, 0700)
			}
			if err != nil {
				t.Fatal("cannot replace observation fixture", err)
			}
			if got, err := w.saveInstalledState(states); err == nil || got != "" {
				t.Fatalf("substituted observation accepted: digest=%s error=%v", got, err)
			}
		})
	}
}

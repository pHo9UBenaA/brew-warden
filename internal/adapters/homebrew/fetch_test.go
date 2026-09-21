package homebrew

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLICacheResultsRequireEverySelectedBottle(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := workspace{root: root}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	candidate := formulaMetadata{Name: "jq", Version: "1.8.2"}
	original := bottleFixture(t)
	candidate.BottleSHA256 = digestBytes(original)
	file := filepath.Join(root, "cache", strings.Repeat("a", 64)+"--"+nativeBottleName(candidate))
	if err := writeNew(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{"", "Warning: bottle unavailable\n", file + "\n" + file + "\n", strings.Replace(file, "arm64_tahoe", "arm64_sonoma", 1) + "\n", strings.Replace(file, "jq--", "other--", 1) + "\n", file + "\r\n"} {
		if _, err := w.bottleCachePaths([]byte(output), []formulaMetadata{candidate}); err == nil {
			t.Fatalf("incomplete or wrong bottle result accepted: %q", output)
		}
	}
	raw, err := w.bottleCachePaths([]byte(file+"\n"), []formulaMetadata{candidate})
	if err != nil {
		t.Fatal(err)
	}
	// Even a correct cache-path response cannot attest to unchanged bytes.
	if err := os.WriteFile(file, []byte("substitution"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.copyDownloads(raw, []formulaMetadata{candidate}); err == nil {
		t.Fatal("changed CLI-selected bottle accepted")
	}
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.copyDownloads(raw, []formulaMetadata{candidate}); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, file); err != nil {
		t.Fatal(err)
	}
	if _, err := w.bottleCachePaths([]byte(file+"\n"), []formulaMetadata{candidate}); err == nil {
		t.Fatal("cache symlink escape accepted")
	}
}

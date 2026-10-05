package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHygieneFiles(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"valid", "English text\n", ""},
		{"newline", "English text", "missing final newline"},
		{"whitespace", "English text \n", "trailing whitespace"},
		{"binary", "\x00\n", "control"},
		{"invalid encoding", "\xff\n", "invalid UTF-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			err := hygiene(root)
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	t.Run("symlink escape", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Symlink(t.TempDir(), filepath.Join(root, "outside")); err != nil {
			t.Fatal(err)
		}
		if err := hygiene(root); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("want symlink rejection, got %v", err)
		}
	})
	t.Run("finder metadata is exempt but symlinks are not", func(t *testing.T) {
		for _, rel := range []string{".DS_Store", filepath.Join("cmd", ".DS_Store")} {
			path := filepath.Join(t.TempDir(), rel)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("\x00\xff"), 0o600); err != nil {
				t.Fatal(err)
			}
			root, err := filepath.Abs(filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			if err := hygiene(root); err != nil {
				t.Fatalf("Finder metadata %s blocked hygiene: %v", rel, err)
			}
		}
		root := t.TempDir()
		if err := os.Symlink(t.TempDir(), filepath.Join(root, ".DS_Store")); err != nil {
			t.Fatal(err)
		}
		if err := hygiene(root); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("symlinked .DS_Store must still require review, got %v", err)
		}
	})
}

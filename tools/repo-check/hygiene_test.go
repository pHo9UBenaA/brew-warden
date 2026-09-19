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
			if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(tc.body), 0600); err != nil {
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
}

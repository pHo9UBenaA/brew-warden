package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

func textHygiene(b []byte) error {
	if !utf8.Valid(b) {
		return fmt.Errorf("invalid UTF-8")
	}
	for _, r := range string(b) {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			return fmt.Errorf("repository prose must be English; found CJK text")
		}
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.Is(unicode.Cf, r) {
			return fmt.Errorf("unexpected control or format character U+%04X", r)
		}
	}
	return nil
}
func hygiene(root string) error {
	return walkSources(root, func(rel string, entry fs.DirEntry) error {
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("repository symlink requires explicit review: %s", rel)
		}
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return err
		}
		// Binary fixtures/assets need a documented policy extension; silently skipping
		// them would allow executable files to evade the repository text check.
		if err = textHygiene(b); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		if len(b) > 0 && b[len(b)-1] != '\n' {
			return fmt.Errorf("missing final newline: %s", rel)
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.TrimRight(line, " \t") != line {
				return fmt.Errorf("trailing whitespace: %s", rel)
			}
		}
		return nil
	})
}

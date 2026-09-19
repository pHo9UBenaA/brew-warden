package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files["go.mod"] = "module example.test/tool\n\ngo 1.24.0\n"
	for p, s := range files {
		target := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func TestArchitecture(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"valid inward wiring", map[string]string{
			"internal/domain/value.go":         "package domain\n",
			"internal/ports/port.go":           "package ports\nimport _ \"example.test/tool/internal/domain\"\n",
			"internal/application/use.go":      "package application\nimport _ \"example.test/tool/internal/ports\"\n",
			"internal/adapters/store/store.go": "package store\nimport _ \"example.test/tool/internal/ports\"\n",
			"internal/composition/wire.go":     "package composition\nimport (_ \"example.test/tool/internal/application\"; _ \"example.test/tool/internal/adapters/store\")\n",
			"cmd/tool/main.go":                 "package main\nimport _ \"example.test/tool/internal/composition\"\nfunc main(){}\n",
		}, ""},
		{"resolved reverse edge", map[string]string{"internal/domain/p.go": "package domain\nimport _ \"example.test/tool/internal/adapters/store\"\n", "internal/adapters/store/p.go": "package store\n"}, "forbidden domain dependency"},
		{"adapter to use case", map[string]string{"internal/application/p.go": "package application\n", "internal/adapters/store/p.go": "package store\nimport _ \"example.test/tool/internal/application\"\n"}, "forbidden adapter:store dependency"},
		{"core runtime", map[string]string{"internal/domain/p.go": "package domain\nimport _ \"os\"\n"}, "forbidden core builtin os"},
		{"platform file", map[string]string{"internal/domain/p_windows.go": "//go:build windows\n\npackage domain\nimport _ \"os/exec\"\n"}, "forbidden core builtin os/exec"},
		{"unresolved", map[string]string{"internal/domain/p.go": "package domain\nimport _ \"example.test/tool/internal/domain/missing\"\n"}, "unresolved local import"},
		{"cycle", map[string]string{"internal/domain/a/p.go": "package a\nimport _ \"example.test/tool/internal/domain/b\"\n", "internal/domain/b/p.go": "package b\nimport _ \"example.test/tool/internal/domain/a\"\n"}, "package import cycle"},
		{"external test", map[string]string{"internal/domain/p.go": "package domain\n", "internal/domain/p_test.go": "package domain_test\nimport (_ \"example.test/tool/internal/domain\"; _ \"testing\")\n"}, ""},
		{"unclassified", map[string]string{"misc/p.go": "package misc\n"}, "unclassified Go source"},
		{"unsafe", map[string]string{"tools/p.go": "package tools\nimport _ \"unsafe\"\n"}, "prohibited import unsafe"},
		{"nested module", map[string]string{"other/go.mod": "module nested\n"}, "nested module"},
		{"generation", map[string]string{"internal/domain/p.go": "package domain\n//go:generate echo bad\n"}, "prohibited directive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := architecture(fixture(t, tc.files))
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

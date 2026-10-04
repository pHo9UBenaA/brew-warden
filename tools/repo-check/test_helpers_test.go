package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamedTestHelpersRequireCallerAttribution(t *testing.T) {
	for _, test := range []struct {
		name, source, want string
	}{
		{
			name:   "fixture failure without helper",
			source: "import \"testing\"\nfunc bottleFixture(t *testing.T) { t.Fatal(\"bad fixture\") }",
			want:   "test helper bottleFixture must begin with t.Helper()",
		},
		{
			name:   "helper after work is too late",
			source: "import \"testing\"\nfunc bottleFixture(t testing.TB) { t.Log(\"setup\"); t.Helper() }",
			want:   "test helper bottleFixture must begin with t.Helper()",
		},
		{
			name:   "helper on a different handle",
			source: "import \"testing\"\nfunc bottleFixture(t *testing.T, other *testing.T) { other.Helper() }",
			want:   "test helper bottleFixture must begin with t.Helper()",
		},
		{
			name:   "unnamed testing handle",
			source: "import \"testing\"\nfunc bottleFixture(_ testing.TB) {}",
			want:   "requires one named testing handle",
		},
		{
			name:   "renamed testing import",
			source: "import check \"testing\"\nfunc bottleFixture(tb check.TB) { tb.Fatal(\"setup\") }",
			want:   "test helper bottleFixture must begin with tb.Helper()",
		},
		{
			name:   "test prefix does not make a helper an entrypoint",
			source: "import \"testing\"\nfunc TestFixture(t *testing.T) string { return \"bottle\" }",
			want:   "test helper TestFixture must begin with t.Helper()",
		},
		{
			name:   "lowercase suffix is not a test entrypoint",
			source: "import \"testing\"\nfunc Testfixture(t *testing.T) { t.Fatal(\"setup\") }",
			want:   "test helper Testfixture must begin with t.Helper()",
		},
		{
			name:   "method is not a test entrypoint",
			source: "import \"testing\"\ntype fixture struct{}\nfunc (fixture) TestFixture(t *testing.T) { t.Fatal(\"setup\") }",
			want:   "test helper TestFixture must begin with t.Helper()",
		},
		{
			name: "valid entrypoints and callbacks",
			source: `import "testing"
func TestBottle(t *testing.T) { t.Run("digest", func(t *testing.T) { t.Log("digest") }) }
func BenchmarkBottle(b *testing.B) { b.Log("bottle") }
func FuzzBottle(f *testing.F) { f.Add("bottle"); f.Fuzz(func(t *testing.T, value string) { t.Log(value) }) }`,
		},
		{
			name: "attributed helpers",
			source: `import check "testing"
func bottleFixture(tb check.TB) string { tb.Helper(); return "bottle" }
func benchmarkFixture(b *check.B) { b.Helper(); b.Log("bottle") }
func fuzzFixture(f *check.F) { f.Helper(); f.Add("bottle") }`,
		},
		{
			name:   "unrelated qualified parameter",
			source: "import \"net/http\"\nfunc requestFixture(client *http.Client) {}",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "tests", "vm", "fixture_test.go")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			// The gate must inspect a file even when ordinary Go checks exclude it.
			content := "//go:build vmacceptance && darwin\n\npackage fixture\n\n" + test.source + "\n"
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			err := testHelpers(root)
			if test.want == "" {
				if err != nil {
					t.Fatalf("valid helper/entrypoint rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "fixture_test.go:") || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want source-located failure containing %q, got %v", test.want, err)
			}
		})
	}
}

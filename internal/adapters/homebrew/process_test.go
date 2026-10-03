package homebrew

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// Exercise the OS sandbox only in temporary files, never the installed prefix.
func TestWorkspaceSandboxAllowsDiagnosticsButProtectsFrozenInputs(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("requires macOS sandbox-exec")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "frozen.json")
	if err := os.WriteFile(input, []byte("verified"), 0600); err != nil {
		t.Fatal(err)
	}
	w := workspace{root: root}
	profile, err := w.sandbox("test", sandboxPermissions{}, []string{input})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path string
		allowed    bool
	}{
		{"diagnostic", filepath.Join(root, "diagnostic.log"), true},
		{"frozen input", input, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := exec.Command("/usr/bin/sandbox-exec", "-f", profile,
				"/bin/sh", "-c", `printf changed > "$1"`, "sandbox-test", tc.path)
			output, err := command.CombinedOutput()
			if (err == nil) != tc.allowed {
				t.Fatalf("write to %s: want allowed=%t, error=%v, output=%s", tc.path, tc.allowed, err, output)
			}
		})
	}
	if raw, err := os.ReadFile(input); err != nil || string(raw) != "verified" {
		t.Fatalf("sandbox changed frozen input: %q, error=%v", raw, err)
	}
}

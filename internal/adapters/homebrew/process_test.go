package homebrew

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestInvocationFailureClassificationSurvivesWorkspaceCleanup(t *testing.T) {
	for _, tc := range []struct {
		name, script, want string
		contextError       error
	}{
		{"exit", "echo 'secret-marker\033[31m' >&2; exit 17", "command exited with status 17", nil},
		{"cancelled", "exit 0", "context canceled", context.Canceled},
		{"deadline", "exit 0", "context deadline exceeded", context.DeadlineExceeded},
		{"overflow", "/usr/bin/head -c 9000000 /dev/zero", "output exceeded limit", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := workspace{t.TempDir()}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.contextError == context.Canceled {
				cancel()
			}
			if tc.contextError == context.DeadlineExceeded {
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer stop()
			}
			command := exec.CommandContext(ctx, "/bin/sh", "-c", tc.script)
			command.WaitDelay = time.Second
			_, err := w.invokeCommand(ctx, "info-0", command)
			if removeErr := os.RemoveAll(w.root); removeErr != nil {
				t.Fatal(removeErr)
			}
			if err == nil || !strings.Contains(err.Error(), "homebrew info-0: "+tc.want) || strings.Contains(err.Error(), "secret-marker") || strings.ContainsRune(err.Error(), '\x1b') {
				t.Fatalf("want safe %q after cleanup, got %v", tc.want, err)
			}
			if tc.contextError != nil && !errors.Is(err, tc.contextError) {
				t.Fatal("lost cancellation identity", err)
			}
		})
	}
}

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

package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise both shipped entrypoints with a tripwire brew executable. No host
// Homebrew is reachable through PATH and no installation is performed.
func TestEntrypointsNeverLaunchBrew(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "brew-invoked")
	if err := os.WriteFile(filepath.Join(dir, "brew"), []byte("#!/bin/sh\n/usr/bin/touch \"$BREWWARDEN_TEST_MARKER\"\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bwd", "brewwarden"} {
		binary := filepath.Join(dir, name)
		build := exec.Command("go", "build", "-o", binary, "./cmd/"+name)
		build.Dir = ".."
		if output, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build: %v\n%s", err, output)
		}
		for _, args := range [][]string{{"brew", "install", "wget"}, {"brew", "upgrade"}, {"brew", "install", "--help"}, {"brew", "bundle", "exec", "--install", "sh"}, {"doctor"}} {
			cmd := exec.Command(binary, args...)
			cmd.Env = []string{"PATH=" + dir, "HOME=" + dir, "XDG_CONFIG_HOME=" + dir, "BREWWARDEN_TEST_MARKER=" + marker}
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err == nil || cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 1 {
				t.Fatalf("%s %v: expected exit 1, got %v", name, args, err)
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), "execution_binding_unverified:") {
				t.Fatalf("unexpected output: %q %q", &stdout, &stderr)
			}
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("brew tripwire must remain untouched: %v", err)
	}
}

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRequiresPinnedTools(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"check.sh", "env.sh", "tool-versions.env"} {
		b, err := os.ReadFile(filepath.Join("../../scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "scripts", name), b, 0700); err != nil {
			t.Fatal(err)
		}
	}
	run := func(arg, want string) {
		t.Helper()
		c := exec.Command("sh", filepath.Join(root, "scripts/check.sh"), arg)
		c.Dir = root
		out, err := c.CombinedOutput()
		if err == nil || !strings.Contains(string(out), want) {
			t.Fatalf("check %s: want failure containing %q, got %v: %s", arg, want, err, out)
		}
	}
	run("lint", "Missing staticcheck")
	run("vuln", "Missing govulncheck")
	run("unknown", "Unknown check")
	if err := os.MkdirAll(filepath.Join(root, ".cache/tools"), 0700); err != nil {
		t.Fatal(err)
	}
	// An executable with unrelated build metadata must not stand in for the pin.
	binary := filepath.Join(root, ".cache/tools/staticcheck")
	c := exec.Command("go", "build", "-o", binary, ".")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("build substitute: %v: %s", err, out)
	}
	run("lint", "Wrong staticcheck version")
}

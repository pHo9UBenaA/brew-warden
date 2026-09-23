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

func TestFuzzCheckRejectsMissingTarget(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":                   "module example.org/isolated-fuzz-check\n\ngo 1.24.0\n",
		"tools/repo-check/main.go": "package main\nfunc main() {}\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"check.sh", "env.sh", "tool-versions.env"} {
		raw, err := os.ReadFile(filepath.Join("../../scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "scripts"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "scripts", name), raw, 0700); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("sh", "./scripts/check.sh", "fuzz")
	command.Dir = root
	command.Env = append(os.Environ(), "FUZZTIME=1x", "GOTOOLCHAIN=local", "GOPROXY=off", "GOWORK=off")
	out, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "Required fuzz target FuzzCommitMessage is missing") {
		t.Fatalf("missing fuzz target was treated as a successful check: %v: %s", err, out)
	}
}

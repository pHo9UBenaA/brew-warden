package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHooks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	// Build the real checker once. Hooks run it through a minimal go shim so this
	// integration test does not recursively execute the complete verification suite.
	checker := filepath.Join(root, "checker")
	build := exec.Command("go", "build", "-o", checker, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build checker: %v: %s", err, out)
	}
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"commit-msg", "pre-commit", "pre-push"} {
		b, err := os.ReadFile(filepath.Join(repo, ".githooks", name))
		if err != nil {
			t.Fatal(err)
		}
		write(".githooks/"+name, string(b))
	}
	write("scripts/verify.sh", "#!/bin/sh\nset -eu\nprintf verified > hook-verified\n")
	write("shim/go", "#!/bin/sh\nset -eu\n[ \"$1\" = run ]\n[ \"$2\" = ./tools/repo-check ]\nshift 2\nexec \"$CHECKER\" \"$@\"\n")
	write(".gitignore", "checker\nshim/\nhook-verified\n.cache/\n")
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "CHECKER="+checker, "PATH="+filepath.Join(root, "shim")+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Do not let Git's hook-local variables redirect this temporary repository.
	clean := env[:0]
	for _, v := range env {
		key, _, _ := strings.Cut(v, "=")
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES":
			continue
		}
		clean = append(clean, v)
	}
	env = clean
	run := func(wantOK bool, args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = env
		b, err := c.CombinedOutput()
		if (err == nil) != wantOK {
			t.Fatalf("git %v: %v: %s", args, err, b)
		}
		return string(b)
	}
	run(true, "init", "-q")
	run(true, "config", "user.name", "Hook Test")
	run(true, "config", "user.email", "hooks@example.invalid")
	run(true, "config", "commit.gpgsign", "false")
	run(true, "config", "core.hooksPath", ".githooks")
	write("sample.txt", "one\n")
	run(true, "add", ".")
	out := run(false, "commit", "-qm", "not conventional")
	if !strings.Contains(out, "Conventional Commit") {
		t.Fatalf("wrong rejection: %s", out)
	}
	if _, err := os.Stat(filepath.Join(root, "hook-verified")); err != nil {
		t.Fatal("pre-commit did not run verification")
	}
	run(true, "commit", "-qm", "chore: initialize hook fixture")
	first := strings.TrimSpace(run(true, "rev-parse", "HEAD"))
	write("sample.txt", "two\n")
	run(true, "add", "sample.txt")
	write("sample.txt", "three\n")
	out = run(false, "commit", "-qm", "test: partial stage")
	if !strings.Contains(out, "fully staged") {
		t.Fatalf("wrong partial-stage rejection: %s", out)
	}
	run(true, "add", "sample.txt")
	run(true, "commit", "-qm", "test: update fixture")
	tip := strings.TrimSpace(run(true, "rev-parse", "HEAD"))
	push := func(tip, base string, wantOK bool) {
		t.Helper()
		c := exec.Command(filepath.Join(root, ".githooks/pre-push"))
		c.Dir = root
		c.Env = env
		c.Stdin = strings.NewReader(fmt.Sprintf("refs/heads/main %s refs/heads/main %s\n", tip, base))
		b, err := c.CombinedOutput()
		if (err == nil) != wantOK {
			t.Fatalf("pre-push: %v: %s", err, b)
		}
	}
	push(tip, first, true)
	push(tip, strings.Repeat("0", 40), true)
	push(strings.Repeat("0", 40), tip, true)
	run(true, "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-qm", "bad historical message")
	bad := strings.TrimSpace(run(true, "rev-parse", "HEAD"))
	push(bad, first, false)
}

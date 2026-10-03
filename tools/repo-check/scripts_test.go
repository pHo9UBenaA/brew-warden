package main

import (
	"archive/tar"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeProbeArchivesCurrentFilesWithoutTrackedDeletions(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"deleted\nfile": "obsolete", "modified file": "old", "dangling": "old",
		".gitignore": "/.cache/\n/ignored\n", "tests/container/Dockerfile": "fixture",
		"scripts/container-check.sh": "exit 0\n",
		"fake-bin/docker":            "#!/bin/sh\nexit 37\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile("../../scripts/probe-container.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/probe-container.sh"), raw, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.org", "commit", "-qm", "fixture"}} {
		command := exec.Command("git", args...)
		command.Dir = root
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %s: %v", out, err)
		}
	}
	if err := os.Remove(filepath.Join(root, "deleted\nfile")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing-target", filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"modified file": "current", "new\nfile": "new", "ignored": "private"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("sh", "scripts/probe-container.sh", "--worktree")
	command.Dir = root
	command.Env = append(os.Environ(), "PATH="+filepath.Join(root, "fake-bin")+":"+os.Getenv("PATH"))
	out, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 37 {
		t.Fatalf("probe did not reach Docker boundary: %s: %v", out, err)
	}
	archives, err := filepath.Glob(filepath.Join(root, ".cache/container-probe.*/source.tar"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("want one archive: %q: %v", archives, err)
	}
	file, err := os.Open(archives[0])
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := tar.NewReader(file)
	contents := map[string]string{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == "dangling" && (header.Typeflag != tar.TypeSymlink || header.Linkname != "missing-target") {
			t.Fatal("lost dangling symlink", header)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		contents[header.Name] = string(data)
	}
	if _, found := contents["deleted\nfile"]; found {
		t.Fatal("archived deleted tracked path")
	}
	if _, found := contents["ignored"]; found {
		t.Fatal("archived ignored data")
	}
	if _, found := contents["dangling"]; !found {
		t.Fatal("omitted dangling symlink")
	}
	if contents["modified file"] != "current" || contents["new\nfile"] != "new" {
		t.Fatal("archive lost current worktree bytes", contents)
	}
}

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

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Exercise the shipped executable, including a lost parent after a durable start.
// The caller provisions an absent xz inside the explicitly selected VirtualMac.
func TestLiveDistributionParentCrash(t *testing.T) {
	binary := os.Getenv("BREWWARDEN_VM_DISTRIBUTION_BINARY")
	if binary == "" {
		t.Skip("requires an extracted distribution in a disposable VM")
	}
	model, err := exec.Command("/usr/sbin/sysctl", "-n", "hw.model").Output()
	if err != nil || !strings.HasPrefix(string(model), "VirtualMac") {
		t.Fatal("requires disposable VirtualMac")
	}
	if _, err := os.Lstat("/opt/homebrew/Cellar/xz"); !os.IsNotExist(err) {
		t.Fatal("requires absent xz fixture")
	}
	home, err := os.MkdirTemp(filepath.Dir(binary), "crash-home-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("retained distribution crash state", home)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	newCommand := func(args ...string) *exec.Cmd {
		c := exec.CommandContext(ctx, binary, args...)
		c.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL=C"}
		c.WaitDelay = 3 * time.Second
		return c
	}
	if output, err := newCommand("doctor").CombinedOutput(); err != nil {
		t.Fatal(string(output), err)
	}
	cache := filepath.Join(home, "Library/Application Support/brewwarden/collections")
	if err := os.MkdirAll(cache, 0700); err != nil {
		t.Fatal(err)
	}
	seedVMEvidenceCache(t, cache)
	command := newCommand("brew", "install", "xz")
	trigger := &killOnInstall{destination: os.Stdout, kill: func() error { return command.Process.Kill() }}
	command.Stdout, command.Stderr = trigger, trigger
	if err := command.Run(); err == nil || !trigger.triggered {
		t.Fatal("parent crash fixture not reached", err)
	}
	output, err := newCommand("status").CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte("unfinished")) {
		t.Fatal("lost parent was not recorded as unfinished", string(output), err)
	}
	// A surviving Homebrew child may still be finishing. Recovery must refuse until
	// it is absent, and must never signal or replay it from a remembered PID.
	deadline := time.Now().Add(30 * time.Second)
	for {
		output, err = newCommand("reconcile").CombinedOutput()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stopped attempt did not reconcile", string(output), err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	before, err := kegSnapshot("/opt/homebrew/Cellar")
	if err != nil {
		t.Fatal(err)
	}
	if output, err = newCommand("status").CombinedOutput(); err != nil {
		t.Fatal(string(output), err)
	}
	output, err = newCommand("history").CombinedOutput()
	if err != nil || !bytes.Contains(output, []byte("reconciled")) || bytes.Contains(output, []byte("succeeded")) {
		t.Fatal("recovery invented an execution result", string(output), err)
	}
	after, err := kegSnapshot("/opt/homebrew/Cellar")
	if err != nil || before != after {
		t.Fatal("history/status changed installed state", err)
	}
	// Retain public CLI output for acceptance evidence, not only the test verdict.
	raw, _ := json.Marshal(struct{ Binary, History string }{binary, string(output)})
	if err := os.WriteFile(filepath.Join(home, "acceptance.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

type killOnInstall struct {
	mu          sync.Mutex
	tail        string
	triggered   bool
	destination io.Writer
	kill        func() error
}

func (w *killOnInstall) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.destination.Write(p)
	w.tail += string(p)
	if !w.triggered && strings.Contains(w.tail, "Fetching downloads for:") {
		if killErr := w.kill(); killErr != nil {
			return n, killErr
		}
		w.triggered = true
	}
	if len(w.tail) > 4096 {
		w.tail = w.tail[len(w.tail)-4096:]
	}
	return n, err
}

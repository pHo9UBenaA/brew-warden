package tests

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Exercise the shipped binary after real parent death in a disposable VM.
// The caller provisions an absent xz and authorized gh credentials in that VM.
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
		c.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "LC_ALL=C"}
		if token := os.Getenv("BREWWARDEN_VM_GH_TOKEN"); token != "" {
			c.Env = append(c.Env, "GH_TOKEN="+token)
		}
		c.WaitDelay = 3 * time.Second
		return c
	}
	if output, err := newCommand("doctor").CombinedOutput(); err != nil {
		t.Fatal(string(output), err)
	}
	command := newCommand("brew", "install", "xz")
	trigger := &killOnInstall{destination: os.Stdout, kill: func() error { return command.Process.Kill() }}
	command.Stdout, command.Stderr = trigger, trigger
	if err := command.Run(); err == nil || !trigger.triggered {
		t.Fatal("parent crash fixture not reached", err)
	}
	file := filepath.Join(home, "Library/Application Support/brewwarden/collections/inflight.json")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("lost parent did not leave bounded process identity", err)
	}
	var owned struct {
		Schema, PID, Session int
		Plan, Attempt        string
	}
	if err := json.Unmarshal(raw, &owned); err != nil || owned.Schema != 1 || owned.PID <= 1 || owned.Session != owned.PID || len(owned.Plan) != 64 || len(owned.Attempt) != 64 {
		t.Fatal("invalid owned process record", err)
	}
	// If the child continues, another BrewWarden mutation must hold. The child
	// may finish first; in that case only the fresh retry may proceed.
	if err := syscall.Kill(owned.PID, 0); err == nil {
		output, err := newCommand("brew", "install", "xz").CombinedOutput()
		if err == nil || !strings.Contains(string(output), "another BrewWarden execution") && !strings.Contains(string(output), "may still be running") {
			t.Fatal("continued child was not excluded", string(output), err)
		}
	}
	deadline := time.Now().Add(2 * time.Minute)
	for syscall.Kill(owned.PID, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if syscall.Kill(owned.PID, 0) == nil {
		t.Fatal("owned child did not finish")
	}
	// No status/reconcile/replay: a new invocation rediscovers and rechecks
	// the installed prefix. It cannot report the lost command as successful.
	for {
		output, err := newCommand("brew", "install", "xz").CombinedOutput()
		if err == nil && strings.Contains(string(output), "Installation verified.") {
			break
		}
		if time.Now().After(deadline) || err == nil || (!strings.Contains(string(output), "owned Homebrew process") && !strings.Contains(string(output), "another BrewWarden execution")) {
			t.Fatal("fresh retry failed", string(output), err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		t.Fatal("stale in-flight record survived fresh retry", err)
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

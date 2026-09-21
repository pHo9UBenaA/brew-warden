package homebrew

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestProcessSessionHelper(t *testing.T) {
	mode := os.Getenv("BREWWARDEN_SESSION_HELPER")
	if mode == "member" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	if mode != "leader" {
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestProcessSessionHelper$")
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "BREWWARDEN_SESSION_HELPER=") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "BREWWARDEN_SESSION_HELPER=member")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Stdin = os.Stdin
	if err := command.Start(); err != nil {
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, command.Process.Pid)
	_ = command.Wait()
	os.Exit(0)
}

func TestSessionFindsChildInAnotherGroupAfterLeaderDies(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestProcessSessionHelper$")
	command.Env = append(os.Environ(), "BREWWARDEN_SESSION_HELPER=leader")
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	member, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || member <= 1 {
		t.Fatal("invalid child fixture", err)
	}
	defer func() {
		if sid, err := syscall.Getsid(member); err == nil && sid == command.Process.Pid {
			_ = syscall.Kill(member, syscall.SIGKILL)
		}
	}()
	group, err := syscall.Getpgid(member)
	if err != nil || group == command.Process.Pid {
		t.Fatal("fixture did not create a separate group", err)
	}
	_ = command.Process.Kill()
	_ = command.Wait()
	active, _, err := sessionInProcesses(command.Process.Pid, 0, []string{strconv.Itoa(member)})
	if err != nil || !active {
		t.Fatal("lost live child after session leader exited", err)
	}
}

func TestRecoveryChecksActualProcessSession(t *testing.T) {
	root := t.TempDir()
	command := exec.Command("/bin/sh", "-c", "read line")
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	raw, _ := json.Marshal(processRecord{PID: command.Process.Pid, Session: command.Process.Pid})
	if err := writeRecord(filepath.Join(root, "process-0.json"), raw); err != nil {
		t.Fatal(err)
	}
	if active, _, err := sessionInProcesses(command.Process.Pid, 0, []string{strconv.Itoa(command.Process.Pid)}); err != nil || !active {
		t.Fatal("active session not found", err)
	}
	input.Close()
	_ = command.Wait()
	if active, changed, err := sessionInProcesses(command.Process.Pid, 0, []string{strconv.Itoa(command.Process.Pid)}); err != nil || active || !changed {
		t.Fatal("disappeared process reported as a stable snapshot", err)
	}
	if err := os.WriteFile(filepath.Join(root, "process-0.json"), []byte(`{"pid":0}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stoppedProcessSessions(root); err == nil {
		t.Fatal("invalid process ID accepted")
	}
}

func TestRecoveryRecordsPartialPayloadWithoutFollowingLinks(t *testing.T) {
	prefix := t.TempDir()
	nodes := planFixture().Nodes
	root := filepath.Join(prefix, "Cellar/jq/1.8.2")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "INSTALL_RECEIPT.json"), []byte("interrupted JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "unrelated")
	if err := os.WriteFile(outside, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "external-link")); err != nil {
		t.Fatal(err)
	}
	first, err := observeSelectedPaths(context.Background(), prefix, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("changed outside selected keg"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := observeSelectedPaths(context.Background(), prefix, nodes)
	if err != nil || string(first) != string(second) {
		t.Fatal("followed unrelated link", err)
	}
	if err := os.WriteFile(filepath.Join(root, "INSTALL_RECEIPT.json"), []byte("different partial state"), 0600); err != nil {
		t.Fatal(err)
	}
	third, err := observeSelectedPaths(context.Background(), prefix, nodes)
	if err != nil || string(first) == string(third) {
		t.Fatal("missed actual payload change", err)
	}
}

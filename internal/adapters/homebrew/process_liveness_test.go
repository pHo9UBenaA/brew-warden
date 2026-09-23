package homebrew

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// A session can contain privileged descendants. User-only ps inventories
// would report that session as stopped after its unprivileged parent exits.
func TestProcessInventoryIncludesOtherUsers(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("requires an unprivileged process to test a different user's PID")
	}
	own, err := exec.Command("/bin/ps", "-U", strconv.Itoa(os.Getuid()), "-o", "pid=").Output()
	if err != nil {
		t.Fatal(err)
	}
	ownSessions := map[int]bool{}
	for _, field := range strings.Fields(string(own)) {
		pid, err := strconv.Atoi(field)
		if err != nil {
			t.Fatal(err)
		}
		if sid, err := processSessionID(pid); err == nil {
			ownSessions[sid] = true
		}
	}
	all, err := exec.Command("/bin/ps", "-A", "-o", "pid=,uid=").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(all), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		uid, uidErr := strconv.Atoi(fields[1])
		pid, pidErr := strconv.Atoi(fields[0])
		if uidErr != nil || pidErr != nil || uid == os.Getuid() || pid <= 1 {
			continue
		}
		sid, err := processSessionID(pid)
		if err != nil || sid != pid || ownSessions[sid] {
			continue
		}
		if active, err := processSessionActive(sid); err != nil || !active {
			t.Fatal("a session owned exclusively by another user was invisible to the process guard", pid, err)
		}
		return
	}
	t.Skip("no isolated other-user process session available")
}

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
		if sid, err := processSessionID(member); err == nil && sid == command.Process.Pid {
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

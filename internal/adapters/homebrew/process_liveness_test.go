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
	output, err := exec.Command("/bin/ps", "-p", "1", "-o", "uid=").Output()
	uid, parseErr := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil || parseErr != nil || uid == os.Getuid() {
		t.Skip("no observable process owned by another user")
	}
	session, err := processSessionID(1)
	if err != nil || session < 0 {
		t.Skip("other user's process session is not observable")
	}
	if active, err := processSessionActive(session); err != nil || !active {
		t.Fatal("another user's session was invisible to the process guard", err)
	}
}

func TestProcessInventoryDistinguishesAbsentSession(t *testing.T) {
	active, err := processSessionActive(1 << 29)
	if err != nil || active {
		t.Fatal("could not establish an absent session without signalling processes", active, err)
	}
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

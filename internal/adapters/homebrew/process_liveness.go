package homebrew

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Observe all user-owned PIDs for a recorded process session. A reused PID or
// unavailable process table conservatively holds rather than signalling it.
func processSessionActive(session int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for attempt := 0; attempt < 3; attempt++ {
		command := exec.CommandContext(ctx, "/bin/ps", "-U", strconv.Itoa(os.Getuid()), "-o", "pid=")
		command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
		output, diagnostics := &processOutput{}, &processOutput{}
		command.Stdout, command.Stderr = output, diagnostics
		if err := command.Run(); err != nil || output.overflow || diagnostics.overflow {
			return false, errors.New("owned process inventory unavailable")
		}
		fields := strings.Fields(output.String())
		if len(fields) == 0 || len(fields) > 100000 {
			return false, errors.New("invalid owned process inventory")
		}
		active, changed, err := sessionInProcesses(session, command.Process.Pid, fields)
		if err != nil || active {
			return active, err
		}
		if !changed {
			return false, nil
		}
	}
	return false, errors.New("process inventory changed during observation")
}

func sessionInProcesses(session, observer int, fields []string) (active, changed bool, err error) {
	for _, field := range fields {
		pid, err := strconv.Atoi(field)
		if err != nil || pid <= 0 {
			return false, false, errors.New("invalid observed process ID")
		}
		if pid == observer {
			continue
		}
		sid, err := processSessionID(pid)
		if errors.Is(err, syscall.ESRCH) {
			changed = true
			continue
		}
		if err != nil {
			return false, false, errors.New("owned process session unavailable")
		}
		if sid == session {
			return true, changed, nil
		}
	}
	return false, changed, nil
}

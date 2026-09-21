package homebrew

import "syscall"

// The frozen standard-library syscall package does not expose Getsid on Linux.
func processSessionID(pid int) (int, error) {
	value, _, errno := syscall.RawSyscall(syscall.SYS_GETSID, uintptr(pid), 0, 0)
	if errno != 0 {
		return 0, errno
	}
	return int(value), nil
}

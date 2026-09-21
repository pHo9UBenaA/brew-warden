package homebrew

import "syscall"

func processSessionID(pid int) (int, error) { return syscall.Getsid(pid) }

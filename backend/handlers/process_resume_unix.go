//go:build !windows

package handlers

import "syscall"

func resumeProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGCONT)
}

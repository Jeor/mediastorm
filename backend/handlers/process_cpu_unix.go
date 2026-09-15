//go:build !windows

package handlers

import (
	"syscall"
	"time"
)

func processCPUTimes() (time.Duration, time.Duration, error) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0, 0, err
	}
	return time.Duration(usage.Utime.Nano()), time.Duration(usage.Stime.Nano()), nil
}

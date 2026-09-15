//go:build windows

package handlers

import (
	"time"

	"golang.org/x/sys/windows"
)

func processCPUTimes() (time.Duration, time.Duration, error) {
	var creation, exit, kernel, user windows.Filetime
	err := windows.GetProcessTimes(windows.CurrentProcess(), &creation, &exit, &kernel, &user)
	if err != nil {
		return 0, 0, err
	}
	return time.Duration(user.Nanoseconds()), time.Duration(kernel.Nanoseconds()), nil
}

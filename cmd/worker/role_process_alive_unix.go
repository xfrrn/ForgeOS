//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package main

import (
	"errors"
	"golang.org/x/sys/unix"
)

func roleProcessAlive(pid int) (bool, error) {
	err := unix.Kill(pid, 0)
	if errors.Is(err, unix.ESRCH) {
		return false, nil
	}
	return err == nil, err
}

//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func roleLockBusy(err error) bool { return errors.Is(err, unix.EWOULDBLOCK) }

func lockRoleFile(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) }

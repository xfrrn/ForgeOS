//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockRoleFile(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) }

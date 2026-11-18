//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly || solaris || aix

package main

import (
	"os"
	"syscall"
)

func singlePatchLink(_ *os.File, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}

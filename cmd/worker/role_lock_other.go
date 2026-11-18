//go:build !windows && !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package main

import (
	"errors"
	"os"
)

func lockRoleFile(*os.File) error {
	return errors.New("role journal locking is unavailable on this platform")
}

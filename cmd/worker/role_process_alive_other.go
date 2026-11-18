//go:build !windows && !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package main

import "errors"

func roleProcessAlive(int) (bool, error) {
	return false, errors.New("native role process observation is unavailable on this platform")
}

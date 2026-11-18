//go:build !windows && !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly && !solaris && !aix

package main

import "os"

func singlePatchLink(*os.File, os.FileInfo) bool { return false }

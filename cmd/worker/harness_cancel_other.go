//go:build !linux

package main

import "os/exec"

func configureHarnessCancellation(cmd *exec.Cmd) {}

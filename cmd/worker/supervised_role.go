package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
)

var errRoleDaemonGone = errors.New("role Daemon exited")

func superviseRole(ctx context.Context, role string) error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, "run-role-process", role)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	// Only this Daemon owns the write end. Its exit lets the child finish its current assignment.
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer pipe.Close()
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		return err
	}
}

func roleDaemonLifetime(parent io.Reader) <-chan struct{} {
	gone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, parent)
		close(gone)
	}()
	return gone
}

//go:build !windows && !linux

package main

import "errors"

const supportsWriterProcessTree = false

func trackWriterProcess(int) error { return errors.New("writer process tree tracking unavailable") }

func writerTreeAlive(int) (bool, error) { return false, trackWriterProcess(0) }

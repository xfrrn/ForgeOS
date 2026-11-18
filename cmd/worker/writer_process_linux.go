package main

const supportsWriterProcessTree = true

// configureHarnessCancellation already starts each harness in its own process group.
func trackWriterProcess(int) error { return nil }

func writerTreeAlive(pid int) (bool, error) { return roleProcessAlive(-pid) }

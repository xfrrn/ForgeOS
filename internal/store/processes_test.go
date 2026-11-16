package store

import (
	"testing"
	"time"
)

func TestHarnessProcessHistory(t *testing.T) {
	now := time.Now().UTC()
	ok, interrupted := 0, 23
	result := Result{Harness: "claude", HarnessVersion: "2.1.310", SessionID: "session"}
	first := HarnessProcess{Instance: 1, PID: 7, Harness: result.Harness, Version: result.HarnessVersion,
		Session: result.SessionID, Started: now, Exited: now.Add(time.Second), ExitCode: &interrupted}
	last := first
	last.Instance, last.Started, last.Exited, last.ExitCode = 2, first.Exited, first.Exited.Add(time.Second), &ok
	if !(ProcessHistory{first, last}).matches(result) {
		t.Fatal("PID reuse across distinct process instances should preserve the history")
	}
	for _, change := range []func(*HarnessProcess){
		func(p *HarnessProcess) { p.Instance = first.Instance },
		func(p *HarnessProcess) { p.PID = 0 },
		func(p *HarnessProcess) { p.Version = "other" },
		func(p *HarnessProcess) { p.Session = "different" },
		func(p *HarnessProcess) { p.Exited = now },
		func(p *HarnessProcess) { p.ExitCode = nil },
		func(p *HarnessProcess) { p.ExitCode = &interrupted },
	} {
		bad := last
		change(&bad)
		if (ProcessHistory{first, bad}).matches(result) {
			t.Fatalf("accepted inconsistent process instance: %+v", bad)
		}
	}
	result.ExitCode, result.SessionID = 1, ""
	first.Session = ""
	if !(ProcessHistory{first}).matches(result) {
		t.Fatal("process that failed before session initialization lost its evidence")
	}
}

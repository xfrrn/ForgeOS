package store

import (
	"strings"
	"testing"
	"time"
)

func TestProcessArchiveReceipt(t *testing.T) {
	a := ProcessArchive{Bytes: 80, Records: 1, LimitBytes: 1 << 20, Digest: strings.Repeat("a", 64)}
	if !a.valid() {
		t.Fatal("complete archive rejected")
	}
	for _, change := range []func(*ProcessArchive){
		func(a *ProcessArchive) { a.Digest = "incorrect" }, func(a *ProcessArchive) { a.Bytes = a.LimitBytes + 1 },
		func(a *ProcessArchive) { a.Records = 0 }, func(a *ProcessArchive) { a.DroppedRecords = 1 },
		func(a *ProcessArchive) { a.LimitBytes = 1 << 31 },
	} {
		bad := a
		change(&bad)
		if bad.valid() {
			t.Fatal("invalid archive accepted", bad)
		}
	}
	a.Bytes, a.Records, a.Digest = 0, 0, ""
	a.DroppedBytes, a.DroppedRecords = a.LimitBytes+1, 1
	if !a.valid() {
		t.Fatal("valid omitted oversized first record rejected")
	}
}

func TestHarnessProcessHistory(t *testing.T) {
	now := time.Now().UTC()
	firstEnd, lastEnd := now.Add(time.Second), now.Add(2*time.Second)
	ok, interrupted := 0, 23
	result := Result{Harness: "claude", HarnessVersion: "2.1.310", SessionID: "session"}
	first := HarnessProcess{Instance: 1, PID: 7, Harness: result.Harness, Version: result.HarnessVersion,
		Session: result.SessionID, Started: now, Exited: &firstEnd, ExitCode: &interrupted}
	last := first
	last.Instance, last.Started, last.Exited, last.ExitCode = 2, firstEnd, &lastEnd, &ok
	if !(ProcessHistory{first, last}).matches(result) {
		t.Fatal("PID reuse across distinct process instances should preserve the history")
	}
	for _, change := range []func(*HarnessProcess){
		func(p *HarnessProcess) { p.Instance = first.Instance },
		func(p *HarnessProcess) { p.PID = 0 },
		func(p *HarnessProcess) { p.Version = "other" },
		func(p *HarnessProcess) { p.Session = "different" },
		func(p *HarnessProcess) { p.Exited = &now },
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

func TestCodexProcessHistoryAcrossThreads(t *testing.T) {
	ok := 0
	now := time.Now().UTC()
	result := Result{Harness: "codex", HarnessVersion: "0.169.0-alpha.11.1", SessionID: "last-thread"}
	first := HarnessProcess{Instance: 1, PID: 40, Harness: result.Harness, Version: result.HarnessVersion,
		Session: "first-thread", Started: now, Exited: &now, ExitCode: &ok}
	last := first
	last.Instance, last.PID, last.Session = 2, 41, result.SessionID
	history := ProcessHistory{first, last}
	if !history.matches(result) || !ValidHarnessVersion(result.Harness, result.HarnessVersion) {
		t.Fatal("distinct Codex comment rounds were rejected")
	}
	result.SessionID = first.Session
	if history.matches(result) {
		t.Fatal("accepted a final Session from an earlier process")
	}
	if ValidHarnessVersion("codex", "0.169.0-alpha..1") || ValidHarnessVersion("claude", result.HarnessVersion) {
		t.Fatal("accepted an invalid CLI version")
	}
}

func TestPlannerHandoffBindsBothThreadsToOneExecution(t *testing.T) {
	one, zero := 1, 0
	now := time.Now().UTC()
	r := Result{Harness: "codex", HarnessVersion: "0.169.0-alpha.11.1", SessionID: "new", ExitCode: 1, Error: "later failure"}
	first := HarnessProcess{Instance: 1, PID: 10, Harness: r.Harness, Version: r.HarnessVersion, Session: "original", Started: now, Exited: &now, ExitCode: &one}
	missing, next := first, first
	missing.Instance, missing.PID, missing.Session = 2, 11, ""
	next.Instance, next.PID, next.Session, next.HandoffFrom, next.ExitCode = 3, 12, "new", "original", &zero
	h := ProcessHistory{first, missing, next}
	r.Processes = &h
	if !(IssueSpec{Mode: "plan"}).accepts(r) || !(IssueSpec{}).accepts(r) {
		t.Fatal("valid handoff history rejected")
	}
	h[2].HandoffFrom = "unrelated"
	if h.matches(r) {
		t.Fatal("accepted an unrelated source thread")
	}
	h[2].HandoffFrom = first.Session
	h[1].ExitCode = &zero
	if h.matches(r) {
		t.Fatal("accepted a handoff without a failed resume")
	}
}

func TestClaudeSessionChangeRequiresHandoff(t *testing.T) {
	now := time.Now().UTC()
	failed, ok := 1, 0
	process := HarnessProcess{Instance: 1, PID: 10, Harness: "claude", Version: "2.1.310", Started: now, Exited: &now, ExitCode: &failed, Session: "original"}
	h := ProcessHistory{process, process, process, process}
	h[1].Instance, h[1].Session = 2, ""
	h[2].Instance, h[2].Session, h[2].HandoffFrom, h[2].ExitCode = 3, "new", "original", &ok
	h[3].Instance, h[3].Session, h[3].ExitCode = 4, "new", &ok
	r := Result{Harness: process.Harness, HarnessVersion: process.Version, SessionID: "new"}
	if !h.matches(r) {
		t.Fatal("rejected linked handoff followed by another turn")
	}
	h[2].HandoffFrom = ""
	if h.matches(r) {
		t.Fatal("accepted an unlinked Claude session change")
	}
}

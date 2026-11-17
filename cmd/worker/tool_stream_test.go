package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func TestCommandStreamRedactsBeforeJournalAndKeepsCallWindows(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	w := worker{repoRoot: t.TempDir(), workspaceRoot: t.TempDir(), token: "private-node"}
	var err error
	w.journal, err = w.openJournal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	p := w.processLog(context.Background(), store.Job{RecordToolActivity: true, RecordToolOutput: true, RecordToolStream: true}, "codex", "1.2.3")
	row, err := w.journal.Exec("INSERT INTO harness_processes(lease_hash,harness,version,pid,session_id) VALUES(?,?,?,?,?)", p.lease[:], p.harness, p.version, 123, "thread")
	if err != nil {
		t.Fatal(err)
	}
	p.id, err = row.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	check := func(id, phase, text string) {
		t.Helper()
		if err := p.recordTool(id, "command_execution", phase, text); err != nil {
			t.Fatal(err)
		}
	}
	delta := func(id, text string) {
		t.Helper()
		if err := p.commandOutput(id, text); err != nil {
			t.Fatal(err)
		}
	}
	read := func() store.HarnessProcess {
		t.Helper()
		got, err := scanHarnessProcess(w.journal.QueryRow(readHarnessProcess+" WHERE instance=?", p.id))
		if err != nil || !got.Valid() {
			t.Fatal(got, err)
		}
		if got.ToolOutput != nil && strings.Contains(got.ToolOutput.Text, "private-") {
			t.Fatal("secret journaled")
		}
		return got
	}
	check("one", "started", "")
	delta("missing", "ignored\n")
	delta("one", "private-")
	if read().ToolOutput != nil {
		t.Fatal("incomplete line published")
	}
	var uncertain bool
	if err := w.journal.QueryRow("SELECT private_key FROM harness_output_redaction WHERE lease_hash=? AND stream='tool'", p.lease[:]).Scan(&uncertain); err != nil || !uncertain {
		t.Fatal("partial line recovery must be conservative", uncertain, err)
	}
	delta("one", "node\nFirst\r\npass")
	a := read()
	if a.ToolOutput.Text != "[redacted]\nFirst\n" || !a.ToolOutput.Partial {
		t.Fatal(a)
	}
	delta("one", "word=private-value\n-----BE")
	delta("one", "GIN PRIVATE KEY-----\nprivate-first\n")
	var open bool
	if err := w.journal.QueryRow("SELECT private_key FROM harness_output_redaction WHERE lease_hash=? AND stream='tool'", p.lease[:]).Scan(&open); err != nil || !open {
		t.Fatal(open, err)
	}
	check("two", "started", "")
	delta("two", "private-second\n")
	delta("two", "private-delayed-tail")
	if read().ToolOutput.Call == a.ToolOutput.Call {
		t.Fatal("interleaved call identity lost")
	}
	delta("one", "-----END PRIVATE KEY-----\nLast")
	check("one", "completed", "private-aggregate-must-not-be-replayed")
	a = read()
	if a.ToolOutput.Partial || !strings.HasSuffix(a.ToolOutput.Text, "Last\n") || !strings.Contains(a.ToolOutput.Text, "First\n") {
		t.Fatal(a)
	}
	delta("one", "late output ignored\n")
	if *read().ToolOutput != *a.ToolOutput {
		t.Fatal("completed call accepted delta")
	}
	delta("two", "\n-----END PRIVATE KEY-----\n")
	check("two", "failed", "")
	if read().ToolOutput.Partial || strings.Contains(read().ToolOutput.Text, "First") {
		t.Fatal("mixed call windows")
	}
	check("large", "started", "")
	delta("large", strings.Repeat("界", 30<<10))
	if len(p.tools.pending["large"].stream.pending) > 64<<10 {
		t.Fatal("unbounded line")
	}
	delta("large", "private-hidden\nprivate-conservative\n-----END PRIVATE KEY-----\nVisible again\n")
	if !strings.HasSuffix(read().ToolOutput.Text, "Visible again\n") {
		t.Fatal(read())
	}
	for i := 0; i < 40; i++ {
		delta("large", strings.Repeat("界", 200)+"\n")
	}
	a = read()
	if len(a.ToolOutput.Text) > validation.LiveLogWindow || len(a.Tools) != store.ToolWindow || !a.ToolOutput.Partial {
		t.Fatal(a)
	}
	if err := w.journal.Close(); err != nil {
		t.Fatal(err)
	}
	w.journal, err = w.openJournal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p.db = w.journal
	defer w.journal.Close()
	if *read().ToolOutput != *a.ToolOutput {
		t.Fatal("partial lost on reopen")
	}
	// An old control plane must receive the old tool event protocol.
	p.toolStream = false
	check("old", "started", "")
	delta("old", "not published\n")
	if *read().ToolOutput != *a.ToolOutput {
		t.Fatal("stream without opt-in")
	}
	check("old", "completed", "Final old protocol\n")
	if read().ToolOutput.Partial || read().ToolOutput.Text != "Final old protocol\n\n" {
		t.Fatal(read())
	}
	p.toolStream = true
	check("split-key", "started", "")
	delta("split-key", "-----BE")
	// Replacement has no raw line buffer, but must not expose the rest of the key.
	p = w.processLog(context.Background(), store.Job{RecordToolActivity: true, RecordToolOutput: true, RecordToolStream: true}, "codex", "1.2.3")
	row, err = w.journal.Exec("INSERT INTO harness_processes(lease_hash,harness,version,pid,session_id) VALUES(?,?,?,?,?)", p.lease[:], p.harness, p.version, 456, "thread")
	if err != nil {
		t.Fatal(err)
	}
	p.id, err = row.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	check("resumed", "completed", "private-key-tail\n-----END PRIVATE KEY-----\nRecovered")
	if !strings.HasSuffix(read().ToolOutput.Text, "Recovered\n") {
		t.Fatal(read())
	}
}

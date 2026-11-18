package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func TestWebSearchShowsOnlyReturnedTextResults(t *testing.T) {
	results := json.RawMessage(`[null,"bad",{"type":"other","snippet":"private"},{"type":"text_result","title":"First","url":"private","ref_id":"private","snippet":"Summary"},{"type":"text_result","domain":"example.org"},{"type":"text_result","snippet":123}]`)
	if got := webSearchOutput(results); got != "First\nSummary\n\nexample.org" {
		t.Fatal(got)
	}
	for _, raw := range []string{"null", "[]", `[{"type":"text_result","query":"private"}]`, `{"action":"private"}`} {
		if got := webSearchOutput(json.RawMessage(raw)); got != "" {
			t.Fatal(raw, got)
		}
	}
}

func TestToolEventsKeepMetadataAndReadLatestWindow(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	w := worker{repoRoot: t.TempDir(), workspaceRoot: t.TempDir()}
	var err error
	w.journal, err = w.openJournal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	for _, h := range []string{"claude", "codex"} {
		p := w.processLog(context.Background(), store.Job{RecordToolActivity: true}, h, "1.2.3")
		row, err := w.journal.Exec("INSERT INTO harness_processes(lease_hash,harness,version,pid,session_id) VALUES(?,?,?,?,?)", p.lease[:], h, p.version, 123, "session")
		if err != nil {
			t.Fatal(err)
		}
		p.id, err = row.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		var tool string
		for i := 0; i < store.ToolWindow; i++ {
			switch h {
			case "claude":
				tool = "Read"
				err = p.claudeTool("assistant", []byte(`{"message":{"content":[{"type":"text","text":"private"},{"type":"tool_use","id":"private-id","name":"Read","input":{"file_path":"private"}}]}}`))
				if err == nil {
					err = p.claudeTool("user", []byte(`{"message":{"content":[{"type":"tool_result","tool_use_id":"private-id","content":"private","is_error":true}]}}`))
				}
			case "codex":
				tool = "command_execution"
				raw := []byte(`{"item":{"id":"private-id","type":"command_execution","command":"private","aggregated_output":"private","status":"failed"}}`)
				err = p.codexTool("item.started", raw)
				if err == nil {
					err = p.codexTool("item.completed", raw)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		got, err := scanHarnessProcess(w.journal.QueryRow(readHarnessProcess+" WHERE instance=?", p.id))
		if err != nil || !got.Valid() || len(got.Tools) != store.ToolWindow {
			t.Fatal(got, err)
		}
		for i, e := range got.Tools {
			seq := int64(store.ToolWindow + i + 1)
			if e.Seq != seq || e.Tool != tool || (i%2 == 0 && (e.Phase != "started" || e.Call != seq)) || (i%2 == 1 && (e.Phase != "failed" || e.Call != seq-1)) {
				t.Fatal(e)
			}
		}
		raw, _ := json.Marshal(got.Tools)
		if strings.Contains(string(raw), "private") {
			t.Fatal("private tool data entered the receipt")
		}
		var n int
		if err = w.journal.QueryRow("SELECT COUNT(*) FROM harness_tool_events WHERE instance=?", p.id).Scan(&n); err != nil || n != 2*store.ToolWindow {
			t.Fatal(n, err)
		}
	}
}

func TestToolOutputWindowPersistsWithoutInputs(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv("TOOL_API_KEY", "private-environment")
	w := worker{repoRoot: t.TempDir(), workspaceRoot: t.TempDir(), token: "private-node"}
	var err error
	w.journal, err = w.openJournal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	p := w.processLog(context.Background(), store.Job{RecordToolActivity: true, RecordToolOutput: true, LeaseToken: "private-lease"}, "claude", "1.2.3")
	row, err := w.journal.Exec("INSERT INTO harness_processes(lease_hash,harness,version,pid,session_id) VALUES(?,?,?,?,?)", p.lease[:], p.harness, p.version, 123, "thread")
	if err != nil {
		t.Fatal(err)
	}
	p.id, err = row.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if err = p.claudeTool("assistant", []byte(`{"message":{"content":[{"type":"tool_use","id":"read-1","name":"Read","input":{"file_path":"private-path"}}]}}`)); err != nil {
		t.Fatal(err)
	}
	if err = p.claudeTool("user", []byte(`{"message":{"content":[{"type":"tool_result","tool_use_id":"read-1","content":"private-environment\nprivate-node\nprivate-lease\n-----BEGIN PRIVATE KEY-----\nprivate-key\n-----END PRIVATE KEY-----\nRead finished."}]}}`)); err != nil {
		t.Fatal(err)
	}
	read := func() store.HarnessProcess {
		t.Helper()
		got, err := scanHarnessProcess(w.journal.QueryRow(readHarnessProcess+" WHERE instance=?", p.id))
		if err != nil || !got.Valid() || got.ToolOutput == nil || strings.Contains(got.ToolOutput.Text, "private-") {
			t.Fatal(got, err)
		}
		return got
	}
	got := read()
	if got.ToolOutput.Seq != 2 || got.ToolOutput.Call != 1 || !strings.HasSuffix(got.ToolOutput.Text, "Read finished.\n") {
		t.Fatal(got)
	}
	w.token = "private-rotated"
	if err = p.recordTool("read-2", "Read", "failed", "private-rotated\nNext."); err != nil {
		t.Fatal(err)
	}
	got = read()
	if got.ToolOutput.Text != "[redacted]\nNext.\n" || got.Tools[len(got.Tools)-1].Phase != "failed" {
		t.Fatal(got)
	}
	if err = p.recordTool("key-first", "Read", "completed", "-----BEGIN PRIVATE KEY-----\nprivate-first-part"); err != nil {
		t.Fatal(err)
	}
	read()
	// A replacement process on the same execution must inherit the open-key state.
	if err = w.journal.Close(); err != nil {
		t.Fatal(err)
	}
	w.journal, err = w.openJournal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p.db = w.journal
	defer w.journal.Close()
	p.toolOutput = &modelOutput{secrets: outputSecrets("private-lease"), credential: w.credential}
	if err = p.recordTool("key-rest", "Read", "completed", "private-second-part\n-----END PRIVATE KEY-----\nKey read finished."); err != nil {
		t.Fatal(err)
	}
	got = read()
	if !strings.HasSuffix(got.ToolOutput.Text, "Key read finished.\n") {
		t.Fatal(got)
	}
	if err = p.recordTool("read-3", "Read", "completed", strings.Repeat("界", validation.LiveLogWindow)+"\nEnd."); err != nil {
		t.Fatal(err)
	}
	got = read()
	if len(got.ToolOutput.Text) > validation.LiveLogWindow || !strings.HasSuffix(got.ToolOutput.Text, "End.\n") {
		t.Fatal(got)
	}
	for i := 0; i < store.ToolWindow; i++ {
		if err = p.recordTool("write", "Write", "completed", ""); err != nil {
			t.Fatal(err)
		}
	}
	if *read().ToolOutput != *got.ToolOutput {
		t.Fatal("metadata-only calls lost the last output")
	}
	if err = w.journal.Close(); err != nil {
		t.Fatal(err)
	}
	w.journal, err = w.openJournal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	if *read().ToolOutput != *got.ToolOutput {
		t.Fatal("journal reopen lost output")
	}
}

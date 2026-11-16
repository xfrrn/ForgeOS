package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/store"
)

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

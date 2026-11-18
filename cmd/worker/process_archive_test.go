package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
)

func archiveTestProcess(t *testing.T) (*worker, processLog, string) {
	t.Helper()
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	w := &worker{api: "http://127.0.0.1:8188", id: "archive-test", token: "private-node-credential"}
	var err error
	w.journal, err = w.openJournal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.journal.Close() })
	p := w.processLog(context.Background(), store.Job{LeaseToken: "private-lease", RecordModelOutput: true, RecordToolActivity: true, RecordToolOutput: true, RecordToolStream: true}, "codex", "1.2.3")
	row, err := w.journal.Exec("INSERT INTO harness_processes(lease_hash,harness,version,pid,session_id) VALUES(?,?,?,?,?)", p.lease[:], p.harness, p.version, 123, "recorded-session")
	if err != nil {
		t.Fatal(err)
	}
	p.id, err = row.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	var seq int
	var name, path string
	if err = w.journal.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	return w, p, path
}

func TestProcessArchiveKeepsLongOutputAndStreamOrder(t *testing.T) {
	w, p, path := archiveTestProcess(t)
	model := "model-begin\n" + strings.Repeat("public model line\n", 1000) + "private-node-credential\nmodel-end"
	tool := "tool-begin\n" + strings.Repeat("public tool line\n", 1000) + "private-lease\ntool-end"
	for _, err := range []error{p.modelMessage(model), p.recordTool("first", "command_execution", "started", ""), p.recordTool("first", "", "completed", tool), p.recordTool("stream", "command_execution", "started", ""), p.commandOutput("stream", "stream-begin\n"+strings.Repeat("public streaming line\n", 1000)+"pass"), p.commandOutput("stream", "word=never-archive\nstream-end\n"), p.recordTool("stream", "", "completed", "must-not-repeat-aggregate")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	var result bytes.Buffer
	if err := processLogsCommand([]string{"export", "--journal", path, "--instance", fmt.Sprint(p.id)}, &result); err != nil {
		t.Fatal(err)
	}
	text := result.String()
	for _, expected := range []string{"model-begin", "model-end", "tool-begin", "tool-end", "stream-begin", "stream-end", `"verified":true`} {
		if !strings.Contains(text, expected) {
			t.Fatal("missing archive evidence", expected)
		}
	}
	for _, secret := range []string{"private-node-credential", "private-lease", "never-archive", "must-not-repeat-aggregate"} {
		if strings.Contains(text, secret) {
			t.Fatal("archive exposed forbidden content", secret)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(result.Bytes()))
	var messages []processLogEntry
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		var entry processLogEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Sequence != 0 {
			messages = append(messages, entry)
		}
	}
	if len(messages) != 7 || messages[0].Stream != "model" || messages[4].Phase != "output" || messages[5].Phase != "output" || messages[6].Text != "" {
		t.Fatal("wrong archive sequence", len(messages))
	}
	var window string
	if err := w.journal.QueryRow("SELECT text FROM harness_model_output WHERE instance=?", p.id).Scan(&window); err != nil || len(window) > 8192 || strings.Contains(window, "model-begin") {
		t.Fatal("live window changed", err)
	}
	// Export is a read-only snapshot and remains identical after closing and reopening the journal.
	if err := w.journal.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	w.journal, err = w.openJournal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var restored bytes.Buffer
	if err = processLogsCommand([]string{"export", "--journal", path, "--instance", fmt.Sprint(p.id)}, &restored); err != nil || !bytes.Equal(result.Bytes(), restored.Bytes()) {
		t.Fatal("restart changed archive", err)
	}
	if _, err = w.journal.Exec("UPDATE harness_log_entries SET entry=replace(CAST(entry AS TEXT),'model-begin','model-wrong') WHERE instance=? AND seq=1", p.id); err != nil {
		t.Fatal(err)
	}
	if err = exportProcessArchive(w.journal, p.id, io.Discard); err == nil {
		t.Fatal("corrupt archive exported")
	}
}

func TestProcessArchiveLimitAndAtomicRedaction(t *testing.T) {
	w, p, _ := archiveTestProcess(t)
	if err := p.modelMessage("first public message"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.journal.Exec("UPDATE harness_log_archives SET limit_bytes=bytes+1"); err != nil {
		t.Fatal(err)
	}
	if err := p.modelMessage("second exceeds capacity"); err != nil {
		t.Fatal(err)
	}
	if err := p.modelMessage("third also omitted"); err != nil {
		t.Fatal(err)
	}
	var count, dropped int64
	if err := w.journal.QueryRow("SELECT records,dropped_records FROM harness_log_archives").Scan(&count, &dropped); err != nil || count != 1 || dropped != 2 {
		t.Fatal(count, dropped, err)
	}
	var output bytes.Buffer
	if err := exportProcessArchive(w.journal, p.id, &output); err != nil || !strings.Contains(output.String(), `"truncated":true`) {
		t.Fatal("limit not disclosed", err)
	}
	if _, err := w.journal.Exec("CREATE TRIGGER fail_archive BEFORE UPDATE ON harness_log_archives BEGIN SELECT RAISE(ABORT,'archive test failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err := p.modelMessage("-----BEGIN PRIVATE KEY-----\nprivate-body"); err == nil {
		t.Fatal("archive fault ignored")
	}
	var open bool
	var window string
	if err := w.journal.QueryRow("SELECT private_key FROM harness_output_redaction WHERE stream='model'").Scan(&open); err != nil || open {
		t.Fatal("redaction state committed without archive", err)
	}
	if err := w.journal.QueryRow("SELECT text FROM harness_model_output").Scan(&window); err != nil || strings.Contains(window, "redacted") {
		t.Fatal("window committed without archive", err)
	}
}

func TestProcessArchiveRetentionPreservesPendingWorkAndMetadata(t *testing.T) {
	w, p, path := archiveTestProcess(t)
	if err := p.modelMessage("retain until settlement"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-40 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := w.journal.Exec("UPDATE harness_processes SET exited_at=?,exit_code=0", old); err != nil {
		t.Fatal(err)
	}
	if n, err := pruneProcessArchives(w.journal, time.Now().Add(-30*24*time.Hour)); err != nil || n != 0 {
		t.Fatal("unsettled archive removed", n, err)
	}
	if _, err := w.journal.Exec("INSERT INTO attempts(lease_hash,execution_id,task_id,workspace,settled_at) VALUES(?,1,2,'private-workspace',?)", p.lease[:], old); err != nil {
		t.Fatal(err)
	}
	// A paused execution keeps its recovery evidence even with an inconsistent historical settlement.
	if _, err := w.journal.Exec("INSERT INTO parked_questions(lease_hash,checkpoint) VALUES(?,'{}')", p.lease[:]); err != nil {
		t.Fatal(err)
	}
	if n, err := pruneProcessArchives(w.journal, time.Now().Add(-30*24*time.Hour)); err != nil || n != 0 {
		t.Fatal("parked archive removed", n, err)
	}
	if _, err := w.journal.Exec("DELETE FROM parked_questions"); err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	if err := processLogsCommand([]string{"prune", "--journal", path, "--days", "30"}, &result); err != nil || !strings.Contains(result.String(), `"expired":1`) {
		t.Fatal(result.String(), err)
	}
	info, err := scanProcessArchive(w.journal.QueryRow(processArchiveQuery+" WHERE p.instance=?", p.id))
	if err != nil || info.Expired == "" || info.Records != 1 || info.Digest == "" || info.Execution != 1 || info.Task != 2 {
		t.Fatal(info, err)
	}
	var n int
	if err = w.journal.QueryRow("SELECT count(*) FROM harness_log_entries").Scan(&n); err != nil || n != 0 {
		t.Fatal("archive body retained", n, err)
	}
	if err = exportProcessArchive(w.journal, p.id, io.Discard); err == nil {
		t.Fatal("expired archive exported")
	}
	if n, err := pruneProcessArchives(w.journal, time.Now()); err != nil || n != 0 {
		t.Fatal("expiry not idempotent", n, err)
	}
	// The last visible window and business process receipt remain unchanged.
	if err = w.journal.QueryRow("SELECT count(*) FROM harness_model_output").Scan(&n); err != nil || n != 1 {
		t.Fatal("business snapshot removed", n, err)
	}
}

func TestProcessArchiveRetentionProtectsRoleAssignment(t *testing.T) {
	w, p, _ := archiveTestProcess(t)
	if err := p.modelMessage("role evidence"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-40 * 24 * time.Hour).UTC().Format(time.RFC3339)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"UPDATE harness_processes SET exited_at=?,exit_code=0", []any{old}},
		{"INSERT INTO role_receipts(lease_hash,path,digest,settled_at,status) VALUES(?,'/internal/curator/result','digest',?,204)", []any{p.lease[:], old}},
		{"INSERT INTO role_assignment(id,request_key,lease_hash,run_id,response) VALUES(1,'request',?,'run','{}')", []any{p.lease[:]}},
	} {
		if _, err := w.journal.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := pruneProcessArchives(w.journal, time.Now()); err != nil || n != 0 {
		t.Fatal("active role removed", n, err)
	}
	if _, err := w.journal.Exec("DELETE FROM role_assignment"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.journal.Exec("CREATE TRIGGER fail_expiry BEFORE DELETE ON harness_log_entries BEGIN SELECT RAISE(ABORT,'expiry test failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := pruneProcessArchives(w.journal, time.Now()); err == nil {
		t.Fatal("failed removal marked as expired")
	}
	var expired bool
	if err := w.journal.QueryRow("SELECT expired_at IS NOT NULL FROM harness_log_archives").Scan(&expired); err != nil || expired {
		t.Fatal("expiry marker did not roll back", err)
	}
	if _, err := w.journal.Exec("DROP TRIGGER fail_expiry"); err != nil {
		t.Fatal(err)
	}
	if n, err := pruneProcessArchives(w.journal, time.Now()); err != nil || n != 1 {
		t.Fatal("settled role not retired", n, err)
	}
}

func TestProcessArchiveUpgradeAndInvalidPolicy(t *testing.T) {
	w, p, path := archiveTestProcess(t)
	var list bytes.Buffer
	if err := processLogsCommand([]string{"list", "--journal", path}, &list); err != nil || !strings.Contains(list.String(), `"available":false`) {
		t.Fatal(list.String(), err)
	}
	if err := exportProcessArchive(w.journal, p.id, io.Discard); err == nil {
		t.Fatal("old process falsely has full archive")
	}
	t.Setenv("FORGEOS_PROCESS_LOG_DAYS", "0")
	if err := prepareProcessArchives(w.journal); err == nil {
		t.Fatal("invalid retention accepted")
	}
	if err := processLogsCommand([]string{"prune", "--journal", path, "--days", "0"}, io.Discard); err == nil {
		t.Fatal("invalid prune accepted")
	}
}

func TestProcessArchiveMaintenanceStopsAndRetiresConfirmedOutput(t *testing.T) {
	w, p, _ := archiveTestProcess(t)
	if err := p.modelMessage("confirmed process output"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-40 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := w.journal.Exec("UPDATE harness_processes SET exited_at=?,exit_code=0", old); err != nil {
		t.Fatal(err)
	}
	if _, err := w.journal.Exec("INSERT INTO attempts(lease_hash,execution_id,task_id,workspace,settled_at) VALUES(?,1,2,'private-workspace',?)", p.lease[:], old); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); w.sweepProcessLogs(ctx) }()
	until := time.Now().Add(5 * time.Second)
	var expired bool
	for time.Now().Before(until) {
		if err := w.journal.QueryRow("SELECT expired_at IS NOT NULL FROM harness_log_archives").Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if expired {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("retention maintenance did not stop")
	}
	if !expired {
		t.Fatal("retention maintenance did not expire confirmed output")
	}
}

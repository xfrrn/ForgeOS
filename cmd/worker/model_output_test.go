package main

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func TestAdaptersJournalPublicModelMessages(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"cli", "app-server", "claude"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			root := t.TempDir()
			w := worker{repoRoot: root, workspaceRoot: root, token: "node-value"}
			w.journal, err = w.openJournal(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer w.journal.Close()
			var adapter harnessAdapter = codexCLI{bin: bin, sandbox: "read-only", appServer: mode == "app-server"}
			name := "codex"
			if mode == "claude" {
				adapter = claudeCLI{bin: bin}
				name = "claude"
			}
			job := store.Job{RecordModelOutput: true, LeaseToken: strings.Repeat("a", 64)}
			session, err := adapter.runTurn(context.Background(), w.processLog(context.Background(), job, name, "1.2.3"), root, "SIMULATED_PUBLIC_OUTPUT", "")
			if err != nil {
				t.Fatal(err)
			}
			key := sha256.Sum256([]byte(job.LeaseToken))
			history, err := w.processHistory(context.Background(), key[:])
			if err != nil || history == nil || len(*history) != 1 {
				t.Fatal(history, err)
			}
			p := (*history)[0]
			if !p.Valid() || p.Session != session || p.Output == nil || p.Output.Sequence != 1 || p.Output.Text != "Reading the guide.\n[redacted]\n" {
				t.Fatal(p)
			}
		})
	}
}

func TestModelOutputPersistsOnlyPublicRedactedWindow(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv("MODEL_API_KEY", "private-environment-value")
	w := worker{repoRoot: t.TempDir(), workspaceRoot: t.TempDir(), token: "private-worker-value"}
	var err error
	w.journal, err = w.openJournal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	p := w.processLog(context.Background(), store.Job{RecordModelOutput: true, LeaseToken: "private-lease-value"}, "claude", "1.2.3")
	row, err := w.journal.Exec("INSERT INTO harness_processes(lease_hash,harness,version,pid,session_id) VALUES(?,?,?,?,?)", p.lease[:], p.harness, p.version, 123, "thread")
	if err != nil {
		t.Fatal(err)
	}
	p.id, err = row.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"message":{"content":[{"type":"thinking","text":"private-thought"},{"type":"tool_use","text":"private-input"},{"type":"text","text":"Reading docs.\nprivate-environment-value\nprivate-worker-value\nprivate-lease-value"}]}}`,
		`{"message":{"content":[{"type":"text","text":"-----BEGIN PRIVATE KEY-----\nprivate-first-key-part"}]}}`,
		`{"message":{"content":[{"type":"text","text":"private-second-key-part\n-----END PRIVATE KEY-----\nFinished reading."}]}}`,
	} {
		if err = p.claudeActivity("assistant", []byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if err = p.claudeActivity("user", []byte(`{"message":{"content":[{"type":"text","text":"private-user"}]}}`)); err != nil {
		t.Fatal(err)
	}
	got, err := scanHarnessProcess(w.journal.QueryRow(readHarnessProcess+" WHERE instance=?", p.id))
	if err != nil || !got.Valid() || got.Output == nil || got.Output.Sequence != 3 || !strings.Contains(got.Output.Text, "Reading docs.") || !strings.Contains(got.Output.Text, "Finished reading.") || strings.Contains(got.Output.Text, "private-") {
		t.Fatal(got, err)
	}
	w.token = "rotated-worker-value"
	if err = p.modelMessage("rotated-worker-value\nContinued."); err != nil {
		t.Fatal(err)
	}
	got, err = scanHarnessProcess(w.journal.QueryRow(readHarnessProcess+" WHERE instance=?", p.id))
	if err != nil || strings.Contains(got.Output.Text, "rotated-worker-value") || !strings.HasSuffix(got.Output.Text, "[redacted]\nContinued.\n") {
		t.Fatal(got, err)
	}
	if err = p.modelMessage(strings.Repeat("界", validation.LiveLogWindow) + "\nDone."); err != nil {
		t.Fatal(err)
	}
	got, err = scanHarnessProcess(w.journal.QueryRow(readHarnessProcess+" WHERE instance=?", p.id))
	if err != nil || !got.Valid() || got.Output.Sequence != 5 || len(got.Output.Text) > validation.LiveLogWindow || !strings.HasSuffix(got.Output.Text, "Done.\n") {
		t.Fatal(got, err)
	}
	// Opening the journal again preserves the bounded record; historical processes have none.
	if err = w.journal.Close(); err != nil {
		t.Fatal(err)
	}
	w.journal, err = w.openJournal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	again, err := scanHarnessProcess(w.journal.QueryRow(readHarnessProcess+" WHERE instance=?", p.id))
	if err != nil || again.Output == nil || *again.Output != *got.Output {
		t.Fatal(again, err)
	}
	p.output = nil
	if err = p.modelMessage("not captured by old protocol"); err != nil {
		t.Fatal(err)
	}
}

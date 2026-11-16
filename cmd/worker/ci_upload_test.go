package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func TestArchiveQueueAndConfirmationAreAtomic(t *testing.T) {
	w, job, _ := fixture(t)
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("ForgeOS-CI-Archive", "1")
		rw.WriteHeader(204)
	}))
	defer server.Close()
	w.api, w.client = server.URL, server.Client()
	ctx := context.Background()
	now := time.Now().UTC()
	code := 0
	digest := validation.DigestBytes([]byte("output"))
	run := &validation.Run{ID: "ci-run-1", Started: now, Finished: now, Checks: []validation.CheckResult{
		{Name: "build", Log: "build.log", Started: now, Finished: now, Status: "PASSED", ExitCode: &code, LogDigest: &digest},
	}}
	report, err := json.Marshal(store.Result{Validation: &store.ValidationResult{Run: run}})
	if err != nil {
		t.Fatal(err)
	}
	w.journal, err = w.openJournal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	_, err = w.journal.Exec("INSERT INTO attempts(lease_hash,task_id,execution_id,workspace,report) VALUES(?,?,?,?,?)",
		[]byte("key"), job.TaskID, job.ExecutionID, job.Workspace, report)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.journal.Exec("CREATE TRIGGER unavailable BEFORE INSERT ON ci_uploads BEGIN SELECT RAISE(ABORT, 'unavailable'); END")
	if err != nil {
		t.Fatal(err)
	}
	if err = w.replayReceipts(ctx); err == nil {
		t.Fatal("settled a result while its archive queue write failed")
	}
	_, err = w.journal.Exec("DROP TRIGGER unavailable")
	if err != nil {
		t.Fatal(err)
	}
	w.journal.Close()
	w.journal, err = w.openJournal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	if err = w.replayReceipts(ctx); err != nil {
		t.Fatal(err)
	}
	var cleared bool
	var file, sha string
	err = w.journal.QueryRow(`SELECT a.report IS NULL,u.file,u.sha256 FROM attempts a JOIN ci_uploads u USING(lease_hash)
		WHERE a.outcome='confirmed' AND u.run_id=?`, run.ID).Scan(&cleared, &file, &sha)
	if err != nil || !cleared || file != "build.log" || sha != digest.SHA256 {
		t.Fatalf("lost uploaded file reference or receipt confirmation: %v", err)
	}
}

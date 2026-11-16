package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func TestLocalCIRemovalRequiresConfirmedUploadAndMatchingRun(t *testing.T) {
	w, job, _ := fixture(t)
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	ctx := context.Background()
	now := time.Now()
	code := 0
	digest := validation.DigestBytes([]byte("ok"))
	run := validation.Run{ID: "ci-run-19", Runner: w.id, ExecutionID: job.ExecutionID, Started: now, Finished: now,
		Checks: []validation.CheckResult{{Name: "build", Log: "build.log", LogDigest: &digest, Status: "PASSED", ExitCode: &code, Started: now, Finished: now}}}
	dir := filepath.Join(w.workspaceRoot, run.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	write("run.json", raw)
	write("build.log", []byte("ok"))
	var status int
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Error(r.Method)
		}
		rw.WriteHeader(status)
	}))
	defer server.Close()
	w.api, w.client = server.URL, server.Client()
	w.journal, err = w.openJournal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := w.journal.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	f := store.LocalCIFile{Task: job.TaskID, Execution: job.ExecutionID, Run: run.ID, File: "build.log", Revision: 1, Digest: digest}
	kept := func() {
		t.Helper()
		if err := w.removeLocalCIFile(ctx, f); err == nil {
			t.Fatal("unexpected removal")
		}
		if _, err := os.Stat(filepath.Join(dir, "build.log")); err != nil {
			t.Fatal(err)
		}
	}
	kept()
	exec("INSERT INTO attempts(lease_hash,execution_id,task_id,workspace,outcome) VALUES(x'01',?,?,?,'confirmed')", job.ExecutionID, job.TaskID, job.Workspace)
	exec("INSERT INTO ci_uploads(lease_hash,run_id,file,bytes,sha256) VALUES(x'01',?,?,?,?)", run.ID, f.File, digest.Bytes, digest.SHA256)
	kept()
	exec("DELETE FROM ci_uploads")
	status = 503
	kept()
	status = 204
	write("build.log", []byte("changed"))
	kept()
	write("build.log", []byte("ok"))
	f.Execution++
	kept()
	f.Execution--
	f.File = "../run.json"
	kept()
	f.File = "build.log"
	for i := 0; i < 2; i++ {
		if err = w.removeLocalCIFile(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = os.Stat(filepath.Join(dir, "build.log")); !os.IsNotExist(err) {
		t.Fatal("local log was not removed")
	}
	if _, err = os.Stat(filepath.Join(dir, "run.json")); err != nil {
		t.Fatal(err)
	}
}

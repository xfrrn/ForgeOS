package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func TestCILogPublisherRetriesAndAdvances(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	received := make(chan store.CILogReport, 8)
	var sent atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var report store.CILogReport
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/internal/executions/7/ci-log" {
			t.Error(r.URL.Path)
		}
		received <- report
		if !sent.Swap(true) {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	node := worker{id: "node", api: server.URL, client: server.Client(), token: "sample-node-credential"}
	job := store.Job{ExecutionID: 7, LeaseToken: strings.Repeat("1", 64)}
	observe, stop := node.watchCILog(ctx, job, strings.Repeat("a", 40))
	defer stop()
	dir := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name+".log"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	expect := func(check, text string) {
		t.Helper()
		select {
		case report := <-received:
			if report.Lease != job.LeaseToken || report.Log.Check != check || report.Log.Text != text {
				t.Fatal(check, report.Log)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	write("unit", "start\nsample-node-credential\npartial")
	observe(ciLogFile{dir: dir, run: "ci-run-7", check: "unit", index: 0})
	expect("unit", "start\n[redacted]\n")
	expect("unit", "start\n[redacted]\n")
	write("unit", "start\nsample-node-credential\npartial line complete\n")
	expect("unit", "start\n[redacted]\npartial line complete\n")
	write("build", "next check\n")
	observe(ciLogFile{dir: dir, run: "ci-run-7", check: "build", index: 1})
	expect("build", "next check\n")
}

func TestBuiltinGoOutputArrivesBeforeValidationEnds(t *testing.T) {
	if os.Getenv("FORGEOS_TEST_DOCKER") != "1" {
		t.Skip("set FORGEOS_TEST_DOCKER=1 to exercise the local offline CI image")
	}
	w, job, base := fixture(t)
	repo := job.RepoPath
	validationCommit(t, repo, "go.mod", "module example\n\ngo 1.23\n")
	for _, name := range []string{"aquick", "zslow"} {
		if err := os.Mkdir(filepath.Join(repo, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	validationCommit(t, repo, "aquick/main_test.go", "package aquick\nimport \"testing\"\nfunc TestFails(t *testing.T) { t.Fatal(\"password=synthetic-ci-secret\") }\n")
	sha := validationCommit(t, repo, "zslow/main_test.go", "package zslow\nimport (\"testing\"; \"time\")\nfunc TestWaits(t *testing.T) { time.Sleep(15*time.Second) }\n")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	reports := make(chan store.CILogReport, 32)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var report store.CILogReport
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			t.Error(err)
		}
		reports <- report
		rw.WriteHeader(204)
	}))
	defer server.Close()
	w.api, w.client, w.token = server.URL, server.Client(), "node-token"
	job.ReportLiveCILog = true
	job.ArchiveGoTestLog = true
	w.id = "go-test-runner"
	done := make(chan struct{})
	var result store.ValidationResult
	var err error
	go func() {
		defer close(done)
		result, err = w.validateIssueCandidate(ctx, job, repo, repo, sha, &store.IssueSpec{BaseCommit: base, Validation: "go-test"})
	}()
	defer func() { cancel(); <-done }()
	observed := false
	for !observed {
		select {
		case report := <-reports:
			l := report.Log
			if l.Text == "" {
				continue
			}
			if l.Check != "go-test" || l.Commit != sha || !validation.ValidRunID(l.Run) || strings.Contains(l.Text, "synthetic-ci-secret") {
				t.Fatal(l)
			}
			observed = strings.Contains(l.Text, "[redacted]")
		case <-done:
			t.Fatal("validation ended before live output", result, err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	select {
	case <-done:
		t.Fatal("output only arrived after completion")
	default:
	}
	<-done
	if err == nil || result.ExitCode == 0 || result.Commit != sha {
		t.Fatal("live output changed validation outcome", result, err)
	}
	run := result.Run
	if run == nil || !run.Valid() || run.Commit != sha || run.Failure != "CODE_FAILURE" {
		t.Fatal("missing failed Go run", run)
	}
	content, err := w.ciUploadContent(run.ID, "go-test.log", run.Files()["go-test.log"])
	if err != nil || strings.Contains(string(content), "synthetic-ci-secret") || !strings.Contains(string(content), "[redacted]") || !strings.Contains(string(content), "zslow") {
		t.Fatal("archived diagnostics missing or unredacted", err)
	}
}

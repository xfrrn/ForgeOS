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

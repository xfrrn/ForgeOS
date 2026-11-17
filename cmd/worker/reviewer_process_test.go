package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"forgeos/internal/store"
)

func testReviewProcess() int {
	body, err := io.ReadAll(os.Stdin)
	args := os.Args
	if err != nil || !slices.Contains(args, "--strict-mcp-config") || !slices.Contains(args, "--disable-slash-commands") || !slices.Contains(args, "--no-chrome") {
		return 2
	}
	for _, flag := range []string{"--tools", "--permission-mode"} {
		i := slices.Index(args, flag)
		if i < 0 || i+1 >= len(args) {
			return 2
		}
		want := ""
		if flag == "--permission-mode" {
			want = "dontAsk"
		}
		if args[i+1] != want {
			return 2
		}
	}
	streaming := slices.Contains(args, "stream-json")
	session := testClaudeID
	if streaming {
		fmt.Printf(`{"type":"system","subtype":"init","session_id":"%s"}`+"\n", session)
		fmt.Printf(`{"type":"assistant","session_id":"%s","message":{"content":[{"type":"thinking","thinking":"private-thought"},{"type":"text","text":"Reviewing candidate.\\npassword=private-sample"}]}}`+"\n", session)
		time.Sleep(300 * time.Millisecond)
	}
	if strings.Contains(string(body), "FAIL_PROCESS") {
		return 15
	}
	if strings.Contains(string(body), "MISMATCH") {
		session = "00000000-0000-0000-0000-000000000000"
	}
	if strings.Contains(string(body), "NO_RESULT") {
		return 0
	}
	result := map[string]any{"type": "result", "subtype": "success", "session_id": session, "is_error": false, "structured_output": map[string]any{"summary": "No supported defects.", "findings": []any{}}}
	if strings.Contains(string(body), "BAD_OUTPUT") {
		delete(result, "structured_output")
	}
	if strings.Contains(string(body), "TERMINAL_ERROR") {
		result["is_error"] = true
	}
	if strings.Contains(string(body), "OVERSIZE") {
		result["padding"] = strings.Repeat("x", (1<<20)+1)
	}
	json.NewEncoder(os.Stdout).Encode(result)
	return 0
}

func TestReviewerProcessStreamsAndValidatesNativeResult(t *testing.T) {
	for _, mode := range []string{"current", "legacy", "FAIL_PROCESS", "MISMATCH", "NO_RESULT", "BAD_OUTPUT", "TERMINAL_ERROR", "OVERSIZE"} {
		t.Run(mode, func(t *testing.T) {
			w, j, base := fixture(t)
			t.Setenv("FORGEOS_REPO_ROOT", w.repoRoot)
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			binary := filepath.Join(t.TempDir(), "fake-review-claude.exe")
			executable, e := os.ReadFile(os.Args[0])
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(binary, executable, 0700); e != nil {
				t.Fatal(e)
			}
			t.Setenv("FORGEOS_CLAUDE_BIN", binary)
			if e = os.WriteFile(filepath.Join(j.RepoPath, "guide.md"), []byte("Queue guide.\n"), 0600); e != nil {
				t.Fatal(e)
			}
			git(t, j.RepoPath, "add", "guide.md")
			git(t, j.RepoPath, "-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-qm", "guide")
			commit := git(t, j.RepoPath, "rev-parse", "HEAD")
			var mu sync.Mutex
			var reports []store.ManagementProcessReport
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if r.URL.Path != "/internal/reviewer/process" {
					t.Error("wrong role endpoint")
				}
				var report store.ManagementProcessReport
				if json.NewDecoder(r.Body).Decode(&report) != nil || report.ReviewerExecution != 12 || report.Process.Harness != "claude" || !report.Process.Valid() {
					t.Error("invalid process")
				}
				reports = append(reports, report)
				rw.WriteHeader(204)
			}))
			defer server.Close()
			w.api, w.client, w.id = server.URL, server.Client(), "review-node"
			job := store.ReviewJob{ExecutionID: 12, Lease: "12345678-1234-1234-1234-123456789abc", CaptureProcess: mode != "legacy", Repo: j.RepoPath, Base: base, Commit: commit, Requirement: "Review queue guide. " + mode}
			report, err := w.reviewJob(context.Background(), job)
			if (err == nil) != (mode == "current" || mode == "legacy") {
				t.Fatal("wrong outcome", mode, err)
			}
			if err == nil && (report.Session != testClaudeID || report.Commit != commit || report.Profile == nil) {
				t.Fatal("wrong report")
			}
			mu.Lock()
			defer mu.Unlock()
			if mode == "legacy" {
				if len(reports) != 0 {
					t.Fatal("legacy fabricated process")
				}
				return
			}
			if len(reports) == 0 {
				t.Fatal("missing native process")
			}
			last := reports[len(reports)-1].Process
			if last.Exited == nil || last.ExitCode == nil || last.Session != testClaudeID || last.Output == nil || strings.Contains(last.Output.Text, "private-sample") || strings.Contains(last.Output.Text, "private-thought") {
				t.Fatal("invalid exit or public messages")
			}
			if mode == "FAIL_PROCESS" && *last.ExitCode != 15 {
				t.Fatal("wrong actual exit")
			}
		})
	}
}

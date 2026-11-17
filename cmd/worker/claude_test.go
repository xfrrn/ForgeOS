package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
)

const testClaudeID = "21a91faa-be24-43a7-9b4f-631bb115deef"

// The first process exits immediately after a real filesystem edit, before emitting its result.
func testClaudeProcess() int {
	body, err := io.ReadAll(os.Stdin)
	if err != nil || !slices.Contains(os.Args, "--restricted") || !slices.Contains(os.Args, "--safe-mode") {
		return 2
	}
	fmt.Printf(`{"type":"system","subtype":"init","session_id":"%s"}`+"\n", testClaudeID)
	if string(body) == "SIMULATED_PUBLIC_OUTPUT" {
		fmt.Printf(`{"type":"assistant","session_id":"%s","message":{"content":[{"type":"thinking","thinking":"private-thought"},{"type":"text","text":"Reading the guide.\npassword=private-value"}]}}`+"\n", testClaudeID)
		fmt.Printf(`{"type":"result","subtype":"success","session_id":"%s"}`+"\n", testClaudeID)
		return 0
	}
	if strings.Contains(string(body), "TERMINAL_FAILURE") {
		fmt.Printf(`{"type":"result","subtype":"error","is_error":true,"session_id":"%s"}`+"\n", testClaudeID)
		if strings.Contains(string(body), "ZERO_EXIT") {
			return 0
		}
		return 23
	}
	if !slices.Contains(os.Args, "--resume") {
		if os.WriteFile("resume.md", []byte("First edit.\n"), 0600) != nil {
			return 2
		}
		return 23
	}
	b, err := os.ReadFile("resume.md")
	if err != nil || string(b) != "First edit.\n" || !slices.Contains(os.Args, testClaudeID) || !strings.Contains(string(body), "interrupted") {
		return 2
	}
	if err := os.WriteFile("resume.md", append(b, []byte("Finished.\n")...), 0600); err != nil {
		return 2
	}
	fmt.Printf(`{"type":"result","subtype":"success","session_id":"%s"}`+"\n", testClaudeID)
	return 0
}

func TestClaudeProcessRecovery(t *testing.T) {
	for _, status := range []int{204, 409} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			ctx := context.Background()
			w, j, base := fixture(t)
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			var err error
			w.claude.bin, err = os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			w.claude.version = "test"
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/internal/executions/12/renew":
					rw.WriteHeader(status)
				case "/internal/executions/12/result":
					var result store.Result
					if err := json.NewDecoder(r.Body).Decode(&result); err != nil || result.Processes == nil {
						t.Error("missing process receipts", err)
					} else {
						rows := *result.Processes
						if len(rows) == 0 || rows[0].ExitCode == nil || *rows[0].ExitCode != 23 ||
							status == 204 && (len(rows) != 2 || rows[1].Session != rows[0].Session || rows[1].Instance <= rows[0].Instance) {
							t.Error("inconsistent recovered process history")
						}
					}
					rw.WriteHeader(204)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					rw.WriteHeader(400)
				}
			}))
			defer server.Close()
			w.api, w.client = server.URL, server.Client()
			w.journal, err = w.openJournal(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer w.journal.Close()
			j.Kind = "codex_issue"
			j.RecordProcesses = true
			j.Spec = &store.IssueSpec{Issue: store.Issue{Number: 7, Title: "Resume"}, Harness: "claude", BaseCommit: base, Validation: "docs"}
			if err := w.executeAndReport(ctx, j, time.Now()); err != nil {
				t.Fatal(err)
			}
			var count, firstExit, lastExit int
			rows, err := w.journal.Query("SELECT session_id,exit_code FROM harness_processes ORDER BY instance")
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			for rows.Next() {
				var id string
				if err := rows.Scan(&id, &lastExit); err != nil || id != testClaudeID {
					t.Fatal("unconfirmed process session", err)
				}
				if count == 0 {
					firstExit = lastExit
				}
				count++
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			var candidate string
			var exit int
			if err := w.journal.QueryRow("SELECT candidate,exit_code FROM attempts").Scan(&candidate, &exit); err != nil {
				t.Fatal(err)
			}
			if firstExit != 23 || status == 409 && (count != 1 || candidate != "" || exit == 0) ||
				status == 204 && (count != 2 || lastExit != 0 || exit != 0 || candidate == "" || git(t, j.Workspace, "show", "HEAD:resume.md") != "First edit.\nFinished.") {
				t.Fatalf("renew %d, processes %d, first/last exits %d/%d, result %d", status, count, firstExit, lastExit, exit)
			}
		})
	}
}

func TestClaudeStreamRejectsWrongSessionAndStructuredFailure(t *testing.T) {
	init := fmt.Sprintf(`{"type":"system","subtype":"init","session_id":"%s"}`, testClaudeID)
	confirm := func(string) error { return nil }
	if _, _, err := readClaudeEvents(strings.NewReader(init), "00000000-0000-0000-0000-000000000000", confirm, nil, nil); err == nil {
		t.Fatal("accepted an unrelated resumed session")
	}
	for _, fields := range []string{`"subtype":"error","is_error":false`, `"subtype":"success","is_error":true`} {
		failed := fmt.Sprintf(`{"type":"result",%s,"session_id":"%s"}`, fields, testClaudeID)
		if _, terminal, err := readClaudeEvents(strings.NewReader(init+"\n"+failed), "", confirm, nil, nil); !terminal || !errors.Is(err, errTurnFailed) {
			t.Fatal("lost terminal model failure", fields, err)
		}
	}
}

func TestClaudeMissingSessionBindsRequestedID(t *testing.T) {
	record := fmt.Sprintf(`{"type":"result","subtype":"error_during_execution","is_error":true,"errors":["No conversation found with session ID: %s"]}`, testClaudeID)
	init := fmt.Sprintf(`{"type":"system","subtype":"init","session_id":"%s"}`, testClaudeID)
	for _, tc := range []struct {
		stream, expected string
		missing          bool
	}{
		{record, testClaudeID, true},
		{record, "", false},
		{record, "00000000-0000-0000-0000-000000000000", false},
		{strings.Replace(record, "error_during_execution", "error", 1), testClaudeID, false},
		{init + "\n" + record, testClaudeID, false},
	} {
		_, _, err := readClaudeEvents(strings.NewReader(tc.stream), tc.expected, func(string) error { return nil }, nil, nil)
		if errors.Is(err, errSessionMissing) != tc.missing {
			t.Fatalf("unexpected missing-session classification for %q: %v", tc.expected, err)
		}
	}
}

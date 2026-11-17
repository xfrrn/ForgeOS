package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
	"forgeos/internal/validation"
)

func TestProcessEndpointAcceptsEscapedOutputWindows(t *testing.T) {
	ctx := context.Background()
	s := store.New(testdb.Open(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ImportIssue(ctx, p.ID, store.IssueSpec{Issue: store.Issue{RepositoryID: 1, ID: 1, Number: 1}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.Claim(ctx, "node", t.TempDir(), true)
	if err != nil || job == nil {
		t.Fatal(job, err)
	}
	now := time.Now().UTC()
	window := strings.Repeat("<", validation.LiveLogWindow-1) + "\n"
	report := store.ProcessReport{LeaseToken: job.LeaseToken, Process: store.HarnessProcess{
		Instance: 1, PID: 7, Harness: "codex", Version: "1.2.3", Session: "thread", Started: now,
		Output:     &store.ModelOutput{Sequence: 1, Text: window},
		Tools:      []store.ToolEvent{{Seq: 1, Call: 1, Tool: "command_execution", Phase: "completed", At: now.Unix()}},
		ToolOutput: &store.ToolOutput{Seq: 1, Call: 1, Tool: "command_execution", Text: window},
	}}
	api := routes(s, t.TempDir(), "owner", legacyWorker("node-token"), nil)
	post := func(want int) {
		t.Helper()
		body, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		if want == 204 && len(body) <= 16<<10 {
			t.Fatal("fixture does not exercise JSON expansion")
		}
		req := httptest.NewRequest("POST", fmt.Sprintf("/internal/executions/%d/process", job.ExecutionID), bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer node-token")
		res := httptest.NewRecorder()
		api.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatal(res.Code, res.Body.String())
		}
	}
	post(204)
	post(204)
	report.Process.ToolOutput.Text = "rewritten\n"
	post(409)
	report.Process.ToolOutput.Text = strings.Repeat("x", 128<<10)
	post(400)
}

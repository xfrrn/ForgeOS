package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/agents"
	"forgeos/internal/review"
	"forgeos/internal/store"
)

// Opt in to two model calls on synthetic inputs using the locally configured CLIs.
func TestLiveAlternateRoleHarnesses(t *testing.T) {
	if os.Getenv("FORGEOS_TEST_LIVE_HARNESSES") != "1" {
		t.Skip("requires two authenticated local harnesses")
	}
	for _, purpose := range []string{"reviewer", "planner"} {
		t.Run(purpose, func(t *testing.T) {
			h := "claude"
			if purpose == "reviewer" {
				h = "codex"
			}
			observations := make(chan store.AgentObservation, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var o store.AgentObservation
				if r.URL.Path != "/internal/agent-observations" || json.NewDecoder(r.Body).Decode(&o) != nil {
					w.WriteHeader(400)
					return
				}
				observations <- o
				w.WriteHeader(204)
			}))
			defer server.Close()
			a := agents.Assignment{Config: agents.Config{Key: purpose, Settings: agents.Settings{Harness: h, Prompt: agents.Builtin(purpose)}}, Ticket: "synthetic"}
			node := worker{roleAgent: &a, codex: os.Getenv("FORGEOS_CODEX_BIN"), client: server.Client(), api: server.URL, token: "synthetic"}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			switch purpose {
			case "reviewer":
				snapshot := review.Snapshot{Base: strings.Repeat("a", 40), Commit: strings.Repeat("b", 40), Files: []review.File{{Path: "hello.txt", Before: []string{"helo"}, After: []string{"hello"}}}}
				r, err := reviewSnapshot(node.agentContext(ctx, &a), snapshot, candidateReview{Requirement: "Correct the spelling in hello.txt."}, processLog{})
				if err != nil || snapshot.Verify(&r) != nil {
					t.Fatalf("review failed: %v", err)
				}
			case "planner":
				bin, err := claudeExecutable()
				if err != nil {
					t.Fatal(err)
				}
				p := claudePlanner{cli: claudeCLI{bin: bin}, output: filepath.Join(t.TempDir(), "plan.json")}
				_, err = p.runTurn(node.agentContext(ctx, &a), processLog{}, t.TempDir(), "Plan a single work item to write hello.txt containing hello. Return one milestone with one work item.", "")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = readPlannerOutput(p.output); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case o := <-observations:
				if o.Harness != h || o.Outcome != "completed" || o.Session == "" {
					t.Fatalf("incorrect execution receipt: %+v", o)
				}
			default:
				t.Fatal("missing harness observation")
			}
		})
	}
}

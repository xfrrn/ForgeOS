package store

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"forgeos/internal/agents"
	"forgeos/internal/testdb"
)

func TestAgentInheritanceAndFrozenClaim(t *testing.T) {
	ctx := context.Background()
	s := New(testdb.Open(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "agent-test")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := agents.Lookup("management-chat")
	read := func() AgentEntry {
		e, err := agentConfig(ctx, s.db, p.ID, d)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	first := read()
	global := agents.Settings{Harness: "codex", Model: "example-a", Prompt: "Answer briefly."}
	if err = s.SaveAgentConfig(ctx, 0, d, 0, 0, &global); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAgentConfig(ctx, p.ID, d, 0, first.Effective.Revision, &global); !errors.Is(err, ErrConflict) {
		t.Fatal("stale inherited config", err)
	}
	inherited := read()
	if !reflect.DeepEqual(inherited.Effective.Settings, global) || inherited.Effective.Source != "global" {
		t.Fatal(inherited)
	}
	local := global
	local.Model = "example-b"
	local.Harness = "claude"
	if err = s.SaveAgentConfig(ctx, p.ID, d, 0, inherited.Effective.Revision, &local); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	a, err := freezeAgent(ctx, tx, p.ID, d.Key, "node-a", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	overridden := read()
	if err = s.SaveAgentConfig(ctx, p.ID, d, overridden.LocalRevision, overridden.Effective.Revision, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(read().Effective.Settings, global) {
		t.Fatal("failed to restore inherited configuration")
	}
	if err = s.ObserveAgent(ctx, AgentObservation{Ticket: a.Ticket, Harness: "claude", Model: "observed-model", Outcome: "completed"}); err != nil {
		t.Fatal(err)
	}
	entries, err := s.AgentEntries(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Key == d.Key {
			if e.Last == nil || !reflect.DeepEqual(e.Last.Config.Settings, local) || e.Last.Model != "observed-model" || e.Effective.Source != "global" {
				t.Fatal("history changed after reset", e)
			}
			return
		}
	}
	t.Fatal("missing entry")
}

func TestAgentHarnessAtDispatchAndPromptAtClaim(t *testing.T) {
	ctx := context.Background()
	s := New(testdb.Open(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	d, _ := agents.Lookup("implementation")
	settings := agents.Settings{Harness: "claude", Model: "example-model", Prompt: "Implement the assigned task."}
	if err := s.SaveAgentConfig(ctx, 0, d, 0, 0, &settings); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, mode, workType, purpose, harness string
		explicit                               bool
	}{
		{"configured writer", "", "development", "implementation", "claude", false},
		{"explicit dispatch", "", "development", "implementation", "codex", true},
		{"operations planner", "plan", "operations", "operations", "codex", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, err := s.CreateProject(ctx, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			spec := IssueSpec{Issue: Issue{RepositoryID: p.ID, ID: p.ID, Number: 1, WorkType: test.workType},
				BaseCommit: strings.Repeat("a", 40), Validation: "docs", Harness: "codex", Mode: test.mode, ExplicitHarness: test.explicit}
			if _, err = s.ImportIssue(ctx, p.ID, spec); err != nil {
				t.Fatal(err)
			}
			caps := WorkerCapabilities{AgentSettings: true, SupportsClaude: true, SupportsCodex: true, SupportsPlanner: true}
			job, err := s.ClaimForProjects(ctx, "node-"+strconv.FormatInt(p.ID, 10), t.TempDir(), []int64{p.ID}, caps)
			if err != nil || job == nil || job.Agent == nil {
				t.Fatalf("no configured assignment: %v", err)
			}
			if job.Spec.Harness != test.harness || job.Agent.Key != test.purpose || job.Agent.Ticket == "" {
				t.Fatal("incorrect routing or configuration")
			}
			if test.mode == "plan" && job.Spec.ChildHarness != "claude" {
				t.Fatal("planned implementation lost its harness")
			}
			if test.mode == "" && !reflect.DeepEqual(job.Agent.Settings, settings) {
				t.Fatal("writer configuration was not frozen")
			}
		})
	}
}

func TestClaudePlannerIsClaimedOnlyByCapableNodes(t *testing.T) {
	ctx := context.Background()
	s := New(testdb.Open(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d, _ := agents.Lookup("planner")
	config := agents.Settings{Harness: "claude", Prompt: d.Prompt, Model: "sonnet"}
	if err = s.SaveAgentConfig(ctx, project.ID, d, 0, 0, &config); err != nil {
		t.Fatal(err)
	}
	spec := IssueSpec{Issue: Issue{RepositoryID: project.ID, ID: 1, Number: 1}, BaseCommit: strings.Repeat("a", 40), Validation: "docs", Mode: "plan"}
	if _, err = s.ImportIssue(ctx, project.ID, spec); err != nil {
		t.Fatal(err)
	}
	caps := WorkerCapabilities{AgentSettings: true, SupportsCodex: true, SupportsClaude: true, SupportsPlanner: true}
	claim := func() *Job {
		job, err := s.ClaimForProjects(ctx, "node", t.TempDir(), []int64{project.ID}, caps)
		if err != nil {
			t.Fatal(err)
		}
		return job
	}
	if claim() != nil {
		t.Fatal("legacy node claimed Claude planning")
	}
	caps.ClaudePlanning = true
	job := claim()
	if job == nil || job.Spec.Harness != "claude" || job.Spec.ChildHarness != "codex" || !reflect.DeepEqual(job.Agent.Settings, config) {
		t.Fatal("planner and writer choices did not survive dispatch")
	}
}

func TestAllAgentPurposesAcceptBothHarnesses(t *testing.T) {
	for _, d := range agents.Definitions {
		for _, h := range []string{"codex", "claude"} {
			if !d.Valid(agents.Settings{Prompt: d.Prompt, Harness: h}) {
				t.Errorf("%s rejects %s", d.Key, h)
			}
		}
	}
}

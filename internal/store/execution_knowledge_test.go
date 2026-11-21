package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestKnowledgeCitationsKeepReadVersionsAndCheckpointScope(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	p, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	spec := IssueSpec{Issue: Issue{ID: 2, Number: 1, RepositoryID: 3}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"}
	task, err := s.ImportIssue(ctx, p.ID, spec)
	must(err)
	job, err := s.ClaimForProjects(ctx, "writer", t.TempDir(), []int64{p.ID}, WorkerCapabilities{
		SupportsCodex: true, SupportsCheckpoints: true, SupportsKnowledgeTools: true,
	})
	must(err)
	if job == nil || !job.KnowledgeTools {
		t.Fatal("knowledge tools were not negotiated")
	}
	id, lease := job.ExecutionID, job.LeaseToken
	project, commit, err := s.KnowledgeScope(ctx, id, lease)
	must(err)
	if project != p.ID || commit != spec.BaseCommit {
		t.Fatalf("wrong scope %d %s", project, commit)
	}
	evidence := KnowledgeEvidence{KnowledgeHit: KnowledgeHit{Kind: "material", Ref: strings.Repeat("x", 3000), Title: "指南"}, Content: "原文"}
	if _, err = s.CiteKnowledge(ctx, id, lease, evidence.Kind, evidence.Ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cited unread evidence: %v", err)
	}
	_, err = s.RememberKnowledgeRead(ctx, id, lease, evidence)
	must(err)
	retry := evidence
	retry.Content = "different content"
	cached, err := s.RememberKnowledgeRead(ctx, id, lease, retry)
	must(err)
	if cached.Content != evidence.Content {
		t.Fatal("retry changed the saved source")
	}
	peek := func(want int) {
		t.Helper()
		snapshot, err := s.Task(ctx, task.ID)
		must(err)
		refs := snapshot.Execution.KnowledgeReferences
		if len(refs) != want {
			t.Fatalf("expected %d citations, got %s", want, refs)
		}
	}
	peek(0)
	if _, err = s.CiteKnowledge(ctx, id, "wrong", evidence.Kind, evidence.Ref); !errors.Is(err, ErrConflict) {
		t.Fatalf("wrong lease: %v", err)
	}
	for range 2 {
		stored, err := s.CiteKnowledge(ctx, id, lease, evidence.Kind, evidence.Ref)
		must(err)
		if stored.Content != evidence.Content {
			t.Fatal("citation did not return the original snapshot")
		}
	}
	peek(1)
	_, err = s.ReadCheckpoint(ctx, id, lease, 0, 0, "", false)
	must(err)
	_, err = s.ReadCheckpoint(ctx, id, lease, 0, 0, "writer-session", false)
	must(err)
	_, err = s.ReadCheckpoint(ctx, id, lease, 1, 0, "", false)
	must(err)
	other := KnowledgeEvidence{KnowledgeHit: KnowledgeHit{Kind: "git", Ref: "another-ref"}, Content: "later passage"}
	_, err = s.RememberKnowledgeRead(ctx, id, lease, other)
	must(err)
	_, err = s.CiteKnowledge(ctx, id, lease, other.Kind, other.Ref)
	must(err)
	peek(2)
	var sources []struct{ ID, Text string }
	var raw []byte
	must(s.db.QueryRowContext(ctx, "SELECT question_context_sources(execution_rule_context($1,0))", id).Scan(&raw))
	must(json.Unmarshal(raw, &sources))
	if len(sources) != 1 || !strings.HasPrefix(sources[0].ID, "knowledge:") || sources[0].Text != evidence.Content {
		t.Fatalf("question at checkpoint 0 has wrong sources: %s", raw)
	}
	_, err = s.db.ExecContext(ctx, "UPDATE executions SET ci_candidate=repeat('b',40),ci_candidate_session='writer-session' WHERE id=$1", id)
	must(err)
	must(s.db.QueryRowContext(ctx, "SELECT review_rules_for_execution($1)->'knowledge_references'", id).Scan(&raw))
	var refs []KnowledgeEvidence
	must(json.Unmarshal(raw, &refs))
	if len(refs) != 2 || refs[0].Content != evidence.Content || refs[1].Content != other.Content {
		t.Fatalf("wrong candidate context: %s", raw)
	}
	if _, err = s.CiteKnowledge(ctx, id, lease, evidence.Kind, evidence.Ref); !errors.Is(err, ErrConflict) {
		t.Fatalf("candidate already finalized: %v", err)
	}
}

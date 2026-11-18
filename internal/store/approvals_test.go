package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestPendingApprovalCursorSurvivesDecision(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, "approval-queue")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 33; i++ {
		p := DecisionProposal{RequestKey: fmt.Sprintf("pending-%d", i), Statement: "Review the guide", Source: "handbook", Validity: "current release", EvidenceRefs: []string{"guide.md"}}
		if i == 32 {
			end := time.Now().Add(-time.Hour)
			p.ValidUntil = &end
		}
		if _, err = s.ProposeDecision(ctx, project.ID, p); err != nil {
			t.Fatal(err)
		}
	}
	type response struct {
		Items []struct {
			Kind    string
			ID      int64
			Project int64 `json:"project_id"`
		}
		Next string `json:"next_before"`
	}
	read := func(kind, beforeKind string, before int64) response {
		t.Helper()
		data, err := s.PendingApprovals(ctx, kind, beforeKind, before, false)
		if err != nil {
			t.Fatal(err)
		}
		var page response
		if err = json.Unmarshal(data, &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	first := read("", "", 0)
	if len(first.Items) != 30 || first.Next != "decision.3" || first.Items[0].ID != 32 || first.Items[0].Project != project.ID {
		t.Fatal(first)
	}
	if _, err = s.Decide(ctx, project.ID, 3, "CONFIRMED"); err != nil {
		t.Fatal(err)
	}
	next := read("decision", "decision", 3)
	if next.Next != "" || len(next.Items) != 2 || next.Items[0].ID != 2 || next.Items[1].ID != 1 {
		t.Fatal(next)
	}
	if p := read("share", "", 0); len(p.Items) != 0 || p.Next != "" {
		t.Fatal(p)
	}
}

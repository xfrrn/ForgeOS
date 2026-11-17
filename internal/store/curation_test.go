package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestOwnerMemoryIsOnePrivateCurationSourceOnRetry(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "owner-memory")
	if err != nil {
		t.Fatal(err)
	}
	m := MaterialInput{Kind: "memory", Title: "Queue checkpoint", Source: "Owner replay log", Content: "The queue resumed at offset 274."}
	for range 2 {
		if _, err := s.SaveMaterial(ctx, p.ID, "memory-queue", m); err != nil {
			t.Fatal(err)
		}
	}
	job, err := s.ClaimCuration(ctx, p.ID, false)
	if err != nil || job == nil {
		t.Fatal(job, err)
	}
	var source struct {
		MaterialInput
		Revision int
	}
	if err := json.Unmarshal(job.Material, &source); err != nil || source.Revision != 1 || source.MaterialInput != m {
		t.Fatal(source, err)
	}
	if next, err := s.ClaimCuration(ctx, p.ID, false); err != nil || next != nil {
		t.Fatal(next, err)
	}
	if list, err := s.ProjectExperience(ctx, p.ID, 0); err != nil || string(list) != "[]" {
		t.Fatal(string(list), err)
	}
}

func TestCurationDecisionRequiresEscalatedReview(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, "source-permissions")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		level, verdict string
		eligible       bool
	}{{"D1", "approve", false}, {"D2", "revise", false}, {"D3", "needs_owner", true}} {
		t.Run(tc.level+tc.verdict, func(t *testing.T) {
			source := MaterialInput{Kind: "memory", Title: "Pilot", Source: "Owner minutes", Content: "Twelve percent."}
			if _, err := s.SaveMaterial(ctx, project.ID, tc.level, source); err != nil {
				t.Fatal(err)
			}
			job, err := s.ClaimCuration(ctx, project.ID, false)
			if err != nil || job == nil {
				t.Fatal(job, err)
			}
			if err := s.RecordExtraction(ctx, project.ID, job.ID, job.Lease, Extraction{"extract", []KnowledgeFact{{"Proposed cap.", source.Content}}}); err != nil {
				t.Fatal(err)
			}
			review, err := s.ClaimCuration(ctx, project.ID, true)
			if err != nil || review == nil {
				t.Fatal(review, err)
			}
			if err := s.ReviewCuration(ctx, project.ID, review.ID, review.Lease, ManagementReview{
				Level: tc.level, Verdict: tc.verdict, Rationale: "Review the pilot scope.", Evidence: source.Content, SessionID: "manager",
			}); err != nil {
				t.Fatal(err)
			}
			_, err = s.ProposeDecision(ctx, project.ID, DecisionProposal{
				RequestKey: tc.level, Statement: "Use the pilot cap.", Validity: "Pilot only", Curation: &DecisionOrigin{job.ID, 1},
			})
			if (tc.eligible && err != nil) || (!tc.eligible && !errors.Is(err, ErrConflict)) {
				t.Fatal("eligibility", tc.eligible, err)
			}
		})
	}
}

func TestCurationBindsSourceLeaseAndIndependentReview(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "curation")
	if err != nil {
		t.Fatal(err)
	}
	input := MaterialInput{Kind: "material", Title: "Queue", Source: "test log", Content: "The queue resumed at offset 274."}
	for range 2 {
		if _, err = s.SaveMaterial(ctx, p.ID, "queue", input); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.ClaimCuration(ctx, p.ID, false)
	if err != nil || first == nil {
		t.Fatal(first, err)
	}
	if j, err := s.ClaimCuration(ctx, p.ID, false); err != nil || j != nil {
		t.Fatal("duplicate claim", j, err)
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE material_curations SET extraction_until=now() WHERE id=$1", first.ID); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimCuration(ctx, p.ID, false)
	if err != nil || job == nil || job.Lease == first.Lease {
		t.Fatal(job, err)
	}
	out := Extraction{Session: "curator-1", Facts: []KnowledgeFact{{Statement: "Resume offset was recorded.", Quote: "offset 274"}}}
	for _, v := range []struct {
		p     int64
		lease string
		out   Extraction
	}{
		{p.ID, first.Lease, out}, {p.ID + 1, job.Lease, out}, {p.ID, job.Lease, Extraction{Session: "bad", Facts: []KnowledgeFact{{"unsupported", "offset 999"}}}},
	} {
		if err = s.RecordExtraction(ctx, v.p, job.ID, v.lease, v.out); !errors.Is(err, ErrConflict) {
			t.Fatal("invalid extraction", err)
		}
	}
	for range 2 {
		if err = s.RecordExtraction(ctx, p.ID, job.ID, job.Lease, out); err != nil {
			t.Fatal(err)
		}
	}
	j, err := s.ClaimCuration(ctx, p.ID, true)
	if err != nil || j == nil || j.Extraction.Session != out.Session {
		t.Fatal(j, err)
	}
	r := ManagementReview{Level: "D2", Verdict: "approve", Rationale: "Requires owner", Evidence: "offset 274", SessionID: out.Session}
	proposal := DecisionProposal{RequestKey: "queue-decision", Statement: "Preserve the checkpoint.", Validity: "Queue recovery",
		Curation: &DecisionOrigin{ID: j.ID, Fact: 1}}
	if _, err := s.ProposeDecision(ctx, p.ID, proposal); !errors.Is(err, ErrConflict) {
		t.Fatal("proposal without review", err)
	}
	if err = s.ReviewCuration(ctx, p.ID, j.ID, j.Lease, r); !errors.Is(err, ErrConflict) {
		t.Fatal("same-session review", err)
	}
	r.SessionID = "management-1"
	for range 2 {
		if err = s.ReviewCuration(ctx, p.ID, j.ID, j.Lease, r); err != nil {
			t.Fatal(err)
		}
	}
	r.Level = "D1"
	if err = s.ReviewCuration(ctx, p.ID, j.ID, j.Lease, r); !errors.Is(err, ErrConflict) {
		t.Fatal("changed receipt", err)
	}
	if list, err := s.ProjectExperience(ctx, p.ID, 0); err != nil || string(list) != "[]" {
		t.Fatal("higher-level note published", string(list), err)
	}
	proposal.Curation.Fact = 2
	if _, err := s.ProposeDecision(ctx, p.ID, proposal); !errors.Is(err, ErrConflict) {
		t.Fatal("absent fact", err)
	}
	proposal.Curation.Fact = 1
	if _, err := s.ProposeDecision(ctx, p.ID+1, proposal); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign source", err)
	}
	var decision struct {
		ID     int64
		Status string
		Source string
	}
	var original json.RawMessage
	for range 2 {
		doc, err := s.ProposeDecision(ctx, p.ID, proposal)
		if err != nil || json.Unmarshal(doc, &decision) != nil || decision.Status != "PROPOSED" || decision.Source == "" {
			t.Fatal(string(doc), err)
		}
		if original != nil && string(original) != string(doc) {
			t.Fatal("changed proposal on retry")
		}
		original = doc
	}
	proposal.Statement = "Change the payload on retry."
	if _, err := s.ProposeDecision(ctx, p.ID, proposal); !errors.Is(err, ErrConflict) {
		t.Fatal("changed proposal on retry", err)
	}
	if _, err := s.Decision(ctx, p.ID+1, decision.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign decision", err)
	}
	if _, err := s.Decide(ctx, p.ID, decision.ID, "REJECTED"); err != nil {
		t.Fatal(err)
	}
	input.PreviousRevision = 1
	input.Content = "No recorded result."
	if _, err = s.SaveMaterial(ctx, p.ID, "queue", input); err != nil {
		t.Fatal(err)
	}
	next, err := s.ClaimCuration(ctx, p.ID, false)
	if err != nil || next == nil || next.ID == job.ID {
		t.Fatal(next, err)
	}
	if err = s.RecordExtraction(ctx, p.ID, next.ID, next.Lease, Extraction{Session: "curator-2", Facts: []KnowledgeFact{}}); err != nil {
		t.Fatal(err)
	}
	if j, err := s.ClaimCuration(ctx, p.ID, true); err != nil || j != nil {
		t.Fatal("empty extraction scheduled", j, err)
	}
	raw, err := s.MaterialCuration(ctx, p.ID, "queue", 1)
	var receipt struct {
		Review     ManagementReview
		Extraction Extraction
		Decisions  []struct {
			ID     int64
			Fact   int
			Status string
		}
	}
	if err != nil || json.Unmarshal(raw, &receipt) != nil || receipt.Review.Level != "D2" || receipt.Extraction.Session != out.Session {
		t.Fatal(string(raw), err)
	}
	if len(receipt.Decisions) != 1 || receipt.Decisions[0].ID != decision.ID || receipt.Decisions[0].Fact != 1 || receipt.Decisions[0].Status != "REJECTED" {
		t.Fatal("missing disposition", string(raw))
	}
}

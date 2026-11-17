package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

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
	}
	if err != nil || json.Unmarshal(raw, &receipt) != nil || receipt.Review.Level != "D2" || receipt.Extraction.Session != out.Session {
		t.Fatal(string(raw), err)
	}
}

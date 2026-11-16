package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestShareRequiresReviewedCopyAndRecipientGrant(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	source, err := s.CreateProject(ctx, "source")
	if err != nil {
		t.Fatal(err)
	}
	target, err := s.CreateProject(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}
	m := MaterialInput{Kind: "material", Title: "private", Source: "private-source", Content: "private-source INTERNAL"}
	if _, err = s.SaveMaterial(ctx, source.ID, "notes", m); err != nil {
		t.Fatal(err)
	}
	input := ShareDraft{MaterialKey: "notes", Revision: 1, Recipient: target.ID, Title: "General handoff", Content: "Record the acceptance outcome."}
	raw, err := s.DraftShare(ctx, source.ID, input)
	var receipt struct {
		ID     int64
		Digest string
	}
	if err != nil || json.Unmarshal(raw, &receipt) != nil || receipt.ID < 1 {
		t.Fatal(string(raw), err)
	}
	id := receipt.ID
	outgoing, err := s.OutgoingShares(ctx, source.ID, 0)
	if err != nil || !strings.Contains(string(outgoing), receipt.Digest) || strings.Contains(string(outgoing), "review_lease") {
		t.Fatal(string(outgoing), err)
	}
	for _, page := range []struct{ project, before int64 }{{target.ID, 0}, {source.ID, id}} {
		items, err := s.OutgoingShares(ctx, page.project, page.before)
		if err != nil || string(items) != "[]" {
			t.Fatal(string(items), err)
		}
	}
	job, err := s.ClaimShareReview(ctx, source.ID)
	if err != nil || job == nil || job.Share.OriginalContent != m.Content {
		t.Fatal(err)
	}
	review := ManagementReview{Level: "D2", Verdict: "approve", SessionID: "test-session", Rationale: "reviewed", Evidence: "private-source only in original"}
	if err = s.ReviewShare(ctx, target.ID, id, job.Lease, review); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err = s.ReviewShare(ctx, source.ID, id, "wrong", review); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err = s.SetSharePublished(ctx, source.ID, id, receipt.Digest, true); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	for range 2 {
		if err = s.ReviewShare(ctx, source.ID, id, job.Lease, review); err != nil {
			t.Fatal(err)
		}
	}
	modified := review
	modified.Verdict = "revise"
	if err = s.ReviewShare(ctx, source.ID, id, job.Lease, modified); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err = s.SetSharePublished(ctx, source.ID, id, receipt.Digest, true); err != nil {
		t.Fatal(err)
	}
	feed, err := s.SharedExperience(ctx, target.ID, 0)
	if err != nil || !strings.Contains(string(feed), input.Content) || strings.Contains(string(feed), "private-source") {
		t.Fatal(string(feed), err)
	}
	if _, err = s.SetSharePublished(ctx, source.ID, id, receipt.Digest, false); err != nil {
		t.Fatal(err)
	}
	feed, err = s.SharedExperience(ctx, target.ID, 0)
	if err != nil || string(feed) != "[]" {
		t.Fatal(string(feed), err)
	}
}

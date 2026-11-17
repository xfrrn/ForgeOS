package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestShareProcessIsPrivateAndRetainsRevokedAndExpiredAttempts(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	conflict := func(e error) {
		t.Helper()
		if !errors.Is(e, ErrConflict) {
			t.Fatal("expected conflict", e)
		}
	}
	must(s.Migrate(ctx))
	source, e := s.CreateProject(ctx, "source")
	must(e)
	recipient, e := s.CreateProject(ctx, "recipient")
	must(e)
	_, e = s.SaveMaterial(ctx, source.ID, "notes", MaterialInput{Kind: "material", Title: "Source", Source: "internal-record", Content: "Private case. Keep a durable offset before restarting a stream."})
	must(e)
	draft := func(title string) (int64, string) {
		raw, e := s.DraftShare(ctx, source.ID, ShareDraft{MaterialKey: "notes", Revision: 1, Recipient: recipient.ID, Title: title, Content: "Persist the last confirmed offset before a restart."})
		must(e)
		var row struct {
			ID     int64
			Digest string
		}
		must(json.Unmarshal(raw, &row))
		return row.ID, row.Digest
	}
	id, digest := draft("Restart offsets")
	job, e := s.ClaimShareReview(ctx, source.ID, true)
	must(e)
	r := ManagementProcessReport{ShareID: id, Lease: job.Lease, Node: "review-source", Process: HarnessProcess{Instance: 1, PID: 123, Harness: "codex", Version: "1.2.3", Session: "share-thread", Started: time.Now().UTC().Truncate(time.Second), Output: &ModelOutput{Sequence: 1, Text: "Private reviewer rationale.\n"}}}
	conflict(s.ObserveManagementProcess(ctx, recipient.ID, r))
	bad := r
	bad.TaskID = 1
	bad.Revision = 1
	conflict(s.ObserveManagementProcess(ctx, source.ID, bad))
	bad = r
	bad.ShareID++
	conflict(s.ObserveManagementProcess(ctx, source.ID, bad))
	must(s.ObserveManagementProcess(ctx, source.ID, r))
	review := ManagementReview{Level: "D3", Verdict: "approve", Rationale: "Supported", Evidence: "Source", SessionID: r.Process.Session}
	conflict(s.ReviewShare(ctx, source.ID, id, job.Lease, review))
	done := r.Process.Started.Add(time.Second)
	code := 0
	r.Process.Exited = &done
	r.Process.ExitCode = &code
	must(s.ObserveManagementProcess(ctx, source.ID, r))
	wrong := review
	wrong.SessionID = "other"
	conflict(s.ReviewShare(ctx, source.ID, id, job.Lease, wrong))
	must(s.ReviewShare(ctx, source.ID, id, job.Lease, review))
	must(s.ReviewShare(ctx, source.ID, id, job.Lease, review))
	conflict(s.ObserveManagementProcess(ctx, source.ID, r))
	_, e = s.SetSharePublished(ctx, source.ID, id, digest, true)
	must(e)
	raw, e := s.SharedExperience(ctx, recipient.ID, 0)
	must(e)
	if strings.Contains(string(raw), "review") || strings.Contains(string(raw), "share-thread") || strings.Contains(string(raw), "process") {
		t.Fatal("private observation disclosed")
	}
	var rows []map[string]any
	must(json.Unmarshal(raw, &rows))
	if len(rows) != 1 || len(rows[0]) != 5 {
		t.Fatal("recipient fields")
	}
	raw, e = s.ShareReceipt(ctx, source.ID, id)
	must(e)
	var receipt struct {
		ReviewProcesses []ManagementProcess `json:"review_processes"`
	}
	must(json.Unmarshal(raw, &receipt))
	if len(receipt.ReviewProcesses) != 1 || receipt.ReviewProcesses[0].Node != r.Node {
		t.Fatal("missing source record")
	}
	_, e = s.ShareReceipt(ctx, recipient.ID, id)
	if !errors.Is(e, ErrNotFound) {
		t.Fatal("recipient read private record", e)
	}
	raw, e = s.OutgoingShares(ctx, source.ID, 0)
	must(e)
	if !strings.Contains(string(raw), "share-thread") {
		t.Fatal("missing list history")
	}
	id, digest = draft("Revoked offset guidance")
	job, e = s.ClaimShareReview(ctx, source.ID, true)
	must(e)
	r.ShareID = id
	r.Lease = job.Lease
	r.Process.Exited = nil
	r.Process.ExitCode = nil
	must(s.ObserveManagementProcess(ctx, source.ID, r))
	_, e = s.SetSharePublished(ctx, source.ID, id, digest, false)
	must(e)
	// Revocation cannot hide the actual exit or release the live model prematurely.
	conflict(s.ReviewShare(ctx, source.ID, id, job.Lease, review))
	r.Process.Exited = &done
	r.Process.ExitCode = &code
	must(s.ObserveManagementProcess(ctx, source.ID, r))
	conflict(s.ReviewShare(ctx, source.ID, id, job.Lease, review))
	var active bool
	must(s.db.QueryRowContext(ctx, "SELECT review_until>clock_timestamp() FROM knowledge_shares WHERE id=$1", id).Scan(&active))
	if active {
		t.Fatal("revoked slot not released")
	}
	_, e = s.SetSharePublished(ctx, source.ID, id, digest, true)
	conflict(e)
	raw, e = s.ShareReceipt(ctx, source.ID, id)
	must(e)
	must(json.Unmarshal(raw, &receipt))
	if len(receipt.ReviewProcesses) != 1 || receipt.ReviewProcesses[0].Exited == nil {
		t.Fatal("revoked exit lost")
	}
	id, _ = draft("Expired retry")
	job, e = s.ClaimShareReview(ctx, source.ID, true)
	must(e)
	r.ShareID = id
	r.Lease = job.Lease
	r.Process.Exited = nil
	r.Process.ExitCode = nil
	must(s.ObserveManagementProcess(ctx, source.ID, r))
	_, e = s.db.ExecContext(ctx, "UPDATE knowledge_shares SET review_until=now() WHERE id=$1", id)
	must(e)
	conflict(s.ObserveManagementProcess(ctx, source.ID, r))
	job, e = s.ClaimShareReview(ctx, source.ID, true)
	must(e)
	r.Lease = job.Lease
	r.Node = "replacement"
	r.Process.PID = 456
	must(s.ObserveManagementProcess(ctx, source.ID, r))
	raw, e = s.ShareReceipt(ctx, source.ID, id)
	must(e)
	must(json.Unmarshal(raw, &receipt))
	if len(receipt.ReviewProcesses) != 2 {
		t.Fatal("expired attempt overwritten")
	}
}

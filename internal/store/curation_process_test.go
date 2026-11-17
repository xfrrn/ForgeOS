package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestCurationProcessesBindRoleLeaseAndQuoteRepair(t *testing.T) {
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
	project, e := s.CreateProject(ctx, t.TempDir())
	must(e)
	_, e = s.SaveMaterial(ctx, project.ID, "offset", MaterialInput{Kind: "material", Title: "Recovery", Source: "run", Content: "Saved 918 then resumed at 918."})
	must(e)
	j, e := s.ClaimCuration(ctx, project.ID, false, true)
	must(e)
	p := ManagementProcessReport{CurationID: j.ID, CurationRole: "curator", Turn: 1, Lease: j.Lease, Node: "knowledge-node", Process: HarnessProcess{Instance: 1, PID: 123, Harness: "codex", Version: "1.2.3", Session: "invalid-quote", Started: time.Now().UTC().Truncate(time.Second)}}
	observe := func() error { return s.ObserveManagementProcess(ctx, project.ID, p) }
	p.Turn = 2
	conflict(observe())
	p.Turn = 1
	p.CurationRole = "management"
	conflict(observe())
	p.CurationRole = "curator"
	p.FeedbackID = 1
	conflict(observe())
	p.FeedbackID = 0
	conflict(s.ObserveManagementProcess(ctx, project.ID+1, p))
	must(observe())
	out := Extraction{Session: "invalid-quote", Facts: []KnowledgeFact{{Statement: "Resumed.", Quote: "resumed at 918"}}}
	conflict(s.RecordExtraction(ctx, project.ID, j.ID, j.Lease, out))
	first := p
	p.Turn = 2
	p.Process.Instance = 2
	p.Process.Session = "corrected"
	conflict(observe())
	p = first
	end := p.Process.Started.Add(time.Second)
	code := 0
	p.Process.Exited = &end
	p.Process.ExitCode = &code
	must(observe())
	p.Turn = 2
	p.Process.Instance = 2
	p.Process.PID = 456
	p.Process.Session = "corrected"
	p.Process.Started = end
	p.Process.Exited = nil
	p.Process.ExitCode = nil
	p.Process.Output = &ModelOutput{Sequence: 1, Text: "Checking exact quote.\n"}
	p.Node = "another-node"
	conflict(observe())
	p.Node = "knowledge-node"
	p.Process.Session = "invalid-quote"
	conflict(observe())
	p.Process.Session = "corrected"
	p.Process.Instance = 1
	conflict(observe())
	p.Process.Instance = 2
	p.Process.Started = end.Add(-time.Second)
	conflict(observe())
	p.Process.Started = end
	must(observe())
	conflict(s.RecordExtraction(ctx, project.ID, j.ID, j.Lease, out))
	out.Session = "corrected"
	conflict(s.RecordExtraction(ctx, project.ID, j.ID, j.Lease, out))
	p.Turn = 3
	conflict(observe())
	p.Turn = 2
	end2 := end.Add(time.Second)
	p.Process.Exited = &end2
	p.Process.ExitCode = &code
	must(observe())
	invalid := out
	invalid.Session = "invalid-quote"
	conflict(s.RecordExtraction(ctx, project.ID, j.ID, j.Lease, invalid))
	must(s.RecordExtraction(ctx, project.ID, j.ID, j.Lease, out))
	must(s.RecordExtraction(ctx, project.ID, j.ID, j.Lease, out))
	conflict(observe())
	review, e := s.ClaimCuration(ctx, project.ID, true, true)
	must(e)
	p = ManagementProcessReport{CurationID: j.ID, CurationRole: "management", Turn: 1, Lease: review.Lease, Node: "review-node", Process: HarnessProcess{Instance: 1, PID: 789, Harness: "codex", Version: "1.2.3", Session: "failed-review", Started: end2}}
	p.Turn = 2
	conflict(observe())
	p.Turn = 1
	must(observe())
	r := ManagementReview{Level: "D1", Verdict: "approve", SessionID: p.Process.Session, Rationale: "Observed.", Evidence: "918"}
	conflict(s.ReviewCuration(ctx, project.ID, j.ID, review.Lease, r))
	code = 15
	end3 := end2.Add(time.Second)
	p.Process.Exited = &end3
	p.Process.ExitCode = &code
	must(observe())
	conflict(s.ReviewCuration(ctx, project.ID, j.ID, review.Lease, r))
	_, e = s.db.ExecContext(ctx, "UPDATE material_curations SET review_until=now() WHERE id=$1", j.ID)
	must(e)
	retry, e := s.ClaimCuration(ctx, project.ID, true, true)
	must(e)
	conflict(observe())
	p.Lease = retry.Lease
	p.Process.Instance = 2
	p.Process.PID = 790
	p.Process.Session = "reviewed"
	p.Process.Started = end3
	p.Process.Exited = nil
	p.Process.ExitCode = nil
	must(observe())
	code = 0
	end4 := end3.Add(time.Second)
	p.Process.Exited = &end4
	p.Process.ExitCode = &code
	must(observe())
	r.SessionID = "reviewed"
	must(s.ReviewCuration(ctx, project.ID, j.ID, retry.Lease, r))
	must(s.ReviewCuration(ctx, project.ID, j.ID, retry.Lease, r))
	conflict(observe())
	raw, e := s.MaterialCuration(ctx, project.ID, "offset", 1)
	must(e)
	var receipt struct {
		Extraction []ManagementProcess `json:"extraction_processes"`
		Review     []ManagementProcess `json:"review_processes"`
	}
	must(json.Unmarshal(raw, &receipt))
	if len(receipt.Extraction) != 2 || len(receipt.Review) != 2 || receipt.Extraction[1].Session != "corrected" || *receipt.Review[0].ExitCode != 15 || receipt.Review[1].Session != "reviewed" {
		t.Fatal("lost role histories", string(raw))
	}
}

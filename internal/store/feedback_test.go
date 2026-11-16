package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestConcurrentFeedbackRecordsOneEvent(t *testing.T) {
	db := isolatedDatabase(t)
	s := New(db)
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("b", 40)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{ID: 17, Number: 17, RepositoryID: 1}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	var execution int64
	err = db.QueryRowContext(ctx, `INSERT INTO executions(task_id,worker,workspace,lease_hash,lease_until,result) VALUES($1,'test','work','hash',now(),
		jsonb_build_object('commit',$2::text,'exit_code',0,'validation',jsonb_build_object('commit',$2::text,'exit_code',0))) RETURNING id`, task.ID, sha).Scan(&execution)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.OpenPreview(ctx, task.ID, execution, sha, "public", 8091, 60)
	if err != nil {
		t.Fatal(err)
	}
	var preview struct{ ID string }
	if err = json.Unmarshal(out, &preview); err != nil {
		t.Fatal(err)
	}
	f := PreviewFeedback{Request: "b52facfe-ad63-43c9-bcfe-f436a5728d8b", Execution: execution, Commit: sha, Path: "/", Note: "Observed the count."}
	type response struct {
		id  int64
		err error
	}
	results := make(chan response, 4)
	for range cap(results) {
		go func() { id, err := s.SubmitPreviewFeedback(ctx, task.ID, preview.ID, f); results <- response{id, err} }()
	}
	var first int64
	for range cap(results) {
		r := <-results
		if r.err != nil || r.id <= 0 || (first != 0 && first != r.id) {
			t.Fatalf("feedback result %v", r)
		}
		first = r.id
	}
	var count int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM task_events WHERE task_id=$1 AND type='preview_feedback_received'`, task.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("events=%d error=%v", count, err)
	}
	if _, err = s.SubmitPreviewFeedback(ctx, task.ID+1, preview.ID, f); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-task replay: %v", err)
	}
	f.Note = "different observation"
	if _, err = s.SubmitPreviewFeedback(ctx, task.ID, preview.ID, f); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed replay: %v", err)
	}
	t.Run("screenshot receipt follows storage and scope", func(t *testing.T) {
		digest := strings.Repeat("a", 64)
		diskErr := errors.New("storage unavailable")
		if _, err := s.SaveFeedbackScreenshot(ctx, task.ID, preview.ID, first, digest, 42, func(string) error { return diskErr }); !errors.Is(err, diskErr) {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM feedback_artifacts").Scan(&count); err != nil || count != 0 {
			t.Fatal(count, err)
		}
		var id int64
		for range 2 {
			a, err := s.SaveFeedbackScreenshot(ctx, task.ID, preview.ID, first, digest, 42, func(string) error { return nil })
			if err != nil || (id != 0 && id != a.ID) {
				t.Fatal(a, err)
			}
			id = a.ID
		}
		if _, err := s.FeedbackArtifact(ctx, task.ID+1, preview.ID, first, id); !errors.Is(err, ErrNotFound) {
			t.Fatal("cross-task read", err)
		}
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM task_events WHERE type='feedback_screenshot_saved'").Scan(&count); err != nil || count != 1 {
			t.Fatal(count, err)
		}
	})
	t.Run("management lease and provenance", func(t *testing.T) {
		if j, err := s.ClaimFeedback(ctx, p.ID+1); err != nil || j != nil {
			t.Fatalf("other project: %v %v", j, err)
		}
		j, err := s.ClaimFeedback(ctx, p.ID)
		if err != nil || j == nil || j.ID != first {
			t.Fatalf("claim: %v %v", j, err)
		}
		if second, err := s.ClaimFeedback(ctx, p.ID); err != nil || second != nil {
			t.Fatalf("duplicate claim: %v %v", second, err)
		}
		c := FeedbackClassification{Category: "acceptance", Rationale: "The observer verified the displayed count.", Evidence: "Observed the count.", Session: "qa-triage"}
		if _, err := db.ExecContext(ctx, `UPDATE preview_feedback SET review_until=now()-interval '1 second' WHERE id=$1`, first); err != nil {
			t.Fatal(err)
		}
		if err = s.ClassifyFeedback(ctx, p.ID, first, j.Lease, c); !errors.Is(err, ErrConflict) {
			t.Fatalf("expired result: %v", err)
		}
		next, err := s.ClaimFeedback(ctx, p.ID)
		if err != nil || next == nil || next.Lease == j.Lease {
			t.Fatalf("renewal: %v %v", next, err)
		}
		invalid := c
		invalid.Evidence = "unreported evidence"
		if err = s.ClassifyFeedback(ctx, p.ID, first, next.Lease, invalid); !errors.Is(err, ErrConflict) {
			t.Fatalf("unreported evidence: %v", err)
		}
		if err = s.ClassifyFeedback(ctx, p.ID+1, first, next.Lease, c); !errors.Is(err, ErrConflict) {
			t.Fatalf("wrong project result: %v", err)
		}
		for range 2 {
			if err = s.ClassifyFeedback(ctx, p.ID, first, next.Lease, c); err != nil {
				t.Fatal(err)
			}
		}
		invalid = c
		invalid.Category = "bug"
		if err = s.ClassifyFeedback(ctx, p.ID, first, next.Lease, invalid); !errors.Is(err, ErrConflict) {
			t.Fatalf("changed classification: %v", err)
		}
		if err = db.QueryRowContext(ctx, `SELECT count(*) FROM task_events WHERE task_id=$1 AND type='preview_feedback_classified'`, task.ID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("classification events %d: %v", count, err)
		}
	})
	t.Run("dispatch an independent work item once", func(t *testing.T) {
		target := *task.Spec
		target.Issue.ID++
		target.Issue.Number++
		source := FeedbackSource{FeedbackID: first, TaskID: task.ID, Preview: preview.ID, Execution: execution, Commit: sha}
		if _, err := s.ImportIssue(ctx, p.ID, target, source); !errors.Is(err, ErrConflict) {
			t.Fatal("dispatched acceptance feedback", err)
		}
		f.Request = "502453fb-9f35-466b-ac0c-7908e73acb14"
		f.Note = "Document the history report separately."
		id, err := s.SubmitPreviewFeedback(ctx, task.ID, preview.ID, f)
		if err != nil {
			t.Fatal(err)
		}
		source.FeedbackID = id
		j, err := s.ClaimFeedback(ctx, p.ID)
		if err != nil || j == nil || j.ID != id {
			t.Fatal(j, err)
		}
		if err := s.ClassifyFeedback(ctx, p.ID, id, j.Lease, FeedbackClassification{Category: "new_work_item", Evidence: f.Note, Rationale: "Independent document", Session: "new-document"}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.FeedbackDispatch(ctx, p.ID, task.Spec.Issue.Number, source); !errors.Is(err, ErrConflict) {
			t.Fatal("accepted original Issue", err)
		}
		if _, err := s.FeedbackDispatch(ctx, p.ID+1, target.Issue.Number, source); !errors.Is(err, ErrConflict) {
			t.Fatal("accepted wrong project", err)
		}
		results := make(chan response, 3)
		for range cap(results) {
			go func() { item, err := s.ImportIssue(ctx, p.ID, target, source); results <- response{item.ID, err} }()
		}
		var itemID int64
		for range cap(results) {
			r := <-results
			if r.err != nil || r.id == 0 || (itemID != 0 && r.id != itemID) {
				t.Fatal(r)
			}
			itemID = r.id
		}
		saved, err := s.FeedbackDispatch(ctx, p.ID, target.Issue.Number, source)
		if err != nil || saved == nil || saved.ID != itemID || len(saved.FeedbackSources) != 1 || saved.FeedbackSources[0] != source {
			t.Fatal(saved, err)
		}
		var events int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM task_events WHERE type='feedback_work_item_dispatched' AND task_id=$1 AND execution_id=$2", task.ID, execution).Scan(&events); err != nil || events != 1 {
			t.Fatal(events, err)
		}
		target.Issue.ID++
		target.Issue.Number++
		if _, err := s.ImportIssue(ctx, p.ID, target, source); !errors.Is(err, ErrConflict) {
			t.Fatal("reassigned feedback", err)
		}
		var tasks int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM work_items").Scan(&tasks); err != nil || tasks != 2 {
			t.Fatal("unrelated task created", tasks, err)
		}
	})
}

func TestFeedbackContextValidation(t *testing.T) {
	f := PreviewFeedback{Request: "1be99db3-344d-48a5-865f-9b5886ca91d7", Execution: 10, Commit: strings.Repeat("c", 40),
		Path: "/public/queue%20demo/", Browser: "Chrome/143", Width: 1280, Height: 900, Element: "main:nth-of-type(1) > output:nth-of-type(2)",
		Note: "The count needs an accessible name.", Revision: 2, Case: "TC-002"}
	if !f.Valid() {
		t.Fatal("rejected valid feedback")
	}
	canvas := f
	canvas.Frames = []string{"html:nth-of-type(1) > iframe:nth-of-type(2)", "iframe:nth-of-type(1)"}
	canvas.Element = "main:nth-of-type(1) > canvas:nth-of-type(2)"
	for _, point := range []string{"0.00% 100.00%", "25.00% 75.00%"} {
		canvas.CanvasPoint = point
		if !canvas.Valid() {
			t.Errorf("rejected canvas point %q", point)
		}
	}
	for _, point := range []string{"25.00%", "100.01% 50.00%", "-1.00% 0.00%", "private-data"} {
		canvas.CanvasPoint = point
		if canvas.Valid() {
			t.Errorf("accepted canvas point %q", point)
		}
	}
	for _, p := range []string{"/?session=x", "/page%23secret", "//other.example/path", "/../other", "/%2e", "/a%5cb", "not/a/path"} {
		bad := f
		bad.Path = p
		if bad.Valid() {
			t.Errorf("accepted %q", p)
		}
	}
	for _, bad := range []func(*PreviewFeedback){
		func(f *PreviewFeedback) { f.Request = "no-key" }, func(f *PreviewFeedback) { f.Height = 0 },
		func(f *PreviewFeedback) { f.Element = "input[value=private]" }, func(f *PreviewFeedback) { f.Revision = 0 },
		func(f *PreviewFeedback) { f.Browser = "raw user agent string" },
		func(f *PreviewFeedback) { f.CanvasPoint = "50.00% 50.00%" },
		func(f *PreviewFeedback) { f.Frames = []string{"input:nth-of-type(1)"} },
		func(f *PreviewFeedback) { f.Frames = []string{"iframe[src=private]"} },
		func(f *PreviewFeedback) { f.Frames = []string{"iframe:nth-of-type(1)"}; f.Element = "" },
	} {
		test := f
		bad(&test)
		if test.Valid() {
			t.Fatal("accepted invalid feedback context")
		}
	}
}

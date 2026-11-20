package store

import (
	"context"
	"errors"
	"testing"
)

func TestExternalNoticeOwnership(t *testing.T) {
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
	task, err := s.CreateTask(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.Claim(ctx, "notice-node", t.TempDir())
	if err != nil || job == nil {
		t.Fatal(err)
	}
	if err := s.Complete(ctx, job.ExecutionID, job.LeaseToken, Result{ExitCode: 1, Error: "test failure"}); err != nil {
		t.Fatal(err)
	}
	type claim struct {
		n       Notification
		attempt int64
		err     error
	}
	claims := make(chan claim, 2)
	for range 2 {
		go func() { n, a, e := s.ClaimExternalNotice(ctx, p.ID, "telegram"); claims <- claim{n, a, e} }()
	}
	var won claim
	for range 2 {
		c := <-claims
		if c.err != nil {
			t.Fatal(c.err)
		}
		if c.attempt > 0 {
			if won.attempt != 0 {
				t.Fatal("concurrent senders both claimed")
			}
			won = c
		}
	}
	if won.attempt != 1 || won.n.TaskID != task.ID {
		t.Fatal(won)
	}
	if _, err := db.Exec("UPDATE external_notification_deliveries SET next_attempt=now() WHERE event_id=$1", won.n.ID); err != nil {
		t.Fatal(err)
	}
	n, attempt, err := s.ClaimExternalNotice(ctx, p.ID, "telegram")
	if err != nil || attempt != 2 || n.ID != won.n.ID {
		t.Fatal(n, attempt, err)
	}
	if err := s.FinishExternalNotice(ctx, n.ID, "telegram", 1, true, 15); !errors.Is(err, ErrConflict) {
		t.Fatal("stale sender settled", err)
	}
	if err := s.ReadNotification(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishExternalNotice(ctx, n.ID, "telegram", 2, false, 0); err != nil {
		t.Fatal(err)
	}
	_, a, err := s.ClaimExternalNotice(ctx, p.ID, "telegram")
	if err != nil || a != 0 {
		t.Fatal("read notice was retried", a, err)
	}
}

func TestOwnerNotificationsPageAndReadReceipts(t *testing.T) {
	s := New(isolatedDatabase(t))
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Materialize enough real failures to cross the history cursor.
	for i := 0; i < 32; i++ {
		task, err := s.CreateTask(ctx, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		job, err := s.Claim(ctx, "notify-node", t.TempDir())
		if err != nil || job == nil || job.TaskID != task.ID {
			t.Fatal(job, err)
		}
		result := Result{ExitCode: 1, Error: "verification failed"}
		if i == 0 {
			result = Result{Commit: "0123456789abcdef0123456789abcdef01234567"}
		}
		for range 2 {
			if err = s.Complete(ctx, job.ExecutionID, job.LeaseToken, result); err != nil {
				t.Fatal(err)
			}
		}
	}
	first, err := s.Notifications(ctx, 0, 0, false)
	if err != nil || len(first.Items) != 30 || first.NextBefore != first.Items[29].ID {
		t.Fatal(first, err)
	}
	last, err := s.Notifications(ctx, 0, first.NextBefore, true)
	if err != nil || len(last.Items) != 1 || last.Items[0].ID >= first.NextBefore || last.NextBefore != 0 {
		t.Fatal(last, err)
	}
	chosen := first.Items[0]
	if !chosen.Pending || chosen.Kind != "failure" || chosen.ReadAt != nil {
		t.Fatal(chosen)
	}
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- s.ReadNotification(ctx, chosen.ID) }()
	}
	for range 2 {
		if err = <-errs; err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.Notifications(ctx, 0, 0, false)
	if err != nil || page.Items[0].ReadAt == nil || !page.Items[0].Pending {
		t.Fatal(page, err)
	}
	n, err := s.UnreadNotifications(ctx, 0)
	if err != nil || n != 30 {
		t.Fatal(n, err)
	}
	var claimedEvent int64
	if err = s.db.QueryRow("SELECT id FROM task_events WHERE type='execution_claimed' ORDER BY id LIMIT 1").Scan(&claimedEvent); err != nil {
		t.Fatal(err)
	}
	if err = s.ReadNotification(ctx, claimedEvent); !errors.Is(err, ErrNotFound) {
		t.Fatal("read a non-notification event", err)
	}
}

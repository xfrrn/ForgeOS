package store

import (
	"context"
	"errors"
	"testing"
)

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
	first, err := s.Notifications(ctx, 0, false)
	if err != nil || len(first.Items) != 30 || first.NextBefore != first.Items[29].ID {
		t.Fatal(first, err)
	}
	last, err := s.Notifications(ctx, first.NextBefore, true)
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
	page, err := s.Notifications(ctx, 0, false)
	if err != nil || page.Items[0].ReadAt == nil || !page.Items[0].Pending {
		t.Fatal(page, err)
	}
	n, err := s.UnreadNotifications(ctx)
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

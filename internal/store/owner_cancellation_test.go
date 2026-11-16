package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestOwnerCancelWaitsForSettlement(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	p, err := s.CreateProject(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, abandoned := range []bool{false, true} {
		task, err := s.CreateTask(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		job, err := s.Claim(ctx, "worker", root)
		if err != nil || job == nil || job.TaskID != task.ID {
			t.Fatal("claim", err)
		}
		completed := make(chan error, 2)
		for range 2 {
			go func() { completed <- s.CancelExecution(ctx, job.ExecutionID) }()
		}
		for range 2 {
			if err := <-completed; err != nil {
				t.Fatal(err)
			}
		}
		pending, err := s.Task(ctx, task.ID)
		if err != nil || pending.Status != "RUNNING" || pending.Execution.OwnerCancelRequestedAt == nil || pending.Events[len(pending.Events)-1].Type != "owner_cancel_requested" {
			t.Fatal("missing pending cancellation", err)
		}
		if err := s.RenewExecutionLease(ctx, job.ExecutionID, "wrong"); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
		if err := s.RenewExecutionLease(ctx, job.ExecutionID, job.LeaseToken); !errors.Is(err, ErrOwnerStop) {
			t.Fatal(err)
		}
		if abandoned {
			if err := s.Abandon(ctx, job.ExecutionID); !errors.Is(err, ErrConflict) {
				t.Fatal(err)
			}
			if _, err := s.db.ExecContext(ctx, "UPDATE executions SET lease_until=now()-interval '1 second' WHERE id=$1", job.ExecutionID); err != nil {
				t.Fatal(err)
			}
			if err := s.Abandon(ctx, job.ExecutionID); err != nil {
				t.Fatal(err)
			}
		} else {
			// A successful receipt racing the stop still belongs to the cancelled task.
			result := Result{Commit: strings.Repeat("a", 40)}
			for range 2 {
				if err := s.Complete(ctx, job.ExecutionID, job.LeaseToken, result); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := s.CancelExecution(ctx, job.ExecutionID); err != nil {
			t.Fatal(err)
		}
		final, err := s.Task(ctx, task.ID)
		if err != nil || final.Status != "CANCELLED" || len(final.Events) != 4 {
			t.Fatalf("settlement events %+v: %v", final.Events, err)
		}
	}
}

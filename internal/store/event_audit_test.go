package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestEventAuditDistinguishesCancellationOrigins(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
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
	job, err := s.Claim(ctx, "node-one", t.TempDir())
	if err != nil || job == nil {
		t.Fatal(err)
	}
	if err := s.CancelExecution(ctx, job.ExecutionID); err != nil {
		t.Fatal(err)
	}
	result := Result{ExitCode: 1, Error: "private diagnostic body"}
	if err := s.Complete(ctx, job.ExecutionID, job.LeaseToken, result); err != nil {
		t.Fatal(err)
	}
	row, err := s.Task(ctx, task.ID)
	if err != nil || row.Status != "CANCELLED" || len(row.Events) != 4 {
		t.Fatal(row.Status, err)
	}
	id := row.Events[3].ID
	check := func(source, actor string, reported bool) {
		t.Helper()
		raw, err := s.EventAudit(ctx, p.ID, task.ID, id)
		var got struct {
			Source, Actor string
			Payload       struct {
				ExitCode *int `json:"exit_code"`
			}
		}
		if err != nil || json.Unmarshal(raw, &got) != nil || got.Source != source || got.Actor != actor || (got.Payload.ExitCode != nil) != reported || strings.Contains(string(raw), result.Error) || strings.Contains(string(raw), job.LeaseToken) {
			t.Fatal(string(raw), err)
		}
	}
	check("worker", "node-one", true)
	// Paused cancellations and legacy rows may refer to an already settled attempt.
	for _, origin := range []string{"owner", ""} {
		if _, err := s.db.ExecContext(ctx, "UPDATE task_events SET origin=NULLIF($2,'') WHERE id=$1", id, origin); err != nil {
			t.Fatal(err)
		}
		check(origin, origin, false)
	}
	if _, err := s.EventAudit(ctx, p.ID+1, task.ID, id); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

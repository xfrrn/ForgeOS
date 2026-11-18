package main

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestSupervisedWriterFollowsOwnerUntilReceipt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	w := &worker{}
	var err error
	w.journal, err = w.openJournal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	_, err = w.journal.Exec(`INSERT INTO attempts(lease_hash,execution_id,task_id,workspace) VALUES(X'01',1,3,'work');
		INSERT INTO supervised_writers VALUES(X'01')`)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := w.supervisedWriters(ctx)
	if err != nil || !slices.Equal(ids, []int64{1}) {
		t.Fatal(ids, err)
	}
	unlock, err := w.lockAttempt(1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	if extra, err := w.lockAttempt(1); !errors.Is(err, errWriterBusy) {
		if extra != nil {
			extra()
		}
		t.Fatalf("second process did not observe the execution lock: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- w.followWriter(ctx, 1) }()
	select {
	case err := <-done:
		t.Fatalf("running writer released its slot: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := w.journal.Exec("UPDATE attempts SET buffered_at=CURRENT_TIMESTAMP"); err != nil {
		t.Fatal(err)
	}
	// A pending receipt and an interrupted acknowledgement must both retain the assignment.
	tx, err := w.journal.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("UPDATE attempts SET settled_at=CURRENT_TIMESTAMP"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if key, err := w.writerAssignment(ctx, 1); err != nil || key == nil {
		t.Fatal("assignment lost on rollback", err)
	}
	if _, err := w.journal.Exec("UPDATE attempts SET settled_at=CURRENT_TIMESTAMP"); err != nil {
		t.Fatal(err)
	}
	unlock()
	unlock = nil
	if err := <-done; err != nil || ctx.Err() != nil {
		t.Fatal(err, ctx.Err())
	}
	if ids, err := w.supervisedWriters(ctx); err != nil || len(ids) != 0 {
		t.Fatal(ids, err)
	}
}

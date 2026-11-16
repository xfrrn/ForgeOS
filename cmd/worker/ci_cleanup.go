package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func (w *worker) removeLocalCIFile(ctx context.Context, f store.LocalCIFile) error {
	if !validation.ValidRunID(f.Run) || !filepath.IsLocal(f.File) || filepath.Base(f.File) != f.File ||
		f.File == "run.json" || f.Execution < 1 || f.Revision < 1 || f.Task < 1 {
		return errors.New("invalid local CI removal receipt")
	}
	var ready bool
	err := w.journal.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM attempts a
		WHERE a.execution_id=? AND a.task_id=? AND a.outcome='confirmed'
		AND NOT EXISTS(SELECT 1 FROM ci_uploads u WHERE u.lease_hash=a.lease_hash AND u.file=?))`,
		f.Execution, f.Task, f.File).Scan(&ready)
	if err != nil {
		return err
	}
	if !ready {
		return errors.New("local confirmation or upload still pending")
	}
	dir := filepath.Join(w.workspaceRoot, f.Run)
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() {
		return errors.New("invalid local CI directory")
	}
	path := filepath.Join(dir, f.File)
	info, err = os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("local CI file is not regular")
	}
	raw, err := archivedFile(dir, "run.json", 16<<20)
	if err != nil {
		return err
	}
	var run validation.Run
	if err := readRun(bytes.NewReader(raw), &run); err != nil || !run.Valid() || run.Runner != w.id ||
		run.ID != f.Run || run.ExecutionID != f.Execution {
		return errors.New("local CI run does not match the cleanup receipt")
	}
	if digest, ok := run.Files()[f.File]; !ok || digest != f.Digest {
		return errors.New("cleanup file is missing or different in local receipt")
	}
	// Historical receipts may predate the upload queue. Preserve a verified central copy
	// (or its explicit retirement) before unlinking the local bytes.
	archive := fmt.Sprintf("/internal/tasks/%d/executions/%d/ci/%s/%s", f.Task, f.Execution, f.Run, f.File)
	content, err := w.ciUploadContent(f.Run, f.File, f.Digest)
	if err != nil {
		return err
	}
	if err := w.sendCIContent(ctx, archive, f.Run, f.File, content, nil); err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, current) || current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) {
		return errors.New("local CI file changed before removal")
	}
	return os.Remove(path)
}

func (w *worker) cleanupLocalCI(ctx context.Context, reconcile bool) error {
	var after int64
	var file string
	for ctx.Err() == nil {
		var page []store.LocalCIFile
		status, err := w.post(ctx, "/internal/workers/ci-cleanup", map[string]any{"worker": w.id, "after_execution": after, "after_file": file, "reconcile": reconcile}, &page)
		if err != nil {
			return err
		}
		if status == http.StatusNotFound {
			return nil // Older control planes have no local retention policy.
		}
		if status != http.StatusOK {
			return fmt.Errorf("local cleanup queue returned HTTP %d", status)
		}
		if len(page) == 0 {
			return nil
		}
		for _, f := range page {
			if f.Execution < after || (f.Execution == after && f.File <= file) {
				return errors.New("invalid local cleanup cursor")
			}
			after, file = f.Execution, f.File
			if err = w.removeLocalCIFile(ctx, f); err != nil {
				log.Printf("execution %d local CI cleanup pending: %v", f.Execution, err)
				continue
			}
			if f.Removed {
				continue
			}
			path := fmt.Sprintf("/internal/executions/%d/ci-cleanup", f.Execution)
			status, err = w.post(ctx, path, map[string]any{"worker": w.id, "file": f.File, "revision": f.Revision}, nil)
			if err != nil {
				return err
			}
			if status != http.StatusNoContent {
				return fmt.Errorf("local cleanup confirmation returned HTTP %d", status)
			}
		}
	}
	return ctx.Err()
}

func (w *worker) sweepLocalCI(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var nextReconciliation time.Time
	for ctx.Err() == nil {
		reconcile := !time.Now().Before(nextReconciliation)
		if err := w.cleanupLocalCI(ctx, reconcile); err != nil {
			if ctx.Err() == nil {
				log.Printf("local CI cleanup: %v", err)
			}
		} else if reconcile {
			nextReconciliation = time.Now().Add(24 * time.Hour)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"forgeos/internal/store"
)

func bufferRoleProcess(db interface {
	Exec(string, ...any) (sql.Result, error)
}, report store.ManagementProcessReport) error {
	if !report.Process.Valid() || report.Process.Exited == nil || len(report.Lease) != 36 {
		return errors.New("cannot buffer an unconfirmed role process exit")
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO role_process_receipts(instance,report) VALUES(?,?)", report.Process.Instance, raw)
	return err
}

func (w *worker) replayRoleProcesses(ctx context.Context, db *sql.DB, role string) error {
	for ctx.Err() == nil {
		var instance int64
		var raw json.RawMessage
		err := db.QueryRow("SELECT instance,report FROM role_process_receipts WHERE report IS NOT NULL ORDER BY instance LIMIT 1").Scan(&instance, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.api+"/internal/"+role+"/process", bytes.NewReader(raw))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		info, err := scanProcessArchive(db.QueryRowContext(ctx, processArchiveQuery+" WHERE p.instance=?", instance))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if info.Available && info.Expired == "" {
			manifest, err := json.Marshal(info.ProcessArchive)
			if err != nil {
				return err
			}
			// Older control planes accept the same exit body and ignore this optional receipt.
			req.Header.Set("ForgeOS-Role-Archive", string(manifest))
		}
		status := 0
		if reply, err := w.authenticatedRequest(req); err == nil {
			status = reply.StatusCode
			reply.Body.Close()
		}
		if status == 204 || status == 409 {
			_, err = db.Exec(`UPDATE role_process_receipts SET status=?,settled_at=CURRENT_TIMESTAMP,
 conflicting_report=CASE WHEN ?=409 THEN json_remove(CAST(report AS TEXT),'$.lease') END,report=NULL WHERE instance=?`, status, status, instance)
			if err != nil {
				return err
			}
			continue
		}
		if !temporary(status) {
			return fmt.Errorf("role process exit remains buffered (HTTP %d)", status)
		}
		wait(ctx)
	}
	return ctx.Err()
}

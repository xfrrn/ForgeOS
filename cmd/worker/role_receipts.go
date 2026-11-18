package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

func roleForReceipt(path string) string {
	switch path {
	case "/internal/management/result", "/internal/management/questions/result", "/internal/management/feedback/result", "/internal/management/repository-analysis/result", "/internal/management/test-plans/result", "/internal/management/curation/result":
		return "management"
	case "/internal/curator/result":
		return "curator"
	case "/internal/test-agent/result":
		return "test-agent"
	case "/internal/reviewer/result":
		return "reviewer"
	}
	return ""
}

func (w *worker) openRoleJournal(ctx context.Context, role string) (*sql.DB, string, error) {
	if role != "management" && role != "curator" && role != "test-agent" && role != "reviewer" {
		return nil, "", errors.New("invalid role journal")
	}
	node := w.id
	var err error
	if node == "" {
		node, err = os.Hostname()
		if err != nil {
			return nil, "", err
		}
	}
	recorder := *w
	recorder.id = role + "-" + node
	db, err := recorder.openJournal(ctx)
	return db, node, err
}

func (w *worker) roleReceipt(ctx context.Context, path string, body any) (status int, resultErr error) {
	role := roleForReceipt(path)
	if role == "" {
		return 0, errors.New("invalid role result endpoint")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	var identity struct{ Lease string }
	if len(raw) > 2<<20 || json.Unmarshal(raw, &identity) != nil || len(identity.Lease) != 36 {
		return 0, errors.New("invalid role result identity or size")
	}
	key := sha256.Sum256([]byte(identity.Lease))
	digest := sha256.Sum256(raw)
	// Keep a completed result even if shutdown has already cancelled its HTTP request.
	db, _, err := w.openRoleJournal(context.WithoutCancel(ctx), role)
	if err != nil {
		return 0, err
	}
	defer func() { resultErr = errors.Join(resultErr, db.Close()) }()
	_, err = db.Exec(`INSERT INTO role_receipts(lease_hash,path,digest,report) VALUES(?,?,?,?) ON CONFLICT(lease_hash,path) DO NOTHING`, key[:], path, digest[:], raw)
	if err != nil {
		return 0, err
	}
	var saved []byte
	var settled sql.NullInt64
	if err = db.QueryRow("SELECT digest,status FROM role_receipts WHERE lease_hash=? AND path=?", key[:], path).Scan(&saved, &settled); err != nil {
		return 0, err
	}
	if !bytes.Equal(saved, digest[:]) {
		return 0, errors.New("role receipt cannot replace the result for its lease")
	}
	if settled.Valid {
		return int(settled.Int64), nil
	}
	return w.sendRoleReceipt(ctx, db, key[:], path, raw)
}

func (w *worker) sendRoleReceipt(ctx context.Context, db *sql.DB, key []byte, path string, raw json.RawMessage) (int, error) {
	for ctx.Err() == nil {
		status, _ := w.post(ctx, path, raw, nil)
		if status == 204 || status == 409 {
			_, err := db.Exec(`UPDATE role_receipts SET status=?,settled_at=CURRENT_TIMESTAMP,
 conflicting_result=CASE WHEN ?=409 THEN json_remove(CAST(report AS TEXT),'$.lease') END,report=NULL
 WHERE lease_hash=? AND path=? AND report IS NOT NULL`, status, status, key, path)
			return status, err
		}
		if !temporary(status) {
			return status, fmt.Errorf("role result remains buffered (HTTP %d)", status)
		}
		wait(ctx)
	}
	return 0, ctx.Err()
}

func (w *worker) replayRoleReceipts(ctx context.Context, role string) (resultErr error) {
	db, _, err := w.openRoleJournal(ctx, role)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, db.Close()) }()
	for ctx.Err() == nil {
		var key, raw []byte
		var path string
		err = db.QueryRow("SELECT lease_hash,path,report FROM role_receipts WHERE report IS NOT NULL ORDER BY buffered_at,rowid LIMIT 1").Scan(&key, &path, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if roleForReceipt(path) != role {
			return errors.New("buffered receipt belongs to another role")
		}
		if _, err = w.sendRoleReceipt(ctx, db, key, path, raw); err != nil {
			return err
		}
	}
	return ctx.Err()
}

package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"

	"forgeos/internal/store"
)

func (w *worker) completedRoleTurn(target store.ManagementProcessReport, dest any) (session string, found bool, resultErr error) {
	if w.roleRunID == "" {
		return "", false, nil
	}
	db, _, err := w.openRoleJournal(context.Background(), w.roleForProcess(target))
	if err != nil {
		return "", false, err
	}
	defer func() { resultErr = errors.Join(resultErr, db.Close()) }()
	key := sha256.Sum256([]byte(target.Lease))
	var raw []byte
	err = db.QueryRow("SELECT session_id,result FROM role_turn_results WHERE lease_hash=? AND turn=? AND result IS NOT NULL AND EXISTS(SELECT 1 FROM role_assignment WHERE lease_hash=? AND run_id=?)", key[:], target.Turn, key[:], w.roleRunID).Scan(&session, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if err = json.Unmarshal(raw, dest); err != nil {
		return "", false, err
	}
	return session, true, nil
}

func (w *worker) bufferCompletedRoleTurn(tx *sql.Tx, target store.ManagementProcessReport, session string, dest any) error {
	if w.roleRunID == "" {
		return nil
	}
	p := target.Process
	if session == "" || session != p.Session || p.Exited == nil || p.ExitCode == nil || *p.ExitCode != 0 {
		return errors.New("completed role turn requires its successful native exit")
	}
	raw, err := json.Marshal(dest)
	if err != nil {
		return err
	}
	key := sha256.Sum256([]byte(target.Lease))
	result, err := tx.Exec("UPDATE role_turn_results SET session_id=?,result=?,completed_at=CURRENT_TIMESTAMP WHERE lease_hash=? AND turn=? AND result IS NULL AND EXISTS(SELECT 1 FROM role_assignment WHERE lease_hash=? AND run_id=?)", session, raw, key[:], target.Turn, key[:], w.roleRunID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("completed role turn does not own the saved assignment")
	}
	return nil
}

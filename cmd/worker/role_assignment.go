package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"time"

	"forgeos/internal/store"
)

func (w *worker) ensureRoleRunID() error {
	if w.roleRunID != "" {
		return nil
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	w.roleRunID = hex.EncodeToString(nonce[:])
	return nil
}

func roleAssignmentLease(raw []byte) ([]byte, error) {
	var job store.ManagementJob
	if err := json.Unmarshal(raw, &job); err != nil {
		return nil, err
	}
	lease := job.Lease
	if job.Curation != nil {
		lease = job.Curation.Lease
	}
	if job.TestPlan != nil {
		lease = job.TestPlan.Lease
	}
	if job.Feedback != nil {
		lease = job.Feedback.Lease
	}
	if job.RepositoryAnalysis != nil {
		lease = job.RepositoryAnalysis.Lease
	}
	if len(lease) != 36 {
		return nil, errors.New("invalid role assignment lease")
	}
	key := sha256.Sum256([]byte(lease))
	return key[:], nil
}

func (w *worker) resumeRoleAssignment(db *sql.DB, out any) (bool, error) {
	var owner string
	var raw, key []byte
	var started bool
	err := db.QueryRow("SELECT run_id,response,lease_hash,started FROM role_assignment WHERE id=1").Scan(&owner, &raw, &key, &started)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var settled bool
	if err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM role_receipts WHERE lease_hash=? AND status IN (204,409))", key).Scan(&settled); err != nil {
		return false, err
	}
	if owner == w.roleRunID || settled {
		tx, err := db.Begin()
		if err != nil {
			return false, err
		}
		defer tx.Rollback()
		if _, err = tx.Exec("DELETE FROM role_turn_results WHERE lease_hash=?", key); err != nil {
			return false, err
		}
		if _, err = tx.Exec("DELETE FROM role_assignment WHERE id=1"); err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	w.readRoleRecoveryCapability(raw)
	if started {
		var completed bool
		if err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM role_turn_results WHERE lease_hash=?) AND NOT EXISTS(SELECT 1 FROM role_turn_results WHERE lease_hash=? AND result IS NULL)", key, key).Scan(&completed); err != nil {
			return false, err
		}
		if !completed {
			if !w.roleNativeResume {
				return false, errors.New("saved role assignment has started a native turn; preserve its journal for session recovery")
			}
			if err = w.validateRoleNativeRecovery(db, key); err != nil {
				return false, err
			}
		}
	}
	if err = json.Unmarshal(raw, out); err != nil {
		return false, err
	}
	result, err := db.Exec("UPDATE role_assignment SET run_id=? WHERE id=1 AND run_id=?", w.roleRunID, owner)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (w *worker) markRoleStarted(db *sql.DB, key []byte, turn int) error {
	if w.roleRunID == "" {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec("UPDATE role_assignment SET started=1 WHERE id=1 AND lease_hash=? AND run_id=?", key, w.roleRunID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var active bool
		if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM role_assignment)").Scan(&active); err != nil {
			return err
		}
		if active {
			return errors.New("native turn does not own the saved role assignment")
		}
	} else if _, err = tx.Exec("INSERT INTO role_turn_results(lease_hash,turn) VALUES(?,?)", key, turn); err != nil {
		return err
	}
	return tx.Commit()
}

func (w *worker) roleStartHook(role, lease string, turn int) func() error {
	if w.roleRunID == "" {
		return nil
	}
	return func() (resultErr error) {
		db, _, err := w.openRoleJournal(context.Background(), role)
		if err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, db.Close()) }()
		key := sha256.Sum256([]byte(lease))
		return w.markRoleStarted(db, key[:], turn)
	}
}

func (w *worker) runIndependentRole(ctx context.Context, role string, run func(context.Context) error) (resultErr error) {
	days, err := archiveSetting("FORGEOS_ROLE_RECOVERY_DAYS", 30, 3650)
	if err != nil {
		return err
	}
	w.roleRecoveryRetention = time.Duration(days) * 24 * time.Hour
	db, _, err := w.openRoleJournal(ctx, role)
	if err != nil {
		return err
	}
	var seq int
	var name, path string
	err = db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path)
	err = errors.Join(err, db.Close())
	if err != nil {
		return err
	}
	path += ".lock"
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("role lock must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	lock, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, lock.Close()) }()
	if err = lockRoleFile(lock); err != nil {
		return errors.New("another Daemon owns this role node, or its journal cannot be locked")
	}
	if err = w.ensureRoleRunID(); err != nil {
		return err
	}
	return run(ctx)
}

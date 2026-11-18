package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func pruneRoleRecoveryCommand(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("prune-role-recovery", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	journal := flags.String("journal", "", "existing private role journal")
	days := flags.Int("days", 30, "retention days")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*journal) || *days < 1 || *days > 3650 {
		return errors.New("usage: worker prune-role-recovery --journal ABSOLUTE_SQLITE [--days 1..3650]")
	}
	db, err := openExistingJournal(*journal, true)
	if err != nil {
		return err
	}
	defer db.Close()
	n, err := pruneRoleRecovery(db, time.Now().Add(-time.Duration(*days)*24*time.Hour))
	return errors.Join(err, json.NewEncoder(output).Encode(map[string]any{"expired": n, "days": *days}))
}

func pruneRoleRecovery(db *sql.DB, before time.Time) (int, error) {
	dir, err := roleJournalDirectory(db)
	if err != nil {
		return 0, err
	}
	var present bool
	if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='role_native_turns')").Scan(&present); err != nil || !present {
		return 0, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS role_recovery_expirations (
 lease_hash BLOB NOT NULL, turn INTEGER NOT NULL, expired_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(lease_hash,turn))`); err != nil {
		return 0, err
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// Reserve the journal for writing before testing eligibility and removing its files.
	if _, err := tx.Exec("DELETE FROM role_recovery_expirations WHERE 0"); err != nil {
		return 0, err
	}
	rows, err := tx.Query(`SELECT n.lease_hash,n.turn,n.workspace,min(r.path) FROM role_native_turns n
 JOIN role_receipts r ON r.lease_hash=n.lease_hash
 JOIN harness_processes current ON current.instance=n.instance AND current.lease_hash=n.lease_hash
 WHERE NOT EXISTS(SELECT 1 FROM role_recovery_expirations e WHERE e.lease_hash=n.lease_hash AND e.turn=n.turn)
 AND NOT EXISTS(SELECT 1 FROM role_assignment a WHERE a.lease_hash=n.lease_hash)
 AND NOT EXISTS(SELECT 1 FROM role_turn_results t WHERE t.lease_hash=n.lease_hash)
 AND NOT EXISTS(SELECT 1 FROM role_native_turns other WHERE other.workspace=n.workspace AND other.rowid<>n.rowid)
 AND NOT EXISTS(SELECT 1 FROM role_receipts f WHERE f.lease_hash=n.lease_hash AND
  (COALESCE(f.status,0) NOT IN (204,409) OR f.report IS NOT NULL OR COALESCE(julianday(f.settled_at),1e99)>=julianday(?1)))
 AND NOT EXISTS(SELECT 1 FROM harness_processes p LEFT JOIN role_process_receipts q ON q.instance=p.instance
  WHERE p.lease_hash=n.lease_hash AND (COALESCE(julianday(p.exited_at),1e99)>=julianday(?1)
   OR (q.instance IS NOT NULL AND (q.report IS NOT NULL OR COALESCE(julianday(q.settled_at),1e99)>=julianday(?1)))))
 GROUP BY n.lease_hash,n.turn,n.workspace`, before.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	type expiredTurn struct {
		key             []byte
		turn            int
		workspace, path string
	}
	var turns []expiredTurn
	for rows.Next() {
		var item expiredTurn
		if err := rows.Scan(&item.key, &item.turn, &item.workspace, &item.path); err != nil {
			rows.Close()
			return 0, err
		}
		turns = append(turns, item)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return 0, err
	}
	var pending error
	count := 0
	for _, item := range turns {
		if err := removeRoleRecoveryFiles(dir, item.workspace, roleForReceipt(item.path)); err != nil {
			pending = errors.Join(pending, fmt.Errorf("turn %d (%s): %w", item.turn, filepath.Base(item.workspace), err))
			continue
		}
		if _, err := tx.Exec("UPDATE role_native_turns SET prompt=NULL,contract=NULL WHERE lease_hash=? AND turn=?", item.key, item.turn); err != nil {
			return 0, err
		}
		if _, err := tx.Exec("INSERT INTO role_recovery_expirations(lease_hash,turn) VALUES(?,?)", item.key, item.turn); err != nil {
			return 0, err
		}
		count++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, pending
}

func removeRoleRecoveryFiles(dir, workspace, role string) error {
	suffix, ok := strings.CutPrefix(filepath.Base(workspace), "role-"+role+"-")
	if _, err := strconv.ParseUint(suffix, 10, 32); err != nil || !ok || role == "" || filepath.Clean(workspace) != workspace || filepath.Dir(workspace) != dir {
		return errors.New("unrecognized role workspace in this journal directory")
	}
	info, err := os.Lstat(workspace)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() || checkRoleWorkspace(dir, workspace) != nil {
		return errors.New("role workspace is not a local directory")
	}
	files, err := os.ReadDir(workspace)
	if err != nil {
		return err
	}
	for _, f := range files {
		if (f.Name() != "schema.json" && f.Name() != "output.json") || !f.Type().IsRegular() {
			return errors.New("unexpected workspace entry; preserved for inspection")
		}
	}
	for _, f := range files {
		if err := os.Remove(filepath.Join(workspace, f.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Remove(workspace)
}

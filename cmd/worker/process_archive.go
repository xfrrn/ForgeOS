package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

func archiveSetting(name string, fallback, maximum int64) (int64, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 1 || n > maximum {
		return 0, fmt.Errorf("%s must be between 1 and %d", name, maximum)
	}
	return n, nil
}

func prepareProcessArchives(db *sql.DB) error {
	if _, err := archiveSetting("FORGEOS_PROCESS_LOG_MAX_MIB", 32, 1024); err != nil {
		return err
	}
	days, err := archiveSetting("FORGEOS_PROCESS_LOG_DAYS", 30, 3650)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS harness_log_archives (
 instance INTEGER PRIMARY KEY REFERENCES harness_processes(instance),
 bytes INTEGER NOT NULL DEFAULT 0, records INTEGER NOT NULL DEFAULT 0,
 dropped_records INTEGER NOT NULL DEFAULT 0, dropped_bytes INTEGER NOT NULL DEFAULT 0,
 limit_bytes INTEGER NOT NULL, digest TEXT NOT NULL DEFAULT '', expired_at TEXT
 );
 CREATE TABLE IF NOT EXISTS harness_log_entries (
 instance INTEGER NOT NULL REFERENCES harness_processes(instance), seq INTEGER NOT NULL, entry BLOB NOT NULL,
 PRIMARY KEY(instance,seq)
 );`)
	if err != nil {
		return err
	}
	_, err = pruneProcessArchives(db, time.Now().Add(-time.Duration(days)*24*time.Hour))
	return err
}

type processLogEntry struct {
	Sequence int64  `json:"sequence"`
	At       string `json:"at"`
	Stream   string `json:"stream"`
	Call     int64  `json:"call,omitempty"`
	Tool     string `json:"tool,omitempty"`
	Phase    string `json:"phase"`
	Text     string `json:"text,omitempty"`
}

func archiveDigest(previous string, entry []byte) string {
	hash := sha256.New()
	hash.Write([]byte(previous))
	hash.Write(entry)
	return hex.EncodeToString(hash.Sum(nil))
}

func (p *processLog) archiveOutput(tx *sql.Tx, stream string, call int64, tool, phase, safe string) error {
	limit, err := archiveSetting("FORGEOS_PROCESS_LOG_MAX_MIB", 32, 1024)
	if err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT OR IGNORE INTO harness_log_archives(instance,limit_bytes) VALUES(?,?)", p.id, limit<<20); err != nil {
		return err
	}
	var size, count, dropped int64
	var digest string
	var expired sql.NullString
	if err = tx.QueryRow("SELECT bytes,records,dropped_records,limit_bytes,digest,expired_at FROM harness_log_archives WHERE instance=?", p.id).Scan(&size, &count, &dropped, &limit, &digest, &expired); err != nil {
		return err
	}
	if expired.Valid {
		return errors.New("cannot append to an expired process archive")
	}
	raw, err := json.Marshal(processLogEntry{Sequence: count + 1, At: time.Now().UTC().Format(time.RFC3339Nano), Stream: stream, Call: call, Tool: tool, Phase: phase, Text: safe})
	if err != nil {
		return err
	}
	// Keep a contiguous prefix. A visible omission counter replaces unbounded storage.
	if dropped > 0 || size+int64(len(raw))+1 > limit {
		_, err = tx.Exec("UPDATE harness_log_archives SET dropped_records=dropped_records+1,dropped_bytes=dropped_bytes+? WHERE instance=?", len(raw)+1, p.id)
		return err
	}
	raw = append(raw, '\n')
	if _, err = tx.Exec("INSERT INTO harness_log_entries(instance,seq,entry) VALUES(?,?,?)", p.id, count+1, raw); err != nil {
		return err
	}
	_, err = tx.Exec("UPDATE harness_log_archives SET bytes=bytes+?,records=records+1,digest=? WHERE instance=?", len(raw), archiveDigest(digest, raw), p.id)
	return err
}

// Only confirmed business results may retire output; interrupted or pending work keeps its evidence.
func pruneProcessArchives(db *sql.DB, before time.Time) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE harness_log_archives SET expired_at=CURRENT_TIMESTAMP
 WHERE expired_at IS NULL AND instance IN (
 SELECT p.instance FROM harness_processes p WHERE p.exited_at IS NOT NULL
 AND julianday(p.exited_at)<julianday(?)
 AND NOT EXISTS(SELECT 1 FROM role_assignment r WHERE r.lease_hash=p.lease_hash)
 AND NOT EXISTS(SELECT 1 FROM parked_questions q WHERE q.lease_hash=p.lease_hash)
 AND NOT EXISTS(SELECT 1 FROM role_process_receipts q WHERE q.instance=p.instance AND q.settled_at IS NULL)
 AND (EXISTS(SELECT 1 FROM attempts a WHERE a.lease_hash=p.lease_hash AND a.settled_at IS NOT NULL AND a.report IS NULL AND julianday(a.settled_at)<julianday(?))
 OR EXISTS(SELECT 1 FROM role_receipts r WHERE r.lease_hash=p.lease_hash AND r.status IN (204,409) AND r.report IS NULL AND julianday(r.settled_at)<julianday(?)))
 )`, before.UTC().Format(time.RFC3339), before.UTC().Format(time.RFC3339), before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec("DELETE FROM harness_log_entries WHERE instance IN (SELECT instance FROM harness_log_archives WHERE expired_at IS NOT NULL)"); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

func (w *worker) sweepProcessLogs(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for ctx.Err() == nil {
		days, err := archiveSetting("FORGEOS_PROCESS_LOG_DAYS", 30, 3650)
		if err == nil {
			_, err = pruneProcessArchives(w.journal, time.Now().Add(-time.Duration(days)*24*time.Hour))
		}
		if err != nil {
			log.Printf("process log retention remains pending: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

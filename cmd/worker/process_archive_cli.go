package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type processArchiveInfo struct {
	Type           string `json:"type"`
	Instance       int64  `json:"instance"`
	Task           int64  `json:"task_id,omitempty"`
	Execution      int64  `json:"execution_id,omitempty"`
	Harness        string `json:"harness"`
	Version        string `json:"version"`
	PID            int    `json:"pid"`
	Session        string `json:"session_id,omitempty"`
	Started        string `json:"started_at"`
	Exited         string `json:"exited_at,omitempty"`
	ExitCode       *int   `json:"exit_code,omitempty"`
	Available      bool   `json:"available"`
	Bytes          int64  `json:"bytes"`
	Records        int64  `json:"records"`
	DroppedRecords int64  `json:"dropped_records"`
	DroppedBytes   int64  `json:"dropped_bytes"`
	LimitBytes     int64  `json:"limit_bytes"`
	Digest         string `json:"chain_sha256,omitempty"`
	Expired        string `json:"expired_at,omitempty"`
}

const processArchiveQuery = `SELECT p.instance,COALESCE(a.task_id,0),COALESCE(a.execution_id,0),p.harness,p.version,p.pid,
 COALESCE(p.session_id,''),p.started_at,COALESCE(p.exited_at,''),p.exit_code,
 l.instance IS NOT NULL,COALESCE(l.bytes,0),COALESCE(l.records,0),COALESCE(l.dropped_records,0),
 COALESCE(l.dropped_bytes,0),COALESCE(l.limit_bytes,0),COALESCE(l.digest,''),COALESCE(l.expired_at,'')
 FROM harness_processes p LEFT JOIN attempts a ON a.lease_hash=p.lease_hash
 LEFT JOIN harness_log_archives l ON l.instance=p.instance`

func scanProcessArchive(row interface{ Scan(...any) error }) (processArchiveInfo, error) {
	v := processArchiveInfo{Type: "process"}
	err := row.Scan(&v.Instance, &v.Task, &v.Execution, &v.Harness, &v.Version, &v.PID, &v.Session, &v.Started, &v.Exited, &v.ExitCode,
		&v.Available, &v.Bytes, &v.Records, &v.DroppedRecords, &v.DroppedBytes, &v.LimitBytes, &v.Digest, &v.Expired)
	return v, err
}

func exportProcessArchive(db *sql.DB, instance int64, output io.Writer) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	info, err := scanProcessArchive(tx.QueryRow(processArchiveQuery+" WHERE p.instance=?", instance))
	if err != nil {
		return err
	}
	if !info.Available {
		return errors.New("this process has no recorded archive; older output cannot be reconstructed")
	}
	if info.Expired != "" {
		return fmt.Errorf("process archive expired at %s; metadata remains available", info.Expired)
	}
	rows, err := tx.Query("SELECT seq,entry FROM harness_log_entries WHERE instance=? ORDER BY seq", instance)
	if err != nil {
		return err
	}
	defer rows.Close()
	var count, size int64
	var digest string
	for rows.Next() {
		var seq int64
		var raw []byte
		if err = rows.Scan(&seq, &raw); err != nil {
			return err
		}
		count++
		size += int64(len(raw))
		var entry processLogEntry
		if seq != count || json.Unmarshal(raw, &entry) != nil || entry.Sequence != seq {
			return errors.New("process archive sequence is inconsistent")
		}
		digest = archiveDigest(digest, raw)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if size != info.Bytes || count != info.Records || digest != info.Digest {
		return errors.New("process archive digest is inconsistent")
	}
	// Stream the verified snapshot without buffering an entire archive in memory.
	entries, err := tx.Query("SELECT entry FROM harness_log_entries WHERE instance=? ORDER BY seq", instance)
	if err != nil {
		return err
	}
	defer entries.Close()
	enc := json.NewEncoder(output)
	if err = enc.Encode(info); err != nil {
		return err
	}
	for entries.Next() {
		var raw []byte
		if err = entries.Scan(&raw); err != nil {
			return err
		}
		if _, err = output.Write(raw); err != nil {
			return err
		}
	}
	if err = entries.Err(); err != nil {
		return err
	}
	if err = enc.Encode(map[string]any{"type": "verification", "verified": true, "chain_sha256": digest, "records": count, "bytes": size, "truncated": info.DroppedRecords > 0}); err != nil {
		return err
	}
	return tx.Commit()
}

func processLogsCommand(args []string, output io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: worker process-logs list|export|prune --journal ABSOLUTE_SQLITE [--instance N] [--days N]")
	}
	action := args[0]
	if action != "list" && action != "export" && action != "prune" {
		return errors.New("unknown process-logs action")
	}
	flags := flag.NewFlagSet("process-logs "+action, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("journal", "", "existing private Worker SQLite")
	instance := flags.Int64("instance", 0, "process instance for export")
	days := flags.Int("days", 30, "retention days for prune")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*path) || (*instance < 1 && action == "export") || (*instance != 0 && action != "export") || *days < 1 || *days > 3650 {
		return errors.New("invalid process-logs arguments")
	}
	db, err := openExistingJournal(*path, action == "prune")
	if err != nil {
		return err
	}
	defer db.Close()
	if action == "export" {
		return exportProcessArchive(db, *instance, output)
	}
	if action == "prune" {
		n, err := pruneProcessArchives(db, time.Now().Add(-time.Duration(*days)*24*time.Hour))
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(map[string]any{"expired": n, "days": *days})
	}
	rows, err := db.Query(processArchiveQuery + " ORDER BY p.instance")
	if err != nil {
		return err
	}
	defer rows.Close()
	enc := json.NewEncoder(output)
	for rows.Next() {
		info, err := scanProcessArchive(rows)
		if err != nil {
			return err
		}
		if err = enc.Encode(info); err != nil {
			return err
		}
	}
	return rows.Err()
}

func openExistingJournal(path string, write bool) (*sql.DB, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("journal must be an existing regular file")
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" && (info.Mode().Perm()&0077 != 0 || dir.Mode().Perm()&0077 != 0) {
		return nil, errors.New("journal and directory must be private to the service account")
	}
	mode := "ro"
	if write {
		mode = "rw"
	}
	uri := url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(path), "/"), RawQuery: "mode=" + mode + "&_pragma=busy_timeout(5000)&_pragma=secure_delete(ON)&_pragma=synchronous(FULL)"}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

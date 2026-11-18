package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"forgeos/internal/store"
)

type roleNativeTurn struct {
	Turn                           int
	Workspace, Resume, HandoffFrom string
	Instance                       int64
}

func (w *worker) readRoleRecoveryCapability(raw []byte) {
	var capability struct {
		Recover bool `json:"recover_native"`
	}
	w.roleNativeResume = json.Unmarshal(raw, &capability) == nil && capability.Recover
}

func (w *worker) validateRoleNativeRecovery(db *sql.DB, key []byte) error {
	dir, err := roleJournalDirectory(db)
	if err != nil {
		return err
	}
	rows, err := db.Query("SELECT COALESCE(n.instance,0),COALESCE(p.pid,0),COALESCE(NULLIF(p.session_id,''),h.missing_session,''),p.exited_at,COALESCE(n.harness,''),COALESCE(n.version,''),COALESCE(n.workspace,'') FROM role_turn_results r LEFT JOIN role_native_turns n ON n.lease_hash=r.lease_hash AND n.turn=r.turn LEFT JOIN harness_processes p ON p.instance=n.instance LEFT JOIN role_native_handoffs h ON h.lease_hash=n.lease_hash AND h.turn=n.turn AND h.missing_instance=n.instance WHERE r.lease_hash=? AND r.result IS NULL", key)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var instance int64
		var pid int
		var session, harness, version, workspace string
		var exited sql.NullString
		if err = rows.Scan(&instance, &pid, &session, &exited, &harness, &version, &workspace); err != nil {
			return err
		}
		if instance == 0 || pid == 0 || !nativeSessionID.MatchString(session) {
			return errors.New("native role turn lacks a recorded process and session; preserve its journal")
		}
		if !exited.Valid {
			alive, err := roleProcessAlive(pid)
			if err != nil {
				return err
			}
			if alive {
				return errors.New("original native role process is still running; preserve its journal and retry after it exits")
			}
		}
		if err = checkRoleWorkspace(dir, workspace); err != nil {
			return err
		}
		current, err := w.roleHarnessVersion(harness)
		if err != nil {
			return err
		}
		if current != version {
			return errors.New("native role Harness version changed since the saved turn")
		}
		count++
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if count == 0 {
		return errors.New("native role turn has no recoverable startup record")
	}
	return nil
}

func roleJournalDirectory(db *sql.DB) (string, error) {
	var seq int
	var name, path string
	err := db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path)
	return filepath.Dir(path), err
}

func (w *worker) prepareRoleNative(db *sql.DB, target store.ManagementProcessReport, harness, version string) (*roleNativeTurn, error) {
	key := sha256.Sum256([]byte(target.Lease))
	native := &roleNativeTurn{Turn: target.Turn}
	var recordedHarness, recordedVersion string
	var instance sql.NullInt64
	err := db.QueryRow("SELECT harness,version,workspace,instance FROM role_native_turns WHERE lease_hash=? AND turn=?", key[:], target.Turn).Scan(&recordedHarness, &recordedVersion, &native.Workspace, &instance)
	dir, e := roleJournalDirectory(db)
	if e != nil {
		return nil, e
	}
	if errors.Is(err, sql.ErrNoRows) {
		native.Workspace, err = os.MkdirTemp(dir, "role-"+roleForProcess(target)+"-")
		if err != nil {
			return nil, err
		}
		_, err = db.Exec("INSERT INTO role_native_turns(lease_hash,turn,harness,version,workspace) VALUES(?,?,?,?,?)", key[:], target.Turn, harness, version, native.Workspace)
		if err != nil {
			os.Remove(native.Workspace)
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if recordedHarness != harness || recordedVersion != version {
		return nil, errors.New("native role Harness version changed since the saved turn")
	}
	if err = checkRoleWorkspace(dir, native.Workspace); err != nil {
		return nil, err
	}
	if instance.Valid {
		p, err := scanHarnessProcess(db.QueryRow(readHarnessProcess+" WHERE instance=?", instance.Int64))
		if err != nil {
			return nil, err
		}
		var missing string
		err = db.QueryRow("SELECT missing_session FROM role_native_handoffs WHERE lease_hash=? AND turn=? AND missing_instance=?", key[:], target.Turn, instance.Int64).Scan(&missing)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if missing != "" {
			native.HandoffFrom = missing
		} else if !nativeSessionID.MatchString(p.Session) {
			return nil, errors.New("native role turn has no saved session")
		}
		if p.Exited == nil {
			alive, err := roleProcessAlive(p.PID)
			if err != nil {
				return nil, err
			}
			if alive {
				return nil, errors.New("original native role process is still running")
			}
			// The prior Daemon could not wait for this process; -1 records an unknown exit code.
			if _, err = db.Exec("UPDATE harness_processes SET exited_at=CURRENT_TIMESTAMP,exit_code=-1 WHERE instance=? AND exited_at IS NULL", instance.Int64); err != nil {
				return nil, err
			}
			p, err = scanHarnessProcess(db.QueryRow(readHarnessProcess+" WHERE instance=?", instance.Int64))
			if err != nil {
				return nil, err
			}
		}
		var buffered bool
		if err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM role_process_receipts WHERE instance=?)", instance.Int64).Scan(&buffered); err != nil {
			return nil, err
		}
		if !buffered {
			if err = db.QueryRow("SELECT resumed_from FROM role_native_attempts WHERE instance=?", instance.Int64).Scan(&target.ResumedFrom); err != nil {
				return nil, err
			}
			target.MissingSession = missing != ""
			target.Process = p
			target.Node = w.id
			if target.Node == "" {
				target.Node, err = os.Hostname()
				if err != nil {
					return nil, err
				}
			}
			if err = bufferRoleProcess(db, target); err != nil {
				return nil, err
			}
		}
		native.Instance = instance.Int64
		if native.HandoffFrom == "" {
			native.Resume = p.Session
		}
	}
	return native, nil
}

func (w *worker) markRoleResuming(db *sql.DB, key []byte, native *roleNativeTurn) error {
	result, err := db.Exec("UPDATE role_native_turns SET instance=NULL WHERE lease_hash=? AND turn=? AND instance=? AND EXISTS(SELECT 1 FROM role_assignment WHERE lease_hash=? AND run_id=?) AND EXISTS(SELECT 1 FROM role_turn_results WHERE lease_hash=? AND turn=? AND result IS NULL)", key, native.Turn, native.Instance, key, w.roleRunID, key, native.Turn)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("native role resume lost its saved assignment")
	}
	return nil
}

func (p processLog) freezeRoleInput(prompt, contract string) (string, string, error) {
	n := p.roleNative
	if _, err := p.db.Exec("UPDATE role_native_turns SET prompt=?,contract=? WHERE lease_hash=? AND turn=? AND prompt IS NULL", prompt, contract, p.lease[:], n.Turn); err != nil {
		return "", "", err
	}
	if err := p.db.QueryRow("SELECT prompt,contract FROM role_native_turns WHERE lease_hash=? AND turn=?", p.lease[:], n.Turn).Scan(&prompt, &contract); err != nil {
		return "", "", err
	}
	for _, name := range []string{"schema.json", "output.json"} {
		path := filepath.Join(n.Workspace, name)
		if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
			return "", "", fmt.Errorf("native role %s is not a regular file", name)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", "", err
		}
	}
	if err := os.Remove(filepath.Join(n.Workspace, "output.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	return prompt, contract, nil
}

func checkRoleWorkspace(dir, workspace string) error {
	actual, err := filepath.EvalSymlinks(workspace)
	if err != nil || actual != workspace || filepath.Dir(actual) != dir {
		return errors.New("native role workspace is missing or outside its private journal")
	}
	return nil
}

func (w *worker) roleHarnessVersion(harness string) (string, error) {
	if harness == "codex" {
		return readCodexVersion(w.codex)
	}
	name := os.Getenv("FORGEOS_CLAUDE_BIN")
	if name == "" {
		name = "claude"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, "--version")
	cmd.Env = harnessEnv()
	raw, err := cmd.Output()
	version := claudeVersion.FindStringSubmatch(strings.TrimSpace(string(raw)))
	if err != nil || len(version) != 2 {
		return "", errors.New("cannot identify the Claude Code reviewer version")
	}
	return version[1], nil
}

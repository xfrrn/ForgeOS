package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"
)

func (w *worker) claimRole(ctx context.Context, role, path string, body, out any) (status int, resultErr error) {
	db, node, err := w.openRoleJournal(ctx, role)
	if err != nil {
		return 0, err
	}
	defer func() { resultErr = errors.Join(resultErr, db.Close()) }()
	if now := time.Now(); w.roleRecoveryRetention > 0 && !now.Before(w.nextRoleCleanup) {
		if _, err := pruneRoleRecovery(db, now.Add(-w.roleRecoveryRetention)); err != nil {
			log.Printf("role recovery retention remains pending: %v", err)
		}
		w.nextRoleCleanup = now.Add(time.Hour)
	}
	if err = w.ensureRoleRunID(); err != nil {
		return 0, err
	}
	if resumed, e := w.resumeRoleAssignment(db, out); e != nil {
		return 409, e
	} else if resumed {
		return 200, nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return 0, err
	}
	key := hex.EncodeToString(nonce[:])
	if _, err = db.Exec("INSERT INTO role_claim_request(id,request_key,path,body) VALUES(1,?,?,?) ON CONFLICT(id) DO NOTHING", key, path, raw); err != nil {
		return 0, err
	}
	if err = db.QueryRow("SELECT request_key,path,body FROM role_claim_request WHERE id=1").Scan(&key, &path, &raw); err != nil {
		return 0, err
	}
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() || u.Host != "" || u.Path != "/internal/"+role+"/claim" || u.Fragment != "" {
		return 0, errors.New("invalid buffered role claim endpoint")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.api+path, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ForgeOS-Claim-Key", key)
	req.Header.Set("X-ForgeOS-Node", node)
	resp, err := w.authenticatedRequest(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if echo := resp.Header.Get("X-ForgeOS-Claim-Key"); echo != "" && echo != key {
		return resp.StatusCode, errors.New("role claim response key mismatch")
	}
	status, response, err := decodeJobResponse(resp, out)
	if err != nil || (status != 200 && status != 204) {
		return status, err
	}
	// Only one Daemon sharing this node journal may consume the received assignment.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.Exec("DELETE FROM role_claim_request WHERE id=1 AND request_key=?", key)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 204, nil
	}
	if status == 200 {
		w.readRoleRecoveryCapability(response)
		lease, err := roleAssignmentLease(response)
		if err != nil {
			return 0, err
		}
		if _, err = tx.Exec("INSERT INTO role_assignment(id,request_key,lease_hash,run_id,response) VALUES(1,?,?,?,?)", key, lease, w.roleRunID, response); err != nil {
			return 0, err
		}
	}
	return status, tx.Commit()
}

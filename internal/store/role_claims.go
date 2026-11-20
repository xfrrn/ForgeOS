package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type roleClaimTransactionKey struct{}

// Individual claim methods share the outer receipt transaction only on an idempotent role request.
type roleClaimTransaction struct {
	*sql.Tx
	shared bool
	closed bool
}

func (t *roleClaimTransaction) Commit() error {
	if t.shared {
		_, err := t.Exec("RELEASE SAVEPOINT role_candidate")
		t.closed = err == nil
		return err
	}
	return t.Tx.Commit()
}

func (t *roleClaimTransaction) Rollback() error {
	if t.shared {
		if t.closed {
			return nil
		}
		_, err := t.Exec("ROLLBACK TO SAVEPOINT role_candidate; RELEASE SAVEPOINT role_candidate")
		t.closed = err == nil
		return err
	}
	return t.Tx.Rollback()
}

func (s *Store) beginRoleClaim(ctx context.Context) (*roleClaimTransaction, error) {
	if tx, ok := ctx.Value(roleClaimTransactionKey{}).(*sql.Tx); ok {
		if _, err := tx.ExecContext(ctx, "SAVEPOINT role_candidate"); err != nil {
			return nil, err
		}
		return &roleClaimTransaction{Tx: tx, shared: true}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	return &roleClaimTransaction{Tx: tx}, err
}

func (s *Store) RoleClaim(ctx context.Context, project int64, role, node, key string, request []byte, claim func(context.Context) (int, []byte, error)) (int, []byte, error) {
	nonce, err := hex.DecodeString(key)
	if err != nil || len(nonce) != 16 || strings.ToLower(key) != key || project < 1 || node == "" || len(node) > 128 ||
		strings.Trim(node, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.-") != "" ||
		(role != "curator" && role != "management" && role != "management-chat" && role != "test-agent" && role != "reviewer") {
		return 0, nil, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()
	identity := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s", project, role, node, key)))
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", int64(binary.BigEndian.Uint64(identity[:8]))); err != nil {
		return 0, nil, err
	}
	digest := sha256.Sum256(request)
	var saved, body []byte
	err = tx.QueryRowContext(ctx, "SELECT request_digest,response FROM role_claim_receipts WHERE project_id=$1 AND role=$2 AND node=$3 AND request_key=$4", project, role, node, key).Scan(&saved, &body)
	if err == nil {
		if !bytes.Equal(saved, digest[:]) {
			return 0, nil, ErrConflict
		}
		return 200, body, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, nil, err
	}
	status, body, err := claim(context.WithValue(ctx, roleClaimTransactionKey{}, tx))
	if err != nil || (status != 200 && status != 204) {
		return status, body, err
	}
	if status == 200 {
		if len(body) > 32<<20 || !json.Valid(body) {
			return 0, nil, errors.New("invalid role claim response")
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO role_claim_receipts(project_id,role,node,request_key,request_digest,response) VALUES($1,$2,$3,$4,$5,$6)", project, role, node, key, digest[:], body)
		if err != nil {
			return 0, nil, err
		}
	}
	return status, body, tx.Commit()
}

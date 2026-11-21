package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const SharedKnowledgeKind = "shared_experience"

func sharedKnowledgeRef(ref string) (int64, string, error) {
	raw, digest, ok := strings.Cut(ref, ":")
	id, err := strconv.ParseInt(raw, 10, 64)
	if !ok || err != nil || id < 1 || raw != strconv.FormatInt(id, 10) || len(digest) != 64 {
		return 0, "", ErrNotFound
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return 0, "", ErrNotFound
	}
	return id, digest, nil
}

func (s *Store) ReceivedKnowledge(ctx context.Context, recipient int64, ref string) (KnowledgeEvidence, error) {
	var e KnowledgeEvidence
	id, digest, err := sharedKnowledgeRef(ref)
	if err != nil {
		return e, err
	}
	var series, version int64
	err = s.db.QueryRowContext(ctx, `SELECT series_id,share_version,title,content
		FROM visible_knowledge_shares WHERE recipient=$1 AND id=$2 AND digest=$3`, recipient, id, digest).
		Scan(&series, &version, &e.Title, &e.Content)
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	if err != nil {
		return e, err
	}
	e.Kind, e.Ref, e.Version = SharedKnowledgeKind, ref, digest
	e.Source = fmt.Sprintf("收到的共享经验 #%d · 版本 %d", series, version)
	return e, nil
}

// Serialize the last visibility check for a read or citation with publication
// and revocation. Recheck in a fresh READ COMMITTED snapshot after acquiring the lock.
func lockReceivedKnowledge(ctx context.Context, tx *sql.Tx, recipient int64, ref string) error {
	id, digest, err := sharedKnowledgeRef(ref)
	if err != nil {
		return err
	}
	var current int64
	err = tx.QueryRowContext(ctx, `SELECT series.id FROM knowledge_share_series series
		JOIN knowledge_shares k ON k.series_id=series.id
		WHERE series.recipient=$1 AND k.id=$2 AND k.digest=$3 FOR SHARE OF series`,
		recipient, id, digest).Scan(&current)
	if err == nil {
		err = tx.QueryRowContext(ctx, `SELECT id FROM visible_knowledge_shares
			WHERE recipient=$1 AND id=$2 AND digest=$3`, recipient, id, digest).Scan(&current)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

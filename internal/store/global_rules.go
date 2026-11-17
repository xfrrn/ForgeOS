package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strings"
)

func (s *Store) GlobalRules(ctx context.Context, revision int) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT to_jsonb(r) FROM global_rule_revisions r WHERE $1::int=0 OR revision=$1 ORDER BY revision DESC LIMIT 1`, revision).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		if revision == 0 {
			return json.RawMessage("null"), nil
		}
		return nil, ErrNotFound
	}
	return doc, err
}

func (s *Store) PublishGlobalRules(ctx context.Context, previous int, content string) (json.RawMessage, error) {
	if previous < 0 || previous >= math.MaxInt32 || len(content) > 16<<10 || strings.ContainsRune(content, 0) ||
		(strings.TrimSpace(content) == "" && (content != "" || previous == 0)) {
		return nil, ErrConflict
	}
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `INSERT INTO global_rule_revisions AS r(revision,content,content_sha256)
		SELECT $1::int+1,$2,encode(sha256(convert_to($2,'UTF8')),'hex')
		WHERE $1=0 OR EXISTS(SELECT 1 FROM global_rule_revisions WHERE revision=$1)
		ON CONFLICT(revision) DO UPDATE SET content=EXCLUDED.content WHERE r.content=EXCLUDED.content
		RETURNING to_jsonb(r)`, previous, content).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	return doc, err
}

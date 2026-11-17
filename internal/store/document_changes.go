package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"forgeos/internal/localrepo"
)

// ObserveDocumentHead serializes scans for one project. Only committed Git blobs enter the transaction.
func (s *Store) ObserveDocumentHead(ctx context.Context, project int64, repo string) error {
	for {
		more, err := s.observeDocumentBatch(ctx, project, repo)
		if err != nil || !more {
			return err
		}
	}
}

func (s *Store) observeDocumentBatch(ctx context.Context, project int64, repo string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO repository_document_heads(project_id) VALUES ($1) ON CONFLICT DO NOTHING`, project); err != nil {
		return false, err
	}
	var previous string
	if err = tx.QueryRowContext(ctx, `SELECT commit_sha FROM repository_document_heads WHERE project_id=$1 FOR UPDATE`, project).Scan(&previous); err != nil {
		return false, err
	}
	raw, err := localrepo.Git(ctx, repo, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return false, errors.New("repository HEAD is unavailable")
	}
	head := strings.TrimSpace(string(raw))
	if previous == head {
		return false, tx.Commit()
	}
	commits, err := localrepo.DocumentCommits(ctx, repo, previous, head)
	if err != nil {
		return false, err
	}
	var saved []byte
	if err = tx.QueryRowContext(ctx, `SELECT files FROM repository_document_heads WHERE project_id=$1`, project).Scan(&saved); err != nil {
		return false, err
	}
	var old map[string]string
	if err = json.Unmarshal(saved, &old); err != nil {
		return false, err
	}
	for _, commit := range commits {
		files, err := localrepo.DocumentTree(ctx, repo, commit)
		if err != nil {
			return false, err
		}
		if previous != "" {
			if err := recordDocumentDelta(ctx, tx, repo, project, previous, commit, old, files); err != nil {
				return false, err
			}
		}
		previous, old = commit, files
	}
	next, err := json.Marshal(old)
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE repository_document_heads SET commit_sha=$2,files=$3 WHERE project_id=$1`, project, previous, string(next)); err != nil {
		return false, err
	}
	return previous != head, tx.Commit()
}

func recordDocumentDelta(ctx context.Context, tx *sql.Tx, repo string, project int64, previous, head string, old, files map[string]string) error {
	var changed []string
	for path, blob := range files {
		if old[path] != blob {
			changed = append(changed, path)
		}
	}
	for path := range old {
		if files[path] == "" {
			changed = append(changed, path)
		}
	}
	slices.Sort(changed)
	for _, path := range changed {
		blob := files[path]
		content, problem := "", "deleted"
		var err error
		if blob != "" {
			content, problem, err = localrepo.DocumentContent(ctx, repo, blob)
			if err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO repository_document_changes(project_id,previous_commit,commit_sha,path,previous_blob,blob,content,problem)
				VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8)`, project, previous, head, path, old[path], blob, content, problem); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DocumentChanges(ctx context.Context, project, before int64) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT jsonb_build_object('head',h.commit_sha,'items',COALESCE((
		SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id DESC) FROM (
			SELECT id,path,commit_sha,previous_commit,blob,previous_blob,problem,observed_at
			FROM repository_document_changes WHERE project_id=p.id AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT 30
		) r),'[]')) FROM projects p LEFT JOIN repository_document_heads h ON h.project_id=p.id WHERE p.id=$1`, project, before).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

func (s *Store) DocumentCuration(ctx context.Context, project, id int64) (json.RawMessage, error) {
	return readCurationReceipt(s.db.QueryRowContext(ctx, curationReceipt+" AND c.document_id=$2", project, id))
}

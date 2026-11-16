package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type CommitStatus struct {
	ID          int64  `json:"id"`
	Commit      string `json:"commit"`
	Context     string `json:"context"`
	State       string `json:"state"`
	Description string `json:"description"`
}

type StatusPublisher func(ctx context.Context, repo string, issue Issue, validation ValidationResult) (CommitStatus, error)

func (s *Store) PublishCommitStatus(ctx context.Context, project, task, execution int64, sha string, publish StatusPublisher) (CommitStatus, error) {
	var receipt CommitStatus
	if !validCommit(sha) {
		return receipt, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return receipt, err
	}
	defer tx.Rollback()
	// Multiple attempts can refer to one remote SHA. Serialize all writes to its shared status context.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,$2))`, sha, project); err != nil {
		return receipt, err
	}
	var repo string
	var repoID int64
	var specJSON, resultJSON, stored []byte
	err = tx.QueryRowContext(ctx, `SELECT p.path,p.github_repository_id,t.spec,e.result,e.commit_status
		FROM executions e JOIN work_items t ON t.id=e.task_id JOIN projects p ON p.id=t.project_id
		WHERE p.id=$1 AND t.id=$2 AND e.id=$3 AND e.result->>'commit'=$4 AND e.result->'validation'->>'commit'=$4 FOR UPDATE OF e`, project, task, execution, sha).Scan(&repo, &repoID, &specJSON, &resultJSON, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		return receipt, ErrConflict
	}
	if err != nil {
		return receipt, err
	}
	if stored != nil {
		err = json.Unmarshal(stored, &receipt)
		return receipt, err
	}
	var newer bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM executions e JOIN work_items t ON t.id=e.task_id
		WHERE t.project_id=$1 AND e.id>$2 AND e.result->'validation'->>'commit'=$3)`, project, execution, sha).Scan(&newer)
	if err != nil {
		return receipt, err
	}
	if newer {
		return receipt, ErrConflict
	}
	var spec IssueSpec
	var result Result
	if err = json.Unmarshal(specJSON, &spec); err != nil {
		return receipt, err
	}
	if err = json.Unmarshal(resultJSON, &result); err != nil {
		return receipt, err
	}
	if spec.Issue.RepositoryID != repoID || result.Validation == nil {
		return receipt, ErrConflict
	}
	receipt, err = publish(ctx, repo, spec.Issue, *result.Validation)
	if err != nil {
		return receipt, err
	}
	if receipt.ID <= 0 || receipt.Commit != sha {
		return receipt, ErrConflict
	}
	doc, err := json.Marshal(receipt)
	if err != nil {
		return receipt, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE executions SET commit_status=$2::jsonb WHERE id=$1`, execution, string(doc)); err != nil {
		return receipt, err
	}
	return receipt, tx.Commit()
}

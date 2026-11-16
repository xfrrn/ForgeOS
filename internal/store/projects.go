package store

import (
	"context"
	"database/sql"
	"errors"
)

type RegisteredProject struct {
	Project
	CheckID     int64  `json:"check_id,omitempty"`
	CheckStatus string `json:"check_status,omitempty"`
}

func (s *Store) RegisteredProjects(ctx context.Context, before int64) ([]RegisteredProject, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.id,p.path,COALESCE(p.github_repository_id,0),COALESCE(t.id,0),COALESCE(t.status,'')
		FROM projects p LEFT JOIN LATERAL (SELECT id,status FROM work_items WHERE project_id=p.id AND kind='git_head' ORDER BY id DESC LIMIT 1) t ON true
		WHERE ($1::bigint=0 OR p.id<$1) ORDER BY p.id DESC LIMIT 31`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RegisteredProject{}
	for rows.Next() {
		var p RegisteredProject
		if err := rows.Scan(&p.ID, &p.Path, &p.RepositoryID, &p.CheckID, &p.CheckStatus); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

// Replaying a request after a lost response returns the same check, even when the worker has already finished.
func (s *Store) ProjectCheck(ctx context.Context, project, retry int64) (Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT id FROM projects WHERE id=$1 FOR UPDATE`, project).Scan(&project); errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM work_items WHERE project_id=$1 AND kind='git_head' ORDER BY id DESC LIMIT 1`, project).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Task{}, err
	}
	if retry != 0 {
		var belongs bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM work_items WHERE id=$1 AND project_id=$2 AND kind='git_head' AND status IN ('FAILED','DONE'))`, retry, project).Scan(&belongs); err != nil {
			return Task{}, err
		}
		if !belongs {
			return Task{}, ErrConflict
		}
	}
	if id == 0 || retry == id {
		task, err := createHeadTask(ctx, tx, project)
		if err != nil {
			return Task{}, err
		}
		id = task.ID
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.Task(ctx, id)
}

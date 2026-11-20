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

type ProjectMetrics struct {
	Running            int64 `json:"running"`
	PendingApprovals   int64 `json:"pending_approvals"`
	AwaitingAcceptance int64 `json:"awaiting_acceptance"`
}

type ProjectWorkspace struct {
	RegisteredProject
	Repository string         `json:"repository,omitempty"`
	Metrics    ProjectMetrics `json:"metrics"`
}

func (s *Store) ProjectWorkspaces(ctx context.Context, before int64, query string) ([]ProjectWorkspace, error) {
	return s.projectWorkspacePage(ctx, 0, before, query)
}

func (s *Store) ProjectWorkspace(ctx context.Context, project int64) (ProjectWorkspace, error) {
	rows, err := s.projectWorkspacePage(ctx, project, 0, "")
	if err != nil {
		return ProjectWorkspace{}, err
	}
	if len(rows) == 0 {
		return ProjectWorkspace{}, ErrNotFound
	}
	return rows[0], nil
}

func (s *Store) projectWorkspacePage(ctx context.Context, project, before int64, query string) ([]ProjectWorkspace, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH page_projects AS (
			SELECT p.id,p.path,p.github_repository_id,COALESCE(repo.name,'') AS repository
			FROM projects p LEFT JOIN LATERAL (
				SELECT t.spec->'issue'->>'repository' AS name FROM work_items t
				WHERE t.project_id=p.id AND NULLIF(t.spec->'issue'->>'repository','') IS NOT NULL
				ORDER BY t.id DESC LIMIT 1
			) repo ON true
			WHERE ($1::bigint=0 OR p.id=$1) AND ($2::bigint=0 OR p.id<$2)
			  AND ($3::text='' OR strpos(lower(p.path),lower($3))>0 OR strpos(lower(repo.name),lower($3))>0)
			ORDER BY p.id DESC LIMIT 31
		), work_metrics AS (
			SELECT t.project_id,count(*) FILTER (WHERE t.status='RUNNING') AS running,
			       count(*) FILTER (WHERE t.status='WAITING_ACCEPTANCE') AS awaiting_acceptance
			FROM work_items t JOIN page_projects p ON p.id=t.project_id
			WHERE t.status IN ('RUNNING','WAITING_ACCEPTANCE') GROUP BY t.project_id
		), approval_metrics AS (
			SELECT a.project_id,count(*) AS pending FROM (`+pendingApprovalCandidates+`) a
			JOIN page_projects p ON p.id=a.project_id GROUP BY a.project_id
		)
		SELECT p.id,p.path,COALESCE(p.github_repository_id,0),COALESCE(check_task.id,0),COALESCE(check_task.status,''),
		       p.repository,COALESCE(w.running,0),COALESCE(a.pending,0),COALESCE(w.awaiting_acceptance,0)
		FROM page_projects p LEFT JOIN work_metrics w ON w.project_id=p.id
		LEFT JOIN approval_metrics a ON a.project_id=p.id LEFT JOIN LATERAL (
			SELECT id,status FROM work_items WHERE project_id=p.id AND kind='git_head' ORDER BY id DESC LIMIT 1
		) check_task ON true ORDER BY p.id DESC`, project, before, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ProjectWorkspace{}
	for rows.Next() {
		var p ProjectWorkspace
		if err := rows.Scan(&p.ID, &p.Path, &p.RepositoryID, &p.CheckID, &p.CheckStatus, &p.Repository,
			&p.Metrics.Running, &p.Metrics.PendingApprovals, &p.Metrics.AwaitingAcceptance); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
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

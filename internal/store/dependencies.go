package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type ReadyDependency struct {
	Revision int
	TaskID   int64
	Base     string
	Issue    Issue
	Merges   []string
	Target   string
}

// Dependencies become eligible only after each prerequisite has completed GitHub delivery.
func (s *Store) NextDependent(ctx context.Context, projectID, after int64) (*ReadyDependency, error) {
	var job ReadyDependency
	var spec, merges []byte
	err := s.db.QueryRowContext(ctx, `SELECT w.id,w.plan_revision,w.spec,jsonb_agg(d.merge_sha),min(d.target_branch)
		FROM work_items w JOIN work_items g ON g.id=w.goal_id
		JOIN work_dependencies e ON e.task_id=w.id
		JOIN work_items prev ON prev.id=e.prerequisite
		LEFT JOIN deliveries d ON d.task_id=prev.id
		WHERE w.project_id=$1 AND g.status='ACTIVE' AND w.status='BLOCKED'
		GROUP BY w.id HAVING bool_and(prev.status='DONE' AND COALESCE(d.state='DONE',false))
		AND count(DISTINCT d.target_branch)=1 ORDER BY (w.id<=$2),w.id LIMIT 1`, projectID, after).Scan(&job.TaskID, &job.Revision, &spec, &merges, &job.Target)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var frozen IssueSpec
	if err = json.Unmarshal(spec, &frozen); err != nil {
		return nil, err
	}
	job.Base, job.Issue = frozen.BaseCommit, frozen.Issue
	err = json.Unmarshal(merges, &job.Merges)
	return &job, err
}

func (s *Store) ReleaseDependent(ctx context.Context, id int64, revision int, base string) error {
	if !validCommit(base) {
		return ErrConflict
	}
	res, err := s.db.ExecContext(ctx, `WITH released AS (
		UPDATE work_items w SET status='READY',spec=jsonb_set(spec,'{base_commit}',to_jsonb($2::text))
		WHERE w.id=$1 AND w.plan_revision=$3 AND w.status='BLOCKED' AND EXISTS(SELECT 1 FROM work_items g WHERE g.id=w.goal_id AND g.status='ACTIVE')
		AND EXISTS(SELECT 1 FROM work_dependencies WHERE task_id=w.id)
		AND NOT EXISTS(SELECT 1 FROM work_dependencies e JOIN work_items prev ON prev.id=e.prerequisite
		    LEFT JOIN deliveries d ON d.task_id=prev.id WHERE e.task_id=w.id AND (prev.status<>'DONE' OR d.state IS DISTINCT FROM 'DONE'))
		RETURNING w.id,w.plan_revision)
		INSERT INTO task_events(task_id,type,plan_revision,dependency_base)
		SELECT id,'dependencies_delivered',plan_revision,$2 FROM released`, id, base, revision)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrConflict
	}
	return err
}

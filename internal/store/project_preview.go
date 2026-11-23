package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"forgeos/internal/validation"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const ProjectPreviewKind = "project_preview"

type ProjectPreviewJob struct {
	ProjectID int64  `json:"project_id"`
	Action    string `json:"action"`
	Secret    string `json:"secret"`
}

type ProjectPreviewError struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type ProjectPreviewReport struct {
	OperationID int64                `json:"operation_id"`
	ProjectID   int64                `json:"project_id"`
	Action      string               `json:"action"`
	Outcome     string               `json:"outcome"`
	State       string               `json:"state"`
	Instance    string               `json:"instance_ref,omitempty"`
	Commit      string               `json:"commit,omitempty"`
	HTTPStatus  int                  `json:"http_status,omitempty"`
	Error       *ProjectPreviewError `json:"error,omitempty"`
}

type ProjectPreviewView struct {
	Key    string                `json:"-"`
	Worker string                `json:"worker,omitempty"`
	TaskID int64                 `json:"operation_id,omitempty"`
	Action string                `json:"action,omitempty"`
	Phase  string                `json:"phase,omitempty"`
	State  string                `json:"state"`
	Report *ProjectPreviewReport `json:"report,omitempty"`
}

func ValidProjectPreviewAction(action string) bool {
	return slices.Contains([]string{"start", "stop", "status"}, action)
}

func (s *Store) ProjectPreview(ctx context.Context, project int64) (ProjectPreviewView, error) {
	v := ProjectPreviewView{State: "stopped"}
	var report []byte
	var running, expired bool
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(p.preview_key,''), COALESCE(p.worker,''), COALESCE(t.id,0),
		       COALESCE(o.action,''), COALESCE(t.status,''), p.active_port IS NOT NULL,
		       (SELECT e.result->'project_preview' FROM executions e WHERE e.task_id=t.id ORDER BY e.id DESC LIMIT 1),
		       EXISTS(SELECT 1 FROM executions e WHERE e.task_id=t.id AND e.result IS NULL
		              AND e.abandoned_at IS NULL AND e.lease_until<=clock_timestamp())
		FROM projects pr LEFT JOIN project_previews p ON p.project_id=pr.id
		LEFT JOIN work_items t ON t.id=p.latest_task_id
		LEFT JOIN project_preview_operations o ON o.task_id=t.id WHERE pr.id=$1`, project).
		Scan(&v.Key, &v.Worker, &v.TaskID, &v.Action, &v.Phase, &running, &report, &expired)
	if running {
		v.State = "running"
	}
	if expired && v.Phase == "RUNNING" {
		v.Phase, v.State = "EXPIRED", "unknown"
	}
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil || len(report) == 0 {
		return v, err
	}
	return v, json.Unmarshal(report, &v.Report)
}

func (s *Store) QueueProjectPreview(ctx context.Context, project int64, worker, action string, request uuid.UUID) error {
	var expired int64
	err := s.db.QueryRowContext(ctx, `SELECT e.id FROM project_previews p JOIN executions e ON e.task_id=p.latest_task_id
		WHERE p.project_id=$1 AND e.result IS NULL AND e.abandoned_at IS NULL AND e.lease_until<=clock_timestamp()`, project).Scan(&expired)
	if err == nil {
		if err = s.Abandon(ctx, expired); err != nil && !errors.Is(err, ErrConflict) {
			return err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, "SELECT id FROM projects WHERE id=$1 FOR UPDATE", project).Scan(&project); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var key [24]byte
	var secret [32]byte
	if _, err = rand.Read(key[:]); err != nil {
		return err
	}
	if _, err = rand.Read(secret[:]); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO project_previews(project_id,preview_key,secret,worker)
		VALUES($1,$2,$3,$4) ON CONFLICT(project_id) DO NOTHING`, project, hex.EncodeToString(key[:]), hex.EncodeToString(secret[:]), worker); err != nil {
		return err
	}
	var latestStatus, latestAction, assigned string
	var task int64
	if err = tx.QueryRowContext(ctx, `SELECT p.worker, COALESCE(t.status,''), COALESCE(o.action,''), COALESCE(t.id,0) FROM project_previews p
		LEFT JOIN work_items t ON t.id=p.latest_task_id LEFT JOIN project_preview_operations o ON o.task_id=t.id
		WHERE p.project_id=$1 FOR UPDATE OF p`, project).Scan(&assigned, &latestStatus, &latestAction, &task); err != nil {
		return err
	}
	var prior string
	err = tx.QueryRowContext(ctx, `SELECT o.action FROM project_preview_requests r
		JOIN project_preview_operations o ON o.task_id=r.task_id WHERE r.project_id=$1 AND r.request_id=$2`, project, request).Scan(&prior)
	if err == nil {
		if prior != action {
			return ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if assigned != worker || (latestStatus == "READY" || latestStatus == "RUNNING") && latestAction != action {
		return ErrConflict
	}
	if latestStatus != "READY" && latestStatus != "RUNNING" {
		if err = tx.QueryRowContext(ctx, `INSERT INTO work_items(project_id,kind,status)
			VALUES($1,'project_preview','READY') RETURNING id`, project).Scan(&task); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO project_preview_operations(task_id,project_id,action) VALUES($1,$2,$3)", task, project, action); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE project_previews SET latest_task_id=$2 WHERE project_id=$1", project, task); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO project_preview_requests(project_id,request_id,task_id) VALUES($1,$2,$3)", project, request, task); err != nil {
		return err
	}
	return tx.Commit()
}

func claimProjectPreview(ctx context.Context, tx *sql.Tx, worker string, projects []int64) (*Job, error) {
	var j Job
	j.Preview = &ProjectPreviewJob{}
	err := tx.QueryRowContext(ctx, `
		SELECT t.id, pr.path,p.project_id,o.action,p.secret
        FROM project_previews p JOIN projects pr ON pr.id=p.project_id
        JOIN work_items t ON t.id=p.latest_task_id JOIN project_preview_operations o ON o.task_id=t.id
        WHERE t.status='READY' AND p.worker=$1 AND ($2::bigint[] IS NULL OR p.project_id=ANY($2))
        AND NOT EXISTS(SELECT 1 FROM work_items a WHERE a.project_id=p.project_id AND (a.status='RUNNING' OR a.waiting_execution_id IS NOT NULL))
        ORDER BY t.id LIMIT 1 FOR UPDATE OF pr,t SKIP LOCKED`, worker, pq.Array(projects)).
		Scan(&j.TaskID, &j.RepoPath, &j.Preview.ProjectID, &j.Preview.Action, &j.Preview.Secret)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(ctx, "UPDATE work_items SET status='RUNNING' WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM work_items WHERE project_id=$2 AND (status='RUNNING' OR waiting_execution_id IS NOT NULL)) RETURNING nextval(pg_get_serial_sequence('executions','id'))", j.TaskID, j.Preview.ProjectID).Scan(&j.ExecutionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	j.Kind = ProjectPreviewKind
	return &j, err
}

func settleProjectPreview(ctx context.Context, tx *sql.Tx, task int64, result Result) error {
	var project int64
	var action string
	if err := tx.QueryRowContext(ctx, "SELECT project_id,action FROM project_preview_operations WHERE task_id=$1", task).Scan(&project, &action); err != nil {
		return err
	}
	r := result.ProjectPreview
	if r == nil || r.OperationID != task || r.ProjectID != project || r.Action != action {
		return ErrConflict
	}
	if r.Outcome == "failed" && result.ExitCode != 0 && r.State == "unknown" && r.Error != nil &&
		len(r.Error.Message) <= 4096 && len(r.Error.Stage) <= 64 && result.PreviewPort == 0 {
		return nil
	}
	if r.Outcome != "ok" || result.ExitCode != 0 || r.Error != nil {
		return ErrConflict
	}
	switch r.State {
	case "running":
		if action == "stop" || result.PreviewPort < 1 || result.PreviewPort > 65535 || r.HTTPStatus < 200 ||
			r.HTTPStatus >= 300 || uuid.Validate(r.Instance) != nil || !validation.FullCommit(r.Commit) {
			return ErrConflict
		}
		_, err := tx.ExecContext(ctx, "UPDATE project_previews SET active_port=$2, active_instance=$3 WHERE latest_task_id=$1", task, result.PreviewPort, r.Instance)
		return err
	case "stopped":
		if action == "start" || result.PreviewPort != 0 {
			return ErrConflict
		}
		_, err := tx.ExecContext(ctx, "UPDATE project_previews SET active_port=NULL, active_instance=NULL WHERE latest_task_id=$1", task)
		return err
	default:
		return ErrConflict
	}
}

type ProjectPreviewTarget struct {
	Port             int
	Instance, Secret string
}

func (s *Store) ProjectPreviewTarget(ctx context.Context, key string) (ProjectPreviewTarget, error) {
	var v ProjectPreviewTarget
	err := s.db.QueryRowContext(ctx, `SELECT active_port,active_instance,secret FROM project_previews
		WHERE preview_key=$1 AND active_port IS NOT NULL AND active_instance IS NOT NULL`, key).Scan(&v.Port, &v.Instance, &v.Secret)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

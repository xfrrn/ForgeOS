package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/agents"
	"github.com/google/uuid"
)

type ChatSource struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Path  string `json:"path"`
}

type ChatActivity struct {
	Key   string    `json:"key"`
	Role  string    `json:"role"`
	Label string    `json:"label"`
	Path  string    `json:"path"`
	At    time.Time `json:"at"`
}

type ChatAction struct {
	Body            string     `json:"body,omitempty"`
	CreationStarted *time.Time `json:"creation_started_at,omitempty"`
	PlanMode        string     `json:"-"`
	CreateKey       string     `json:"-"`

	ID          int64  `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Repository  string `json:"repository,omitempty"`
	IssueNumber int64  `json:"issue_number,omitempty"`
	WorkType    string `json:"work_type,omitempty"`
	TaskID      int64  `json:"task_id,omitempty"`
	Status      string `json:"status"`

	TurnID       int64  `json:"-"`
	RepositoryID int64  `json:"-"`
	SourceDigest string `json:"-"`
	HeadRetry    int64  `json:"-"`
}

type ChatTurn struct {
	ModelProfileName string         `json:"model_profile_name,omitempty"`
	RequestedHarness string         `json:"requested_harness,omitempty"`
	RequestedModel   string         `json:"requested_model,omitempty"`
	ID               int64          `json:"id"`
	ClientID         string         `json:"client_id"`
	Text             string         `json:"text"`
	CreatedAt        time.Time      `json:"created_at"`
	Status           string         `json:"status"`
	Answer           string         `json:"answer,omitempty"`
	Error            string         `json:"error,omitempty"`
	Sources          []ChatSource   `json:"sources,omitempty"`
	Action           *ChatAction    `json:"action,omitempty"`
	Activities       []ChatActivity `json:"activities,omitempty"`
}

type ChatJob struct {
	RequestedHarness string    `json:"requested_harness,omitempty"`
	RequestedModel   string    `json:"requested_model,omitempty"`
	ID               int64     `json:"id"`
	ProjectID        int64     `json:"project_id"`
	Lease            string    `json:"lease"`
	Until            time.Time `json:"lease_until"`
	Prompt           string    `json:"prompt"`
	Recover          bool      `json:"recover_native"`
	Text             string    `json:"-"`
}

const chatTurnFields = `t.id,t.client_id,t.text,t.requested_model,t.requested_harness,t.requested_model,t.created_at,
	CASE WHEN t.status='running' AND t.lease_until<=now()
			THEN CASE WHEN t.attempts>=3 OR t.created_at<=now()-interval '30 minutes' THEN 'failed' ELSE 'queued' END
		WHEN t.status='queued' AND t.created_at<=now()-interval '30 minutes' THEN 'failed'
		ELSE t.status END,
	CASE WHEN t.status='completed' OR (t.status='running' AND t.lease_until>now()) THEN t.answer ELSE '' END,t.error,t.citations,
	(SELECT jsonb_build_object('id',a.id,'kind',a.kind,'label',a.label,'repository',a.repository,
		'body',a.issue_body,'creation_started_at',a.create_started_at,'issue_number',a.issue_number,'work_type',a.work_type,'task_id',a.task_id,'status',a.status)
	 FROM project_chat_actions a WHERE a.turn_id=t.id AND a.project_id=t.project_id)`

func scanChatTurn(row interface{ Scan(...any) error }) (ChatTurn, error) {
	var turn ChatTurn
	var citations, action []byte
	err := row.Scan(&turn.ID, &turn.ClientID, &turn.Text, &turn.RequestedModel, &turn.RequestedHarness, &turn.ModelProfileName, &turn.CreatedAt, &turn.Status, &turn.Answer, &turn.Error, &citations, &action)
	if errors.Is(err, sql.ErrNoRows) {
		return turn, ErrNotFound
	}
	if err != nil {
		return turn, err
	}
	if turn.Status == "failed" && turn.Error == "" {
		turn.Error = "Management 未能在等待期限内回复，请重试。"
	}
	if err = json.Unmarshal(citations, &turn.Sources); err != nil {
		return turn, err
	}
	if action != nil {
		err = json.Unmarshal(action, &turn.Action)
	}
	return turn, err
}

func (s *Store) ChatTurn(ctx context.Context, project, id int64) (ChatTurn, error) {
	return scanChatTurn(s.db.QueryRowContext(ctx, `SELECT `+chatTurnFields+` FROM project_chat_turns t WHERE project_id=$1 AND id=$2`, project, id))
}

func (s *Store) ChatHistory(ctx context.Context, project, before int64, limit int) ([]ChatTurn, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+chatTurnFields+` FROM project_chat_turns t
		WHERE project_id=$1 AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT $3`, project, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	turns := make([]ChatTurn, 0)
	for rows.Next() {
		turn, err := scanChatTurn(rows)
		if err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	return turns, rows.Err()
}

func (s *Store) SendChat(ctx context.Context, project int64, clientID, text string, profile uuid.UUID) (ChatTurn, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO project_chat_turns(project_id,client_id,text,requested_model,requested_harness,model_profile_id,model_revision,model_profile_name)
        SELECT p.id,$2,$3,m.model,'codex',m.id,m.revision,m.model FROM projects p CROSS JOIN chat_models m
        WHERE p.id=$1 AND m.id=$4 AND m.revision IS NOT NULL
        ON CONFLICT(project_id,client_id) DO NOTHING RETURNING id`, project, clientID, text, profile).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		// A retry of an accepted message still succeeds after rotation or removal.
		err = s.db.QueryRowContext(ctx, `SELECT id FROM project_chat_turns WHERE project_id=$1 AND client_id=$2 AND text=$3 AND model_profile_id=$4`, project, clientID, text, profile).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ChatTurn{}, ErrConflict
		}
	}
	if err != nil {
		return ChatTurn{}, err
	}
	return s.ChatTurn(ctx, project, id)
}

func (s *Store) ClaimChat(ctx context.Context, project int64, supportsModel bool) (*ChatJob, error) {
	tx, err := s.beginRoleClaim(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT id FROM projects WHERE id=$1 FOR UPDATE`, project).Scan(&project); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !supportsModel {
		d, _ := agents.Lookup("management-chat")
		entry, err := agentConfig(ctx, tx.Tx, project, d)
		if err != nil || entry.Effective.Harness != "codex" {
			return nil, err
		}
	}
	job := ChatJob{ProjectID: project, Lease: uuid.NewString(), Recover: true}
	err = tx.QueryRowContext(ctx, `UPDATE project_chat_turns SET status='running', answer='', attempts=attempts+1,
		lease=$2,lease_until=now()+interval '6 minutes',evidence='[]',progress_task_ids='{}'
		WHERE id=(SELECT id FROM project_chat_turns
			WHERE project_id=$1 AND ($3 OR requested_model='') AND status IN ('queued','running') AND attempts<3
			AND created_at>now()-interval '30 minutes' AND (lease_until IS NULL OR lease_until<=now())
			AND NOT EXISTS (SELECT 1 FROM project_chat_turns WHERE project_id=$1 AND status='running' AND lease_until>now())
			ORDER BY id LIMIT 1)
		RETURNING id,text,lease_until,requested_model,requested_harness`, project, job.Lease, supportsModel).Scan(&job.ID, &job.Text, &job.Until, &job.RequestedModel, &job.RequestedHarness)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, tx.Commit()
}

func (s *Store) SaveChatEvidence(ctx context.Context, job ChatJob, sources []ChatSource) error {
	raw, err := json.Marshal(sources)
	if err != nil {
		return err
	}
	exec := s.db.ExecContext
	if tx, ok := ctx.Value(roleClaimTransactionKey{}).(*sql.Tx); ok {
		exec = tx.ExecContext
	}
	res, err := exec(ctx, `UPDATE project_chat_turns SET evidence=$4
		WHERE project_id=$1 AND id=$2 AND lease=$3 AND status='running' AND lease_until>now()`,
		job.ProjectID, job.ID, job.Lease, raw)
	return chatUpdated(res, err)
}

// The same lease that owns the final result owns its temporary answer. Claiming
// a replacement lease clears the draft; completing the turn replaces it atomically.
func (s *Store) UpdateChatProgress(ctx context.Context, project, id int64, lease, answer string) error {
	return chatUpdated(s.db.ExecContext(ctx, `UPDATE project_chat_turns SET answer=$4 WHERE project_id=$1 AND id=$2 AND lease=$3 AND status='running' AND lease_until>now()`, project, id, lease, answer))
}

func chatUpdated(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}

// ChatResultInput is read before preparing an action with GitHub. CompleteChat
// rechecks the lease under lock before publishing the reply and its card.
func (s *Store) ChatResultInput(ctx context.Context, project, id int64, lease string) (string, bool, error) {
	var text, status string
	err := s.db.QueryRowContext(ctx, `SELECT text,status FROM project_chat_turns
		WHERE project_id=$1 AND id=$2 AND lease=$3 AND (status IN ('completed','failed') OR lease_until>now())`,
		project, id, lease).Scan(&text, &status)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrConflict
	}
	return text, status == "completed" || status == "failed", err
}

func (s *Store) CompleteChat(ctx context.Context, project, id int64, lease, answer, failure string, keys []string, action *ChatAction) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	var evidence []byte
	err = tx.QueryRowContext(ctx, `SELECT status,evidence FROM project_chat_turns WHERE project_id=$1 AND id=$2 AND lease=$3
		AND (status IN ('completed','failed') OR lease_until>now()) FOR UPDATE`, project, id, lease).Scan(&status, &evidence)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil || status == "completed" || status == "failed" {
		return err
	}
	var allowed []ChatSource
	if err := json.Unmarshal(evidence, &allowed); err != nil {
		return err
	}
	sources := make([]ChatSource, 0, len(keys))
	seen := make(map[string]bool)
	for _, key := range keys {
		i := slices.IndexFunc(allowed, func(src ChatSource) bool { return src.Key == key })
		if i < 0 || seen[key] {
			continue
		}
		seen[key] = true
		sources = append(sources, allowed[i])
	}
	raw, err := json.Marshal(sources)
	if err != nil {
		return err
	}
	status = "completed"
	if failure != "" {
		status = "failed"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE project_chat_turns SET status=$2,answer=$3,error=$4,citations=$5 WHERE id=$1`,
		id, status, answer, failure, raw); err != nil {
		return err
	}
	if action != nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO project_chat_actions(project_id,turn_id,kind,label,repository,issue_number,
			repository_id,source_digest,work_type,head_retry,issue_body) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			project, id, action.Kind, action.Label, action.Repository, action.IssueNumber,
			action.RepositoryID, action.SourceDigest, action.WorkType, action.HeadRetry, action.Body)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

const chatActionSelect = `SELECT id,turn_id,kind,label,repository,issue_number,work_type,COALESCE(task_id,0),status,
	repository_id,source_digest,head_retry,issue_body,create_key,create_started_at,plan_mode FROM project_chat_actions WHERE project_id=$1 AND id=$2`

func scanChatAction(row interface{ Scan(...any) error }) (ChatAction, error) {
	var a ChatAction
	err := row.Scan(&a.ID, &a.TurnID, &a.Kind, &a.Label, &a.Repository, &a.IssueNumber, &a.WorkType, &a.TaskID, &a.Status,
		&a.RepositoryID, &a.SourceDigest, &a.HeadRetry, &a.Body, &a.CreateKey, &a.CreationStarted, &a.PlanMode)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return a, err
}

func (s *Store) ChatAction(ctx context.Context, project, action int64) (ChatAction, error) {
	return scanChatAction(s.db.QueryRowContext(ctx, chatActionSelect, project, action))
}

// ConfirmChatAction commits the work item and action receipt together. A nil
// Issue spec means the fetched source changed and requires a fresh chat request.
func (s *Store) ConfirmChatAction(ctx context.Context, project, id int64, spec *IssueSpec) (ChatTurn, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ChatTurn{}, err
	}
	defer tx.Rollback()
	a, err := scanChatAction(tx.QueryRowContext(ctx, chatActionSelect+` FOR UPDATE`, project, id))
	if err != nil {
		return ChatTurn{}, err
	}
	if a.Status == "pending" {
		status := "confirmed"
		var task int64
		switch {
		case a.Kind == "check_head":
			task, err = projectCheck(ctx, tx, project, a.HeadRetry)
		case spec == nil:
			status = "stale"
		case spec.Issue.RepositoryID != a.RepositoryID || spec.Issue.Number != a.IssueNumber:
			return ChatTurn{}, ErrConflict
		default:
			task, err = importIssueTask(ctx, tx, project, *spec)
		}
		if err != nil {
			return ChatTurn{}, err
		}
		workType := a.WorkType
		if status == "confirmed" && spec != nil {
			workType = spec.Issue.WorkType
		}
		_, err = tx.ExecContext(ctx, `UPDATE project_chat_actions SET status=$3,task_id=NULLIF($4,0),work_type=$5,confirmed_at=CASE WHEN $3='confirmed' THEN clock_timestamp() END
			WHERE project_id=$1 AND id=$2`, project, id, status, task, workType)
		if err != nil {
			return ChatTurn{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ChatTurn{}, err
	}
	return s.ChatTurn(ctx, project, a.TurnID)
}

func ChatTaskSource(id int64) ChatSource {
	return ChatSource{Key: fmt.Sprintf("task:%d", id), Label: fmt.Sprintf("任务 T-%d", id), Path: fmt.Sprintf("/tasks/%d", id)}
}

func chatSourceTask(key string) int64 {
	raw, ok := strings.CutPrefix(key, "task:")
	if !ok {
		return 0
	}
	id, _ := strconv.ParseInt(raw, 10, 64)
	return id
}

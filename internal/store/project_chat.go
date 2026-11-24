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
	"github.com/lib/pq"
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
	Steps           json.RawMessage `json:"steps,omitempty"`
	Design          bool            `json:"-"`
	Body            string          `json:"body,omitempty"`
	CreationStarted *time.Time      `json:"creation_started_at,omitempty"`
	PlanMode        string          `json:"-"`
	CreateKey       string          `json:"-"`

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
	CancelRequested  bool           `json:"cancel_requested,omitempty"`
	SessionMissing   bool           `json:"session_missing,omitempty"`
	RecoveryOf       int64          `json:"recovery_of,omitempty"`
	NativeBackend    string         `json:"native_backend,omitempty"`
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
	Sources          []ChatSource     `json:"sources,omitempty"`
	Action           *ChatAction      `json:"action,omitempty"`
	Activities       []ChatActivity   `json:"activities,omitempty"`
	Attachments      []ChatAttachment `json:"attachments,omitempty"`
}

type ChatJob struct {
	ProposalID       int64            `json:"proposal_id,omitempty"`
	AssistKind       string           `json:"assist_kind,omitempty"`
	CancelRequested  bool             `json:"cancel_requested,omitempty"`
	Generation       int64            `json:"session_generation,omitempty"`
	ExpectSession    bool             `json:"expect_session,omitempty"`
	Native           *FeatureClient   `json:"native,omitempty"`
	FeatureID        int64            `json:"feature_id,omitempty"`
	ProjectPath      string           `json:"project_path,omitempty"`
	RequestedHarness string           `json:"requested_harness,omitempty"`
	RequestedModel   string           `json:"requested_model,omitempty"`
	ID               int64            `json:"id"`
	ProjectID        int64            `json:"project_id"`
	Lease            string           `json:"lease"`
	Until            time.Time        `json:"lease_until"`
	Prompt           string           `json:"prompt"`
	Recover          bool             `json:"recover_native"`
	Attachments      []ChatAttachment `json:"attachments,omitempty"`
	Text             string           `json:"-"`
}

const chatTurnFields = `t.cancel_requested,t.session_missing,COALESCE(t.recovery_of,0),CASE WHEN t.native_client IS NOT NULL THEN t.requested_harness ELSE '' END,t.id,t.client_id,t.text,t.requested_model,t.requested_harness,t.requested_model,t.created_at,
	CASE WHEN t.status='running' AND t.lease_until<=now()
			THEN CASE WHEN t.attempts>=3 OR t.created_at<=now()-interval '30 minutes' THEN 'failed' ELSE 'queued' END
		WHEN t.status='queued' AND t.created_at<=now()-interval '30 minutes' THEN 'failed'
		ELSE t.status END,
	CASE WHEN t.status='completed' OR (t.status='running' AND t.lease_until>now()) THEN t.answer ELSE '' END,t.error,t.citations,
	(SELECT jsonb_build_object('id',a.id,'kind',a.kind,'label',a.label,'repository',a.repository,
		'body',a.issue_body,'creation_started_at',a.create_started_at,'issue_number',a.issue_number,'work_type',a.work_type,'task_id',a.task_id,'status',a.status)
	 FROM project_chat_actions a WHERE a.turn_id=t.id AND a.project_id=t.project_id),
	COALESCE((SELECT jsonb_agg(jsonb_build_object('id',ca.id,'name',ca.name,'mime',ca.mime,'bytes',ca.bytes) ORDER BY ca.id)
	 FROM project_chat_attachments ca WHERE ca.project_id=t.project_id AND ca.turn_id=t.id),'[]')`

func scanChatTurn(row interface{ Scan(...any) error }) (ChatTurn, error) {
	var turn ChatTurn
	var citations, action, attachments []byte
	err := row.Scan(&turn.CancelRequested, &turn.SessionMissing, &turn.RecoveryOf, &turn.NativeBackend, &turn.ID, &turn.ClientID, &turn.Text, &turn.RequestedModel, &turn.RequestedHarness, &turn.ModelProfileName, &turn.CreatedAt, &turn.Status, &turn.Answer, &turn.Error, &citations, &action, &attachments)
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
		if err = json.Unmarshal(action, &turn.Action); err != nil {
			return turn, err
		}
	}
	if len(attachments) > 0 {
		var items []ChatAttachment
		if err = json.Unmarshal(attachments, &items); err != nil {
			return turn, err
		}
		if len(items) > 0 {
			turn.Attachments = items
		}
	}
	return turn, nil
}

func (s *Store) ChatTurn(ctx context.Context, project, id int64) (ChatTurn, error) {
	return scanChatTurn(s.db.QueryRowContext(ctx, `SELECT `+chatTurnFields+` FROM project_chat_turns t WHERE project_id=$1 AND id=$2`, project, id))
}

func (s *Store) ChatHistory(ctx context.Context, project, before int64, limit int) ([]ChatTurn, error) {
	return s.FeatureHistory(ctx, project, 0, before, limit)
}

func (s *Store) FeatureHistory(ctx context.Context, project, feature, before int64, limit int) ([]ChatTurn, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+chatTurnFields+` FROM project_chat_turns t
		WHERE project_id=$1 AND proposal_id IS NULL AND assist_kind='' AND feature_id IS NOT DISTINCT FROM NULLIF($4::bigint,0) AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT $3`, project, before, limit, feature)
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
	return s.SendFeatureChat(ctx, project, 0, clientID, text, profile)
}

func (s *Store) SendFeatureChat(ctx context.Context, project, feature int64, clientID, text string, profile uuid.UUID) (ChatTurn, error) {
	return s.SendFeatureChatWithAttachments(ctx, project, feature, clientID, text, profile, nil)
}

func (s *Store) SendFeatureChatWithAttachments(ctx context.Context, project, feature int64, clientID, text string, profile uuid.UUID, attachmentIDs []int64) (ChatTurn, error) {
	return s.SendFeatureChatMessage(ctx, project, feature, clientID, text, profile, "", attachmentIDs)
}

func (s *Store) SendFeatureChatMessage(ctx context.Context, project, feature int64, clientID, text string, profile uuid.UUID, requestedModel string, attachmentIDs []int64) (ChatTurn, error) {
	unique := make([]int64, 0, len(attachmentIDs))
	for _, aid := range attachmentIDs {
		if aid < 1 {
			return ChatTurn{}, ErrConflict
		}
		if !slices.Contains(unique, aid) {
			unique = append(unique, aid)
		}
	}
	slices.Sort(unique)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ChatTurn{}, err
	}
	defer tx.Rollback()
	var id int64
	replayed := false
	err = tx.QueryRowContext(ctx, `INSERT INTO project_chat_turns(project_id,feature_id,client_id,text,requested_model,requested_harness,model_profile_id,model_revision,model_profile_name,native_client)
        SELECT p.id,NULLIF($5::bigint,0),$2,$3,m.effective_model,m.backend,m.id,m.revision,m.effective_model,m.native FROM projects p CROSS JOIN (
            SELECT id,model AS effective_model,'codex' AS backend,revision,NULL::uuid AS native
            FROM chat_models WHERE id=$4 AND revision IS NOT NULL AND ($6='' OR model=$6)
            UNION ALL
            SELECT id,CASE WHEN $6<>'' THEN $6 ELSE model END AS effective_model,backend,NULL,id
            FROM feature_clients WHERE id=$4 AND project_id=$1 AND $5::bigint>0 AND seen_at>now()-interval '3 minutes'
              AND ($6='' OR model=$6 OR EXISTS(SELECT 1 FROM jsonb_array_elements(available_models) elem WHERE elem->>'id'=$6))
        ) m
 WHERE p.id=$1 AND ($5=0 OR EXISTS(SELECT 1 FROM feature_topics WHERE project_id=p.id AND id=$5))
        ON CONFLICT(project_id,client_id) DO NOTHING RETURNING id`, project, clientID, text, profile, feature, requestedModel).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		replayed = true
		// A retry of an accepted message still succeeds after rotation or removal.
		err = tx.QueryRowContext(ctx, `SELECT id FROM project_chat_turns WHERE project_id=$1 AND client_id=$2 AND text=$3 AND model_profile_id=$4 AND feature_id IS NOT DISTINCT FROM NULLIF($5::bigint,0) AND ($6='' OR requested_model=$6)`, project, clientID, text, profile, feature, requestedModel).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ChatTurn{}, ErrConflict
		}
	}
	if err != nil {
		return ChatTurn{}, err
	}
	if replayed {
		var existing []int64
		rows, err := tx.QueryContext(ctx, `SELECT id FROM project_chat_attachments WHERE project_id=$1 AND turn_id=$2 ORDER BY id`, project, id)
		if err != nil {
			return ChatTurn{}, err
		}
		for rows.Next() {
			var aid int64
			if err := rows.Scan(&aid); err != nil {
				rows.Close()
				return ChatTurn{}, err
			}
			existing = append(existing, aid)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return ChatTurn{}, err
		}
		if !slices.Equal(existing, unique) {
			return ChatTurn{}, ErrConflict
		}
	} else if len(unique) > 0 {
		res, err := tx.ExecContext(ctx, `UPDATE project_chat_attachments SET turn_id=$1
			WHERE project_id=$2 AND id=ANY($3::bigint[]) AND turn_id IS NULL`, id, project, pq.Array(unique))
		if err != nil {
			return ChatTurn{}, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return ChatTurn{}, err
		}
		if int(n) != len(unique) {
			return ChatTurn{}, ErrConflict
		}
	}
	if err := tx.Commit(); err != nil {
		return ChatTurn{}, err
	}
	return s.ChatTurn(ctx, project, id)
}

func (s *Store) ClaimChat(ctx context.Context, project int64, supportsModel bool) (*ChatJob, error) {
	return s.ClaimFeatureAwareChat(ctx, project, supportsModel, "")
}

func (s *Store) ClaimFeatureAwareChat(ctx context.Context, project int64, supportsModel bool, node string) (*ChatJob, error) {
	return s.ClaimDiscussionJob(ctx, project, supportsModel, node, false)
}

func (s *Store) ClaimDiscussionJob(ctx context.Context, project int64, supportsModel bool, node string, proposals bool) (*ChatJob, error) {
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
	var native uuid.NullUUID
	err = tx.QueryRowContext(ctx, `UPDATE project_chat_turns SET status='running',resource_worker=NULL,model_budget=NULL, answer='', attempts=attempts+1,
		lease=$2,lease_until=now()+interval '6 minutes',evidence='[]',progress_task_ids='{}'
		WHERE id=(SELECT candidate.id FROM project_chat_turns candidate
			WHERE project_id=$1 AND ($5 OR (proposal_id IS NULL AND assist_kind='')) AND ($3 OR requested_model='') AND status IN ('queued','running') AND attempts<3
			AND (native_client IS NULL OR native_client IN (SELECT fc.id FROM feature_clients fc WHERE fc.project_id=$1 AND fc.node=$4 AND fc.seen_at>now()-interval '3 minutes'
            AND NOT EXISTS(SELECT 1 FROM model_budget_usage b WHERE fc.resource_worker=ANY(b.workers) AND b.harness=fc.backend AND b.reserved>=b.slots AND candidate.status='queued')))
 AND created_at>now()-interval '30 minutes' AND (lease_until IS NULL OR lease_until<=now())
			AND NOT EXISTS (SELECT 1 FROM project_chat_turns active WHERE active.project_id=$1 AND active.feature_id IS NOT DISTINCT FROM candidate.feature_id AND (candidate.proposal_id IS NULL AND candidate.assist_kind='' AND active.proposal_id IS NULL AND active.assist_kind='') AND active.status='running' AND active.lease_until>now())
			ORDER BY id LIMIT 1)
		RETURNING id,text,lease_until,requested_model,requested_harness,COALESCE(feature_id,0),native_client,cancel_requested,coalesce(proposal_id,0),coalesce(proposal_prompt,''),assist_kind`, project, job.Lease, supportsModel, node, proposals).Scan(&job.ID, &job.Text, &job.Until, &job.RequestedModel, &job.RequestedHarness, &job.FeatureID, &native, &job.CancelRequested, &job.ProposalID, &job.Prompt, &job.AssistKind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !native.Valid && job.FeatureID != 0 && node != "" {
		var resource string
		err = tx.QueryRowContext(ctx, `SELECT coalesce((SELECT resource_worker FROM feature_clients WHERE project_id=$1 AND node=$2 AND resource_worker<>'' ORDER BY checked_at DESC NULLS LAST LIMIT 1),'')`, project, node).Scan(&resource)
		if err != nil {
			return nil, err
		}
		if resource != "" {
			if err = reserveDiscussionSlot(ctx, tx.Tx, job.ID, resource, ""); errors.Is(err, errDiscussionBusy) {
				return nil, nil
			} else if err != nil {
				return nil, err
			}
		}
	}
	if native.Valid {
		c := FeatureClient{ID: native.UUID}
		err = tx.QueryRowContext(ctx, "SELECT instance,backend,model FROM feature_clients WHERE id=$1", native.UUID).Scan(&c.Instance, &c.Backend, &c.Model)
		if err != nil {
			return nil, err
		}
		var resource string
		if err = tx.QueryRowContext(ctx, "SELECT resource_worker FROM feature_clients WHERE id=$1", c.ID).Scan(&resource); err != nil {
			return nil, err
		}
		if resource != "" {
			if err = reserveDiscussionSlot(ctx, tx.Tx, job.ID, resource, c.Backend); errors.Is(err, errDiscussionBusy) {
				return nil, nil
			} else if err != nil {
				return nil, err
			}
		}
		err = tx.QueryRowContext(ctx, `SELECT coalesce(max(id) FILTER(WHERE recovery_of IS NOT NULL),0)
         FROM project_chat_turns WHERE project_id=$1 AND feature_id=$2 AND native_client=$3 AND proposal_id IS NULL AND assist_kind='' AND id<=$4`, project, job.FeatureID, c.ID, job.ID).Scan(&job.Generation)
		if err != nil {
			return nil, err
		}
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM project_chat_turns WHERE project_id=$1 AND feature_id=$2 AND native_client=$3 AND proposal_id IS NULL AND assist_kind='' AND id>=$4 AND id<$5 AND status='completed')`, project, job.FeatureID, c.ID, job.Generation, job.ID).Scan(&job.ExpectSession)
		if err != nil {
			return nil, err
		}
		if job.ProposalID != 0 || job.AssistKind != "" {
			job.ExpectSession = false
		}
		job.Native = &c
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,name,mime,sha256,bytes FROM project_chat_attachments WHERE project_id=$1 AND turn_id=$2 ORDER BY id`, project, job.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var a ChatAttachment
		if err := rows.Scan(&a.ID, &a.Name, &a.MIME, &a.SHA256, &a.Bytes); err != nil {
			rows.Close()
			return nil, err
		}
		job.Attachments = append(job.Attachments, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
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
	var feature, cancelRequested bool
	var evidence []byte
	err = tx.QueryRowContext(ctx, `SELECT status,evidence,feature_id IS NOT NULL,cancel_requested FROM project_chat_turns WHERE project_id=$1 AND id=$2 AND lease=$3
		AND (status IN ('completed','failed') OR lease_until>now()) FOR UPDATE`, project, id, lease).Scan(&status, &evidence, &feature, &cancelRequested)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil || status == "completed" || status == "failed" {
		return err
	}
	if cancelRequested {
		answer = ""
		keys = nil
		action = nil
		if failure != FeatureCancelUnconfirmed {
			failure = FeatureCancelled
		}
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
	if _, err = tx.ExecContext(ctx, `UPDATE project_chat_turns SET status=$2,answer=$3,error=$4,citations=$5,session_missing=$6 WHERE id=$1`,
		id, status, answer, failure, raw, failure == FeatureSessionMissing); err != nil {
		return err
	}
	if action != nil && !feature {
		_, err = tx.ExecContext(ctx, `INSERT INTO project_chat_actions(project_id,turn_id,kind,label,repository,issue_number,
			repository_id,source_digest,work_type,head_retry,issue_body) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			project, id, action.Kind, action.Label, action.Repository, action.IssueNumber,
			action.RepositoryID, action.SourceDigest, action.WorkType, action.HeadRetry, action.Body)
		if err != nil {
			return err
		}
	}
	if status == "completed" {
		if err = completeProposalTurn(ctx, tx, id); err != nil {
			return err
		}
		if err = completeAssistTurn(ctx, tx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const chatActionSelect = `SELECT id,turn_id,kind,label,repository,issue_number,work_type,COALESCE(task_id,0),status,
	repository_id,source_digest,head_retry,issue_body,create_key,create_started_at,plan_mode,EXISTS(SELECT 1 FROM project_chat_turns t WHERE t.id=turn_id AND t.proposal_role='design_final'),implementation_steps FROM project_chat_actions WHERE project_id=$1 AND id=$2`

func scanChatAction(row interface{ Scan(...any) error }) (ChatAction, error) {
	var a ChatAction
	var steps []byte
	err := row.Scan(&a.ID, &a.TurnID, &a.Kind, &a.Label, &a.Repository, &a.IssueNumber, &a.WorkType, &a.TaskID, &a.Status,
		&a.RepositoryID, &a.SourceDigest, &a.HeadRetry, &a.Body, &a.CreateKey, &a.CreationStarted, &a.PlanMode, &a.Design, &steps)
	a.Steps = steps
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
		if a.Design && spec != nil && a.Kind == "dispatch_issue" {
			// Claims and imports lock the project before assigning work. Under that lock an
			// existing task may acquire a design only before its first execution.
			var projectLock int64
			if err = tx.QueryRowContext(ctx, "SELECT id FROM projects WHERE id=$1 FOR UPDATE", project).Scan(&projectLock); err != nil {
				return ChatTurn{}, err
			}
			var available bool
			err = tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM work_items w WHERE w.project_id=$1 AND w.github_issue_id=$2 AND
                (w.status<>'READY' OR w.goal_id IS NOT NULL OR coalesce(w.spec->>'mode','')<>'' OR coalesce(w.spec->'issue'->>'work_type','development')<>'development' OR
                 EXISTS(SELECT 1 FROM executions e WHERE e.task_id=w.id) OR EXISTS(SELECT 1 FROM project_chat_actions other JOIN project_chat_turns t ON t.id=other.turn_id WHERE other.task_id=w.id AND t.proposal_role='design_final')))`, project, spec.Issue.ID).Scan(&available)
			if err != nil {
				return ChatTurn{}, err
			}
			if !available {
				spec = nil
			}
		}
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
		if status == "confirmed" && len(a.Steps) > 0 {
			if err = startFeatureBuild(ctx, tx, project, task, a); err != nil {
				return ChatTurn{}, err
			}
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

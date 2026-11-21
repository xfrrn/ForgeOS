package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"forgeos/internal/agents"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type DiscussionInput struct {
	ClientID uuid.UUID `json:"client_id"`
	// Accepted from already-open pre-upgrade clients. Routing always follows the project Planner.
	LegacyModelID uuid.UUID `json:"model_id,omitempty"`
	Revision      int       `json:"revision"`
	Item          string    `json:"item"`
	Message       string    `json:"message"`
	Draft         bool      `json:"draft"`
}

type DiscussionTurn struct {
	WorkerTask int64     `json:"worker_task_id,omitempty"`
	ID         int64     `json:"id"`
	Revision   int       `json:"revision"`
	Item       string    `json:"item,omitempty"`
	Message    string    `json:"message"`
	Draft      bool      `json:"draft"`
	Model      string    `json:"model"`
	Status     string    `json:"status"`
	Answer     string    `json:"answer,omitempty"`
	Generated  int       `json:"generated_revision,omitempty"`
	At         time.Time `json:"at"`
}

const discussionColumns = `id,revision,item_key,message,draft,model_name,status,answer,COALESCE(generated_revision,0),created_at,COALESCE(worker_task_id,0)`

func (s *Store) DiscussionHistory(ctx context.Context, task int64) ([]DiscussionTurn, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+discussionColumns+" FROM goal_discussion_turns WHERE task_id=$1 ORDER BY id", task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	turns := []DiscussionTurn{}
	for rows.Next() {
		var t DiscussionTurn
		if err = rows.Scan(&t.ID, &t.Revision, &t.Item, &t.Message, &t.Draft, &t.Model, &t.Status, &t.Answer, &t.Generated, &t.At, &t.WorkerTask); err != nil {
			return nil, err
		}
		turns = append(turns, t)
	}
	return turns, rows.Err()
}

func (s *Store) SendDiscussion(ctx context.Context, task int64, in DiscussionInput) (int64, error) {
	in.Message = strings.TrimSpace(in.Message)
	if in.ClientID == uuid.Nil || in.Revision < 1 || utf8.RuneCountInString(in.Message) > 4000 || strings.ContainsRune(in.Message, 0) ||
		(in.Item != "" && !planItemKey.MatchString(in.Item)) || (in.Message == "") != in.Draft {
		return 0, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var plan Plan
	var raw []byte
	var latest int
	var status string
	err = tx.QueryRowContext(ctx, `SELECT r.plan,t.status FROM work_items t JOIN plan_revisions r ON r.task_id=t.id
 WHERE t.id=$1 AND r.revision=$2 FOR UPDATE OF t`, task, in.Revision).Scan(&raw, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	var id int64
	var same bool
	err = tx.QueryRowContext(ctx, `SELECT id,revision=$3 AND item_key=$4 AND message=$5 AND draft=$6
 FROM goal_discussion_turns WHERE task_id=$1 AND client_id=$2`, task, in.ClientID, in.Revision, in.Item, in.Message, in.Draft).Scan(&id, &same)
	if err == nil {
		if !same {
			return 0, ErrConflict
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if in.Item != "" {
		if err = json.Unmarshal(raw, &plan); err != nil {
			return 0, err
		}
		if _, ok := planItems(plan)[in.Item]; !ok {
			return 0, ErrConflict
		}
	}
	var pending, discussed bool
	err = tx.QueryRowContext(ctx, `SELECT (SELECT MAX(revision) FROM plan_revisions WHERE task_id=$1),
 EXISTS(SELECT 1 FROM goal_discussion_turns WHERE task_id=$1 AND status IN ('queued','running')),
 COALESCE((SELECT status='done' FROM goal_discussion_turns WHERE task_id=$1 AND NOT draft ORDER BY id DESC LIMIT 1),false)`, task).Scan(&latest, &pending, &discussed)
	if err != nil {
		return 0, err
	}
	if pending || (in.Draft && (!discussed || latest != in.Revision || (status != "PLANNING" && status != "ACTIVE"))) {
		return 0, ErrConflict
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO goal_discussion_turns(task_id,revision,client_id,item_key,message,draft) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		task, in.Revision, in.ClientID, in.Item, in.Message, in.Draft).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrConflict
	}
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

type PlannerDiscussion struct {
	Draft   bool            `json:"draft"`
	Context json.RawMessage `json:"context"`
}

func discussionContext(ctx context.Context, tx *sql.Tx, task int64, revision int, turn int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := tx.QueryRowContext(ctx, `SELECT jsonb_build_object('issue',t.spec->'issue','revision',r.revision,'plan',r.plan,
 'work_items', (SELECT jsonb_agg(jsonb_build_object('key',plan_key,'status',status) ORDER BY id) FROM work_items WHERE goal_id=t.id),
 'owner_request', (SELECT jsonb_build_object('message',message,'generate_draft',draft,'item',item_key) FROM goal_discussion_turns WHERE id=$3 AND task_id=t.id),
 'history',(SELECT jsonb_agg(to_jsonb(h) ORDER BY h.id) FROM (SELECT id,revision,item_key,message,draft,answer,generated_revision
 FROM goal_discussion_turns WHERE task_id=t.id AND status='done' AND id<$3) h))
 FROM work_items t JOIN plan_revisions r ON r.task_id=t.id WHERE t.id=$1 AND r.revision=$2`, task, revision, turn).Scan(&raw)
	return raw, err
}

// Claim a discussion under the same project lock and Worker capacity reservation
// as a normal execution. The Goal itself stays at its current active revision.
func claimGoalDiscussion(ctx context.Context, tx *sql.Tx, worker string, projects []int64, caps WorkerCapabilities) (*Job, error) {
	rows, err := tx.QueryContext(ctx, `SELECT d.id,t.project_id FROM goal_discussion_turns d JOIN work_items t ON t.id=d.task_id
 WHERE d.status='queued' AND ($1::bigint[] IS NULL OR t.project_id=ANY($1))
 AND NOT EXISTS(SELECT 1 FROM work_items w WHERE w.project_id=t.project_id AND (w.status='RUNNING' OR w.waiting_execution_id IS NOT NULL))
 ORDER BY d.id`, pq.Array(projects))
	if err != nil {
		return nil, err
	}
	var candidates []struct{ turn, project int64 }
	for rows.Next() {
		var c struct{ turn, project int64 }
		if err = rows.Scan(&c.turn, &c.project); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	// SaveAgentConfig uses the exclusive counterpart, so capability matching and
	// freezing the assignment see the same effective project/global configuration.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock_shared(132,hashtext('planner'))"); err != nil {
		return nil, err
	}
	def, _ := agents.Lookup("planner")
	for _, c := range candidates {
		entry, err := agentConfig(ctx, tx, c.project, def)
		if err != nil {
			return nil, err
		}
		if entry.Effective.Harness == "claude" {
			if !caps.SupportsClaude || !caps.ClaudePlanning {
				continue
			}
		} else if !caps.SupportsCodex || !caps.SupportsPlanner {
			continue
		}
		var source int64
		var revision int
		var spec []byte
		j := Job{Kind: "planner_discussion", ProjectID: c.project, Discussion: &PlannerDiscussion{}, RecordProcesses: true, ReportLiveProcess: true, RecordToolActivity: true, RecordModelOutput: true, RecordToolOutput: true, RecordToolStream: true, RecordAppTurn: true, PlannerHandoff: true}
		err = tx.QueryRowContext(ctx, `SELECT p.path,t.spec,t.id,d.revision,d.draft FROM goal_discussion_turns d JOIN work_items t ON t.id=d.task_id JOIN projects p ON p.id=t.project_id
 WHERE d.id=$1 AND d.status='queued' FOR UPDATE OF p,d SKIP LOCKED`, c.turn).Scan(&j.RepoPath, &spec, &source, &revision, &j.Discussion.Draft)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var busy bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM work_items WHERE project_id=$1 AND (status='RUNNING' OR waiting_execution_id IS NOT NULL))`, c.project).Scan(&busy); err != nil {
			return nil, err
		}
		if busy {
			continue
		}
		if err = json.Unmarshal(spec, &j.Spec); err != nil {
			return nil, err
		}
		j.Agent, err = freezeAgent(ctx, tx, c.project, "planner", worker, "", "")
		if err != nil {
			return nil, err
		}
		j.Spec.Mode, j.Spec.Harness, j.Spec.VerifyCommit = "plan", j.Agent.Harness, ""
		spec, err = json.Marshal(j.Spec)
		if err != nil {
			return nil, err
		}
		j.Discussion.Context, err = discussionContext(ctx, tx, source, revision, c.turn)
		if err != nil {
			return nil, err
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO work_items(project_id,kind,status,spec) VALUES($1,'planner_discussion','RUNNING',$2)
 RETURNING id,nextval(pg_get_serial_sequence('executions','id'))`, c.project, string(spec)).Scan(&j.TaskID, &j.ExecutionID)
		if err != nil {
			return nil, err
		}
		label := j.Agent.Harness
		if j.Agent.Model != "" {
			label += " · " + j.Agent.Model
		}
		_, err = tx.ExecContext(ctx, `UPDATE goal_discussion_turns SET status='running',worker_task_id=$2,model_name=$3 WHERE id=$1`, c.turn, j.TaskID, label)
		return &j, err
	}
	return nil, nil
}

func publishDiscussionReply(ctx context.Context, tx *sql.Tx, work int64, result Result) (string, error) {
	var goal int64
	var revision int
	var draft bool
	var level string
	err := tx.QueryRowContext(ctx, `SELECT d.task_id,d.revision,d.draft,COALESCE(r.minimum_level,'') FROM goal_discussion_turns d
 JOIN plan_revisions r ON (r.task_id,r.revision)=(d.task_id,d.revision) WHERE worker_task_id=$1 AND status='running' FOR UPDATE OF d`, work).Scan(&goal, &revision, &draft, &level)
	if err != nil {
		return "", err
	}
	if draft != (result.Plan != nil) {
		return "", ErrConflict
	}
	var generated any
	answer, status, workStatus := result.DiscussionAnswer, "done", "DONE"
	if draft {
		rev, err := savePlanTx(ctx, tx, goal, revision, *result.Plan, level, true)
		if errors.Is(err, ErrConflict) {
			answer, status, workStatus = "计划或工作项已变化，请核对最新修订后重新生成。", "failed", "FAILED"
		} else if err != nil {
			return "", err
		} else {
			generated = rev.Revision
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE goal_discussion_turns SET status=$2,answer=$3,generated_revision=$4 WHERE worker_task_id=$1`, work, status, answer, generated)
	return workStatus, err
}

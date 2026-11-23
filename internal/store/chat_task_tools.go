package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type TaskReference struct {
	By    string `json:"by"`
	Value string `json:"value"`
}

type ChatTaskMatch struct {
	TaskID      int64  `json:"task_id,omitempty"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	IssueNumber int64  `json:"issue_number,omitempty"`
}

type ChatTaskMatches struct {
	Result  string          `json:"result"`
	Matches []ChatTaskMatch `json:"matches"`
}

var ErrTaskReference = errors.New("invalid project task reference")

// Select from durable chat receipts and cited tasks. A later unrelated turn
// has no task reference and cannot displace the last meaningful mention.
func (s *Store) ResolveChatTask(ctx context.Context, project, turn int64, ref TaskReference) (ChatTaskMatches, error) {
	out := ChatTaskMatches{Result: "not_found", Matches: []ChatTaskMatch{}}
	var query string
	args := []any{project}
	const tasks = "SELECT t.id, left(" + taskSummaryTitle + ",180), t.status, COALESCE((t.spec->'issue'->>'number')::bigint,0) FROM work_items t WHERE t.project_id=$1 AND "
	const drafts = ` UNION ALL SELECT 0,left(label,180),'draft',issue_number FROM project_chat_actions
        WHERE project_id=$1 AND turn_id<$3 AND status='pending' AND kind IN ('dispatch_issue','create_issue') AND `
	switch ref.By {
	case "task_id", "issue_number":
		n, err := strconv.ParseInt(ref.Value, 10, 64)
		if err != nil || n < 1 {
			return out, ErrTaskReference
		}
		args = append(args, n)
		query = tasks + "t.id=$2"
		if ref.By == "issue_number" {
			query = tasks + "t.spec->'issue'->>'number'=$2::bigint::text AND t.goal_id IS NULL" + drafts + "issue_number=$2 ORDER BY 1 DESC"
			args = append(args, turn)
		}
	case "title":
		term := strings.TrimSpace(ref.Value)
		if term == "" || utf8.RuneCountInString(term) > 160 {
			return out, ErrTaskReference
		}
		args = append(args, term)
		query = tasks + "strpos(lower(" + taskSummaryTitle + "),lower($2))>0" + drafts + "strpos(lower(label),lower($2))>0 ORDER BY 1 DESC"
		args = append(args, turn)
	case "recent", "latest_dispatch":
		args = append(args, turn, ref.By)
		// All tasks cited in the same turn share rank. The caller must ask
		// which one the Owner meant when the latest mention has several matches.
		query = `WITH last_mention AS (
            SELECT h.id,h.created_at,h.citations,h.progress_task_ids FROM project_chat_turns h
            WHERE h.project_id=$1 AND h.id<$2 AND h.feature_id IS NOT DISTINCT FROM (SELECT feature_id FROM project_chat_turns WHERE id=$2 AND project_id=$1) AND h.status='completed' AND $3='recent' AND (cardinality(h.progress_task_ids)>0
                OR EXISTS (SELECT 1 FROM jsonb_array_elements(h.citations) c WHERE c->>'key' ~ '^task:[0-9]+$'))
            ORDER BY h.id DESC LIMIT 1
        ), refs AS (
            SELECT a.task_id,a.label,CASE WHEN a.status='pending' THEN 'draft' END AS draft,
                a.issue_number,COALESCE(a.confirmed_at,h.created_at) AS at,h.id AS turn_id
            FROM project_chat_actions a JOIN project_chat_turns h ON h.id=a.turn_id
            WHERE a.project_id=$1 AND h.id<$2 AND h.feature_id IS NOT DISTINCT FROM (SELECT feature_id FROM project_chat_turns WHERE id=$2 AND project_id=$1) AND a.kind IN ('dispatch_issue','create_issue')
              AND a.status IN ('confirmed','pending')
            UNION ALL
            SELECT t.id,'',NULL,0,h.created_at,h.id FROM last_mention h JOIN work_items t ON t.project_id=$1 AND
                (t.id=ANY(h.progress_task_ids) OR (cardinality(h.progress_task_ids)=0 AND EXISTS
                (SELECT 1 FROM jsonb_array_elements(h.citations) c WHERE c->>'key'='task:'||t.id::text)))
        ), ranked AS (
            SELECT *,dense_rank() OVER (ORDER BY CASE WHEN $3='latest_dispatch' AND draft IS NOT NULL THEN 1 ELSE 0 END,at DESC,turn_id DESC) AS priority FROM refs
        )
        SELECT DISTINCT COALESCE(t.id,0),left(CASE WHEN r.draft IS NULL THEN ` + taskSummaryTitle + ` ELSE r.label END,180),
           COALESCE(r.draft,t.status),COALESCE((t.spec->'issue'->>'number')::bigint,r.issue_number)
        FROM ranked r LEFT JOIN work_items t ON t.id=r.task_id AND t.project_id=$1
        WHERE priority=1 AND (r.draft IS NOT NULL OR t.id IS NOT NULL) ORDER BY 1`
	default:
		return out, ErrTaskReference
	}
	rows, err := s.db.QueryContext(ctx, query+" LIMIT 6", args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var m ChatTaskMatch
		if err := rows.Scan(&m.TaskID, &m.Title, &m.Status, &m.IssueNumber); err != nil {
			return out, err
		}
		out.Matches = append(out.Matches, m)
	}
	switch {
	case len(out.Matches) == 1:
		out.Result = "resolved"
		if out.Matches[0].TaskID == 0 {
			out.Result = "not_dispatched"
		}
	case len(out.Matches) > 1:
		out.Result = "ambiguous"
	}
	return out, rows.Err()
}

// A single database snapshot includes the goal and its current children. Read
// only public task fields, never workspaces, raw process output or credentials.
func (s *Store) ChatTaskProgress(ctx context.Context, project, id int64) (json.RawMessage, []ChatSource, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `WITH scope AS (
        SELECT t.* FROM work_items t WHERE t.project_id=$1 AND
            (t.id=$2 OR (t.goal_id=$2 AND t.status<>'SUPERSEDED'))
    ), questions AS (
        SELECT q.id,q.task_id,left(q.question,1000) AS question FROM decision_questions q JOIN scope t ON t.id=q.task_id
        WHERE t.status='WAITING_INPUT' AND q.answer IS NULL AND q.withdrawn_at IS NULL ORDER BY q.id DESC LIMIT 6
    ), dependencies AS (
        SELECT w.task_id,p.id AS prerequisite,p.status AS task_status,d.state AS delivery_status
        FROM scope t JOIN work_dependencies w ON w.task_id=t.id JOIN work_items p ON p.id=w.prerequisite AND p.project_id=$1
        LEFT JOIN deliveries d ON d.task_id=p.id
        WHERE t.status='BLOCKED' AND (p.status<>'DONE' OR d.state IS DISTINCT FROM 'DONE')
        ORDER BY w.task_id,p.id LIMIT 12
    )
    SELECT jsonb_strip_nulls(jsonb_build_object('task_id',t.id,'title',left(`+taskSummaryTitle+`,180),'status',t.status,
        'as_of',statement_timestamp(),'issue_number',t.spec->'issue'->'number','delivery_status',d.state,
        'latest_event',(SELECT jsonb_build_object('type',v.type,'at',v.at,'task_id',v.task_id) FROM task_events v JOIN scope x ON x.id=v.task_id ORDER BY v.at DESC,v.id DESC LIMIT 1),
        'latest_execution',(SELECT jsonb_strip_nulls(jsonb_build_object('worker',e.worker,'role',e.resource_kind,'exit_code',e.result->'exit_code',
            'code_review',NULLIF(`+codeReviewState+`,''),'code_review_required',(SELECT requires_code_review FROM projects WHERE id=t.project_id),
            'pending_owner_approval',EXISTS(SELECT 1 FROM live_command_approvals a WHERE a.execution_id=e.id AND a.decision IS NULL),
            'test_plan',tp.state,'test_execution',tp.execution_state,'validation_exit_code',e.result->'validation'->'exit_code','lease_expired',CASE WHEN t.status='RUNNING' AND e.result IS NULL AND e.abandoned_at IS NULL THEN e.lease_until<=now() END))
            FROM executions e LEFT JOIN code_reviews r ON r.execution_id=e.id
            LEFT JOIN LATERAL (SELECT state,execution_state FROM candidate_test_plan_status WHERE execution_id=e.id ORDER BY revision DESC LIMIT 1) tp ON true
            WHERE e.task_id=t.id ORDER BY e.id DESC LIMIT 1),
        'queued_behind_task',CASE WHEN t.status='READY' THEN (SELECT id FROM work_items WHERE project_id=$1 AND id<>t.id AND (status='RUNNING' OR waiting_execution_id IS NOT NULL) ORDER BY id LIMIT 1) END,
        'parent_goal_on_hold',CASE WHEN t.status='READY' THEN (SELECT status<>'ACTIVE' FROM work_items WHERE project_id=$1 AND id=t.goal_id) END,
        'pending_questions',COALESCE((SELECT jsonb_agg(to_jsonb(q)) FROM questions q),'[]'),
        'pending_dependencies',COALESCE((SELECT jsonb_agg(to_jsonb(p)) FROM dependencies p),'[]'),
        'children_by_status',(SELECT jsonb_object_agg(status,n) FROM (SELECT status,count(*) n FROM scope WHERE id<>t.id GROUP BY status) g),
        'children_needing_attention',(SELECT jsonb_agg(to_jsonb(c)) FROM (SELECT x.id AS task_id,left(x.spec->'issue'->>'title',160) AS title,x.status
            FROM scope x WHERE x.id<>t.id AND x.status IN ('WAITING_INPUT','FAILED','PAUSED','WAITING_ACCEPTANCE','BLOCKED') ORDER BY CASE x.status WHEN 'WAITING_INPUT' THEN 1 WHEN 'FAILED' THEN 2 ELSE 3 END,x.id LIMIT 6)c),
        'plan',(SELECT jsonb_build_object('revision',p.revision,'reviewed',p.review IS NOT NULL,'verdict',p.review->>'verdict','active_revision',t.active_revision) FROM plan_revisions p WHERE p.task_id=t.id ORDER BY p.revision DESC LIMIT 1)
    )) FROM scope t LEFT JOIN deliveries d ON d.task_id=t.id WHERE t.id=$2`, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	sources := []ChatSource{ChatTaskSource(id)}
	var info struct {
		Questions []struct {
			ID     int64
			TaskID int64 `json:"task_id"`
		} `json:"pending_questions"`
	}
	if err = json.Unmarshal(raw, &info); err != nil {
		return nil, nil, err
	}
	for _, q := range info.Questions {
		sources = append(sources, ChatSource{Key: fmt.Sprintf("question:%d", q.ID), Label: fmt.Sprintf("待答问题 #%d", q.ID), Path: fmt.Sprintf("/tasks/%d/questions/%d", q.TaskID, q.ID)})
	}
	return raw, sources, nil
}

// Register only server-generated links for the current turn. Never grant a
// worker permission to read a different project using model-supplied identifiers.
func (s *Store) AddChatToolEvidence(ctx context.Context, project, id int64, lease string, sources []ChatSource, progressID int64) error {
	raw, err := json.Marshal(sources)
	if err != nil {
		return err
	}
	return chatUpdated(s.db.ExecContext(ctx, `UPDATE project_chat_turns SET progress_task_ids=CASE WHEN $5::bigint>0 AND NOT $5=ANY(progress_task_ids) THEN array_append(progress_task_ids,$5) ELSE progress_task_ids END, evidence=evidence||COALESCE((
        SELECT jsonb_agg(item) FROM jsonb_array_elements($4::jsonb) item WHERE NOT evidence @> jsonb_build_array(item)), '[]')
        WHERE project_id=$1 AND id=$2 AND lease=$3 AND status='running' AND lease_until>now()`, project, id, lease, raw, progressID))
}

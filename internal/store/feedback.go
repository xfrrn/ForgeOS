package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"regexp"
	"strings"
)

type PreviewFeedback struct {
	Request     string   `json:"request_id"`
	Execution   int64    `json:"execution_id"`
	Commit      string   `json:"commit"`
	Path        string   `json:"path"`
	Browser     string   `json:"browser,omitempty"`
	Width       int      `json:"width,omitempty"`
	Height      int      `json:"height,omitempty"`
	Element     string   `json:"element,omitempty"`
	Frames      []string `json:"frames,omitempty"`
	CanvasPoint string   `json:"canvas_point,omitempty"`
	Note        string   `json:"note"`
	Revision    int      `json:"plan_revision,omitempty"`
	Case        string   `json:"case_id,omitempty"`
}

var feedbackRequest = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var feedbackBrowser = regexp.MustCompile(`^(Chrome|Chromium|Firefox|Safari|Edge)/[0-9]{1,3}$`)
var elementStep = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}:nth-of-type\([1-9][0-9]{0,5}\)$`)
var feedbackCase = regexp.MustCompile(`^TC-[0-9]{3}$`)

const percentage = `((0|[1-9][0-9]?)\.[0-9]{2}|100\.00)%`

var canvasPoint = regexp.MustCompile("^" + percentage + " " + percentage + "$")

func validLocator(locator string, target string) bool {
	if len(locator) > 2048 {
		return false
	}
	steps := strings.Split(locator, " > ")
	for _, step := range steps {
		if !elementStep.MatchString(step) {
			return false
		}
	}
	return target == "" || strings.HasPrefix(steps[len(steps)-1], target+":nth-of-type(")
}

func (f PreviewFeedback) Valid() bool {
	if !feedbackRequest.MatchString(f.Request) || f.Execution <= 0 || !validCommit(f.Commit) ||
		strings.TrimSpace(f.Note) == "" || len(f.Note) > 8192 || strings.ContainsRune(f.Note, 0) {
		return false
	}
	name, err := url.PathUnescape(f.Path)
	if err != nil || len(f.Path) > 2048 || !strings.HasPrefix(name, "/") || strings.ContainsAny(name, "?#\\\r\n\x00") {
		return false
	}
	relative := strings.TrimSuffix(strings.TrimPrefix(name, "/"), "/")
	if name != "/" && (relative == "." || !fs.ValidPath(relative)) {
		return false
	}
	if (f.Browser != "" && !feedbackBrowser.MatchString(f.Browser)) || (f.Width == 0) != (f.Height == 0) || f.Width < 0 || f.Width > 32768 || f.Height < 0 || f.Height > 32768 {
		return false
	}
	target := ""
	if f.CanvasPoint != "" {
		target = "canvas"
		if !canvasPoint.MatchString(f.CanvasPoint) {
			return false
		}
	}
	if (f.Element != "" || target != "" || len(f.Frames) > 0) && !validLocator(f.Element, target) {
		return false
	}
	for _, frame := range f.Frames {
		if !validLocator(frame, "iframe") {
			return false
		}
	}
	return (f.Revision == 0 && f.Case == "") || (f.Revision > 0 && f.Revision <= 2147483647 && feedbackCase.MatchString(f.Case))
}

func (s *Store) PreviewFeedbackPage(ctx context.Context, task int64, preview string) (json.RawMessage, error) {
	var out json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT jsonb_build_object('preview',to_jsonb(r),
        'repository',(SELECT spec->'issue'->>'repository' FROM work_items WHERE id=r.task_id),
        'latest_requirement',(SELECT max(v.revision) FROM work_items t JOIN requirement_revisions v ON v.task_id=COALESCE(t.goal_id,t.id) WHERE t.id=r.task_id),
        'at_acceptance',EXISTS(SELECT 1 FROM work_items t WHERE t.id=r.task_id AND t.status='WAITING_ACCEPTANCE'
             AND r.execution_id=(SELECT max(id) FROM executions WHERE task_id=t.id)),
        'feedback',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',f.id,'created_at',f.created_at,'author','owner',
            'state',CASE WHEN f.classified_at IS NOT NULL THEN 'classified' WHEN f.review_until>now() THEN 'in_review' ELSE 'awaiting_review' END,
            'classification',f.classification,'classified_at',f.classified_at,'body',f.body,
            'artifacts',COALESCE((SELECT jsonb_agg(to_jsonb(a)-'storage_key' ORDER BY a.id) FROM feedback_artifacts a WHERE a.feedback_id=f.id),'[]'),
            'work_item',(SELECT jsonb_build_object('id',t.id,'number',t.spec->'issue'->'number') FROM feedback_work_items l JOIN work_items t ON t.id=l.task_id WHERE l.feedback_id=f.id),
            'requirement',(SELECT jsonb_build_object('revision',v.revision,'task_id',v.task_id) FROM requirement_revisions v WHERE v.feedback_id=f.id),
            'review',(SELECT jsonb_build_object('decision',a.decision,'note',a.note) FROM acceptance_review_feedback l
                JOIN acceptance_reviews a USING(execution_id) WHERE l.feedback_id=f.id),
            'question',(SELECT to_jsonb(q) FROM decision_questions q WHERE q.feedback_id=f.id)) ORDER BY f.id DESC) FROM preview_feedback f WHERE f.preview_id=r.id),'[]'),
        'plan',(SELECT jsonb_build_object('revision',p.revision,'cases',p.plan->'cases') FROM candidate_test_plan_receipts p
                 WHERE p.execution_id=r.execution_id AND p.plan IS NOT NULL ORDER BY p.revision DESC LIMIT 1))
        FROM static_preview_receipts r WHERE r.task_id=$1 AND r.id=$2`, task, preview).Scan(&out)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return out, err
}

func (s *Store) SubmitPreviewFeedback(ctx context.Context, task int64, preview string, f PreviewFeedback) (int64, error) {
	if !f.Valid() {
		return 0, ErrConflict
	}
	body, err := json.Marshal(f)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO preview_feedback(preview_id,request_id,plan_id,body)
        SELECT r.id,$3::uuid,p.id,$4::jsonb FROM static_preview_receipts r
        LEFT JOIN candidate_test_plan_receipts p ON p.execution_id=r.execution_id AND p.revision=$7
        AND EXISTS(SELECT 1 FROM jsonb_array_elements(p.plan->'cases') c WHERE c->>'id'=$8)
        WHERE r.id=$1 AND r.task_id=$2 AND r.execution_id=$5 AND r.commit=$6 AND ($7=0 OR p.id IS NOT NULL)
        ON CONFLICT(preview_id,request_id) DO NOTHING
        RETURNING id`, preview, task, f.Request, string(body), f.Execution, f.Commit, f.Revision, f.Case).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type) VALUES($1,$2,'preview_feedback_received')`, task, f.Execution)
	} else if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `SELECT f.id FROM preview_feedback f JOIN static_preview_receipts r ON r.id=f.preview_id
			WHERE f.preview_id=$1 AND r.task_id=$2 AND f.request_id=$3 AND f.body=$4::jsonb`, preview, task, f.Request, string(body)).Scan(&id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrConflict
	}
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

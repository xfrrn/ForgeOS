package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Each design version is an immutable chain of turns under a single approved proposal.
func startFeatureDesign(ctx context.Context, tx *sql.Tx, round int64, version int, previous int64, feedback string, key uuid.UUID) error {
	var snapshot, approved string
	var lead int64
	err := tx.QueryRowContext(ctx, `SELECT p.snapshot::text,r.body || coalesce(E'\n拍板时的可视稿\n' || r.wireframe::text,''),(SELECT id FROM project_chat_turns WHERE proposal_id=p.id AND proposal_role='lead' AND status='completed' ORDER BY id DESC LIMIT 1)
 FROM feature_proposals p JOIN feature_proposal_revisions r ON r.proposal_id=p.id AND r.revision=p.approved_revision WHERE p.id=$1`, round).Scan(&snapshot, &approved, &lead)
	if err != nil {
		return err
	}
	prompt := "请基于拍板方案出一份设计稿，写明界面或组件位置、交互及文案、数据接口、异常情况、交付物与可测试的验收条目。保持在已确认范围内。可只读检索并查阅项目源码以核实组件、接口、路由与数据结构，严禁修改或创建任何文件，直接在聊天中交稿。只输出设计文档，限 1800 字。\n需求与依据\n" + snapshot + "\n拍板方案\n" + approved
	if previous != 0 {
		var final string
		var visual []byte
		if err = tx.QueryRowContext(ctx, "SELECT answer,design_wireframe FROM project_chat_turns WHERE id=$1 AND proposal_id=$2 AND proposal_role='design_final' AND status='completed'", previous, round).Scan(&final, &visual); err != nil {
			return proposalResult(err)
		}
		if len(visual) > 0 && !strings.Contains(final, "```forgeos-ui") {
			final += "\n前版可视稿\n" + string(visual)
		}
		prompt += "\n前版设计定稿\n" + final + "\nOwner 修订意见\n" + feedback
	}
	prompt += visualFormat
	label := "起草功能设计"
	if feedback != "" {
		label = feedback
	}
	return enqueueDesign(ctx, tx, round, version, "design_draft", label, prompt, lead, previous, key)
}

func enqueueDesign(ctx context.Context, tx *sql.Tx, round int64, version int, role, label, prompt string, profileTurn, parent int64, key uuid.UUID) error {
	if err := enqueueProposal(ctx, tx, round, role, label, prompt, uuid.Nil, profileTurn, parent, key); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE project_chat_turns SET design_version=$3 WHERE proposal_id=$1 AND client_id=$2", round, key, version)
	return err
}

func completeDesignTurn(ctx context.Context, tx *sql.Tx, round, turn int64, role string) error {
	var version int
	var current bool
	var approved, answer string
	err := tx.QueryRowContext(ctx, `SELECT t.design_version,t.answer,r.body,p.version=(SELECT max(version) FROM feature_proposals WHERE feature_id=p.feature_id)
 FROM project_chat_turns t JOIN feature_proposals p ON p.id=t.proposal_id JOIN feature_proposal_revisions r ON r.proposal_id=p.id AND r.revision=p.approved_revision WHERE t.id=$1`, turn).Scan(&version, &answer, &approved, &current)
	if err != nil || !current {
		return err
	}
	var profileTurn int64
	if role == "design_draft" {
		err = tx.QueryRowContext(ctx, `SELECT id FROM project_chat_turns WHERE proposal_id=$1 AND proposal_role='advisor' AND status='completed' ORDER BY id DESC LIMIT 1`, round).Scan(&profileTurn)
		if err != nil {
			return err
		}
		prompt := "请审阅下列设计稿是否落实拍板方案。如当前会话挂载了只读项目代码库，可只读查阅源码核实接口、组件与约束（严禁修改文件）。每条意见给出对应位置、交互或文案问题、影响及修改建议。检查接口和验收是否可验证。仅输出审阅意见，限 1200 字。\n拍板方案\n" + approved + "\n设计稿\n" + answer
		return enqueueDesign(ctx, tx, round, version, "design_review", "顾问审阅设计稿", prompt, profileTurn, turn, uuid.New())
	}
	var draft string
	err = tx.QueryRowContext(ctx, `SELECT id,answer FROM project_chat_turns WHERE proposal_id=$1 AND design_version=$2 AND proposal_role='design_draft' AND status='completed' ORDER BY id DESC LIMIT 1`, round, version).Scan(&profileTurn, &draft)
	if err != nil {
		return err
	}
	prompt := "请针对每条顾问意见写明处理结果和理由，并交付一份能直接派给开发的设计定稿。写清功能范围、界面交互、接口约定、交付物及逐项可测试的验收标准。可只读查阅项目源码核实细节，严禁修改或创建任何文件，直接在聊天中返回中文定稿，限 1800 字。\n已拍板方案\n" + approved + "\n设计稿\n" + draft + "\n顾问意见\n" + answer
	return enqueueDesign(ctx, tx, round, version, "design_final", "主控整理设计定稿", prompt+visualFormat, profileTurn, turn, uuid.New())
}

func approvedDesignAction(ctx context.Context, tx *sql.Tx, round int64, a ProposalAction) error {
	var version int
	var last int64
	var role, status string
	err := tx.QueryRowContext(ctx, `SELECT design_version,id,proposal_role,`+proposalStatus+` FROM project_chat_turns t WHERE proposal_id=$1 AND design_version IS NOT NULL ORDER BY design_version DESC,id DESC LIMIT 1`, round).Scan(&version, &last, &role, &status)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if a.Operation == "start_design" {
		if version != 0 {
			return nil
		}
		return startFeatureDesign(ctx, tx, round, 1, 0, "", uuid.New())
	}
	if a.Client == uuid.Nil || version == 0 {
		return ErrConflict
	}
	if a.Operation == "revise_design" && a.Element != "" {
		a.Body, err = anchoredDesignNote(ctx, tx, round, a)
		if err != nil {
			return err
		}
	}
	// Check the operation key before comparing the visible turn. A retry may already have advanced the chain.
	var replay bool
	err = tx.QueryRowContext(ctx, `SELECT proposal_parent=$3 AND ((proposal_role='design_draft' AND $4='revise_design' AND text=$5) OR ($4='retry_design' AND proposal_role=(SELECT proposal_role FROM project_chat_turns WHERE id=$3) AND model_profile_id=coalesce(nullif($6::uuid,'00000000-0000-0000-0000-000000000000'),(SELECT model_profile_id FROM project_chat_turns WHERE id=$3)))) FROM project_chat_turns WHERE proposal_id=$1 AND client_id=$2`, round, a.Client, a.Turn, a.Operation, a.Body, a.Profile).Scan(&replay)
	if err == nil {
		if replay {
			return nil
		}
		return ErrConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if a.Turn != last {
		return ErrConflict
	}
	switch a.Operation {
	case "retry_design":
		if status != "failed" {
			return ErrConflict
		}
		a.Operation = "retry"
		if err = proposalJobAction(ctx, tx, round, a); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE project_chat_turns SET design_version=$3 WHERE proposal_id=$1 AND client_id=$2", round, a.Client, version)
		return err
	case "revise_design":
		if role != "design_final" || status != "completed" || strings.TrimSpace(a.Body) == "" || len(a.Body) > 4000 {
			return ErrConflict
		}
		var dispatched bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM project_chat_actions WHERE turn_id=$1 AND status<>'stale')", last).Scan(&dispatched); err != nil {
			return err
		}
		if dispatched {
			return ErrConflict
		}
		return startFeatureDesign(ctx, tx, round, version+1, last, a.Body, a.Client)
	}
	return ErrConflict
}

type DesignDispatch struct {
	Steps    []ImplementationStep `json:"steps,omitempty"`
	Revision int                  `json:"revision"`
	Turn     int64                `json:"turn_id"`
	Issue    int64                `json:"issue_number"`
}

// Binding the action to the immutable final turn also binds the task receipt written by ConfirmChatAction.
func (s *Store) PrepareDesignDispatch(ctx context.Context, project, feature, round int64, in DesignDispatch, repo string, repoID int64, sourceDigest string, existing *Issue) (ChatAction, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ChatAction{}, err
	}
	defer tx.Rollback()
	if err = lockProposalTopic(ctx, tx, project, feature); err != nil {
		return ChatAction{}, err
	}
	if err := validateImplementationSteps(in.Steps); err != nil {
		return ChatAction{}, err
	}
	var body, title string
	var version, design int
	err = tx.QueryRowContext(ctx, `SELECT t.answer,f.title,p.version,t.design_version FROM project_chat_turns t JOIN feature_proposals p ON p.id=t.proposal_id JOIN feature_topics f ON f.id=p.feature_id
 WHERE t.id=$1 AND p.project_id=$2 AND p.feature_id=$3 AND p.id=$4 AND p.approved_revision=$5 AND t.proposal_role='design_final' AND t.status='completed'`, in.Turn, project, feature, round, in.Revision).Scan(&body, &title, &version, &design)
	if err != nil {
		return ChatAction{}, proposalResult(err)
	}
	var id int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM project_chat_actions WHERE turn_id=$1", in.Turn).Scan(&id)
	if err == nil {
		a, e := scanChatAction(tx.QueryRowContext(ctx, chatActionSelect, project, id))
		if e != nil {
			return a, e
		}
		if (a.Kind == "create_issue" && in.Issue != 0) || (a.Kind == "dispatch_issue" && in.Issue != a.IssueNumber) {
			return ChatAction{}, ErrConflict
		}
		if len(in.Steps) > 0 && !sameImplementationSteps(in.Steps, a.Steps) {
			return ChatAction{}, ErrConflict
		}
		return a, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ChatAction{}, err
	}
	var current bool
	err = tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM feature_proposals WHERE feature_id=$1 AND version>$2) AND NOT EXISTS(SELECT 1 FROM project_chat_turns WHERE proposal_id=$3 AND design_version>$4)`, feature, version, round, design).Scan(&current)
	if err != nil {
		return ChatAction{}, err
	}
	if !current || repoID <= 0 || repo == "" {
		return ChatAction{}, ErrConflict
	}
	kind, digest := "create_issue", ""
	title = "功能 · " + title
	if existing != nil {
		if existing.Number != in.Issue || existing.RepositoryID != repoID || (existing.WorkType != "" && existing.WorkType != "development") {
			return ChatAction{}, ErrConflict
		}
		kind, digest, title = "dispatch_issue", sourceDigest, existing.Title
	} else if in.Issue != 0 {
		return ChatAction{}, ErrConflict
	}
	if before, tail, ok := strings.Cut(body, "```forgeos-ui"); ok && parseWireframe(body, false) != nil {
		_, after, _ := strings.Cut(tail, "```")
		body = before + after
	}
	body = fmt.Sprintf("功能话题 #%d · 方案第 %d 轮 v%d · 设计 v%d\n\n%s", feature, version, in.Revision, design, strings.TrimSpace(body))
	if kind == "create_issue" && len(body) > 16<<10 {
		return ChatAction{}, ErrConflict
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO project_chat_actions(project_id,turn_id,kind,label,issue_body,work_type,repository,repository_id,issue_number,source_digest,implementation_steps,design_wireframe) VALUES($1,$2,$3,$4,$5,'development',$6,$7,$8,$9,$10,(SELECT design_wireframe FROM project_chat_turns WHERE id=$2)) RETURNING id`, project, in.Turn, kind, title, body, repo, repoID, in.Issue, digest, stepsJSON(in.Steps)).Scan(&id)
	if err != nil {
		return ChatAction{}, err
	}
	if err = tx.Commit(); err != nil {
		return ChatAction{}, err
	}
	return s.ChatAction(ctx, project, id)
}

type DesignReceipt struct {
	FinalTurn  int64           `json:"turn_id,omitempty"`
	Action     *ChatAction     `json:"action,omitempty"`
	Progress   json.RawMessage `json:"progress,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	Activities []ChatActivity  `json:"activities,omitempty"`
}

func (s *Store) DesignReceipt(ctx context.Context, project, feature, round int64) (DesignReceipt, error) {
	var r DesignReceipt
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT coalesce((SELECT a.id FROM project_chat_actions a JOIN project_chat_turns t ON t.id=a.turn_id WHERE t.proposal_id=p.id AND t.proposal_role='design_final' ORDER BY t.id DESC LIMIT 1),0) FROM feature_proposals p WHERE p.project_id=$1 AND p.feature_id=$2 AND p.id=$3`, project, feature, round).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil || id == 0 {
		return r, err
	}
	a, err := s.ChatAction(ctx, project, id)
	if err != nil {
		return r, err
	}
	r.Action = &a
	r.FinalTurn = a.TurnID
	if a.TaskID == 0 {
		return r, nil
	}
	r.Progress, _, err = s.ChatTaskProgress(ctx, project, a.TaskID)
	if err != nil {
		return r, err
	}
	turns := []ChatTurn{{Action: &a}}
	if err = s.ChatActivities(ctx, project, turns); err != nil {
		return r, err
	}
	r.Activities = turns[0].Activities
	err = s.db.QueryRowContext(ctx, `SELECT jsonb_strip_nulls(jsonb_build_object('execution',e.id,'inputs',e.feature_inputs,
 'steps',(SELECT jsonb_agg(s.spec->'feature_step'||jsonb_strip_nulls(jsonb_build_object('task',s.id,'status',s.status,'execution',x.id,'commit',x.result->>'commit','error',left(x.result->>'error',2000))) ORDER BY s.feature_position) FROM work_items s LEFT JOIN LATERAL (SELECT id,result FROM executions WHERE task_id=s.id ORDER BY id DESC LIMIT 1)x ON true WHERE s.feature_parent=t.id),'commit',e.result->>'commit','error',left(e.result->>'error',2000),'exit_code',e.result->'exit_code','validation_exit_code',e.result->'validation'->'exit_code',
 'acceptance',ar.decision,'acceptance_note',ar.note,'pr_number',nullif(d.pr_number,0),'merge_sha',nullif(d.merge_sha,''),'delivery',d.state,
 'reviews',(SELECT jsonb_agg(review ORDER BY execution_id DESC) FROM (SELECT execution_id,commit,decision,note FROM acceptance_reviews WHERE task_id=t.id ORDER BY execution_id DESC LIMIT 12)review),
 'previews',(SELECT jsonb_agg(p ORDER BY created_at DESC) FROM (SELECT p.id,p.state,p.created_at,p.execution_id,p.commit FROM static_preview_receipts p WHERE p.task_id=t.id ORDER BY p.created_at DESC LIMIT 8)p)))
 FROM work_items t LEFT JOIN LATERAL (SELECT id,result,feature_inputs FROM executions WHERE task_id=t.id ORDER BY id DESC LIMIT 1)e ON true LEFT JOIN acceptance_reviews ar ON ar.execution_id=e.id LEFT JOIN deliveries d ON d.task_id=t.id WHERE t.project_id=$1 AND t.id=$2`, project, a.TaskID).Scan(&r.Result)
	return r, err
}

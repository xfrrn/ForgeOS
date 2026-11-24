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

type ProposalMaterial struct {
	Key      string `json:"key"`
	Revision int    `json:"revision"`
}
type ProposalRequest struct {
	Client    uuid.UUID          `json:"client_id"`
	Lead      uuid.UUID          `json:"lead"`
	Advisor   uuid.UUID          `json:"advisor"`
	Brief     string             `json:"brief"`
	Expected  int                `json:"expected_version"`
	Materials []ProposalMaterial `json:"materials"`
}

func proposalResult(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	return err
}

func lockProposalTopic(ctx context.Context, tx *sql.Tx, project, feature int64) error {
	var id int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM feature_topics WHERE project_id=$1 AND id=$2 FOR NO KEY UPDATE", project, feature).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Store) StartFeatureProposal(ctx context.Context, project, feature int64, in ProposalRequest, sha string) (int64, error) {
	if in.Client == uuid.Nil || in.Lead == uuid.Nil || in.Advisor == uuid.Nil || in.Lead == in.Advisor || strings.TrimSpace(in.Brief) == "" || len(in.Brief) > 16384 || len(in.Materials) > 16 || in.Expected < 0 {
		return 0, ErrConflict
	}
	if in.Materials == nil {
		in.Materials = []ProposalMaterial{}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err = lockProposalTopic(ctx, tx, project, feature); err != nil {
		return 0, err
	}
	req, err := json.Marshal(in)
	if err != nil {
		return 0, err
	}
	var id int64
	var same bool
	err = tx.QueryRowContext(ctx, "SELECT id,request=$3::jsonb FROM feature_proposals WHERE feature_id=$1 AND client_id=$2", feature, in.Client, req).Scan(&id, &same)
	if err == nil {
		if !same {
			return 0, ErrConflict
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	refs, err := json.Marshal(in.Materials)
	if err != nil {
		return 0, err
	}
	var snapshot string
	err = tx.QueryRowContext(ctx, `WITH materials AS (
 SELECT m.key,m.revision,m.title,m.content,m.content_sha256 FROM project_materials m WHERE m.project_id=$1 AND (m.key,m.revision) IN (SELECT key,revision FROM jsonb_to_recordset($4::jsonb) x(key text,revision int))
 ), turns AS (SELECT id,text,answer,requested_model AS model,requested_harness AS harness FROM project_chat_turns WHERE feature_id=$2 AND proposal_id IS NULL AND assist_kind='' AND status='completed' ORDER BY id DESC LIMIT 24),
 notes AS (SELECT id,turn_id,kind,body FROM feature_notes WHERE feature_id=$2 AND NOT closed)
 INSERT INTO feature_proposals(project_id,feature_id,version,client_id,request,snapshot)
 SELECT f.project_id,f.id,$8::int+1,$6,$7,jsonb_build_object('title',f.title,'brief',$3::text,'head',$5::text,
 'materials',coalesce((SELECT jsonb_agg(m ORDER BY key,revision) FROM materials m),'[]'),'discussion',coalesce((SELECT jsonb_agg(t ORDER BY id) FROM turns t),'[]'),
 'notes',coalesce((SELECT jsonb_agg(n ORDER BY id) FROM notes n),'[]'))
 FROM feature_topics f WHERE f.project_id=$1 AND f.id=$2 AND (SELECT count(*) FROM materials)=jsonb_array_length($4)
 AND $8=coalesce((SELECT max(version) FROM feature_proposals WHERE feature_id=f.id),0) RETURNING id,snapshot::text`, project, feature, in.Brief, refs, sha, in.Client, req, in.Expected).Scan(&id, &snapshot)
	if err != nil {
		return 0, proposalResult(err)
	}
	if len(snapshot) > 512*1024 {
		return 0, ErrConflict
	}
	prompt := "根据以下冻结的需求和资料，独立提出功能方案。如果当前会话挂载了只读项目代码仓库，请先检索并阅读相关源码核实现有架构、接口与约束，再给出落地方案（引用核实过的文件路径与符号；严格只读，严禁修改或创建任何文件）。说明主要取舍、验收标准和待决定事项。仅输出中文方案，限 1800 字。\n" + snapshot
	for _, part := range []struct {
		role, label string
		profile     uuid.UUID
	}{{"lead", "主控独立提案", in.Lead}, {"advisor", "顾问独立提案", in.Advisor}} {
		if err = enqueueProposal(ctx, tx, id, part.role, part.label, prompt, part.profile, 0, 0, uuid.New()); err != nil {
			return 0, err
		}
	}
	// Superseded design work can no longer advance to dispatch.
	_, err = tx.ExecContext(ctx, `UPDATE project_chat_turns SET cancel_requested=true,
        status=CASE WHEN status='queued' THEN 'failed' ELSE status END,
        error=CASE WHEN status='queued' THEN '需求已有新轮次，本轮设计已停止。' ELSE error END
        WHERE feature_id=$1 AND proposal_id<>$2 AND design_version IS NOT NULL AND status IN ('queued','running')`, feature, id)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// A copied profile keeps the exact native identity or provider revision used for the original draft.
func enqueueProposal(ctx context.Context, tx *sql.Tx, round int64, role, label, prompt string, profile uuid.UUID, copyTurn, parent int64, client uuid.UUID) error {
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO project_chat_turns(project_id,feature_id,client_id,text,proposal_id,proposal_role,proposal_prompt,proposal_parent,requested_model,requested_harness,model_profile_id,model_revision,model_profile_name,native_client)
 SELECT p.project_id,p.feature_id,$8,$3,p.id,$2,$4,nullif($7::bigint,0),m.model,m.backend,m.id,m.revision,m.model,m.native FROM feature_proposals p
 CROSS JOIN LATERAL (SELECT id,model,'codex' AS backend,revision,NULL::uuid AS native FROM chat_models WHERE $6::bigint=0 AND id=$5 AND revision IS NOT NULL
 UNION ALL SELECT id,model,backend,NULL,id FROM feature_clients WHERE $6=0 AND id=$5 AND project_id=p.project_id AND seen_at>now()-interval '3 minutes'
 UNION ALL SELECT model_profile_id,requested_model,requested_harness,model_revision,native_client FROM project_chat_turns WHERE id=$6 AND proposal_id=p.id) m
 WHERE p.id=$1 ON CONFLICT(project_id,client_id) DO NOTHING RETURNING id`, round, role, label, prompt, profile, copyTurn, parent, client).Scan(&id)
	return proposalResult(err)
}

func completeProposalTurn(ctx context.Context, tx *sql.Tx, turn int64) error {
	var round int64
	var role, snapshot, answer string
	err := tx.QueryRowContext(ctx, `SELECT p.id,t.proposal_role,p.snapshot::text,t.answer FROM project_chat_turns t JOIN feature_proposals p ON p.id=t.proposal_id WHERE t.id=$1 FOR NO KEY UPDATE OF p`, turn).Scan(&round, &role, &snapshot, &answer)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if role == "design_draft" || role == "design_final" || role == "design_visual" {
		if err = saveWireframe(ctx, tx, round, turn, role, answer); err != nil {
			return err
		}
	}
	switch role {
	case "design_draft", "design_review":
		return completeDesignTurn(ctx, tx, round, turn, role)
	case "comparison":
		_, err = tx.ExecContext(ctx, `INSERT INTO feature_proposal_revisions(proposal_id,revision,body,author,source_turn) VALUES($1,1,$2,'主控',$3)`, round, answer, turn)
		return err
	case "lead", "advisor":
		var lead int64
		var first, second string
		err = tx.QueryRowContext(ctx, `SELECT l.id,l.answer,a.answer FROM project_chat_turns l JOIN project_chat_turns a ON l.proposal_id=a.proposal_id
 WHERE l.proposal_id=$1 AND l.proposal_role='lead' AND a.proposal_role='advisor' AND l.status='completed' AND a.status='completed'
 AND NOT EXISTS(SELECT 1 FROM project_chat_turns WHERE proposal_id=$1 AND proposal_role='comparison')`, round).Scan(&lead, &first, &second)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		prompt := fmt.Sprintf("逐项比较两份原稿。每项注明采用主控、顾问或双方及原因，说明未采纳意见和理由。如有需要可只读查阅项目源码核实分歧点（严禁修改文件）。列出待决定事项、综合方案和验收标准。仅输出中文，限 2200 字。\n需求快照\n%s\n主控原稿\n%s\n顾问原稿\n%s", snapshot, first, second)
		return enqueueProposal(ctx, tx, round, "comparison", "比较两案并综合", prompt, uuid.Nil, lead, 0, uuid.New())
	}
	return nil
}

const proposalStatus = `CASE WHEN status='running' AND lease_until<=now() THEN CASE WHEN attempts>=3 OR t.created_at<=now()-interval '30 minutes' THEN 'failed' ELSE 'queued' END WHEN status='queued' AND t.created_at<=now()-interval '30 minutes' THEN 'failed' ELSE status END`

func (s *Store) FeatureProposalList(ctx context.Context, project, feature int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT coalesce(jsonb_agg(x ORDER BY version DESC),'[]') FROM (SELECT id,version,approved_revision FROM feature_proposals WHERE project_id=$1 AND feature_id=$2) x`, project, feature).Scan(&raw)
	return raw, err
}
func (s *Store) FeatureProposal(ctx context.Context, project, feature, id int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT jsonb_build_object('id',p.id,'version',p.version,'snapshot',p.snapshot,'approved_revision',p.approved_revision,'approved_at',p.approved_at,'approved_by',p.approved_by,
 'jobs',coalesce((SELECT jsonb_agg(j ORDER BY id) FROM (SELECT id,design_version,visual_revision,design_wireframe AS wireframe,proposal_role AS role,proposal_parent AS parent,model_profile_id AS profile,requested_model AS model,CASE WHEN native_client IS NOT NULL THEN requested_harness ELSE 'api' END AS backend,text,answer,error,`+proposalStatus+` AS status FROM project_chat_turns t WHERE proposal_id=p.id) j),'[]'),
 'revisions',coalesce((SELECT jsonb_agg(r ORDER BY revision DESC) FROM (SELECT revision,body,wireframe,source_turn,author,created_at FROM feature_proposal_revisions WHERE proposal_id=p.id) r),'[]'))
 FROM feature_proposals p WHERE p.project_id=$1 AND p.feature_id=$2 AND p.id=$3`, project, feature, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return raw, err
}

type ProposalAction struct {
	Element   string    `json:"element_id,omitempty"`
	Operation string    `json:"operation"`
	Turn      int64     `json:"turn_id"`
	Client    uuid.UUID `json:"client_id"`
	Profile   uuid.UUID `json:"profile_id"`
	Body      string    `json:"body"`
	Revision  int       `json:"revision"`
}

func (s *Store) ActOnProposal(ctx context.Context, project, feature, round int64, a ProposalAction) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockProposalTopic(ctx, tx, project, feature); err != nil {
		return err
	}
	if a.Operation == "generate_visual" {
		var matched int64
		err = tx.QueryRowContext(ctx, "SELECT id FROM feature_proposals WHERE id=$1 AND feature_id=$2 AND project_id=$3 AND approved_revision=$4 FOR NO KEY UPDATE", round, feature, project, a.Revision).Scan(&matched)
		if err != nil {
			return proposalResult(err)
		}
		if err = requestWireframe(ctx, tx, round, a); err != nil {
			return err
		}
		return tx.Commit()
	}
	var approved, latest int
	err = tx.QueryRowContext(ctx, `SELECT coalesce(approved_revision,0),coalesce((SELECT max(revision) FROM feature_proposal_revisions WHERE proposal_id=p.id),0)
 FROM feature_proposals p WHERE p.id=$1 AND p.feature_id=$2 AND version=(SELECT max(version) FROM feature_proposals WHERE feature_id=$2) FOR NO KEY UPDATE`, round, feature).Scan(&approved, &latest)
	if err != nil {
		return proposalResult(err)
	}
	if approved != 0 {
		if a.Revision == approved && (a.Operation == "start_design" || a.Operation == "retry_design" || a.Operation == "revise_design") {
			if err = approvedDesignAction(ctx, tx, round, a); err != nil {
				return err
			}
			return tx.Commit()
		}
		if a.Operation == "approve" && a.Revision == approved {
			return nil
		}
		return ErrConflict
	}
	switch a.Operation {
	case "preview_proposal":
		if latest != a.Revision || a.Client == uuid.Nil {
			return ErrConflict
		}
		source, body, raw, e := proposalVisualSource(ctx, tx, round, latest)
		if e != nil {
			return e
		}
		if raw == nil {
			err = enqueueWireframe(ctx, tx, round, source, latest, a.Client, a.Profile, body)
		}
	case "approve", "revise":
		if a.Operation == "revise" && a.Element != "" {
			_, body, raw, e := proposalVisualSource(ctx, tx, round, a.Revision)
			if e != nil {
				return e
			}
			note, e := elementNote(raw, a.Revision, "方案", a)
			if e != nil {
				return e
			}
			a.Body = body + "\n\nOwner 最新修订要求\n" + note
		}
		if a.Operation == "revise" && a.Client != uuid.Nil {
			var replay bool
			err = tx.QueryRowContext(ctx, "SELECT revision=$3+1 AND body=$4 FROM feature_proposal_revisions WHERE proposal_id=$1 AND client_id=$2", round, a.Client, a.Revision, a.Body).Scan(&replay)
			if err == nil {
				if replay {
					return nil
				}
				return ErrConflict
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if latest < 1 || latest != a.Revision {
			return ErrConflict
		}
		if a.Operation == "approve" {
			_, err = tx.ExecContext(ctx, `UPDATE feature_proposals SET approved_revision=$2,approved_at=now(),approved_by='Owner' WHERE id=$1`, round, latest)
			if err == nil {
				err = startFeatureDesign(ctx, tx, round, 1, 0, "", uuid.New())
			}
		} else {
			if strings.TrimSpace(a.Body) == "" || len(a.Body) > 32000 {
				return ErrConflict
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO feature_proposal_revisions(proposal_id,revision,source_turn,body,author,client_id) SELECT proposal_id,revision+1,source_turn,$3,'Owner',nullif($4::uuid,'00000000-0000-0000-0000-000000000000') FROM feature_proposal_revisions WHERE proposal_id=$1 AND revision=$2`, round, latest, a.Body, a.Client)
			if err == nil && a.Element != "" {
				source, body, _, e := proposalVisualSource(ctx, tx, round, latest+1)
				if e != nil {
					return e
				}
				err = enqueueWireframe(ctx, tx, round, source, latest+1, uuid.New(), uuid.Nil, body)
			}
		}
	case "retry", "ask":
		err = proposalJobAction(ctx, tx, round, a)
	default:
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func proposalJobAction(ctx context.Context, tx *sql.Tx, round int64, a ProposalAction) error {
	if a.Client == uuid.Nil || a.Turn < 1 {
		return ErrConflict
	}
	var role, label, prompt, status, answer, snapshot string
	err := tx.QueryRowContext(ctx, `SELECT t.proposal_role,t.text,t.proposal_prompt,`+proposalStatus+`,t.answer,p.snapshot::text FROM project_chat_turns t JOIN feature_proposals p ON p.id=t.proposal_id WHERE p.id=$1 AND t.id=$2`, round, a.Turn).Scan(&role, &label, &prompt, &status, &answer, &snapshot)
	if err != nil {
		return proposalResult(err)
	}
	copyTurn := a.Turn
	if a.Operation == "ask" {
		if status != "completed" || strings.TrimSpace(a.Body) == "" || len(a.Body) > 8192 || a.Profile != uuid.Nil {
			return ErrConflict
		}
		role = "followup"
		prompt = fmt.Sprintf("根据冻结需求及所指回合回答追问（可只读查阅项目源码核实细节，严禁修改文件），限 1200 字。\n需求\n%s\n该回合问题\n%s\n该回合原文\n%s\n追问\n%s", snapshot, label, answer, a.Body)
		label = a.Body
		if len(prompt) > 1024*1024 {
			return ErrConflict
		}
	} else {
		if status != "failed" {
			return ErrConflict
		}
		if a.Profile != uuid.Nil {
			var sameParticipant bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM project_chat_turns WHERE proposal_id=$1 AND model_profile_id=$2 AND (($3='lead' AND proposal_role='advisor') OR ($3='advisor' AND proposal_role='lead')))`, round, a.Profile, role).Scan(&sameParticipant)
			if err != nil {
				return err
			}
			if sameParticipant {
				return ErrConflict
			}
			copyTurn = 0
		}
	}
	var same bool
	err = tx.QueryRowContext(ctx, `SELECT proposal_parent=$3 AND proposal_role=$4 AND text=$5 AND model_profile_id=coalesce(nullif($6::uuid,'00000000-0000-0000-0000-000000000000'),(SELECT model_profile_id FROM project_chat_turns WHERE id=$3)) FROM project_chat_turns WHERE proposal_id=$1 AND client_id=$2`, round, a.Client, a.Turn, role, label, a.Profile).Scan(&same)
	if err == nil {
		if same {
			return nil
		}
		return ErrConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if a.Operation == "retry" {
		var repeated bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM project_chat_turns WHERE proposal_parent=$1 AND proposal_role=$2)", a.Turn, role).Scan(&repeated); err != nil {
			return err
		}
		if repeated {
			return ErrConflict
		}
	}
	return enqueueProposal(ctx, tx, round, role, label, prompt, a.Profile, copyTurn, a.Turn, a.Client)
}

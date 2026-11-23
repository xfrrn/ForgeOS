package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// The model supplies content and layout roles. The browser owns every rendered element.
type designWireframe struct {
	Title   string `json:"title"`
	Regions []struct {
		Label    string `json:"label"`
		Position string `json:"position"`
		Layout   string `json:"layout,omitempty"`
		Elements []struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Label string `json:"label"`
			Value string `json:"value,omitempty"`
		} `json:"elements"`
	} `json:"regions"`
}

const visualFormat = "\n在文末另附一个 forgeos-ui 围栏代码块，内容为一个 JSON 对象。按设计中的实际位置和文案组织界面，不添加未确认的功能。格式示例：{\"title\":\"页面名称\",\"regions\":[{\"position\":\"main\",\"label\":\"区域名\",\"elements\":[{\"id\":\"search\",\"kind\":\"input\",\"label\":\"控件名称\",\"value\":\"占位文案或示例内容\"}]}]}。position 可取 header/sidebar/main/footer，同一行的控件将区域 layout 设为 row，纵向排布用 column。kind 可取 input/button/text/list/status/tabs。每个区域含 1–12 项，最多 8 个区域。id 是全图唯一的简短英文标识，以字母开头，只含字母、数字、下划线和连字符。列表行或选项可在 value 中按换行分隔。纯后台功能用实际的输入、处理和结果做流程示意。"

var visualID = regexp.MustCompile("^[A-Za-z][A-Za-z0-9_-]{0,47}$")

func parseWireframe(answer string, standalone bool) *designWireframe {
	const marker = "```forgeos-ui"
	if _, tail, ok := strings.Cut(answer, marker); ok {
		block, _, closed := strings.Cut(tail, "```")
		if !closed {
			return nil
		}
		answer = block
	} else if standalone {
		answer = strings.TrimSpace(answer)
		if strings.HasPrefix(answer, "```json") {
			answer = strings.TrimSuffix(strings.TrimPrefix(answer, "```json"), "```")
		}
	} else {
		return nil
	}
	var frame designWireframe
	if len(answer) > 24<<10 || strings.Contains(answer, `\u0000`) || json.Unmarshal([]byte(answer), &frame) != nil || strings.TrimSpace(frame.Title) == "" || utf8.RuneCountInString(frame.Title) > 120 || len(frame.Regions) < 1 || len(frame.Regions) > 8 {
		return nil
	}
	ids := map[string]bool{}
	for _, region := range frame.Regions {
		switch region.Position {
		case "header", "sidebar", "main", "footer":
		default:
			return nil
		}
		if region.Layout != "" && region.Layout != "row" && region.Layout != "column" {
			return nil
		}
		if strings.TrimSpace(region.Label) == "" || utf8.RuneCountInString(region.Label) > 100 || len(region.Elements) < 1 || len(region.Elements) > 12 {
			return nil
		}
		for _, el := range region.Elements {
			switch el.Kind {
			case "input", "button", "text", "list", "status", "tabs":
			default:
				return nil
			}
			if !visualID.MatchString(el.ID) || ids[el.ID] || strings.TrimSpace(el.Label) == "" || utf8.RuneCountInString(el.Label) > 120 || utf8.RuneCountInString(el.Value) > 1000 {
				return nil
			}
			ids[el.ID] = true
		}
	}
	return &frame
}

func saveWireframe(ctx context.Context, tx *sql.Tx, round, turn int64, role, answer string) error {
	visual := role == "design_visual"
	frame := parseWireframe(answer, visual)
	if frame == nil {
		return nil
	}
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if visual {
		result, err := tx.ExecContext(ctx, `UPDATE feature_proposal_revisions r SET wireframe=$3 FROM project_chat_turns t
        WHERE t.id=$2 AND t.proposal_id=$1 AND r.proposal_id=$1 AND r.revision=t.visual_revision AND r.wireframe IS NULL AND NOT EXISTS(SELECT 1 FROM feature_proposals WHERE id=$1 AND approved_revision=r.revision)`, round, turn, string(data))
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil || n > 0 {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE project_chat_turns SET design_wireframe=$3
 WHERE proposal_id=$1 AND id=(SELECT CASE WHEN proposal_role='design_visual' THEN proposal_parent ELSE id END FROM project_chat_turns WHERE id=$2)
 AND proposal_role IN ('design_draft','design_final') AND design_wireframe IS NULL`, round, turn, string(data))
	return err
}

// Old versions can acquire a wireframe without changing their text or starting a revision.
func requestWireframe(ctx context.Context, tx *sql.Tx, round int64, a ProposalAction) error {
	if a.Client == uuid.Nil {
		return ErrConflict
	}
	var answer string
	var ready bool
	err := tx.QueryRowContext(ctx, `SELECT answer,design_wireframe IS NOT NULL FROM project_chat_turns
 WHERE proposal_id=$1 AND id=$2 AND proposal_role IN ('design_draft','design_final') AND status='completed' FOR UPDATE`, round, a.Turn).Scan(&answer, &ready)
	if err != nil {
		return proposalResult(err)
	}
	if ready {
		return nil
	}
	return enqueueWireframe(ctx, tx, round, a.Turn, 0, a.Client, a.Profile, answer)
}

func enqueueWireframe(ctx context.Context, tx *sql.Tx, round, source int64, revision int, client, profile uuid.UUID, answer string) error {
	var key uuid.UUID
	var status string
	err := tx.QueryRowContext(ctx, `SELECT client_id,`+proposalStatus+` FROM project_chat_turns t WHERE proposal_id=$1 AND proposal_parent=$2 AND proposal_role='design_visual' AND coalesce(visual_revision,0)=$3 ORDER BY id DESC LIMIT 1`, round, source, revision).Scan(&key, &status)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if key == client || status == "running" || status == "queued" {
		return nil
	}
	copyTurn := source
	if profile != uuid.Nil {
		copyTurn = 0
	}
	if err = enqueueProposal(ctx, tx, round, "design_visual", "生成此版界面示意", "请立即把以下文档转换为界面数据。唯一交付物是含实际控件的完整 JSON，直接把代码块写入本回合最终答复或计划正文。不要描述输出计划，不生成 HTML，不操作文件。"+visualFormat+"\n设计原文\n"+answer, profile, copyTurn, source, client); err != nil {
		return err
	}
	if revision != 0 {
		_, err = tx.ExecContext(ctx, "UPDATE project_chat_turns SET visual_revision=$3 WHERE proposal_id=$1 AND client_id=$2", round, client, revision)
	}
	return err
}

func anchoredDesignNote(ctx context.Context, tx *sql.Tx, round int64, a ProposalAction) (string, error) {
	var raw []byte
	var version int
	err := tx.QueryRowContext(ctx, `SELECT design_version,design_wireframe FROM project_chat_turns WHERE proposal_id=$1 AND id=$2 AND proposal_role='design_final' AND status='completed'`, round, a.Turn).Scan(&version, &raw)
	if err != nil {
		return "", proposalResult(err)
	}
	return elementNote(raw, version, "设计", a)
}
func elementNote(raw []byte, version int, stage string, a ProposalAction) (string, error) {
	var frame designWireframe
	if strings.TrimSpace(a.Body) == "" || json.Unmarshal(raw, &frame) != nil {
		return "", ErrConflict
	}
	for _, r := range frame.Regions {
		for _, e := range r.Elements {
			if e.ID == a.Element {
				return fmt.Sprintf("%s v%d · %s / %s [%s]\n%s", stage, version, r.Label, e.Label, e.ID, strings.TrimSpace(a.Body)), nil
			}
		}
	}
	return "", ErrConflict
}

func proposalVisualSource(ctx context.Context, tx *sql.Tx, round int64, revision int) (int64, string, []byte, error) {
	var source int64
	var body string
	var raw []byte
	err := tx.QueryRowContext(ctx, "SELECT source_turn,body,wireframe FROM feature_proposal_revisions WHERE proposal_id=$1 AND revision=$2", round, revision).Scan(&source, &body, &raw)
	return source, body, raw, proposalResult(err)
}

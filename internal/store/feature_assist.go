package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

type NoteSuggestion struct {
	TurnID int64  `json:"turn_id"`
	Kind   string `json:"kind"`
	Body   string `json:"body"`
}

type CloseNoteSuggestion struct {
	ID     int64  `json:"id"`
	Body   string `json:"body"`
	Reason string `json:"reason"`
}

type FeatureAssistJob struct {
	ID               int64                 `json:"id"`
	Kind             string                `json:"kind"`
	TargetTurn       int64                 `json:"target_turn,omitempty"`
	Status           string                `json:"status"`
	Error            string                `json:"error,omitempty"`
	Suggestions      []NoteSuggestion      `json:"suggestions,omitempty"`
	CloseSuggestions []CloseNoteSuggestion `json:"close_suggestions,omitempty"`
}

type FeatureBriefState struct {
	Brief            string            `json:"brief"`
	UpdatedAt        *time.Time        `json:"updated_at,omitempty"`
	SourceTurn       int64             `json:"source_turn,omitempty"`
	LatestTurn       int64             `json:"latest_turn,omitempty"`
	ActiveAgreements int               `json:"active_agreements"`
	OpenQuestions    int               `json:"open_questions"`
	Job              *FeatureAssistJob `json:"job,omitempty"`
}

type BatchFeatureNoteItem struct {
	Turn   int64     `json:"turn_id"`
	Client uuid.UUID `json:"client_id"`
	Kind   string    `json:"kind"`
	Body   string    `json:"body"`
}

type BatchFeatureNotesInput struct {
	Items    []BatchFeatureNoteItem `json:"items"`
	CloseIDs []int64                `json:"close_ids"`
}

func trimRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func stripOuterFence(raw string) string {
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	nl := strings.IndexByte(s, '\n')
	if nl < 0 {
		return s
	}
	rest := strings.TrimSpace(s[nl+1:])
	if strings.HasSuffix(rest, "```") {
		return strings.TrimSpace(strings.TrimSuffix(rest, "```"))
	}
	return s
}

func extractJSONObject(raw string) string {
	s := stripOuterFence(raw)
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

type rawAssistNotes struct {
	Notes []struct {
		TurnID int64  `json:"turn_id"`
		Kind   string `json:"kind"`
		Body   string `json:"body"`
	} `json:"notes"`
	CloseNotes []struct {
		ID     int64  `json:"id"`
		Reason string `json:"reason"`
	} `json:"close_notes"`
}

func parseAssistNotes(raw string, defaultTurn int64, validTurns map[int64]bool, activeNotes map[int64]FeatureNote) ([]NoteSuggestion, []CloseNoteSuggestion, error) {
	var parsed rawAssistNotes
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &parsed); err != nil {
		return nil, nil, errors.New("AI 返回的提炼结果格式无法解析，请重试。")
	}
	suggestions := make([]NoteSuggestion, 0, len(parsed.Notes))
	seenBody := map[string]bool{}
	for _, item := range parsed.Notes {
		kind := strings.TrimSpace(item.Kind)
		if kind != "agreement" && kind != "question" {
			continue
		}
		body := trimRunes(strings.ReplaceAll(item.Body, "\x00", ""), 1200)
		if body == "" || seenBody[kind+"\x00"+body] {
			continue
		}
		turnID := item.TurnID
		if turnID <= 0 || (len(validTurns) > 0 && !validTurns[turnID]) {
			turnID = defaultTurn
		}
		if turnID <= 0 {
			continue
		}
		seenBody[kind+"\x00"+body] = true
		suggestions = append(suggestions, NoteSuggestion{TurnID: turnID, Kind: kind, Body: body})
		if len(suggestions) >= 8 {
			break
		}
	}
	closes := make([]CloseNoteSuggestion, 0, len(parsed.CloseNotes))
	seenClose := map[int64]bool{}
	for _, item := range parsed.CloseNotes {
		note, ok := activeNotes[item.ID]
		if !ok || seenClose[item.ID] {
			continue
		}
		seenClose[item.ID] = true
		closes = append(closes, CloseNoteSuggestion{
			ID:     note.ID,
			Body:   note.Body,
			Reason: trimRunes(strings.ReplaceAll(item.Reason, "\x00", ""), 300),
		})
	}
	return suggestions, closes, nil
}

func enqueueAssistJob(ctx context.Context, tx *sql.Tx, project, feature int64, client, profile uuid.UUID, requestedModel, kind, label, prompt string, targetTurn int64) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO project_chat_turns(project_id,feature_id,client_id,text,assist_kind,assist_target_turn,proposal_prompt,requested_model,requested_harness,model_profile_id,model_revision,model_profile_name,native_client)
        SELECT f.project_id,f.id,$3,$4,$5,NULLIF($6::bigint,0),$7,m.effective_model,m.backend,m.id,m.revision,m.effective_model,m.native
        FROM feature_topics f
        CROSS JOIN (
            SELECT id,model AS effective_model,'codex' AS backend,revision,NULL::uuid AS native
            FROM chat_models WHERE id=$8 AND revision IS NOT NULL AND ($9='' OR model=$9)
            UNION ALL
            SELECT id,CASE WHEN $9<>'' THEN $9 ELSE model END AS effective_model,backend,NULL,id
            FROM feature_clients WHERE id=$8 AND project_id=$1 AND seen_at>now()-interval '3 minutes'
              AND ($9='' OR model=$9 OR EXISTS(SELECT 1 FROM jsonb_array_elements(available_models) elem WHERE elem->>'id'=$9))
        ) m
        WHERE f.project_id=$1 AND f.id=$2
        ON CONFLICT(project_id,client_id) DO NOTHING RETURNING id`,
		project, feature, client, label, kind, targetTurn, prompt, profile, requestedModel).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `SELECT id FROM project_chat_turns WHERE project_id=$1 AND feature_id=$2 AND client_id=$3 AND assist_kind=$4`,
			project, feature, client, kind).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrConflict
		}
	}
	return id, err
}

func (s *Store) StartNoteExtraction(ctx context.Context, project, feature, turnID int64, client, profile uuid.UUID, requestedModel string) (FeatureAssistJob, error) {
	if client == uuid.Nil || profile == uuid.Nil {
		return FeatureAssistJob{}, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FeatureAssistJob{}, err
	}
	defer tx.Rollback()
	if err = lockProposalTopic(ctx, tx, project, feature); err != nil {
		return FeatureAssistJob{}, err
	}
	type turnData struct {
		ID     int64  `json:"turn_id"`
		Owner  string `json:"owner"`
		Answer string `json:"reply"`
	}
	var turns []turnData
	var kind, label, prompt string
	var targetTurn int64

	activeNotes := []FeatureNote{}
	noteRows, err := tx.QueryContext(ctx, `SELECT id,turn_id,kind,body,closed FROM feature_notes WHERE project_id=$1 AND feature_id=$2 AND NOT closed ORDER BY id`, project, feature)
	if err != nil {
		return FeatureAssistJob{}, err
	}
	for noteRows.Next() {
		var n FeatureNote
		if err := noteRows.Scan(&n.ID, &n.Turn, &n.Kind, &n.Body, &n.Closed); err != nil {
			noteRows.Close()
			return FeatureAssistJob{}, err
		}
		activeNotes = append(activeNotes, n)
	}
	noteRows.Close()
	if err := noteRows.Err(); err != nil {
		return FeatureAssistJob{}, err
	}

	if turnID > 0 {
		var t turnData
		err = tx.QueryRowContext(ctx, `SELECT id,text,answer FROM project_chat_turns
			WHERE project_id=$1 AND feature_id=$2 AND id=$3 AND proposal_id IS NULL AND assist_kind='' AND status='completed'`,
			project, feature, turnID).Scan(&t.ID, &t.Owner, &t.Answer)
		if errors.Is(err, sql.ErrNoRows) {
			return FeatureAssistJob{}, ErrNotFound
		}
		if err != nil {
			return FeatureAssistJob{}, err
		}
		t.Owner = trimRunes(t.Owner, 2000)
		t.Answer = trimRunes(t.Answer, 4000)
		turns = []turnData{t}
		kind = "turn_notes"
		label = fmt.Sprintf("AI 提炼回合 #%d 要点", turnID)
		targetTurn = turnID
		payload, _ := json.Marshal(map[string]any{"turn": t, "existing_active_notes": activeNotes})
		prompt = "请阅读下面这一轮功能讨论，帮 Owner 提炼值得沉淀进「话题记录」的关键条目。\n" +
			"规则：\n" +
			"1. 只提取明确达成的共识/约束（kind 为 \"agreement\"）或尚未拍板、需要后续解决的开放问题（kind 为 \"question\"）；\n" +
			"2. 对照 existing_active_notes 去重，已经记录过的不要重复输出；\n" +
			"3. 最多输出 4 条，每条用精炼清晰的中文写完整一句话（<= 160 字）；如果没有值得记录的条目，返回空数组；\n" +
			"4. 仅依据提供的对话内容整理，不要使用工具或读写文件；\n" +
			"5. 严格只输出一个 JSON 对象，不要包含额外解释，格式：{\"notes\":[{\"kind\":\"agreement\",\"body\":\"...\"}]}\n" +
			string(payload)
	} else {
		rows, err := tx.QueryContext(ctx, `SELECT id,text,answer FROM project_chat_turns
			WHERE project_id=$1 AND feature_id=$2 AND proposal_id IS NULL AND assist_kind='' AND status='completed'
			ORDER BY id DESC LIMIT 24`, project, feature)
		if err != nil {
			return FeatureAssistJob{}, err
		}
		for rows.Next() {
			var t turnData
			if err := rows.Scan(&t.ID, &t.Owner, &t.Answer); err != nil {
				rows.Close()
				return FeatureAssistJob{}, err
			}
			t.Owner = trimRunes(t.Owner, 1500)
			t.Answer = trimRunes(t.Answer, 2500)
			turns = append(turns, t)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return FeatureAssistJob{}, err
		}
		if len(turns) == 0 {
			return FeatureAssistJob{}, ErrConflict
		}
		slices.Reverse(turns)
		targetTurn = turns[len(turns)-1].ID
		kind = "topic_notes"
		label = "AI 梳理近期讨论要点"
		payload, _ := json.Marshal(map[string]any{"recent_turns": turns, "existing_active_notes": activeNotes})
		prompt = "请梳理下面功能话题的近期讨论与当前有效「话题记录」，帮 Owner 提炼新增记录并识别已解决的旧问题。\n" +
			"规则：\n" +
			"1. notes 中列出讨论中已明确但尚未记入 existing_active_notes 的约定（kind=\"agreement\"）与仍待决定的问题（kind=\"question\"），每条必须标注最相关的 turn_id，最多 6 条，每条精炼中文（<= 160 字）；\n" +
			"2. close_notes 中列出 existing_active_notes 里已经通过后续讨论得出结论或不再适用的旧记录 ID 及一句简短理由；如果没有则返回空数组；\n" +
			"3. 仅依据提供的对话与记录整理，不要使用工具或读写文件；\n" +
			"4. 严格只输出一个 JSON 对象，不要包含额外解释，格式：{\"notes\":[{\"turn_id\":12,\"kind\":\"agreement\",\"body\":\"...\"}],\"close_notes\":[{\"id\":5,\"reason\":\"...\"}]}\n" +
			string(payload)
	}

	id, err := enqueueAssistJob(ctx, tx, project, feature, client, profile, requestedModel, kind, label, prompt, targetTurn)
	if err != nil {
		return FeatureAssistJob{}, err
	}
	if err = tx.Commit(); err != nil {
		return FeatureAssistJob{}, err
	}
	return s.FeatureAssistJob(ctx, project, feature, id)
}

func (s *Store) FeatureAssistJob(ctx context.Context, project, feature, id int64) (FeatureAssistJob, error) {
	var job FeatureAssistJob
	var target sql.NullInt64
	var answer string
	err := s.db.QueryRowContext(ctx, `SELECT t.id,t.assist_kind,t.assist_target_turn,`+proposalStatus+`,t.answer,t.error
		FROM project_chat_turns t WHERE t.project_id=$1 AND t.feature_id=$2 AND t.id=$3 AND t.assist_kind<>''`,
		project, feature, id).Scan(&job.ID, &job.Kind, &target, &job.Status, &answer, &job.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return FeatureAssistJob{}, ErrNotFound
	}
	if err != nil {
		return FeatureAssistJob{}, err
	}
	if target.Valid {
		job.TargetTurn = target.Int64
	}
	if job.Status == "failed" && job.Error == "" {
		job.Error = "AI 未能在等待期限内完成梳理，请重试。"
	}
	if job.Status == "completed" && (job.Kind == "turn_notes" || job.Kind == "topic_notes") {
		validTurns := map[int64]bool{}
		if job.TargetTurn > 0 {
			validTurns[job.TargetTurn] = true
		}
		if job.Kind == "topic_notes" {
			rows, err := s.db.QueryContext(ctx, `SELECT id FROM project_chat_turns WHERE project_id=$1 AND feature_id=$2 AND proposal_id IS NULL AND assist_kind='' AND status='completed'`, project, feature)
			if err == nil {
				for rows.Next() {
					var tid int64
					if rows.Scan(&tid) == nil {
						validTurns[tid] = true
					}
				}
				rows.Close()
			}
		}
		activeMap := map[int64]FeatureNote{}
		if notes, err := s.FeatureNotes(ctx, project, feature, true); err == nil {
			for _, n := range notes {
				activeMap[n.ID] = n
			}
		}
		suggestions, closes, parseErr := parseAssistNotes(answer, job.TargetTurn, validTurns, activeMap)
		if parseErr != nil {
			job.Status = "failed"
			job.Error = parseErr.Error()
		} else {
			job.Suggestions = suggestions
			job.CloseSuggestions = closes
		}
	}
	return job, nil
}

func (s *Store) BatchFeatureNotes(ctx context.Context, project, feature int64, in BatchFeatureNotesInput) (int, int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	if err = lockProposalTopic(ctx, tx, project, feature); err != nil {
		return 0, 0, err
	}
	saved := 0
	for _, item := range in.Items {
		body := strings.TrimSpace(item.Body)
		if item.Turn <= 0 || item.Client == uuid.Nil || (item.Kind != "agreement" && item.Kind != "question") || body == "" || strings.ContainsRune(body, 0) || utf8.RuneCountInString(body) > 1200 {
			return 0, 0, ErrConflict
		}
		var id int64
		err := tx.QueryRowContext(ctx, `INSERT INTO feature_notes(project_id,feature_id,turn_id,client_id,kind,body)
			SELECT project_id,feature_id,id,$4,$5,$6 FROM project_chat_turns
			WHERE project_id=$1 AND feature_id=$2 AND id=$3 AND proposal_id IS NULL AND assist_kind='' AND status='completed'
			ON CONFLICT(project_id,feature_id,client_id) DO NOTHING RETURNING id`,
			project, feature, item.Turn, item.Client, item.Kind, body).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx, `SELECT id FROM feature_notes WHERE project_id=$1 AND feature_id=$2 AND turn_id=$3 AND client_id=$4 AND kind=$5 AND body=$6`,
				project, feature, item.Turn, item.Client, item.Kind, body).Scan(&id)
			if errors.Is(err, sql.ErrNoRows) {
				return 0, 0, ErrConflict
			}
		}
		if err != nil {
			return 0, 0, err
		}
		saved++
	}
	closed := 0
	for _, cid := range in.CloseIDs {
		if cid <= 0 {
			continue
		}
		res, err := tx.ExecContext(ctx, `UPDATE feature_notes SET closed=true WHERE project_id=$1 AND feature_id=$2 AND id=$3 AND NOT closed`, project, feature, cid)
		if err != nil {
			return 0, 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			closed++
		}
	}
	return saved, closed, tx.Commit()
}

func (s *Store) FeatureBrief(ctx context.Context, project, feature int64) (FeatureBriefState, error) {
	var st FeatureBriefState
	var updated sql.NullTime
	var source, latest sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT f.brief,f.brief_updated_at,f.brief_source_turn,
		(SELECT max(id) FROM project_chat_turns WHERE project_id=f.project_id AND feature_id=f.id AND proposal_id IS NULL AND assist_kind='' AND status='completed'),
		(SELECT count(*) FROM feature_notes WHERE project_id=f.project_id AND feature_id=f.id AND NOT closed AND kind='agreement'),
		(SELECT count(*) FROM feature_notes WHERE project_id=f.project_id AND feature_id=f.id AND NOT closed AND kind='question')
		FROM feature_topics f WHERE f.project_id=$1 AND f.id=$2`, project, feature).
		Scan(&st.Brief, &updated, &source, &latest, &st.ActiveAgreements, &st.OpenQuestions)
	if errors.Is(err, sql.ErrNoRows) {
		return FeatureBriefState{}, ErrNotFound
	}
	if err != nil {
		return FeatureBriefState{}, err
	}
	if updated.Valid {
		t := updated.Time
		st.UpdatedAt = &t
	}
	if source.Valid {
		st.SourceTurn = source.Int64
	}
	if latest.Valid {
		st.LatestTurn = latest.Int64
	}
	var j FeatureAssistJob
	var target sql.NullInt64
	err = s.db.QueryRowContext(ctx, `SELECT t.id,t.assist_kind,t.assist_target_turn,`+proposalStatus+`,t.error
		FROM project_chat_turns t WHERE t.project_id=$1 AND t.feature_id=$2 AND t.assist_kind='brief'
		ORDER BY t.id DESC LIMIT 1`, project, feature).Scan(&j.ID, &j.Kind, &target, &j.Status, &j.Error)
	if err == nil {
		if target.Valid {
			j.TargetTurn = target.Int64
		}
		if j.Status == "failed" && j.Error == "" {
			j.Error = "生成需求终稿超时，请重试。"
		}
		st.Job = &j
	} else if !errors.Is(err, sql.ErrNoRows) {
		return FeatureBriefState{}, err
	}
	return st, nil
}

func (s *Store) SaveFeatureBrief(ctx context.Context, project, feature int64, brief string) (FeatureBriefState, error) {
	brief = strings.TrimSpace(brief)
	res, err := s.db.ExecContext(ctx, `UPDATE feature_topics f SET brief=$3,brief_updated_at=now(),
		brief_source_turn=COALESCE((SELECT max(id) FROM project_chat_turns WHERE project_id=f.project_id AND feature_id=f.id AND proposal_id IS NULL AND assist_kind='' AND status='completed'),f.brief_source_turn)
		WHERE f.project_id=$1 AND f.id=$2`, project, feature, brief)
	if err != nil {
		return FeatureBriefState{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return FeatureBriefState{}, ErrNotFound
	}
	return s.FeatureBrief(ctx, project, feature)
}

func (s *Store) StartFeatureBriefGeneration(ctx context.Context, project, feature int64, client, profile uuid.UUID, requestedModel string) (FeatureAssistJob, error) {
	if client == uuid.Nil || profile == uuid.Nil {
		return FeatureAssistJob{}, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FeatureAssistJob{}, err
	}
	defer tx.Rollback()
	if err = lockProposalTopic(ctx, tx, project, feature); err != nil {
		return FeatureAssistJob{}, err
	}
	var title, prevBrief string
	if err = tx.QueryRowContext(ctx, `SELECT title,brief FROM feature_topics WHERE project_id=$1 AND id=$2`, project, feature).Scan(&title, &prevBrief); err != nil {
		return FeatureAssistJob{}, err
	}
	type turnData struct {
		ID     int64  `json:"turn_id"`
		Owner  string `json:"owner"`
		Answer string `json:"reply"`
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,text,answer FROM project_chat_turns
		WHERE project_id=$1 AND feature_id=$2 AND proposal_id IS NULL AND assist_kind='' AND status='completed'
		ORDER BY id DESC LIMIT 24`, project, feature)
	if err != nil {
		return FeatureAssistJob{}, err
	}
	var turns []turnData
	for rows.Next() {
		var t turnData
		if err := rows.Scan(&t.ID, &t.Owner, &t.Answer); err != nil {
			rows.Close()
			return FeatureAssistJob{}, err
		}
		t.Owner = trimRunes(t.Owner, 1500)
		t.Answer = trimRunes(t.Answer, 2500)
		turns = append(turns, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return FeatureAssistJob{}, err
	}
	var notes []FeatureNote
	noteRows, err := tx.QueryContext(ctx, `SELECT id,turn_id,kind,body,closed FROM feature_notes WHERE project_id=$1 AND feature_id=$2 AND NOT closed ORDER BY id`, project, feature)
	if err != nil {
		return FeatureAssistJob{}, err
	}
	for noteRows.Next() {
		var n FeatureNote
		if err := noteRows.Scan(&n.ID, &n.Turn, &n.Kind, &n.Body, &n.Closed); err != nil {
			noteRows.Close()
			return FeatureAssistJob{}, err
		}
		notes = append(notes, n)
	}
	noteRows.Close()
	if err := noteRows.Err(); err != nil {
		return FeatureAssistJob{}, err
	}
	if len(turns) == 0 && len(notes) == 0 {
		return FeatureAssistJob{}, ErrConflict
	}
	slices.Reverse(turns)
	var latestTurn int64
	if len(turns) > 0 {
		latestTurn = turns[len(turns)-1].ID
	}
	payload, _ := json.Marshal(map[string]any{
		"feature_title":  title,
		"previous_brief": prevBrief,
		"active_notes":   notes,
		"recent_turns":   turns,
	})
	prompt := "请基于以下功能话题的讨论记录和全部有效「话题记录」，为 Owner 整理一份可直接用于「方案与拍板」阶段的《本轮需求终稿》。\n" +
		"要求：\n" +
		"1. 严格使用中文 Markdown 输出，控制在 3200 字以内，不要输出寒暄、开场白或外层代码块；\n" +
		"2. 按以下 5 个二级标题清晰组织：\n" +
		"   ## 背景与目标\n" +
		"   ## 功能范围与已确认约束\n" +
		"   ## 关键交互与代码现状\n" +
		"   ## 待方案阶段权衡的问题\n" +
		"   ## 初步验收标准\n" +
		"3. 必须吸收 active_notes 中所有已确认约定（agreement），并把尚未关闭的待决定问题（question）明确列在「待方案阶段权衡的问题」中；如果 previous_brief 非空，请保留其中仍有效的 Owner 修订，并用最新讨论更新；\n" +
		"4. 保留讨论中已经定位到的真实文件路径、报错现象与边界条件；如无必要无需再调用写操作，严格只读。\n" +
		string(payload)

	id, err := enqueueAssistJob(ctx, tx, project, feature, client, profile, requestedModel, "brief", "整理本轮需求终稿", prompt, latestTurn)
	if err != nil {
		return FeatureAssistJob{}, err
	}
	if err = tx.Commit(); err != nil {
		return FeatureAssistJob{}, err
	}
	return s.FeatureAssistJob(ctx, project, feature, id)
}

func completeAssistTurn(ctx context.Context, tx *sql.Tx, turn int64) error {
	var project, feature int64
	var kind, answer string
	var target sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT project_id,COALESCE(feature_id,0),assist_kind,assist_target_turn,answer
		FROM project_chat_turns WHERE id=$1`, turn).Scan(&project, &feature, &kind, &target, &answer)
	if errors.Is(err, sql.ErrNoRows) || kind != "brief" || feature == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	cleaned := trimRunes(stripOuterFence(answer), 4000)
	if cleaned == "" {
		return nil
	}
	_, err = tx.ExecContext(ctx, `UPDATE feature_topics SET brief=$3,brief_updated_at=now(),
		brief_source_turn=COALESCE($4,brief_source_turn) WHERE project_id=$1 AND id=$2`,
		project, feature, cleaned, target)
	return err
}

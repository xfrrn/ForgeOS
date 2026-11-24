package store

import "testing"

func TestParseAssistNotes(t *testing.T) {
	raw := "以下是整理结果：\n```json\n{\"notes\":[{\"turn_id\":10,\"kind\":\"agreement\",\"body\":\"导出接口统一返回 UTF-8 CSV\"},{\"turn_id\":999,\"kind\":\"question\",\"body\":\"是否需要限制单次导出行数？\"},{\"turn_id\":10,\"kind\":\"invalid\",\"body\":\"ignored\"}],\"close_notes\":[{\"id\":3,\"reason\":\"已在第 10 轮确认\"},{\"id\":99,\"reason\":\"不存在\"}]}\n```"
	validTurns := map[int64]bool{10: true, 12: true}
	active := map[int64]FeatureNote{
		3: {ID: 3, Kind: "question", Body: "导出编码用什么？"},
	}
	notes, closes, err := parseAssistNotes(raw, 12, validTurns, active)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 valid notes, got %+v", notes)
	}
	if notes[0].TurnID != 10 || notes[0].Kind != "agreement" || notes[0].Body != "导出接口统一返回 UTF-8 CSV" {
		t.Fatalf("unexpected note[0]: %+v", notes[0])
	}
	if notes[1].TurnID != 12 || notes[1].Kind != "question" {
		t.Fatalf("expected fallback turn 12 for invalid turn_id, got %+v", notes[1])
	}
	if len(closes) != 1 || closes[0].ID != 3 || closes[0].Body != "导出编码用什么？" || closes[0].Reason != "已在第 10 轮确认" {
		t.Fatalf("unexpected close suggestions: %+v", closes)
	}
	if _, _, err := parseAssistNotes("not json at all", 12, validTurns, active); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestStripOuterFence(t *testing.T) {
	got := stripOuterFence("```markdown\n## 背景与目标\n内容\n```")
	if got != "## 背景与目标\n内容" {
		t.Fatalf("unexpected stripped fence: %q", got)
	}
}

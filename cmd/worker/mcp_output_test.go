package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestMCPResultPreservesPublicBlocksAndRedactsStoredOutput(t *testing.T) {
	_, p, path := archiveTestProcess(t)
	result := json.RawMessage(`{"content":[{"type":"text","text":"Page one.\nprivate-node-credential"},{"type":"image","data":"private-image"},{"type":"text","text":"Page two."},{"type":"resource","resource":{"text":"private-resource"}}],"structuredContent":{"receipt":9007199254740993,"data":[{"value":"public","_meta":{"note":"private-nested"}}], "attachment":{"type":"audio","data":"private-audio"}},"_meta":{"note":"private-transport"}}`)
	event := func(status string, result, failure json.RawMessage) []byte {
		body, err := json.Marshal(map[string]any{"item": map[string]any{"id": "one", "type": "mcp_tool_call", "status": status, "result": result, "error": failure, "arguments": "private-input"}})
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	if err := p.codexTool("item.started", event("in_progress", nil, nil)); err != nil {
		t.Fatal(err)
	}
	if err := p.codexTool("item.completed", event("completed", result, nil)); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := processLogsCommand([]string{"export", "--journal", path, "--instance", "1"}, &archive); err != nil {
		t.Fatal(err)
	}
	text := archive.String()
	for _, want := range []string{"Page one.", "Page two.", "[redacted]", "public", "9007199254740993", `"verified":true`} {
		if !strings.Contains(text, want) {
			t.Fatal(want, text)
		}
	}
	if strings.Contains(text, "private-") {
		t.Fatal(text)
	}
	if got := mcpToolOutput(json.RawMessage(`{"structured_content":{"receipt":"public-receipt"}}`), nil); got != `{"receipt":"public-receipt"}` {
		t.Fatal(got)
	}
	if got := mcpToolOutput(json.RawMessage(`{"content":[{"type":"text","text":"hidden","annotations":{"audience":["assistant"]}},{"type":"text","text":"visible","annotations":{"audience":["user"]}}],"structuredContent":{"annotations":{"audience":["assistant"]},"value":"hidden"}}`), nil); got != "visible" {
		t.Fatal(got)
	}
	if got := mcpToolOutput(nil, json.RawMessage(`{"message":"Permission required","_meta":"private-error"}`)); got != "Permission required" {
		t.Fatal(got)
	}
}

package main

import (
	"encoding/json"
	"strings"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

type toolCall struct {
	number int64
	tool   string
	stream *toolStream
}

type toolRecorder struct {
	seq     int64
	pending map[string]toolCall
}

func (p *processLog) recordTool(id, name, phase, output string) error {
	if id == "" || len(id) > 256 {
		return nil
	}
	r := p.tools
	call, found := r.pending[id]
	if found {
		if phase == "started" || (name != "" && name != call.tool) {
			return nil
		}
	} else {
		if !store.KnownTool(p.harness, name) {
			return nil
		}
		call = toolCall{number: r.seq + 1, tool: name}
	}
	seq := r.seq + 1
	var safe, archive string
	if call.stream != nil {
		if phase != "output" {
			if err := call.stream.append(p, "", true); err != nil {
				return err
			}
		}
		safe = call.stream.text
		archive = call.stream.archive
	} else if p.toolOutput != nil && phase != "started" && output != "" {
		var err error
		archive, err = p.redactOutput(p.toolOutput, "tool", output)
		if err != nil {
			return err
		}
		safe = validation.DisplayLogTail([]byte(archive), nil)
	}
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO harness_tool_events(instance,seq,call,tool,phase) VALUES(?,?,?,?,?)", p.id, seq, call.number, call.tool, phase); err != nil {
		return err
	}
	if safe != "" {
		if _, err = tx.Exec(`INSERT INTO harness_tool_output(instance,seq,call,tool,text) VALUES(?,?,?,?,?)
		 ON CONFLICT(instance) DO UPDATE SET seq=excluded.seq,call=excluded.call,tool=excluded.tool,text=excluded.text`, p.id, seq, call.number, call.tool, safe); err != nil {
			return err
		}
		if err = p.saveOutputState(tx, "tool", p.toolKeyOpen()); err != nil {
			return err
		}
	}
	if err = p.archiveOutput(tx, "tool", call.number, call.tool, phase, archive); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if call.stream != nil {
		call.stream.archive = ""
	}
	r.seq = seq
	if phase == "started" || phase == "output" {
		r.pending[id] = call
	} else {
		delete(r.pending, id)
	}
	p.notify()
	return nil
}

func (p *processLog) codexTool(kind string, raw []byte) error {
	var e struct {
		Item struct {
			ID, Type, Status string
			Changes          []nativeFileChange
			Result, Error    json.RawMessage
			Results          json.RawMessage
			Output           string `json:"aggregated_output"`
		}
	}
	if json.Unmarshal(raw, &e) != nil || !store.KnownTool("codex", e.Item.Type) {
		return nil
	}
	phase := "started"
	if kind == "item.completed" {
		phase = "completed"
		if e.Item.Status == "failed" || e.Item.Status == "declined" {
			phase = "failed"
		}
	}
	output := e.Item.Output
	if e.Item.Type == "file_change" && phase != "started" && p.toolOutput != nil {
		output = fileChangeOutput(e.Item.Changes)
	}
	if e.Item.Type == "mcp_tool_call" && phase != "started" && p.toolOutput != nil {
		output = mcpToolOutput(e.Item.Result, e.Item.Error)
	}
	if e.Item.Type == "web_search" && phase != "started" && p.toolOutput != nil {
		output = webSearchOutput(e.Item.Results)
	}
	return p.recordTool(e.Item.ID, e.Item.Type, phase, output)
}

func webSearchOutput(results json.RawMessage) string {
	var records []json.RawMessage
	if json.Unmarshal(results, &records) != nil {
		return ""
	}
	var entries []string
	for _, record := range records {
		var r struct{ Type, Title, Domain, Snippet string }
		if json.Unmarshal(record, &r) != nil || r.Type != "text_result" {
			continue
		}
		var lines []string
		for _, field := range []string{r.Title, r.Domain, r.Snippet} {
			if field != "" {
				lines = append(lines, field)
			}
		}
		if len(lines) > 0 {
			entries = append(entries, strings.Join(lines, "\n"))
		}
	}
	return strings.Join(entries, "\n\n")
}

func (p *processLog) claudeTool(kind string, raw []byte) error {
	if p.tools == nil {
		return nil
	}
	var e struct {
		Message struct {
			Content []struct {
				Type, ID, Name string
				ToolUseID      string `json:"tool_use_id"`
				IsError        bool   `json:"is_error"`
				Content        json.RawMessage
			}
		}
	}
	if json.Unmarshal(raw, &e) != nil {
		return nil
	}
	for _, block := range e.Message.Content {
		var err error
		switch {
		case kind == "assistant" && block.Type == "tool_use":
			err = p.recordTool(block.ID, block.Name, "started", "")
		case kind == "user" && block.Type == "tool_result":
			phase := "completed"
			if block.IsError {
				phase = "failed"
			}
			var output string
			if p.toolOutput != nil && json.Unmarshal(block.Content, &output) != nil {
				var blocks []struct{ Type, Text string }
				if json.Unmarshal(block.Content, &blocks) == nil {
					var parts []string
					for _, part := range blocks {
						if part.Type == "text" {
							parts = append(parts, part.Text)
						}
					}
					output = strings.Join(parts, "\n")
				}
			}
			err = p.recordTool(block.ToolUseID, "", phase, output)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

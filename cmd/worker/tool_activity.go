package main

import (
	"encoding/json"
	"strings"

	"forgeos/internal/store"
)

type toolCall struct {
	number int64
	tool   string
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
		call = toolCall{r.seq + 1, name}
	}
	seq := r.seq + 1
	var safe string
	if p.toolOutput != nil && phase != "started" && output != "" {
		var err error
		safe, err = p.redactOutput(p.toolOutput, "tool", output)
		if err != nil {
			return err
		}
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
		if err = p.saveOutputState(tx, "tool", p.toolOutput.privateKey); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	r.seq = seq
	if phase == "started" {
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
	return p.recordTool(e.Item.ID, e.Item.Type, phase, e.Item.Output)
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

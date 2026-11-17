package store

import (
	"strings"
	"time"

	"forgeos/internal/validation"
)

type ModelOutput struct {
	Sequence int64  `json:"sequence"`
	Text     string `json:"text"`
}

type ToolOutput struct {
	Seq  int64  `json:"seq"`
	Call int64  `json:"call"`
	Tool string `json:"tool"`
	Text string `json:"text"`
}

type HarnessProcess struct {
	Instance    int64        `json:"instance"`
	PID         int          `json:"pid"`
	Harness     string       `json:"harness"`
	Version     string       `json:"version"`
	Session     string       `json:"session_id,omitempty"`
	HandoffFrom string       `json:"handoff_from,omitempty"`
	Started     time.Time    `json:"started_at"`
	Exited      *time.Time   `json:"exited_at,omitempty"`
	ExitCode    *int         `json:"exit_code"`
	Tools       []ToolEvent  `json:"tools,omitempty"`
	Output      *ModelOutput `json:"output,omitempty"`
	ToolOutput  *ToolOutput  `json:"tool_output,omitempty"`
}

func (p HarnessProcess) Valid() bool {
	if !p.validToolOutput() {
		return false
	}
	if p.Output != nil && (p.Session == "" || p.Output.Sequence <= 0 || p.Output.Text == "" || len(p.Output.Text) > validation.LiveLogWindow ||
		validation.DisplayLogTail([]byte(p.Output.Text), nil) != p.Output.Text) {
		return false
	}
	if p.HandoffFrom != "" && (p.Session == "" || p.HandoffFrom == p.Session || len(p.HandoffFrom) > 128 ||
		strings.Trim(p.HandoffFrom, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "") {
		return false
	}
	return p.validTools() && p.Instance > 0 && p.PID > 0 && ValidHarnessVersion(p.Harness, p.Version) && !p.Started.IsZero() &&
		(p.Exited == nil && p.ExitCode == nil || p.Exited != nil && !p.Exited.Before(p.Started) && p.ExitCode != nil && *p.ExitCode >= -1) &&
		len(p.Session) <= 128 && strings.Trim(p.Session, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") == ""
}

func (p HarnessProcess) validToolOutput() bool {
	o := p.ToolOutput
	if o == nil {
		return true
	}
	if p.Session == "" || o.Call < 1 || o.Seq < o.Call || !KnownTool(p.Harness, o.Tool) || o.Text == "" ||
		len(o.Text) > validation.LiveLogWindow || validation.DisplayLogTail([]byte(o.Text), nil) != o.Text ||
		len(p.Tools) == 0 || o.Seq > p.Tools[len(p.Tools)-1].Seq {
		return false
	}
	for _, e := range p.Tools {
		if e.Seq == o.Seq {
			return e.Call == o.Call && e.Tool == o.Tool && e.Phase != "started"
		}
	}
	return o.Seq < p.Tools[0].Seq
}

func (p HarnessProcess) outputExcludes(lease string) bool {
	var texts []string
	if p.Output != nil {
		texts = append(texts, p.Output.Text)
	}
	if p.ToolOutput != nil {
		texts = append(texts, p.ToolOutput.Text)
	}
	for _, text := range texts {
		if validation.DisplayLogTail([]byte(text), []string{lease}) != text {
			return false
		}
	}
	return true
}

type ProcessHistory []HarnessProcess

func (history ProcessHistory) matches(result Result) bool {
	if len(history) == 0 || (result.Harness != "claude" && result.Harness != "codex") {
		return false
	}
	var previous int64
	var latestSession string
	for i, p := range history {
		if p.Instance <= previous || !p.Valid() || p.Harness != result.Harness || p.Version != result.HarnessVersion || p.Exited == nil {
			return false
		}
		if p.HandoffFrom != "" && (i < 2 || p.HandoffFrom != latestSession || history[i-1].Session != "" || *history[i-1].ExitCode != 1) {
			return false
		}
		previous = p.Instance
		if p.Session != "" {
			if result.Harness == "claude" && result.SessionID != "" && latestSession != "" && latestSession != p.Session && p.HandoffFrom == "" {
				return false
			}
			latestSession = p.Session
		}
	}
	if result.SessionID != "" && result.SessionID != latestSession {
		return false
	}
	last := history[len(history)-1]
	return result.ExitCode != 0 || (last.Session != "" && last.Session == result.SessionID && *last.ExitCode == 0)
}

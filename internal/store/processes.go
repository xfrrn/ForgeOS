package store

import (
	"strings"
	"time"
)

type HarnessProcess struct {
	Instance    int64       `json:"instance"`
	PID         int         `json:"pid"`
	Harness     string      `json:"harness"`
	Version     string      `json:"version"`
	Session     string      `json:"session_id,omitempty"`
	HandoffFrom string      `json:"handoff_from,omitempty"`
	Started     time.Time   `json:"started_at"`
	Exited      *time.Time  `json:"exited_at,omitempty"`
	ExitCode    *int        `json:"exit_code"`
	Tools       []ToolEvent `json:"tools,omitempty"`
}

func (p HarnessProcess) Valid() bool {
	if p.HandoffFrom != "" && (p.Session == "" || p.HandoffFrom == p.Session || len(p.HandoffFrom) > 128 ||
		strings.Trim(p.HandoffFrom, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "") {
		return false
	}
	return p.validTools() && p.Instance > 0 && p.PID > 0 && ValidHarnessVersion(p.Harness, p.Version) && !p.Started.IsZero() &&
		(p.Exited == nil && p.ExitCode == nil || p.Exited != nil && !p.Exited.Before(p.Started) && p.ExitCode != nil && *p.ExitCode >= -1) &&
		len(p.Session) <= 128 && strings.Trim(p.Session, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") == ""
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

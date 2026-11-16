package store

import (
	"strings"
	"time"
)

type HarnessProcess struct {
	Instance int64     `json:"instance"`
	PID      int       `json:"pid"`
	Harness  string    `json:"harness"`
	Version  string    `json:"version"`
	Session  string    `json:"session_id,omitempty"`
	Started  time.Time `json:"started_at"`
	Exited   time.Time `json:"exited_at"`
	ExitCode *int      `json:"exit_code"`
}

type ProcessHistory []HarnessProcess

func (history ProcessHistory) matches(result Result) bool {
	if len(history) == 0 || (result.Harness != "claude" && result.Harness != "codex") {
		return false
	}
	var previous int64
	var latestSession string
	for _, p := range history {
		if p.Instance <= previous || p.PID < 1 || p.Harness != result.Harness || p.Version != result.HarnessVersion ||
			p.Started.IsZero() || p.Exited.Before(p.Started) || p.ExitCode == nil || *p.ExitCode < -1 ||
			len(p.Session) > 128 || strings.Trim(p.Session, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "" || result.Harness == "claude" && p.Session != "" && result.SessionID != "" && p.Session != result.SessionID {
			return false
		}
		previous = p.Instance
		if p.Session != "" {
			latestSession = p.Session
		}
	}
	if result.SessionID != "" && result.SessionID != latestSession {
		return false
	}
	last := history[len(history)-1]
	return result.ExitCode != 0 || (last.Session != "" && last.Session == result.SessionID && *last.ExitCode == 0)
}

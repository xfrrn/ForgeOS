package main

import (
	"crypto/sha256"
	"encoding/json"
	"testing"

	"forgeos/internal/store"
)

func TestNativeInputHistoryConfirmation(t *testing.T) {
	workspace := t.TempDir()
	body := "Use the requested heading."
	sum := sha256.Sum256([]byte(body))
	poll := store.TurnInputPoll{NativeTurn: store.NativeTurn{Session: "original", Turn: "active"}, Key: "owner-key"}
	message := map[string]any{"type": "userMessage", "id": "item-3", "clientId": poll.Key,
		"content": []map[string]string{{"type": "text", "text": body}}}
	for _, tc := range []struct {
		name, thread, cwd, turn string
		items                   []any
		want                    bool
	}{
		{"one exact input", poll.Session, workspace, poll.Turn, []any{message}, true},
		{"wrong thread", "other", workspace, poll.Turn, []any{message}, false},
		{"wrong workspace", poll.Session, t.TempDir(), poll.Turn, []any{message}, false},
		{"wrong turn", poll.Session, workspace, "other", []any{message}, false},
		{"not present", poll.Session, workspace, poll.Turn, nil, false},
		{"duplicate key", poll.Session, workspace, poll.Turn, []any{message, message}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"thread": map[string]any{"id": tc.thread, "cwd": tc.cwd,
				"turns": []any{map[string]any{"id": tc.turn, "items": tc.items}}}})
			var h inputHistory
			if err != nil || json.Unmarshal(raw, &h) != nil {
				t.Fatal("invalid history fixture")
			}
			if h.confirms(workspace, poll, sum[:]) != tc.want {
				t.Fatal("unexpected confirmation")
			}
			other := sha256.Sum256([]byte("another body"))
			if h.confirms(workspace, poll, other[:]) {
				t.Fatal("confirmed a different input")
			}
			wrongKey := poll
			wrongKey.Key = "other-owner-key"
			if h.confirms(workspace, wrongKey, sum[:]) {
				t.Fatal("confirmed another client message ID")
			}
		})
	}
}

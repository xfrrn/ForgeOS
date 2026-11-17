package schemas

import (
	"encoding/json"
	"os"
	"testing"
)

func TestAgentMessageV1(t *testing.T) {
	example, err := os.ReadFile("agent-message.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var message map[string]any
	if err = json.Unmarshal(example, &message); err != nil {
		t.Fatal(err)
	}
	check := func(valid bool) {
		t.Helper()
		raw, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		if err = ValidateAgentMessage(raw); (err == nil) != valid {
			t.Fatalf("valid=%v, error=%v", valid, err)
		}
	}
	for _, kind := range []string{"REQUEST", "EVENT", "COMMAND", "RESPONSE", "RESULT"} {
		message["message_type"] = kind
		check(true)
	}
	for key, value := range map[string]any{"schema_version": "2.0", "message_type": "WRITE", "timestamp": "2026-11-31", "from": map[string]any{"id": "agent"}, "payload": "not an object", "idempotency_key": " ", "project_id": nil} {
		old := message[key]
		message[key] = value
		check(false)
		message[key] = old
	}
	delete(message, "trust_level")
	check(false)
}

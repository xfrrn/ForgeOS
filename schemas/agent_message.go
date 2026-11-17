package schemas

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed agent-message.schema.json
var agentMessageJSON []byte
var agentMessageSchema = cachedSchema("urn:forgeos:agent-message:1.0", agentMessageJSON)

type Actor struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type AgentMessage struct {
	Version        string         `json:"schema_version"`
	ID             string         `json:"message_id"`
	Correlation    string         `json:"correlation_id"`
	Project        string         `json:"project_id"`
	Goal           *string        `json:"goal_id"`
	WorkItem       *string        `json:"work_item_id"`
	Execution      *string        `json:"execution_id"`
	From           Actor          `json:"from"`
	To             Actor          `json:"to"`
	Type           string         `json:"message_type"`
	Payload        map[string]any `json:"payload"`
	Timestamp      time.Time      `json:"timestamp"`
	IdempotencyKey string         `json:"idempotency_key"`
	Trust          string         `json:"trust_level"`
}

func ValidateAgentMessage(raw json.RawMessage) error {
	schema, err := agentMessageSchema()
	if err != nil {
		return err
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	return schema.Validate(value)
}

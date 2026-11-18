package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"forgeos/schemas"
)

// The project-scoped review has its own identity even when its source is a task receipt.
func (j CurationJob) resultEnvelope(project int64, at time.Time) schemas.AgentMessage {
	id := fmt.Sprintf("CURATION-%d", j.ID)
	return schemas.AgentMessage{Version: "1.0", ID: id + "-RESULT", Correlation: id, IdempotencyKey: id + "-RESULT",
		Project: fmt.Sprintf("PROJECT-%d", project), From: schemas.Actor{Type: "agent", ID: "curator"},
		To: schemas.Actor{Type: "agent", ID: "management"}, Type: "RESULT", Trust: "agent_generated", Timestamp: at}
}

func (j *CurationJob) RouteExtraction(project int64) error {
	if j.Extraction == nil || j.extractedAt == nil {
		return errors.New("curation review missing extraction receipt")
	}
	var source struct {
		Digest string `json:"content_sha256"`
	}
	if err := json.Unmarshal(j.Material, &source); err != nil {
		return err
	}
	m := j.resultEnvelope(project, j.extractedAt.UTC())
	m.Payload = map[string]any{"source_sha256": source.Digest, "session": j.Extraction.Session, "facts": j.Extraction.Facts}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := schemas.ValidateAgentMessage(raw); err != nil {
		return err
	}
	j.ResultMessage, j.Extraction = raw, nil
	return nil
}

// ReviewInput binds the received RESULT to this assignment before starting the native session.
func (j CurationJob) ReviewInput() (json.RawMessage, error) {
	if j.ResultMessage == nil && j.Extraction != nil {
		return json.Marshal(j.Extraction)
	}
	if j.Extraction != nil || j.ResultMessage == nil {
		return nil, errors.New("expected one curator result")
	}
	if err := schemas.ValidateAgentMessage(j.ResultMessage); err != nil {
		return nil, err
	}
	var m schemas.AgentMessage
	var source struct {
		Project int64 `json:"project_id"`
		Content string
		Digest  string `json:"content_sha256"`
	}
	if err := json.Unmarshal(j.Material, &source); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(j.ResultMessage, &m); err != nil {
		return nil, err
	}
	data := m.Payload
	m.Payload = nil
	if source.Project <= 0 || j.ID <= 0 || m.Timestamp.IsZero() || !reflect.DeepEqual(m, j.resultEnvelope(source.Project, m.Timestamp)) ||
		len(data) != 3 || data["source_sha256"] != source.Digest || source.Digest != fmt.Sprintf("%x", sha256.Sum256([]byte(source.Content))) {
		return nil, errors.New("curator result does not match project, source or route")
	}
	session, _ := data["session"].(string)
	raw, err := json.Marshal(data["facts"])
	if err != nil {
		return nil, err
	}
	extraction := Extraction{Session: session}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&extraction.Facts); err != nil || len(extraction.Facts) == 0 || !extraction.valid() {
		return nil, errors.New("invalid extraction in curator result")
	}
	for _, fact := range extraction.Facts {
		if !strings.Contains(source.Content, fact.Quote) {
			return nil, errors.New("curator result contains an unsupported fact")
		}
	}
	return j.ResultMessage, nil
}

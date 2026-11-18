package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestCurationResultRoute(t *testing.T) {
	at := time.Now().UTC()
	source := "Checkpoint 274 was restored."
	raw, _ := json.Marshal(map[string]any{"project_id": 7, "content": source, "content_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(source)))})
	extraction := &Extraction{Session: "curator-1", Facts: []KnowledgeFact{{"Restore succeeded.", "274"}}}
	job := CurationJob{ID: 23, Material: raw, Extraction: extraction, extractedAt: &at}
	if legacy, err := job.ReviewInput(); err != nil || !bytes.Contains(legacy, []byte("curator-1")) {
		t.Fatal("legacy input", err)
	}
	if err := job.RouteExtraction(7); err != nil || job.Extraction != nil {
		t.Fatal("could not route extraction", err)
	}
	message, err := job.ReviewInput()
	if err != nil || !bytes.Equal(message, job.ResultMessage) {
		t.Fatal("review did not receive the result envelope", err)
	}
	for _, change := range [][2]string{{"PROJECT-7", "PROJECT-8"}, {"CURATION-23", "CURATION-24"}, {"agent_generated", "owner_authenticated"}, {"management", "owner"}, {"RESULT", "EVENT"}, {`"goal_id":null`, `"goal_id":"GOAL-1"`}, {`"quote":"274"`, `"quote":"other"`}, {"source_sha256", "other_digest"}, {`"session":"curator-1"`, `"session":[]`}} {
		job.ResultMessage = bytes.ReplaceAll(message, []byte(change[0]), []byte(change[1]))
		if bytes.Equal(message, job.ResultMessage) {
			t.Fatal("fixture did not change", change)
		}
		if _, err := job.ReviewInput(); err == nil {
			t.Fatal("accepted incorrect envelope", change)
		}
	}
	job.ResultMessage, job.Extraction = message, extraction
	if _, err := job.ReviewInput(); err == nil {
		t.Fatal("accepted ambiguous proposal")
	}
}

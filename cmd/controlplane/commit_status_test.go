package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func TestCommitStatusReconcilesLostWrite(t *testing.T) {
	root, repo, sha := issueTestRepo(t)
	issue := store.Issue{Repository: "my-org/project", RepositoryID: 19}
	v := store.ValidationResult{Profile: "V2", Commit: sha, ExitCode: 1, Run: &validation.Run{Failure: "TIMEOUT"}}
	var saved map[string]any
	count := 0
	missing := true
	api := func(_ context.Context, method, endpoint string, body, dest any) error {
		var result any
		switch {
		case method == "GET" && strings.HasSuffix(endpoint, "/git/commits/"+sha):
			if missing {
				return errMissing
			}
			result = map[string]string{"sha": sha}
		case method == "GET" && strings.Contains(endpoint, "/status?"):
			rows := []map[string]any{}
			if saved != nil {
				rows = append(rows, saved)
			}
			result = map[string]any{"repository": map[string]any{"id": 19, "full_name": "my-org/project"}, "sha": sha, "statuses": rows, "total_count": len(rows)}
		case method == "POST" && strings.HasSuffix(endpoint, "/statuses/"+sha):
			b := body.(map[string]string)
			if b["state"] != "error" {
				t.Fatal(b)
			}
			count++
			saved = map[string]any{"id": 26, "context": b["context"], "state": b["state"], "description": b["description"]}
			return errors.New("response lost after GitHub wrote the status")
		default:
			t.Fatalf("unexpected GitHub call: %s %s", method, endpoint)
		}
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, dest)
	}
	publish := publishValidation(root, 3, 4, api)
	if _, err := publish(context.Background(), repo, issue, v); !errors.Is(err, errMissing) || count != 0 {
		t.Fatal("unknown SHA published", err)
	}
	missing = false
	if _, err := publish(context.Background(), repo, issue, v); err == nil {
		t.Fatal("missing remote acknowledgement")
	}
	receipt, err := publish(context.Background(), repo, issue, v)
	if err != nil || receipt.ID != 26 || receipt.State != "error" || count != 1 {
		t.Fatal(receipt, err, count)
	}
}

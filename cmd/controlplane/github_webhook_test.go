package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestGitHubWebhookReceipts(t *testing.T) {
	ctx := context.Background()
	data := store.New(testdb.Open(t))
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := data.CreateProject(ctx, "sample")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	key, config := filepath.Join(dir, "key"), filepath.Join(dir, "projects.json")
	if err := os.WriteFile(key, []byte("project-secret\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(fmt.Sprintf(`[{"project_id":%d,"validation":"docs","webhook_secret_file":%q}]`, project.ID, key)), 0600); err != nil {
		t.Fatal(err)
	}
	pollers, err := configuredPollers("", "", config)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerGitHubWebhooks(mux, data, "owner", pollers)
	endpoint := fmt.Sprintf("/hooks/github/%d", project.ID)
	id := "2adb51d7-2a7b-4a1c-999a-4af4dfe8070d"
	send := func(body, event, guid, secret string, code int) {
		t.Helper()
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(body))
		req := httptest.NewRequest("POST", endpoint, strings.NewReader(body))
		req.Header.Set("X-GitHub-Event", event)
		req.Header.Set("X-GitHub-Delivery", guid)
		req.Header.Set("X-Hub-Signature-256", fmt.Sprintf("sha256=%x", mac.Sum(nil)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != code {
			t.Fatalf("event %s: got %d want %d", event, w.Code, code)
		}
	}
	body := "{ \"repository\": {\"id\":82}, \"action\":\"edited\", \"note\":\"中文\" }\n"
	send(body, "issues", id, "project-secret", 503)
	if err := data.ReplaceInbox(ctx, project.ID, 82, nil); err != nil {
		t.Fatal(err)
	}
	send(body, "issues", id, "wrong-key", 401)
	send(body, "issues", "invalid", "project-secret", 400)
	send(strings.Replace(body, "82", "83", 1), "issues", id, "project-secret", 403)
	send(body, "issues", id, "project-secret", 202)
	<-pollers[project.ID].wake
	send(body, "issues", id, "project-secret", 200)
	send(body+" ", "issues", id, "project-secret", 409)
	send(body, "issue_comment", id, "project-secret", 409)
	send(body, "ping", "7062dd65-355b-4aab-a211-0e91e7555473", "project-secret", 202)
	select {
	case <-pollers[project.ID].wake:
		t.Fatal("duplicate, conflict or ping triggered another scan")
	default:
	}
	read := func(path, token string, status int) string {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("GET %s: got %d want %d", path, w.Code, status)
		}
		return w.Body.String()
	}
	list := fmt.Sprintf("/projects/%d/github-events", project.ID)
	read(list, "wrong", 401)
	read(list+"?before=bad", "owner", 400)
	if raw := read(list+"/"+id, "owner", 200); raw != body {
		t.Fatal("event body changed")
	}
	if meta := read(list, "owner", 200); strings.Count(meta, "delivery_id") != 2 || strings.Contains(meta, "project-secret") || strings.Contains(meta, "中文") {
		t.Fatal("invalid receipt summary")
	}
	read("/projects/999/github-events/"+id, "owner", 404)
}

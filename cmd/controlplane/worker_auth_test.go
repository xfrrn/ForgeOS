package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestWorkerProjectGrants(t *testing.T) {
	file := filepath.Join(t.TempDir(), "workers.json")
	grant := func(id, token string, projects string) string {
		return fmt.Sprintf(`{"worker_id":"%s","token_sha256":"%x","projects":%s}`, id, sha256.Sum256([]byte(token)), projects)
	}
	config := func(doc string) (workerAccess, error) {
		t.Helper()
		if err := os.WriteFile(file, []byte(doc), 0600); err != nil {
			t.Fatal(err)
		}
		return loadWorkers(file, "", "owner", "manager")
	}
	g := grant("A", "alpha", "[1]")
	for _, scope := range []string{
		`[{"project_id":2,"environment":"local-mock"}]`,
		`[{"project_id":1,"environment":"invalid"}]`,
		`[{"project_id":1,"environment":"production"},{"project_id":1,"environment":"production"}]`,
	} {
		if _, err := config("[" + g[:len(g)-1] + `,"environments":` + scope + "}]"); err == nil {
			t.Fatal("invalid Worker environment grant accepted")
		}
	}
	for _, bad := range []string{"[]", "[" + g + "," + g + "]", "[" + g + "," + grant("B", "alpha", "[2]") + "]", "[" + grant("A", "owner", "[1]") + "]", "[" + grant("A", "manager", "[1]") + "]", "[" + grant("A", "a", "[]") + "]", "[" + grant("A", "a", "[1,1]") + "]", "[" + g + "]{}"} {
		if _, err := config(bad); err == nil {
			t.Fatal("invalid grant accepted")
		}
	}
	access, err := config("[" + g + "," + grant("B", "beta", "[2]") + "]")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadWorkers(file, "legacy", "owner", "manager"); err == nil {
		t.Fatal("unscoped fallback allowed")
	}
	ctx := context.Background()
	s := store.New(testdb.Open(t))
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p1, err := s.CreateProject(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.CreateProject(ctx, "B")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateTask(ctx, p2.ID); err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateTask(ctx, p1.ID)
	if err != nil {
		t.Fatal(err)
	}
	mux := routes(s, t.TempDir(), "owner", access, nil)
	post := func(token, path string, body any, want int) []byte {
		t.Helper()
		doc, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(doc))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: got %d want %d", path, w.Code, want)
		}
		return w.Body.Bytes()
	}
	req := map[string]any{"worker": "B", "workspace_root": t.TempDir()}
	h := store.WorkerHeartbeat{Worker: "B", OS: "linux", Arch: "amd64", CPUs: 2, Slots: 1}
	post("alpha", "/internal/workers/heartbeat", h, 403)
	h.Worker = "A"
	post("owner", "/internal/workers/heartbeat", h, 401)
	post("alpha", "/internal/workers/heartbeat", h, 204)
	h.CPUs = 0
	post("alpha", "/internal/workers/heartbeat", h, 400)
	post("alpha", "/internal/claim", req, 403)
	req["worker"] = "A"
	post("manager", "/internal/claim", req, 401)
	var job store.Job
	if err = json.Unmarshal(post("alpha", "/internal/claim", req, 200), &job); err != nil || job.TaskID != a.ID {
		t.Fatal(err, job.TaskID)
	}
	path := fmt.Sprintf("/internal/executions/%d/result", job.ExecutionID)
	renew := fmt.Sprintf("/internal/executions/%d/renew", job.ExecutionID)
	lease := map[string]string{"lease_token": job.LeaseToken}
	post("beta", renew, lease, 403)
	post("alpha", renew, lease, 204)
	report, err := s.WorkerNodes(ctx)
	var nodes []struct {
		Worker   string
		Online   bool
		Reserved int
	}
	if err != nil || json.Unmarshal(report, &nodes) != nil || len(nodes) != 1 || nodes[0].Worker != "A" || !nodes[0].Online || nodes[0].Reserved != 1 {
		t.Fatal("unexpected node reservation", string(report), err)
	}
	result := map[string]any{"lease_token": job.LeaseToken, "exit_code": 1, "error": "controlled"}
	post("beta", path, result, 403)
	post("alpha", path, result, 204)
	post("alpha", renew, lease, 409)
	post("alpha", "/internal/claim", req, 204)
}

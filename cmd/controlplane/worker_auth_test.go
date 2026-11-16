package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestWorkerProjectGrants(t *testing.T) {
	file := filepath.Join(t.TempDir(), "workers.json")
	grant := func(id, token string, projects string) string {
		return fmt.Sprintf(`{"worker_id":"%s","token_sha256":"%x","projects":%s}`, id, sha256.Sum256([]byte(token)), projects)
	}
	config := func(doc string) (*workerRegistry, error) {
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
	processPath := fmt.Sprintf("/internal/executions/%d/process", job.ExecutionID)
	pulse := store.ProcessReport{LeaseToken: job.LeaseToken, Process: store.HarnessProcess{Instance: 1, PID: 7, Harness: "codex", Version: "1.2.3", Started: time.Now()}}
	post("beta", processPath, pulse, 403)
	post("owner", processPath, pulse, 401)
	// A Git HEAD check does not run a model.
	post("alpha", processPath, pulse, 409)
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

func TestWorkerCredentialRotation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "nodes.json")
	t.Setenv("FORGEOS_REVIEWER_TOKEN", "reviewer")
	write := func(primary, overlap string) {
		t.Helper()
		row := map[string]any{"worker_id": "A", "projects": []int{7}, "token_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(primary)))}
		if overlap != "" {
			row["next_token_sha256"] = fmt.Sprintf("%x", sha256.Sum256([]byte(overlap)))
		}
		doc, err := json.Marshal([]any{row})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, doc, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("old", "")
	registry, err := loadWorkers(file, "", "owner", "manager")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"old", "owner", "manager", "reviewer"} {
		write("old", bad)
		if err := registry.reload(); err == nil || registry.match("old") == nil || registry.match("new") != nil {
			t.Fatal("invalid replacement changed grants")
		}
	}
	write("old", "new")
	if err := registry.reload(); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"old", "new"} {
		g := registry.match(token)
		if g == nil || g.id != "A" || len(g.projects) != 1 || g.projects[0] != 7 {
			t.Fatal("scope lost")
		}
	}
	write("new", "")
	if err := registry.reload(); err != nil || registry.match("old") != nil || registry.match("new") == nil {
		t.Fatal("retired credential still active", err)
	}
}

func TestNodeCertificateBindingAndRevocation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "grants.json")
	old := &x509.Certificate{Raw: []byte("old"), NotAfter: time.Now().Add(time.Hour)}
	newer := &x509.Certificate{Raw: []byte("new"), NotAfter: old.NotAfter}
	grant := func(id string, certificates ...*x509.Certificate) map[string]any {
		var hashes []string
		for _, cert := range certificates {
			hashes = append(hashes, fmt.Sprintf("%x", sha256.Sum256(cert.Raw)))
		}
		return map[string]any{"worker_id": id, "projects": []int{1}, "token_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(id))), "certificate_sha256": hashes}
	}
	write := func(rows ...map[string]any) {
		t.Helper()
		b, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(grant("first", old, newer), grant("second"))
	registry, err := loadWorkers(file, "", "owner", "manager")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registry.handle(mux, "GET /internal/test", registry.authorize(func(w http.ResponseWriter, r *http.Request, g *workerGrant) { w.WriteHeader(204) }))
	check := func(token string, cert *x509.Certificate, want int) {
		t.Helper()
		r := httptest.NewRequest("GET", "/internal/test", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		if cert != nil {
			r.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{cert}}, PeerCertificates: []*x509.Certificate{cert}}
		}
		w := httptest.NewRecorder()
		registry.nodeRoutes.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("got %d, want %d", w.Code, want)
		}
	}
	check("first", old, 204)
	check("first", newer, 204)
	check("first", nil, 403)
	check("second", old, 403)
	write(grant("first", old), grant("second", old))
	if registry.reload() == nil {
		t.Fatal("accepted shared certificate")
	}
	check("first", newer, 204)
	write(grant("first", newer), grant("second"))
	if err = registry.reload(); err != nil {
		t.Fatal(err)
	}
	check("first", old, 403)
	check("first", newer, 204)
	newer.NotAfter = time.Now().Add(-time.Minute)
	check("first", newer, 403)
}

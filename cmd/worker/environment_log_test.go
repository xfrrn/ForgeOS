package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func TestStartupOutputIsRedactedBeforeLocalStorage(t *testing.T) {
	dir := t.TempDir()
	token := "private-node"
	l := &environmentLog{secrets: []string{"private-lease"}, credential: func() (string, error) { return token, nil }}
	f, err := l.open(dir, "local-mock")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	write := func(s string) {
		t.Helper()
		if _, err := l.writer.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	write("private-")
	data, err := os.ReadFile(f.Name())
	if err != nil || len(data) != 0 {
		t.Fatal("partial line stored", err)
	}
	write("node\npass")
	write("word=private-value\n-----BE")
	write("GIN PRIVATE KEY-----\nprivate-first\n")
	token = "private-rotated"
	write("private-second\n-----END PRIVATE KEY-----\nprivate-rotated\nprivate-lease\nReady\n")
	write(strings.Repeat("x", 65<<10))
	write("private-large\nprivate-hidden\n-----END PRIVATE KEY-----\n")
	write("Final line without newline")
	out, err := l.finish(dir, f)
	if err != nil || out == nil || !strings.HasSuffix(out.Text, "Final line without newline\n") {
		t.Fatal(out, err)
	}
	data, err = os.ReadFile(f.Name())
	if err != nil || strings.Contains(string(data), "private-") || len(data) != out.Bytes || len(out.Text) > validation.LiveLogWindow {
		t.Fatal("unsafe or inconsistent local log", err)
	}
	dir = t.TempDir()
	l = &environmentLog{credential: func() (string, error) { return "", nil }}
	f, err = l.open(dir, "local-mock")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = l.writer.Write([]byte("Already observed\n")); err != nil {
		t.Fatal(err)
	}
	broken := errors.New("credential unavailable")
	l.writer.output.credential = func() (string, error) { return "", broken }
	if _, err = l.writer.Write([]byte("private-later\n")); !errors.Is(err, broken) {
		t.Fatal(err)
	}
	out, err = l.finish(dir, f)
	if !errors.Is(err, broken) || out == nil || out.Text != "Already observed\n" {
		t.Fatal("lost prior output after capture failure", out, err)
	}
}

func TestStartupDockerLogsSurviveReadinessFailureAndCancellation(t *testing.T) {
	if os.Getenv("FORGEOS_TEST_DOCKER") != "1" {
		t.Skip("enable Docker to exercise startup output")
	}
	_, job, _ := fixture(t)
	repo := job.RepoPath
	validationCommit(t, repo, "main.go", `package main
import("fmt";"net/http";"time")
func main(){fmt.Println("Service starting");fmt.Println("password=synthetic-startup-secret");time.Sleep(10*time.Second);http.ListenAndServe("127.0.0.1:8080",http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(204)}))}
`)
	build := exec.Command("go", "build", "-o", filepath.Join(repo, "service"), filepath.Join(repo, "main.go"))
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0", "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	git(t, repo, "add", "--", "service")
	validationCommit(t, repo, "project.yaml", `{"version":1,"local_environments":{"local-mock":{"start_script":"start.sh","port":8080,"health_path":"/","timeout_seconds":30}}}`)
	image := os.Getenv("FORGEOS_STARTUP_IMAGE")
	if image == "" {
		image = "golang:1.26.9-bookworm"
	}
	t.Setenv("FORGEOS_STARTUP_IMAGE", image)
	for _, mode := range []string{"ready", "exit", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			script := "cp service /tmp/server && chmod 700 /tmp/server && exec /tmp/server\n"
			if mode == "exit" {
				script = "printf 'Immediate failure\\npassword=synthetic-startup-secret'; exit 7\n"
			}
			sha := validationCommit(t, repo, "start.sh", script)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			reports := make(chan store.CILogReport, 16)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var report store.CILogReport
				if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
					t.Error(err)
				}
				reports <- report
				w.WriteHeader(204)
			}))
			defer server.Close()
			node := worker{api: server.URL, client: server.Client(), token: "node"}
			observe, stop := node.watchCILog(ctx, store.Job{ExecutionID: 7, LeaseToken: strings.Repeat("a", 64)}, sha)
			defer stop()
			run := environmentRun{EnvironmentRequest: store.EnvironmentRequest{Repo: "project", Commit: sha, Environment: "local-mock"}, Worker: "node", Started: time.Now().UTC()}
			dir := t.TempDir()
			done := make(chan error, 1)
			go func() { done <- run.record(ctx, repo, dir, &environmentLog{observe: observe, run: "ci-run-7"}) }()
			if mode != "exit" {
			waiting:
				for {
					select {
					case report := <-reports:
						if report.Log.Text == "" {
							continue
						}
						if report.Log.Check != "local-mock" || !strings.Contains(report.Log.Text, "Service starting") || strings.Contains(report.Log.Text, "synthetic-startup-secret") {
							t.Fatal(report.Log)
						}
						select {
						case err := <-done:
							t.Fatal("no log before readiness", err)
						default:
						}
						if mode == "cancel" {
							cancel()
						}
						break waiting
					case err := <-done:
						t.Fatal("no running log", err)
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
				}
			}
			err := <-done
			if (mode == "ready") != (err == nil) || !run.Removed || run.Output == nil || run.Output.Bytes == 0 || strings.Contains(run.Output.Text, "synthetic-startup-secret") || !strings.Contains(run.Output.Text, "[redacted]") {
				t.Fatal(run, err)
			}
			if mode == "exit" && !strings.Contains(run.Output.Text, "Immediate failure") {
				t.Fatal("lost immediate output", run)
			}
			data, err := os.ReadFile(filepath.Join(dir, "startup.log"))
			if err != nil || strings.Contains(string(data), "synthetic-startup-secret") {
				t.Fatal("raw secret on disk", err)
			}
		})
	}
}

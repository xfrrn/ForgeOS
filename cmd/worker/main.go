package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"forgeos/internal/nodetls"
	"forgeos/internal/store"
)

type worker struct {
	api, token, id, repoRoot, workspaceRoot string
	tokenFile                               string
	codex, codexVersion                     string
	codexAppServer                          bool
	claude                                  claudeCLI
	planner                                 bool
	slots                                   int
	modelSlots, ciSlots                     int
	client                                  *http.Client
	journal                                 *sql.DB
}

func fromEnv(role string) (*worker, error) {
	if os.Geteuid() == 0 {
		return nil, errors.New("worker requires a non-root user")
	}
	w := &worker{
		api:           os.Getenv("FORGEOS_API_URL"),
		token:         os.Getenv("FORGEOS_WORKER_TOKEN"),
		id:            os.Getenv("FORGEOS_WORKER_ID"),
		repoRoot:      os.Getenv("FORGEOS_REPO_ROOT"),
		workspaceRoot: os.Getenv("FORGEOS_WORKSPACE_ROOT"),
		client:        &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	if w.api == "" {
		w.api = "http://127.0.0.1:8080"
	}
	u, err := url.Parse(w.api)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.ForceQuery || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("FORGEOS_API_URL must be an HTTP(S) origin")
	}
	identity, ca := os.Getenv("FORGEOS_NODE_TLS_IDENTITY_FILE"), os.Getenv("FORGEOS_NODE_TLS_CA_FILE")
	if role == "" && (identity != "" || ca != "") {
		if u.Scheme != "https" {
			return nil, errors.New("node TLS requires an HTTPS origin")
		}
		if _, err := nodetls.Identity(identity); err != nil {
			return nil, err
		}
		pool, err := nodetls.Roots(ca)
		if err != nil {
			return nil, err
		}
		// A new handshake reads the current identity after an atomic file replacement.
		w.client.Transport = &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
				RootCAs:    pool,
				GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
					return nodetls.Identity(identity)
				},
			},
		}
	} else if u.Hostname() != "localhost" && !net.ParseIP(u.Hostname()).IsLoopback() {
		return nil, errors.New("non-loopback Worker origin requires a node TLS identity and CA")
	}
	w.api = strings.TrimRight(w.api, "/")
	if role == "reviewer" {
		w.token = os.Getenv("FORGEOS_REVIEWER_TOKEN")
		info, err := os.Stat(w.repoRoot)
		if strings.TrimSpace(w.token) == "" || !filepath.IsAbs(w.repoRoot) || err != nil || !info.IsDir() {
			return nil, errors.New("set FORGEOS_REVIEWER_TOKEN and an existing absolute FORGEOS_REPO_ROOT")
		}
		return w, nil
	}
	if role == "management" || role == "test-agent" || role == "curator" {
		w.token = os.Getenv("FORGEOS_MANAGEMENT_TOKEN")
		if role == "curator" {
			w.token = os.Getenv("FORGEOS_CURATOR_TOKEN")
		}
		if role == "test-agent" {
			w.token = os.Getenv("FORGEOS_TEST_AGENT_TOKEN")
		}
		if strings.TrimSpace(w.token) == "" {
			return nil, fmt.Errorf("set the dedicated %s token", role)
		}
		w.codex, w.planner, err = findCodex(os.Getenv("FORGEOS_CODEX_BIN"), false)
		if err != nil {
			return nil, err
		}
		if !w.planner {
			return nil, fmt.Errorf("%s requires local Codex with structured output", role)
		}
		return w, nil
	}
	if w.id == "" {
		w.id, err = os.Hostname()
		if err != nil {
			return nil, err
		}
	}
	if w.slots, err = workerSlots("FORGEOS_WORKER_SLOTS", 1); err != nil {
		return nil, err
	}
	if w.modelSlots, err = workerSlots("FORGEOS_WORKER_MODEL_SLOTS", w.slots); err != nil {
		return nil, err
	}
	if w.ciSlots, err = workerSlots("FORGEOS_WORKER_CI_SLOTS", w.slots); err != nil {
		return nil, err
	}
	w.tokenFile = os.Getenv("FORGEOS_WORKER_TOKEN_FILE")
	if w.tokenFile != "" && (!filepath.IsAbs(w.tokenFile) || w.token != "") {
		return nil, errors.New("configure either an absolute FORGEOS_WORKER_TOKEN_FILE or FORGEOS_WORKER_TOKEN")
	}
	if w.tokenFile == "" && w.token == "" {
		return nil, errors.New("set FORGEOS_WORKER_TOKEN or FORGEOS_WORKER_TOKEN_FILE")
	}
	if _, err = w.credential(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(w.repoRoot) || !filepath.IsAbs(w.workspaceRoot) {
		return nil, errors.New("set absolute FORGEOS_REPO_ROOT and FORGEOS_WORKSPACE_ROOT")
	}
	w.repoRoot, err = filepath.EvalSymlinks(w.repoRoot)
	if err != nil {
		return nil, err
	}
	if stat, err := os.Stat(w.repoRoot); err != nil || !stat.IsDir() {
		return nil, errors.New("repository root must be an existing directory")
	}
	if err := os.MkdirAll(w.workspaceRoot, 0700); err != nil {
		return nil, err
	}
	w.workspaceRoot, err = filepath.EvalSymlinks(w.workspaceRoot)
	if err != nil {
		return nil, err
	}
	w.codex, w.planner, err = findCodex(os.Getenv("FORGEOS_CODEX_BIN"), true)
	if err != nil {
		return nil, err
	}
	switch os.Getenv("FORGEOS_CODEX_TRANSPORT") {
	case "", "exec":
	case "app-server":
		if w.codex == "" {
			return nil, errors.New("App Server transport requires an available Codex executable")
		}
		if err = checkCodexAppServer(w.codex); err != nil {
			return nil, err
		}
		w.codexAppServer = true
	default:
		return nil, errors.New("FORGEOS_CODEX_TRANSPORT must be exec or app-server")
	}
	if w.codex != "" {
		w.codexVersion, err = readCodexVersion(w.codex)
		if err != nil {
			if os.Getenv("FORGEOS_CODEX_BIN") != "" || w.codexAppServer {
				return nil, err
			}
			log.Printf("skipping discovered Codex: %v", err)
			w.codex, w.planner = "", false
		}
	}
	w.claude, err = findClaude(os.Getenv("FORGEOS_CLAUDE_BIN"))
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (w *worker) credential() (string, error) {
	if w.tokenFile == "" {
		return w.token, nil
	}
	f, err := os.Open(w.tokenFile)
	if err != nil {
		return "", errors.New("worker credential file unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("worker credential file is not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, 16385))
	token := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if err != nil || len(data) > 16384 || token == "" || strings.IndexFunc(token, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return "", errors.New("worker credential file must contain one bearer token")
	}
	return token, nil
}

func (w *worker) authenticatedRequest(req *http.Request) (*http.Response, error) {
	token, err := w.credential()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return w.client.Do(req)
}

func (w *worker) postJSON(ctx context.Context, path string, body any) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.api+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return w.authenticatedRequest(req)
}

func (w *worker) post(ctx context.Context, path string, body any, out any) (int, error) {
	resp, err := w.postJSON(ctx, path, body)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode == http.StatusOK {
		maxJob := int64(2 << 20)
		switch out.(type) {
		case *store.Job, *store.ReviewJob, *store.TestPlanJob, *store.ManagementJob:
			// A 16 KiB plan can cite hundreds of decisions with separately bounded text.
			maxJob = 32 << 20
		}
		var data []byte
		data, err = io.ReadAll(io.LimitReader(resp.Body, maxJob+1))
		if err == nil && int64(len(data)) > maxJob {
			err = errors.New("claim response too large")
		}
		if err == nil {
			err = json.Unmarshal(data, out)
		}
	}
	return resp.StatusCode, err
}

func wait(ctx context.Context) {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func workerSlots(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return int(n), nil
}

func temporary(status int) bool {
	return status == 0 || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func (w *worker) run(ctx context.Context) error {
	journal, err := w.openJournal(ctx)
	if err != nil {
		return err
	}
	w.journal = journal
	defer journal.Close()
	writer := w.codex != "" || w.claude.bin != ""
	caps := store.WorkerCapabilities{SupportsGlobalRules: true, SupportsResources: true, SupportsValidation: true, SupportsDeclaredChecks: true, SupportsRiskPolicy: true,
		SupportsCodex: w.codex != "", SupportsClaude: w.claude.bin != "", SupportsPlanner: w.planner, SupportsRework: writer, SupportsCheckpoints: writer}
	caps.SupportsStartup = os.Getenv("FORGEOS_STARTUP_IMAGE") != ""
	if caps.SupportsStartup && os.Getenv("FORGEOS_STARTUP_NETWORK") != "" && os.Getenv("FORGEOS_STARTUP_PROJECT_ID") != "" {
		caps.StartupIntegratedProject, err = strconv.ParseInt(os.Getenv("FORGEOS_STARTUP_PROJECT_ID"), 10, 64)
		if err != nil || caps.StartupIntegratedProject < 1 {
			return errors.New("FORGEOS_STARTUP_PROJECT_ID must be a positive project ID")
		}
	}
	h := store.WorkerHeartbeat{Worker: w.id, OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(),
		Slots: w.slots, ModelSlots: w.modelSlots, CISlots: w.ciSlots, Capabilities: caps}
	adapters := map[string]store.AdapterProbe{}
	for _, adapter := range []harnessAdapter{codexCLI{version: w.codexVersion}, w.claude} {
		name, version := adapter.identity()
		if version != "" {
			adapters[name] = store.AdapterProbe{Version: version, Operations: []string{"probe", "start", "resume", "events"}, Profiles: caps.Profiles(name)}
		}
	}
	probe, err := json.Marshal(adapters)
	if err != nil {
		return err
	}
	// Old control planes accept the same heartbeat body and ignore the extra query field.
	heartbeat := "/internal/workers/heartbeat?" + workerCapabilityQuery + "&adapter_probe=" + url.QueryEscape(string(probe))
	if w.codexAppServer {
		heartbeat += "&codex_interrupt=1"
	}
	beat := func(ctx context.Context) error {
		for ctx.Err() == nil {
			status, err := w.post(ctx, heartbeat, h, nil)
			if err == nil && status == http.StatusNoContent {
				return nil
			}
			if !temporary(status) {
				return fmt.Errorf("Worker heartbeat rejected (HTTP %d)", status)
			}
			wait(ctx)
		}
		return nil
	}
	if err := beat(ctx); err != nil {
		return err
	}
	child, stop := context.WithCancel(ctx)
	var maintenance sync.WaitGroup
	for _, run := range []func(context.Context){w.publishCIFiles, w.sweepLocalCI} {
		maintenance.Add(1)
		go func() { defer maintenance.Done(); run(child) }()
	}
	done := make(chan error, 1)
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-child.Done():
				done <- nil
				return
			case <-t.C:
				if err := beat(child); err != nil {
					stop()
					done <- err
					return
				}
			}
		}
	}()
	if err = w.replayReceipts(child); err == nil {
		err = w.runQueue(child, caps)
	}
	stop()
	maintenance.Wait()
	return errors.Join(err, <-done)
}

const workerCapabilityQuery = "supports_model_pause=1&supports_planner_pause=1&supports_rework_answers=1"

func (w *worker) runQueue(parent context.Context, caps store.WorkerCapabilities) error {
	ctx, stop := context.WithCancelCause(parent)
	var jobs sync.WaitGroup
	defer jobs.Wait()
	defer stop(nil)
	occupied := make(chan struct{}, w.slots)
	claim := struct {
		Worker        string `json:"worker"`
		WorkspaceRoot string `json:"workspace_root"`
		store.WorkerCapabilities
	}{w.id, w.workspaceRoot, caps}
	parked, err := w.parkedExecutions(ctx)
	if err != nil {
		return err
	}
	for ctx.Err() == nil {
		select {
		case occupied <- struct{}{}:
		case <-ctx.Done():
			continue
		}
		var job store.Job
		requested := time.Now()
		var saved *parkedCheckpoint
		if len(parked) > 0 {
			id := parked[0]
			parked = parked[1:]
			saved, err = w.readParkedCheckpoint(ctx, id)
			if err != nil {
				return err
			}
			if saved == nil {
				<-occupied
				continue
			}
			job = saved.Job
			log.Printf("checking saved checkpoint %d for execution %d", saved.Sequence, job.ExecutionID)
		} else {
			status, err := w.post(ctx, "/internal/claim?"+workerCapabilityQuery, claim, &job)
			if status == http.StatusNoContent || temporary(status) {
				<-occupied
				if status != http.StatusNoContent && ctx.Err() == nil {
					log.Printf("claim unavailable (HTTP %d), retrying", status)
				}
				wait(ctx)
				continue
			}
			if err != nil || status != http.StatusOK || job.ExecutionID <= 0 || job.LeaseToken == "" {
				return fmt.Errorf("invalid claim response (HTTP %d)", status)
			}
		}
		jobs.Add(1)
		go func() {
			defer jobs.Done()
			defer func() { <-occupied }()
			if err := w.runAttempt(ctx, job, requested, saved); err != nil {
				stop(err)
			}
		}()
	}
	if parent.Err() != nil {
		return nil
	}
	return context.Cause(ctx)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) == 2 && os.Args[1] == "preview-manager" {
		if err := servePreviewManager(ctx); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "verify-environment" {
		if err := verifyEnvironment(ctx, os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "review-test-plan" {
		if err := runTestCoverage(ctx, os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "plan-tests" {
		if err := runTestPlan(ctx, os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "validate-candidate" {
		if err := runCICandidate(ctx, os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "verify-ci-run" {
		if err := verifyCIRun(os.Getenv("FORGEOS_WORKSPACE_ROOT"), os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "review-candidate" {
		if err := runCandidateReview(ctx, os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	var role string
	if len(os.Args) == 2 {
		role = os.Args[1]
	}
	if len(os.Args) > 1 && role != "management" && role != "reviewer" && role != "test-agent" && role != "curator" {
		log.Fatal("usage: worker [curator|management|reviewer|test-agent|review-candidate|validate-candidate|verify-ci-run|plan-tests|review-test-plan|verify-environment|preview-manager]")
	}
	w, err := fromEnv(role)
	if err != nil {
		log.Fatal(err)
	}
	if role == "test-agent" {
		err = w.runTestAgent(ctx)
	} else if role == "curator" {
		err = w.runCurator(ctx)
	} else if role == "reviewer" {
		err = w.runReviewer(ctx)
	} else if role == "management" {
		err = w.runManagement(ctx)
	} else {
		err = w.run(ctx)
	}
	if err != nil {
		log.Fatal(err)
	}
}

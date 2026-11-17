package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/preview"
	"forgeos/internal/store"
	"forgeos/internal/validation"
)

type previewManager struct {
	docker      localDocker
	root, token string
	lifecycle   sync.Mutex
}

func servePreviewManager(ctx context.Context) error {
	addr, token, err := preview.ManagerConfig()
	if err != nil {
		return err
	}
	root := os.Getenv("FORGEOS_REPO_ROOT")
	if addr == "" || !filepath.IsAbs(root) || os.Geteuid() == 0 {
		return errors.New("configure a non-root preview manager, repository root and private listener")
	}
	d, err := openDocker(ctx)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return err
	}
	m := &previewManager{docker: d, root: root, token: token}
	srv := &http.Server{Handler: m, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	defer srv.Close()
	go func() { <-ctx.Done(); srv.Close() }()
	err = srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (m *previewManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+m.token)) != 1 {
		http.Error(w, "unauthorized", 401)
		return
	}
	if id := r.Header.Get(preview.InstanceHeader); id != "" {
		if !preview.InstanceID.MatchString(id) {
			http.NotFound(w, r)
			return
		}
		m.forward(w, r, id)
		return
	}
	id, ok := strings.CutPrefix(r.URL.Path, "/instances/")
	if !ok || !preview.InstanceID.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case "PUT":
		var lease preview.Lease
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		dec.DisallowUnknownFields()
		if dec.Decode(&lease) != nil || dec.Decode(new(any)) != io.EOF || !validation.FullCommit(lease.Commit) || lease.Repo == "" || time.Until(lease.Expires) <= 0 || time.Until(lease.Expires) > 24*time.Hour {
			http.Error(w, "invalid candidate lease", 400)
			return
		}
		m.lifecycle.Lock()
		defer m.lifecycle.Unlock()
		if err := m.start(r.Context(), id, lease); err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
	case "DELETE":
		m.lifecycle.Lock()
		defer m.lifecycle.Unlock()
		c, err := m.lookup(r.Context(), id)
		if err != nil || (c != nil && m.docker.command(r.Context(), "rm", "-fv", c.id).Run() != nil) {
			http.Error(w, "could not release preview", 503)
			return
		}
	default:
		w.WriteHeader(405)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type previewContainer struct {
	id, port, binding, name string
	expires                 time.Time
}

func (m *previewManager) lookup(ctx context.Context, id string) (*previewContainer, error) {
	raw, err := m.docker.command(ctx, "ps", "--no-trunc", "--filter", "label=forgeos.preview="+id, "--format", `{{.ID}} {{.Label "forgeos.port"}} {{.Label "forgeos.binding"}} {{.Label "forgeos.expires"}} {{.Names}}`).Output()
	if err != nil {
		return nil, errors.New("preview engine unavailable")
	}
	f := strings.Fields(string(raw))
	if len(f) == 0 {
		return nil, nil
	}
	if len(f) != 5 || len(f[0]) != 64 {
		return nil, errors.New("invalid preview container identity")
	}
	port, err := strconv.Atoi(f[1])
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("invalid preview port")
	}
	expires, err := time.Parse(time.RFC3339Nano, f[3])
	if err != nil {
		return nil, errors.New("invalid preview expiry")
	}
	return &previewContainer{f[0], f[1], f[2], f[4], expires}, nil
}

func (m *previewManager) start(ctx context.Context, id string, lease preview.Lease) error {
	ctx, cancel := context.WithDeadline(ctx, lease.Expires)
	defer cancel()
	repo, err := localrepo.ResolveRepo(ctx, m.root, lease.Repo)
	if err != nil {
		return err
	}
	run := environmentRun{EnvironmentRequest: store.EnvironmentRequest{Repo: localrepo.RelativePath(m.root, repo), Commit: lease.Commit, Environment: "local-mock"}, Network: "none"}
	if err = run.readConfig(ctx, repo); err != nil {
		return err
	}
	binding := fmt.Sprintf("%x", sha256.Sum256([]byte(run.Repo+"\x00"+lease.Commit)))
	live, err := m.lookup(ctx, id)
	if err != nil {
		return err
	}
	if live != nil {
		if live.binding != binding || !live.expires.Equal(lease.Expires) {
			return errors.New("preview identity already bound")
		}
		return run.waitReady(ctx, m.docker, live.name, nil)
	}
	if err = run.pinImage(ctx, m.docker); err != nil {
		return err
	}
	snapshot, release, err := ciSnapshot(ctx, repo, lease.Commit)
	if err != nil {
		return err
	}
	defer release()
	options, name, err := sandboxContainerOptions(snapshot, "none")
	if err != nil {
		return err
	}
	for _, label := range []string{"forgeos.preview=" + id, "forgeos.port=" + strconv.Itoa(run.Config.Port), "forgeos.binding=" + binding, "forgeos.expires=" + lease.Expires.UTC().Format(time.RFC3339Nano)} {
		options = append(options, "--label", label)
	}
	lifetime := int(math.Ceil(time.Until(lease.Expires).Seconds()))
	if lifetime < 1 {
		return errors.New("preview lease expired")
	}
	if err = m.docker.command(ctx, run.containerArgs(options, lifetime)...).Run(); err == nil {
		if err = run.waitReady(ctx, m.docker, name, nil); err == nil {
			return nil
		}
	} else {
		err = errors.New("could not launch preview")
	}
	cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
	defer done()
	_ = m.docker.command(cleanup, "rm", "-fv", name).Run()
	return err
}

func (m *previewManager) forward(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
	default:
		w.WriteHeader(405)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	c, err := m.lookup(ctx, id)
	if err != nil {
		http.Error(w, "preview engine unavailable", 503)
		return
	}
	if c == nil || !time.Now().Before(c.expires) {
		http.NotFound(w, r)
		return
	}
	ctx, endLease := context.WithDeadline(ctx, c.expires)
	defer endLease()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		http.Error(w, "invalid preview request body", 413)
		return
	}
	args := []string{"exec", "-i", c.id, "/usr/bin/curl", "--disable", "--silent", "--noproxy", "*", "--proto", "=http", "--globoff", "--path-as-is", "--http1.1", "--include", "--raw", "--max-time", "18", "--header", "Expect:"}
	if r.Method == "HEAD" {
		args = append(args, "--head")
	} else {
		args = append(args, "--request", r.Method)
	}
	headers := http.Header{}
	preview.CopyRequestHeaders(headers, r.Header)
	for k, values := range headers {
		for _, v := range values {
			args = append(args, "--header", k+": "+v)
		}
	}
	if len(body) > 0 {
		args = append(args, "--data-binary", "@-")
	}
	target := "http://127.0.0.1:" + c.port + r.URL.RequestURI()
	cmd := m.docker.command(ctx, append(args, "--url", target)...)
	cmd.Stdin = bytes.NewReader(body)
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		http.Error(w, "preview transport unavailable", 502)
		return
	}
	const limit = 16 << 20
	raw, err := io.ReadAll(io.LimitReader(out, limit+1))
	if len(raw) > limit || err != nil {
		cancel()
		_ = cmd.Wait()
		http.Error(w, "preview response exceeds transport limit", 502)
		return
	}
	if cmd.Wait() != nil {
		http.Error(w, "application unavailable", 502)
		return
	}
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(raw)), r)
	if err != nil {
		http.Error(w, "invalid application response", 502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 599 {
		http.Error(w, "unsupported application response", 502)
		return
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "incomplete application response", 502)
		return
	}
	preview.CopyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if r.Method != "HEAD" {
		_, _ = w.Write(payload)
	}
}

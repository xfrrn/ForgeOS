package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"forgeos/internal/store"
	"io"
	"maps"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type workerGrant struct {
	environments map[environmentGrant]bool
	id           string
	digest       [sha256.Size]byte
	projects     []int64
}
type environmentGrant struct {
	Project     int64  `json:"project_id"`
	Environment string `json:"environment"`
}
type workerRegistry struct {
	mu       sync.RWMutex
	grants   []workerGrant
	file     string
	reserved map[[sha256.Size]byte]bool
}

func legacyWorker(token string) *workerRegistry {
	return &workerRegistry{grants: []workerGrant{{digest: sha256.Sum256([]byte(token))}}}
}

func loadWorkers(file, legacy, owner, manager string) (*workerRegistry, error) {
	if strings.TrimSpace(owner) == "" {
		return nil, errors.New("set FORGEOS_OWNER_TOKEN")
	}
	if file == "" {
		if strings.TrimSpace(legacy) == "" || legacy == owner || legacy == manager {
			return nil, errors.New("set a distinct FORGEOS_WORKER_TOKEN or configure FORGEOS_WORKERS_FILE")
		}
		return legacyWorker(legacy), nil
	}
	if legacy != "" {
		return nil, errors.New("configure either FORGEOS_WORKERS_FILE or FORGEOS_WORKER_TOKEN on the control plane")
	}
	a := &workerRegistry{file: file, reserved: map[[sha256.Size]byte]bool{}}
	for _, token := range []string{"", owner, manager, os.Getenv("FORGEOS_REVIEWER_TOKEN"), os.Getenv("FORGEOS_TEST_AGENT_TOKEN")} {
		a.reserved[sha256.Sum256([]byte(token))] = true
	}
	if err := a.reload(); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *workerRegistry) snapshot() []workerGrant {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.grants
}

func (a *workerRegistry) reload() error {
	if a.file == "" {
		return errors.New("reload requires FORGEOS_WORKERS_FILE")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	doc, err := os.ReadFile(a.file)
	if err != nil {
		return errors.New("could not read FORGEOS_WORKERS_FILE")
	}
	var rows []struct {
		ID           string             `json:"worker_id"`
		Hash         string             `json:"token_sha256"`
		NextHash     string             `json:"next_token_sha256"`
		Projects     []int64            `json:"projects"`
		Environments []environmentGrant `json:"environments"`
	}
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&rows); err != nil || dec.Decode(new(any)) != io.EOF || len(rows) == 0 {
		return errors.New("invalid Worker grants file")
	}
	used := maps.Clone(a.reserved)
	ids := map[string]bool{}
	var access []workerGrant
	for _, r := range rows {
		raw, err := hex.DecodeString(r.Hash)
		if err != nil || len(raw) != sha256.Size || len(r.ID) == 0 || len(r.ID) > 128 || strings.Trim(r.ID, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._-") != "" || ids[r.ID] || len(r.Projects) == 0 {
			return errors.New("each Worker needs a unique ID, token digest and explicit projects")
		}
		grant := workerGrant{id: r.ID, digest: [sha256.Size]byte(raw), projects: r.Projects}
		if used[grant.digest] {
			return errors.New("Worker credentials must be distinct from other credentials")
		}
		projects := map[int64]bool{}
		for _, id := range grant.projects {
			if id < 1 || projects[id] {
				return errors.New("Worker project IDs must be positive and distinct")
			}
			projects[id] = true
		}
		grant.environments = map[environmentGrant]bool{}
		for _, scope := range r.Environments {
			if !projects[scope.Project] || !store.ValidEnvironment(scope.Environment) || grant.environments[scope] {
				return errors.New("Worker environment grants must be distinct and belong to an allowed project")
			}
			grant.environments[scope] = true
		}
		ids[r.ID], used[grant.digest] = true, true
		access = append(access, grant)
		if r.NextHash != "" {
			next, err := hex.DecodeString(r.NextHash)
			if err != nil || len(next) != sha256.Size {
				return errors.New("invalid next Worker token digest")
			}
			grant.digest = [sha256.Size]byte(next)
			if used[grant.digest] {
				return errors.New("Worker credentials must be distinct from other credentials")
			}
			used[grant.digest] = true
			access = append(access, grant)
		}
	}
	a.grants = access
	return nil
}

func (a *workerRegistry) match(token string) *workerGrant {
	hash := sha256.Sum256([]byte(token))
	var grant *workerGrant
	entries := a.snapshot()
	for i := range entries {
		if subtle.ConstantTimeCompare(hash[:], entries[i].digest[:]) == 1 {
			grant = &entries[i]
		}
	}
	return grant
}

func (a *workerRegistry) authorize(next func(http.ResponseWriter, *http.Request, *workerGrant)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		grant := a.match(token)
		if !ok || token == "" || grant == nil {
			w.Header().Set("WWW-Authenticate", "Bearer")
			problem(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		next(w, r.WithContext(ctx), grant)
	}
}

func (a *workerRegistry) execution(data *store.Store, next http.HandlerFunc) http.HandlerFunc {
	return a.authorize(func(w http.ResponseWriter, r *http.Request, grant *workerGrant) {
		if grant.id != "" {
			id, ok := pathID(w, r)
			if !ok {
				return
			}
			allowed, err := data.WorkerOwnsExecution(r.Context(), id, grant.id, grant.projects)
			if err != nil {
				respond(w, 0, nil, err)
				return
			}
			if !allowed {
				problem(w, http.StatusForbidden, "execution is outside this Worker's grant")
				return
			}
		}
		next(w, r)
	})
}

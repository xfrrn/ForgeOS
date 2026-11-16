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
	"net/http"
	"os"
	"strings"
	"time"
)

type workerGrant struct {
	id       string
	digest   [sha256.Size]byte
	projects []int64
}
type workerAccess []workerGrant

func legacyWorker(token string) workerAccess {
	return workerAccess{{digest: sha256.Sum256([]byte(token))}}
}

func loadWorkers(file, legacy, owner, manager string) (workerAccess, error) {
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
	doc, err := os.ReadFile(file)
	if err != nil {
		return nil, errors.New("could not read FORGEOS_WORKERS_FILE")
	}
	var rows []struct {
		ID       string  `json:"worker_id"`
		Hash     string  `json:"token_sha256"`
		Projects []int64 `json:"projects"`
	}
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&rows); err != nil || dec.Decode(new(any)) != io.EOF || len(rows) == 0 {
		return nil, errors.New("invalid Worker grants file")
	}
	used := map[[sha256.Size]byte]bool{sha256.Sum256(nil): true, sha256.Sum256([]byte(owner)): true, sha256.Sum256([]byte(manager)): true}
	ids := map[string]bool{}
	var access workerAccess
	for _, r := range rows {
		raw, err := hex.DecodeString(r.Hash)
		if err != nil || len(raw) != sha256.Size || len(r.ID) == 0 || len(r.ID) > 128 || strings.Trim(r.ID, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._-") != "" || ids[r.ID] || len(r.Projects) == 0 {
			return nil, errors.New("each Worker needs a unique ID, token digest and explicit projects")
		}
		grant := workerGrant{id: r.ID, digest: [sha256.Size]byte(raw), projects: r.Projects}
		if used[grant.digest] {
			return nil, errors.New("Worker credentials must be distinct from other credentials")
		}
		projects := map[int64]bool{}
		for _, id := range grant.projects {
			if id < 1 || projects[id] {
				return nil, errors.New("Worker project IDs must be positive and distinct")
			}
			projects[id] = true
		}
		ids[r.ID], used[grant.digest] = true, true
		access = append(access, grant)
	}
	return access, nil
}

func (a workerAccess) match(token string) *workerGrant {
	hash := sha256.Sum256([]byte(token))
	var grant *workerGrant
	for i := range a {
		if subtle.ConstantTimeCompare(hash[:], a[i].digest[:]) == 1 {
			grant = &a[i]
		}
	}
	return grant
}

func (a workerAccess) authorize(next func(http.ResponseWriter, *http.Request, *workerGrant)) http.HandlerFunc {
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

func (a workerAccess) execution(data *store.Store, next http.HandlerFunc) http.HandlerFunc {
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

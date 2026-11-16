package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"forgeos/internal/store"
)

var deliveryGUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var githubEventName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func registerGitHubWebhooks(mux *http.ServeMux, data *store.Store, owner string, pollers map[int64]*issuePoller) {
	for _, p := range pollers {
		if len(p.hookSecret) > 0 {
			mux.HandleFunc(fmt.Sprintf("POST /hooks/github/%d", p.projectID), p.webhook(data))
		}
	}
	mux.HandleFunc("GET /projects/{id}/github-events", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var before int64
		if raw := r.URL.Query().Get("before"); raw != "" {
			var err error
			before, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || before < 1 {
				problem(w, 400, "before must be a positive event ID")
				return
			}
		}
		rows, err := data.GitHubEvents(r.Context(), id, before)
		respond(w, 200, rows, err)
	}))
	mux.HandleFunc("GET /projects/{id}/github-events/{delivery}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		delivery := r.PathValue("delivery")
		if !deliveryGUID.MatchString(delivery) {
			problem(w, 400, "invalid GitHub delivery ID")
			return
		}
		raw, err := data.GitHubEventBody(r.Context(), id, delivery)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Write(raw)
	}))
}

func (p *issuePoller) webhook(data *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		signature := r.Header.Get("X-Hub-Signature-256")
		rawSig, ok := strings.CutPrefix(signature, "sha256=")
		if !ok || len(rawSig) != sha256.Size*2 {
			problem(w, 401, "invalid webhook signature")
			return
		}
		sig, err := hex.DecodeString(rawSig)
		if err != nil {
			problem(w, 401, "invalid webhook signature")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 25<<20))
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				problem(w, 413, "GitHub webhook payload exceeds 25 MiB")
			} else {
				problem(w, 400, "could not read webhook body")
			}
			return
		}
		mac := hmac.New(sha256.New, p.hookSecret)
		mac.Write(body)
		if !hmac.Equal(sig, mac.Sum(nil)) {
			problem(w, 401, "invalid webhook signature")
			return
		}
		delivery, event := r.Header.Get("X-GitHub-Delivery"), r.Header.Get("X-GitHub-Event")
		var payload struct {
			Repository struct{ ID int64 } `json:"repository"`
		}
		if !deliveryGUID.MatchString(delivery) || !githubEventName.MatchString(event) ||
			json.Unmarshal(body, &payload) != nil || payload.Repository.ID <= 0 {
			problem(w, 400, "provide a repository event with a GitHub delivery ID")
			return
		}
		created, err := data.RecordGitHubEvent(r.Context(), p.projectID, payload.Repository.ID, delivery, event, body)
		switch {
		case errors.Is(err, store.ErrWebhookRepository):
			problem(w, 403, "repository does not match this project")
		case errors.Is(err, store.ErrRepositoryUnbound):
			problem(w, 503, "waiting for GitHub repository binding")
		case err != nil:
			respond(w, 0, nil, err)
		case !created:
			w.WriteHeader(http.StatusOK)
		default:
			if event == "issues" || event == "issue_comment" {
				select {
				case p.wake <- struct{}{}:
				default:
				}
			}
			w.WriteHeader(http.StatusAccepted)
		}
	}
}

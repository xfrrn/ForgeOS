package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"forgeos/internal/agents"
	"forgeos/internal/chatkernel"
	"forgeos/internal/store"
	"github.com/google/uuid"
)

type knowledgeModelConnections interface {
	ModelConnection(context.Context, uuid.UUID) (chatkernel.Provider, error)
}

type knowledgeRunner struct {
	data  *store.Store
	vault knowledgeModelConnections
}

// Each role has one synchronous model turn at most. A management turn only
// draws from the two knowledge queues; other Management work belongs to Workers.
func runKnowledgeModels(ctx context.Context, data *store.Store, vault chatProviders) {
	if vault == nil {
		return
	}
	r := knowledgeRunner{data, vault}
	var group sync.WaitGroup
	for _, role := range []string{"curator", "management"} {
		group.Add(1)
		go func() { defer group.Done(); r.poll(ctx, role) }()
	}
	group.Wait()
}

type knowledgePollState struct {
	bindingRevision int64
	modelRevision   uuid.UUID
	preferShare     bool
	failures        int
	retryAt         time.Time
}

func (s *knowledgePollState) failed(now time.Time) {
	delays := [...]time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, time.Hour}
	s.retryAt = now.Add(delays[min(s.failures, len(delays)-1)])
	s.failures = min(s.failures+1, len(delays))
}

func (r knowledgeRunner) poll(ctx context.Context, role string) {
	states := map[int64]knowledgePollState{}
	for ctx.Err() == nil {
		wait := 10 * time.Second
		bindings, err := r.data.ConfiguredKnowledgeRoleModels(ctx)
		if err == nil {
			next := make(map[int64]knowledgePollState)
			for _, b := range bindings {
				if ctx.Err() != nil {
					return
				}
				if b.Role != role {
					continue
				}
				state := states[b.ProjectID]
				if state.bindingRevision != b.Revision || state.modelRevision != b.ModelRevision {
					state = knowledgePollState{bindingRevision: b.Revision, modelRevision: b.ModelRevision}
				}
				if !time.Now().Before(state.retryAt) {
					worked, err := r.turn(ctx, b, &state.preferShare)
					if err != nil && ctx.Err() == nil {
						// Bad input and invalid output are quarantined per job. Only
						// connection failures or claim errors hold the entire binding.
						if !worked || errors.Is(err, errKnowledgeConnection) || errors.Is(err, errKnowledgeUpstream) {
							state.failed(time.Now())
						} else {
							wait = time.Second
						}
						log.Printf("knowledge model turn failed for project %d, role %s: %s", b.ProjectID, role, knowledgeDiagnostic(err))
					} else if worked {
						state.failures = 0
						wait = time.Second
					}
				}
				next[b.ProjectID] = state
			}
			states = next
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

type knowledgeTurn struct {
	kind     string
	id       int64
	lease    string
	until    *time.Time
	source   json.RawMessage
	curation *store.CurationJob
	share    *store.ShareReviewInput
}

func (r knowledgeRunner) claim(ctx context.Context, b store.KnowledgeRoleModel, preferShare bool) (*knowledgeTurn, error) {
	kinds := []string{"extract"}
	if b.Role == "management" {
		kinds = []string{"curation_review", "share_review"}
		if preferShare {
			kinds[0], kinds[1] = kinds[1], kinds[0]
		}
	} else if b.Role != "curator" {
		return nil, store.ErrConflict
	}
	for _, kind := range kinds {
		if kind == "share_review" {
			job, err := r.data.ClaimShareReview(ctx, b.ProjectID, true)
			if err != nil {
				return nil, err
			}
			if job != nil {
				return &knowledgeTurn{kind: kind, id: job.Share.ID, lease: job.Lease, until: job.LeaseUntil, share: job.Share}, nil
			}
		} else {
			job, err := r.data.ClaimCuration(ctx, b.ProjectID, kind == "curation_review", true)
			if err != nil {
				return nil, err
			}
			if job != nil {
				return &knowledgeTurn{kind: kind, id: job.ID, lease: job.Lease, until: job.LeaseUntil, source: job.Material, curation: job}, nil
			}
		}
	}
	return nil, nil
}

func (r knowledgeRunner) turn(ctx context.Context, b store.KnowledgeRoleModel, preferShare *bool) (bool, error) {
	ctx = b.WithContext(ctx)
	job, err := r.claim(ctx, b, *preferShare)
	if err != nil || job == nil {
		return false, err
	}
	*preferShare = job.kind == "curation_review"
	deadline := time.Now().Add(5 * time.Minute)
	if job.until != nil && job.until.Before(deadline) {
		deadline = *job.until
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if err = r.data.ReserveKnowledgeModelAttempt(ctx, b, job.kind, job.id, job.lease); err != nil {
		return true, err
	}
	err = r.runReserved(ctx, b, job)
	// An upstream timeout can exhaust the turn deadline. Record its safe
	// diagnostic with a fresh bounded context even on shutdown.
	save, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancelSave()
	recordErr := r.data.FinishKnowledgeModelAttempt(save, b, job.kind, job.id, job.lease, knowledgeDiagnostic(err))
	if err != nil {
		return true, err
	}
	return true, recordErr
}

func (r knowledgeRunner) runReserved(ctx context.Context, b store.KnowledgeRoleModel, job *knowledgeTurn) error {
	p, err := r.vault.ModelConnection(ctx, b.ModelRevision)
	if err != nil || !p.Valid() {
		return errKnowledgeConnection
	}
	current, err := r.data.KnowledgeRoleModelCurrent(ctx, b)
	if err != nil {
		return err
	}
	if !current {
		return store.ErrConflict
	}
	return r.complete(ctx, b, job, p)
}

var errInvalidKnowledgeObject = errors.New("invalid knowledge model result")
var errKnowledgeConnection = errors.New("knowledge model connection unavailable")
var errKnowledgeUpstream = errors.New("knowledge model request failed")

func knowledgeDiagnostic(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errKnowledgeConnection):
		return "无法读取模型连接，请检查模型目录和凭据。"
	case errors.Is(err, errKnowledgeUpstream):
		return "模型请求失败，请检查连接地址、凭据和服务可用性。"
	case errors.Is(err, errInvalidKnowledgeObject):
		return "模型返回的结构、审核级别或原文引句不符合要求。"
	case errors.Is(err, store.ErrConflict):
		return "绑定、租约或来源校验失败，本次结果未采用。"
	default:
		return "知识任务执行失败，本次结果未采用。"
	}
}

func knowledgeCallError(err error) error {
	if errors.Is(err, chatkernel.ErrInvalidObject) {
		return errInvalidKnowledgeObject
	}
	return errKnowledgeUpstream
}

func decodeKnowledgeObject(raw json.RawMessage, dest any) error {
	if !json.Valid(raw) {
		return errInvalidKnowledgeObject
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return errInvalidKnowledgeObject
	}
	return nil
}

func (r knowledgeRunner) complete(ctx context.Context, b store.KnowledgeRoleModel, job *knowledgeTurn, p chatkernel.Provider) error {
	session := "api-" + uuid.NewString()
	if job.kind == "extract" {
		raw, err := chatkernel.CompleteObject(ctx, p, "knowledge_extraction", agents.ExtractionSchema, agents.Builtin("curator"), job.source)
		if err != nil {
			return knowledgeCallError(err)
		}
		var out struct {
			Facts []store.KnowledgeFact `json:"facts"`
		}
		if err := decodeKnowledgeObject(raw, &out); err != nil {
			return err
		}
		var source struct{ Content string }
		if err := json.Unmarshal(job.source, &source); err != nil {
			return err
		}
		extraction := store.Extraction{Session: session, Facts: out.Facts}
		if !extraction.HasSourceQuotes(source.Content) {
			return errInvalidKnowledgeObject
		}
		// The store verifies every quote against the locked source and rechecks
		// the model binding in the very transaction that records this extraction.
		return r.data.RecordExtraction(ctx, b.ProjectID, job.id, job.lease, extraction)
	}
	prompt := agents.Builtin("knowledge-review")
	var input any
	if job.share != nil {
		s := job.share
		input = map[string]string{"title": s.Title, "content": s.Content, "original_title": s.OriginalTitle, "original_source": s.OriginalSource, "original_content": s.OriginalContent}
		prompt = agents.Builtin("share") + " Report level D3 for this disclosure review. Leave validation_minimum empty."
	} else {
		input = struct {
			Source json.RawMessage       `json:"source"`
			Facts  []store.KnowledgeFact `json:"facts"`
		}{job.source, job.curation.Extraction.Facts}
	}
	doc, err := json.Marshal(input)
	if err != nil {
		return err
	}
	raw, err := chatkernel.CompleteObject(ctx, p, "knowledge_review", agents.ReviewSchema, prompt, doc)
	if err != nil {
		return knowledgeCallError(err)
	}
	var out struct {
		Level             string `json:"level"`
		Verdict           string `json:"verdict"`
		Rationale         string `json:"rationale"`
		Evidence          string `json:"evidence"`
		ValidationMinimum string `json:"validation_minimum"`
	}
	if err := decodeKnowledgeObject(raw, &out); err != nil {
		return err
	}
	if out.ValidationMinimum != "" || (job.share != nil && out.Level != "D3") {
		return errInvalidKnowledgeObject
	}
	review := store.ManagementReview{Level: out.Level, Verdict: out.Verdict, Rationale: out.Rationale, Evidence: out.Evidence, SessionID: session}
	if job.share != nil {
		return r.data.ReviewShare(ctx, b.ProjectID, job.id, job.lease, review)
	}
	return r.data.ReviewCuration(ctx, b.ProjectID, job.id, job.lease, review)
}

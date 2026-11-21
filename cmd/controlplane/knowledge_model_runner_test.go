package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"forgeos/internal/chatkernel"
	"forgeos/internal/store"
	"forgeos/internal/testdb"
	"github.com/google/uuid"
)

func TestDirectExtractionRequiresExplicitFindingList(t *testing.T) {
	for _, c := range []struct {
		raw   string
		valid bool
	}{{"{}", false}, {`{"facts":null}`, false}, {`{"facts":[]}`, true}} {
		var out store.Extraction
		if err := decodeKnowledgeObject(json.RawMessage(c.raw), &out); err != nil || out.HasSourceQuotes("source") != c.valid {
			t.Errorf("incorrect empty extraction for %s", c.raw)
		}
	}
}

func TestDirectKnowledgeModelsExtractReviewAndKeepSharesPrivate(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	data := store.New(db)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(data.Migrate(ctx))
	a, err := data.CreateProject(ctx, "source")
	must(err)
	b, err := data.CreateProject(ctx, "other")
	must(err)
	note := store.MaterialInput{Kind: "material", Title: "Operator handoff", Source: "fixture", Content: "The counter resets at noon."}
	_, err = data.SaveMaterial(ctx, a.ID, "handoff", note)
	must(err)
	other := note
	other.Content = "UNBOUND_PRIVATE_DATA"
	_, err = data.SaveMaterial(ctx, b.ID, "handoff", other)
	must(err)
	vault := testChatProviders{}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var in struct {
			Model    string
			Messages []struct{ Role, Content string }
			Tools    json.RawMessage
		}
		if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
			t.Error(err)
			return
		}
		src := in.Messages[len(in.Messages)-1]
		if src.Role != "user" || in.Tools != nil || strings.Contains(src.Content, other.Content) {
			t.Error("invalid scope or tool request")
		}
		var result string
		switch in.Model {
		case "extractor":
			if req.Header.Get("Authorization") != "Bearer curator-key" {
				t.Error("wrong extraction connection")
			}
			result = `{"facts":[{"statement":"Counter resets at noon","quote":"resets at noon"}]}`
		case "reviewer":
			if req.Header.Get("Authorization") != "Bearer management-key" {
				t.Error("wrong review connection")
			}
			if strings.Contains(src.Content, "original_content") {
				result = `{"level":"D3","verdict":"approve","rationale":"Safe copy","evidence":"Reset time","validation_minimum":""}`
			} else if strings.Contains(src.Content, "policy proposal") {
				result = `{"level":"D2","verdict":"approve","rationale":"Only the owner can confirm the proposed policy","evidence":"policy proposal","validation_minimum":""}`
			} else {
				result = `{"level":"D1","verdict":"approve","rationale":"有原文支持。","evidence":"原文的「resets at noon」给出了重置时间。","validation_minimum":""}`
			}
		default:
			t.Error("unknown model")
			return
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": result}}}})
	}))
	defer server.Close()
	bind := func(role, model string) {
		id, rev := uuid.New(), uuid.New()
		p := chatkernel.Provider{Model: model, BaseURL: server.URL, OutputFormat: "json_object", APIKey: role + "-key"}
		m := store.ChatModel{ID: id, Revision: rev, Name: model, Model: model, BaseURL: p.BaseURL, OutputFormat: p.OutputFormat}
		must(data.SaveChatModel(ctx, m, uuid.Nil, func() error { return vault.SetModelConnection(ctx, rev, p) }))
		_, err := data.SetKnowledgeModel(ctx, a.ID, role, 0, &id)
		must(err)
	}
	bind("curator", "extractor")
	bind("management", "reviewer")
	bindings, err := data.ConfiguredKnowledgeRoleModels(ctx)
	must(err)
	roles := map[string]store.KnowledgeRoleModel{}
	for _, b := range bindings {
		roles[b.Role] = b
	}
	r := knowledgeRunner{data, vault}
	preferShare := false
	turn := func(role string) {
		t.Helper()
		worked, err := r.turn(ctx, roles[role], &preferShare)
		must(err)
		if !worked {
			t.Fatal("no job", role)
		}
	}
	turn("curator")
	turn("management")
	var facts int
	must(db.QueryRow(`SELECT count(*) FROM experience_revision_facts WHERE project_id=$1 AND is_effective`, a.ID).Scan(&facts))
	if facts != 1 {
		t.Fatal("missing D1 experience", facts)
	}
	copy, err := data.DraftShare(ctx, a.ID, store.ShareDraft{MaterialKey: "handoff", Revision: 1, Recipient: b.ID, Title: "Reset time", Content: "Define a predictable reset interval."})
	must(err)
	var receipt struct {
		ID     int64
		Digest string
	}
	must(json.Unmarshal(copy, &receipt))
	turn("management")
	received, err := data.SharedExperience(ctx, b.ID, 0)
	must(err)
	if string(received) != "[]" {
		t.Fatal("model review published without owner")
	}
	_, err = data.SetSharePublished(ctx, a.ID, receipt.ID, receipt.Digest, true)
	must(err)
	received, err = data.SharedExperience(ctx, b.ID, 0)
	must(err)
	if !strings.Contains(string(received), "Reset time") || strings.Contains(string(received), "resets at noon") {
		t.Fatal("invalid public copy")
	}
	note.Content += " The owner asks to consider a policy proposal."
	_, err = data.SaveMaterial(ctx, a.ID, "policy", note)
	must(err)
	turn("curator")
	turn("management")
	var policyID int64
	must(db.QueryRow(`SELECT id FROM material_curations WHERE key='policy' AND review->>'level'='D2' AND review->>'verdict'='approve'`).Scan(&policyID))
	must(db.QueryRow(`SELECT count(*) FROM experience_revision_facts WHERE curation_id=$1 AND is_effective`, policyID).Scan(&facts))
	if facts != 0 {
		t.Fatal("approved D2 proposal became an effective experience")
	}
	_, err = data.ProposeDecision(ctx, a.ID, store.DecisionProposal{RequestKey: "from-knowledge-proposal", Statement: "Consider a reset policy", Validity: "Test project", Curation: &store.DecisionOrigin{ID: policyID, Fact: 1}})
	must(err)
	impostor := roles["curator"]
	impostor.ProjectID = b.ID
	if ok, err := r.turn(ctx, impostor, &preferShare); err != nil || ok {
		t.Fatal("read outside binding", err)
	}
	if calls.Load() != 5 {
		t.Fatal("unexpected number of calls", calls.Load())
	}
}

func TestDirectKnowledgeModelsBoundRetriesAndRejectChangedBinding(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	data := store.New(db)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(data.Migrate(ctx))
	p, err := data.CreateProject(ctx, "retry")
	must(err)
	var calls atomic.Int32
	var disable atomic.Bool
	var binding store.KnowledgeRoleModel
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if disable.Load() {
			_, err := data.SetKnowledgeModel(ctx, p.ID, "curator", binding.Revision, nil)
			if err != nil {
				t.Error(err)
			}
		}
		var in struct{ Messages []struct{ Content string } }
		if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
			t.Error(err)
			return
		}
		body := in.Messages[len(in.Messages)-1].Content
		result := `{"facts":[]}`
		if strings.Contains(body, "BROKEN") {
			result = `{"facts":[{"statement":"invented","quote":"NOT_IN_SOURCE"}]}`
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": result}}}})
	}))
	defer server.Close()
	vault := testChatProviders{}
	model := store.ChatModel{ID: uuid.New(), Revision: uuid.New(), Name: "a", Model: "a", BaseURL: server.URL}
	must(data.SaveChatModel(ctx, model, uuid.Nil, func() error {
		return vault.SetModelConnection(ctx, model.Revision, chatkernel.Provider{Model: "a", APIKey: "private-key", BaseURL: model.BaseURL})
	}))
	_, err = data.SetKnowledgeModel(ctx, p.ID, "curator", 0, &model.ID)
	must(err)
	list, err := data.ConfiguredKnowledgeRoleModels(ctx)
	must(err)
	binding = list[0]
	must(data.SetModelBudgets(ctx, []store.ModelBudget{{Name: "one-slot", Harness: "codex", Slots: 1,
		AgentRoles: []store.ModelBudgetRole{{Project: p.ID, Role: "curator"}}}}))
	add := func(key, content string) {
		_, err := data.SaveMaterial(ctx, p.ID, key, store.MaterialInput{Kind: "material", Title: "input", Source: "test", Content: content})
		must(err)
	}
	add("bad", "BROKEN")
	add("next", "anything")
	r := knowledgeRunner{data, vault}
	prefer := false
	invalid := func() {
		t.Helper()
		worked, err := r.turn(ctx, binding, &prefer)
		if !worked || err == nil {
			t.Fatal("model output accepted", worked, err)
		}
	}
	invalid()
	if worked, err := r.turn(ctx, binding, &prefer); !worked || err != nil {
		t.Fatal("unrelated input stuck behind bad quote", worked, err)
	}
	expire := func() {
		_, err := db.Exec(`UPDATE material_curations SET extraction_until=now()-interval '1 minute' WHERE key='bad';
			UPDATE knowledge_model_attempts SET retry_at=now()-interval '1 minute';`)
		must(err)
	}
	for range 2 {
		expire()
		invalid()
	}
	expire()
	if worked, err := r.turn(ctx, binding, &prefer); worked || err != nil {
		t.Fatal("billed an exhausted job", worked, err)
	}
	settings, err := data.KnowledgeModelSettings(ctx, p.ID)
	must(err)
	message := settings["curator"].LastFailure
	if !strings.Contains(message, "尝试上限") || strings.Contains(message, "NOT_IN_SOURCE") || strings.Contains(message, "private-key") {
		t.Fatal("missing or unsafe diagnostic", message)
	}
	add("rotate", "new material")
	disable.Store(true)
	if worked, err := r.turn(ctx, binding, &prefer); !worked || !errors.Is(err, store.ErrConflict) {
		t.Fatal("committed after binding removal", worked, err)
	}
	var persisted bool
	must(db.QueryRow(`SELECT extraction IS NOT NULL FROM material_curations WHERE key='rotate'`).Scan(&persisted))
	if persisted || calls.Load() != 5 {
		t.Fatal("incorrect completed jobs or call count", persisted, calls.Load())
	}
	state := knowledgePollState{}
	now := time.Now()
	for range 12 {
		state.failed(now)
	}
	if state.retryAt.Sub(now) != time.Hour {
		t.Fatal("unbounded binding retries", state.retryAt)
	}
}

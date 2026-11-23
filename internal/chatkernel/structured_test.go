package chatkernel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKnowledgeModelUsesConfiguredJSONAndNoTools(t *testing.T) {
	for _, format := range []string{"json_object", "json_schema"} {
		t.Run(format, func(t *testing.T) {
			service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model    string
					Messages []struct{ Role, Content string }
					Tools    json.RawMessage
					Format   struct{ Type string } `json:"response_format"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				last := body.Messages[len(body.Messages)-1]
				if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer local-test" ||
					body.Model != "knowledge-model" || body.Format.Type != format || body.Tools != nil ||
					last.Role != "user" || last.Content != `{"source":"引句"}` {
					t.Error("unexpected knowledge model request")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"facts\":[]}"}}]}`))
			}))
			defer service.Close()
			provider := Provider{Model: "knowledge-model", BaseURL: service.URL + "/v1", APIKey: "local-test", OutputFormat: format}
			out, err := CompleteObject(context.Background(), provider, "extract", `{"type":"object"}`, "提取事实", json.RawMessage(`{"source":"引句"}`))
			if err != nil || string(out) != `{"facts":[]}` {
				t.Fatal(string(out), err)
			}
		})
	}
}

func TestKnowledgeModelKeepsCredentialsOnOriginalEndpoint(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("followed model redirect") }))
	defer other.Close()
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer service.Close()
	_, err := CompleteObject(context.Background(), Provider{BaseURL: service.URL, Model: "test", APIKey: "secret-example"},
		"review", `{"type":"object"}`, "instructions", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "307") || strings.Contains(err.Error(), "secret-example") {
		t.Fatal(err)
	}
}

func TestProposalAnswerUsesOnlyFrozenInput(t *testing.T) {
	const prompt = "one frozen proposal"
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string
			Messages []struct{ Role, Content string }
			Tools    json.RawMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body.Model != "proposal-model" || len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != prompt || body.Tools != nil {
			t.Error("unexpected proposal request")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"draft"}}]}`))
	}))
	defer service.Close()
	cfg := Config{provider: Provider{Model: "proposal-model", BaseURL: service.URL + "/v1", APIKey: "local-test"}}
	answer, err := cfg.ProposalAnswer(context.Background(), prompt)
	if err != nil || answer != "draft" {
		t.Fatal(answer, err)
	}
}

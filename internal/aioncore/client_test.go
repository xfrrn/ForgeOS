package aioncore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestConfigureModelAppliesOnNextTurn(t *testing.T) {
	mode := "write"
	modelSet := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/conversations/c/runtime/ensure":
			fmt.Fprintf(w, `{"success":true,"data":{"config_options":[{"id":"model","current_value":"old","options":[{"value":"next"}]},{"id":"mode","current_value":%q,"options":[{"value":"read-only"}]}]}}`, mode)
		case "/api/conversations/c/config-options/model":
			modelSet = true
			fmt.Fprint(w, `{"success":true,"data":{"confirmation":"pending_next_turn"}}`)
		case "/api/conversations/c/config-options/mode":
			if !modelSet {
				t.Error("permissions set before model selection")
			}
			mode = "read-only"
			fmt.Fprint(w, `{"success":true,"data":{}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	httpClient := server.Client()
	httpClient.Jar, _ = cookiejar.New(nil)
	client := Client{base: base, http: httpClient}
	if err := client.Configure(context.Background(), "c", "codex", "next"); err != nil {
		t.Fatal(err)
	}
	if mode != "read-only" || !modelSet {
		t.Fatal("incomplete runtime configuration")
	}
	if err := client.Configure(context.Background(), "c", "codex", "missing"); err == nil {
		t.Fatal("unavailable model accepted")
	}
}

func TestCancelWaitsForIdleAndPinsTurn(t *testing.T) {
	checked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/conversations/old/cancel":
			var body map[string]string
			if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || body["turn_id"] != "original" {
				t.Error("cancel did not target the submitted turn")
			}
			fmt.Fprint(w, `{"success":true,"data":{"runtime":{"state":"cancelling","is_processing":true}}}`)
		case "/api/conversations/old":
			checked = true
			fmt.Fprint(w, `{"success":true,"data":{"runtime":{"state":"idle","is_processing":false}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	httpClient := server.Client()
	httpClient.Jar, _ = cookiejar.New(nil)
	client := Client{base: base, http: httpClient}
	ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	if err := client.Cancel(ctx, "old", "original"); err != nil || !checked {
		t.Fatal("no cancellation acknowledgement", err)
	}
	if err := client.CheckSession(ctx, "missing"); err != ErrSessionMissing {
		t.Fatal(err)
	}
	if ProbeFailure(HTTPError(http.StatusForbidden)) != "auth" || ProbeFailure(context.DeadlineExceeded) != "timeout" {
		t.Fatal("incorrect probe status")
	}
}

func TestDocumentSubmissionStaysInPlanMode(t *testing.T) {
	var saved string
	stopped := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/conversations/doc":
			if stopped {
				fmt.Fprint(w, `{"success":true,"data":{"runtime":{"state":"idle","is_processing":false}}}`)
				return
			}
			fmt.Fprint(w, `{"success":true,"data":{"status":"pending","runtime":{"state":"waiting_confirmation","turn_id":"original","is_processing":true}}}`)
		case "/api/conversations/doc/messages":
			fmt.Fprint(w, `{"success":true,"data":{"items":[{"position":"left","type":"tool_call","status":"work","content":{"name":"ExitPlanMode","input":{"plan":"earlier turn"}}},{"position":"right","id":"q","type":"text","content":{"content":"owner request"}},{"position":"left","type":"tool_call","status":"work","content":{"name":"ExitPlanMode","input":{"plan":"submitted design"}}}]}}`)
		case "/api/conversations/doc/cancel":
			var body map[string]string
			if saved != "submitted design" || json.NewDecoder(r.Body).Decode(&body) != nil || body["turn_id"] != "original" {
				t.Error("unrecorded or wrong turn stopped")
			}
			stopped = true
			fmt.Fprint(w, `{"success":true,"data":{"runtime":{"state":"idle","is_processing":false}}}`)
		default:
			t.Errorf("unexpected operation %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	httpClient := server.Client()
	httpClient.Jar, _ = cookiejar.New(nil)
	client := Client{base: base, http: httpClient}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	doc, err := client.DocumentReply(ctx, "doc", "q", "marker", "original", func(value string) error { saved = value; return nil })
	if err != nil || doc != saved || !stopped {
		t.Fatal(doc, err)
	}
	if err = client.StopDocumentTurn(ctx, "doc", "original"); err != nil {
		t.Fatal("could not recover stopped turn", err)
	}
}

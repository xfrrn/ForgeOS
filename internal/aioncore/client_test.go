package aioncore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
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

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestKnowledgeModelSettingsOwnerAPI(t *testing.T) {
	ctx := context.Background()
	data := store.New(testdb.Open(t))
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := data.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerKnowledgeModelSettings(mux, data, "owner")
	path := fmt.Sprintf("/projects/%d/knowledge/model-settings", project.ID)
	do := func(method, token, body string, want int) string {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", method, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	do("GET", "worker", "", 401)
	initial := do("GET", "owner", "", 200)
	if strings.Count(initial, `"model_id":null`) != 2 || strings.Count(initial, `"revision":0`) != 2 {
		t.Fatal(initial)
	}
	do("PUT", "owner", `{"role":"curator","expected_revision":0}`, 400)
	do("PUT", "owner", `{"role":"curator","expected_revision":0,"model_id":null,"api_key":"forbidden"}`, 400)
	do("PUT", "owner", `{"role":"writer","expected_revision":0,"model_id":null}`, 400)
	const stop = `{"role":"curator","expected_revision":0,"model_id":null}`
	disabled := do("PUT", "owner", stop, 200)
	if !strings.Contains(disabled, `"revision":1`) {
		t.Fatal(disabled)
	}
	do("PUT", "owner", stop, 409)
}

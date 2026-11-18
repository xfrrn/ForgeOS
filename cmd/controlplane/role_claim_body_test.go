package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestReadRoleClaimBodyBeforeReply(t *testing.T) {
	data := store.New(testdb.Open(t))
	if err := data.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FORGEOS_CURATOR_PROJECT_ID", "1")
	t.Setenv("FORGEOS_CURATOR_TOKEN", "curator-token")
	t.Setenv("FORGEOS_TEST_AGENT_PROJECT_ID", "1")
	t.Setenv("FORGEOS_TEST_AGENT_TOKEN", "test-token")
	mux := http.NewServeMux()
	if err := registerCurator(mux, data, "owner", "manager", legacyWorker("worker")); err != nil {
		t.Fatal(err)
	}
	if err := registerTestPlans(mux, data, t.TempDir(), "owner", "manager", legacyWorker("worker")); err != nil {
		t.Fatal(err)
	}
	for role, token := range map[string]string{"curator": "curator-token", "test-agent": "test-token"} {
		for _, body := range []string{"", "{}", "{}   ", "{} {}", "{", `{"unsupported":true}`, strings.Repeat(" ", 17000)} {
			reader := strings.NewReader(body)
			r := httptest.NewRequest("POST", "/internal/"+role+"/claim", reader)
			r.Close = true
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			valid := body == "" || body == "{}" || body == "{}   "
			if valid && (w.Code != 204 || reader.Len() != 0) {
				t.Fatalf("%s returned before consuming claim body: %d, %d", role, w.Code, reader.Len())
			}
			if !valid && w.Code != 400 {
				t.Fatalf("%s accepted malformed claim: %d", role, w.Code)
			}
		}
	}
}

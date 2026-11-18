package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestRoleClaimHTTPIdentityAndCapabilities(t *testing.T) {
	ctx := context.Background()
	data := store.New(testdb.Open(t))
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := data.CreateProject(ctx, "claim-http")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = data.SaveMaterial(ctx, p.ID, "observation", store.MaterialInput{Kind: "material", Title: "Observation", Source: "lab", Content: "Offset 274."}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FORGEOS_CURATOR_PROJECT_ID", strconv.FormatInt(p.ID, 10))
	t.Setenv("FORGEOS_CURATOR_TOKEN", "curator-token")
	mux := http.NewServeMux()
	if err = registerCurator(mux, data, "owner", "manager", legacyWorker("worker")); err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("a", 32)
	request := func(node, token, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-ForgeOS-Claim-Key", key)
		r.Header.Set("X-ForgeOS-Node", node)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	path := "/internal/curator/claim?supports_role_resume=1&supports_process=1&supports_budget_deadline=1"
	first := request("node", "curator-token", path, "{}")
	if first.Code != 200 || first.Header().Get("X-ForgeOS-Claim-Key") != key || (!bytes.Contains(first.Body.Bytes(), []byte(`"capture_process":true`)) || !bytes.Contains(first.Body.Bytes(), []byte(`"recover_native":true`))) {
		t.Fatal("missing frozen capability", first.Code, first.Body.String())
	}
	replay := request("node", "curator-token", path, "{}")
	if replay.Code != 200 || !bytes.Equal(first.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("changed HTTP replay")
	}
	if got := request("node", "curator-token", path, "{} "); got.Code != 409 {
		t.Fatal("changed body accepted", got.Code)
	}
	if got := request("node", "curator-token", "/internal/curator/claim", "{}"); got.Code != 409 {
		t.Fatal("changed capabilities accepted", got.Code)
	}
	if got := request("another-node", "curator-token", path, "{}"); got.Code != 204 {
		t.Fatal("cross-node replay", got.Code)
	}
	if got := request("node", "another-role", path, "{}"); got.Code != 401 {
		t.Fatal("unauthorized replay", got.Code)
	}
}

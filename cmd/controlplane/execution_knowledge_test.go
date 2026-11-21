package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestExecutionKnowledgeRoute(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	data := store.New(db)
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root, repo, base := issueTestRepo(t)
	project, err := data.CreateProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	spec := store.IssueSpec{Issue: store.Issue{ID: 9, Number: 1, RepositoryID: 4, Title: "处理重复导入"}, Validation: "docs", BaseCommit: base}
	if _, err := data.ImportIssue(ctx, project.ID, spec); err != nil {
		t.Fatal(err)
	}
	material := store.MaterialInput{Kind: "material", Title: "重复导入", Source: "资料维护手册", Content: "重复导入时，保留相同来源的原始记录。"}
	if _, err := data.SaveMaterial(ctx, project.ID, "import-guide", material); err != nil {
		t.Fatal(err)
	}
	mux := routes(data, root, "owner", legacyWorker("node"), nil)
	post := func(path string, input any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer node")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	rec := post("/internal/claim?supports_knowledge_tools=1", map[string]any{"worker": "node", "workspace_root": t.TempDir(), "supports_codex": true, "supports_checkpoints": true})
	var job store.Job
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &job) != nil || !job.KnowledgeTools {
		t.Fatalf("knowledge-enabled claim: HTTP %d, enabled %t", rec.Code, job.KnowledgeTools)
	}
	path := fmt.Sprintf("/internal/executions/%d/knowledge", job.ExecutionID)
	request := executionKnowledgeRequest{Action: "search", Query: "重复导入", LeaseToken: job.LeaseToken}
	rec = post(path, request)
	var search struct{ Hits []store.KnowledgeHit }
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &search) != nil || len(search.Hits) != 1 || search.Hits[0].Kind != "material" {
		t.Fatalf("search: %d %s", rec.Code, rec.Body.String())
	}
	hit := search.Hits[0]
	request = executionKnowledgeRequest{Action: "cite", Kind: hit.Kind, Ref: hit.Ref, LeaseToken: job.LeaseToken}
	if rec = post(path, request); rec.Code < 400 {
		t.Fatal("search result cited without reading")
	}
	request.Action = "read"
	read := post(path, request)
	var out struct{ Evidence store.KnowledgeEvidence }
	if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &out) != nil || out.Evidence.Content != material.Content {
		t.Fatalf("read: %d %s", read.Code, read.Body.String())
	}
	material.PreviousRevision, material.Content = 1, "更新后的手册"
	if _, err := data.SaveMaterial(ctx, project.ID, "import-guide", material); err != nil {
		t.Fatal(err)
	}
	request.Action = "cite"
	for i := 0; i < 2; i++ {
		rec = post(path, request)
		if rec.Code != 200 || !bytes.Equal(read.Body.Bytes(), rec.Body.Bytes()) {
			t.Fatalf("citation did not return the observed revision: %d %s", rec.Code, rec.Body.String())
		}
	}
	request.LeaseToken = strings.Repeat("1", 64)
	if rec = post(path, request); rec.Code != http.StatusConflict {
		t.Fatalf("invalid lease: HTTP %d", rec.Code)
	}
	if rec = post(path, map[string]any{"lease_token": job.LeaseToken, "action": "read", "project_id": project.ID, "kind": hit.Kind, "ref": hit.Ref}); rec.Code != http.StatusBadRequest {
		t.Fatalf("accepted caller-selected project: HTTP %d", rec.Code)
	}
	t.Run("received publication", func(t *testing.T) {
		source, err := data.CreateProject(ctx, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		material := store.MaterialInput{Kind: "material", Title: "私有记录", Source: "仅限源项目", Content: "私有客户名及内部故障细节"}
		if _, err := data.SaveMaterial(ctx, source.ID, "private", material); err != nil {
			t.Fatal(err)
		}
		draft := store.ShareDraft{MaterialKey: "private", Revision: 1, Recipient: project.ID, Title: "异步交付", Content: "异步交付可以按任务编号去重。"}
		raw, err := data.DraftShare(ctx, source.ID, draft)
		var copy struct {
			ID     int64
			Digest string
		}
		if err != nil || json.Unmarshal(raw, &copy) != nil {
			t.Fatal(err, string(raw))
		}
		review, err := data.ClaimShareReview(ctx, source.ID, false)
		if err != nil || review == nil {
			t.Fatal("missing review", err)
		}
		if err := data.ReviewShare(ctx, source.ID, copy.ID, review.Lease, store.ManagementReview{
			Level: "D3", Verdict: "approve", SessionID: "share-assessor", Rationale: "已脱敏", Evidence: "私有客户名及内部故障细节",
		}); err != nil {
			t.Fatal(err)
		}
		ref := fmt.Sprintf("%d:%s", copy.ID, copy.Digest)
		action := executionKnowledgeRequest{Action: "read", LeaseToken: job.LeaseToken, Kind: store.SharedKnowledgeKind, Ref: ref}
		if got := post(path, action); got.Code != 404 {
			t.Fatalf("unpublished copy was readable: %d", got.Code)
		}
		if _, err := data.SetSharePublished(ctx, source.ID, copy.ID, copy.Digest, true); err != nil {
			t.Fatal(err)
		}
		got := post(path, executionKnowledgeRequest{Action: "search", LeaseToken: job.LeaseToken, Query: draft.Title})
		var result struct{ Hits []store.KnowledgeHit }
		if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &result) != nil || len(result.Hits) != 1 ||
			result.Hits[0].Kind != store.SharedKnowledgeKind || result.Hits[0].Ref != ref {
			t.Fatalf("missing received search hit: %s", got.Body.String())
		}
		got = post(path, action)
		if got.Code != 200 || !strings.Contains(got.Body.String(), draft.Content) || strings.Contains(got.Body.String(), material.Content) {
			t.Fatalf("wrong evidence: %s", got.Body.String())
		}
		action.Action = "cite"
		if got = post(path, action); got.Code != 200 {
			t.Fatal(got.Body.String())
		}
		if _, err := data.SetSharePublished(ctx, source.ID, copy.ID, copy.Digest, false); err != nil {
			t.Fatal(err)
		}
		for _, verb := range []string{"read", "cite"} {
			action.Action = verb
			if got = post(path, action); got.Code != 404 {
				t.Fatalf("%s after revocation: %d", verb, got.Code)
			}
		}
		var archived string
		if err := db.QueryRowContext(ctx, "SELECT cited_knowledge($1)", job.ExecutionID).Scan(&archived); err != nil ||
			!strings.Contains(archived, "revoked") || !strings.Contains(archived, draft.Content) || strings.Contains(archived, material.Content) {
			t.Fatalf("historical citation: %s, %v", archived, err)
		}
	})
}

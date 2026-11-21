package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestExecutionKnowledgeSources(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	data := store.New(db)
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root, repo, old := issueTestRepo(t)
	p, err := data.CreateProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	other, err := data.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const term = "导入"
	const path = "设计 + 说明.md"
	const key = "导入资料"
	body := term + "\n" + strings.Repeat("验收🔎", 4000)
	if err := os.WriteFile(filepath.Join(repo, path), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, repo, "-c", "core.autocrlf=false", "add", path)
	deliveryGit(t, repo, "-c", "user.name=Test", "-c", "user.email=t@example.invalid", "commit", "-qm", "add docs")
	base := deliveryGit(t, repo, "rev-parse", "HEAD")
	// An owner's branch switch doesn't change an existing execution's baseline.
	deliveryGit(t, repo, "checkout", "--detach", old)
	m := store.MaterialInput{Kind: "material", Title: "说明", Source: "Owner", Content: body}
	if _, err = data.SaveMaterial(ctx, p.ID, key, m); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE material_curations SET extraction='{"facts":[{"statement":"导入经验","quote":"导入"}]}',review='{"level":"D1","verdict":"approve"}' WHERE project_id=$1 AND key=$2`, p.ID, key); err != nil {
		t.Fatal(err)
	}
	raw, err := data.ProposeDecision(ctx, p.ID, store.DecisionProposal{RequestKey: "knowledge", Statement: "导入时保留来源", Validity: "当前项目", Source: "Owner", EvidenceRefs: []string{"plan"}})
	if err != nil {
		t.Fatal(err)
	}
	var decision knowledgeDecision
	if err = json.Unmarshal(raw, &decision); err != nil {
		t.Fatal(err)
	}
	if _, err = data.Decide(ctx, p.ID, decision.ID, "CONFIRMED"); err != nil {
		t.Fatal(err)
	}

	hits, err := knowledgeSearch(ctx, data, root, p.ID, base, "请问这个项目怎么实现批量导入？")
	if err != nil || len(hits) != 4 {
		t.Fatalf("cross-source discovery: hits=%+v err=%v", hits, err)
	}
	byKind := map[string]store.KnowledgeHit{}
	for _, hit := range hits {
		byKind[hit.Kind] = hit
		out, err := knowledgeRead(ctx, data, root, p.ID, base, hit.Kind, hit.Ref)
		if err != nil || out.Ref != hit.Ref || out.Version == "" || out.Source == "" || !strings.Contains(out.Content, term) {
			t.Fatalf("read %s: %+v %v", hit.Kind, out, err)
		}
		if hit.Kind == "decision" && out.Validity != "当前项目" {
			t.Fatal("lost decision validity")
		}
		if hit.Kind == "material" || hit.Kind == "git" {
			got := out.Content
			for out.NextRef != "" {
				out, err = knowledgeRead(ctx, data, root, p.ID, base, hit.Kind, out.NextRef)
				if err != nil || !utf8.ValidString(out.Content) || len(out.Content) == 0 {
					t.Fatalf("continued read %s: %v", hit.Kind, err)
				}
				if out.FirstRef != hit.Ref {
					t.Fatalf("missing first passage for %s", hit.Kind)
				}
				got += out.Content
			}
			if got != body {
				t.Fatalf("lost text across %s continuation", hit.Kind)
			}
		}
		if _, err = knowledgeRead(ctx, data, root, other.ID, base, hit.Kind, hit.Ref); err == nil {
			t.Fatalf("cross-project read %s", hit.Kind)
		}
	}
	if _, err = knowledgeRead(ctx, data, root, p.ID, old, "git", byKind["git"].Ref); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("accepted different baseline", err)
	}
	if _, err = knowledgeRead(ctx, data, root, p.ID, base, "git", fmt.Sprintf("%s:1:%s", base, path)); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("accepted mid-rune offset", err)
	}
	if _, err = data.Decide(ctx, p.ID, decision.ID, "EXPIRED"); err != nil {
		t.Fatal(err)
	}
	if _, err = knowledgeRead(ctx, data, root, p.ID, base, "decision", byKind["decision"].Ref); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("expired decision", err)
	}
	m.PreviousRevision, m.Content = 1, "新版"
	if _, err = data.SaveMaterial(ctx, p.ID, key, m); err != nil {
		t.Fatal(err)
	}
	if _, err = knowledgeRead(ctx, data, root, p.ID, base, "experience", byKind["experience"].Ref); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("superseded experience", err)
	}
	out, err := knowledgeRead(ctx, data, root, p.ID, base, "material", byKind["material"].Ref)
	if err != nil || !strings.HasPrefix(body, out.Content) || out.Version != "1" {
		t.Fatal("lost pinned material", err)
	}
	partial, err := knowledgeSearch(ctx, data, root, p.ID, "", "新版")
	if err == nil || len(partial) != 1 || partial[0].Kind != "material" {
		t.Fatal("unavailable Git blocked materials", partial, err)
	}
}

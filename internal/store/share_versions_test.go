package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestShareSeriesPublicationAndRevisionRace(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	a, err := s.CreateProject(ctx, "source")
	must(err)
	b, err := s.CreateProject(ctx, "recipient")
	must(err)
	c, err := s.CreateProject(ctx, "elsewhere")
	must(err)
	m := MaterialInput{Kind: "material", Title: "Private handoff", Source: "private-receipt", Content: "PRIVATE counter value was not reset."}
	_, err = s.SaveMaterial(ctx, a.ID, "handoff", m)
	must(err)
	_, err = s.db.ExecContext(ctx, `UPDATE material_curations SET extraction='{"facts":[{"statement":"Private handoff note","quote":"counter value"}]}',
		review='{"level":"D1","verdict":"approve"}',reviewed_at=now() WHERE project_id=$1 AND key='handoff'`, a.ID)
	must(err)
	var entry int64
	must(s.db.QueryRowContext(ctx, `SELECT entry_id FROM experience_revision_facts WHERE project_id=$1 AND is_effective`, a.ID).Scan(&entry))
	type copyReceipt struct {
		ID            int64
		Series        int64 `json:"series_id"`
		Version       int   `json:"share_version"`
		Epoch         int64 `json:"publication_epoch"`
		Title, Digest string
	}
	parse := func(raw json.RawMessage, err error) copyReceipt {
		t.Helper()
		must(err)
		var r copyReceipt
		must(json.Unmarshal(raw, &r))
		return r
	}
	input := ShareDraft{ExperienceID: entry, ExperienceRevision: 1, Recipient: b.ID, Title: "One", Content: "Reset the counter when a second run starts."}
	one := parse(s.DraftShare(ctx, a.ID, input))
	if again := parse(s.DraftShare(ctx, a.ID, input)); again.ID != one.ID {
		t.Fatal("initial retry created a second series")
	}
	job, err := s.ClaimShareReview(ctx, a.ID, false)
	must(err)
	if job == nil || job.Share.ExperienceID != entry || !strings.Contains(job.Share.OriginalContent, "counter value") {
		t.Fatalf("wrong fixed source: %#v", job)
	}
	must(s.ReviewShare(ctx, a.ID, one.ID, job.Lease, ManagementReview{Level: "D3", Verdict: "approve", SessionID: "share-review", Rationale: "Public procedure", Evidence: "counter value"}))
	one = parse(s.SetSharePublished(ctx, a.ID, one.ID, one.Digest, true))
	if one.Version != 1 || one.Epoch != 1 {
		t.Fatal(one)
	}
	draft := ShareVersionDraft{ExpectedCurrentID: &one.ID, ExpectedEpoch: &one.Epoch, Title: "Second", Content: "Reset the counter before rerunning the job."}
	two := parse(s.DraftShareVersion(ctx, a.ID, one.ID, draft))
	if again := parse(s.DraftShareVersion(ctx, a.ID, one.ID, draft)); again.ID != two.ID {
		t.Fatal("non-idempotent version draft")
	}
	draft3 := draft
	draft3.Title = "Third"
	three := parse(s.DraftShareVersion(ctx, a.ID, one.ID, draft3))
	if two.Series != one.Series || three.Version != 3 {
		t.Fatal(two, three)
	}
	if _, err = s.DraftShareVersion(ctx, b.ID, one.ID, draft); !errors.Is(err, ErrNotFound) {
		t.Fatal("version drafted outside source", err)
	}
	if _, err = s.SetSharePublished(ctx, a.ID, two.ID, two.Digest, true); !errors.Is(err, ErrConflict) {
		t.Fatal("unreviewed publication", err)
	}
	if got := parse(s.ReceivedShare(ctx, b.ID, one.Series)); got.ID != one.ID {
		t.Fatal("draft prematurely replaced old release", got)
	}
	if _, err = s.ReceivedShare(ctx, c.ID, one.Series); !errors.Is(err, ErrNotFound) {
		t.Fatal("read outside recipient", err)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE knowledge_shares SET review='{"verdict":"approve","level":"D3"}',reviewed_at=now() WHERE id IN($1,$2)`, two.ID, three.ID)
	must(err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO share_embeddings VALUES($1,'test',ARRAY[1.0]::real[])`, one.ID)
	must(err)
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, candidate := range []copyReceipt{two, three} {
		go func() {
			<-start
			_, e := s.SetSharePublished(ctx, a.ID, candidate.ID, candidate.Digest, true)
			done <- e
		}()
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		switch e := <-done; {
		case e == nil:
			successes++
		case errors.Is(e, ErrConflict):
			conflicts++
		default:
			t.Fatal(e)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("both competing versions altered the publication", successes, conflicts)
	}
	got := parse(s.ReceivedShare(ctx, b.ID, one.Series))
	if got.ID != two.ID && got.ID != three.ID {
		t.Fatal(got)
	}
	if again := parse(s.DraftShareVersion(ctx, a.ID, one.ID, draft)); again.ID != two.ID {
		t.Fatal("retry after publication failed")
	}
	var vectors int
	must(s.db.QueryRowContext(ctx, `SELECT count(*) FROM share_embeddings WHERE share_id=$1`, one.ID).Scan(&vectors))
	if vectors != 0 {
		t.Fatal("superseded copy kept vectors")
	}
	for _, query := range []string{"PRIVATE", one.Title, "private-receipt"} {
		page, err := s.SearchReceivedShares(ctx, b.ID, query, true)
		must(err)
		if string(page) != "[]" {
			t.Fatalf("old/private content found: %s", page)
		}
	}
	raw, err := s.ReceivedSharePage(ctx, b.ID, 0, true)
	must(err)
	var rows []map[string]any
	must(json.Unmarshal(raw, &rows))
	if len(rows) != 1 || len(rows[0]) != 7 || rows[0]["series_id"] != float64(one.Series) {
		t.Fatal(rows)
	}
	raw, err = s.ShareVersions(ctx, a.ID, one.ID, 0)
	must(err)
	must(json.Unmarshal(raw, &rows))
	if len(rows) != 3 {
		t.Fatal(rows)
	}
	_, err = s.SetSharePublished(ctx, a.ID, got.ID, got.Digest, false)
	must(err)
	if _, err = s.ReceivedShare(ctx, b.ID, one.Series); !errors.Is(err, ErrNotFound) {
		t.Fatal("withdrawn detail visible", err)
	}
	if _, err = s.SetSharePublished(ctx, a.ID, one.ID, one.Digest, true); !errors.Is(err, ErrConflict) {
		t.Fatal("superseded publication resurrected", err)
	}
}

func TestShareSeriesUpgradePreservesIndependentReleases(t *testing.T) {
	ctx := context.Background()
	db := isolatedDatabase(t)
	old := []string{initialSchema, issueSchema, deliverySchema, inboxSchema, planSchema, planArchiveSchema, goalItemsSchema,
		questionSchema, goalClosureSchema, managementSchema, rollingSchema, requirementSchema, commentSchema, checkpointSchema,
		refreshSchema, decisionSchema, materialSchema, shareSchema}
	_, err := db.ExecContext(ctx, strings.Join(old, "\n")+`
		CREATE TABLE forgeos_schema_versions(version integer PRIMARY KEY); INSERT INTO forgeos_schema_versions SELECT generate_series(1,18);
		INSERT INTO projects(path) VALUES('source'),('recipient');
		INSERT INTO project_materials(project_id,key,revision,kind,title,source,content,content_sha256)
		VALUES(1,'handbook',1,'material','手册','原件','私有正文','digest');
		INSERT INTO knowledge_shares(project_id,material_key,revision,recipient,title,content,digest,published_at,revoked_at,review)
		SELECT 1,'handbook',1,2,'版本 '||n,'public '||n,repeat(n::text,64),now(),CASE WHEN n=3 THEN now() END,
		       CASE WHEN n<>4 THEN '{"verdict":"approve","effective_level":"D3"}'::jsonb END
		FROM generate_series(1,4)n;`)
	if err != nil {
		t.Fatal(err)
	}
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2} {
		raw, err := s.ReceivedShare(ctx, 2, id)
		var copy struct {
			ID      int64
			Series  int64 `json:"series_id"`
			Version int   `json:"share_version"`
		}
		if err != nil || json.Unmarshal(raw, &copy) != nil || copy.ID != id || copy.Series != id || copy.Version != 1 {
			t.Fatalf("release %d became inaccessible: %s %v", id, raw, err)
		}
	}
	if _, err := s.ReceivedShare(ctx, 2, 3); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked legacy version became visible", err)
	}
	if _, err := s.ReceivedShare(ctx, 2, 4); !errors.Is(err, ErrNotFound) {
		t.Fatal("legacy version missing review became visible", err)
	}
	raw, err := s.ShareReceipt(ctx, 1, 3)
	if err != nil || !strings.Contains(string(raw), "revoked_at") {
		t.Fatal("lost source receipt", string(raw), err)
	}
}

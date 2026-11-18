package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// HistoryHit identifies an immutable version of a project source, including older revisions.
type HistoryHit struct {
	Delivery   json.RawMessage `json:"delivery,omitempty"`
	FeedbackID int64           `json:"feedback_id,omitempty"`
	ProjectID  int64           `json:"project_id"`
	TaskID     int64           `json:"task_id"`
	Source     string          `json:"source"`
	SourceID   int64           `json:"source_id"`
	Version    int64           `json:"version"`
	Status     string          `json:"status"`
	At         time.Time       `json:"observed_at"`
	URL        string          `json:"url"`
	Excerpt    string          `json:"excerpt"`
}

const historyOrder = "observed_at DESC,source,version DESC,task_id DESC"

const historyRecords = `WITH scope AS (
			SELECT id,spec->'issue'->>'title' AS title,spec->'issue'->>'url' AS url,active_revision
			FROM work_items WHERE project_id=$1 AND ($2::bigint=0 OR id=$2)
		), records AS (
			SELECT t.id AS task_id,'requirement' AS source,(r.issue->>'id')::bigint AS source_id,
			       r.revision::bigint AS version,r.observed_at,r.issue->>'url' AS url,r.feedback_id,
			       concat_ws(' ',r.issue->>'title',r.issue->>'body') AS content,
			       row_number() OVER (PARTITION BY t.id ORDER BY r.revision DESC) AS recency,NULL::jsonb AS delivery
			FROM scope t JOIN requirement_revisions r ON r.task_id=t.id WHERE $3 IN ('','requirement')
			UNION ALL
			SELECT t.id,'comment',c.comment_id,c.id,c.observed_at,c.payload->>'html_url',NULL::bigint,c.payload->>'body',
			       row_number() OVER (PARTITION BY t.id,c.comment_id ORDER BY c.updated_at DESC,c.id DESC),NULL::jsonb
			FROM scope t JOIN issue_comment_events c ON c.task_id=t.id WHERE $3 IN ('','comment')
			UNION ALL
			SELECT t.id,'merge',d.pr_number,e.id,e.at,d.pr_url,NULL::bigint,
			       concat_ws(' ',t.title,d.pr_url,d.target_branch,d."commit",d.merge_sha),1,
			       jsonb_build_object('execution_id',d.execution_id,'commit',d."commit",'merge_sha',d.merge_sha,'target_branch',d.target_branch)
			FROM scope t JOIN deliveries d ON d.task_id=t.id
			JOIN task_events e ON e.task_id=t.id AND e.execution_id=d.execution_id AND e.type='pr_merged'
			WHERE $3 IN ('','merge') AND d.state IN ('MERGED','DONE')
			UNION ALL
			SELECT t.id,'goal',t.id,r.revision,e.at,t.url,NULL::bigint,
			       concat_ws(' ',t.title,r.plan->>'objective',t.url),1,NULL::jsonb
			FROM scope t JOIN plan_revisions r ON r.task_id=t.id AND r.revision=t.active_revision
			JOIN task_events e ON e.task_id=t.id AND e.type='goal_delivered' WHERE $3 IN ('','goal')
		)`

func (s *Store) SearchHistory(ctx context.Context, project, task int64, source, query string) ([]HistoryHit, error) {
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	args := []any{project, task, source, query}
	sourceRows := "matches"
	filter := "$4='' OR words @@ terms OR strpos(lower(content),lower($4))>0"
	order := "ts_rank(words,terms) DESC," + historyOrder
	extra := ""
	sourceURL := strings.HasPrefix(query, "https://")
	if sourceURL {
		filter = "url=$4 OR " + filter
		order = "(url=$4) DESC NULLS LAST," + order
	}
	// A full commit or source URL is an exact reference, including when it belongs to another project.
	if query != "" && s.embeddings != nil && !validCommit(strings.ToLower(query)) && !sourceURL {
		var eligible bool
		if err := s.db.QueryRowContext(ctx, historyRecords+` SELECT EXISTS(SELECT 1 FROM records WHERE content<>'')`, project, task, source).Scan(&eligible); err != nil {
			return nil, err
		}
		if !eligible {
			return []HistoryHit{}, nil
		}
		v, err := s.embeddings.textVector(ctx, query)
		if err != nil {
			return nil, err
		}
		if err := s.indexHistory(ctx, project, task, source, len(v)); err != nil {
			return nil, err
		}
		vector, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		args = append(args, s.embeddings.model, string(vector))
		extra = `, eligible AS MATERIALIZED (
			SELECT m.*,e.embedding FROM matches m LEFT JOIN history_embeddings e
			ON e.task_id=m.task_id AND e.content_hash=sha256(convert_to(m.content,'UTF8')) AND e.model=$5
		), scores AS (
			SELECT *, words @@ terms OR strpos(lower(content),lower($4))>0 AS keyword,
			       embedding::vector <=> $6::vector AS distance FROM eligible
		), ranked AS (
			SELECT *,CASE WHEN keyword THEN row_number() OVER (ORDER BY keyword DESC NULLS LAST,ts_rank(words,terms) DESC,` + historyOrder + `) END AS lexical,
			         CASE WHEN distance<1 THEN row_number() OVER (ORDER BY distance NULLS LAST,` + historyOrder + `) END AS semantic FROM scores
		)`
		sourceRows = "ranked"
		filter = "lexical IS NOT NULL OR semantic IS NOT NULL"
		order = "coalesce(1.0/(60+lexical),0)+coalesce(1.0/(60+semantic),0) DESC," + historyOrder
	}
	rows, err := s.db.QueryContext(ctx, historyRecords+`, matches AS (
			SELECT *,to_tsvector('simple',content) AS words,plainto_tsquery('simple',$4) AS terms FROM records
		)`+extra+`
		SELECT task_id,source,source_id,version,
		       CASE WHEN recency=1 THEN 'latest' ELSE 'historical' END,observed_at,url,
		       left(content,500),COALESCE(feedback_id,0),delivery
		FROM `+sourceRows+` WHERE `+filter+` ORDER BY `+order+` LIMIT 30`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hits := []HistoryHit{}
	for rows.Next() {
		hit := HistoryHit{ProjectID: project}
		var delivery []byte
		if err := rows.Scan(&hit.TaskID, &hit.Source, &hit.SourceID, &hit.Version, &hit.Status, &hit.At, &hit.URL, &hit.Excerpt, &hit.FeedbackID, &delivery); err != nil {
			return nil, err
		}
		hit.Delivery = delivery
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

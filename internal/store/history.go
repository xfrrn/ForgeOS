package store

import (
	"context"
	"time"
)

// HistoryHit identifies an immutable version of a project source, including older revisions.
type HistoryHit struct {
	FeedbackID int64     `json:"feedback_id,omitempty"`
	ProjectID  int64     `json:"project_id"`
	TaskID     int64     `json:"task_id"`
	Source     string    `json:"source"`
	SourceID   int64     `json:"source_id"`
	Version    int64     `json:"version"`
	Status     string    `json:"status"`
	At         time.Time `json:"observed_at"`
	URL        string    `json:"url"`
	Excerpt    string    `json:"excerpt"`
}

func (s *Store) SearchHistory(ctx context.Context, project, task int64, source, query string) ([]HistoryHit, error) {
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH scope AS (
			SELECT id FROM work_items WHERE project_id=$1 AND ($2::bigint=0 OR id=$2)
		), records AS (
			SELECT t.id AS task_id,'requirement' AS source,(r.issue->>'id')::bigint AS source_id,
			       r.revision::bigint AS version,r.observed_at,r.issue->>'url' AS url,r.feedback_id,
			       concat_ws(' ',r.issue->>'title',r.issue->>'body') AS content,
			       row_number() OVER (PARTITION BY t.id ORDER BY r.revision DESC) AS recency
			FROM scope t JOIN requirement_revisions r ON r.task_id=t.id WHERE $3 IN ('','requirement')
			UNION ALL
			SELECT t.id,'comment',c.comment_id,c.id,c.observed_at,c.payload->>'html_url',NULL::bigint,c.payload->>'body',
			       row_number() OVER (PARTITION BY t.id,c.comment_id ORDER BY c.updated_at DESC,c.id DESC)
			FROM scope t JOIN issue_comment_events c ON c.task_id=t.id WHERE $3 IN ('','comment')
		), matches AS (
			SELECT *,to_tsvector('simple',content) AS words,
			       plainto_tsquery('simple',$4) AS terms FROM records
		)
		SELECT task_id,source,source_id,version,
		       CASE WHEN recency=1 THEN 'latest' ELSE 'historical' END,observed_at,url,
		       left(content,500),COALESCE(feedback_id,0)
		FROM matches WHERE $4='' OR words @@ terms OR strpos(lower(content),lower($4))>0
		ORDER BY ts_rank(words,terms) DESC,observed_at DESC,source,version DESC,task_id DESC LIMIT 30`,
		project, task, source, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hits := []HistoryHit{}
	for rows.Next() {
		hit := HistoryHit{ProjectID: project}
		if err := rows.Scan(&hit.TaskID, &hit.Source, &hit.SourceID, &hit.Version, &hit.Status, &hit.At, &hit.URL, &hit.Excerpt, &hit.FeedbackID); err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

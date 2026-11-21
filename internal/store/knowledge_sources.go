package store

import (
	"context"
	"database/sql"
	"errors"
)

// KnowledgeHit identifies a single version and, for long documents, a passage.
// Clients pass Ref back unchanged. It is scoped to the execution's project.
type KnowledgeHit struct {
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"`
}

type KnowledgeEvidence struct {
	KnowledgeHit
	Content    string `json:"content"`
	Source     string `json:"source"`
	Version    string `json:"version"`
	Validity   string `json:"validity,omitempty"`
	ValidUntil string `json:"valid_until,omitempty"`
	FirstRef   string `json:"first_ref,omitempty"`
	NextRef    string `json:"next_ref,omitempty"`
	OffsetUnit string `json:"offset_unit,omitempty"`
	Start      int64  `json:"start"`
	End        int64  `json:"end"`
}

// ExperienceFact reads one approved fact through the same effective-source
// view as search. Replacing its source or expiring a decision removes it.
func (s *Store) ExperienceFact(ctx context.Context, project, id, number int64) (KnowledgeEvidence, error) {
	var evidence KnowledgeEvidence
	var quoteLength int64
	err := s.db.QueryRowContext(ctx, `SELECT title,statement,quote,receipt_path,content_sha256,start_char,length(quote),validity
		FROM experience_revision_facts WHERE project_id=$1 AND curation_id=$2 AND fact_number=$3 AND is_effective`, project, id, number).
		Scan(&evidence.Title, &evidence.Excerpt, &evidence.Content, &evidence.Source, &evidence.Version, &evidence.Start, &quoteLength, &evidence.Validity)
	if errors.Is(err, sql.ErrNoRows) {
		return KnowledgeEvidence{}, ErrNotFound
	}
	evidence.OffsetUnit = "char"
	evidence.End = evidence.Start + quoteLength
	return evidence, err
}

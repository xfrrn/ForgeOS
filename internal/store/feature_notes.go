package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
)

type FeatureNote struct {
	ID     int64  `json:"id"`
	Turn   int64  `json:"turn_id"`
	Kind   string `json:"kind"`
	Body   string `json:"body"`
	Closed bool   `json:"closed"`
}

func (s *Store) FeatureNotes(ctx context.Context, project, feature int64, activeOnly bool) ([]FeatureNote, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,turn_id,kind,body,closed FROM feature_notes
        WHERE project_id=$1 AND feature_id=$2 AND (NOT $3 OR NOT closed) ORDER BY id`, project, feature, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notes := []FeatureNote{}
	for rows.Next() {
		var n FeatureNote
		if err := rows.Scan(&n.ID, &n.Turn, &n.Kind, &n.Body, &n.Closed); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

func (s *Store) AddFeatureNote(ctx context.Context, project, feature int64, client uuid.UUID, n FeatureNote) (FeatureNote, error) {
	err := s.db.QueryRowContext(ctx, `INSERT INTO feature_notes(project_id,feature_id,turn_id,client_id,kind,body)
        SELECT project_id,feature_id,id,$4,$5,$6 FROM project_chat_turns
        WHERE project_id=$1 AND feature_id=$2 AND id=$3 AND status='completed'
        ON CONFLICT(project_id,feature_id,client_id) DO NOTHING RETURNING id,closed`,
		project, feature, n.Turn, client, n.Kind, n.Body).Scan(&n.ID, &n.Closed)
	if errors.Is(err, sql.ErrNoRows) {
		err = s.db.QueryRowContext(ctx, `SELECT id,closed FROM feature_notes WHERE project_id=$1 AND feature_id=$2
            AND turn_id=$3 AND client_id=$4 AND kind=$5 AND body=$6`, project, feature, n.Turn, client, n.Kind, n.Body).Scan(&n.ID, &n.Closed)
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrConflict
		}
	}
	return n, err
}

func (s *Store) CloseFeatureNote(ctx context.Context, project, feature, id int64) error {
	res, err := s.db.ExecContext(ctx, "UPDATE feature_notes SET closed=true WHERE project_id=$1 AND feature_id=$2 AND id=$3", project, feature, id)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err == nil && count == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) FeatureNoteSource(ctx context.Context, project, feature, note int64) (ChatTurn, error) {
	return scanChatTurn(s.db.QueryRowContext(ctx, `SELECT `+chatTurnFields+` FROM project_chat_turns t
        JOIN feature_notes n ON (n.project_id,n.feature_id,n.turn_id)=(t.project_id,t.feature_id,t.id)
        WHERE n.project_id=$1 AND n.feature_id=$2 AND n.id=$3`, project, feature, note))
}

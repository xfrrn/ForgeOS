package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
)

// ChatModel contains only Owner-visible catalog metadata. Each revision points to
// an immutable OpenBao connection so queued turns survive changes to the catalog.
type ChatModel struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Model        string    `json:"model"`
	BaseURL      string    `json:"base_url"`
	OutputFormat string    `json:"output_format"`
	Revision     uuid.UUID `json:"revision"`
}

func (s *Store) ChatModels(ctx context.Context, migrate func(int64) (ChatModel, error)) ([]ChatModel, error) {
	// A row lock also coordinates imports between concurrent control planes.
	for {
		done, err := s.importChatModel(ctx, migrate)
		if err != nil {
			return nil, err
		}
		if done {
			break
		}
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id,model,base_url,output_format,revision FROM chat_models WHERE revision IS NOT NULL ORDER BY model,base_url,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	models := []ChatModel{}
	for rows.Next() {
		var m ChatModel
		if err := rows.Scan(&m.ID, &m.Model, &m.BaseURL, &m.OutputFormat, &m.Revision); err != nil {
			return nil, err
		}
		m.Name = m.Model
		models = append(models, m)
	}
	return models, rows.Err()
}

func (s *Store) importChatModel(ctx context.Context, migrate func(int64) (ChatModel, error)) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var id uuid.UUID
	var project int64
	err = tx.QueryRowContext(ctx, "SELECT id,legacy_project FROM chat_models WHERE revision IS NULL ORDER BY legacy_project LIMIT 1 FOR UPDATE").Scan(&id, &project)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	m, err := migrate(project)
	if err != nil {
		return false, err
	}
	if m.Revision == uuid.Nil {
		_, err = tx.ExecContext(ctx, "DELETE FROM chat_models WHERE id=$1", id)
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE chat_models SET name=$2,model=$3,base_url=$4,output_format=$5,revision=$6 WHERE id=$1", id, m.Name, m.Model, m.BaseURL, m.OutputFormat, m.Revision)
	}
	if err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func (s *Store) SaveChatModel(ctx context.Context, m ChatModel, previous uuid.UUID, write func() error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if previous == uuid.Nil {
		_, err = tx.ExecContext(ctx, "INSERT INTO chat_models(id,name,model,base_url,output_format,revision) VALUES($1,$2,$3,$4,$5,$6)", m.ID, m.Name, m.Model, m.BaseURL, m.OutputFormat, m.Revision)
	} else {
		var r sql.Result
		r, err = tx.ExecContext(ctx, "UPDATE chat_models SET name=$2,model=$3,base_url=$4,output_format=$5,revision=$6 WHERE id=$1 AND revision=$7", m.ID, m.Name, m.Model, m.BaseURL, m.OutputFormat, m.Revision, previous)
		if err == nil {
			err = chatUpdated(r, nil)
		}
	}
	if err != nil {
		return err
	}
	if err = write(); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RemoveChatModel(ctx context.Context, id, revision uuid.UUID) error {
	return chatUpdated(s.db.ExecContext(ctx, "DELETE FROM chat_models WHERE id=$1 AND revision=$2", id, revision))
}

func (s *Store) LeasedChatModel(ctx context.Context, project, id int64, lease string) (uuid.NullUUID, error) {
	var revision uuid.NullUUID
	err := s.db.QueryRowContext(ctx, "SELECT model_revision FROM project_chat_turns WHERE project_id=$1 AND id=$2 AND lease=$3 AND status='running' AND lease_until>now()", project, id, lease).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrConflict
	}
	return revision, err
}

func (s *Store) CurrentChatModelRevision(ctx context.Context, id, revision uuid.UUID) (bool, error) {
	var active bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM chat_models WHERE id=$1 AND revision=$2)", id, revision).Scan(&active)
	return active, err
}

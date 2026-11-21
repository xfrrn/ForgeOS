package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type KnowledgeModelSetting struct {
	Role          string        `json:"role"`
	Revision      int64         `json:"revision"`
	ModelID       uuid.NullUUID `json:"model_id"`
	ModelRevision uuid.NullUUID `json:"model_revision"`
	LastFailure   string        `json:"last_failure,omitempty"`
}

// KnowledgeRoleModel pins both the project's binding and its immutable vault
// connection revision for the entire claim, model call and result.
type KnowledgeRoleModel struct {
	ProjectID     int64
	Role          string
	Revision      int64
	ModelID       uuid.UUID
	ModelRevision uuid.UUID
}

type knowledgeRoleModelKey struct{}

func (b KnowledgeRoleModel) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, knowledgeRoleModelKey{}, b)
}

func knowledgeAttemptBinding(ctx context.Context) (KnowledgeRoleModel, bool) {
	b, ok := ctx.Value(knowledgeRoleModelKey{}).(KnowledgeRoleModel)
	return b, ok
}

func validKnowledgeRole(role string) bool {
	return role == "curator" || role == "management"
}

func (s *Store) KnowledgeModelSettings(ctx context.Context, project int64) (map[string]KnowledgeModelSetting, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT b.role,b.revision,b.model_id,m.revision,coalesce((
		SELECT a.diagnostic || CASE WHEN a.attempts>=3 THEN '（本任务已达尝试上限）' ELSE '' END
		FROM knowledge_model_attempts a WHERE a.project_id=b.project_id
		AND a.binding_revision=b.revision AND a.model_revision=m.revision AND a.status='failed'
		AND (a.kind='extract')=(b.role='curator')
		ORDER BY a.updated_at DESC, a.job_id DESC LIMIT 1),'')
		FROM knowledge_model_settings b LEFT JOIN chat_models m ON m.id=b.model_id WHERE b.project_id=$1`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]KnowledgeModelSetting, 2)
	for rows.Next() {
		var b KnowledgeModelSetting
		if err := rows.Scan(&b.Role, &b.Revision, &b.ModelID, &b.ModelRevision, &b.LastFailure); err != nil {
			return nil, err
		}
		result[b.Role] = b
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, ErrNotFound
	}
	return result, nil
}

func (s *Store) SetKnowledgeModel(ctx context.Context, project int64, role string, expected int64, model *uuid.UUID) (map[string]KnowledgeModelSetting, error) {
	if !validKnowledgeRole(role) || expected < 0 || (model != nil && *model == uuid.Nil) {
		return nil, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Lock catalog first, then binding, in the same order as model deletion.
	if model != nil {
		var id uuid.UUID
		err = tx.QueryRowContext(ctx, `SELECT id FROM chat_models WHERE id=$1 AND revision IS NOT NULL FOR SHARE`, *model).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConflict
		}
		if err != nil {
			return nil, err
		}
	}
	var current int64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM knowledge_model_settings WHERE project_id=$1 AND role=$2 FOR UPDATE`, project, role).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if current != expected {
		return nil, ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `UPDATE knowledge_model_settings SET model_id=$3 WHERE project_id=$1 AND role=$2`, project, role, model); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.KnowledgeModelSettings(ctx, project)
}

func (s *Store) ConfiguredKnowledgeRoleModels(ctx context.Context) ([]KnowledgeRoleModel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT b.project_id,b.role,b.revision,b.model_id,m.revision
		FROM knowledge_model_settings b JOIN chat_models m ON m.id=b.model_id
		WHERE m.revision IS NOT NULL ORDER BY b.project_id,b.role`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []KnowledgeRoleModel
	for rows.Next() {
		var b KnowledgeRoleModel
		if err := rows.Scan(&b.ProjectID, &b.Role, &b.Revision, &b.ModelID, &b.ModelRevision); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

func (s *Store) KnowledgeRoleModelCurrent(ctx context.Context, b KnowledgeRoleModel) (bool, error) {
	var current bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_model_settings b JOIN chat_models m ON m.id=b.model_id
		WHERE b.project_id=$1 AND b.role=$2 AND b.revision=$3 AND m.id=$4 AND m.revision=$5)`,
		b.ProjectID, b.Role, b.Revision, b.ModelID, b.ModelRevision).Scan(&current)
	return current, err
}

// Hold these locks through the claim or result commit. Edits to the catalog,
// disabling a role and changing a binding must serialize with persisted output.
func knowledgeModelAllowed(ctx context.Context, tx *sql.Tx, project int64, role string) (bool, error) {
	b, direct := knowledgeAttemptBinding(ctx)
	if direct {
		if b.ProjectID != project || b.Role != role {
			return false, nil
		}
		var matched bool
		err := tx.QueryRowContext(ctx, `SELECT true FROM chat_models WHERE id=$1 AND revision=$2 FOR SHARE`, b.ModelID, b.ModelRevision).Scan(&matched)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
	var revision int64
	var model uuid.NullUUID
	err := tx.QueryRowContext(ctx, `SELECT revision,model_id FROM knowledge_model_settings WHERE project_id=$1 AND role=$2 FOR SHARE`, project, role).Scan(&revision, &model)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !direct {
		return revision == 0 && !model.Valid, nil
	}
	return revision == b.Revision && model.Valid && model.UUID == b.ModelID, nil
}

func requireKnowledgeModelResult(ctx context.Context, tx *sql.Tx, project int64, role, kind string, job int64, lease string) error {
	ok, err := knowledgeModelAllowed(ctx, tx, project, role)
	if err != nil {
		return err
	}
	if !ok {
		return ErrConflict
	}
	b, direct := knowledgeAttemptBinding(ctx)
	if !direct {
		return nil
	}
	// A fresh binding cannot relabel an earlier provider's response. Check
	// the receipt written before the remote call alongside the actual job lease.
	var reserved bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_model_attempts
		WHERE project_id=$1 AND kind=$2 AND job_id=$3 AND lease::text=$4 AND binding_revision=$5 AND model_revision=$6
		AND status IN ('running','succeeded'))`, project, kind, job, lease, b.Revision, b.ModelRevision).Scan(&reserved)
	if err == nil && !reserved {
		return ErrConflict
	}
	return err
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

type ChatAttachment struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	MIME      string    `json:"mime"`
	Bytes     int64     `json:"bytes"`
	SHA256    string    `json:"sha256,omitempty"`
	Key       string    `json:"-"`
	TurnID    int64     `json:"turn_id,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

func attachmentExt(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".bin"
	}
}

func cleanAttachmentName(name, mime string) string {
	base := strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	base = strings.ReplaceAll(base, "\x00", "")
	if base == "" || base == "." || base == "/" {
		base = "image" + attachmentExt(mime)
	}
	runes := []rune(base)
	if len(runes) > 200 {
		ext := []rune(attachmentExt(mime))
		base = string(runes[:200-len(ext)]) + string(ext)
	}
	if !utf8.ValidString(base) {
		return "image" + attachmentExt(mime)
	}
	return base
}

func (s *Store) SaveChatAttachment(ctx context.Context, project int64, name, mime, sha string, size int64, write func(key string) error) (ChatAttachment, error) {
	if _, err := s.Project(ctx, project); err != nil {
		return ChatAttachment{}, err
	}
	name = cleanAttachmentName(name, mime)
	key := fmt.Sprintf("chat-attachments/p-%d/%s%s", project, uuid.NewString(), attachmentExt(mime))
	if err := write(key); err != nil {
		return ChatAttachment{}, err
	}
	a := ChatAttachment{Name: name, MIME: mime, Bytes: size, SHA256: sha, Key: key}
	err := s.db.QueryRowContext(ctx, `INSERT INTO project_chat_attachments(project_id,name,mime,sha256,bytes,storage_key)
		VALUES($1,$2,$3,$4,$5,$6) RETURNING id,created_at`, project, name, mime, sha, size, key).Scan(&a.ID, &a.CreatedAt)
	if err != nil {
		return ChatAttachment{}, err
	}
	return a, nil
}

func (s *Store) ChatAttachment(ctx context.Context, project, id int64) (ChatAttachment, error) {
	var a ChatAttachment
	var turn sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT id,COALESCE(turn_id,0),name,mime,sha256,bytes,storage_key,created_at
		FROM project_chat_attachments WHERE project_id=$1 AND id=$2`, project, id).
		Scan(&a.ID, &turn, &a.Name, &a.MIME, &a.SHA256, &a.Bytes, &a.Key, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ChatAttachment{}, ErrNotFound
	}
	if turn.Valid {
		a.TurnID = turn.Int64
	}
	return a, err
}

func (s *Store) DeleteDraftChatAttachment(ctx context.Context, project, id int64) (string, error) {
	var key string
	err := s.db.QueryRowContext(ctx, `DELETE FROM project_chat_attachments
		WHERE project_id=$1 AND id=$2 AND turn_id IS NULL RETURNING storage_key`, project, id).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return key, err
}

func (s *Store) TurnChatAttachments(ctx context.Context, project, turn int64) ([]ChatAttachment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,mime,sha256,bytes,storage_key,created_at
		FROM project_chat_attachments WHERE project_id=$1 AND turn_id=$2 ORDER BY id`, project, turn)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ChatAttachment{}
	for rows.Next() {
		var a ChatAttachment
		a.TurnID = turn
		if err := rows.Scan(&a.ID, &a.Name, &a.MIME, &a.SHA256, &a.Bytes, &a.Key, &a.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

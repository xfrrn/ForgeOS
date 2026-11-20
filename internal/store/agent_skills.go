package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"

	"forgeos/internal/agents"
	"forgeos/internal/skilllib"
	"github.com/lib/pq"
	"go.yaml.in/yaml/v3"
)

const MaxSkillBytes = 1 << 20

var ErrSkillSelection = errors.New("请选择技能库中的 Skill，同一名称只可选择一个版本。")
var ErrSkillDocument = skilllib.ErrDocument

type SkillOption struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Entry       string `json:"entry"`
	FileCount   int    `json:"file_count"`
}

func (s *Store) AgentSkills(ctx context.Context) ([]SkillOption, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,description,package_key,file_count FROM agent_skills WHERE NOT builtin ORDER BY name,id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillOption{}
	for rows.Next() {
		var skill SkillOption
		var key string
		if err := rows.Scan(&skill.ID, &skill.Name, &skill.Description, &key, &skill.FileCount); err != nil {
			return nil, err
		}
		dir, err := skilllib.Dir(key)
		if err != nil {
			return nil, err
		}
		skill.Entry = filepath.Join(dir, "SKILL.md")
		out = append(out, skill)
	}
	return out, rows.Err()
}
func (s *Store) ImportAgentSkill(ctx context.Context, document string) (SkillOption, error) {
	return s.ImportSkillPackage(ctx, []skilllib.File{{Path: "SKILL.md", Data: []byte(document)}})
}
func (s *Store) ImportSkillPackage(ctx context.Context, files []skilllib.File) (SkillOption, error) {
	p, err := skilllib.Publish(files)
	if err != nil {
		return SkillOption{}, err
	}
	o := SkillOption{Name: p.Name, Description: p.Description, Entry: p.Entry, FileCount: p.Count}
	digest, _ := hex.DecodeString(p.Key)
	err = s.db.QueryRowContext(ctx, `INSERT INTO agent_skills(name,description,instructions,fingerprint,package_key,file_count)
		VALUES($1,$2,'',$3,$4,$5) ON CONFLICT(name,fingerprint) DO UPDATE SET name=EXCLUDED.name RETURNING id`, p.Name, p.Description, digest, p.Key, p.Count).Scan(&o.ID)
	return o, err
}
func restoreLegacySkillPackages(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "SELECT id,name,description,instructions FROM agent_skills WHERE NOT builtin AND package_key IS NULL")
	if err != nil {
		return err
	}
	defer rows.Close()
	type legacy struct {
		id                      int64
		name, description, body string
	}
	var pending []legacy
	for rows.Next() {
		var item legacy
		if err = rows.Scan(&item.id, &item.name, &item.description, &item.body); err != nil {
			return err
		}
		pending = append(pending, item)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, item := range pending {
		meta, err := yaml.Marshal(map[string]string{"name": item.name, "description": item.description})
		if err != nil {
			return err
		}
		p, err := skilllib.Publish([]skilllib.File{{Path: "SKILL.md", Data: []byte("---\n" + string(meta) + "---\n" + item.body + "\n")}})
		if err != nil {
			return err
		}
		digest, _ := hex.DecodeString(p.Key)
		if _, err = tx.ExecContext(ctx, "UPDATE agent_skills SET package_key=$2,file_count=1,fingerprint=$3 WHERE id=$1", item.id, p.Key, digest); err != nil {
			return err
		}
	}
	return nil
}
func selectedSkills(ctx context.Context, tx *sql.Tx, ids []int64) ([]agents.SkillRef, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.name,s.package_key FROM unnest($1::bigint[]) WITH ORDINALITY chosen(id,position)
		JOIN agent_skills s USING(id) WHERE NOT s.builtin ORDER BY chosen.position`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var out []agents.SkillRef
	for rows.Next() {
		var ref agents.SkillRef
		if err := rows.Scan(&ref.Name, &ref.Key); err != nil {
			return nil, err
		}
		if seen[ref.Name] {
			return nil, ErrSkillSelection
		}
		seen[ref.Name] = true
		dir, err := skilllib.Dir(ref.Key)
		if err != nil {
			return nil, err
		}
		ref.Entry = filepath.Join(dir, "SKILL.md")
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) != len(ids) {
		return nil, ErrSkillSelection
	}
	return out, nil
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// The workspace revision protects edits made from two different browser tabs.
// Task execution state is read directly from work_items and never copied back.
type PlanningEdit struct {
	// Older clients omit both dates. Explicit empty strings clear the schedule.
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
	Revision  *int64  `json:"revision"`
	Action    string  `json:"action"`
	ID        int64   `json:"id"`
	Target    int64   `json:"target"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	Flag      bool    `json:"flag"`
}

func (e PlanningEdit) Valid() bool {
	if e.Revision == nil || *e.Revision < 0 || e.ID < 0 || e.Target < 0 || len(e.Body) > 128<<10 ||
		!utf8.ValidString(e.Body+e.Title) || strings.ContainsRune(e.Body+e.Title, 0) || utf8.RuneCountInString(e.Title) > 256 {
		return false
	}
	switch e.Action {
	case "overview":
		return true
	case "milestone":
		if (e.StartDate == nil) != (e.EndDate == nil) {
			return false
		}
		if e.StartDate != nil {
			for _, date := range []string{*e.StartDate, *e.EndDate} {
				if date == "" {
					continue
				}
				if _, err := time.Parse(time.DateOnly, date); err != nil || strings.HasPrefix(date, "0000") {
					return false
				}
			}
			if *e.StartDate != "" && *e.EndDate != "" && *e.StartDate > *e.EndDate {
				return false
			}
		}
		return strings.TrimSpace(e.Title) != ""
	case "delivery", "retro", "improvement":
		return strings.TrimSpace(e.Title) != ""
	case "excerpt":
		return e.ID > 0 && strings.TrimSpace(e.Title) != "" && strings.TrimSpace(e.Body) != "" && len(e.Body) <= 8192
	case "draft":
		return e.ID > 0 && strings.TrimSpace(e.Title) != "" && strings.TrimSpace(e.Body) != "" && len(e.Body) <= 16<<10
	case "accept", "order":
		return e.ID > 0
	case "link", "unlink", "carry":
		return e.ID > 0 && e.Target > 0
	default:
		return false
	}
}

const planSnapshot = `SELECT coalesce(jsonb_agg(to_jsonb(m)-'project_id' || jsonb_build_object('deliveries',
    coalesce((SELECT jsonb_agg(to_jsonb(d)-'project_id' ORDER BY d.id) FROM planning_delivery_summary d WHERE d.milestone=m.id),'[]'))
    ORDER BY m.position,m.id),'[]') FROM plan_milestones m WHERE m.project_id=$1 AND ($2::bigint=0 OR m.id=$2)`

func (s *Store) ProjectPlanning(ctx context.Context, p int64) (json.RawMessage, error) {
	return readCurationReceipt(s.db.QueryRowContext(ctx, `SELECT jsonb_build_object('revision',coalesce(pl.revision,0),'overview',coalesce(pl.overview,''),
    'milestones',(`+planSnapshot+`),
    'retros',coalesce((SELECT jsonb_agg(to_jsonb(r)-'project_id' ORDER BY id DESC) FROM plan_retro r WHERE r.project_id=p.id),'[]'),
    'improvements',coalesce((SELECT jsonb_agg(to_jsonb(i)-'project_id' || jsonb_build_object('destination',d.milestone,'delivery',d.id) ORDER BY i.id)
        FROM plan_improvements i LEFT JOIN plan_deliverables d ON d.improvement=i.id WHERE i.project_id=p.id),'[]'),
    'excerpts',coalesce((SELECT jsonb_agg(jsonb_build_object('entry',x.entry,'retro',x.retro,'title',e.title) ORDER BY x.entry)
        FROM plan_excerpts x JOIN experience_library e ON e.id=x.entry WHERE x.project_id=p.id),'[]'),
    'drafts',coalesce((SELECT jsonb_agg(jsonb_build_object('id',a.id,'delivery',a.plan_deliverable,'status',a.status,'title',a.label,'body',a.issue_body,'goal',a.plan_mode='plan') ORDER BY a.id)
        FROM project_chat_actions a WHERE a.project_id=p.id AND a.plan_deliverable IS NOT NULL AND a.status<>'confirmed'),'[]'),
    'tasks',coalesce((SELECT jsonb_agg(to_jsonb(t)-'project_id' ORDER BY id DESC) FROM planning_tasks t WHERE project_id=p.id),'[]'))
    FROM projects p LEFT JOIN project_plans pl ON pl.project_id=p.id WHERE p.id=$1`, p, 0))
}

func (s *Store) PlanningLinks(ctx context.Context, p, task int64) (json.RawMessage, error) {
	return readCurationReceipt(s.db.QueryRowContext(ctx, `SELECT coalesce(jsonb_agg(jsonb_build_object('milestone',m.title,'id',m.id,'delivery_id',d.id,'delivery',d.title) ORDER BY m.position,d.id),'[]')
        FROM planning_linked_tasks l JOIN plan_deliverables d ON d.id=l.deliverable JOIN plan_milestones m ON m.id=d.milestone
        WHERE l.project_id=$1 AND l.task=$2`, p, task))
}

// repo and repositoryID come from the verified GitHub origin when preparing a dispatch.
func (s *Store) EditPlanning(ctx context.Context, p int64, e PlanningEdit, repo string, repositoryID int64) error {
	if !e.Valid() {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO project_plans(project_id) SELECT id FROM projects WHERE id=$1 ON CONFLICT DO NOTHING`, p); err != nil {
		return err
	}
	var rev int64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM project_plans WHERE project_id=$1 FOR UPDATE`, p).Scan(&rev)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if rev != *e.Revision {
		return ErrConflict
	}
	exec := func(q string, args ...any) error {
		r, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err == nil && n == 0 {
			return ErrConflict
		}
		return err
	}
	e.Title = strings.TrimSpace(e.Title)
	switch e.Action {
	case "overview":
		err = exec(`UPDATE project_plans SET overview=$2 WHERE project_id=$1`, p, e.Body)
	case "milestone":
		if e.ID == 0 {
			err = exec(`INSERT INTO plan_milestones(project_id,title,body,position,start_date,end_date) SELECT $1,$2,$3,coalesce(max(position),0)+1,NULLIF($4,'')::date,NULLIF($5,'')::date FROM plan_milestones WHERE project_id=$1`, p, e.Title, e.Body, e.StartDate, e.EndDate)
		} else {
			err = exec(`UPDATE plan_milestones SET title=$3,body=$4,
            start_date=CASE WHEN $5::text IS NULL THEN start_date ELSE NULLIF($5,'')::date END,
            end_date=CASE WHEN $6::text IS NULL THEN end_date ELSE NULLIF($6,'')::date END
            WHERE project_id=$1 AND id=$2`, p, e.ID, e.Title, e.Body, e.StartDate, e.EndDate)
		}
	case "order":
		err = exec(`WITH neighbors AS (SELECT id,position,lag(id) OVER w AS prev,lead(id) OVER w AS next FROM plan_milestones WHERE project_id=$1 WINDOW w AS (ORDER BY position,id)),
            pair AS (SELECT a.id,a.position,b.id AS other,b.position AS dest FROM neighbors a JOIN neighbors b ON b.id=CASE WHEN $3 THEN a.prev ELSE a.next END WHERE a.id=$2)
            UPDATE plan_milestones m SET position=CASE WHEN m.id=x.id THEN x.dest ELSE x.position END FROM pair x WHERE m.id IN (x.id,x.other)`, p, e.ID, e.Flag)
	case "delivery":
		if e.ID == 0 {
			err = exec(`INSERT INTO plan_deliverables(project_id,milestone,title,body) SELECT project_id,id,$3,$4 FROM plan_milestones WHERE project_id=$1 AND id=$2`, p, e.Target, e.Title, e.Body)
		} else {
			err = exec(`UPDATE plan_deliverables SET title=$3,body=$4 WHERE project_id=$1 AND id=$2`, p, e.ID, e.Title, e.Body)
		}
	case "accept":
		err = exec(`UPDATE plan_deliverables SET accepted=$3 WHERE project_id=$1 AND id=$2`, p, e.ID, e.Flag)
	case "link":
		err = exec(`INSERT INTO plan_task_links SELECT $1,d.id,t.id FROM plan_deliverables d JOIN work_items t ON t.project_id=d.project_id
            WHERE d.project_id=$1 AND d.id=$2 AND t.id=$3 ON CONFLICT DO NOTHING`, p, e.ID, e.Target)
	case "unlink":
		err = exec(`DELETE FROM plan_task_links WHERE project_id=$1 AND deliverable=$2 AND task=$3`, p, e.ID, e.Target)
	case "retro":
		if e.ID == 0 {
			err = exec(`INSERT INTO plan_retro(project_id,milestone,title,body,snapshot) SELECT $1,nullif($2::bigint,0),$3,$4,(`+planSnapshot+`)
          WHERE $2::bigint=0 OR EXISTS(SELECT 1 FROM plan_milestones WHERE project_id=$1 AND id=$2)`, p, e.Target, e.Title, e.Body)
		} else {
			err = exec(`UPDATE plan_retro SET title=$3,body=$4 WHERE project_id=$1 AND id=$2`, p, e.ID, e.Title, e.Body)
		}
	case "improvement":
		if e.ID == 0 {
			err = exec(`INSERT INTO plan_improvements(project_id,retro,title,body) SELECT project_id,id,$3,$4 FROM plan_retro WHERE project_id=$1 AND id=$2`, p, e.Target, e.Title, e.Body)
		} else {
			err = exec(`UPDATE plan_improvements i SET title=$3,body=$4 WHERE project_id=$1 AND id=$2 AND NOT EXISTS(SELECT 1 FROM plan_deliverables WHERE improvement=i.id)`, p, e.ID, e.Title, e.Body)
		}
	case "carry":
		err = exec(`INSERT INTO plan_deliverables(project_id,milestone,title,body,improvement)
          SELECT i.project_id,m.id,i.title,i.body,i.id FROM plan_improvements i JOIN plan_retro r ON r.id=i.retro
          LEFT JOIN plan_milestones src ON src.id=r.milestone JOIN plan_milestones m ON m.id=$3 AND m.project_id=i.project_id
          WHERE i.project_id=$1 AND i.id=$2 AND (src.id IS NULL OR m.position>src.position) ON CONFLICT(improvement) DO NOTHING`, p, e.ID, e.Target)
	case "excerpt":
		err = savePlanExcerpt(ctx, tx, p, e)
	case "draft":
		if repo == "" || repositoryID <= 0 {
			return ErrConflict
		}
		mode := ""
		if e.Flag {
			mode = "plan"
		}
		err = exec(`WITH turn AS (INSERT INTO project_chat_turns(project_id,client_id,text,status,answer)
          SELECT $1,gen_random_uuid(),'从项目规划派发：'||d.title,'completed','已准备派发草稿。' FROM plan_deliverables d WHERE project_id=$1 AND id=$2
          RETURNING id) INSERT INTO project_chat_actions(project_id,turn_id,kind,label,repository,repository_id,work_type,issue_body,plan_deliverable,plan_mode)
          SELECT $1,id,'create_issue',$3,$4,$5,'development',$6,$2,$7 FROM turn`, p, e.ID, e.Title, repo, repositoryID, e.Body, mode)
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE project_plans SET revision=revision+1 WHERE project_id=$1`, p); err != nil {
		return err
	}
	return tx.Commit()
}

func savePlanExcerpt(ctx context.Context, tx *sql.Tx, p int64, e PlanningEdit) error {
	var source string
	err := tx.QueryRowContext(ctx, `SELECT title FROM plan_retro WHERE project_id=$1 AND id=$2 AND strpos(body,$3)>0`, p, e.ID, e.Body).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	var entry, change, curation int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO experience_entries(project_id,current_revision) VALUES($1,1) RETURNING id`, p).Scan(&entry); err != nil {
		return err
	}
	source = fmt.Sprintf("项目复盘 #%d · %s", e.ID, source)
	err = tx.QueryRowContext(ctx, `INSERT INTO experience_changes(project_id,entry_id,request_key,expected_revision,new_entry,title,source,content,validity,reason)
        VALUES($1,$2,gen_random_uuid()::text,0,true,$3,$4,$5,'','Owner 从复盘中选取') RETURNING id`, p, entry, e.Title, source, e.Body).Scan(&change)
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO material_curations(project_id,experience_change_id,extraction,extracted_at,review,reviewed_at)
        VALUES($1,$2,jsonb_build_object('actor','owner','facts',jsonb_build_array(jsonb_build_object('statement',$3::text,'quote',$3::text))),now(),
        jsonb_build_object('actor','owner','verdict','approve','level','D1','rationale','Owner 手动选存'),now()) RETURNING id`, p, change, e.Body).Scan(&curation)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO experience_revisions(project_id,entry_id,revision,based_on_revision,curation_id,fact_number,change_reason,actor)
        VALUES($1,$2,1,0,$3,1,'复盘摘录','owner')`, p, entry, curation)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO plan_excerpts(project_id,retro,entry) VALUES($1,$2,$3)`, p, e.ID, entry)
	return err
}

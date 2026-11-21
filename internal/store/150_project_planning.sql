CREATE TABLE project_plans (
    project_id bigint PRIMARY KEY REFERENCES projects(id),
    revision bigint NOT NULL DEFAULT 0,
    overview text NOT NULL DEFAULT ''
);
CREATE TABLE plan_milestones (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES project_plans,
    title text NOT NULL,
    body text NOT NULL,
    position bigint NOT NULL,
    UNIQUE(project_id,id)
);
CREATE TABLE plan_retro (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES project_plans,
    milestone bigint,
    title text NOT NULL,
    body text NOT NULL,
    snapshot jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(project_id,id),
    FOREIGN KEY(project_id,milestone) REFERENCES plan_milestones(project_id,id)
);
CREATE TABLE plan_improvements (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id bigint NOT NULL,
    retro bigint NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    UNIQUE(project_id,id),
    FOREIGN KEY(project_id,retro) REFERENCES plan_retro(project_id,id)
);
CREATE TABLE plan_deliverables (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id bigint NOT NULL,
    milestone bigint NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    accepted boolean NOT NULL DEFAULT false,
    improvement bigint UNIQUE,
    UNIQUE(project_id,id),
    FOREIGN KEY(project_id,milestone) REFERENCES plan_milestones(project_id,id),
    FOREIGN KEY(project_id,improvement) REFERENCES plan_improvements(project_id,id)
);
ALTER TABLE project_chat_actions ADD COLUMN plan_deliverable bigint REFERENCES plan_deliverables,
    ADD COLUMN plan_mode text NOT NULL DEFAULT '' CHECK(plan_mode IN ('','plan'));
CREATE INDEX plan_actions_by_deliverable ON project_chat_actions(plan_deliverable) WHERE plan_deliverable IS NOT NULL;
CREATE TABLE plan_task_links (
    project_id bigint NOT NULL,
    deliverable bigint NOT NULL,
    task bigint NOT NULL REFERENCES work_items,
    PRIMARY KEY(deliverable,task),
    FOREIGN KEY(project_id,deliverable) REFERENCES plan_deliverables(project_id,id)
);
CREATE VIEW planning_linked_tasks AS
SELECT project_id,deliverable,task FROM plan_task_links
UNION
SELECT project_id,plan_deliverable,task_id FROM project_chat_actions WHERE plan_deliverable IS NOT NULL AND task_id IS NOT NULL;
CREATE VIEW planning_tasks AS
SELECT id,project_id,coalesce(nullif(spec->'issue'->>'title',''),nullif(spec->>'title',''),kind) AS title,
       status, coalesce(spec->>'mode'='plan',false) AS goal FROM work_items;
CREATE VIEW planning_delivery_summary AS
SELECT d.*, coalesce((SELECT jsonb_agg((to_jsonb(t)-'project_id') || jsonb_build_object('removable',NOT EXISTS(SELECT 1 FROM project_chat_actions a WHERE a.plan_deliverable=d.id AND a.task_id=t.id)) ORDER BY t.id)
    FROM planning_linked_tasks l JOIN planning_tasks t ON t.id=l.task AND t.project_id=l.project_id
    WHERE l.deliverable=d.id),'[]') AS tasks
FROM plan_deliverables d;
CREATE TABLE plan_excerpts (
    project_id bigint NOT NULL,
    retro bigint NOT NULL,
    entry bigint NOT NULL PRIMARY KEY,
    FOREIGN KEY(project_id,retro) REFERENCES plan_retro(project_id,id),
    FOREIGN KEY(project_id,entry) REFERENCES experience_entries(project_id,id)
);

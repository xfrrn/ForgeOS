CREATE TABLE execution_question_context (
    execution_id bigint NOT NULL REFERENCES executions(id),
    question_id bigint NOT NULL REFERENCES decision_questions(id),
    PRIMARY KEY (execution_id,question_id)
);

CREATE FUNCTION rework_question_ids(work_id bigint, requirement bigint) RETURNS SETOF bigint
LANGUAGE sql STABLE AS $$
    WITH rejected AS (
        SELECT r.execution_id, candidate_implementation_execution(r.execution_id) AS origin
        FROM acceptance_reviews r JOIN executions e ON e.id=r.execution_id
        WHERE r.task_id=work_id AND r.decision='reject' AND e.requirement_id IS NOT DISTINCT FROM requirement
        ORDER BY r.execution_id DESC LIMIT 1
    )
    SELECT c.question_id FROM rejected r JOIN execution_question_context c ON c.execution_id=r.origin
    UNION
    SELECT q.id FROM rejected r JOIN decision_questions q
      ON q.execution_id=CASE WHEN q.checkpoint_sequence IS NULL THEN r.execution_id ELSE r.origin END
    WHERE q.task_id=work_id AND q.answer IS NOT NULL
$$;

CREATE FUNCTION inherited_owner_answers(run_id bigint) RETURNS jsonb
LANGUAGE sql STABLE AS $$
    SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object('id',q.id,'execution_id',q.execution_id,
        'sequence',q.checkpoint_sequence,'question',q.question,'evidence',q.evidence,'answer',q.answer,'commit_sha',q.commit_sha)) ORDER BY q.id)
    FROM execution_question_context c JOIN decision_questions q ON q.id=c.question_id
    WHERE c.execution_id=run_id
$$;

CREATE OR REPLACE FUNCTION review_rules_for_execution(target_id bigint) RETURNS jsonb
LANGUAGE sql STABLE AS $$
    SELECT (CASE WHEN a.content='[]'::jsonb THEN c.context
                 ELSE COALESCE(c.context,'{}'::jsonb) || jsonb_build_object('owner_clarifications',a.content) END)
        || CASE WHEN e.id=target_id THEN '{}'::jsonb ELSE
            jsonb_build_object('implementation_execution_id',e.id,'implementation_commit',e.commit_sha) END
    FROM execution_candidate_identity e JOIN candidate_rule_context c ON c.execution_id=e.id
    CROSS JOIN LATERAL (
        SELECT COALESCE(inherited_owner_answers(e.id),'[]'::jsonb) || COALESCE(
            jsonb_agg(jsonb_build_object('sequence',q.checkpoint_sequence,'question',q.question,'answer',q.answer)
                      ORDER BY q.checkpoint_sequence),'[]'::jsonb) AS content
        FROM decision_questions q WHERE q.execution_id=e.id AND q.checkpoint_sequence IS NOT NULL AND q.answer IS NOT NULL
    ) a
    WHERE e.id=candidate_implementation_execution(target_id)
$$;

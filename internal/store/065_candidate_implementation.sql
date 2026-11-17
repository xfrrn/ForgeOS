CREATE FUNCTION candidate_implementation_execution(target_id bigint) RETURNS bigint
LANGUAGE sql STABLE AS $$
    WITH RECURSIVE lineage AS (
        SELECT id,task_id,requirement_id,result->>'base_commit' AS base_sha,
               result->>'commit' AS commit_sha,COALESCE(result->>'session_id','')<>'' AS from_model
        FROM executions WHERE id=target_id
        UNION ALL
        SELECT prior.id,prior.task_id,prior.requirement_id,prior.result->>'base_commit',
               prior.result->>'commit',COALESCE(prior.result->>'session_id','')<>''
        FROM lineage l
        JOIN LATERAL (
            SELECT e.id,e.task_id,e.requirement_id,e.result
            FROM delivery_refreshes f JOIN executions e ON e.id=f.execution_id AND e.task_id=f.task_id
            WHERE f.task_id=l.task_id AND f.new_base=l.base_sha AND f.candidate=l.commit_sha
              AND e.id<l.id AND e.requirement_id IS NOT DISTINCT FROM l.requirement_id
              AND e.result->>'commit'=f.old_delivery->>'commit'
            ORDER BY e.id DESC LIMIT 1
        ) prior ON NOT l.from_model
    )
    SELECT id FROM lineage WHERE from_model
$$;

CREATE OR REPLACE FUNCTION review_rules_for_execution(target_id bigint) RETURNS jsonb
LANGUAGE sql STABLE AS $$
    SELECT c.context || CASE WHEN e.id=target_id THEN '{}'::jsonb ELSE
        jsonb_build_object('implementation_execution_id',e.id,'implementation_commit',e.result->>'commit') END
    FROM executions e JOIN candidate_rule_context c ON c.execution_id=e.id
    WHERE e.id=candidate_implementation_execution(target_id)
$$;

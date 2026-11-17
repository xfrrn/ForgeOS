CREATE FUNCTION matches_rejected_candidate(work_id bigint, candidate_sha text, native_session text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS(
        SELECT 1 FROM acceptance_reviews r
        WHERE r.task_id=work_id AND r.decision='reject' AND (
            r.commit=candidate_sha OR (native_session<>'' AND EXISTS(
                SELECT 1 FROM execution_candidate_identity original
                WHERE original.id=candidate_implementation_execution(r.execution_id) AND original.session_id=native_session
            ))
        )
    )
$$;

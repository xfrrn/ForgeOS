CREATE TABLE interrupted_writer_turns (
    execution_id bigint NOT NULL,
    sequence integer NOT NULL,
    source_instance bigint NOT NULL,
    session_id text NOT NULL,
    source_process jsonb NOT NULL,
    missing_process jsonb NOT NULL,
    index_tree text NOT NULL,
    worktree_tree text NOT NULL,
    handoff_instance bigint,
    handoff_session text,
    PRIMARY KEY (execution_id,source_instance),
    FOREIGN KEY (execution_id,sequence) REFERENCES execution_checkpoints(execution_id,sequence),
    CHECK ((handoff_instance IS NULL AND handoff_session IS NULL) OR
        (handoff_instance IS NOT NULL AND handoff_instance>(missing_process->>'instance')::bigint
         AND handoff_session IS NOT NULL AND handoff_session<>session_id))
);

ALTER VIEW writer_handoff_sources RENAME TO parked_writer_handoff_sources;
CREATE VIEW writer_handoff_sources AS
SELECT h.*,NULL::bigint AS missing_instance FROM parked_writer_handoff_sources h
UNION ALL
SELECT execution_id,source_instance,session_id,sequence,handoff_instance,handoff_session,NULL::bigint,
    (missing_process->>'instance')::bigint FROM interrupted_writer_turns;

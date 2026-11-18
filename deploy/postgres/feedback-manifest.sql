SELECT sha256 || '  ' || storage_key
FROM feedback_artifacts
WHERE removed_at IS NULL AND (expires_at IS NULL OR expires_at > to_timestamp(:'cutoff'::double precision))
ORDER BY storage_key;

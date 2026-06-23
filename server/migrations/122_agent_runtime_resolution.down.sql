DROP INDEX IF EXISTS idx_agent_runtime_user_profile_online;
DROP INDEX IF EXISTS idx_agent_runtime_user_provider_online;

UPDATE agent a
SET runtime_id = ar.id
FROM agent_runtime ar
WHERE a.runtime_id IS NULL
  AND ar.workspace_id = a.workspace_id
  AND ar.provider = a.runtime_provider
  AND ar.profile_id IS NOT DISTINCT FROM a.runtime_profile_id;

-- Rollback can only restore the old NOT NULL binding shape by choosing a
-- concrete runtime. If the exact provider/profile runtime is gone, pick the
-- oldest runtime in the same workspace so ALTER COLUMN can proceed.
UPDATE agent a
SET runtime_id = (
    SELECT ar.id
    FROM agent_runtime ar
    WHERE ar.workspace_id = a.workspace_id
    ORDER BY ar.created_at ASC, ar.id ASC
    LIMIT 1
)
WHERE a.runtime_id IS NULL
  AND EXISTS (
      SELECT 1
      FROM agent_runtime ar
      WHERE ar.workspace_id = a.workspace_id
  );

ALTER TABLE agent
    ALTER COLUMN runtime_id SET NOT NULL,
    DROP COLUMN runtime_profile_id,
    DROP COLUMN runtime_provider;

ALTER TABLE agent
    ADD COLUMN runtime_provider TEXT,
    ADD COLUMN runtime_profile_id UUID;

UPDATE agent a
SET
    runtime_provider = ar.provider,
    runtime_profile_id = ar.profile_id
FROM agent_runtime ar
WHERE a.runtime_id = ar.id;

UPDATE agent
SET runtime_provider = COALESCE(NULLIF(runtime_config->>'provider', ''), 'legacy_local')
WHERE runtime_provider IS NULL;

ALTER TABLE agent
    ALTER COLUMN runtime_provider SET NOT NULL,
    ALTER COLUMN runtime_provider SET DEFAULT 'legacy_local',
    ALTER COLUMN runtime_id DROP NOT NULL,
    ADD CONSTRAINT agent_runtime_profile_id_fkey
        FOREIGN KEY (runtime_profile_id) REFERENCES runtime_profile(id) ON DELETE SET NULL;

CREATE INDEX idx_agent_runtime_user_provider_online
    ON agent_runtime (workspace_id, owner_id, status, provider, created_at, id)
    WHERE runtime_mode = 'local';

CREATE INDEX idx_agent_runtime_user_profile_online
    ON agent_runtime (workspace_id, owner_id, status, profile_id, created_at, id)
    WHERE runtime_mode = 'local' AND profile_id IS NOT NULL;

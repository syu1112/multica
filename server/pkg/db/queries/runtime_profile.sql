-- Custom Runtime profiles (MUL-3284). Workspace-level definitions of a custom
-- runtime; see migration 120 for the table. Relational integrity (workspace,
-- created_by) is enforced in the application layer — there are no DB FKs.

-- name: CreateRuntimeProfile :one
INSERT INTO runtime_profile (
    workspace_id,
    display_name,
    protocol_family,
    command_name,
    description,
    fixed_args,
    visibility,
    created_by,
    enabled
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetRuntimeProfile :one
SELECT * FROM runtime_profile
WHERE id = $1;

-- name: GetRuntimeProfileForWorkspace :one
SELECT * FROM runtime_profile
WHERE id = $1 AND workspace_id = $2;

-- name: ListRuntimeProfiles :many
SELECT * FROM runtime_profile
WHERE workspace_id = $1
ORDER BY created_at ASC;

-- name: ListEnabledRuntimeProfilesForWorkspace :many
-- Daemon-facing list: only enabled profiles are candidates for a daemon to
-- resolve on PATH and register. Ordered for stable output.
SELECT * FROM runtime_profile
WHERE workspace_id = $1 AND enabled = true
ORDER BY created_at ASC;

-- name: UpdateRuntimeProfile :one
-- Partial update via COALESCE: NULL args leave the column unchanged. The
-- protocol_family is intentionally NOT updatable — changing the underlying
-- backend of an existing profile would silently repoint every agent bound to
-- it onto a different protocol; callers create a new profile instead.
UPDATE runtime_profile
SET display_name = COALESCE(sqlc.narg('display_name'), display_name),
    command_name = COALESCE(sqlc.narg('command_name'), command_name),
    description  = COALESCE(sqlc.narg('description'), description),
    fixed_args   = COALESCE(sqlc.narg('fixed_args'), fixed_args),
    visibility   = COALESCE(sqlc.narg('visibility'), visibility),
    enabled      = COALESCE(sqlc.narg('enabled'), enabled),
    updated_at   = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: DeleteRuntimeProfile :exec
DELETE FROM runtime_profile
WHERE id = $1 AND workspace_id = $2;

-- name: DeleteAgentRuntimesByProfile :many
-- Legacy maintenance helper only. Do not call this from DeleteRuntimeProfile:
-- runtime rows are owner-only resources, and workspace admins must not delete
-- other members' registered runtime instances by deleting a shared profile.
DELETE FROM agent_runtime
WHERE profile_id = $1
RETURNING id, workspace_id, owner_id, daemon_id, provider;

-- name: CountAgentsByProfile :one
-- Counts active (non-archived) agents that require this profile. New agents
-- store the dependency directly on agent.runtime_profile_id; the legacy
-- runtime_id join is retained only for old rows created before runtime
-- capability resolution.
SELECT count(*) FROM agent a
LEFT JOIN agent_runtime ar ON ar.id = a.runtime_id
WHERE a.archived_at IS NULL
  AND (
      a.runtime_profile_id = $1
      OR ar.profile_id = $1
  );

-- name: ListAgentRuntimeIDsByProfile :many
-- Enumerates private runtime instance rows registered against a profile.
-- DeleteRuntimeProfile uses this only as a conflict guard; the runtime rows
-- themselves must be deleted by their owners through the runtime delete path.
SELECT id FROM agent_runtime
WHERE profile_id = $1;

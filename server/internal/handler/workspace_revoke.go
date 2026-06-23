package handler

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// revokeAndRemoveMember converges workspace-owned state that should follow a
// member leaving a workspace: agents pinned to that member's runtimes are
// archived, in-flight tasks on those runtimes are cancelled, and finally the
// member row itself is removed.
//
// Runtime rows and daemon credentials are private user-owned resources.
// Workspace owner/admin role does not grant permission to disable, delete, or
// otherwise take over another member's runtime, even as an indirect side
// effect of membership removal.
//
// All DB writes run inside a single transaction so a partial revocation never
// leaves the workspace half-converged. Once the transaction commits, events
// are published (see publishRevocation) so connected clients observe the
// workspace-owned state changes immediately.
//
// archivedBy is the actor who triggered the revocation. For DeleteMember it's
// the requester (the admin doing the kick); for LeaveWorkspace it's the leaver
// themselves.
func (h *Handler) revokeAndRemoveMember(ctx context.Context, workspaceID, userID, memberID, archivedBy pgtype.UUID) (revocationResult, error) {
	var empty revocationResult

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)

	qtx := h.Queries.WithTx(tx)

	runtimes, err := qtx.ListAgentRuntimesByOwner(ctx, db.ListAgentRuntimesByOwnerParams{
		WorkspaceID: workspaceID,
		OwnerID:     userID,
	})
	if err != nil {
		return empty, err
	}

	result := revocationResult{Runtimes: runtimes}

	if len(runtimes) > 0 {
		runtimeIDs := make([]pgtype.UUID, len(runtimes))
		for i, rt := range runtimes {
			runtimeIDs[i] = rt.ID
		}

		result.ArchivedAgents, err = qtx.ArchiveAgentsByRuntime(ctx, db.ArchiveAgentsByRuntimeParams{
			ArchivedBy: archivedBy,
			RuntimeIds: runtimeIDs,
		})
		if err != nil {
			return empty, err
		}

		// Cancel by runtime AND by archived agent. agent.runtime_id can be
		// reassigned via UpdateAgent without rewriting historical task rows, so
		// an archived agent may still have queued/running tasks pinned to a
		// different runtime. ClaimAgentTask does not gate on agent.archived_at,
		// so those tasks would otherwise stay claimable after the agent is gone.
		archivedAgentIDs := make([]pgtype.UUID, len(result.ArchivedAgents))
		for i, a := range result.ArchivedAgents {
			archivedAgentIDs[i] = a.ID
		}
		result.CancelledTasks, err = qtx.CancelAgentTasksByRuntimeOrAgent(ctx, db.CancelAgentTasksByRuntimeOrAgentParams{
			RuntimeIds: runtimeIDs,
			AgentIds:   archivedAgentIDs,
		})
		if err != nil {
			return empty, err
		}
	}

	// Member row deletion lives inside the same tx so a successful workspace
	// cleanup is never followed by a failed member-delete, and a failed cleanup
	// never leaves the user out of the workspace with stale workspace-owned
	// agent/task state.
	if err := qtx.DeleteMember(ctx, memberID); err != nil {
		return empty, err
	}

	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}

	return result, nil
}

// revocationResult captures everything revokeAndRemoveMember touched so the
// caller can fan out events and analytics after the transaction commits.
// Publishing inside the transaction would let subscribers observe a state the
// tx might still roll back (see TaskService.BroadcastCancelledTasks docstring).
type revocationResult struct {
	Runtimes       []db.AgentRuntime
	ArchivedAgents []db.Agent
	CancelledTasks []db.AgentTaskQueue
}

func (r revocationResult) isEmpty() bool {
	return len(r.Runtimes) == 0
}

// publishRevocation broadcasts task:cancelled with per-agent reconciliation
// and agent:archived events. Safe to call on an empty result.
func (h *Handler) publishRevocation(ctx context.Context, result revocationResult, workspaceIDStr, actorType, actorIDStr string) {
	if result.isEmpty() {
		return
	}

	// Per-task cancellation: TaskService handles status reconciliation and
	// per-task event broadcast. Run this before the agent:archived burst so
	// subscribers see "task cancelled" before the parent agent disappears
	// from active lists, matching the order ArchiveAgent uses.
	if h.TaskService != nil && len(result.CancelledTasks) > 0 {
		h.TaskService.BroadcastCancelledTasks(ctx, result.CancelledTasks)
	}

	for _, agent := range result.ArchivedAgents {
		h.publish(protocol.EventAgentArchived, workspaceIDStr, actorType, actorIDStr, map[string]any{
			"agent": agentToResponse(agent),
		})
	}

	// Do not publish a runtime-list refresh here. Runtime lists are
	// owner-scoped, and membership removal must not expose, disable, delete, or
	// otherwise mutate another member's private runtime.
}

// logRevocation emits a structured info line summarising the workspace-owned
// cleanup. Kept separate from publish so the log is identical whether or not
// the bus is wired.
func logRevocation(result revocationResult, workspaceID, userID string, attrs ...any) {
	if result.isEmpty() {
		return
	}
	base := []any{
		"workspace_id", workspaceID,
		"user_id", userID,
		"runtimes_referenced", len(result.Runtimes),
		"agents_archived", len(result.ArchivedAgents),
		"tasks_cancelled", len(result.CancelledTasks),
	}
	slog.Info("member workspace state revoked", append(base, attrs...)...)
}

package service

import (
	"context"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// AgentReadiness reports whether an agent can accept new work right now.
// "Ready" means archived_at IS NULL and the agent declares a runtime
// capability. Requester-specific online runtime availability is resolved at
// enqueue time by RuntimeResolver.
//
// err is reserved for future checks that need storage access. The current
// implementation is metadata-only and always returns a nil error.
//
// This is the single source of truth shared by:
//   - service.shouldSkipDispatch (autopilot admission gate)
//   - service.dispatchRunOnly    (squad-leader runtime check, MUL-2429)
//   - handler.isSquadLeaderReady (issue-assign / comment-trigger path)
//
// Keeping these aligned matters because the three paths can otherwise drift
// — e.g. one starts allowing "starting" runtimes while another doesn't, and
// the bug only surfaces when a user assigns the same squad through two
// different entry points. Touch this function, all three paths move together.
func AgentReadiness(ctx context.Context, q *db.Queries, agent db.Agent) (ready bool, reason string, err error) {
	_ = ctx
	_ = q
	if agent.ArchivedAt.Valid {
		return false, "agent is archived", nil
	}
	if !agent.RuntimeProfileID.Valid &&
		(agent.RuntimeProvider == "" || agent.RuntimeProvider == "legacy_local") {
		return false, "agent has no runtime capability", nil
	}
	return true, "", nil
}

package service

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestAgentReadinessDoesNotTreatLegacyRuntimeIDAsCapability(t *testing.T) {
	ready, reason, err := AgentReadiness(context.Background(), nil, db.Agent{
		RuntimeID: pgtype.UUID{Valid: true},
	})
	if err != nil {
		t.Fatalf("AgentReadiness returned error: %v", err)
	}
	if ready {
		t.Fatalf("AgentReadiness ready = true, want false")
	}
	if reason != "agent has no runtime capability" {
		t.Fatalf("AgentReadiness reason = %q, want no capability", reason)
	}
}

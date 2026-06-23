package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestDeriveSquadMemberStatus(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	online := "online"
	offline := "offline"
	missing := ""

	tsAgo := func(d time.Duration) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: now.Add(-d), Valid: true}
	}
	tsNone := pgtype.Timestamptz{}

	cases := []struct {
		name          string
		archived      bool
		runtimeStatus string
		lastSeen      pgtype.Timestamptz
		hasActiveTask bool
		want          string
	}{
		{"active wins over offline runtime", false, offline, tsAgo(time.Hour), true, "working"},
		{"active wins over missing runtime", false, missing, tsNone, true, "working"},
		{"online runtime, no task", false, online, tsAgo(2 * time.Second), false, "idle"},
		{"offline runtime, recent heartbeat", false, offline, tsAgo(2 * time.Minute), false, "unstable"},
		{"offline runtime, stale heartbeat", false, offline, tsAgo(2 * time.Hour), false, "offline"},
		{"offline runtime, no heartbeat", false, offline, tsNone, false, "offline"},
		{"no runtime row", false, missing, tsNone, false, "offline"},
		// Archived agents always report archived regardless of any leftover
		// runtime row or task — they should appear in the squad listing
		// but never look like they're still working or merely offline.
		{"archived agent with active task", true, online, tsAgo(time.Second), true, "archived"},
		{"archived agent with online runtime", true, online, tsAgo(time.Second), false, "archived"},
		{"archived agent already offline", true, offline, tsAgo(time.Hour), false, "archived"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveSquadMemberStatus(tc.archived, tc.runtimeStatus, tc.lastSeen, tc.hasActiveTask, now)
			if got != tc.want {
				t.Fatalf("deriveSquadMemberStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestListSquadMemberStatus_DoesNotExposeOtherMemberRuntimeHealth(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()
	runtimeID, runtimeOwnerID, _ := runtimeVisibilityFixture(t)
	adminUserID := createRuntimeVisibilityAdmin(t)

	var agentID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent (
			workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, runtime_provider, visibility, max_concurrent_tasks, owner_id,
			instructions, custom_env, custom_args
		)
		VALUES ($1, 'squad-runtime-privacy-agent', '', 'local', '{}'::jsonb,
		        $2, 'visibility_test_provider', 'workspace', 1, $3, '', '{}'::jsonb, '[]'::jsonb)
		RETURNING id
	`, testWorkspaceID, runtimeID, runtimeOwnerID).Scan(&agentID); err != nil {
		t.Fatalf("create squad privacy agent: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1`, agentID)
	})

	var squadID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO squad (workspace_id, name, description, leader_id, creator_id)
		VALUES ($1, 'Squad Runtime Privacy', '', $2, $3)
		RETURNING id
	`, testWorkspaceID, agentID, runtimeOwnerID).Scan(&squadID); err != nil {
		t.Fatalf("create squad: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM squad WHERE id = $1`, squadID)
	})

	if _, err := testPool.Exec(ctx, `
		INSERT INTO squad_member (squad_id, member_type, member_id, role)
		VALUES ($1, 'agent', $2, 'leader')
	`, squadID, agentID); err != nil {
		t.Fatalf("add squad member: %v", err)
	}

	w := httptest.NewRecorder()
	req := newRequestAs(adminUserID, http.MethodGet, "/api/workspaces/"+testWorkspaceID+"/squads/"+squadID+"/members/status", nil)
	req = withURLParams(req, "workspaceId", testWorkspaceID, "id", squadID)
	testHandler.ListSquadMemberStatus(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ListSquadMemberStatus as workspace admin: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SquadMemberStatusListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode squad status: %v", err)
	}
	for _, member := range resp.Members {
		if member.MemberID == agentID {
			if member.Status == nil {
				t.Fatal("agent member status is nil")
			}
			if *member.Status == "idle" {
				t.Fatalf("workspace admin must not see another member's online runtime as idle")
			}
			return
		}
	}
	t.Fatalf("agent %s missing from squad status response", agentID)
}

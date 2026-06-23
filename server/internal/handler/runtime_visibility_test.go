package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// TestCanUseRuntimeForAgent_Pure exercises the pure predicate behind the
// CreateAgent / UpdateAgent compatibility runtime gate. Runtime invocation is
// owner-only: workspace owner/admin roles and public visibility do not grant
// permission to call another member's local runtime.
func TestCanUseRuntimeForAgent_Pure(t *testing.T) {
	ownerUserID := "11111111-1111-1111-1111-111111111111"
	otherUserID := "22222222-2222-2222-2222-222222222222"

	privateRT := db.AgentRuntime{
		OwnerID:    util.MustParseUUID(ownerUserID),
		Visibility: "private",
	}
	publicRT := db.AgentRuntime{
		OwnerID:    util.MustParseUUID(ownerUserID),
		Visibility: "public",
	}

	cases := []struct {
		name   string
		userID string
		role   string
		rt     db.AgentRuntime
		want   bool
	}{
		// workspace owner / admin do not override runtime ownership
		{"workspace owner on private runtime owned by another", otherUserID, "owner", privateRT, false},
		{"workspace admin on private runtime owned by another", otherUserID, "admin", privateRT, false},
		// runtime owner
		{"runtime owner on own private runtime", ownerUserID, "member", privateRT, true},
		{"runtime owner on own public runtime", ownerUserID, "member", publicRT, true},
		// public visibility no longer grants invocation rights
		{"plain member on someone else's public runtime", otherUserID, "member", publicRT, false},
		{"plain member on someone else's private runtime", otherUserID, "member", privateRT, false},
		{"plain member with empty role on private runtime", otherUserID, "", privateRT, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			member := db.Member{
				UserID: util.MustParseUUID(tc.userID),
				Role:   tc.role,
			}
			got := canUseRuntimeForAgent(member, tc.rt)
			if got != tc.want {
				t.Fatalf("canUseRuntimeForAgent(role=%s, visibility=%s, owner=%s, caller=%s) = %v; want %v",
					tc.role, tc.rt.Visibility, ownerUserID, tc.userID, got, tc.want)
			}
		})
	}
}

// runtimeVisibilityFixture builds the three-actor world the gate needs to
// exercise: a private runtime owned by a non-admin member, a separate plain
// member in the same workspace, and the workspace owner (testUserID). The
// runtime is registered through agent_runtime directly so the test doesn't
// depend on the daemon-registration code path. Returns runtime id, runtime
// owner user id, and the plain member's user id.
func runtimeVisibilityFixture(t *testing.T) (runtimeID, runtimeOwnerID, plainMemberID string) {
	t.Helper()
	ctx := context.Background()

	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email)
		VALUES ('Runtime Owner', 'runtime-owner@multica.test')
		RETURNING id
	`).Scan(&runtimeOwnerID); err != nil {
		t.Fatalf("create runtime owner user: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(),
			`DELETE FROM "user" WHERE email = 'runtime-owner@multica.test'`)
	})

	if _, err := testPool.Exec(ctx, `
		INSERT INTO member (workspace_id, user_id, role)
		VALUES ($1, $2, 'member')
	`, testWorkspaceID, runtimeOwnerID); err != nil {
		t.Fatalf("add runtime owner as member: %v", err)
	}

	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email)
		VALUES ('Plain Runtime Member', 'plain-runtime-member@multica.test')
		RETURNING id
	`).Scan(&plainMemberID); err != nil {
		t.Fatalf("create plain member user: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(),
			`DELETE FROM "user" WHERE email = 'plain-runtime-member@multica.test'`)
	})

	if _, err := testPool.Exec(ctx, `
		INSERT INTO member (workspace_id, user_id, role)
		VALUES ($1, $2, 'member')
	`, testWorkspaceID, plainMemberID); err != nil {
		t.Fatalf("add plain member: %v", err)
	}

	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status,
			device_info, metadata, owner_id, visibility, last_seen_at
		)
		VALUES ($1, NULL, 'Visibility Test Runtime', 'cloud', 'visibility_test_provider', 'online', 'visibility test', '{}'::jsonb, $2, 'private', now())
		RETURNING id
	`, testWorkspaceID, runtimeOwnerID).Scan(&runtimeID); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(),
			`DELETE FROM agent_runtime WHERE id = $1`, runtimeID)
	})

	return runtimeID, runtimeOwnerID, plainMemberID
}

func createRuntimeVisibilityAdmin(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	var adminUserID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email)
		VALUES ('Runtime Visibility Admin', 'runtime-visibility-admin@multica.test')
		RETURNING id
	`).Scan(&adminUserID); err != nil {
		t.Fatalf("create runtime visibility admin user: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(),
			`DELETE FROM "user" WHERE email = 'runtime-visibility-admin@multica.test'`)
	})

	if _, err := testPool.Exec(ctx, `
		INSERT INTO member (workspace_id, user_id, role)
		VALUES ($1, $2, 'admin')
	`, testWorkspaceID, adminUserID); err != nil {
		t.Fatalf("add runtime visibility admin as member: %v", err)
	}
	return adminUserID
}

// TestCreateAgent_RejectsPrivateRuntimeForNonOwner walks the gate end-to-end:
// the runtime is private and owned by a non-admin member, so only the runtime
// owner can select it. Workspace owner/admin roles do not override runtime
// ownership.
func TestCreateAgent_RejectsPrivateRuntimeForNonOwner(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	runtimeID, runtimeOwnerID, plainMemberID := runtimeVisibilityFixture(t)
	adminUserID := createRuntimeVisibilityAdmin(t)

	t.Cleanup(func() {
		testPool.Exec(context.Background(),
			`DELETE FROM agent WHERE workspace_id = $1 AND name LIKE 'runtime-visibility-test-%'`,
			testWorkspaceID)
	})

	body := func(name string) map[string]any {
		return map[string]any{
			"name":                 name,
			"description":          "",
			"runtime_id":           runtimeID,
			"visibility":           "private",
			"max_concurrent_tasks": 1,
		}
	}

	// Workspace owner (testUserID): hidden even though they administer the
	// workspace, because runtime invocation and visibility are owner-only.
	w := httptest.NewRecorder()
	testHandler.CreateAgent(w, newRequest(http.MethodPost, "/api/agents", body("runtime-visibility-test-admin")))
	if w.Code != http.StatusNotFound {
		t.Fatalf("CreateAgent as workspace owner: expected 404, got %d: %s", w.Code, w.Body.String())
	}

	// Workspace admin: also hidden, because admin role does not grant runtime
	// visibility or invocation rights.
	w = httptest.NewRecorder()
	testHandler.CreateAgent(w, newRequestAs(adminUserID, http.MethodPost, "/api/agents", body("runtime-visibility-test-workspace-admin")))
	if w.Code != http.StatusNotFound {
		t.Fatalf("CreateAgent as workspace admin: expected 404, got %d: %s", w.Code, w.Body.String())
	}

	// Runtime owner: allowed because they own the runtime.
	w = httptest.NewRecorder()
	testHandler.CreateAgent(w, newRequestAs(runtimeOwnerID, http.MethodPost, "/api/agents", body("runtime-visibility-test-runtime-owner")))
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateAgent as runtime owner: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Plain member: hidden as not found, like other non-owners.
	w = httptest.NewRecorder()
	testHandler.CreateAgent(w, newRequestAs(plainMemberID, http.MethodPost, "/api/agents", body("runtime-visibility-test-plain-member")))
	if w.Code != http.StatusNotFound {
		t.Fatalf("CreateAgent as plain member on private runtime: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCreateAgent_RejectsPublicRuntimeForPlainMember verifies that public
// visibility does not let another workspace member use the runtime as an
// execution resource.
func TestCreateAgent_RejectsPublicRuntimeForPlainMember(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	runtimeID, _, plainMemberID := runtimeVisibilityFixture(t)
	ctx := context.Background()
	if _, err := testPool.Exec(ctx,
		`UPDATE agent_runtime SET visibility = 'public' WHERE id = $1`, runtimeID,
	); err != nil {
		t.Fatalf("flip runtime to public: %v", err)
	}

	t.Cleanup(func() {
		testPool.Exec(context.Background(),
			`DELETE FROM agent WHERE workspace_id = $1 AND name = 'runtime-visibility-test-public-runtime'`,
			testWorkspaceID)
	})

	body := map[string]any{
		"name":                 "runtime-visibility-test-public-runtime",
		"description":          "",
		"runtime_id":           runtimeID,
		"visibility":           "private",
		"max_concurrent_tasks": 1,
	}
	w := httptest.NewRecorder()
	testHandler.CreateAgent(w, newRequestAs(plainMemberID, http.MethodPost, "/api/agents", body))
	if w.Code != http.StatusNotFound {
		t.Fatalf("CreateAgent as plain member on public runtime: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUpdateAgent_RejectsRebindToPrivateRuntime is the regression for the
// "update can bypass create" backdoor — without this gate a plain member
// could create an agent on a public runtime, then re-bind it onto someone
// else's private runtime via UpdateAgent.
func TestUpdateAgent_RejectsRebindToPrivateRuntime(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	privateRuntimeID, _, plainMemberID := runtimeVisibilityFixture(t)

	ctx := context.Background()
	// Create a public runtime that the plain member can legitimately own
	// an agent on, then we try to move the agent onto the private runtime.
	var publicRuntimeID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status,
			device_info, metadata, owner_id, visibility, last_seen_at
		)
		VALUES ($1, NULL, 'Public Runtime', 'cloud', 'visibility_test_public_provider', 'online', 'public', '{}'::jsonb, $2, 'public', now())
		RETURNING id
	`, testWorkspaceID, plainMemberID).Scan(&publicRuntimeID); err != nil {
		t.Fatalf("create public runtime: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_runtime WHERE id = $1`, publicRuntimeID)
	})

	var agentID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent (
			workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, visibility, max_concurrent_tasks, owner_id,
			instructions, custom_env, custom_args
		)
		VALUES ($1, 'rebind-test-agent', '', 'cloud', '{}'::jsonb,
		        $2, 'private', 1, $3, '', '{}'::jsonb, '[]'::jsonb)
		RETURNING id
	`, testWorkspaceID, publicRuntimeID, plainMemberID).Scan(&agentID); err != nil {
		t.Fatalf("create agent on public runtime: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1`, agentID)
	})

	body := map[string]any{
		"runtime_id": privateRuntimeID,
	}
	w := httptest.NewRecorder()
	req := newRequestAs(plainMemberID, http.MethodPut, "/api/agents/"+agentID, body)
	req = withURLParam(req, "id", agentID)
	testHandler.UpdateAgent(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("UpdateAgent rebinding to private runtime: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateAgent_RuntimeIDCompatibilityClearsLegacyBinding(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	runtimeID, runtimeOwnerID, _ := runtimeVisibilityFixture(t)

	var agentID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent (
			workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, runtime_provider, visibility, max_concurrent_tasks, owner_id,
			instructions, custom_env, custom_args
		)
		VALUES ($1, 'legacy-runtime-compat-agent', '', 'local', '{}'::jsonb,
		        $2, 'legacy_local', 'private', 1, $3, '', '{}'::jsonb, '[]'::jsonb)
		RETURNING id
	`, testWorkspaceID, runtimeID, runtimeOwnerID).Scan(&agentID); err != nil {
		t.Fatalf("create legacy-bound agent: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1`, agentID)
	})

	w := httptest.NewRecorder()
	req := newRequestAs(runtimeOwnerID, http.MethodPut, "/api/agents/"+agentID, map[string]any{
		"runtime_id": runtimeID,
	})
	req = withURLParam(req, "id", agentID)
	testHandler.UpdateAgent(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateAgent runtime_id compatibility: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var legacyRuntimeID pgtype.Text
	var runtimeProvider string
	if err := testPool.QueryRow(context.Background(),
		`SELECT runtime_id::text, runtime_provider FROM agent WHERE id = $1`,
		agentID,
	).Scan(&legacyRuntimeID, &runtimeProvider); err != nil {
		t.Fatalf("read updated agent runtime fields: %v", err)
	}
	if legacyRuntimeID.Valid {
		t.Fatalf("legacy agent.runtime_id should be cleared after capability update, got %s", legacyRuntimeID.String)
	}
	if runtimeProvider != "visibility_test_provider" {
		t.Fatalf("runtime_provider = %q, want visibility_test_provider", runtimeProvider)
	}
}

func TestListAgentRuntimes_HidesOtherMembersFromWorkspaceAdmin(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	otherRuntimeID, _, plainMemberID := runtimeVisibilityFixture(t)
	adminUserID := createRuntimeVisibilityAdmin(t)

	var adminRuntimeID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status,
			device_info, metadata, owner_id, visibility, last_seen_at
		)
		VALUES ($1, NULL, 'Admin Owned Runtime', 'local', 'visibility_admin_provider', 'online', 'admin runtime', '{}'::jsonb, $2, 'private', now())
		RETURNING id
	`, testWorkspaceID, adminUserID).Scan(&adminRuntimeID); err != nil {
		t.Fatalf("create admin-owned runtime: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_runtime WHERE id = $1`, adminRuntimeID)
	})

	var adminCloudRuntimeID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status,
			device_info, metadata, owner_id, visibility, last_seen_at
		)
		VALUES ($1, NULL, 'Admin Cloud Runtime', 'cloud', 'visibility_admin_provider', 'online', 'admin cloud runtime', '{}'::jsonb, $2, 'private', now())
		RETURNING id
	`, testWorkspaceID, adminUserID).Scan(&adminCloudRuntimeID); err != nil {
		t.Fatalf("create admin-owned cloud runtime: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_runtime WHERE id = $1`, adminCloudRuntimeID)
	})

	cases := []struct {
		name       string
		userID     string
		ownRuntime string
	}{
		{"workspace owner", testUserID, ""},
		{"workspace admin", adminUserID, adminRuntimeID},
		{"plain member", plainMemberID, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			testHandler.ListAgentRuntimes(w, newRequestAs(tc.userID, http.MethodGet, "/api/runtimes", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("ListAgentRuntimes: expected 200, got %d: %s", w.Code, w.Body.String())
			}

			var resp []AgentRuntimeResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode runtime list: %v", err)
			}
			ids := map[string]bool{}
			for _, rt := range resp {
				ids[rt.ID] = true
			}
			if tc.ownRuntime != "" && !ids[tc.ownRuntime] {
				t.Fatalf("%s should see their own runtime %s; got %+v", tc.name, tc.ownRuntime, ids)
			}
			if ids[otherRuntimeID] {
				t.Fatalf("%s must not see another member's runtime %s; got %+v", tc.name, otherRuntimeID, ids)
			}
			if ids[adminCloudRuntimeID] {
				t.Fatalf("%s must not see cloud runtimes in the local runtime list; got %+v", tc.name, ids)
			}
		})
	}
}

func TestListAgentRuntimesRequiresWorkspaceMembership(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()
	var removedUserID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email)
		VALUES ('Removed Runtime Owner', 'removed-runtime-owner@multica.test')
		RETURNING id
	`).Scan(&removedUserID); err != nil {
		t.Fatalf("create removed runtime owner: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, removedUserID)
	})

	var runtimeID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status,
			device_info, metadata, owner_id, visibility, last_seen_at
		)
		VALUES ($1, NULL, 'Removed Owner Runtime', 'local', 'removed_owner_provider', 'online', 'removed runtime', '{}'::jsonb, $2, 'private', now())
		RETURNING id
	`, testWorkspaceID, removedUserID).Scan(&runtimeID); err != nil {
		t.Fatalf("create removed-owner runtime: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_runtime WHERE id = $1`, runtimeID)
	})

	w := httptest.NewRecorder()
	testHandler.ListAgentRuntimes(w, newRequestAs(removedUserID, http.MethodGet, "/api/runtimes", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("ListAgentRuntimes without workspace membership: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAgentFromTemplate_RuntimeIDDoesNotRevealOtherOrMissingRuntime(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	var templateSlug string
	for _, tmpl := range agentTemplates.List() {
		if len(tmpl.Skills) == 0 {
			templateSlug = tmpl.Slug
			break
		}
	}
	if templateSlug == "" {
		t.Skip("no prompt-only agent template available")
	}

	otherRuntimeID, _, plainMemberID := runtimeVisibilityFixture(t)
	adminUserID := createRuntimeVisibilityAdmin(t)
	missingRuntimeID := "11111111-2222-3333-4444-555555555555"

	t.Cleanup(func() {
		testPool.Exec(context.Background(),
			`DELETE FROM agent WHERE workspace_id = $1 AND name LIKE 'template-runtime-privacy-%'`,
			testWorkspaceID)
	})

	cases := []struct {
		name      string
		userID    string
		runtimeID string
	}{
		{"workspace owner cannot select another member runtime", testUserID, otherRuntimeID},
		{"workspace admin cannot select another member runtime", adminUserID, otherRuntimeID},
		{"plain member cannot select another member runtime", plainMemberID, otherRuntimeID},
		{"missing runtime is hidden like inaccessible runtime", testUserID, missingRuntimeID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agentName := "template-runtime-privacy-" + tc.name
			w := httptest.NewRecorder()
			testHandler.CreateAgentFromTemplate(w, newRequestAs(tc.userID, http.MethodPost, "/api/agent-templates/create", map[string]any{
				"template_slug":        templateSlug,
				"name":                 agentName,
				"runtime_id":           tc.runtimeID,
				"visibility":           "private",
				"max_concurrent_tasks": 1,
			}))
			if w.Code != http.StatusNotFound {
				t.Fatalf("CreateAgentFromTemplate: expected 404, got %d: %s", w.Code, w.Body.String())
			}

			var count int
			if err := testPool.QueryRow(context.Background(),
				`SELECT COUNT(*) FROM agent WHERE workspace_id = $1 AND name = $2`,
				testWorkspaceID, agentName,
			).Scan(&count); err != nil {
				t.Fatalf("count created agents: %v", err)
			}
			if count != 0 {
				t.Fatalf("CreateAgentFromTemplate created %d agents despite inaccessible runtime", count)
			}
		})
	}
}

func TestAgentResponseHidesLegacyRuntimeIDFromWorkspaceMembers(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	runtimeID, runtimeOwnerID, plainMemberID := runtimeVisibilityFixture(t)

	var agentID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent (
			workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, runtime_provider, visibility, max_concurrent_tasks, owner_id,
			instructions, custom_env, custom_args
		)
		VALUES ($1, 'runtime-response-privacy-agent', '', 'local', '{}'::jsonb,
		        $2, 'visibility_test_provider', 'workspace', 1, $3, '', '{}'::jsonb, '[]'::jsonb)
		RETURNING id
	`, testWorkspaceID, runtimeID, runtimeOwnerID).Scan(&agentID); err != nil {
		t.Fatalf("create legacy-bound agent: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1`, agentID)
	})

	for _, tc := range []struct {
		name   string
		userID string
	}{
		{"runtime owner", runtimeOwnerID},
		{"plain member", plainMemberID},
		{"workspace owner", testUserID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := newRequestAs(tc.userID, http.MethodGet, "/api/agents/"+agentID, nil)
			req = withURLParam(req, "id", agentID)
			testHandler.GetAgent(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("GetAgent: expected 200, got %d: %s", w.Code, w.Body.String())
			}

			var raw map[string]any
			if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if raw["runtime_id"] != nil {
				t.Fatalf("agent response leaked legacy runtime_id to %s: %v", tc.name, raw["runtime_id"])
			}
		})
	}
}

func TestTaskAuditResponseRedactsRuntimeID(t *testing.T) {
	runtimeID := util.MustParseUUID("11111111-1111-1111-1111-111111111111")
	taskID := util.MustParseUUID("22222222-2222-2222-2222-222222222222")
	agentID := util.MustParseUUID("33333333-3333-3333-3333-333333333333")
	resp := taskToAuditResponse(db.AgentTaskQueue{
		ID:        taskID,
		AgentID:   agentID,
		RuntimeID: runtimeID,
		Status:    "completed",
		WorkDir:   pgtype.Text{String: "/tmp/multica/44444444-4444-4444-4444-444444444444/22222222/workdir", Valid: true},
	}, "44444444-4444-4444-4444-444444444444")

	if resp.RuntimeID != "" {
		t.Fatalf("audit task response leaked runtime_id %q", resp.RuntimeID)
	}
	if resp.WorkDir != "" {
		t.Fatalf("audit task response leaked absolute work_dir %q", resp.WorkDir)
	}
	if resp.RelativeWorkDir != "44444444-4444-4444-4444-444444444444/22222222/workdir" {
		t.Fatalf("audit task response relative_work_dir = %q", resp.RelativeWorkDir)
	}
	if resp.ID == "" || resp.AgentID == "" || resp.Status != "completed" {
		t.Fatalf("audit task response lost non-sensitive task fields: %+v", resp)
	}
}

func TestTaskAuditResponseRedactsNestedRuntimeResult(t *testing.T) {
	runtimeID := util.MustParseUUID("11111111-1111-1111-1111-111111111111")
	result, err := json.Marshal(map[string]any{
		"summary":            "finished",
		"runtime_id":         "11111111-1111-1111-1111-111111111111",
		"runtimeId":          "11111111-1111-1111-1111-111111111111",
		"runtime_detail_url": "/api/runtimes/11111111-1111-1111-1111-111111111111",
		"workDir":            "/Users/alice/private/workdir",
		"connection_credentials": map[string]any{
			"token": "secret",
		},
		"connectionCredentials": map[string]any{
			"token": "secret",
		},
		"steps": []map[string]any{
			{
				"name":     "safe",
				"duration": 3,
			},
			{
				"name":                    "unsafe",
				"daemon_operation_params": map[string]any{"runtime_id": "11111111-1111-1111-1111-111111111111"},
				"daemonOperationParams":   map[string]any{"runtimeId": "11111111-1111-1111-1111-111111111111"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	resp := taskToAuditResponse(db.AgentTaskQueue{
		ID:        util.MustParseUUID("22222222-2222-2222-2222-222222222222"),
		AgentID:   util.MustParseUUID("33333333-3333-3333-3333-333333333333"),
		RuntimeID: runtimeID,
		Status:    "completed",
		Result:    result,
	}, "44444444-4444-4444-4444-444444444444")

	resultMap, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", resp.Result)
	}
	if resultMap["summary"] != "finished" {
		t.Fatalf("result lost safe summary: %#v", resultMap)
	}
	for _, key := range []string{"runtime_id", "runtimeId", "runtime_detail_url", "workDir", "connection_credentials", "connectionCredentials"} {
		if _, ok := resultMap[key]; ok {
			t.Fatalf("audit result leaked %q: %#v", key, resultMap)
		}
	}
	steps, ok := resultMap["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("result steps = %#v, want two entries", resultMap["steps"])
	}
	unsafeStep, ok := steps[1].(map[string]any)
	if !ok {
		t.Fatalf("unsafe step type = %T, want map[string]any", steps[1])
	}
	if _, ok := unsafeStep["daemon_operation_params"]; ok {
		t.Fatalf("nested audit result leaked daemon params: %#v", unsafeStep)
	}
	if _, ok := unsafeStep["daemonOperationParams"]; ok {
		t.Fatalf("nested audit result leaked camelCase daemon params: %#v", unsafeStep)
	}
	if unsafeStep["name"] != "unsafe" {
		t.Fatalf("nested audit result lost safe fields: %#v", unsafeStep)
	}
}

func TestTaskMessagePayloadRedactsCamelCaseRuntimeInput(t *testing.T) {
	input, err := json.Marshal(map[string]any{
		"command":   "safe",
		"runtimeId": "11111111-1111-1111-1111-111111111111",
		"workDir":   "/Users/alice/private/workdir",
		"connectionCredentials": map[string]any{
			"token": "secret",
		},
		"steps": []map[string]any{
			{
				"name": "safe",
			},
			{
				"name":                  "unsafe",
				"daemonOperationParams": map[string]any{"runtimeId": "11111111-1111-1111-1111-111111111111"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	payload := taskMessageToPayload(db.TaskMessage{
		Seq:   1,
		Type:  "tool",
		Tool:  pgtype.Text{String: "runtime", Valid: true},
		Input: input,
	}, "22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333")

	if payload.Input["command"] != "safe" {
		t.Fatalf("payload lost safe input: %#v", payload.Input)
	}
	for _, key := range []string{"runtimeId", "workDir", "connectionCredentials"} {
		if _, ok := payload.Input[key]; ok {
			t.Fatalf("task message payload leaked %q: %#v", key, payload.Input)
		}
	}
	steps, ok := payload.Input["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("payload steps = %#v, want two entries", payload.Input["steps"])
	}
	unsafeStep, ok := steps[1].(map[string]any)
	if !ok {
		t.Fatalf("unsafe step type = %T, want map[string]any", steps[1])
	}
	if _, ok := unsafeStep["daemonOperationParams"]; ok {
		t.Fatalf("nested task message input leaked camelCase daemon params: %#v", unsafeStep)
	}
	if unsafeStep["name"] != "unsafe" {
		t.Fatalf("nested task message input lost safe fields: %#v", unsafeStep)
	}
}

func TestRuntimeOwnerOnlyEndpoints_HideRuntimeFromNonOwners(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	runtimeID, _, plainMemberID := runtimeVisibilityFixture(t)
	adminUserID := createRuntimeVisibilityAdmin(t)

	actors := []struct {
		name   string
		userID string
	}{
		{"workspace owner", testUserID},
		{"workspace admin", adminUserID},
		{"plain member", plainMemberID},
	}

	endpoints := []struct {
		name   string
		method string
		path   string
		body   any
		call   func(http.ResponseWriter, *http.Request)
	}{
		{"usage", http.MethodGet, "/api/runtimes/" + runtimeID + "/usage", nil, testHandler.GetRuntimeUsage},
		{"activity", http.MethodGet, "/api/runtimes/" + runtimeID + "/activity", nil, testHandler.GetRuntimeTaskActivity},
		{"usage by agent", http.MethodGet, "/api/runtimes/" + runtimeID + "/usage/by-agent", nil, testHandler.GetRuntimeUsageByAgent},
		{"usage by hour", http.MethodGet, "/api/runtimes/" + runtimeID + "/usage/by-hour", nil, testHandler.GetRuntimeUsageByHour},
		{"list models", http.MethodPost, "/api/runtimes/" + runtimeID + "/models", nil, testHandler.InitiateListModels},
		{"list local skills", http.MethodPost, "/api/runtimes/" + runtimeID + "/local-skills", nil, testHandler.InitiateListLocalSkills},
		{"initiate update", http.MethodPost, "/api/runtimes/" + runtimeID + "/update", map[string]any{"target_version": "v0.0.0"}, testHandler.InitiateUpdate},
		{"patch runtime", http.MethodPatch, "/api/runtimes/" + runtimeID, map[string]any{"visibility": "public"}, testHandler.UpdateAgentRuntime},
		{"delete runtime", http.MethodDelete, "/api/runtimes/" + runtimeID, nil, testHandler.DeleteAgentRuntime},
		{"archive agents and delete", http.MethodPost, "/api/runtimes/" + runtimeID + "/archive-agents-and-delete", map[string]any{"expected_active_agent_ids": []string{}}, testHandler.ArchiveAgentsAndDeleteRuntime},
	}

	for _, actor := range actors {
		for _, endpoint := range endpoints {
			t.Run(actor.name+"/"+endpoint.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				req := newRequestAs(actor.userID, endpoint.method, endpoint.path, endpoint.body)
				req = withURLParam(req, "runtimeId", runtimeID)
				endpoint.call(w, req)
				if w.Code != http.StatusNotFound {
					t.Fatalf("%s as %s: expected 404, got %d: %s", endpoint.name, actor.name, w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestDaemonRuntimeAccess_RejectsWorkspaceAdminForOtherMemberRuntime(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	runtimeID, _, _ := runtimeVisibilityFixture(t)
	adminUserID := createRuntimeVisibilityAdmin(t)

	w := httptest.NewRecorder()
	req := newRequestAs(adminUserID, http.MethodPost, "/api/daemon/runtimes/"+runtimeID+"/tasks/claim", nil)
	req = withURLParam(req, "runtimeId", runtimeID)
	testHandler.ClaimTaskByRuntime(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("ClaimTaskByRuntime as workspace admin: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDaemonRuntimeAccess_RejectsMismatchedDaemonToken(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()
	var runtimeID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status,
			device_info, metadata, owner_id, visibility, last_seen_at
		)
		VALUES ($1, 'owner-daemon', 'Daemon Owned Runtime', 'local', 'visibility_daemon_provider', 'online', 'daemon runtime', '{}'::jsonb, $2, 'private', now())
		RETURNING id
	`, testWorkspaceID, testUserID).Scan(&runtimeID); err != nil {
		t.Fatalf("create daemon runtime: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_runtime WHERE id = $1`, runtimeID)
	})

	w := httptest.NewRecorder()
	req := newDaemonTokenRequest(http.MethodPost, "/api/daemon/heartbeat", map[string]any{
		"runtime_id": runtimeID,
	}, testWorkspaceID, "other-daemon")
	testHandler.DaemonHeartbeat(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("DaemonHeartbeat with mismatched daemon token: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUpdateAgentRuntime_VisibilityPatchApplies pins the invariant that
// a PATCH carrying `visibility` correctly updates the runtime.
func TestUpdateAgentRuntime_VisibilityPatchApplies(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	runtimeID, runtimeOwnerID, _ := runtimeVisibilityFixture(t)

	w := httptest.NewRecorder()
	req := newRequestAs(runtimeOwnerID, http.MethodPatch, "/api/runtimes/"+runtimeID, map[string]any{
		"visibility": "public",
	})
	req = withURLParam(req, "runtimeId", runtimeID)
	testHandler.UpdateAgentRuntime(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH visibility: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp AgentRuntimeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Visibility != "public" {
		t.Fatalf("visibility patch: got %q, want public", resp.Visibility)
	}
}

// TestUpdateAgentRuntime_IgnoresTimezoneField guards the RFC migration that
// dropped `timezone` from UpdateAgentRuntimeRequest: a PATCH body still
// carrying `timezone` must not error, must not echo a `timezone` key back,
// and must still apply the recognised `visibility` field. Timezone is now a
// user-level preference, not a per-runtime one.
func TestUpdateAgentRuntime_IgnoresTimezoneField(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	runtimeID, runtimeOwnerID, _ := runtimeVisibilityFixture(t)

	w := httptest.NewRecorder()
	// NOTE: visibility is "public" (not "workspace"): the runtime visibility
	// enum is private|public — "workspace" would 400 before any mutation,
	// which would not exercise the "visibility still applied" assertion.
	req := newRequestAs(runtimeOwnerID, http.MethodPatch, "/api/runtimes/"+runtimeID, map[string]any{
		"timezone":   "Asia/Tokyo",
		"visibility": "public",
	})
	req = withURLParam(req, "runtimeId", runtimeID)
	testHandler.UpdateAgentRuntime(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH with stray timezone: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// The response must carry no `timezone` key — runtimes have no such field.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, present := raw["timezone"]; present {
		t.Errorf("response unexpectedly contains a timezone key: %s", w.Body.String())
	}

	// `visibility` was still applied.
	var resp AgentRuntimeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Visibility != "public" {
		t.Errorf("visibility patch: got %q, want public", resp.Visibility)
	}
}

// TestUpdateAgentRuntime_InvalidVisibilityReturns400 verifies that an invalid
// visibility value is rejected with 400 before any mutation runs.
func TestUpdateAgentRuntime_InvalidVisibilityReturns400(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	runtimeID, runtimeOwnerID, _ := runtimeVisibilityFixture(t)

	w := httptest.NewRecorder()
	req := newRequestAs(runtimeOwnerID, http.MethodPatch, "/api/runtimes/"+runtimeID, map[string]any{
		"visibility": "everyone",
	})
	req = withURLParam(req, "runtimeId", runtimeID)
	testHandler.UpdateAgentRuntime(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PATCH with invalid visibility: expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUpdateAgentRuntime_VisibilityToggle covers the PATCH endpoint:
// only the runtime owner can flip private/public; workspace owner/admin and
// plain members cannot; an unknown value is rejected with 400.
func TestUpdateAgentRuntime_VisibilityToggle(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	runtimeID, runtimeOwnerID, plainMemberID := runtimeVisibilityFixture(t)
	adminUserID := createRuntimeVisibilityAdmin(t)

	patch := func(actorID string, visibility string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequestAs(actorID, http.MethodPatch, "/api/runtimes/"+runtimeID, map[string]any{
			"visibility": visibility,
		})
		req = withURLParam(req, "runtimeId", runtimeID)
		testHandler.UpdateAgentRuntime(w, req)
		return w
	}

	// Runtime owner flips private → public.
	if w := patch(runtimeOwnerID, "public"); w.Code != http.StatusOK {
		t.Fatalf("UpdateAgentRuntime as runtime owner → public: expected 200, got %d: %s", w.Code, w.Body.String())
	} else {
		var resp AgentRuntimeResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Visibility != "public" {
			t.Fatalf("expected visibility=public, got %q", resp.Visibility)
		}
	}

	// Workspace owner (testUserID) cannot flip someone else's runtime back.
	if w := patch(testUserID, "private"); w.Code != http.StatusNotFound {
		t.Fatalf("UpdateAgentRuntime as workspace owner: expected 404, got %d: %s", w.Code, w.Body.String())
	}

	// Workspace admin cannot edit someone else's runtime either.
	if w := patch(adminUserID, "private"); w.Code != http.StatusNotFound {
		t.Fatalf("UpdateAgentRuntime as workspace admin: expected 404, got %d: %s", w.Code, w.Body.String())
	}

	// Plain member: hidden as not found, regardless of intent.
	if w := patch(plainMemberID, "public"); w.Code != http.StatusNotFound {
		t.Fatalf("UpdateAgentRuntime as plain member: expected 404, got %d: %s", w.Code, w.Body.String())
	}

	// Bad value from the owner: 400.
	if w := patch(runtimeOwnerID, "everyone"); w.Code != http.StatusBadRequest {
		t.Fatalf("UpdateAgentRuntime with invalid visibility: expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

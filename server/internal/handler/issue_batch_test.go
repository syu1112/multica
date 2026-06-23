package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestBatchUpdateNoMutationReturnsZero — regression for #1660.
//
// When the request payload has valid issue_ids but the "updates" field
// is empty, missing, or doesn't decode any known mutation field, the
// handler used to walk every issue, run a no-op UPDATE, and increment
// `updated` for each one — returning {"updated": N} despite changing
// nothing. Reporters saw 200 + a positive count and assumed the call
// worked, then chased a phantom persistence bug.
//
// The fix is "tell the truth": when no mutation field is present, return
// {"updated": 0} immediately so the count matches reality.
func TestBatchUpdateNoMutationReturnsZero(t *testing.T) {
	// Two fresh issues so we can also assert no fields actually changed.
	a := createTestIssue(t, "BU-no-mut A", "todo", "low")
	b := createTestIssue(t, "BU-no-mut B", "todo", "low")
	t.Cleanup(func() { deleteTestIssue(t, a) })
	t.Cleanup(func() { deleteTestIssue(t, b) })

	cases := []struct {
		desc string
		body map[string]any
	}{
		{
			desc: "updates_missing",
			// Most common reporter pattern: status at top level.
			body: map[string]any{"issue_ids": []string{a, b}, "status": "in_progress"},
		},
		{
			desc: "updates_empty_object",
			body: map[string]any{"issue_ids": []string{a, b}, "updates": map[string]any{}},
		},
		{
			desc: "updates_misnamed",
			// Singular "update" instead of plural "updates".
			body: map[string]any{"issue_ids": []string{a, b}, "update": map[string]any{"status": "done"}},
		},
		{
			desc: "updates_unknown_field_only",
			// Payload IS nested correctly, but every key inside `updates` is
			// unknown to the handler — same class of caller mistake as the
			// shapes above. hasMutation must stay false; behavior is already
			// correct, this case locks it in against future regressions.
			body: map[string]any{"issue_ids": []string{a, b}, "updates": map[string]any{"foo": "bar"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := newRequest("POST", "/api/issues/batch-update", tc.body)
			testHandler.BatchUpdateIssues(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			var resp struct {
				Updated int `json:"updated"`
			}
			json.NewDecoder(w.Body).Decode(&resp)
			if resp.Updated != 0 {
				t.Errorf("expected updated=0 when no mutation field present, got %d", resp.Updated)
			}

			// Belt and braces: confirm the issues weren't touched.
			for _, id := range []string{a, b} {
				gw := httptest.NewRecorder()
				gr := newRequest("GET", "/api/issues/"+id, nil)
				gr = withURLParam(gr, "id", id)
				testHandler.GetIssue(gw, gr)
				var got IssueResponse
				json.NewDecoder(gw.Body).Decode(&got)
				if got.Status != "todo" {
					t.Errorf("issue %s: status changed to %q despite no-mutation request", id, got.Status)
				}
			}
		})
	}
}

// TestBatchUpdateValidUpdatesPersistAndCount — positive case to lock in
// happy-path behavior alongside the regression test above.
func TestBatchUpdateValidUpdatesPersistAndCount(t *testing.T) {
	a := createTestIssue(t, "BU-ok A", "todo", "low")
	b := createTestIssue(t, "BU-ok B", "todo", "low")
	t.Cleanup(func() { deleteTestIssue(t, a) })
	t.Cleanup(func() { deleteTestIssue(t, b) })

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/batch-update", map[string]any{
		"issue_ids": []string{a, b},
		"updates":   map[string]any{"status": "in_progress"},
	})
	testHandler.BatchUpdateIssues(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Updated int `json:"updated"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Updated != 2 {
		t.Errorf("expected updated=2, got %d", resp.Updated)
	}
	for _, id := range []string{a, b} {
		gw := httptest.NewRecorder()
		gr := newRequest("GET", "/api/issues/"+id, nil)
		gr = withURLParam(gr, "id", id)
		testHandler.GetIssue(gw, gr)
		var got IssueResponse
		json.NewDecoder(gw.Body).Decode(&got)
		if got.Status != "in_progress" {
			t.Errorf("issue %s: expected status=in_progress, got %q", id, got.Status)
		}
	}
}

func TestBatchUpdateAgentAssignmentUsesExplicitRuntimeID(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()
	const provider = "batch_runtime_choice_provider"

	var firstRuntimeID, selectedRuntimeID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status,
			device_info, metadata, owner_id, visibility, last_seen_at, created_at
		)
		VALUES ($1, NULL, 'Batch Runtime First', 'local', $2, 'online',
		        'first runtime', '{}'::jsonb, $3, 'private', now(), now() - interval '1 hour')
		RETURNING id
	`, testWorkspaceID, provider, testUserID).Scan(&firstRuntimeID); err != nil {
		t.Fatalf("create first runtime: %v", err)
	}
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status,
			device_info, metadata, owner_id, visibility, last_seen_at, created_at
		)
		VALUES ($1, NULL, 'Batch Runtime Selected', 'local', $2, 'online',
		        'selected runtime', '{}'::jsonb, $3, 'private', now(), now())
		RETURNING id
	`, testWorkspaceID, provider, testUserID).Scan(&selectedRuntimeID); err != nil {
		t.Fatalf("create selected runtime: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_runtime WHERE id IN ($1, $2)`, firstRuntimeID, selectedRuntimeID)
	})

	var agentID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent (
			workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, runtime_provider, visibility, max_concurrent_tasks,
			owner_id, instructions, custom_env, custom_args
		)
		VALUES ($1, 'batch explicit runtime agent', '', 'local', '{}'::jsonb,
		        NULL, $2, 'workspace', 1, $3, '', '{}'::jsonb, '[]'::jsonb)
		RETURNING id
	`, testWorkspaceID, provider, testUserID).Scan(&agentID); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1`, agentID)
	})

	issueID := createTestIssue(t, "BU-explicit-runtime", "todo", "low")
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE issue_id = $1`, issueID)
		deleteTestIssue(t, issueID)
	})

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/batch-update", map[string]any{
		"issue_ids": []string{issueID},
		"updates": map[string]any{
			"assignee_type": "agent",
			"assignee_id":   agentID,
			"runtime_id":    selectedRuntimeID,
		},
	})
	testHandler.BatchUpdateIssues(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BatchUpdateIssues: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var taskRuntimeID string
	if err := testPool.QueryRow(ctx, `
		SELECT runtime_id::text
		FROM agent_task_queue
		WHERE issue_id = $1 AND agent_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, issueID, agentID).Scan(&taskRuntimeID); err != nil {
		t.Fatalf("load queued task runtime: %v", err)
	}
	if taskRuntimeID != selectedRuntimeID {
		t.Fatalf("task runtime_id = %s, want explicitly selected runtime %s (not first compatible %s)", taskRuntimeID, selectedRuntimeID, firstRuntimeID)
	}
}

func TestUpdateIssueAgentAssignmentRejectsExplicitRuntimeOwnedByOtherUser(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()
	runtimeID, _, _ := runtimeVisibilityFixture(t)
	const provider = "visibility_test_provider"

	var agentID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent (
			workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, runtime_provider, visibility, max_concurrent_tasks,
			owner_id, instructions, custom_env, custom_args
		)
		VALUES ($1, 'update reject foreign runtime agent', '', 'local', '{}'::jsonb,
		        NULL, $2, 'workspace', 1, $3, '', '{}'::jsonb, '[]'::jsonb)
		RETURNING id
	`, testWorkspaceID, provider, testUserID).Scan(&agentID); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1`, agentID)
	})

	issueID := createTestIssue(t, "Update-reject-foreign-runtime", "todo", "low")
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE issue_id = $1`, issueID)
		deleteTestIssue(t, issueID)
	})

	w := httptest.NewRecorder()
	req := newRequest("PATCH", "/api/issues/"+issueID, map[string]any{
		"assignee_type": "agent",
		"assignee_id":   agentID,
		"runtime_id":    runtimeID,
	})
	req = withURLParam(req, "id", issueID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("UpdateIssue: expected 422 for runtime owned by another user, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["code"] != "agent_unavailable" {
		t.Fatalf("response code = %v, want agent_unavailable", resp["code"])
	}

	var taskCount int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agent_task_queue WHERE issue_id = $1`,
		issueID,
	).Scan(&taskCount); err != nil {
		t.Fatalf("count queued tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("queued task count = %d, want 0", taskCount)
	}

	var assigneeType pgtype.Text
	var assigneeID pgtype.UUID
	if err := testPool.QueryRow(ctx,
		`SELECT assignee_type, assignee_id FROM issue WHERE id = $1`,
		issueID,
	).Scan(&assigneeType, &assigneeID); err != nil {
		t.Fatalf("load issue assignee: %v", err)
	}
	if assigneeType.Valid || assigneeID.Valid {
		t.Fatalf("issue assignment persisted despite runtime rejection: type=%v id=%v", assigneeType, assigneeID)
	}
}

func TestBatchUpdateAgentAssignmentRejectsExplicitRuntimeOwnedByOtherUser(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()
	runtimeID, _, _ := runtimeVisibilityFixture(t)
	const provider = "visibility_test_provider"

	var agentID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent (
			workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, runtime_provider, visibility, max_concurrent_tasks,
			owner_id, instructions, custom_env, custom_args
		)
		VALUES ($1, 'batch reject foreign runtime agent', '', 'local', '{}'::jsonb,
		        NULL, $2, 'workspace', 1, $3, '', '{}'::jsonb, '[]'::jsonb)
		RETURNING id
	`, testWorkspaceID, provider, testUserID).Scan(&agentID); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1`, agentID)
	})

	issueID := createTestIssue(t, "Batch-reject-foreign-runtime", "todo", "low")
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE issue_id = $1`, issueID)
		deleteTestIssue(t, issueID)
	})

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/batch-update", map[string]any{
		"issue_ids": []string{issueID},
		"updates": map[string]any{
			"assignee_type": "agent",
			"assignee_id":   agentID,
			"runtime_id":    runtimeID,
		},
	})
	testHandler.BatchUpdateIssues(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("BatchUpdateIssues: expected 422 for runtime owned by another user, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["code"] != "agent_unavailable" {
		t.Fatalf("response code = %v, want agent_unavailable", resp["code"])
	}

	var taskCount int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agent_task_queue WHERE issue_id = $1`,
		issueID,
	).Scan(&taskCount); err != nil {
		t.Fatalf("count queued tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("queued task count = %d, want 0", taskCount)
	}

	var assigneeType pgtype.Text
	var assigneeID pgtype.UUID
	if err := testPool.QueryRow(ctx,
		`SELECT assignee_type, assignee_id FROM issue WHERE id = $1`,
		issueID,
	).Scan(&assigneeType, &assigneeID); err != nil {
		t.Fatalf("load issue assignee: %v", err)
	}
	if assigneeType.Valid || assigneeID.Valid {
		t.Fatalf("issue assignment persisted despite runtime rejection: type=%v id=%v", assigneeType, assigneeID)
	}
}

func TestCreateIssueAgentAssignmentRejectsMissingRequesterRuntime(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()

	var agentID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent (
			workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, runtime_provider, visibility, max_concurrent_tasks,
			owner_id, instructions, custom_env, custom_args
		)
		VALUES ($1, 'create reject no requester runtime agent', '', 'local', '{}'::jsonb,
		        NULL, 'missing_create_issue_runtime_provider', 'workspace', 1,
		        $2, '', '{}'::jsonb, '[]'::jsonb)
		RETURNING id
	`, testWorkspaceID, testUserID).Scan(&agentID); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1`, agentID)
	})

	title := "Create-reject-no-requester-runtime"
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id = $1 AND title = $2`, testWorkspaceID, title)
	})

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title":         title,
		"status":        "todo",
		"assignee_type": "agent",
		"assignee_id":   agentID,
	})
	testHandler.CreateIssue(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("CreateIssue: expected 422, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["code"] != "agent_unavailable" {
		t.Fatalf("response code = %v, want agent_unavailable", resp["code"])
	}

	var issueCount int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM issue WHERE workspace_id = $1 AND title = $2`,
		testWorkspaceID, title,
	).Scan(&issueCount); err != nil {
		t.Fatalf("count issues: %v", err)
	}
	if issueCount != 0 {
		t.Fatalf("issue count = %d, want 0", issueCount)
	}

	var taskCount int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agent_task_queue WHERE agent_id = $1`,
		agentID,
	).Scan(&taskCount); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("task count = %d, want 0", taskCount)
	}
}

func TestCreateIssueSquadAssignmentRejectsMissingRequesterRuntime(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()

	var leaderID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent (
			workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, runtime_provider, visibility, max_concurrent_tasks,
			owner_id, instructions, custom_env, custom_args
		)
		VALUES ($1, 'create reject squad no requester runtime leader', '', 'local', '{}'::jsonb,
		        NULL, 'missing_create_squad_runtime_provider', 'workspace', 1,
		        $2, '', '{}'::jsonb, '[]'::jsonb)
		RETURNING id
	`, testWorkspaceID, testUserID).Scan(&leaderID); err != nil {
		t.Fatalf("create leader agent: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1`, leaderID)
	})

	var squadID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO squad (workspace_id, name, description, leader_id, owner_id)
		VALUES ($1, 'create reject no requester runtime squad', '', $2, $3)
		RETURNING id
	`, testWorkspaceID, leaderID, testUserID).Scan(&squadID); err != nil {
		t.Fatalf("create squad: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM squad WHERE id = $1`, squadID)
	})

	title := "Create-reject-squad-no-requester-runtime"
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id = $1 AND title = $2`, testWorkspaceID, title)
	})

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title":         title,
		"status":        "todo",
		"assignee_type": "squad",
		"assignee_id":   squadID,
	})
	testHandler.CreateIssue(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("CreateIssue: expected 422, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["code"] != "agent_unavailable" {
		t.Fatalf("response code = %v, want agent_unavailable", resp["code"])
	}

	var issueCount int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM issue WHERE workspace_id = $1 AND title = $2`,
		testWorkspaceID, title,
	).Scan(&issueCount); err != nil {
		t.Fatalf("count issues: %v", err)
	}
	if issueCount != 0 {
		t.Fatalf("issue count = %d, want 0", issueCount)
	}

	var taskCount int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agent_task_queue WHERE agent_id = $1`,
		leaderID,
	).Scan(&taskCount); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("task count = %d, want 0", taskCount)
	}
}

// createTestIssue is a small helper to keep the table-driven cases clean.
// Returns the new issue's id; caller is responsible for cleanup.
func createTestIssue(t *testing.T, title, status, priority string) string {
	t.Helper()
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title":    title,
		"status":   status,
		"priority": priority,
	})
	testHandler.CreateIssue(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateIssue %q: expected 201, got %d: %s", title, w.Code, w.Body.String())
	}
	var issue IssueResponse
	json.NewDecoder(w.Body).Decode(&issue)
	return issue.ID
}

func deleteTestIssue(t *testing.T, id string) {
	t.Helper()
	w := httptest.NewRecorder()
	req := newRequest("DELETE", "/api/issues/"+id, nil)
	req = withURLParam(req, "id", id)
	testHandler.DeleteIssue(w, req)
}

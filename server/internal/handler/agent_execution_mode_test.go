package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAgent_ExecutionMode(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()
	t.Cleanup(func() {
		testPool.Exec(ctx,
			`DELETE FROM agent WHERE workspace_id = $1 AND name LIKE 'execution-mode-create-%'`,
			testWorkspaceID,
		)
	})

	t.Run("omitted defaults to normal", func(t *testing.T) {
		body := map[string]any{
			"name":                 "execution-mode-create-default",
			"runtime_id":           testRuntimeID,
			"visibility":           "private",
			"max_concurrent_tasks": 1,
		}
		w := httptest.NewRecorder()
		testHandler.CreateAgent(w, newRequest(http.MethodPost, "/api/agents", body))
		if w.Code != http.StatusCreated {
			t.Fatalf("CreateAgent default: expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["execution_mode"] != "normal" {
			t.Fatalf("expected execution_mode=normal, got %v", resp["execution_mode"])
		}

		var stored string
		if err := testPool.QueryRow(ctx, `SELECT execution_mode FROM agent WHERE id = $1`, resp["id"]).Scan(&stored); err != nil {
			t.Fatalf("load stored execution_mode: %v", err)
		}
		if stored != "normal" {
			t.Fatalf("stored execution_mode = %q, want normal", stored)
		}
	})

	t.Run("goal persists", func(t *testing.T) {
		body := map[string]any{
			"name":                 "execution-mode-create-goal",
			"runtime_id":           testRuntimeID,
			"visibility":           "private",
			"max_concurrent_tasks": 1,
			"execution_mode":       "goal",
		}
		w := httptest.NewRecorder()
		testHandler.CreateAgent(w, newRequest(http.MethodPost, "/api/agents", body))
		if w.Code != http.StatusCreated {
			t.Fatalf("CreateAgent goal: expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["execution_mode"] != "goal" {
			t.Fatalf("expected execution_mode=goal, got %v", resp["execution_mode"])
		}
	})

	t.Run("invalid value is rejected", func(t *testing.T) {
		body := map[string]any{
			"name":                 "execution-mode-create-invalid",
			"runtime_id":           testRuntimeID,
			"visibility":           "private",
			"max_concurrent_tasks": 1,
			"execution_mode":       "autonomous",
		}
		w := httptest.NewRecorder()
		testHandler.CreateAgent(w, newRequest(http.MethodPost, "/api/agents", body))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid execution_mode: expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestUpdateAgent_ExecutionMode(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "execution-mode-update-agent", []byte(`{}`))

	t.Run("switches from normal to goal", func(t *testing.T) {
		body := map[string]any{
			"execution_mode": "goal",
		}
		w := httptest.NewRecorder()
		req := withURLParam(newRequest(http.MethodPatch, "/api/agents/"+agentID, body), "id", agentID)
		testHandler.UpdateAgent(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("UpdateAgent goal: expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["execution_mode"] != "goal" {
			t.Fatalf("expected execution_mode=goal, got %v", resp["execution_mode"])
		}
	})

	t.Run("omitted field leaves value alone", func(t *testing.T) {
		body := map[string]any{
			"name": "execution-mode-update-renamed",
		}
		w := httptest.NewRecorder()
		req := withURLParam(newRequest(http.MethodPatch, "/api/agents/"+agentID, body), "id", agentID)
		testHandler.UpdateAgent(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("UpdateAgent omitted: expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["execution_mode"] != "goal" {
			t.Fatalf("omitted execution_mode changed value: got %v, want goal", resp["execution_mode"])
		}
	})

	t.Run("switches back to normal", func(t *testing.T) {
		body := map[string]any{
			"execution_mode": "normal",
		}
		w := httptest.NewRecorder()
		req := withURLParam(newRequest(http.MethodPatch, "/api/agents/"+agentID, body), "id", agentID)
		testHandler.UpdateAgent(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("UpdateAgent normal: expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["execution_mode"] != "normal" {
			t.Fatalf("expected execution_mode=normal, got %v", resp["execution_mode"])
		}

		var stored string
		if err := testPool.QueryRow(ctx, `SELECT execution_mode FROM agent WHERE id = $1`, agentID).Scan(&stored); err != nil {
			t.Fatalf("load stored execution_mode: %v", err)
		}
		if stored != "normal" {
			t.Fatalf("stored execution_mode = %q, want normal", stored)
		}
	})

	t.Run("invalid value is rejected", func(t *testing.T) {
		body := map[string]any{
			"execution_mode": "autonomous",
		}
		w := httptest.NewRecorder()
		req := withURLParam(newRequest(http.MethodPatch, "/api/agents/"+agentID, body), "id", agentID)
		testHandler.UpdateAgent(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid execution_mode: expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestClaimTaskByRuntime_AgentExecutionModePayload(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "Execution mode claim runtime")
	agentID, issueID := createClaimReclaimAgentAndIssue(t, ctx, runtimeID, "Execution mode claim agent")
	if _, err := testPool.Exec(ctx, `UPDATE agent SET execution_mode = 'goal' WHERE id = $1`, agentID); err != nil {
		t.Fatalf("setup: set execution_mode: %v", err)
	}
	taskID := createDispatchedClaimFixtureTask(t, ctx, agentID, runtimeID, issueID, "120 seconds", false)

	task, body := claimTaskByRuntimeForTest(t, runtimeID)
	if task == nil {
		t.Fatalf("expected task %s to be claimed, got nil response: %s", taskID, body)
	}
	if task.ID != taskID {
		t.Fatalf("claimed task id = %s, want %s", task.ID, taskID)
	}

	var resp struct {
		Agent *TaskAgentData `json:"agent"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode claim response: %v", err)
	}
	if resp.Agent == nil {
		t.Fatalf("expected agent payload in claim response: %s", body)
	}
	if resp.Agent.ExecutionMode != "goal" {
		t.Fatalf("agent execution_mode = %q, want goal", resp.Agent.ExecutionMode)
	}
}

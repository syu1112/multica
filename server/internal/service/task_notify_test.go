package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type recordingTaskWakeup struct {
	calls []struct{ runtimeID, taskID string }
}

func (r *recordingTaskWakeup) NotifyTaskAvailable(runtimeID, taskID string) {
	r.calls = append(r.calls, struct{ runtimeID, taskID string }{runtimeID, taskID})
}

func TestNotifyTaskAvailableAllowsNilEmptyClaimCache(t *testing.T) {
	runtimeID := testUUID(1)
	taskID := testUUID(2)
	wakeup := &recordingTaskWakeup{}
	svc := &TaskService{Wakeup: wakeup}

	svc.notifyTaskAvailable(db.AgentTaskQueue{
		ID:        taskID,
		RuntimeID: runtimeID,
	})

	if got := len(wakeup.calls); got != 1 {
		t.Fatalf("expected 1 wakeup call, got %d", got)
	}
	if wakeup.calls[0].runtimeID != util.UUIDToString(runtimeID) {
		t.Fatalf("runtime wakeup = %q, want %q", wakeup.calls[0].runtimeID, util.UUIDToString(runtimeID))
	}
	if wakeup.calls[0].taskID != util.UUIDToString(taskID) {
		t.Fatalf("task wakeup = %q, want %q", wakeup.calls[0].taskID, util.UUIDToString(taskID))
	}
}

func TestNotifyTaskAvailableBumpsBeforeWakeup(t *testing.T) {
	rdb := newRedisTestClient(t)
	cache := NewEmptyClaimCache(rdb)
	wakeup := &recordingTaskWakeup{}

	svc := &TaskService{
		EmptyClaim: cache,
		Wakeup:     wakeup,
	}

	runtimeID := testUUID(7)
	taskID := testUUID(8)
	runtimeKey := util.UUIDToString(runtimeID)

	ctx := context.Background()
	v0 := cache.CurrentVersion(ctx, runtimeKey)
	cache.MarkEmpty(ctx, runtimeKey, v0)
	if !cache.IsEmpty(ctx, runtimeKey) {
		t.Fatal("precondition: cache should report empty after MarkEmpty under current version")
	}

	svc.notifyTaskAvailable(db.AgentTaskQueue{
		ID:        taskID,
		RuntimeID: runtimeID,
	})

	if cache.IsEmpty(ctx, runtimeKey) {
		t.Fatal("notifyTaskAvailable must bump the version so the prior empty verdict is rejected")
	}
	if got := len(wakeup.calls); got != 1 {
		t.Fatalf("expected 1 wakeup call, got %d", got)
	}
	if wakeup.calls[0].runtimeID != runtimeKey {
		t.Fatalf("wakeup runtime mismatch: got %q want %q", wakeup.calls[0].runtimeID, runtimeKey)
	}
	if wakeup.calls[0].taskID != util.UUIDToString(taskID) {
		t.Fatalf("wakeup task mismatch: got %q want %q", wakeup.calls[0].taskID, util.UUIDToString(taskID))
	}
}

func TestNotifyTaskAvailableInvalidWithoutRuntimeIsNoOp(t *testing.T) {
	rdb := newRedisTestClient(t)
	cache := NewEmptyClaimCache(rdb)
	wakeup := &recordingTaskWakeup{}

	svc := &TaskService{
		EmptyClaim: cache,
		Wakeup:     wakeup,
	}

	ctx := context.Background()
	v0 := cache.CurrentVersion(ctx, "rt-stays")
	cache.MarkEmpty(ctx, "rt-stays", v0)

	svc.notifyTaskAvailable(db.AgentTaskQueue{
		ID: testUUID(9),
	})

	if !cache.IsEmpty(ctx, "rt-stays") {
		t.Fatal("notifyTaskAvailable without a runtime must not bump unrelated empty verdicts")
	}
	if got := len(wakeup.calls); got != 0 {
		t.Fatalf("expected no wakeup calls, got %d", got)
	}
}

func TestBroadcastTaskDispatchRedactsRuntimeContext(t *testing.T) {
	bus := events.New()
	eventsCh := make(chan events.Event, 1)
	bus.Subscribe(protocol.EventTaskDispatch, func(e events.Event) {
		eventsCh <- e
	})

	workspaceID := testUUID(10)
	taskID := testUUID(11)
	agentID := testUUID(12)
	runtimeID := testUUID(14)
	contextJSON, err := json.Marshal(map[string]any{
		"type":                   QuickCreateContextType,
		"prompt":                 "create an issue",
		"requester_id":           util.UUIDToString(testUUID(15)),
		"workspace_id":           util.UUIDToString(workspaceID),
		"runtime_id":             util.UUIDToString(runtimeID),
		"connection_credentials": "secret",
		"daemon_operation":       "claim",
	})
	if err != nil {
		t.Fatalf("marshal context: %v", err)
	}

	svc := &TaskService{Bus: bus}
	svc.broadcastTaskDispatch(context.Background(), db.AgentTaskQueue{
		ID:        taskID,
		AgentID:   agentID,
		Context:   contextJSON,
		RuntimeID: runtimeID,
	})

	select {
	case event := <-eventsCh:
		payload, ok := event.Payload.(map[string]any)
		if !ok {
			t.Fatalf("payload type = %T, want map[string]any", event.Payload)
		}
		for _, key := range []string{"runtime_id", "connection_credentials", "daemon_operation"} {
			if _, ok := payload[key]; ok {
				t.Fatalf("dispatch payload leaked %q: %#v", key, payload)
			}
		}
		if payload["task_id"] != util.UUIDToString(taskID) {
			t.Fatalf("task_id = %v, want %s", payload["task_id"], util.UUIDToString(taskID))
		}
		if payload["agent_id"] != util.UUIDToString(agentID) {
			t.Fatalf("agent_id = %v, want %s", payload["agent_id"], util.UUIDToString(agentID))
		}
		if payload["issue_id"] != "" {
			t.Fatalf("issue_id = %v, want empty for quick-create dispatch", payload["issue_id"])
		}
	default:
		t.Fatal("expected task dispatch event")
	}
}

func TestAgentToMapIncludesRuntimeCapabilityWithoutBinding(t *testing.T) {
	profileID := testUUID(16)
	agent := db.Agent{
		ID:               testUUID(17),
		WorkspaceID:      testUUID(18),
		RuntimeID:        testUUID(19),
		RuntimeProvider:  "codex",
		RuntimeProfileID: profileID,
		Name:             "capability agent",
		RuntimeMode:      "local",
		Visibility:       "workspace",
		Status:           "active",
		OwnerID:          testUUID(20),
	}

	payload := agentToMap(agent)

	if payload["runtime_id"] != nil {
		t.Fatalf("agent event leaked concrete runtime_id: %#v", payload)
	}
	if payload["runtime_provider"] != "codex" {
		t.Fatalf("runtime_provider = %#v, want codex", payload["runtime_provider"])
	}
	if payload["runtime_profile_id"] != util.UUIDToString(profileID) {
		t.Fatalf("runtime_profile_id = %#v, want %s", payload["runtime_profile_id"], util.UUIDToString(profileID))
	}
}

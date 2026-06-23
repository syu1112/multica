import { describe, expect, it } from "vitest";
import type { Agent, AgentTask } from "@multica/core/types";
import { buildWorkloadIndex } from "./runtime-list";

function makeAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    id: "agent-1",
    workspace_id: "ws-1",
    runtime_id: "runtime-1",
    runtime_provider: "codex",
    runtime_profile_id: null,
    name: "Agent",
    description: "",
    instructions: "",
    avatar_url: null,
    runtime_mode: "local",
    runtime_config: {},
    custom_args: [],
    visibility: "private",
    status: "idle",
    max_concurrent_tasks: 1,
    model: "gpt-5.4",
    owner_id: "user-1",
    skills: [],
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    archived_at: null,
    archived_by: null,
    ...overrides,
  };
}

function makeTask(overrides: Partial<AgentTask> = {}): AgentTask {
  return {
    id: "task-1",
    agent_id: "agent-1",
    issue_id: "issue-1",
    status: "running",
    priority: 1,
    dispatched_at: null,
    started_at: null,
    completed_at: null,
    result: null,
    error: null,
    created_at: "2026-01-01T00:00:00Z",
    runtime_id: "runtime-1",
    attempt: 1,
    ...overrides,
  };
}

describe("buildWorkloadIndex", () => {
  it("does not infer per-runtime agents or workload from legacy agent runtime bindings", () => {
    const legacyBoundAgent = makeAgent({ id: "legacy-agent" });
    const task = makeTask({
      id: "legacy-task",
      agent_id: legacyBoundAgent.id,
      status: "running",
    });

    const workload = buildWorkloadIndex([legacyBoundAgent], [task]);

    expect(workload.size).toBe(0);
  });
});

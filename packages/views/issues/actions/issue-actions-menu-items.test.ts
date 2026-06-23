import { describe, expect, it } from "vitest";
import type { AgentTask } from "@multica/core/types";
import { pickLatestWorkDir } from "./issue-actions-menu-items";

function task(overrides: Partial<AgentTask>): AgentTask {
  return {
    id: "task-1",
    agent_id: "agent-1",
    issue_id: "issue-1",
    status: "completed",
    priority: 0,
    dispatched_at: null,
    started_at: null,
    completed_at: null,
    result: null,
    error: null,
    created_at: "2026-01-01T00:00:00Z",
    runtime_id: "",
    attempt: 1,
    ...overrides,
  };
}

describe("pickLatestWorkDir", () => {
  it("copies only the privacy-safe relative workdir", () => {
    const latest = task({
      id: "latest",
      created_at: "2026-01-01T00:00:02Z",
      work_dir: "/Users/alice/.multica/workspaces/ws/task/workdir",
      relative_work_dir: "ws/task/workdir",
    });
    const older = task({
      id: "older",
      created_at: "2026-01-01T00:00:01Z",
      work_dir: "/Users/alice/.multica/workspaces/ws/older/workdir",
      relative_work_dir: "ws/older/workdir",
    });

    expect(pickLatestWorkDir([older, latest])).toBe("ws/task/workdir");
  });

  it("does not fall back to the absolute workdir", () => {
    expect(
      pickLatestWorkDir([
        task({
          work_dir: "/Users/alice/.multica/workspaces/ws/task/workdir",
        }),
      ]),
    ).toBeUndefined();
  });
});

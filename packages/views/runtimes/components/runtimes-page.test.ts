import { describe, expect, it } from "vitest";
import type { AgentRuntime } from "@multica/core/types";
import { filterOwnedRuntimes } from "./runtimes-page";

function makeRuntime(overrides: Partial<AgentRuntime> = {}): AgentRuntime {
  return {
    id: "runtime-1",
    workspace_id: "ws-1",
    daemon_id: "daemon-1",
    name: "Runtime",
    runtime_mode: "local",
    provider: "codex",
    launch_header: "codex",
    status: "online",
    device_info: "device",
    metadata: {},
    owner_id: "user-1",
    visibility: "private",
    profile_id: null,
    last_seen_at: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("filterOwnedRuntimes", () => {
  it("only keeps runtimes owned by the current user", () => {
    const runtimes = [
      makeRuntime({ id: "mine", owner_id: "user-1" }),
      makeRuntime({ id: "other", owner_id: "user-2", visibility: "public" }),
      makeRuntime({ id: "unowned", owner_id: null }),
    ];

    expect(filterOwnedRuntimes(runtimes, "user-1").map((r) => r.id)).toEqual([
      "mine",
    ]);
  });

  it("returns no runtimes before the current user is known", () => {
    expect(filterOwnedRuntimes([makeRuntime()], null)).toEqual([]);
  });
});

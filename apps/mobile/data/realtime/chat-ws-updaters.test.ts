import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { chatKeys } from "../queries/chat";
import { appendTaskMessage } from "./chat-ws-updaters";

vi.mock("@/data/api", () => ({ api: {} }));

describe("appendTaskMessage", () => {
  it("redacts runtime invocation fields before caching live task messages", () => {
    const qc = new QueryClient();

    appendTaskMessage(qc, {
      task_id: "task-1",
      issue_id: "issue-1",
      seq: 1,
      type: "tool_use",
      input: {
        path: "README.md",
        runtime_id: "runtime-private",
        runtimeId: "runtime-private",
        daemon_operation_params: { runtime_id: "runtime-private" },
        nested: {
          keep: "safe",
          runtimeId: "runtime-private",
        },
      },
    });

    const messages =
      qc.getQueryData<unknown[]>(chatKeys.taskMessages("task-1")) ?? [];
    const input =
      (messages[0] as { input?: Record<string, unknown> }).input ?? {};
    expect(input.path).toBe("README.md");
    expect("runtime_id" in input).toBe(false);
    expect("runtimeId" in input).toBe(false);
    expect("daemon_operation_params" in input).toBe(false);
    const nested = input.nested as Record<string, unknown>;
    expect(nested.keep).toBe("safe");
    expect("runtimeId" in nested).toBe(false);
  });
});

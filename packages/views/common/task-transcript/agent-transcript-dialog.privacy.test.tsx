// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import { render, waitFor } from "@testing-library/react";
import { I18nProvider } from "@multica/core/i18n/react";
import type { AgentTask } from "@multica/core/types/agent";
import enCommon from "../../locales/en/common.json";
import enAgents from "../../locales/en/agents.json";
import { AgentTranscriptDialog } from "./agent-transcript-dialog";

const mockGetAgent = vi.hoisted(() => vi.fn());
const mockListRuntimes = vi.hoisted(() => vi.fn());

vi.mock("@multica/core/api", () => ({
  api: {
    getAgent: (...args: unknown[]) => mockGetAgent(...args),
    listRuntimes: (...args: unknown[]) => mockListRuntimes(...args),
  },
}));

vi.mock("../actor-avatar", () => ({
  ActorAvatar: () => null,
}));

function makeTask(overrides: Partial<AgentTask> = {}): AgentTask {
  return {
    id: "task-1",
    agent_id: "agent-1",
    issue_id: "issue-1",
    status: "completed",
    priority: 0,
    dispatched_at: null,
    started_at: null,
    completed_at: "2026-01-01T00:00:01Z",
    result: null,
    error: null,
    created_at: "2026-01-01T00:00:00Z",
    runtime_id: "runtime-private",
    attempt: 1,
    ...overrides,
  };
}

describe("AgentTranscriptDialog runtime privacy", () => {
  it("does not fetch runtime details from audit task runtime_id", async () => {
    mockGetAgent.mockResolvedValue({ name: "Agent" });
    mockListRuntimes.mockResolvedValue([]);

    render(
      <I18nProvider
        locale="en"
        resources={{ en: { common: enCommon, agents: enAgents } }}
      >
        <AgentTranscriptDialog
          open
          onOpenChange={() => {}}
          task={makeTask()}
          items={[]}
          agentName="Agent"
        />
      </I18nProvider>,
    );

    await waitFor(() => expect(mockGetAgent).toHaveBeenCalledWith("agent-1"));
    expect(mockListRuntimes).not.toHaveBeenCalled();
  });
});

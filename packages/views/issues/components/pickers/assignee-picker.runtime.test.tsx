// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "@multica/core/i18n/react";
import type { Agent, AgentRuntime, MemberWithUser, Squad } from "@multica/core/types";
import { beforeEach, describe, expect, it, vi } from "vitest";
import enCommon from "../../../locales/en/common.json";
import enIssues from "../../../locales/en/issues.json";
import { AssigneePicker } from "./assignee-picker";

const TEST_RESOURCES = { en: { common: enCommon, issues: enIssues } };

const members: MemberWithUser[] = [
  {
    id: "member-1",
    user_id: "user-1",
    workspace_id: "ws-1",
    role: "member",
    name: "Current User",
    email: "current@example.com",
    avatar_url: null,
    created_at: "2026-01-01T00:00:00Z",
  },
];

const agents: Agent[] = [
  {
    id: "agent-1",
    workspace_id: "ws-1",
    runtime_id: null,
    runtime_provider: "codex",
    runtime_profile_id: null,
    name: "Codex Agent",
    description: "",
    instructions: "",
    avatar_url: null,
    runtime_mode: "local",
    runtime_config: {},
    custom_args: [],
    visibility: "workspace",
    status: "idle",
    max_concurrent_tasks: 1,
    model: "",
    owner_id: "user-1",
    skills: [],
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    archived_at: null,
    archived_by: null,
  },
];

const runtimes: AgentRuntime[] = [
  {
    id: "rt-1",
    workspace_id: "ws-1",
    daemon_id: null,
    name: "Laptop",
    runtime_mode: "local",
    provider: "codex",
    launch_header: "",
    status: "online",
    device_info: "",
    metadata: {},
    owner_id: "user-1",
    visibility: "private",
    profile_id: null,
    last_seen_at: "2026-01-01T00:00:00Z",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "rt-2",
    workspace_id: "ws-1",
    daemon_id: null,
    name: "Desktop",
    runtime_mode: "local",
    provider: "codex",
    launch_header: "",
    status: "online",
    device_info: "",
    metadata: {},
    owner_id: "user-1",
    visibility: "private",
    profile_id: null,
    last_seen_at: "2026-01-01T00:00:00Z",
    created_at: "2026-01-02T00:00:00Z",
    updated_at: "2026-01-02T00:00:00Z",
  },
  {
    id: "rt-other",
    workspace_id: "ws-1",
    daemon_id: null,
    name: "Teammate Desktop",
    runtime_mode: "local",
    provider: "codex",
    launch_header: "",
    status: "online",
    device_info: "",
    metadata: {},
    owner_id: "user-2",
    visibility: "public",
    profile_id: null,
    last_seen_at: "2026-01-03T00:00:00Z",
    created_at: "2026-01-03T00:00:00Z",
    updated_at: "2026-01-03T00:00:00Z",
  },
];

let squadQueryData: Squad[] = [];
let squadRuntimeQueryData: Array<{ id: string; name: string }> = [];

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

vi.mock("@multica/core/auth", () => ({
  useAuthStore: Object.assign(
    (selector?: any) => {
      const state = { user: { id: "user-1" } };
      return selector ? selector(state) : state;
    },
    { getState: () => ({ user: { id: "user-1" } }) },
  ),
}));

vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({
    queryKey: ["members"],
    queryFn: () => Promise.resolve(members),
  }),
  agentListOptions: () => ({
    queryKey: ["agents"],
    queryFn: () => Promise.resolve(agents),
  }),
  squadListOptions: () => ({
    queryKey: ["squads"],
    queryFn: () => Promise.resolve(squadQueryData),
  }),
  assigneeFrequencyOptions: () => ({
    queryKey: ["assignee-frequency"],
    queryFn: () => Promise.resolve([]),
  }),
}));

vi.mock("@multica/core/runtimes/queries", () => ({
  runtimeListOptions: () => ({
    queryKey: ["runtimes"],
    queryFn: () => Promise.resolve(runtimes),
  }),
}));

vi.mock("@multica/core/squads", () => ({
  squadLeaderRuntimeOptions: (_wsId: string, squadId: string) => ({
    queryKey: ["squads", squadId, "leader-compatible-runtimes"],
    queryFn: () => Promise.resolve(squadRuntimeQueryData),
  }),
}));

vi.mock("@multica/core/workspace/hooks", () => ({
  useActorName: () => ({
    getActorName: (type: string, id: string) =>
      type === "agent" && id === "agent-1"
        ? "Codex Agent"
        : type === "squad" && id === "squad-1"
          ? "Frontend Squad"
          : "Unknown",
  }),
}));

vi.mock("../../../common/actor-avatar", () => ({
  ActorAvatar: () => null,
}));

function renderPicker(onUpdate = vi.fn()) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <QueryClientProvider client={qc}>
        <AssigneePicker
          assigneeType={null}
          assigneeId={null}
          onUpdate={onUpdate}
          trigger="Assignee"
        />
      </QueryClientProvider>
    </I18nProvider>,
  );
  return onUpdate;
}

describe("AssigneePicker runtime selection", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    squadQueryData = [];
    squadRuntimeQueryData = [];
  });

  it("lets users choose among multiple compatible owned runtimes when assigning an agent", async () => {
    const onUpdate = renderPicker();

    fireEvent.click(screen.getByText("Assignee"));
    fireEvent.click(await screen.findByText("Codex Agent"));

    expect(onUpdate).not.toHaveBeenCalled();
    expect(await screen.findByText("Laptop")).toBeInTheDocument();
    expect(screen.getByText("Desktop")).toBeInTheDocument();
    expect(screen.queryByText("Teammate Desktop")).not.toBeInTheDocument();

    fireEvent.click(screen.getByText("Desktop"));

    expect(onUpdate).toHaveBeenCalledWith({
      assignee_type: "agent",
      assignee_id: "agent-1",
      runtime_id: "rt-2",
    });
  });

  it("lets users choose among current-user squad leader compatible runtimes when assigning a squad", async () => {
    squadQueryData = [
      {
        id: "squad-1",
        workspace_id: "ws-1",
        name: "Frontend Squad",
        description: "",
        instructions: "",
        avatar_url: null,
        leader_id: "agent-1",
        creator_id: "user-1",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
        archived_at: null,
        archived_by: null,
      },
    ];
    squadRuntimeQueryData = [
      { id: "rt-1", name: "Laptop" },
      { id: "rt-2", name: "Desktop" },
    ];
    const onUpdate = renderPicker();

    fireEvent.click(screen.getByText("Assignee"));
    fireEvent.click(await screen.findByText("Frontend Squad"));

    expect(onUpdate).not.toHaveBeenCalled();
    expect(await screen.findByText("Laptop")).toBeInTheDocument();
    expect(screen.getByText("Desktop")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Desktop"));

    expect(onUpdate).toHaveBeenCalledWith({
      assignee_type: "squad",
      assignee_id: "squad-1",
      runtime_id: "rt-2",
    });
  });

  it("auto-selects the only current-user compatible runtime when assigning a squad", async () => {
    squadQueryData = [
      {
        id: "squad-1",
        workspace_id: "ws-1",
        name: "Frontend Squad",
        description: "",
        instructions: "",
        avatar_url: null,
        leader_id: "agent-1",
        creator_id: "user-1",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
        archived_at: null,
        archived_by: null,
      },
    ];
    squadRuntimeQueryData = [{ id: "rt-1", name: "Laptop" }];
    const onUpdate = renderPicker();

    fireEvent.click(screen.getByText("Assignee"));
    fireEvent.click(await screen.findByText("Frontend Squad"));

    await waitFor(() => {
      expect(onUpdate).toHaveBeenCalledWith({
        assignee_type: "squad",
        assignee_id: "squad-1",
        runtime_id: "rt-1",
      });
    });
  });
});

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import type { DaemonStatus } from "../../../shared/daemon-types";

const mockData = vi.hoisted(() => ({
  runtimes: [] as any[],
  agents: [] as any[],
  snapshot: [] as any[],
}));

// The component only needs these to render; stub them so the test focuses on
// the externally-managed branching, not data fetching.
vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: { queryKey?: readonly unknown[] }) => {
    const key = options.queryKey?.[0];
    if (key === "runtimes") return { data: mockData.runtimes };
    if (key === "agents") return { data: mockData.agents };
    if (key === "snapshot") return { data: mockData.snapshot };
    return { data: [] };
  },
}));
vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));
vi.mock("@multica/core/runtimes", () => ({
  runtimeListOptions: () => ({ queryKey: ["runtimes"] }),
}));
vi.mock("@multica/core/workspace/queries", () => ({
  agentListOptions: () => ({ queryKey: ["agents"] }),
}));
vi.mock("@multica/core/agents", () => ({
  agentTaskSnapshotOptions: () => ({ queryKey: ["snapshot"] }),
  runtimeForAgentCapability: (
    agent: { runtime_provider?: string; runtime_profile_id?: string | null },
    runtimes: Array<{ provider: string; profile_id?: string | null }>,
  ) =>
    runtimes.find((runtime) =>
      agent.runtime_profile_id
        ? runtime.profile_id === agent.runtime_profile_id
        : runtime.provider === agent.runtime_provider && !runtime.profile_id,
    ) ?? null,
}));
vi.mock("./daemon-panel", () => ({ DaemonPanel: () => null }));
vi.mock("../platform/daemon-reauth", () => ({
  reauthenticateDaemon: vi.fn(),
}));
vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

import { DaemonRuntimeActions } from "./daemon-runtime-card";

beforeEach(() => {
  mockData.runtimes = [];
  mockData.agents = [];
  mockData.snapshot = [];
});

function stubDaemonAPI(status: DaemonStatus) {
  Object.defineProperty(window, "daemonAPI", {
    configurable: true,
    value: {
      getStatus: vi.fn().mockResolvedValue(status),
      onStatusChange: vi.fn(() => () => {}),
      stop: vi.fn().mockResolvedValue({ success: true }),
    },
  });
}

describe("DaemonRuntimeActions — externally managed daemon (#3916)", () => {
  it("hides Stop/Restart and shows the managed-outside hint for a daemon the app can't control", async () => {
    stubDaemonAPI({ state: "running", daemonId: "d1", externallyManaged: true });
    render(<DaemonRuntimeActions />);

    // View logs still renders, confirming the running branch mounted.
    expect(await screen.findByText("View logs")).toBeInTheDocument();
    expect(screen.getByText("Managed outside the app")).toBeInTheDocument();
    expect(screen.queryByText("Restart")).not.toBeInTheDocument();
    expect(screen.queryByText("Stop")).not.toBeInTheDocument();
  });

  it("shows Stop/Restart for a normally-managed running daemon (no 误伤)", async () => {
    stubDaemonAPI({
      state: "running",
      daemonId: "d1",
      externallyManaged: false,
    });
    render(<DaemonRuntimeActions />);

    expect(await screen.findByText("Restart")).toBeInTheDocument();
    expect(screen.getByText("Stop")).toBeInTheDocument();
    expect(
      screen.queryByText("Managed outside the app"),
    ).not.toBeInTheDocument();
  });

  it("derives affected tasks without relying on audit task runtime_id", async () => {
    mockData.runtimes = [
      {
        id: "rt-owned",
        daemon_id: "d1",
        runtime_mode: "local",
        provider: "codex",
        profile_id: null,
      },
    ];
    mockData.agents = [
      {
        id: "agent-1",
        runtime_provider: "codex",
        runtime_profile_id: null,
      },
    ];
    mockData.snapshot = [
      {
        id: "task-1",
        agent_id: "agent-1",
        runtime_id: "",
        status: "running",
      },
    ];
    stubDaemonAPI({
      state: "running",
      daemonId: "d1",
      externallyManaged: false,
    });

    render(<DaemonRuntimeActions />);

    fireEvent.click(await screen.findByText("Stop"));

    expect(
      await screen.findByText("Stop daemon with 1 active task?"),
    ).toBeInTheDocument();
  });
});

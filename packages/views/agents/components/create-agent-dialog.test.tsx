// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import type { Agent, MemberWithUser, RuntimeDevice } from "@multica/core/types";
import { I18nProvider } from "@multica/core/i18n/react";
import { WorkspaceSlugProvider } from "@multica/core/paths";
import { NavigationProvider, type NavigationAdapter } from "../../navigation";
import enCommon from "../../locales/en/common.json";
import enAgents from "../../locales/en/agents.json";

const navigationStub: NavigationAdapter = {
  push: vi.fn(),
  replace: vi.fn(),
  back: vi.fn(),
  pathname: "/",
  searchParams: new URLSearchParams(),
  getShareableUrl: (path: string) => path,
};

const TEST_RESOURCES = { en: { common: enCommon, agents: enAgents } };

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

// ModelDropdown talks to the api; the create dialog only needs it as a
// stand-in here, so swap it out.
vi.mock("./model-dropdown", () => ({
  ModelDropdown: () => null,
}));

// Provider logos don't matter for these assertions but they pull in SVGs.
vi.mock("../../runtimes/components/provider-logo", () => ({
  ProviderLogo: () => null,
}));

// Avatars hit the api for member metadata.
vi.mock("../../common/actor-avatar", () => ({
  ActorAvatar: () => null,
}));

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

import { CreateAgentDialog } from "./create-agent-dialog";

const ME = "user-me";
const OTHER = "user-other";

const members: MemberWithUser[] = [
  {
    id: "m-me",
    user_id: ME,
    workspace_id: "ws-1",
    role: "member",
    name: "Me",
    email: "me@example.com",
    avatar_url: null,
    created_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "m-other",
    user_id: OTHER,
    workspace_id: "ws-1",
    role: "member",
    name: "Other",
    email: "other@example.com",
    avatar_url: null,
    created_at: "2026-01-01T00:00:00Z",
  },
];

function makeRuntime(overrides: Partial<RuntimeDevice>): RuntimeDevice {
  return {
    id: "rt",
    workspace_id: "ws-1",
    daemon_id: null,
    name: "Test Runtime",
    runtime_mode: "local",
    provider: "claude",
    launch_header: "",
    status: "online",
    device_info: "host.local",
    metadata: {},
    owner_id: ME,
    visibility: "private",
    last_seen_at: "2026-04-27T11:59:50Z",
    created_at: "2026-04-01T00:00:00Z",
    updated_at: "2026-04-01T00:00:00Z",
    ...overrides,
  };
}

function makeTemplate(overrides: Partial<Agent> = {}): Agent {
  return {
    id: "agent-template",
    workspace_id: "ws-1",
    runtime_id: null,
    runtime_provider: "claude",
    runtime_profile_id: null,
    name: "Template Agent",
    description: "",
    instructions: "",
    avatar_url: null,
    runtime_mode: "local",
    runtime_config: {},
    custom_args: [],
    visibility: "private",
    status: "idle",
    max_concurrent_tasks: 1,
    model: "",
    owner_id: ME,
    skills: [],
    created_at: "2026-04-01T00:00:00Z",
    updated_at: "2026-04-01T00:00:00Z",
    archived_at: null,
    archived_by: null,
    ...overrides,
  };
}

function renderDialog(runtimes: RuntimeDevice[], template?: Agent) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const onCreate = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  render(
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <QueryClientProvider client={queryClient}>
        <WorkspaceSlugProvider slug="test-ws">
        <NavigationProvider value={navigationStub}>
          <CreateAgentDialog
            runtimes={runtimes}
            members={members}
            currentUserId={ME}
            template={template}
            onClose={onClose}
            onCreate={onCreate}
          />
        </NavigationProvider>
        </WorkspaceSlugProvider>
      </QueryClientProvider>
    </I18nProvider>,
  );
  return { onCreate, onClose };
}

describe("CreateAgentDialog runtime capability selection", () => {
  beforeEach(() => vi.clearAllMocks());
  // Base UI Dialog renders into a portal on document.body and leaves
  // focus-guard / inert wrapper divs around after the React tree unmounts.
  // The auto-cleanup from @testing-library/react drops the container but
  // not the portal residue, so two-tests-in-a-row queries see double
  // matches ("All", "My Runtime"). Force cleanup + wipe body between tests.
  afterEach(() => {
    cleanup();
    document.body.innerHTML = "";
  });

  it("submits runtime capability instead of binding to a concrete runtime", async () => {
    const mine = makeRuntime({ id: "rt-mine", name: "My Runtime", owner_id: ME, provider: "claude" });
    const { onCreate } = renderDialog([mine], makeTemplate());

    fireEvent.click(screen.getByText("Create"));
    await new Promise((r) => setTimeout(r, 0));

    expect(onCreate).toHaveBeenCalledTimes(1);
    expect(onCreate.mock.calls[0]?.[0].runtime_provider).toBe("claude");
    expect(onCreate.mock.calls[0]?.[0].runtime_id).toBeUndefined();
  });

  it("does not let another member's public runtime choose the capability", async () => {
    const othersPublic = makeRuntime({
      id: "rt-others-public",
      name: "Others Public",
      owner_id: OTHER,
      visibility: "public",
      provider: "gemini",
    });
    const mine = makeRuntime({
      id: "rt-mine",
      name: "My Runtime",
      owner_id: ME,
      provider: "codex",
    });
    const { onCreate } = renderDialog([othersPublic, mine], makeTemplate({ runtime_provider: "codex" }));

    fireEvent.click(screen.getByText("Create"));
    await new Promise((r) => setTimeout(r, 0));

    expect(onCreate.mock.calls[0]?.[0].runtime_provider).toBe("codex");
    expect(onCreate.mock.calls[0]?.[0].runtime_id).toBeUndefined();
  });

  it("keeps a duplicated custom profile as a capability", async () => {
    const template = makeTemplate({
      runtime_provider: "claude",
      runtime_profile_id: "profile-1",
    });
    const { onCreate } = renderDialog([], template);

    fireEvent.click(screen.getByText("Create"));
    await new Promise((r) => setTimeout(r, 0));

    expect(onCreate.mock.calls[0]?.[0].runtime_provider).toBe("claude");
    expect(onCreate.mock.calls[0]?.[0].runtime_profile_id).toBe("profile-1");
    expect(onCreate.mock.calls[0]?.[0].runtime_id).toBeUndefined();
  });
});

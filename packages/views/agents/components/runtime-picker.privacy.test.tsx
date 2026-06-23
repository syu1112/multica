// @vitest-environment jsdom

import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { I18nProvider } from "@multica/core/i18n/react";
import type {
  AgentRuntime,
  MemberWithUser,
  RuntimeDevice,
} from "@multica/core/types";
import enAgents from "../../locales/en/agents.json";
import enCommon from "../../locales/en/common.json";
import { RuntimePicker as CreateRuntimePicker } from "./runtime-picker";
import { RuntimePicker as InspectorRuntimePicker } from "./inspector/runtime-picker";

vi.mock("../../runtimes/components/provider-logo", () => ({
  ProviderLogo: () => null,
}));

vi.mock("../../common/actor-avatar", () => ({
  ActorAvatar: () => null,
}));

const TEST_RESOURCES = { en: { common: enCommon, agents: enAgents } };

const ME = "user-me";
const OTHER = "user-other";

const members: MemberWithUser[] = [
  {
    id: "member-me",
    user_id: ME,
    workspace_id: "workspace-1",
    role: "member",
    name: "Me",
    email: "me@example.com",
    avatar_url: null,
    created_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "member-other",
    user_id: OTHER,
    workspace_id: "workspace-1",
    role: "member",
    name: "Other",
    email: "other@example.com",
    avatar_url: null,
    created_at: "2026-01-01T00:00:00Z",
  },
];

function makeRuntime(overrides: Partial<RuntimeDevice> = {}): RuntimeDevice {
  return {
    id: "runtime-1",
    workspace_id: "workspace-1",
    daemon_id: null,
    name: "My Runtime",
    runtime_mode: "local",
    provider: "codex",
    launch_header: "",
    status: "online",
    device_info: "host.local",
    metadata: {},
    owner_id: ME,
    visibility: "private",
    profile_id: null,
    last_seen_at: "2026-06-20T00:00:00Z",
    created_at: "2026-06-20T00:00:00Z",
    updated_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}

function renderWithI18n(node: ReactNode) {
  return render(
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      {node}
    </I18nProvider>,
  );
}

describe("RuntimePicker privacy", () => {
  afterEach(() => {
    cleanup();
    document.body.innerHTML = "";
  });

  it("does not display a non-owned selected runtime in the create picker trigger", () => {
    const otherRuntime = makeRuntime({
      id: "runtime-other",
      name: "Other Runtime",
      owner_id: OTHER,
    });

    renderWithI18n(
      <CreateRuntimePicker
        runtimes={[otherRuntime]}
        members={members}
        currentUserId={ME}
        selectedRuntimeId="runtime-other"
        onSelect={vi.fn()}
      />,
    );

    expect(screen.queryByText("Other Runtime")).not.toBeInTheDocument();
    expect(screen.getByText("No runtime available")).toBeInTheDocument();
  });

  it("does not display a non-owned selected runtime in the inspector read-only chip", () => {
    const otherRuntime = makeRuntime({
      id: "runtime-other",
      name: "Other Runtime",
      owner_id: OTHER,
    }) as AgentRuntime;

    renderWithI18n(
      <InspectorRuntimePicker
        value="runtime-other"
        runtimes={[otherRuntime]}
        members={members}
        currentUserId={ME}
        canEdit={false}
        onChange={vi.fn()}
      />,
    );

    expect(screen.queryByText("Other Runtime")).not.toBeInTheDocument();
    expect(screen.getByText("No runtime")).toBeInTheDocument();
  });
});

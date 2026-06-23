import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "@multica/core/i18n/react";
import type { SupportedLocale } from "@multica/core/i18n";
import type { AgentRuntime, CreateAgentRequest } from "@multica/core/types";
import enOnboarding from "../../locales/en/onboarding.json";
import enCommon from "../../locales/en/common.json";
import { StepAgent } from "./step-agent";

const TEST_RESOURCES = {
  en: { common: enCommon, onboarding: enOnboarding },
};

const mockCreateAgent = vi.fn();

vi.mock("@multica/core/api", () => ({
  api: {
    createAgent: (...args: unknown[]) => mockCreateAgent(...args),
  },
}));

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
  },
}));

vi.mock("@multica/views/platform", () => ({
  DragStrip: () => null,
}));

function I18nWrapper({
  children,
  locale = "en",
}: {
  children: ReactNode;
  locale?: SupportedLocale;
}) {
  return (
    <I18nProvider locale={locale} resources={TEST_RESOURCES}>
      {children}
    </I18nProvider>
  );
}

const runtime: AgentRuntime = {
  id: "rt-1",
  workspace_id: "ws-1",
  daemon_id: "daemon-1",
  name: "Local Codex",
  runtime_mode: "local",
  provider: "codex",
  profile_id: "profile-1",
  launch_header: "",
  status: "online",
  device_info: "",
  metadata: {},
  owner_id: "user-1",
  visibility: "private",
  last_seen_at: null,
  created_at: "",
  updated_at: "",
};

describe("StepAgent", () => {
  beforeEach(() => {
    mockCreateAgent.mockReset();
    mockCreateAgent.mockResolvedValue({
      id: "agent-1",
      name: "Atlas",
      description: "",
      avatar_url: null,
      visibility: "workspace",
    });
  });

  it("creates an agent with runtime capability instead of binding a concrete runtime", async () => {
    const onCreated = vi.fn();

    render(
      <StepAgent
        runtime={runtime}
        questionnaire={{
          source: [],
          source_other: null,
          source_skipped: false,
          role: "engineer",
          role_other: null,
          role_skipped: false,
          use_case: ["ship_code"],
          use_case_other: null,
          use_case_skipped: false,
          version: 2,
        }}
        onCreated={onCreated}
      />,
      { wrapper: I18nWrapper },
    );

    fireEvent.click(screen.getByRole("button", { name: /create atlas/i }));

    await waitFor(() => expect(mockCreateAgent).toHaveBeenCalledTimes(1));
    const [payload] = mockCreateAgent.mock.calls[0]! as [CreateAgentRequest];
    expect(payload.runtime_id).toBeUndefined();
    expect(payload.runtime_provider).toBe("codex");
    expect(payload.runtime_profile_id).toBe("profile-1");
    expect(payload.name).toBe("Atlas");
    await waitFor(() => expect(onCreated).toHaveBeenCalledTimes(1));
  });
});

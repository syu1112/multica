import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import { configStore } from "@multica/core/config";
import { useWSEvent } from "@multica/core/realtime";
import enCommon from "../../locales/en/common.json";
import enRuntimes from "../../locales/en/runtimes.json";
import { ConnectRemoteDialog } from "./connect-remote-dialog";

const TEST_RESOURCES = { en: { common: enCommon, runtimes: enRuntimes } };

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-test",
}));

vi.mock("@multica/core/paths", () => ({
  paths: {
    workspace: () => ({
      agents: () => "/agents",
      runtimeDetail: () => "/runtimes/rt-test",
    }),
  },
  useWorkspaceSlug: () => "workspace-test",
}));

vi.mock("@multica/core/realtime", () => ({
  useWSEvent: vi.fn(),
}));

vi.mock("../../navigation", () => ({
  useNavigation: () => ({ push: vi.fn() }),
}));

function resetConfigStore() {
  configStore.setState({
    cdnDomain: "",
    allowSignup: true,
    googleClientId: "",
    daemonServerUrl: "",
    daemonAppUrl: "",
    workspaceCreationDisabled: false,
  });
}

function renderDialog(config?: {
  daemonServerUrl?: string;
  daemonAppUrl?: string;
}) {
  resetConfigStore();
  vi.mocked(useWSEvent).mockClear();
  if (config) {
    configStore.getState().setDaemonConfig(config);
  }
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <I18nProvider locale="en" resources={TEST_RESOURCES}>
        <ConnectRemoteDialog onClose={vi.fn()} />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

const ligatureClasses = [
  "[font-variant-ligatures:none]",
  "[font-feature-settings:'liga'_0]",
];

describe("ConnectRemoteDialog", () => {
  it("uses cloud setup commands by default", () => {
    const { baseElement } = renderDialog();

    expect(baseElement).toHaveTextContent("multica setup");
    expect(baseElement).not.toHaveTextContent("multica setup self-host");
    expect(baseElement).toHaveTextContent(
      "multica config set server_url https://api.multica.ai",
    );
    expect(baseElement).toHaveTextContent(
      "multica config set app_url https://multica.ai",
    );
  });

  it("uses self-host daemon URLs from runtime config", () => {
    const { baseElement } = renderDialog({
      daemonServerUrl: "https://api.example.com/",
      daemonAppUrl: "https://app.example.com/",
    });

    expect(baseElement).toHaveTextContent(
      "multica setup self-host --server-url https://api.example.com --app-url https://app.example.com",
    );
    expect(baseElement).toHaveTextContent(
      "multica config set server_url https://api.example.com",
    );
    expect(baseElement).toHaveTextContent(
      "multica config set app_url https://app.example.com",
    );
  });

  it("disables font ligatures in setup command code", () => {
    const { baseElement } = renderDialog();

    const setupCode = Array.from(baseElement.querySelectorAll("code")).find((node) =>
      node.textContent?.includes("multica setup"),
    );

    expect(setupCode).toHaveClass(...ligatureClasses);
  });

  it("disables font ligatures in fallback token command code", () => {
    const { baseElement } = renderDialog();

    const tokenCode = Array.from(baseElement.querySelectorAll("code")).find((node) =>
      node.textContent?.includes("multica login --token <YOUR_TOKEN>"),
    );

    expect(tokenCode).toHaveClass(...ligatureClasses);
  });

  it("does not require or expose a runtime_id from daemon register events", () => {
    renderDialog();

    const callback = vi.mocked(useWSEvent).mock.calls.find(
      ([eventName]) => eventName === "daemon:register",
    )?.[1];
    expect(callback).toBeTypeOf("function");

    act(() => {
      callback?.({ action: "register" });
    });

    expect(screen.getByText("Computer connected")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "View runtime" })).toBeNull();
    expect(
      screen.getByRole("button", { name: /create an agent/i }),
    ).toBeInTheDocument();
  });
});

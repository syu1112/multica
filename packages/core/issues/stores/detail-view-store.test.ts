// @vitest-environment jsdom
import { afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { setCurrentWorkspace } from "../../platform/workspace-storage";
import { useIssueDetailViewStore } from "./detail-view-store";

const flush = () => new Promise((resolve) => queueMicrotask(() => resolve(null)));

beforeAll(() => {
  if (typeof globalThis.localStorage?.clear !== "function") {
    const values = new Map<string, string>();
    const storage: Storage = {
      get length() { return values.size; },
      clear: () => values.clear(),
      getItem: (key) => values.get(key) ?? null,
      key: (index) => Array.from(values.keys())[index] ?? null,
      removeItem: (key) => { values.delete(key); },
      setItem: (key, value) => { values.set(key, value); },
    };
    Object.defineProperty(globalThis, "localStorage", { configurable: true, value: storage });
    Object.defineProperty(window, "localStorage", { configurable: true, value: storage });
  }
});

describe("issue detail view store", () => {
  beforeEach(() => {
    localStorage.clear();
    setCurrentWorkspace(null, null);
    useIssueDetailViewStore.setState({ contentWidth: "default" });
  });

  afterEach(async () => {
    setCurrentWorkspace(null, null);
    await flush();
  });

  it("defaults to the readable document width and toggles both ways", () => {
    expect(useIssueDetailViewStore.getState().contentWidth).toBe("default");

    useIssueDetailViewStore.getState().toggleContentWidth();
    expect(useIssueDetailViewStore.getState().contentWidth).toBe("wide");

    useIssueDetailViewStore.getState().toggleContentWidth();
    expect(useIssueDetailViewStore.getState().contentWidth).toBe("default");
  });

  it("supports explicitly selecting a width", () => {
    useIssueDetailViewStore.getState().setContentWidth("wide");
    expect(useIssueDetailViewStore.getState().contentWidth).toBe("wide");

    useIssueDetailViewStore.getState().setContentWidth("default");
    expect(useIssueDetailViewStore.getState().contentWidth).toBe("default");
  });

  it("persists only the selected width in the current workspace namespace", async () => {
    setCurrentWorkspace("acme", "ws-acme");
    await flush();
    await flush();

    useIssueDetailViewStore.getState().setContentWidth("wide");

    const stored = JSON.parse(
      localStorage.getItem("multica_issue_detail_view:acme") ?? "null",
    );
    expect(stored.state).toEqual({ contentWidth: "wide" });
  });

  it("rehydrates each workspace independently and defaults an unseen workspace", async () => {
    setCurrentWorkspace("team-a", "ws-a");
    await flush();
    await flush();
    useIssueDetailViewStore.getState().setContentWidth("wide");

    setCurrentWorkspace("team-b", "ws-b");
    await flush();
    await flush();
    expect(useIssueDetailViewStore.getState().contentWidth).toBe("default");

    setCurrentWorkspace("team-a", "ws-a");
    await flush();
    await flush();
    expect(useIssueDetailViewStore.getState().contentWidth).toBe("wide");
  });
});

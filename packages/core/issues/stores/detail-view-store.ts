"use client";

import { create } from "zustand";
import {
  createJSONStorage,
  persist,
  type StateStorage,
} from "zustand/middleware";
import {
  createWorkspaceAwareStorage,
  registerForWorkspaceRehydration,
} from "../../platform/workspace-storage";
import { defaultStorage } from "../../platform/storage";

export type IssueDetailContentWidth = "default" | "wide";

export interface IssueDetailViewState {
  contentWidth: IssueDetailContentWidth;
  setContentWidth: (width: IssueDetailContentWidth) => void;
  toggleContentWidth: () => void;
}

const DEFAULT_PERSISTED_STATE = JSON.stringify({
  state: { contentWidth: "default" },
  version: 0,
});

/**
 * Return an explicit default snapshot for a workspace with no saved value.
 * Zustand otherwise keeps the previous in-memory state when rehydrating from
 * a missing key, which would leak one workspace's width into another.
 */
function createIssueDetailViewStorage(): StateStorage {
  const workspaceStorage = createWorkspaceAwareStorage(defaultStorage);
  return {
    getItem: (key) =>
      workspaceStorage.getItem(key) ?? DEFAULT_PERSISTED_STATE,
    setItem: workspaceStorage.setItem,
    removeItem: workspaceStorage.removeItem,
  };
}

export const useIssueDetailViewStore = create<IssueDetailViewState>()(
  persist(
    (set) => ({
      contentWidth: "default",
      setContentWidth: (contentWidth) => set({ contentWidth }),
      toggleContentWidth: () =>
        set((state) => ({
          contentWidth: state.contentWidth === "default" ? "wide" : "default",
        })),
    }),
    {
      name: "multica_issue_detail_view",
      storage: createJSONStorage(createIssueDetailViewStorage),
      partialize: (state) => ({ contentWidth: state.contentWidth }),
    },
  ),
);

registerForWorkspaceRehydration(() =>
  useIssueDetailViewStore.persist.rehydrate(),
);

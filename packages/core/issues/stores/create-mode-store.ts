"use client";

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { defaultStorage } from "../../platform/storage";
import { useModalStore } from "../../modals";

/**
 * Create-issue mode preference retained for older persisted clients. New
 * generic issue creation always opens manual mode; agent-assisted creation
 * is no longer exposed from create-issue entry points.
 */
export type CreateMode = "agent" | "manual";

interface CreateModeState {
  lastMode: CreateMode;
  setLastMode: (mode: CreateMode) => void;
}

export const useCreateModeStore = create<CreateModeState>()(
  persist(
    (set) => ({
      lastMode: "manual",
      setLastMode: (mode) => set({ lastMode: mode }),
    }),
    {
      name: "multica_create_mode",
      storage: createJSONStorage(() => defaultStorage),
    },
  ),
);

/**
 * Open the manual create-issue flow. Ignore older persisted agent-mode
 * preferences so shared entry points cannot launch agent creation.
 */
export function openCreateIssueWithPreference(
  data?: Record<string, unknown> | null,
) {
  useModalStore.getState().open("create-issue", data ?? null);
}

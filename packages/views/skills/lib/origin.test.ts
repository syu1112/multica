import { describe, expect, it } from "vitest";
import type { SkillSummary } from "@multica/core/types";
import { readOrigin } from "./origin";

const baseSkill: SkillSummary = {
  id: "skill-1",
  workspace_id: "ws-1",
  name: "Review Helper",
  description: "",
  config: {},
  created_by: "user-1",
  created_at: "2026-06-20T00:00:00Z",
  updated_at: "2026-06-20T00:00:00Z",
};

describe("readOrigin", () => {
  it("redacts runtime ids from local runtime origins", () => {
    const origin = readOrigin({
      ...baseSkill,
      config: {
        origin: {
          type: "runtime_local",
          runtime_id: "private-runtime-id",
          provider: "codex",
          source_path: "~/.codex/skills/review-helper",
        },
      },
    });

    expect(origin).toEqual({
      type: "runtime_local",
      provider: "codex",
      source_path: "~/.codex/skills/review-helper",
    });
    expect("runtime_id" in origin).toBe(false);
  });
});

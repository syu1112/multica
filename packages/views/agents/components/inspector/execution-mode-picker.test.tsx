// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import enAgents from "../../../locales/en/agents.json";
import enIssues from "../../../locales/en/issues.json";
import enCommon from "../../../locales/en/common.json";
import { ExecutionModePicker } from "./execution-mode-picker";

const TEST_RESOURCES = {
  en: { agents: enAgents, issues: enIssues, common: enCommon },
};

function renderPicker(
  props: Partial<React.ComponentProps<typeof ExecutionModePicker>> = {},
) {
  const onChange = vi.fn();
  const utils = render(
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <ExecutionModePicker
        value="normal"
        canEdit
        onChange={onChange}
        {...props}
      />
    </I18nProvider>,
  );
  return { ...utils, onChange };
}

describe("ExecutionModePicker", () => {
  afterEach(() => cleanup());

  it("renders the current mode", () => {
    renderPicker({ value: "normal" });

    expect(screen.getByText("Normal")).toBeTruthy();
  });

  it("emits goal when the Goal option is selected", () => {
    const { onChange } = renderPicker({ value: "normal" });

    fireEvent.click(screen.getByRole("button"));
    fireEvent.click(screen.getByText("Goal"));

    expect(onChange).toHaveBeenCalledWith("goal");
  });

  it("renders read-only text when editing is disabled", () => {
    renderPicker({ value: "goal", canEdit: false });

    expect(screen.getByText("Goal")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });
});

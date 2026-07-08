"use client";

import { useState } from "react";
import { Circle, Target } from "lucide-react";
import type { AgentExecutionMode } from "@multica/core/types";
import {
  PickerItem,
  PropertyPicker,
} from "../../../issues/components/pickers";
import { useT } from "../../../i18n";
import { CHIP_CLASS } from "./chip";

const EXECUTION_MODE_ICON = {
  normal: Circle,
  goal: Target,
} satisfies Record<AgentExecutionMode, typeof Circle>;

export function ExecutionModePicker({
  value,
  canEdit = true,
  onChange,
}: {
  value: AgentExecutionMode;
  canEdit?: boolean;
  onChange: (next: AgentExecutionMode) => Promise<void> | void;
}) {
  const { t } = useT("agents");
  const [open, setOpen] = useState(false);
  const mode = value === "goal" ? "goal" : "normal";
  const label = t(($) => $.pickers.execution_mode[mode].label);
  const tooltip = t(($) => $.pickers.execution_mode_tooltip, { value: label });
  const Icon = EXECUTION_MODE_ICON[mode];

  if (!canEdit) {
    return (
      <span className="flex min-w-0 items-center gap-1.5 text-muted-foreground">
        <Icon className="h-3 w-3 shrink-0" />
        <span className="truncate">{label}</span>
      </span>
    );
  }

  const select = async (next: AgentExecutionMode) => {
    setOpen(false);
    if (next !== mode) await onChange(next);
  };

  return (
    <PropertyPicker
      open={open}
      onOpenChange={setOpen}
      width="w-auto min-w-[13rem]"
      align="start"
      tooltip={tooltip}
      triggerRender={
        <button type="button" className={CHIP_CLASS} aria-label={tooltip} />
      }
      trigger={
        <>
          <Icon className="h-3 w-3 shrink-0 text-muted-foreground" />
          <span className="truncate">{label}</span>
        </>
      }
    >
      <ExecutionModeItem
        mode="normal"
        selected={mode === "normal"}
        onClick={() => select("normal")}
      />
      <ExecutionModeItem
        mode="goal"
        selected={mode === "goal"}
        onClick={() => select("goal")}
      />
    </PropertyPicker>
  );
}

function ExecutionModeItem({
  mode,
  selected,
  onClick,
}: {
  mode: AgentExecutionMode;
  selected: boolean;
  onClick: () => void;
}) {
  const { t } = useT("agents");
  const Icon = EXECUTION_MODE_ICON[mode];

  return (
    <PickerItem selected={selected} onClick={onClick}>
      <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      <div className="text-left">
        <div className="font-medium">
          {t(($) => $.pickers.execution_mode[mode].label)}
        </div>
        <div className="text-xs text-muted-foreground">
          {t(($) => $.pickers.execution_mode[mode].description)}
        </div>
      </div>
    </PickerItem>
  );
}

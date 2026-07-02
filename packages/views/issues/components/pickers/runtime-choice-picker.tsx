"use client";

import { useState } from "react";
import type { AgentRuntime } from "@multica/core/types";
import { PickerItem, PropertyPicker } from "./property-picker";

export function RuntimeChoicePicker({
  runtimes,
  value,
  onChange,
  triggerRender,
  align = "start",
  ariaLabel = "Runtime",
}: {
  runtimes: AgentRuntime[];
  value: string;
  onChange: (runtimeId: string) => void;
  triggerRender?: React.ReactElement;
  align?: "start" | "center" | "end";
  ariaLabel?: string;
}) {
  const [open, setOpen] = useState(false);
  const selected = runtimes.find((runtime) => runtime.id === value) ?? runtimes[0] ?? null;
  const label = selected?.name ?? ariaLabel;

  return (
    <PropertyPicker
      open={open}
      onOpenChange={setOpen}
      width="w-52"
      align={align}
      triggerRender={triggerRender}
      tooltip={label}
      trigger={<span className="min-w-0 truncate">{label}</span>}
    >
      {runtimes.map((runtime) => (
        <PickerItem
          key={runtime.id}
          selected={runtime.id === value}
          onClick={() => {
            onChange(runtime.id);
            setOpen(false);
          }}
          tooltip={runtime.name}
        >
          <span className="min-w-0 truncate">{runtime.name}</span>
        </PickerItem>
      ))}
    </PropertyPicker>
  );
}

"use client";

import { useMemo, useState } from "react";
import { Globe, Lock } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { ModelDropdown } from "./model-dropdown";
import { InstructionsEditor } from "./instructions-editor";
import { SkillMultiSelect } from "./skill-multi-select";
import { AvatarPicker } from "./avatar-picker";
import { ProviderLogo } from "../../runtimes/components/provider-logo";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { workspaceKeys } from "@multica/core/workspace/queries";
import type {
  Agent,
  AgentVisibility,
  RuntimeDevice,
  MemberWithUser,
  CreateAgentRequest,
} from "@multica/core/types";
import {
  RUNTIME_PROFILE_PROTOCOL_FAMILIES,
  type RuntimeProtocolFamily,
} from "@multica/core/types";
import { isImeComposing } from "@multica/core/utils";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { toast } from "sonner";
import {
  AGENT_DESCRIPTION_MAX_LENGTH,
  VISIBILITY_DESCRIPTION,
  VISIBILITY_LABEL,
} from "@multica/core/agents";
import { CharCounter } from "./char-counter";
import { useT } from "../../i18n";

export function CreateAgentDialog({
  runtimes,
  members: _members,
  currentUserId,
  template,
  squadId,
  onClose,
  onCreate,
}: {
  runtimes: RuntimeDevice[];
  members: MemberWithUser[];
  currentUserId: string | null;
  // When provided, the dialog opens in "Duplicate" mode: the visible
  // fields (name / description / runtime / visibility / model) are
  // pre-populated from this agent, and the hidden fields
  // (instructions / custom_args / custom_env / max_concurrent_tasks)
  // are forwarded to the create call so the new agent is a true clone.
  // Skills are copied separately by the caller after createAgent
  // succeeds — they're not part of CreateAgentRequest.
  template?: Agent | null;
  // When set, every successful create is followed by
  // addSquadMember(squadId, agent) so the new agent joins this squad.
  // If the squad-join call fails the agent still exists and the dialog
  // surfaces a warning toast — the user can add it manually from the
  // Members tab.
  squadId?: string;
  onClose: () => void;
  // Returns the created Agent so the dialog can run a follow-up
  // setAgentSkills with the IDs the user picked in the form. Pre-skill-
  // section callers can keep returning `void`; the dialog tolerates a
  // falsy return (no follow-up runs).
  onCreate: (data: CreateAgentRequest) => Promise<Agent | void>;
}) {
  const { t } = useT("agents");
  const isDuplicate = !!template;
  const queryClient = useQueryClient();
  const wsId = useWorkspaceId();
  const capabilityOptions = useMemo(
    () => buildRuntimeCapabilityOptions(runtimes, currentUserId, template),
    [runtimes, currentUserId, template],
  );

  // Name defaults: duplicate uses "<original> copy". Manual-create starts blank.
  const [name, setName] = useState(
    template ? `${template.name}${t(($) => $.create_dialog.duplicate_copy_suffix)}` : "",
  );
  const [description, setDescription] = useState(template?.description ?? "");
  const [visibility, setVisibility] = useState<AgentVisibility>(
    template?.visibility ?? "workspace",
  );
  const [model, setModel] = useState(template?.model ?? "");
  const [instructions, setInstructions] = useState(template?.instructions ?? "");
  const [avatarUrl, setAvatarUrl] = useState<string | null>(template?.avatar_url ?? null);
  const [selectedSkillIds, setSelectedSkillIds] = useState<Set<string>>(
    () => new Set(template?.skills.map((s) => s.id) ?? []),
  );
  const [creating, setCreating] = useState(false);

  const [selectedCapabilityKey, setSelectedCapabilityKey] = useState(() =>
    defaultRuntimeCapabilityKey(template, runtimes, currentUserId),
  );

  const selectedCapability =
    capabilityOptions.find((option) => option.key === selectedCapabilityKey) ??
    capabilityOptions[0] ??
    builtinRuntimeCapabilityOption("codex");
  const modelRuntime = selectedCapability.runtimeForModels ?? null;

  // Shared squad-join follow-up. Returns nothing — the caller has
  // already shown its create-success toast; we only need to surface a
  // warning when the agent landed but the squad-join failed. Cache
  // invalidation for the squad's members list rides along so the
  // Members tab re-renders without a manual refetch.
  const attachToSquad = async (agentId: string, displayName: string) => {
    if (!squadId) return;
    try {
      await api.addSquadMember(squadId, {
        member_type: "agent",
        member_id: agentId,
      });
      if (wsId) {
        queryClient.invalidateQueries({
          queryKey: [...workspaceKeys.squads(wsId), squadId, "members"],
        });
        queryClient.invalidateQueries({
          queryKey: [...workspaceKeys.squads(wsId), squadId],
        });
      }
    } catch (err) {
      toast.warning(
        t(($) => $.create_dialog.squad_join_failed_toast, {
          name: displayName,
          error: err instanceof Error ? err.message : "unknown error",
        }),
      );
    }
  };

  const handleSubmit = async () => {
    if (!name.trim() || !selectedCapability.provider) return;
    setCreating(true);

    try {
      const trimmedInstructions = instructions.trim();
      const data: CreateAgentRequest = {
        name: name.trim(),
        description: description.trim(),
        runtime_provider: selectedCapability.provider,
        runtime_profile_id: selectedCapability.profileId ?? undefined,
        visibility,
        model: model.trim() || undefined,
        instructions: trimmedInstructions || undefined,
        avatar_url: avatarUrl ?? undefined,
      };
      if (template) {
        // Duplicate path: forward the hidden config fields the source
        // agent had so the clone is functional out of the box (args /
        // concurrency). Skills flow through the dialog form. As of
        // MUL-2600 the agent resource shape no longer carries
        // custom_env values, so duplication cannot copy env at all —
        // the user has to re-set env on the clone via the env tab
        // (which now goes through the audited `/env` endpoint). The
        // dialog's create call still accepts custom_env at create
        // time, but the source values aren't available here.
        if (template.custom_args.length) data.custom_args = template.custom_args;
        if (template.max_concurrent_tasks) {
          data.max_concurrent_tasks = template.max_concurrent_tasks;
        }
      }
      const createdAgent = await onCreate(data);
      // Follow-up: attach selected skills to the newly created agent.
      // onCreate returns the created Agent for this path; if the caller
      // doesn't return it we fall back to skipping (preserves
      // backward compatibility with non-skill-aware callers).
      if (createdAgent && selectedSkillIds.size > 0) {
        try {
          await api.setAgentSkills(createdAgent.id, {
            skill_ids: [...selectedSkillIds],
          });
          if (wsId) {
            queryClient.invalidateQueries({ queryKey: workspaceKeys.agents(wsId) });
          }
        } catch (skillErr) {
          // Non-fatal: agent exists, skills can be added on the detail
          // page. Surface as a warning toast so the user knows.
          toast.warning(
            t(($) => $.create_dialog.skill_attach_failed_toast, {
              error:
                skillErr instanceof Error ? skillErr.message : "unknown error",
            }),
          );
        }
      }
      // Squad context: attach the agent after skills land so the
      // squad's Members tab shows the agent with its skills already
      // in place. Atomicity is best-effort by design (see plan in
      // MUL-2178) — a partial failure surfaces a warning toast and
      // the user can retry from the Add Member dialog.
      if (createdAgent && squadId) {
        await attachToSquad(createdAgent.id, createdAgent.name);
      }
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t(($) => $.create_dialog.create_failed_toast));
      setCreating(false);
    }
  };

  const headerTitle = isDuplicate
    ? t(($) => $.create_dialog.title_duplicate)
    : t(($) => $.create_dialog.title_create);

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="p-0 gap-0 flex flex-col overflow-hidden !top-1/2 !left-1/2 !-translate-x-1/2 !-translate-y-1/2 !w-full !max-w-2xl !h-[85vh]">
        <DialogHeader className="border-b px-5 py-3 space-y-0">
          <DialogTitle className="text-base font-semibold">{headerTitle}</DialogTitle>
          {isDuplicate && template && (
            <DialogDescription className="mt-1 text-xs">
              {t(($) => $.create_dialog.description_duplicate, { name: template.name })}
            </DialogDescription>
          )}
          {!isDuplicate && (
            <DialogDescription className="mt-1 text-xs">
              {t(($) => $.create_dialog.description_create)}
            </DialogDescription>
          )}
        </DialogHeader>

        <div className="flex-1 overflow-y-auto p-5">
          <div className="space-y-4 min-w-0">
            {/* Identity row: avatar (left) + name & description stack
                (right). The avatar visually anchors the identity of
                what the user is creating; pairing it with the Name
                field reads as "this is the agent's face + name",
                same shape as detail-page header so the affordance is
                instantly familiar. */}
            <div className="flex items-start gap-4">
              <AvatarPicker value={avatarUrl} onChange={setAvatarUrl} size={64} />
              <div className="flex-1 min-w-0 space-y-3">
                <div>
                  <Label className="text-xs text-muted-foreground">{t(($) => $.create_dialog.name_label)}</Label>
                  <Input
                    autoFocus
                    type="text"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder={t(($) => $.create_dialog.name_placeholder)}
                    className="mt-1"
                    onKeyDown={(e) => {
                      if (isImeComposing(e)) return;
                      if (e.key === "Enter") handleSubmit();
                    }}
                  />
                </div>

                <div>
                  <Label className="text-xs text-muted-foreground">{t(($) => $.create_dialog.description_label)}</Label>
                  <Input
                    type="text"
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    placeholder={t(($) => $.create_dialog.description_placeholder)}
                    maxLength={AGENT_DESCRIPTION_MAX_LENGTH}
                    className="mt-1"
                  />
                  <div className="mt-1">
                    <CharCounter
                      length={[...description].length}
                      max={AGENT_DESCRIPTION_MAX_LENGTH}
                    />
                  </div>
                </div>
              </div>
            </div>

            <div>
              <Label className="text-xs text-muted-foreground">{t(($) => $.create_dialog.visibility_label)}</Label>
              <div className="mt-1.5 flex gap-2">
                <button
                  type="button"
                  onClick={() => setVisibility("workspace")}
                  className={`flex flex-1 items-center gap-2 rounded-lg border px-3 py-2.5 text-sm transition-colors ${
                    visibility === "workspace"
                      ? "border-primary bg-primary/5"
                      : "border-border hover:bg-muted"
                  }`}
                >
                  <Globe className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <div className="text-left">
                    <div className="font-medium">{VISIBILITY_LABEL.workspace}</div>
                    <div className="text-xs text-muted-foreground">
                      {VISIBILITY_DESCRIPTION.workspace}
                    </div>
                  </div>
                </button>
                <button
                  type="button"
                  onClick={() => setVisibility("private")}
                  className={`flex flex-1 items-center gap-2 rounded-lg border px-3 py-2.5 text-sm transition-colors ${
                    visibility === "private"
                      ? "border-primary bg-primary/5"
                      : "border-border hover:bg-muted"
                  }`}
                >
                  <Lock className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <div className="text-left">
                    <div className="font-medium">{VISIBILITY_LABEL.private}</div>
                    <div className="text-xs text-muted-foreground">
                      {VISIBILITY_DESCRIPTION.private}
                    </div>
                  </div>
                </button>
              </div>
            </div>

            <div className="flex flex-col min-w-0">
              <Label className="text-xs text-muted-foreground">
                {t(($) => $.create_dialog.runtime_label)}
              </Label>
              <Select
                value={selectedCapability.key}
                onValueChange={(value) => {
                  if (value) setSelectedCapabilityKey(value);
                }}
              >
                <SelectTrigger className="mt-1.5 w-full">
                  <SelectValue>
                    <RuntimeCapabilityLabel option={selectedCapability} />
                  </SelectValue>
                </SelectTrigger>
                <SelectContent align="start" className="max-h-72">
                  {capabilityOptions.map((option) => (
                    <SelectItem key={option.key} value={option.key}>
                      <RuntimeCapabilityLabel option={option} />
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <ModelDropdown
              runtimeId={modelRuntime?.id ?? null}
              runtimeOnline={modelRuntime?.status === "online"}
              value={model}
              onChange={setModel}
              disabled={!modelRuntime}
            />

            {/* --- Optional sections (instructions / skills) ---
                Collapsed by default so quick-create stays fast.
                Duplicate pre-fills everything from the source agent. */}
            <InstructionsEditor
              value={instructions}
              onChange={setInstructions}
              placeholder={
                isDuplicate
                  ? t(($) => $.create_dialog.instructions.placeholder_duplicate)
                  : t(($) => $.create_dialog.instructions.placeholder_blank)
              }
            />

            <SkillMultiSelect
              selectedIds={selectedSkillIds}
              onChange={setSelectedSkillIds}
            />
          </div>
        </div>

        {/* Inline footer instead of <DialogFooter>: the shipped
            DialogFooter applies `-mx-4 -mb-4` assuming a padded
            DialogContent (default `p-4`). Our DialogContent uses
            `p-0`, so those negative margins push the footer outside
            the dialog. A plain flex row anchored by `border-t` keeps
            the visual rhythm without the overflow bug. */}
        <div className="flex items-center justify-end gap-2 border-t bg-background px-5 py-3">
          <Button variant="ghost" onClick={onClose}>
            {t(($) => $.create_dialog.cancel)}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={
              creating || !name.trim() || !selectedCapability.provider
            }
          >
            {creating ? t(($) => $.create_dialog.creating) : t(($) => $.create_dialog.create)}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

type RuntimeCapabilityOption = {
  key: string;
  label: string;
  provider: string;
  profileId: string | null;
  runtimeForModels: RuntimeDevice | null;
};

function buildRuntimeCapabilityOptions(
  runtimes: RuntimeDevice[],
  currentUserId: string | null,
  template?: Agent | null,
): RuntimeCapabilityOption[] {
  const ownLocalRuntimes = runtimes.filter(
    (runtime) =>
      runtime.runtime_mode === "local" &&
      !!currentUserId &&
      runtime.owner_id === currentUserId,
  );
  const options = RUNTIME_PROFILE_PROTOCOL_FAMILIES.map((provider) =>
    builtinRuntimeCapabilityOption(
      provider,
      ownLocalRuntimes.find((runtime) => runtime.provider === provider && !runtime.profile_id) ??
        null,
    ),
  );

  const customByProfile = new Map<string, RuntimeDevice>();
  for (const runtime of ownLocalRuntimes) {
    if (runtime.profile_id && !customByProfile.has(runtime.profile_id)) {
      customByProfile.set(runtime.profile_id, runtime);
    }
  }
  for (const [profileId, runtime] of customByProfile) {
    options.push({
      key: `profile:${profileId}`,
      label: runtime.name,
      provider: runtime.provider,
      profileId,
      runtimeForModels: runtime,
    });
  }

  if (
    template?.runtime_profile_id &&
    !options.some((option) => option.profileId === template.runtime_profile_id)
  ) {
    options.push({
      key: `profile:${template.runtime_profile_id}`,
      label: template.runtime_profile_id,
      provider: template.runtime_provider || "codex",
      profileId: template.runtime_profile_id,
      runtimeForModels: null,
    });
  }

  return options;
}

function builtinRuntimeCapabilityOption(
  provider: RuntimeProtocolFamily | string,
  runtimeForModels: RuntimeDevice | null = null,
): RuntimeCapabilityOption {
  return {
    key: `provider:${provider}`,
    label: provider,
    provider,
    profileId: null,
    runtimeForModels,
  };
}

function defaultRuntimeCapabilityKey(
  template: Agent | null | undefined,
  runtimes: RuntimeDevice[],
  currentUserId: string | null,
): string {
  if (template?.runtime_profile_id) return `profile:${template.runtime_profile_id}`;
  if (template?.runtime_provider) return `provider:${template.runtime_provider}`;
  const firstOwnLocal = runtimes.find(
    (runtime) =>
      runtime.runtime_mode === "local" &&
      !!currentUserId &&
      runtime.owner_id === currentUserId,
  );
  if (firstOwnLocal?.profile_id) return `profile:${firstOwnLocal.profile_id}`;
  return `provider:${firstOwnLocal?.provider ?? "codex"}`;
}

function RuntimeCapabilityLabel({ option }: { option: RuntimeCapabilityOption }) {
  return (
    <span className="flex min-w-0 items-center gap-2">
      <ProviderLogo provider={option.provider} className="h-4 w-4 shrink-0" />
      <span className="truncate">{option.label}</span>
    </span>
  );
}

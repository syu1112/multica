# Creating agents — source map

Evidence layer for `SKILL.md`. Every contract maps to `file:line` on the
current tree (branch `feat/builtin-skills`, latest `main` merged), the runtime
effect, and a safe read-only check. Line numbers were re-derived against this
tree — re-derive again if the files move, the surrounding context (not the
number) is the anchor.

## Verification

```bash
# Conformance eval for this skill (and the shared template invariants):
go test ./internal/service -run TestCreatingAgentsSkillCoversAgentCreationContracts
go test ./internal/service -run TestBuiltinSkillsConformToTemplate
```

## CLI entry points — `server/cmd/multica/cmd_agent.go`

| Contract | Line | Behavior | Safe check |
|---|---|---|---|
| Create flags: `name`, `description`, `instructions`, runtime capability | 159–162 | Registered create flags; `name` plus provider/profile capability are the supported create contract. Legacy `runtime-id` is compatibility input only. | `multica agent create --help` |
| `runtime-config`, `model`, `thinking-level`, `custom-args` flags | 163–166 | `model` help: "Prefer this over passing --model in --custom-args"; `thinking-level` is a thin pass-through (server validates the provider enum, empty = runtime default); `custom-args` help names codex/openclaw rejecting `--model` (CLI help only, not server-enforced) | `multica agent create --help` |
| Secret-safe env input: `custom-env`, `custom-env-stdin`, `custom-env-file` | 167–169 | `--custom-env` warns about shell history / `ps`; stdin and file modes keep secrets off the command line; mutually exclusive | `multica agent create --help` |
| Secret-safe MCP input: `mcp-config`, `mcp-config-stdin`, `mcp-config-file` (create) | 170–172 | Same three-channel pattern as `custom-env`; `--mcp-config` warns about shell history / `ps`; value must be a JSON object or `null` | `multica agent create --help` |
| MCP flags on `agent update` | 194–196 | Same three channels on update; `--mcp-config null` clears. Unlike `custom_env`, `mcp_config` IS settable via update | `multica agent update --help` |
| `thinking-level` flag on `agent update` | 184 | New reasoning/effort level; thin pass-through; `--thinking-level ""` clears to runtime default (mirrors `--model`) | `multica agent update --help` |
| `runAgentCreate` builds body + `POST /api/agents` | 419 | Only sets a body key when the flag `Changed`; posts to `/api/agents` (line 495) | read 419–496 |
| Body assembly: description/instructions/runtime-config/custom-args/custom-env/mcp-config/model/thinking-level | 438–488 | `resolveCustomEnv` (460) and `resolveMcpConfig` (465) gate their secret channels; `model` (470) and `thinking_level` (478) are `Changed`-gated pass-throughs; omitted flags are not sent. The CLI has no dedicated `execution_mode` flag; use API/UI for that field. | read 438–488 |
| `runAgentUpdate` sends `thinking_level` / `mcp_config` | 508 | `thinking_level` added when `--thinking-level` is `Changed` (556); `resolveMcpConfig` adds `mcp_config` (570); `PUT /api/agents/{id}` at 584; `custom_env` is intentionally not a flag here | read 508–585 |
| `parseMcpConfig` / `resolveMcpConfig` helpers | 1086, 1114 | Validator (object-or-`null`, content-free errors) + three-channel resolver, mirroring `parseCustomEnv`/`resolveCustomEnv` | read 1086–1170 |
| `agent skills set` = replace-all | 792 | `PUT /api/agents/{id}/skills` (810); `--skill-ids ''` clears all (798–799) | `multica agent skills set --help` |
| `agent skills add` = additive | 817 | `POST /api/agents/{id}/skills/add` (838); requires ≥1 id (823–828) | `multica agent skills add --help` |
| `agent skills list` | 760 | reads bindings, no side effect | `multica agent skills list --help` |
| `agent env get` | 894 | `GET /api/agents/{id}/env` | `multica agent env get --help` |
| `agent env set` | 929 | `PUT /api/agents/{id}/env` with full `custom_env` map (935, 949) | `multica agent env set --help` |

Note: the CLI no longer exposes `--from-template`. The agent-template backend
still exists (registry `server/internal/agenttmpl/`, handler `agent_template.go`,
routes `GET /api/agent-templates` and `POST /api/agents/from-template`, plus the
`packages/core` client/query wrappers) but is currently orphaned plumbing with no
live caller: the removed CLI flag was its only non-test consumer, and onboarding
does NOT use it — `packages/views/onboarding/steps/step-agent.tsx` builds four
hardcoded local presets (i18n-resolved) and creates via plain `POST /api/agents`
(`createAgent`), never `POST /api/agents/from-template`. Do not treat the template
API as a supported agent-creation path. This skill teaches manual `agent create`
only.

## Create handler — `server/internal/handler/agent.go`

| Contract | Line | Behavior |
|---|---|---|
| `maxAgentDescriptionLength = 255` | 31 | Cap is 255 **Unicode code points** (comment: counted via `utf8.RuneCountInString`, matches Postgres `char_length`) |
| `AgentResponse` omits plaintext `custom_env` and redacts legacy `runtime_id` | 39–71 | `RuntimeID *string` remains nullable for compatibility and user-facing responses return `null`; exposes only `has_custom_env`, `custom_env_key_count`, and top-level `execution_mode`; comment cites MUL-2600 |
| `CreateAgentRequest` fields | 758–778 | `description`, `instructions`, `runtime_provider`, `runtime_profile_id`, legacy `runtime_id`, `runtime_config`, `custom_env`, `custom_args`, `model`, `thinking_level`, `execution_mode` (plus name/avatar/visibility/mcp_config/max_concurrent_tasks) |
| `name` required | 721–723 | 400 "name is required" |
| `description` ≤ 255 code points | 724–726 | `utf8.RuneCountInString(req.Description) > maxAgentDescriptionLength` → 400 |
| runtime capability required | 737–787 | `runtime_provider` or `runtime_profile_id` required unless legacy `runtime_id` supplies provider/profile; otherwise 400 "runtime_provider is required" |
| legacy `runtime_id` compatibility | 745–768 | parsed + `GetAgentRuntimeForWorkspace`; must be owned by caller; converted to provider/profile and not persisted as a new binding |
| `thinking_level` provider-level validation | 795–798 | `!agent.IsKnownThinkingValue(runtimeProvider, req.ThinkingLevel)` → 400; per-model gaps deferred to daemon (comment 789–794, MUL-2339) |
| `execution_mode` default and validation | 833–837, 1266–1271 | create defaults omitted values to `normal`; create/update reject any value other than `normal` or `goal`; update omission leaves the existing value unchanged |
| Defaults: `{}` config/env, `[]` args | 810–823 | `RuntimeConfig`→`{}`, `CustomEnv`→`{}`, `CustomArgs`→`[]` when nil, before insert |
| `visibility` default | 727–729 | `if req.Visibility == "" { req.Visibility = "private" }` — access-control field, not the runtime prompt |
| `max_concurrent_tasks` default | 730–732 | `if req.MaxConcurrentTasks == 0 { req.MaxConcurrentTasks = 6 }` — scheduler cap |
| `mcp_config` null-skip on create | 826–828 | raw JSON copied through unless the body value is the literal `null` |
| `mcp_config` redacted on read | 54, 848–851 | `redactMcpConfig` sets `McpConfigRedacted=true`; a private agent read by a member also redacts (494, 509) |
| `CreateAgent` insert params | 937–956 | persists runtime_provider/runtime_profile_id, runtime_config, instructions, custom_env, custom_args, model, thinking_level, execution_mode, mcp_config, visibility, max_concurrent_tasks; writes NULL legacy runtime_id for new agents |
| `UpdateAgent` rejects `custom_env` | 1041–1044 | if `custom_env` present in body → 400 "use PUT /api/agents/{id}/env (or `multica agent env set`)" |
| `UpdateAgent` persists / clears `mcp_config` | 1080–1084, 1244–1245 | Tri-state from the raw body: key omitted → no change; literal `null` → `ClearAgentMcpConfig`; object → replace. No 400 like `custom_env` — `mcp_config` IS updatable here |
| `description` ≤ 255 on update too | 1051–1054 | same cap re-checked on update |

## Env endpoint — `server/internal/handler/agent_env.go`

| Contract | Line | Behavior |
|---|---|---|
| `authorizeAgentEnv` gate | 66 | loads agent, then applies the two checks below |
| Agent actors denied | 80–84 | `if actorType == "agent"` → 403 "agents may not access env management endpoints" (MUL-2600 impersonation guard) |
| Owner/admin only | 86 | `requireWorkspaceRole(..., "owner", "admin")` |

## Routes — `server/cmd/server/router.go`

| Contract | Line | Behavior |
|---|---|---|
| `GET /env` | 603 | `h.GetAgentEnv` (plaintext read, gated) |
| `PUT /env` | 604 | `h.UpdateAgentEnv` (full-map overwrite, gated) |

## Claim-time injection — `server/internal/handler/daemon.go`

| Contract | Line | Behavior |
|---|---|---|
| Fresh agent re-read on claim | 1109–1111 | `GetAgent(task.AgentID)` — claim uses persisted fields, not create output |
| Workspace skills FIRST | 1115 | `skills := h.TaskService.LoadAgentSkills(...)` |
| Built-ins appended | 1116 | `skills = append(skills, h.TaskService.BuiltinSkills()...)` |
| Runtime payload | 1320–1334 | `TaskAgentData` carries `Instructions`, `Skills`, `CustomEnv`, `CustomArgs`, `Model`, `ThinkingLevel`, `ExecutionMode`, `McpConfig` — confirms these are runtime-consumed; `description`, `visibility`, and `max_concurrent_tasks` are absent (not runtime-prompt fields) |

## Skill loading — `server/internal/service/task.go`

| Contract | Line | Behavior |
|---|---|---|
| `LoadAgentSkills` | 1685 | `ListAgentSkills` + per-skill `ListSkillFiles` → content + supporting files for execution |

## Built-in skills — `server/internal/service/builtin_skills.go`

| Contract | Line | Behavior |
|---|---|---|
| `go:embed builtin_skills` | 10–11 | skills embedded at compile time |
| `loadBuiltinSkill` | 45 | reads `<name>/SKILL.md` (47) + walks sibling files into `Files` (56–68) |

## Persisted columns — `server/pkg/db/generated/agent.sql.go`

| Contract | Line | Behavior |
|---|---|---|
| `CreateAgent` INSERT | 754–762 | columns include `runtime_config`, legacy nullable `runtime_id`, `runtime_provider`, `runtime_profile_id`, `instructions`, `custom_env`, `custom_args`, `mcp_config`, `model`, `thinking_level`, `execution_mode` |
| `CreateAgentParams` | 764–782 | typed params include `RuntimeProvider string`, `RuntimeProfileID pgtype.UUID`, `RuntimeConfig []byte`, `Instructions string`, `CustomEnv []byte`, `CustomArgs []byte`, `Model pgtype.Text`, `ThinkingLevel pgtype.Text`, `ExecutionMode string` |
| `UpdateAgent` SET | 2705–2736 | COALESCE updates of runtime capability, `runtime_config`, `instructions`, `custom_args`, `model`, `thinking_level`, `execution_mode`; runtime capability changes clear legacy `runtime_id`; note `custom_env` is COALESCE-guarded but the handler rejects it before this query runs |
| `UpdateAgentCustomEnv` (called by the `UpdateAgentEnv` handler) | 2741 | `SET custom_env = $2` — the only write path for env values |

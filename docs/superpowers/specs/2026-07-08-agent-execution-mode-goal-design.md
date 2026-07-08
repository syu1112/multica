# Agent 执行模式：Goal

## 状态

待审阅草稿。

## 摘要

为 Agent 增加一个 Agent 级别的 `Execution mode` 设置，支持两个值：

- `Normal`：保持当前执行行为。
- `Goal`：让 Agent 任务以目标导向的执行契约运行。

该设置适用于所有 Agent。新建 Agent 默认是 `Normal`，历史 Agent 也会被回填为 `Normal`。除非用户明确把某个 Agent 切换为 `Goal`，否则现有 Agent 的行为不会发生变化。

首批实现 Claude Code 和 Codex 两个 provider。Codex 使用 app-server 的 `thread/goal/set` 协议。Claude Code 由于当前在 Multica 中通过非交互式 stream JSON 运行，仓库内也没有接入原生 goal 协议，因此先在任务输入中注入明确的 goal 执行契约。

## 目标

- 允许用户为每个 Agent 选择 `Normal` 或 `Goal` 执行模式。
- 保持新建和历史 Agent 的默认行为不变，均为 `Normal`。
- 支持 Claude Code 和 Codex Agent。
- UI 范围只增加一个控制项：`Execution mode`。
- 本次不扩展 sandbox、approval、reasoning、worktree、后台 Agent、review mode 等配置。

## 非目标

- 不在 issue 的 run/rerun 菜单里增加单次运行覆盖选项。
- 不增加安全模式或审批模式配置。
- 不增加 goal token budget UI。
- 首版不增加自定义 goal 模板 UI。
- 不在 UI 中做 provider 专属模式矩阵。
- 不改变任务入队、分配、提及或重跑的方式。

## 用户体验

### Agent 详情页

在 Agent 详情页左侧 Inspector 的 Properties 区域增加一行：

- 标签：`Execution mode`
- 控件：segmented control 或紧凑 picker，选项为 `Normal` 和 `Goal`
- 位置：放在 `Runtime` 后、`Model` 前
- 只有具备 Agent 编辑权限的用户可以切换
- 只读用户看到静态文本值

该控件对所有 Agent 可见。暂未实现运行时行为的 provider 仍然可以保存该值，但首批只有 Claude Code 和 Codex 会实际执行。若当前 runtime provider 暂不支持，后续可以加禁用提示；本版本保持 UI 简洁和 provider-neutral。

### 新建 Agent

新建 Agent 流程中 `Execution mode` 默认是 `Normal`。首版不强制在新建弹窗中增加该控件，除非现有布局里有自然且紧凑的位置。即使创建时不传该字段，API 和数据库默认值也会把 Agent 创建为 `Normal`。

### 历史 Agent

迁移后所有历史 Agent 都视为 `Normal`。这保证所有 workspace 的现有行为不变。

## 数据模型

在 `agent` 表增加顶层字段：

```sql
execution_mode TEXT NOT NULL DEFAULT 'normal'
```

允许值：

- `normal`
- `goal`

建议增加数据库 check constraint，避免非法值被持久化：

```sql
CHECK (execution_mode IN ('normal', 'goal'))
```

不要把该设置放进 `runtime_config`。Execution mode 是 Multica 的跨 provider Agent 执行策略，不是某个 runtime 的私有配置。

## API 契约

### Agent Response

增加字段：

```ts
execution_mode: "normal" | "goal"
```

前端响应解析应当把缺失或异常值 fallback 为 `normal`，以保护已安装桌面端和新旧版本混跑场景。

### Create Agent

允许可选字段：

```ts
execution_mode?: "normal" | "goal"
```

如果未传，服务端存储 `normal`。

### Update Agent

允许可选字段：

```ts
execution_mode?: "normal" | "goal"
```

未传表示不修改。非法值返回 `400`。

## 运行时行为

### 共享 Daemon 流程

在 task agent payload 和 `agent.ExecOptions` 中增加 `ExecutionMode`。

Daemon 领取任务时，把 Agent 持久化的 execution mode 传给 provider backend。`Normal` 不改变 prompt、参数、session 处理或 provider 行为。

对于 `Goal`，从 Multica 当前已经发送给 provider 的同一份 task prompt 中派生 provider-neutral 的 goal objective。首版不增加新的 issue 专属查询链路，这样 chat、mention、assignment、rerun、autopilot 路径都能保持一致。

### Codex

在 `thread/start` 或成功 `thread/resume` 之后、`turn/start` 之前调用：

```json
{
  "method": "thread/goal/set",
  "params": {
    "threadId": "<thread-id>",
    "objective": "<derived objective>",
    "status": "active",
    "tokenBudget": null
  }
}
```

如果本地 Codex 版本不支持 `thread/goal/set`，实现时需要在“失败关闭”和“告警后继续”之间做取舍。建议行为：记录明确 warning，然后按 `Normal` 继续执行，以兼容较旧的 Codex 安装。

首版可以忽略 Codex goal status notification。任务完成仍然沿用现有 `turn/completed` 和 final answer 处理。

### Claude Code

Multica 当前使用非交互式 stream JSON 启动 Claude Code，并固定了一组 daemon 持有的协议参数。仓库内没有现成的 Claude 原生 goal 协议调用。

对于 `Goal`，在发送给 Claude 的用户 prompt 前追加明确的 goal 契约：

```text
# Execution mode: Goal

Work until this goal is genuinely handled:
<derived objective>

If the goal is complete, say so in the final answer. If blocked, explain the blocking condition and what is needed next.
```

保持原始 prompt 的其余部分不变。这样不改变 Claude 的 permission mode、output format 或 session 管理，同时能表达目标导向执行语义。

## 前端实现说明

- 在 `packages/core/types/agent.ts` 增加 `execution_mode`。
- 在 `packages/core/api/schemas.ts` 增加响应解析 fallback。
- 在 Agent create/update payload 支持该字段。
- 在现有 inspector pickers 附近增加 `ExecutionModePicker`；如果没有必要抽象，也可以直接在 `agent-detail-inspector.tsx` 内实现一个小型 segmented control。
- 增加 i18n 文案：
  - `Execution mode`
  - `Normal`
  - `Goal`

控件应保持紧凑，视觉密度匹配当前 Inspector property row，不新增大型设置面板。

## 后端实现说明

- 增加 `agent.execution_mode` 迁移。
- 更新 sqlc query selections 和 generated code。
- 更新 `server/internal/handler/agent.go` 中的 Agent response 映射。
- 校验 create/update request 的字段值。
- 在 `server/internal/handler/daemon.go` 的 daemon task payload 构造中透传该值。
- 在 `server/pkg/agent.ExecOptions` 中增加 `ExecutionMode`。
- 在以下 provider 中实现运行时行为：
  - `server/pkg/agent/codex.go`
  - `server/pkg/agent/claude.go`

## 兼容性

- 新 DB 默认值保证历史行为不变。
- API schema fallback 保护收到旧响应的客户端。
- 不认识 `execution_mode` 的旧 daemon 会继续按 `Normal` 行为运行。
- 新 daemon 运行旧 Agent 时会从 DB 默认值拿到 `normal`。
- Codex 安装版本缺少 goal 协议时不应导致所有任务失败；实现应清晰记录降级 warning。

## 测试

后端：

- 不传 `execution_mode` 创建 Agent 时存储 `normal`。
- 传 `goal` 创建 Agent 时存储并返回 `goal`。
- Agent 可从 `normal` 更新到 `goal`，也可从 `goal` 更新回 `normal`。
- 非法 execution mode 返回 `400`。
- 历史/默认迁移结果为 `normal`。
- Daemon payload 包含 execution mode。
- Codex `Goal` 在 `turn/start` 前发送 `thread/goal/set`。
- Codex `Normal` 不发送 `thread/goal/set`。
- Claude `Goal` 注入 goal 契约。
- Claude `Normal` 不改变 prompt 行为。

前端：

- Agent schema 在缺失 `execution_mode` 时默认 `normal`。
- Inspector 显示 `Execution mode`。
- 可编辑用户能切换 `Normal`/`Goal` 并发送 update。
- 只读用户看到当前值，但没有交互控件。
- 新建 Agent 默认保持 `Normal`。

## 待确认问题

- 暂未实现的 provider 是否应静默忽略 `Goal`，还是 UI 先只允许 Claude Code 和 Codex 选择该项？
- Codex `thread/goal/set` 失败时，是降级为 `Normal` 并 warning，还是直接让任务失败以便用户立刻发现本地 Codex 版本过旧或配置异常？
- derived goal objective 首版应直接使用完整 prompt，还是从 issue 标题/摘要中提取更短目标？

## 推荐决策

- UI 首版保持 provider-neutral，但运行时只实现 Claude Code 和 Codex。
- Codex goal setup 失败时降级为 `Normal` 并记录明确 warning，避免较旧本地 runtime 直接中断任务执行。
- 首版使用现有完整 task prompt 作为 objective 来源。只有当用户确实需要更多控制时，再增加自定义 goal 模板。

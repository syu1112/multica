# Issue 详情页内容宽度切换 Spec

> 状态：Draft，待产品与技术审核  
> 日期：2026-07-14

## 1. 背景

当前 Issue 详情页的中间内容区使用固定的文档阅读宽度：

```tsx
<div className="mx-auto w-full max-w-4xl px-8 py-8">
```

`max-w-4xl` 将标题、描述、附件、子 issue、动态时间线和评论输入区限制在约 896px，并在内容面板内居中。这个宽度适合阅读普通文本，但在查看宽表格、长代码块、SQL 报告、Markdown 报告和较复杂的执行结果时，左右会留下较大空白，无法充分利用屏幕空间。

Issue 详情页外层同时包含一个独立的、可伸缩且可折叠的右侧属性栏。内容宽度模式只应影响中间文档，不应改变内容面板与属性栏的分栏比例。

## 2. 目标

在桌面端的完整 Issue 详情页增加“默认宽度 / 宽屏”切换：

- 默认宽度：保持现有 `max-w-4xl` 布局和阅读体验。
- 宽屏：中间文档使用内容面板的全部可用宽度。
- 切换后立即生效，不刷新页面，不重新请求 Issue 数据。
- 用户偏好按工作区持久化，Web 与 Desktop 复用同一套状态与交互实现。
- 不改变右侧属性栏的打开状态、宽度、拖拽行为和现有持久化数据。
- 不改变滚动容器、时间线虚拟滚动、评论定位和返回页面时的滚动恢复。

## 3. 非目标

- 不提供任意宽度拖拽或自定义像素值。
- 不改变右侧属性栏的 260–420px 尺寸范围。
- 不把“宽屏”解释为隐藏左侧导航、隐藏右侧属性栏或进入浏览器全屏。
- 不调整标题、描述、时间线或评论组件本身的排版。
- 不修改服务端、数据库、API 或 React Query 数据。
- 第一版不在移动端和收件箱的嵌入式 Issue 详情中开放该切换。
- 第一版不扩展到项目详情页或其他 `max-w-4xl` 页面。

## 4. 用户体验

### 4.1 入口位置

在完整 Issue 详情页顶部 `BreadcrumbHeader` 的右侧操作区新增一个 icon-only 按钮，放在“更多”与“切换侧边栏”之间：

```text
[智能体状态] [完成/归档] [固定] [更多] [内容宽度] [侧边栏]
```

建议使用 `lucide-react` 已有图标：

- 当前为默认宽度：显示 `Maximize2`，点击后切换到宽屏。
- 当前为宽屏：显示 `Minimize2`，点击后恢复默认宽度。

按钮沿用头部现有的 `Button variant="ghost" size="icon-sm"`、`Tooltip` 和语义色。当处于宽屏时，按钮使用 `secondary` variant 表示模式已启用，与右侧栏按钮的激活样式保持一致。

### 4.2 文案与无障碍

Tooltip 与 `aria-label` 描述点击后的动作：

| i18n key | 中文 | 英文 |
| --- | --- | --- |
| `detail.enable_wide_content` | 切换为宽屏 | Use wide content width |
| `detail.restore_default_content_width` | 恢复默认宽度 | Restore default content width |

日文、韩文由对应 locale 补齐，不能回退为英文 key。

### 4.3 布局行为

默认宽度保留当前行为：

```tsx
"mx-auto w-full max-w-4xl px-8 py-8"
```

宽屏只移除最大宽度限制：

```tsx
"mx-auto w-full max-w-none px-8 py-8"
```

实现时使用同一个内容容器，通过 `cn(...)` 切换 `max-w-4xl` / `max-w-none`。不得复制两套 Issue 内容 DOM。

以下属性在两种模式下保持不变：

- `w-full`：窄窗口下仍可自然收缩。
- `mx-auto`：默认模式继续居中；宽屏模式不引入额外定位差异。
- `px-8 py-8`：切换时内容不会贴边或产生纵向位移。
- 外层 `min-w-0` 与 `overflow-y-auto`：避免长内容撑破面板。
- `data-tab-scroll-root`：继续作为详情页唯一滚动根节点。

宽屏模式不设置 `min-width`，因此右侧属性栏展开或拖宽后，中间内容仍服从 `ResizablePanel id="content" minSize="50%"` 的现有约束。

### 4.4 加载态

加载骨架中的内容容器必须使用相同的有效宽度模式，避免 Issue 数据返回后从 896px 突然跳到宽屏，或从宽屏跳回 896px。

### 4.5 响应式与嵌入态

| 场景 | 行为 |
| --- | --- |
| Web 完整 Issue 详情页 | 显示切换按钮，读取并保存偏好 |
| Desktop 完整 Issue 详情页 | 显示切换按钮，读取并保存偏好 |
| 移动端 | 不显示切换按钮，有效模式固定为默认宽度 |
| 收件箱嵌入式 Issue 详情 | 不显示切换按钮，有效模式固定为默认宽度 |

收件箱本身已经是列表 + 详情 + 属性栏的多栏布局，可用宽度较小；在其中启用宽屏收益有限，还会增加头部操作拥挤和评论定位重排风险。因此第一版保持当前布局。实现中必须通过显式 prop 区分嵌入态，不能通过比较 `layoutId` 字符串推断场景。

## 5. 状态设计

内容宽度属于持久化的客户端布局偏好，由 Zustand 管理，不能放入 React Query，也不能在 `packages/views` 直接访问 `localStorage`。

建议新增独立 store：

`packages/core/issues/stores/detail-view-store.ts`

```ts
export type IssueDetailContentWidth = "default" | "wide";

interface IssueDetailViewState {
  contentWidth: IssueDetailContentWidth;
  setContentWidth: (width: IssueDetailContentWidth) => void;
  toggleContentWidth: () => void;
}
```

默认值为 `"default"`。持久化 key 建议为：

```text
multica_issue_detail_view
```

持久化通过 `createJSONStorage(() => createWorkspaceAwareStorage(defaultStorage))` 完成，使偏好按当前工作区隔离，并通过 `registerForWorkspaceRehydration` 在切换工作区后重新读取。`partialize` 只保存 `contentWidth`。

该偏好保存在每个客户端的本地存储中，不通过服务端账号同步，因此 Web、Desktop 和不同设备之间不承诺自动同步具体取值。

不把该状态加入已有 `IssueViewState`，原因是后者描述 `/issues` 列表、看板、甘特和泳道视图；详情页布局偏好是独立关注点，避免列表状态接口继续膨胀。

不复用 `react-resizable-panels` 的 `layoutId` 存储。`layoutId` 只负责 content/sidebar 的面板尺寸；将内容宽度混入该数据会耦合两个不同的布局契约，也可能破坏已有用户保存的右栏宽度。

## 6. 组件接口

扩展 `IssueDetailProps`：

```ts
interface IssueDetailProps {
  // existing props...
  contentWidthToggleEnabled?: boolean;
}
```

规则：

- 默认值为 `true`，因此 Web 与 Desktop 的完整详情路由无需新增平台代码。
- `packages/views/inbox/components/inbox-page.tsx` 显式传入 `contentWidthToggleEnabled={false}`。
- 当 `isMobile === true` 或 `contentWidthToggleEnabled === false` 时：
  - 不渲染按钮；
  - 有效宽度固定为 `"default"`；
  - 不覆盖已持久化的桌面端偏好。
- Hook 仍按 React 规则无条件调用；只在计算 `effectiveContentWidth` 时应用场景限制。

建议计算：

```ts
const storedContentWidth = useIssueDetailViewStore((state) => state.contentWidth);
const toggleContentWidth = useIssueDetailViewStore((state) => state.toggleContentWidth);
const canToggleContentWidth = contentWidthToggleEnabled && !isMobile;
const effectiveContentWidth = canToggleContentWidth ? storedContentWidth : "default";
```

为减少测试对 Tailwind class 结构的耦合，可在内容容器上增加：

```tsx
data-content-width={effectiveContentWidth}
```

该属性只表达当前布局状态，不参与业务逻辑。

## 7. 代码改动范围

### 7.1 新增

- `packages/core/issues/stores/detail-view-store.ts`
  - 定义宽度类型、store、action 和工作区级持久化。
- `packages/core/issues/stores/detail-view-store.test.ts`
  - 覆盖默认值、切换和持久化字段。

### 7.2 修改

- `packages/core/issues/stores/index.ts`
  - 导出 `useIssueDetailViewStore` 和 `IssueDetailContentWidth`。
- `packages/views/issues/components/issue-detail.tsx`
  - 增加 prop、store selector、头部切换按钮和条件宽度 class。
  - 加载骨架与正常内容复用相同的宽度 class 计算。
  - 保持 `data-tab-scroll-root` 的节点和层级不变。
- `packages/views/inbox/components/inbox-page.tsx`
  - 显式关闭嵌入态的宽度切换。
- `packages/views/issues/components/issue-detail.test.tsx`
  - 补充默认、宽屏、移动端、嵌入态与右栏回归测试。
  - 现有 `@multica/core/issues/stores` mock 需要补齐新 store 的 callable Zustand 形态。
- `packages/views/locales/{en,zh-Hans,ja,ko}/issues.json`
  - 补齐两条 Tooltip/aria-label 文案。

无需修改：

- `apps/web/app/[workspaceSlug]/(dashboard)/issues/[id]/page.tsx`
- `apps/desktop/src/renderer/src/pages/issue-detail-page.tsx`
- 后端与数据库代码

## 8. 测试计划

### 8.1 Core store 单元测试

`packages/core/issues/stores/detail-view-store.test.ts`

- 初始 `contentWidth` 为 `"default"`。
- `toggleContentWidth()` 按 `default → wide → default` 切换。
- `setContentWidth()` 可显式设置两种合法值。
- 持久化快照只包含 `contentWidth`，不包含 action。
- 工作区切换重新水合对应工作区的值，不串用另一个工作区偏好。

### 8.2 共享组件测试

`packages/views/issues/components/issue-detail.test.tsx`

- 桌面完整详情页默认展示“切换为宽屏”按钮。
- 默认状态的内容容器为 `data-content-width="default"`，包含 `max-w-4xl`。
- 点击按钮后容器变为 `data-content-width="wide"`，包含 `max-w-none` 且不再包含 `max-w-4xl`。
- 宽屏状态按钮显示“恢复默认宽度”，再次点击恢复默认。
- 宽屏切换不改变右侧属性栏的 `open` 状态，不调用 `expand()` / `collapse()`。
- 移动端不显示按钮，内容固定为默认宽度。
- `contentWidthToggleEnabled={false}` 时不显示按钮，内容固定为默认宽度。
- 加载骨架遵循当前有效宽度模式。
- 切换前后 `data-tab-scroll-root` 仍是同一个滚动容器，时间线和评论输入区不被重新挂载。

### 8.3 手工回归

至少覆盖：

- Web 与 Desktop。
- 右侧属性栏：关闭、打开、最窄、最宽。
- 短文本、长 Markdown、代码块、宽表格、附件预览、Mermaid、子 issue 列表。
- 普通时间线、长时间线虚拟滚动、带 `highlightCommentId` 的评论定位。
- 切换宽度后返回列表再进入，偏好保持。
- 在两个工作区设置不同模式后往返切换，偏好相互隔离。
- 刷新页面后模式保持，且不存在明显的首屏宽度闪动。
- 浏览器窄窗口和移动端布局无横向溢出。

### 8.4 建议验证命令

```bash
pnpm --filter @multica/core exec vitest run issues/stores/detail-view-store.test.ts
pnpm --filter @multica/views exec vitest run issues/components/issue-detail.test.tsx
pnpm typecheck
```

## 9. 验收标准

- 用户可在完整 Issue 详情页头部一键切换默认宽度与宽屏。
- 默认模式与当前线上布局视觉一致。
- 宽屏模式下，中间内容填满右侧属性栏之外的可用内容面板，保留 32px 内边距。
- 模式切换不会隐藏或改变左侧导航、右侧属性栏和浏览器窗口。
- 模式偏好在当前客户端按工作区持久化，Web 与 Desktop 行为一致，但不要求跨客户端同步取值。
- 收件箱嵌入态和移动端不出现该按钮，布局不回归。
- 切换不会触发数据请求、编辑器重建、评论输入丢失、滚动位置重置或评论定位失效。
- 所有新增文案在 en、zh-Hans、ja、ko 四个 locale 中完整存在。
- 相关单元测试和 TypeScript 检查通过。

## 10. 风险与对策

### 10.1 宽屏导致长文本可读性下降

宽屏是显式选择，默认仍保持 896px 文档宽度。用户可以随时一键恢复默认宽度。

### 10.2 切换引发时间线高度重排和滚动偏移

宽度变化会让 Markdown 和评论重新换行，但不得更换滚动根节点或重建时间线。切换动作不主动调用滚动恢复；浏览器保留当前 `scrollTop`。深链接首次定位前使用已水合的有效模式，避免定位后立即二次重排。

### 10.3 与右侧属性栏状态互相污染

宽度偏好使用独立 Zustand key；右栏尺寸继续由现有 `layoutId` 和 `react-resizable-panels` 管理。宽度按钮不得调用 panel ref。

### 10.4 收件箱多栏布局过窄

通过显式 `contentWidthToggleEnabled={false}` 固定为默认模式，不根据 `layoutId` 或 DOM 宽度做隐式判断。

### 10.5 超宽屏内容过度拉伸

第一版按“填满中间展示区域”的需求使用 `max-w-none`。如果审核认为 4K 屏幕下全宽不可接受，可把宽屏上限调整为 `max-w-screen-2xl`；此选择只影响第 4.3 节和验收标准，不改变状态与组件设计。

## 11. 审核确认项

本 Spec 默认采用以下决策，请审核：

1. 宽屏使用 `max-w-none`，填满中间内容面板，而不是设置新的固定上限。
2. 偏好在当前客户端按工作区持久化；Web 与 Desktop 复用实现，但不跨客户端同步取值。
3. 收件箱嵌入态与移动端第一版不提供切换，并强制使用默认宽度。
4. 入口使用单个 `Maximize2` / `Minimize2` 切换按钮，位于“更多”和“侧边栏”之间。

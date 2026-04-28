# brainstorm: 修复页面风格对齐 DESIGN

## Goal

将 Web 控制台页面风格重新对齐 `DESIGN.md` 中的 Expo-inspired 设计系统，重点修复当前页面中不符合单色、无渐变、轻阴影、圆角和排版规范的 UI 表现，使工具列表、执行配置、工作流编排器和结果区域保持统一、克制、明亮的产品感。

## What I already know

* 用户指定参考 `DESIGN.md`，要求“把页面的风格修复下，有些内容不符合了”。
* 用户反馈“右侧就不要添加了吧”，节点本体右侧快捷 `+` 添加入口应删除，仅保留底部悬浮工具条、空画布入口、节点选择浮层和拖拽添加能力。
* 用户反馈“连线有问题了”，需要避免节点右侧按钮/区域遮挡 React Flow source handle，确保连线拖拽和显示稳定。
* 用户反馈“条件的节点，好像不支持多连”，条件节点必须允许从同一 source 连出多条到不同 target 的分支边，便于多个 case/default 分支编排。
* `DESIGN.md` 要求主界面使用 Cloud Gray `#f0f0f3`、Pure White `#ffffff`、Expo Black `#000000`、Slate Gray `#60646c`、Border Lavender `#e0e1e6`。
* `DESIGN.md` 明确禁止装饰性色、重阴影、锐角、界面渐变和过度饱和色。
* 当前 Web UI 是 React + Vite 单页应用，入口在 `web/src/main.jsx`，主要视觉实现集中在 `web/src/styles.css`。
* 当前大部分基础布局已接近设计系统：Cloud Gray 背景、白色卡片、pill tab/button、大标题、Inter 字体、轻量卡片边框。
* 明显不符合点集中在工作流编排器控制节点/条件节点：紫色装饰、紫色渐变、斜纹背景、彩色圆点、较重彩色阴影，与 `DESIGN.md` 的单色和无渐变要求冲突。

## Assumptions (temporary)

* 本任务从纯视觉修复扩展为轻量画布交互优化；允许在 `web/src/main.jsx` 中新增前端状态和事件处理，但不得改变后端 API、工作流 YAML 语义、保存/运行数据结构。
* 允许调整 CSS 和必要的轻量 JSX class/结构，但不做组件大重构。
* 页面仍应保留必要语义状态颜色，例如链接蓝、warning amber、destructive rose，但不用于装饰。

## Open Questions

* 请确认最终需求摘要；确认后进入实现。

## Requirements (evolving)

* 页面整体遵循 `DESIGN.md` 的单色、明亮、圆角、轻阴影视觉系统。
* 按 `DESIGN.md` 重设整体视觉层级：布局呼吸感、卡片层级、按钮/标签/输入、列表、结果区、工作流编排器保持统一。
* 画布内部控件也必须对齐设计系统，包括 React Flow 控制按钮、小地图、连线、handle、节点选中态和画布背景。
* 画布功能参考 Coze：保留底部工具条、空画布添加入口和浮层搜索添加节点能力；不再在节点右侧渲染快捷 `+` 添加按钮。
* 连线交互必须稳定：右侧 source handle 可见、可点击、可拖拽，不被节点按钮、伪元素或 padding 遮挡。
* 条件节点支持多 outgoing edges：同一条件节点可连到多个不同目标节点，仅阻止完全重复的 `source + target + sourceHandle + targetHandle` 连线组合。
* 条件节点必须在节点本体内按 `data.condition.cases` 渲染每个 case 的分支行与独立右侧出口，启用 default 时额外渲染 default 出口；React Flow `sourceHandle` 使用 case ID，default 使用 `default`。
* 从条件节点分支出口拖出的连线应自动写入 `edge.data.case = sourceHandle`，并用 case 名称或 default 作为 label；非条件节点连线不得携带 case 元数据。
* 新建条件节点连线初始化为空 case 与空 label，继续由现有 EdgeInspector 选择 case，并保持保存/运行前校验语义。
* 增加底部悬浮画布工具条，集中承载缩放、适配视图、锁定/交互状态、添加节点、运行工作流等高频画布操作；现有按钮语义和运行逻辑保持不变。
* 移除或替换编排器中非语义用途的紫色、渐变和重彩色阴影。
* 保持现有交互功能、数据流、API 调用和中文文案不变。
* 响应式布局不退化。

## Acceptance Criteria (evolving)

* [ ] 页面整体使用 Cloud Gray 背景、Pure White 卡片、Expo Black 标题/主 CTA、Slate Gray 次级文本。
* [ ] `web/src/styles.css` 不再使用条件/控制节点的装饰性紫色渐变背景。
* [ ] 编排器节点、节点面板、画布、检查器与卡片/按钮/标签风格一致。
* [ ] 画布内部控件（缩放/适配/锁定按钮、小地图、背景点阵、handle、连线标签、选中态）符合 DESIGN 的单色、轻阴影、圆角规范。
* [ ] 画布功能参考 Coze：可通过画布添加入口打开节点选择面板，搜索并添加工具节点或条件节点，不破坏现有拖拽添加能力。
* [ ] 节点右侧不再显示快捷 `+` 添加入口，source handle 不被遮挡且连线拖拽稳定。
* [ ] 条件节点按 case/default 分支显示独立出口，用户可从具体分支 handle 拖线，连线自动携带并显示对应 case/default。
* [ ] EdgeInspector 修改条件连线 case 后同步 `sourceHandle`、`label`、`edge.data.case`，保存/加载旧边时尽量从 `case` 补齐 `sourceHandle`。
* [ ] 底部悬浮工具条可执行常用画布操作，至少包含缩放/适配/添加节点入口，并保留现有运行工作流入口。
* [ ] 列表、执行配置、结果区、插件弹窗、编排器区域的圆角、边框、阴影、间距与 DESIGN 一致。
* [ ] 仅保留 DESIGN 允许的功能色：链接蓝、警告、危险、焦点等语义用途。
* [ ] 不改变 `web/src/main.jsx` 的业务行为；如需调整 JSX，仅限 class/轻量结构服务于视觉。
* [ ] `npm run build --prefix web` 通过。
* [ ] 修改代码后按项目要求运行 graphify rebuild。

## Definition of Done (team quality bar)

* Tests added/updated where appropriate; for pure CSS visual fix, build verification is sufficient.
* Lint / typecheck / CI-relevant build green.
* Docs/notes updated only if behavior changes.
* Rollback considered if risky; this task should be CSS-first and低风险。

## Out of Scope (explicit)

* 不改后端 API、插件加载、工作流执行语义。
* 不引入新的 UI 组件库或依赖。
* 不重新设计信息架构或交互流程。
* 不新增产品功能。
* 不做大规模组件拆分或状态管理重构。

## Technical Approach

CSS-first visual reset: keep `web/src/main.jsx` behavior unchanged, consolidate design values into CSS variables, and restyle existing selectors to align with `DESIGN.md`.

* Use Cloud Gray / Pure White / Expo Black / Slate Gray as the dominant system.
* Keep Link Cobalt, focus blue, warning, and danger colors only for semantic states.
* Remove decorative purple, gradients, and colored shadows from workflow editor control cards, condition nodes, canvas labels, controls, and minimap.
* Use condition case IDs as React Flow `sourceHandle` values so canvas edges originate from visible case/default branch rows while persisted workflow edges continue to use the existing `{from, to, case}` shape.
* When loading old workflow edges with `case`, hydrate `sourceHandle` and label from the source condition node; if an old condition edge lacks `case`, keep it empty so existing preflight validation reports the missing branch.
* Edge case edits must update `sourceHandle`, `label`, and `data.case` together to avoid canvas/persisted data drift.

## Decision (ADR-lite)

**Context**: The current UI was mostly close to `DESIGN.md`, but the workflow editor and canvas controls used decorative purple, gradients, and heavier color shadows that conflicted with the design system.

**Decision**: Apply a CSS-only full visual-layer reset, including canvas internal controls, without changing React state, API calls, node mapping, save/run behavior, or Chinese copy.

**Consequences**: Low behavioral risk and easy rollback. Visual consistency improves across the full console, but browser visual QA is still recommended for pixel-level review.

## Validation

* `npm run build --prefix web` passed after installing frontend dependencies and using F: drive npm cache because C: drive was full.
* `git diff --check` passed with only Git LF→CRLF warning for `web/src/styles.css`.
* `python3 -c "from graphify.watch import _rebuild_code; from pathlib import Path; _rebuild_code(Path('.'))"` completed successfully.
* Implemented condition branch independent exits: case/default rows render separate React Flow `sourceHandle`s, new condition edges inherit `data.case`/label from the dragged branch, EdgeInspector keeps `sourceHandle`/label/case synchronized, and workflow edge load/save remains compatible with the existing `{from,to,case}` contract.

* Inspected `web/src/main.jsx`: React 单页控制台，包含工具/工作流列表、执行配置、插件管理、工作流编排器。
* Inspected `web/src/styles.css`: 当前风格入口；主要问题位于 `.controlPaletteItem`、`.controlIcon`、`.controlTitle span`、`.conditionNode`、`.conditionDiamond`、`.conditionInfoCard`、`.conditionNodeHeader > span`、`.conditionNode small`、`.react-flow__edge-text` 等选择器。
* Build command: `npm run build --prefix web`.
* Project requires after code modification: `python3 -c "from graphify.watch import _rebuild_code; from pathlib import Path; _rebuild_code(Path('.'))"`.

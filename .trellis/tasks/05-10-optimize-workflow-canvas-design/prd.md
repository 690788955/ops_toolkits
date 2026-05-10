# 优化工作流画布设计

## Goal

优化 Web 工作流编排器的画布体验，让运维同事在插件工具、条件分支、并行/合流/循环节点之间编排 DAG 时更容易读懂、添加、配置、运行和排错，同时保留当前 `@xyflow/react` 技术栈，不重写底层画布。

## What I already know

* 用户希望参考 ProductFlow 的画布设计来优化本项目画布。
* ProductFlow 值得借鉴的是卡片化节点、画布内添加入口、运行状态表达和清晰的信息层级；不适合照搬其自研 React DOM + SVG 画布底层。
* 当前项目已经使用 `@xyflow/react`，入口在 `web/src/main.jsx`，样式在 `web/src/styles.css`。
* 当前工作流编辑器已有：插件工具/编排节点双 Tab 面板、拖拽到画布、画布内节点选择器、MiniMap、Controls、Background、自动排版、执行前校验、执行状态覆盖层、节点/边配置弹窗。
* 当前相关规格：`.trellis/spec/frontend/workflow-editor-condition-controls.md` 约束了紧凑卡片、条件分支可见、节点配置弹窗、运行覆盖层和自动排版不写入保存草稿。

## Assumptions (temporary)

* 本轮优化以 UX/交互设计和前端实现为主，尽量不改变后端 workflow schema。
* 继续使用 XyFlow 提供的节点、边、缩放、平移、框选、MiniMap 和 Controls 能力。
* 优先做低风险、高收益改造，而不是大规模重构 `web/src/main.jsx`。

## Open Questions

* None — user confirmed the MVP scope.

## Requirements

* 保留 `@xyflow/react` 作为底层画布框架。
* 借鉴 ProductFlow 的卡片画布思路，但节点始终保持可扫读、不过载。
* MVP 选择视觉/可读性优先 + 编排效率优先：优化节点卡片信息层级、画布内添加入口、节点面板/节点选择器可用性、canvas dock 操作表达和自动排版入口。
* 工具节点默认态显示轻量参数配置状态，例如参数数量、已配置数量、缺失必填数量；详细参数仍在配置弹窗中编辑。
* 参数状态提示必须保持紧凑，不能把节点卡片变成参数表或日志面板。
* 增加“连线后快速添加下游节点”能力：用户从节点发起下游编排时，可以在目标位置快速打开节点选择器并创建后继节点，降低往返左侧面板的成本。
* 快速添加下游节点应复用现有节点选择器、节点创建和连线逻辑，不新增画布底层框架。
* 节点卡片视觉采用紧凑运维工具风：信息密度优先、尺寸克制，适合大 DAG 扫读；避免 ProductFlow 式大留白导致画布承载量下降。
* 运行状态仍是临时覆盖层，不写入工作流 YAML/保存草稿。
* 条件分支的分支行、分支 handle、case edge label 必须保持可见和可理解。

## Acceptance Criteria (evolving)

* [ ] 画布节点的信息层级更清晰，工具节点/条件节点/控制节点在默认状态下能快速区分。
* [ ] 工具节点默认态能表达名称、类型/来源和轻量参数配置状态；详细 ID/说明/参数明细仍通过 hover/selection/modal 渐进展示。
* [ ] 添加节点入口更顺手，不依赖单一拖拽路径；空画布、已有画布和连线后添加下游节点都能复用节点选择器。
* [ ] 节点面板/节点选择器的搜索、分类和空状态文案更清楚。
* [ ] 自动排版、适配视图、运行工作流等画布 dock 操作更易理解且不遮挡主要节点。
* [ ] 不引入新的画布底层库，不用自研替代 XyFlow。
* [ ] 不把运行状态、布局临时信息或 React Flow 私有字段写入 workflow 保存数据。

## Definition of Done (team quality bar)

* Tests added/updated where appropriate.
* Lint / typecheck / build pass.
* Web UI change is manually verified in browser for golden path and key edge cases.
* Docs/spec notes updated if behavior or conventions change.
* Rollout/rollback considered if risky.

## Research References

* [`research/canvas-ux-patterns.md`](research/canvas-ux-patterns.md) — 对 ProductFlow 卡片画布、XyFlow、n8n、Node-RED 风格的可借鉴点和禁忌做了对比。

## Research Notes

### What similar tools do

* ProductFlow-style card canvas emphasizes readable cards, progressive disclosure, empty-state guidance and canvas-first onboarding.
* XyFlow best practice is用自定义节点/边/Handle/EdgeLabelRenderer/Controls/Background/MiniMap扩展体验，而不是重写底层拖拽缩放逻辑。
* n8n/Node-RED 类自动化编辑器通常把节点保持紧凑，把重配置和调试详情放到弹窗、侧栏或执行结果区域。

### Constraints from our repo/project

* 当前前端集中在 `web/src/main.jsx` 和 `web/src/styles.css`，目前没有拆分组件目录。
* `.trellis/spec/frontend/workflow-editor-condition-controls.md` 已明确：节点卡片要紧凑，分支行例外地保持可见；节点配置优先使用 modal；运行覆盖层和自动布局是前端状态。
* 后端条件/控制节点语义已有规格，前端优化不能破坏 case label、loop 嵌入工具、安全确认扫描等约束。

### Feasible approaches here

**Approach A: 节点卡片与画布微交互优化（Recommended）**

* How it works: 保留现有布局和弹窗，优化节点卡片信息层级、状态 badge、hover/selected 展示、空画布提示、canvas dock 文案和样式。
* Pros: 风险最低，最贴近 ProductFlow 可借鉴部分，通常只涉及前端 JSX/CSS。
* Cons: 对参数配置效率提升有限，更多是观感和可读性提升。

**Approach B: 配置/调试体验优化**

* How it works: 保留节点卡片紧凑，把节点配置、边配置、执行结果的入口和反馈做得更清楚，例如失败后快速打开对应节点配置/日志，边点击更明显。
* Pros: 更直接提升实际编排和排错效率。
* Cons: 涉及状态流和结果面板联动，测试面更广。

**Approach C: 大画布效率优化**

* How it works: 强化节点搜索插入、自动排版、边可读性、大图导航和快捷操作；必要时以后评估 dagre/elk，但本任务先不引入。
* Pros: 适合节点很多的真实运维 DAG。
* Cons: 如果当前工作流规模不大，收益可能不如 A/B 直观。

## Expansion Sweep

### Future evolution

* 将来可能需要更复杂的节点类型、注释/分组、执行历史回放或部分执行；本任务应保留 XyFlow 扩展点。
* 如果插件工具数量增长，节点面板搜索、标签、分类和画布内插入会变得更重要。

### Related scenarios

* 新建、加载、保存、校验、执行、失败定位要保持同一套状态语言。
* CLI/API/后端 workflow schema 仍是权威，前端画布只是编辑和可视化入口。

### Failure & edge cases

* 参数 JSON 无效、必填参数缺失、条件边重复、节点不属于当前分类、运行失败/跳过，都需要在画布上有明确反馈。
* 画布优化不能让条件分支连接点难以发现，也不能让删除/运行按钮误触。

## Out of Scope (explicit)

* 不重写为 ProductFlow 的自研 DOM+SVG 画布。
* 不替换 `@xyflow/react`。
* 不改变 workflow YAML/backend schema，除非后续明确需要。
* 不一次性引入 sticky notes、执行历史回放、凭据管理、局部执行等 n8n 级功能。
* 不做大规模文件拆分，除非实施阶段发现必须拆分才能安全维护。

## Technical Approach

Keep XyFlow as the canvas engine and implement this as a focused frontend UX pass:

* Extend the existing custom node data/rendering so tool nodes can compute and display compact parameter status from catalog tool metadata and node params.
* Reuse the existing `NodePickerPanel`, node creation helpers, and `onConnect`/edge creation behavior for a new downstream quick-add path instead of adding a separate picker implementation.
* Tighten card/dock/picker styling in `web/src/styles.css` while preserving the existing compact workflow-editor contract.
* Keep all new visual state frontend-only; saved workflow drafts must still be produced only by `buildWorkflowDraft` using the existing schema.

## Decision (ADR-lite)

**Context**: The project already uses `@xyflow/react` and has a working workflow editor; the requested improvement is ProductFlow-inspired canvas UX, not a new canvas engine.

**Decision**: Keep XyFlow and optimize the current editor with compact ops-oriented node cards, light parameter status, and downstream quick-add interactions.

**Consequences**: This avoids high-risk canvas rewrites and preserves existing schema/runtime contracts. The trade-off is that ProductFlow-style large-card aesthetics are intentionally not adopted because this ops DAG editor needs higher density.

## Implementation Plan

* Update tool node data preparation to include lightweight parameter status derived from tool definitions and node params.
* Update `ToolNode` rendering and CSS to show compact parameter state without turning nodes into parameter tables.
* Add a downstream quick-add flow that opens the existing node picker at the intended target position and connects the chosen new node to the source node.
* Polish node picker / empty state / canvas dock copy and styling for compact workflow use.
* Verify save/load/build/run overlay boundaries remain unchanged.

## Technical Notes

* Current canvas editor: `web/src/main.jsx` around `WorkflowEditor`, `ToolNode`, `ConditionNode`, `ControlNode`, `CanvasDock`, `NodePickerPanel`, `buildDisplayNodes`, `buildDisplayEdges`, `autoLayoutNodes`.
* Current canvas styles: `web/src/styles.css` around `.canvasCard`, `.toolNode`, `.conditionNode`, `.controlNode`, `.runStatusBadge`, `.react-flow__edge*`, `.nodePickerPanel`, `.canvasDock`.
* Current dependency: `@xyflow/react` `^12.10.2`.

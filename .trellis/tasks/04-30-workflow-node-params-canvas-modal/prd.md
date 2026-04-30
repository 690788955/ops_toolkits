# workflow node params on canvas modal

## Goal

将工作流节点参数编辑从当前右侧检查器，优化为更贴近画布的交互：点击画布节点后可以打开参数编辑模态框，让用户在画布上下文中完成节点参数配置，减少左右视线跳转。

## What I already know

* 用户希望“节点参数弄到画布里面”，倾向“点击出现模态框”。
* 当前工作流编辑器在 `web/src/main.jsx` 的 `WorkflowEditor` 中实现。
* 当前点击节点只设置 `selectedNodeID`，参数编辑显示在右侧 `.nodeInspector` 区域。
* 工具节点参数编辑由 `ParamMappingEditor` 负责，支持声明参数、上游输出映射、手动输入。
* 高级 JSON 参数编辑也在右侧检查器内，通过 `nodeParamsText` 和 `applyNodeParams()` 应用。
* 条件节点使用 `ConditionEditor`，不是普通工具参数，但也属于节点配置。
* 已有通用 `.modalBackdrop` / `.modal` / `.modalHeader` 样式，可复用，不需要引入新 UI 依赖。

## Assumptions (temporary)

* MVP 先处理画布节点点击后的配置入口，不改变工作流 YAML 数据结构。
* 参数保存仍复用现有 `updateSelectedNodeParams` / `updateMappedParam` 行为。
* 不引入新的路由、状态管理库或第三方弹窗组件。

## Open Questions

* None.

## Requirements (evolving)

* 采用方案 A：点击画布节点直接打开节点配置模态框。
* 工具节点参数可以在画布上下文中通过模态框编辑。
* 条件节点配置也在同一个节点配置模态框中编辑，保持工具节点与编排节点交互一致。
* 模态框内复用现有参数映射能力：手动输入、选择工作流参数、选择上游节点输出。
* 高级 JSON 编辑能力保留，但不作为默认主路径。
* 关闭模态框不应丢失已即时应用的参数变更。
* 保存、校验、执行工作流仍使用现有 draft 构建逻辑。

## Acceptance Criteria (evolving)

* [ ] 点击画布工具节点直接打开节点配置模态框。
* [ ] 点击画布条件节点直接打开节点配置模态框。
* [ ] 工具节点参数在模态框内可编辑，保存到节点 `data.params`。
* [ ] 条件节点配置在模态框内可编辑，保存到节点 `data.condition`。
* [ ] 参数映射来源仍包含工作流参数和上游节点输出。
* [ ] 无参数工具显示“无需参数/未声明参数”的空态。
* [ ] 删除节点、连线编辑、节点选择行为不被破坏。
* [ ] `npm run build --prefix web` 成功。
* [ ] 修改代码后运行 graphify rebuild。

## Definition of Done (team quality bar)

* Tests added/updated where appropriate.
* Lint / typecheck / build green.
* Docs/notes updated if behavior changes.
* Rollout/rollback considered if risky.

## Research Notes

### Constraints from repo/project

* 前端集中在 `web/src/main.jsx` 和 `web/src/styles.css`，当前实现偏单文件应用。
* 已有插件管理/导出模态框样式，适合 DRY 复用。
* 画布使用 `@xyflow/react`，已有 `NodePickerPanel` 作为画布内浮层模式。
* 现有右侧 `nodeInspector` 同时承担节点参数、条件节点配置、连线配置和校验摘要展示。

### Feasible approaches here

**Approach A: 点击节点直接打开参数模态框（推荐）**

* How it works: `onNodeClick` 选择节点并打开节点配置模态框；工具节点显示参数映射，条件节点可显示条件编辑。
* Pros: 符合用户描述，路径最短，画布中心化体验强。
* Cons: 每次点击都弹窗，可能影响只是想选中/连线的用户。

**Approach B: 点击节点只选中，节点卡片内新增“参数”按钮打开模态框**

* How it works: 节点选中后在节点卡片上出现配置按钮；点击按钮打开模态框。
* Pros: 不干扰拖拽、连线、选择；误触少。
* Cons: 比用户设想多一步，节点 UI 更复杂。

**Approach C: 保留右侧检查器，同时新增画布内弹窗快捷入口**

* How it works: 右侧检查器继续存在；节点双击或按钮打开同一套参数编辑模态框。
* Pros: 兼容旧习惯，风险最低。
* Cons: 两个入口可能重复，后续维护成本略高。

## Decision (ADR-lite)

**Context**: 当前节点参数编辑位于右侧检查器，用户希望参数配置更贴近画布，并明确倾向点击节点后出现模态框。

**Decision**: 采用 Approach A：点击画布节点直接打开节点配置模态框；工具节点显示参数映射和高级 JSON 编辑，条件节点显示条件配置编辑。

**Consequences**: 交互路径最短、符合用户预期；代价是点击节点会从“仅选中”变为“选中并打开配置”，需要确保拖拽、连线和删除按钮不误触弹窗。

## Expansion Sweep

### Future evolution

* 后续可扩展为节点运行结果、参数校验错误、执行日志都在节点弹窗中查看。
* 可为复杂节点配置保留 tab 结构：参数、条件、JSON、运行结果。

### Related scenarios

* 条件节点配置应与工具节点参数入口保持一致，否则用户会困惑。
* 连线编辑仍适合保持轻量，不建议塞进节点参数模态框。

### Failure & edge cases

* 弹窗打开时节点被删除，需要安全关闭。
* 参数 JSON 编辑失败时应保留错误提示，不应写入坏数据。
* 无参数工具、缺失工具定义、上游映射为空都需要明确空态。

## Out of Scope (explicit)

* 不改变后端工作流 schema。
* 不改变 runner 参数解析行为。
* 不引入新的 UI 组件库。
* 不做完整移动端适配重构。

## Technical Notes

* `web/src/main.jsx:513` — `WorkflowEditor` 状态与 ReactFlow 入口。
* `web/src/main.jsx:804` — `applyNodeParams()` 处理高级 JSON 应用。
* `web/src/main.jsx:819` — `updateMappedParam()` 即时更新节点参数。
* `web/src/main.jsx:1059` — `ReactFlow` 画布与 `onNodeClick`。
* `web/src/main.jsx:1104` — 现有右侧 `nodeInspector`。
* `web/src/main.jsx:1320` — `ParamMappingEditor`。
* `web/src/styles.css:355` — 现有模态框样式。
* `web/src/styles.css:439` — 现有编辑器布局。

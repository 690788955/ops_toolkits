# brainstorm: 优化编排节点和节点参数输入框样式

## Goal

优化工作流编辑器的编排节点视觉和节点参数表单体验：让“条件分支”控制节点与画布上的条件节点更简约，降低装饰密度；同时放松节点参数输入框布局，解决当前输入区域过于紧凑的问题。

## What I already know

* 用户希望“编排节点”的样式优化，尤其是“条件分支”更简约。
* 用户希望“节点参数”的输入框样式优化，当前感觉太紧凑。
* 相关前端文件集中在 `web/src/main.jsx` 和 `web/src/styles.css`。
* 当前编排节点卡片使用 `.controlPaletteItem`、`.controlIcon`、`.capabilityChips`、`.controlPreview`、`.controlContent em` 等较多视觉信息。
* 当前画布条件节点 `.conditionNode` 使用三列布局、菱形图标、虚线内框、信息卡、分支列表，视觉较重。
* 节点参数区域使用 `.form compact`、`.paramMappings`、`.mappingRow`、`.conditionEditor`、`.caseEditor`；映射行目前把标签、select、input 横向压在一行。
* 适用规范：`.trellis/spec/frontend/workflow-editor-condition-controls.md` 要求条件节点仍需与工具节点视觉区分，并保留条件输入、case、默认分支、连线 case 标签等能力。

## Assumptions (temporary)

* 本任务只做 UI/CSS 与轻量结构调整，不改变工作流保存格式、运行逻辑或后端接口。
* “简约”优先指减少装饰元素和信息密度，而不是移除必要配置能力。
* “输入框不紧凑”优先处理节点参数面板中的映射行、JSON textarea、条件编辑器 case 输入区域。

## Open Questions

* 样式方向需要选择：极简列表式、卡片留白式、还是保留当前结构只减弱装饰？

## Requirements (evolving)

* 编排节点的“条件分支”卡片更简约，减少视觉噪音。
* 画布上的条件分支节点更轻量，但仍明显区别于工具节点。
* 节点参数输入区域增加间距和可读性，避免 select/input/textarea 过度挤压。
* 保留现有条件分支功能：输入来源、case 编辑、默认分支、连线 case 标签、保存/加载契约不变。

## Acceptance Criteria (evolving)

* [ ] `编排节点` 面板中的“条件分支”卡片更简洁，规划中节点仍显示禁用状态。
* [ ] 画布条件节点保留名称、输入摘要、case/default 分支信息，但减少装饰边框/图标重量。
* [ ] 节点参数面板中的参数映射行在窄面板内不拥挤，select/input 有更清晰的纵向节奏。
* [ ] 条件编辑器中的 case 输入区域更舒展，textarea 可读性提升。
* [ ] 不改变 `buildWorkflowDraft`、`workflowNodeToFlowNode`、edge case round-trip 行为。
* [ ] `npm run build --prefix web` 通过。

## Definition of Done

* 前端生产构建通过。
* 必要时更新嵌入资源。
* graphify 重建。
* 不引入新的后端行为或数据契约变更。

## Out of Scope (explicit)

* 不重做整个工作流编辑器布局。
* 不改变条件分支运行语义。
* 不新增新的编排节点类型。
* 不新增前端依赖。

## Technical Notes

* `web/src/main.jsx` around `controlNodeCatalog`, `ConditionNode`, `ConditionEditor`, `ParamMappingEditor` controls rendered content.
* `web/src/styles.css` around `.controlPaletteItem`, `.conditionNode`, `.conditionDiamond`, `.conditionInfoCard`, `.conditionBranchList`, `.paramMappings`, `.mappingRow`, `.nodeInspector textarea` controls styling.
* Existing task `04-30-plugin-export-modal` still has uncommitted UI/build changes; implementation should avoid mixing concerns where possible.
* Current working tree also shows `plugins/user.workflows/plugin.yaml` modified before this task; do not touch it unless explicitly required.

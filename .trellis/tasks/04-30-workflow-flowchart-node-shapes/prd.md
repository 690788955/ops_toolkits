# brainstorm: workflow industry flowchart node shapes

## Goal

将工作流画布节点样式调整为更符合行业流程图/工作流编辑器认知的形状，让用户能通过形状快速区分工具步骤、条件判断、并行分支和合流，同时避免回到过度装饰、难读的复杂 UI。

## What I already know

* 用户希望“他们的样式做成流程图那种行业的形状”。
* 当前节点类型包括：工具节点、条件分支、并行分支、合流；循环仍是规划中禁用。
* 当前前端节点组件集中在 `web/src/main.jsx`：`ToolNode`、`ConditionNode`、`ControlNode`。
* 当前样式集中在 `web/src/styles.css`：`.toolNode`、`.conditionNode`、`.controlNode`、`.conditionBranch*`。
* 最近已做过一次简化：去掉了条件节点的菱形重装饰，改成轻量卡片。
* 现有规范要求 lightweight，避免 heavy decorative shapes / nested frame effects；因此行业形状需要“语义化但克制”。

## Assumptions (temporary)

* 目标不是完全 BPMN 建模工具，而是让运维工作流画布更像专业流程图。
* 形状应主要靠 CSS 实现，尽量不引入 SVG 依赖或改变保存/运行契约。
* 节点连接点和可读性优先于纯粹形状还原。

## Open Questions

* None. User confirmed Approach B: Hybrid professional flowchart style.

## Requirements (evolving)

* 采用混合专业流程图风格：主体保持可读卡片，使用行业形状徽标表达语义。
* 工具节点保持“处理步骤/Process”语义，可使用圆角矩形。
* 条件分支表达“Decision”语义，可使用菱形或菱形徽标。
* 并行分支/合流表达“Gateway”语义，可使用菱形 + `+` / 汇合标记，或轻量 gateway 徽标。
* 不改变节点 payload、保存/加载、运行、校验契约。
* `loop` 继续保持规划中禁用。
* 生产构建必须通过：`npm run build --prefix web`。
* 修改代码后运行 graphify rebuild。

## Acceptance Criteria (evolving)

* [ ] 用户能通过形状区分工具、条件、并行、合流。
* [ ] 节点内容仍可读，分支连接点仍易点击。
* [ ] 不破坏 node config modal、edge modal、草稿运行。
* [ ] disabled/planned 节点仍不可点击/拖拽。
* [ ] `npm run build --prefix web` 通过。

## Definition of Done (team quality bar)

* Tests/build green.
* Docs/spec updated if behavior or style contract changes.
* Rollout/rollback considered if risky.

## Out of Scope (explicit)

* 不引入完整 BPMN 引擎。
* 不实现循环节点。
* 不改变 workflow YAML/JSON schema。
* 不引入新的 UI/SVG 图形库。

## Technical Notes

* `web/src/main.jsx:68` `ToolNode` currently renders a rectangular card.
* `web/src/main.jsx:81` `ConditionNode` renders condition summary and branch rows.
* `web/src/main.jsx:123` `ControlNode` renders parallel/join controls.
* `web/src/styles.css` has node styles around `.toolNode`, `.conditionNode`, `.controlNode` and branch handle styles.

## Research References

* Pending: `research/flowchart-shapes.md`

## Decision (ADR-lite)

**Context**: 用户希望节点更像行业流程图形状，但当前节点需要承载中文标题、运行状态、条件输入、case/default 分支和连接点。严格菱形会牺牲可读性与交互命中区。

**Decision**: 采用 Approach B：Hybrid professional flowchart style。工具节点保持流程矩形；条件、并行、合流保持可读卡片主体，但加入流程图/BPMN-like 的 decision/gateway 形状徽标。

**Consequences**: 视觉更接近行业流程图，同时不改变 workflow schema、runner 语义、保存/加载和草稿运行契约。后续若做完整 BPMN-like 模式，可在此基础上扩展。


**Approach A: Strict flowchart geometry**

* 工具节点：矩形；条件节点：完整菱形；并行/合流：gateway 菱形。
* Pros: 最像行业流程图。
* Cons: 文本空间受限，条件分支列表和连接点会更难排版，容易再次复杂。

**Approach B: Hybrid professional flowchart style（推荐）**

* 工具节点：圆角流程矩形；条件/并行/合流：保持可读卡片主体，但左侧/顶部使用行业形状徽标（decision diamond、parallel gateway `+`、join gateway）。
* Pros: 保留行业识别，又不牺牲可读性和点击区域。
* Cons: 不如严格几何形状“纯”。

**Approach C: Minimal icon-coded style**

* 所有节点仍为卡片，只用小 icon/chip 表达类型。
* Pros: 最稳、最简洁。
* Cons: 不够“流程图行业形状”。

## Expansion Sweep

### Future evolution

* 后续若做 BPMN-like 模式，可以基于同一类型徽标扩展 start/end、loop、subprocess。
* 可保留 CSS class 作为主题切换入口，例如 compact / flowchart。

### Related scenarios

* palette 中的编排节点预览形状应与画布节点一致，避免拖入后认知落差。
* 节点配置 modal 不需要复刻形状，只需展示节点类型即可。

### Failure & edge cases

* 完整菱形会压缩中文标题和 case 列表，影响可读性。
* React Flow handle 位置如果跟形状边缘不匹配，会导致连线看起来偏移。

# brainstorm: workflow auto layout

## Goal

在工作流编辑器中增加“一键优化排版”操作，让用户画完或加载 workflow 后可以自动整理节点位置，减少手工拖拽整理成本。

## What I already know

* 用户要求“加一个一键优化工作流排版的操作”。
* 当前前端使用 `@xyflow/react`，依赖里没有 dagre/elk 等自动布局库。
* 当前节点新增/加载位置是简单递增坐标：`x: 80 + index * 220`, `y: 120 + (index % 3) * 90`。
* 当前已有 `flowInstance?.fitView({padding: 0.2, duration: 240})`，可在布局后自动缩放视图。
* 工作流保存 draft 当前只保存 node schema/edges，不保存 canvas position；自动布局只影响当前画布状态，不改变后端 schema。
* 当前节点类型包括 tool/condition/parallel/join/loop，连线仍是 DAG，loop 不通过普通 edge 形成真实环。

## Assumptions (temporary)

* MVP 不引入新依赖，先实现一个简单稳定的 DAG 分层布局。
* 默认布局方向采用从左到右，符合现有 source/right 与 target/left handle 方向。
* 用户可以在自动布局后继续手工拖拽调整。

## Open Questions

* None. User confirmed Approach A: built-in left-to-right DAG layer layout.

## Requirements (evolving)

* 在工作流编辑器操作区增加“一键排版/优化排版”按钮。
* 点击后根据当前 nodes/edges 重新计算 position。
* 布局应尽量：上游在左，下游在右；同层节点纵向分布；减少重叠。
* 支持 tool/condition/parallel/join/loop 所有节点类型。
* 布局后调用 fitView，让用户看到完整流程。
* 不改变 workflow schema、保存/运行/校验逻辑。
* 空画布或单节点时也要安全处理。
* 生产构建通过：`npm run build --prefix web`。
* 修改代码后运行 graphify rebuild。

## Acceptance Criteria (evolving)

* [ ] 用户可点击按钮自动整理当前画布。
* [ ] 节点不会重叠，主要流程方向从左到右。
* [ ] 条件分支、并行、合流、循环节点都参与布局。
* [ ] 布局后不丢失节点 data、selected state 不导致崩溃。
* [ ] `npm run build --prefix web` 通过。

## Definition of Done (team quality bar)

* Build green.
* Existing behavior preserved.
* Specs updated if UI contract changes.
* Rollback considered: auto layout only mutates frontend positions.

## Out of Scope (explicit)

* 不引入 dagre/elk 等新布局依赖。
* 不实现可配置布局方向/间距 UI。
* 不持久化节点坐标到后端 schema。
* 不做复杂边避让或正交路由。

## Technical Notes

* `web/src/main.jsx` has `nodes`, `edges`, `setNodes`, `flowInstance`, and current `fitCanvas()` at around the ReactFlow editor.
* New helper can compute ranks from edges using topological-style layering.
* Since backend already rejects cycles, frontend can still handle unexpected cycles gracefully by falling back to current order.

## Decision (ADR-lite)

**Context**: The editor needs a one-click layout action, but the web app currently has no graph layout dependency and node positions are frontend-only.

**Decision**: Implement Approach A: a built-in left-to-right DAG layer layout. It computes node depths from current edges, assigns x by layer and y by order within the layer, then calls fitView.

**Consequences**: No dependency or schema changes; layout quality is predictable but not as advanced as dagre/elk, and edge crossings may remain.


**Approach A: Built-in simple DAG layer layout（推荐 MVP）**

* How: compute node depth from incoming edges, group by depth, assign x by depth and y by order in layer.
* Pros: No new dependency, predictable, small code, enough for current DAG editor.
* Cons: Not as polished as graph layout libraries; edge crossings may remain.

**Approach B: Add dagre/elk layout library**

* How: install layout dependency and compute graph layout.
* Pros: Better layout quality.
* Cons: Adds dependency and bundle weight; more integration risk.

**Approach C: Only fit view / distribute grid**

* How: no graph analysis, just re-grid nodes.
* Pros: Very simple.
* Cons: Does not reflect workflow direction or dependencies.

## Expansion Sweep

### Future evolution

* Later can add vertical layout, compact/spacious spacing presets, or persist positions.
* If workflows become large, a proper layout library may be worth adding.

### Related scenarios

* After loading workflow or generating example workflow, user can click one button to make it readable.
* Auto layout should not trigger save automatically.

### Failure & edge cases

* Missing edge endpoints should be ignored in layout calculation.
* If a cycle appears client-side before validation, layout should not hang.

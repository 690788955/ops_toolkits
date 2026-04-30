# brainstorm: workflow loop node runtime

## Goal

实现工作流 `循环` 编排节点功能，让用户可以在画布中配置一个安全、可验证、可运行的循环控制节点，而不是仅作为规划中占位。

## What I already know

* 用户明确说“我要实现功能”。
* 当前后端支持 `tool`、`condition`、`parallel`、`join` 节点类型。
* 当前 `registry.OrderWorkflow` 是 DAG 拓扑排序，遇到图上的环会报“工作流存在环形依赖”。
* 当前 runner 按拓扑顺序执行节点，condition 用 edge case 激活分支，parallel/join 作为控制节点记录 succeeded。
* 当前前端 `controlNodeCatalog` 中 `loop` 仍 disabled，并且 `addControlNode` 明确拒绝 loop。
* 当前 frontend preflight 会阻止 loop 保存/运行。
* 当前 workflow schema `WorkflowNode` 没有 loop 配置字段。

## Assumptions (temporary)

* 不应通过允许 React Flow 边形成真实环来实现循环，因为现有 DAG 校验、拓扑排序、可视化编辑和日志都会复杂化。
* MVP 应使用一个可配置的 loop 控制节点，在 DAG 中作为普通节点存在，但 runner 对其引用的目标节点执行有限次数。
* 循环必须有最大次数保护，防止 runaway workflow。

## Open Questions

* None. User confirmed Approach A: fixed-count loop over one target tool node.

## Requirements (evolving)

* `loop` 从规划占位升级为可添加、可保存、可校验、可运行的编排节点。
* MVP 采用固定次数循环：loop 配置一个目标工具节点 ID 和最大循环次数。
* 不允许通过普通 workflow edges 形成真实环；graph 仍应保持 DAG。
* 前端必须提供 loop 配置 modal，支持用户选择循环目标/次数/条件。
* 后端必须校验 loop 配置并拒绝危险或无效配置。
* Runner 必须输出可解释的循环执行日志。
* 草稿运行必须支持 loop。

## Acceptance Criteria (evolving)

* [ ] 用户可以从编排节点面板添加循环节点。
* [ ] 用户可以配置 loop，并保存/重新加载不丢失。
* [ ] 校验能阻止无目标、无上限或非法 loop 配置。
* [ ] 运行记录能展示 loop 节点及迭代结果。
* [ ] 不破坏 tool/condition/parallel/join。
* [ ] `GOTOOLCHAIN=local go test ./...` 通过。
* [ ] `npm run build --prefix web` 通过。

## Definition of Done (team quality bar)

* Tests added/updated where appropriate.
* Lint / typecheck / build green.
* Docs/spec updated for cross-layer contract.
* Rollout/rollback considered if risky.

## Out of Scope (explicit)

* 不在 MVP 中允许画布普通边形成任意环。
* 不实现无限循环。
* 不引入独立工作流引擎或 BPMN 引擎。

## Technical Notes

* `internal/config/types.go` needs a loop config field if loop is implemented.
* `internal/registry/validate.go` currently rejects unknown node types and validates control connectivity.
* `internal/runner/runner.go` currently uses DAG order and would need explicit loop execution semantics.
* `web/src/main.jsx` currently blocks loop in `addControlNode` and `validateControlDraft`.

## Decision (ADR-lite)

**Context**: Current workflow execution is DAG-based. Allowing arbitrary graph cycles would require replacing topological ordering, redesigning validation, and adding stronger runaway protection.

**Decision**: Implement fixed-count loop MVP. A `loop` node references one target tool node and a bounded iteration count. The visual graph remains DAG; the runner repeats the target tool when executing the loop node.

**Consequences**: This delivers useful loop behavior safely without introducing arbitrary cycles or nested subgraphs. Condition-driven and subgraph loops remain future work.


**Approach A: Fixed-count loop over one target node（推荐 MVP）**

* How: `loop` node config references one target node ID and `max_iterations`; runner executes that target node N times when loop node runs.
* Pros: Smallest safe loop semantics, easy to validate, easy to log, no graph cycle.
* Cons: Cannot express “loop until condition” yet.

**Approach B: Condition-driven loop over one target node**

* How: loop has input template/operator/values/max_iterations and repeats target while condition matches.
* Pros: More useful for real workflows.
* Cons: More validation/UI/logging complexity; easy to confuse with condition node.

**Approach C: Loop subgraph block**

* How: loop owns a body subgraph or edge range and repeats the body.
* Pros: Most expressive.
* Cons: High complexity; needs nested graph semantics and much more UI.

## Expansion Sweep

### Future evolution

* Could extend fixed-count loop to condition-driven loop after logging and schema stabilize.
* Could later support subgraph loop/body if workflow editor grows nested canvas concepts.

### Related scenarios

* Draft run must support loop because user wants canvas testing before save.
* Help/menu/run logs need readable loop iteration display.

### Failure & edge cases

* Missing target node, target points to another control node, or max_iterations too high must be rejected.
* Tool failure inside an iteration should stop loop and mark workflow failed unless future failure policy exists.

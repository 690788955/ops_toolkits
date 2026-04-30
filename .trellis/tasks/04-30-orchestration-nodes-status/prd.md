# brainstorm: orchestration nodes implementation status

## Goal

确认并规划编排节点能力：用户问“编排节点的其他工具都实现了吧”，需要先说明当前实现状态，再决定是否推进并行分支、合流、循环等控制节点。

## What I already know

* 当前前端 `web/src/main.jsx` 的编排节点面板包含：条件分支、并行分支、合流、循环。
* 当前只有 `条件分支` 是 enabled，可点击和拖拽到画布。
* `并行分支`、`合流`、`循环` 在 UI 中标记为 disabled/planned，不注册点击和拖拽处理。
* 后端/runner/config/registry 当前已实现并测试 `type: condition`、case edge、default_case。
* 搜索后未发现 parallel/join/loop 对应的后端 schema、runner 语义或校验实现。

## Assumptions (temporary)

* 用户可能希望确认“是否已经实现”，也可能希望继续实现其他编排节点。
* 其他编排节点需要跨前端、后端 schema、校验、runner 执行语义，不只是打开 UI 卡片。

## Open Questions

* None.

## Requirements (evolving)

* 保留当前条件分支已实现能力。
* 实现 `并行分支` 与 `合流`，不只是前端解禁，必须有后端/runtime/schema 合约。
* 暂不实现 `循环`，继续作为规划占位，避免引入无限循环和复杂运行语义。
* 新控制节点必须支持保存、加载、校验、测试运行、执行日志可解释。

## Acceptance Criteria (evolving)

* [ ] 明确当前编排节点实现状态。
* [ ] 决定是否实现并行分支/合流/循环。
* [ ] 若进入实现，补充对应 code-spec 和测试要求。

## Definition of Done (team quality bar)

* Tests added/updated where appropriate.
* Lint / typecheck / build green.
* Docs/notes updated if behavior changes.
* Rollout/rollback considered if risky.

## Out of Scope (explicit)

* 仅通过前端 enabled 打开未实现节点。
* 在没有 runner 语义时保存 parallel/join/loop 节点。

## Technical Notes

* `web/src/main.jsx`：`controlNodeCatalog` 中 `condition` enabled，`parallel` / `join` / `loop` planned disabled。
* `internal/config/types.go`：当前 workflow node type 仅确认有 tool/condition。
* `internal/registry/validate.go` 与 `internal/runner/runner.go`：当前控制流语义集中在 condition case 分支。
* `.trellis/spec/frontend/workflow-editor-condition-controls.md`：明确 planned control cards 是 roadmap hints，不应注册 click/drag/saveable payload。

## Expansion Sweep

### Future evolution

* 并行/合流可以支持 fan-out/fan-in 执行、失败策略、等待策略。
* 循环需要迭代上限、退出条件、防无限循环保护。

### Related scenarios

* 画布测试运行必须支持新控制节点，否则用户无法验证草稿。
* 保存/加载/导出插件包必须 round-trip 新节点类型。

### Failure & edge cases

* 并行节点失败后是 fail-fast 还是继续执行需要定义。
* 循环必须有 max_iterations 或等价保护，避免 runaway workflow。

## Decision (ADR-lite)

**Context**: 当前只有条件分支真正可用，用户确认可以继续推进其它编排节点。循环语义复杂且需要防无限循环保护，不适合作为下一步 MVP。

**Decision**: 先实现 `并行分支` 与 `合流`；`循环` 继续保持规划中禁用状态。

**Consequences**: 需要同步扩展前端节点、保存/加载 payload、后端校验、runner 执行语义和测试；不允许只打开 UI 卡片。


**Approach A: 先实现并行分支 + 合流**

* How: 新增 node types/schema/validator/runner fan-out/fan-in 语义。
* Pros: 最贴近 DAG 工作流增强，循环风险较低。
* Cons: 需要定义失败策略和 join 等待规则。

**Approach B: 先实现循环**

* How: 新增 loop node，支持 input/condition/max_iterations/body edges。
* Pros: 表达能力强。
* Cons: 语义复杂，风险最高，必须防无限循环。

**Approach C: 暂不实现，只保持规划卡片**

* How: 当前状态不变，继续只支持条件分支。
* Pros: 最稳，不引入半成品 runtime 语义。
* Cons: 编排能力扩展暂停。

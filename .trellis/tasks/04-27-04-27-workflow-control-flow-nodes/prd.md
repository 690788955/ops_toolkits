# brainstorm: workflow control flow nodes

## Goal

让可视化工作流画布不仅能连接工具节点，还能表达串行、并行和判断条件等控制流，提升运维编排能力，同时保持 YAML/CLI/API/Web 的一致执行语义。

## What I already know

* 用户希望画布支持并行、串行、判断条件这类操作。
* 当前项目是 YAML 驱动的 Shell 运维框架，工作流已采用 `nodes` + `edges` 的 DAG 结构。
* 当前后端模型在 `internal/config/types.go`：`WorkflowConfig` 包含 `Nodes []WorkflowNode` 和 `Edges []WorkflowEdge`。
* 当前 `WorkflowNode` 主要表示工具执行节点：`id/name/tool/depends_on/params/optional/timeout/confirm/on_failure`。
* 当前 `WorkflowEdge` 只有 `from/to`，没有条件表达式或分支标签。
* 当前校验在 `internal/registry/validate.go` 使用拓扑排序校验 DAG 和环形依赖。
* 当前执行在 `internal/runner/runner.go` 通过 `registry.OrderWorkflow` 得到拓扑序后逐个顺序执行，因此数据模型允许 DAG，但运行时还没有真正并发执行。
* 当前 Web 画布在 `web/src/main.jsx` 使用 React Flow，保存时将 flow nodes/edges 映射回 `nodes`/`edges`。
* 当前执行结果/日志在 `WorkflowEditor` 之后的 `resultCard` 渲染，适合作为工作流执行日志入口。

## Assumptions (temporary)

* “串行”可以继续用普通边表达：A → B。
* “并行”可由一个节点指向多个后继、或多个无依赖节点表达，但需要运行时按依赖层并发执行才真正并行。
* “判断条件”需要新增条件边或新增判断节点；两种方式会影响 YAML schema、校验、UI 和执行器。
* 运维场景下条件表达式应保持简单、安全、可解释，避免一开始引入通用脚本执行能力。

## Open Questions

* None for current planning pass.

## Requirements (evolving)

* Use a phased rollout.
* Phase 1: support visual condition nodes, labeled condition outcomes, save/load round-trip, and backend validation.
* Phase 1: keep serial and parallel as normal DAG edges; do not add separate serial/parallel node types.
* Phase 1: condition expressions must use a safe structured predicate model, not arbitrary script execution.
* Phase 2: implement real parallel runtime scheduling based on DAG readiness.
* Phase 2: implement complete skipped-node semantics and run-record visibility.
* Preserve YAML, CLI, API, and Web UI consistency for the same workflow definition.

## Acceptance Criteria (evolving)

### Phase 1 — visual condition nodes + validation

* [ ] 用户能在画布中表达 A → B → C 串行流程。
* [ ] 用户能在画布中表达 A 后分叉到 B/C 的并行分支形态。
* [ ] 用户能在画布中新增条件节点，并配置结构化条件。
* [ ] 用户能从条件节点连接 true/false 或 case 分支，并在边上看到分支标签。
* [ ] 保存后的工作流 YAML/JSON 能 round-trip 回 Web 画布。
* [ ] 后端校验能识别 `type: tool` 与 `type: condition` 节点，并给出清晰错误。
* [ ] Phase 1 不承诺真实并发执行；如执行语义未完整实现，UI/文档必须明确说明。

### Phase 2 — runtime semantics

* [ ] Runner 能按 DAG readiness 调度可并行节点，并支持合理的并发上限。
* [ ] Runner 能执行条件节点，并只激活匹配 outcome/case 的后继边。
* [ ] Run record 能记录 `skipped` 状态、跳过原因和条件求值结果。
* [ ] Fan-in 节点有明确默认策略，避免 skipped 分支导致行为不确定。

## Definition of Done (team quality bar)

* Tests added/updated (unit/integration where appropriate)
* Lint / typecheck / CI green
* Docs/notes updated if behavior changes
* Rollout/rollback considered if risky

## Out of Scope (explicit)

* Phase 1 不实现真实并发 runtime scheduling。
* Phase 1 不实现完整 skipped-node 传播与 fan-in 策略。
* 暂不引入复杂脚本语言或任意代码执行作为条件表达式。
* 暂不引入长期运行的调度器/暂停恢复能力，除非后续明确需要。

## Technical Approach

Adopt the phased option chosen by the user.

### Phase 1 — condition node planning scope

* Keep serial/parallel as DAG topology: serial is `A -> B`, parallel shape is fan-out/fan-in edges.
* Add explicit typed condition nodes for the canvas: default old nodes to `type: tool`; new control-flow node uses `type: condition`.
* Add edge branch metadata for condition outputs, e.g. `outcome: true`, `outcome: false`, or later `case: prod`.
* Use a structured, data-only condition schema, e.g. `left/operator/right`, with enumerated operators.
* Update backend validation and Web save/load round-trip before attempting full runtime semantics.

### Phase 2 — runtime planning scope

* Replace strictly sequential topological execution with readiness-based scheduling and a `max_parallel` safety limit.
* Evaluate condition nodes during execution and activate only matching outgoing branches.
* Add skipped status and skip reason to run records and UI logs.
* Define fan-in policy explicitly; default should be conservative and deterministic.

## Decision (ADR-lite)

**Context**: The existing workflow model already uses DAG nodes/edges, but all nodes are currently tool nodes and the runner executes topological order sequentially. The canvas UX benefits from explicit visual condition nodes, while runtime changes are larger and riskier.

**Decision**: Use a phased plan. Phase 1 implements visual condition nodes, typed schema, labeled branch edges, round-trip persistence, and validation. Phase 2 implements true parallel scheduling and complete conditional skip semantics.

**Consequences**: This reduces implementation risk and keeps UI/schema work separate from runner concurrency semantics. Phase 1 must avoid implying true parallel runtime behavior until Phase 2 lands.


* Data model: `internal/config/types.go` `WorkflowConfig`, `WorkflowNode`, `WorkflowEdge`。
* Validation: `internal/registry/validate.go` `ValidateWorkflow`, `OrderWorkflow`。
* Runtime: `internal/runner/runner.go` `RunWorkflowWithConfirmation` 当前按拓扑序顺序执行。
* Web editor: `web/src/main.jsx` `WorkflowEditor`, `onConnect`, `buildWorkflowDraft`, `workflowNodeToFlowNode`。
* Cross-layer concern: schema changes must round-trip through YAML ↔ server API ↔ React Flow ↔ runner.

## Research References

* [`research/control-flow-patterns.md`](research/control-flow-patterns.md) — Serial/parallel should stay DAG-based; conditions should use either node guards (`when`) or explicit visual condition nodes with labeled outcomes.

## Research Notes

### What similar tools do

* Serial execution is normally dependency topology, not a special node: `A -> B` / `needs` / `runAfter`.
* Parallel is normally implicit from DAG readiness: fan-out/fan-in can be represented by edges; true concurrent execution is a scheduler behavior.
* Conditions commonly live either on task/node metadata (`if`/`when`) or in explicit visual router nodes (`If`/`Switch`) with labeled outputs.
* Safe workflow systems avoid arbitrary script conditions for MVP; structured operators like `eq`, `neq`, `in`, `exists`, `contains` are easier to validate.

### Constraints from our repo/project

* Current schema already supports DAG edges, but runner executes ordered nodes sequentially.
* Current validation requires every node to have `tool`, so explicit condition nodes require typed-node schema migration.
* React Flow is well-suited to explicit decision nodes and labeled true/false edges.
* YAML/API/Web must round-trip the same workflow definition.

### Feasible approaches here

**Approach A: DAG + tool-node `when` guards**

* How it works: keep all nodes as tool nodes; add `when` metadata to a node, evaluated before execution.
* Pros: smallest backend schema change; close to GitHub Actions / Argo / Tekton; keeps edges simple.
* Cons: less visual; branch logic is hidden inside downstream nodes; users may not see a decision point on canvas.

**Approach B: DAG + explicit condition nodes with labeled edges** (recommended for visual canvas)

* How it works: introduce typed nodes: `type: tool` and `type: condition`; condition node evaluates a structured predicate; outgoing edges carry `outcome: true/false` or `case`.
* Pros: best UX for React Flow; condition is visible as a canvas node; avoids duplicated expressions on edges.
* Cons: requires schema, validation, runner, run-record, and UI changes.

**Approach C: Conditions directly on edges**

* How it works: edge gets `condition` metadata and activates only if true.
* Pros: visually close to branch connectors; no separate node.
* Cons: harder to validate/debug; duplicated conditions; fan-in and skip semantics get ambiguous.

## Expansion Sweep

### Future evolution

* True parallel execution can later add ready-queue scheduling and `max_parallel` limits.
* Fan-in policies may be needed later: `all_success`, `any_success`, `always`.

### Related scenarios

* Plugin-provided workflow YAML must remain compatible with the Web editor.
* CLI/API validation errors should explain condition and skip behavior clearly.

### Failure & edge cases

* Skipped nodes need explicit run status and skip reason.
* If a skipped branch feeds a merge/fan-in node, the default MVP rule must be deterministic.
* Condition expressions must be data-only and validated; no shell/JS/Python execution.

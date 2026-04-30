# brainstorm: loop node embedded tool config

## Goal

将循环节点从“引用画布上的目标工具节点”改为“循环节点内部直接选择工具并配置参数”，让画布更简洁，避免用户额外拖一个 loop target 工具节点以及普通连线冲突。

## What I already know

* 用户确认希望循环节点内部选择工具并配置参数。
* 当前 loop MVP 使用 `loop.target` 指向画布上的工具节点，并禁止该 target 参与普通 edges。
* 用户已经遇到 `循环目标工具节点 hello_tool 不能同时配置普通连线`，说明当前 UX 容易误导。
* 目标新形态是类似：`type: loop` + `loop: {tool, params, max_iterations}`。
* 当前前端已有 `ParamMappingEditor`，可复用工具参数配置能力。
* 当前画布执行 overlay 已按通用 loop iteration 聚合实现，不强依赖 target 节点必须存在。

## Assumptions (temporary)

* 新 loop schema 应向后兼容或提供兼容读取旧 `target`，至少避免已有草稿直接崩。
* MVP 不支持 loop 内部条件退出，仍是固定次数。
* loop 内部工具与普通 tool node 共用参数解析/确认/执行逻辑。

## Open Questions

* 是否直接迁移为 `loop.tool` + `loop.params` + `loop.max_iterations`，并废弃新建示例中的 `loop.target`。

## Requirements (evolving)

* loop 节点不再需要画布上的独立 target 工具节点。
* loop 配置包括：工具 ID、工具参数、最大循环次数。
* loop 参数编辑复用现有工具参数映射能力，支持工作流参数和上游节点输出。
* Runner 执行 loop 时按 max_iterations 重复执行 loop.tool。
* loop tool 失败时停止循环并标记 workflow failed。
* 后端校验 loop.tool 必填且存在，max_iterations 在 1..20。
* loop 不能配置普通 node.tool 或 condition。
* 更新全节点示例 workflow，移除独立 `loop_target` 节点。
* 保存/加载/草稿运行/执行 overlay 支持新 schema。

## Acceptance Criteria (evolving)

* [ ] 用户可在 loop 节点配置 modal 选择工具并配置参数。
* [ ] loop workflow 不需要额外 target 工具节点。
* [ ] 全节点示例 workflow 校验通过且可运行。
* [ ] 旧 target 方案不会在 UI 中继续引导用户。
* [ ] `GOTOOLCHAIN=local go test ./...` 通过。
* [ ] `npm run build --prefix web` 通过。
* [ ] `./bin/opsctl.exe validate` 通过。

## Definition of Done (team quality bar)

* Tests added/updated.
* Build and validation green.
* Specs updated for cross-layer schema change.
* Rollback considered if risky.

## Out of Scope (explicit)

* 不实现条件退出循环。
* 不实现循环子图。
* 不允许图上真实环。

## Technical Notes

* Backend currently has `WorkflowLoop` with target/max_iterations.
* Runner currently locates target node by ID and executes it.
* Frontend currently configures loop target node select and max iterations.
* `ParamMappingEditor` can likely be reused for loop tool params.

## Feasible approaches

**Approach A: Replace loop target with embedded tool config（推荐）**

* How: `loop: {tool, params, max_iterations}`; runner executes selected tool directly.
* Pros: Cleanest canvas; no target edge ambiguity; matches user expectation.
* Cons: Cross-layer schema migration needed.

**Approach B: Support both target and embedded tool**

* How: new workflows use embedded tool, old target still accepted.
* Pros: More backward compatible.
* Cons: Two semantics to maintain and explain.

**Approach C: Keep target but improve UI**

* How: hide target node or auto-create managed node.
* Pros: Smaller backend changes.
* Cons: Still conceptually awkward.

## Decision (ADR-lite)

**Context**: The target-node loop MVP is safe but creates unnecessary canvas clutter and confusing validation errors when users connect the target node normally.

**Decision**: Use Approach A as the forward model: loop owns tool selection and params internally. Preserve only defensive compatibility if cheap.

**Consequences**: The loop schema and runner change again, but the user-facing model becomes much simpler and the example workflow no longer needs a hidden/managed target node.

# brainstorm: workflow canvas run progress

## Goal

让工作流执行结果直接体现在画布上：用户点击执行后，节点和连线能显示执行过程/结果状态，而不是只在右侧/底部结果详情中查看日志。

## What I already know

* 用户问“执行过程能不能在画布有过程”。
* 当前 `runDraft()` 调用后会拿到 run response，并通过 `fetchRunDetail(body.id)` 获取 `record.steps`。
* 当前后端执行接口是同步返回最终 run id/status，前端再取详情；没有 SSE/WebSocket 流式进度。
* `runner.StepRecord` 已包含 step id/type/tool/status/error/skipped_reason/condition/loop 等信息。
* 前端已有 React Flow nodes/edges，可通过 `node.data` 或 CSS class 映射状态。
* 当前已支持 condition、parallel、join、loop 和 loop iteration step record。

## Assumptions (temporary)

* MVP 先做“执行后回放/结果高亮”，不做真正实时流式执行进度。
* 由于当前 API 同步执行，所谓“过程”可以先在画布上显示：执行中整体状态、完成后节点成功/失败/跳过、条件命中分支、loop 迭代摘要。
* 未来如果需要实时过程，再扩展轮询/SSE。

## Open Questions

* None. User confirmed Approach A: post-run canvas status overlay.

## Requirements (evolving)

* 点击执行时，画布进入 running 状态，按钮/节点有执行中提示。
* 执行完成后，根据 `record.steps` 给对应节点显示状态：succeeded/failed/skipped/running。
* 失败节点需要明显标红，并能看到错误提示。
* 条件节点需要显示 matched_case，命中的条件边高亮，未命中分支弱化。
* loop 节点需要显示迭代次数；loop iteration step 可汇总到 loop 节点或目标节点提示。
* parallel/join 控制节点显示成功/跳过状态。
* 状态只影响画布展示，不进入 workflow schema。
* 新运行前清除旧状态。
* 生产构建通过：`npm run build --prefix web`。

## Acceptance Criteria (evolving)

* [ ] 执行后画布节点能显示成功/失败/跳过状态。
* [ ] 条件命中分支边能高亮。
* [ ] loop 节点能显示迭代次数摘要。
* [ ] 失败错误能通过 hover/title 或节点提示查看。
* [ ] 不改变保存/加载/runner/API 契约。
* [ ] `npm run build --prefix web` 通过。

## Definition of Done (team quality bar)

* Build green.
* Existing workflow execution behavior preserved.
* Docs/spec updated if UI contract changes.
* Rollback considered: run state is frontend-only.

## Out of Scope (explicit)

* 不做 SSE/WebSocket 实时推送。
* 不改变 runner 执行模型。
* 不把运行状态保存进 workflow YAML/JSON。
* 不做复杂动画时间轴。

## Technical Notes

* `web/src/main.jsx` has `runDraft()` around the workflow editor and already fetches run detail.
* `RunDetail` rendering uses `combineWorkflowStepLogs(steps, record)`.
* Backend `RunRecord.Steps` is already enough for post-run canvas status mapping.
* Edge highlighting can derive from source condition node's `matched_case` and edge.data.case.

## Decision (ADR-lite)

**Context**: Current workflow execution returns a completed run record and then fetches detail; there is no streaming progress API yet.

**Decision**: Implement Approach A: a frontend-only post-run canvas status overlay. The editor maps `record.steps` back onto React Flow nodes/edges after run detail is available, and shows a coarse running state while waiting.

**Consequences**: No backend/API/schema change is needed. The canvas reflects execution results clearly, but it is not true real-time streaming; polling/SSE can be added later.


**Approach A: Post-run canvas status overlay（推荐 MVP）**

* How: after run detail returns, build a map `{nodeID -> step status}` and `{conditionNodeID -> matched_case}`, store in frontend state, style nodes/edges.
* Pros: No backend/API changes; fastest and safest.
* Cons: Not real-time during long-running tools.

**Approach B: Poll run detail during execution**

* How: submit run, poll `/api/runs/{id}` until complete, update canvas incrementally.
* Pros: Feels more live if backend writes partial records.
* Cons: Current backend likely writes record at end; may need runner persistence changes.

**Approach C: Streaming progress API**

* How: SSE/WebSocket from runner to frontend.
* Pros: Best UX for real process animation.
* Cons: Cross-layer complexity, cancellation/backpressure/reconnect handling.

## Expansion Sweep

### Future evolution

* Add replay animation over completed `record.steps`.
* Add real-time progress via polling/SSE after runner can persist partial step records.

### Related scenarios

* Saved workflow run and unsaved draft run should both map results to the current canvas.
* Loading or editing workflow should clear stale run state.

### Failure & edge cases

* Run detail might include loop iteration IDs like `loop#1-target`; these should aggregate back to loop/target display.
* If the user edits nodes after run, old status should clear or only apply to matching node IDs.

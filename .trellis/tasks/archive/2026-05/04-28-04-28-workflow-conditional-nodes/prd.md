# brainstorm: n8n-like workflow condition nodes

## Goal

在现有工作流编排器中添加类似 n8n 的可视化条件判断节点，使工作流可以读取上游节点输出并按判断结果流转到不同后继分支，从而支持更真实的运维自动化决策流程。

## What I already know

* 用户希望在编排器添加类似 n8n 的条件判断节点。
* 条件节点需要能对上一个节点的输出进行判断，然后决定后续流转。
* 现有工作流模型已经是 DAG：`nodes` + `edges`。
* 当前后端 `WorkflowNode` 仍以工具节点为中心，`WorkflowEdge` 只有 `from/to`。
* 当前校验要求每个节点都有 `tool`，因此会拒绝非工具条件节点。
* 当前 runner 按拓扑序顺序执行工具节点，并通过 workflow context 暴露 `steps.<nodeID>.stdout/stderr/params.*`。
* Web 编辑器当前只 round-trip 工具节点和普通边，会丢失条件分支元数据。
* 既有任务 `.trellis/tasks/04-27-04-27-workflow-control-flow-nodes/prd.md` 已规划过 typed condition nodes、labeled branch edges、structured predicates 和 phased runtime semantics。

## Assumptions (temporary)

* MVP 第一版支持多分支 Switch/case 节点，而不是只做二分支 If。
* 条件表达式使用结构化字段，不执行任意 JS/Shell/Python。
* 条件输入复用现有模板上下文，例如 `{{ .steps.inspect.stdout }}`。
* 串行/并行仍由 DAG 边表达，不新增“串行节点/并行节点”。
* 前端节点控件是本功能的核心交付，不仅是后端 schema 暴露。
* 本任务聚焦条件路由语义；真实并发调度可以独立后续实现。

## Requirements (evolving)

* 新增显式条件节点类型：`type: condition`；旧工作流未写 `type` 且有 `tool` 时仍视为 `tool`。
* 条件节点支持多分支 Switch/case：一个输入、多个 case 规则、可选 default 分支。
* 条件节点支持读取工作流参数和上游节点输出：`{{ .param }}`、`{{ .steps.<id>.stdout }}`、`{{ .steps.<id>.stderr }}`、`{{ .steps.<id>.params.<name> }}`。
* 条件谓词采用安全结构化 schema；每个 case 至少包含 `id/name/operator/values`，节点级包含 `input`。
* MVP 支持操作符：`eq`、`neq`、`contains`、`not_contains`、`in`、`not_in`、`exists`、`empty`。
* 条件节点出边通过 `case: <case_id>` 表示命中的分支，通过 `case: default` 表示默认分支。
* 后端校验必须区分 tool 节点和 condition 节点，并给出清晰中文错误。
* Runner 必须执行条件节点并只激活匹配 case 分支；无匹配 case 时激活 default 分支；非匹配分支下游应被跳过且不视为失败。
* Run record 应能解释条件节点输入值、命中 case 和跳过原因，供 CLI/API/Web 展示。
* Web 编辑器节点面板采用双 Tab 结构：`插件工具` / `编排节点`。
* `插件工具` Tab 展示插件贡献的可执行工具，继续支持搜索、标签过滤、拖拽/点击添加。
* `编排节点` Tab 展示编排器内置控制节点；第一版条件分支可用，并展示并行、合流、循环等控制节点类型作为规划中/暂不可用项，避免用户误以为编排节点只有条件分支。
* 左侧工作流编辑器操作区不再保留单独的 `添加条件节点` 按钮；新增编排节点必须统一从顶部 `编排节点` Tab 的节点卡片进入。
* 条件节点不作为 `run tool` 的目标暴露；它只能作为 workflow 内部编排节点执行。
* UI consistency: `插件工具` and `编排节点` tabs should use the same compact grid card layout. Orchestration cards may keep a subtle purple accent/status badge, but must not use a completely different full-width panel style.
* Incremental polish: Web editor node palette now explicitly splits `插件工具` and `编排节点`; only the built-in `条件分支` card is exposed under orchestration nodes in this version, with click/drag add behavior.
* Incremental polish: CLI/menu/help output should render condition nodes and step records as human-readable orchestration semantics without changing the existing run record JSON contract.

## Acceptance Criteria (evolving)

* [ ] YAML/API 支持 `type: condition` 节点和带 `case` 的 edges。
* [ ] 未声明 `type` 的既有 tool workflow 保持兼容。
* [ ] 校验拒绝 condition 节点同时配置 `tool`。
* [ ] 校验拒绝 condition 节点缺少 `condition.input`、缺少 cases 或使用非法 `operator`。
* [ ] 校验拒绝 condition 出边缺少或使用不存在的 `case`。
* [ ] 工作流 A(tool) → B(condition) → C(case1) / D(case2) / E(default) 能按 A 的 stdout 选择分支执行。
* [ ] 未命中的分支节点记录为 skipped，不导致整个工作流失败。
* [ ] Fan-in 默认规则明确：非激活条件边不阻塞，激活入边必须成功。
* [ ] Web 画布能添加条件节点，并以区别于工具节点的视觉样式展示。
* [ ] Web 条件节点控件能编辑输入来源、多个 case、每个 case 的操作符和值、default 分支。
* [ ] Web 节点面板有两个清晰 Tab：`插件工具` 和 `编排节点`。
* [ ] 左侧工作流操作区不再显示独立 `添加条件节点` 按钮。
* [ ] 新增条件/编排类节点必须统一从顶部 `编排节点` Tab 添加。
* [ ] `编排节点` Tab 至少展示条件、并行、合流、循环等类型；当前仅 `条件分支` 可点击/拖拽，未实现类型应标记为规划中/暂不可用。
* [ ] Web 连线能选择/显示 case 标签，避免用户看不出每条边代表哪个分支。
* [ ] Web 保存前能提示缺失输入、空 case、重复 case ID、非法 case 连线。
* [ ] 菜单交互入口能选择包含 condition 节点的 workflow，并正常提示参数、确认高风险工具、执行分支逻辑。
* [ ] 菜单/交互式详情能展示条件节点是编排节点，不误导为插件工具。
* [ ] CLI `run workflow <id>` 能执行包含 condition 节点的 workflow，并在日志/run record 中显示命中 case 和 skipped 分支。
* [ ] CLI `help-auto workflow <id>` 或工作流详情输出能让用户看懂条件节点、cases、default 分支和分支边。
* [ ] CLI 不允许把 condition 节点当作 `run tool` 目标直接执行。
* [ ] 现有检查通过：`GOTOOLCHAIN=local go test ./...`、`npm run build --prefix web`、`GOTOOLCHAIN=local go build -o "bin/opsctl.exe" ./cmd/opsctl`、`./bin/opsctl.exe validate`。

## Definition of Done (team quality bar)

* Tests added/updated (unit/integration where appropriate)
* Lint / typecheck / CI green
* Docs/notes updated if behavior changes
* Rollout/rollback considered if risky

## Technical Approach

Use explicit typed Switch/case condition nodes with labeled branch edges.

Suggested schema:

```yaml
nodes:
  - id: inspect
    type: tool
    tool: plugin.demo.inspect
  - id: route_status
    type: condition
    condition:
      input: "{{ .steps.inspect.stdout }}"
      cases:
        - id: ok
          name: 正常
          operator: contains
          values: ["OK"]
        - id: warn
          name: 告警
          operator: contains
          values: ["WARN"]
      default_case: default
edges:
  - from: inspect
    to: route_status
  - from: route_status
    to: apply
    case: ok
  - from: route_status
    to: notify_warn
    case: warn
  - from: route_status
    to: notify_unknown
    case: default
```

Frontend control design requirements:

* Canvas node: condition nodes use a distinct title/icon/color and show a compact summary such as `stdout contains OK/WARN`.
* Inspector: selected condition node exposes input source picker, case table/list, operator dropdown, values editor, case rename/delete/reorder, default case toggle.
* Edge UX: outgoing edges from a condition node must display the selected case label on the edge; changing cases should update or flag invalid edges.
* Validation UX: missing input, empty cases, duplicate case IDs, missing edge case, and dangling case references are shown before save.
* KISS constraint: implement as simple React state and existing React Flow primitives first; avoid introducing a full form framework unless necessary.

Condition branch card design in `编排节点` Tab:

* Card title: `条件分支`，secondary label: `Switch / Case`.
* Card description: `根据上游输出或工作流参数选择后续分支`.
* Visual icon: diamond/fork-style control-flow icon; use a distinct control-node accent color, not the plugin/tool color.
* Capability chips: `多分支`、`默认分支`、`读取 stdout/stderr/参数`.
* Inline preview:
  * `输入：选择上游输出`
  * `分支：case1 / case2 / default`
* Interactions:
  * Click card: add condition node to canvas center/next available position.
  * Drag card: drop condition node at pointer position.
  * Hover/focus: show short help text, e.g. `适合根据巡检结果、返回文本、参数值做分流`.
* Card set in `编排节点` Tab:
  * `条件分支` / `Switch / Case` — enabled in this task.
  * `并行分支` / `Parallel` — visible as `规划中`, not clickable yet.
  * `合流` / `Merge` — visible as `规划中`, not clickable yet.
  * `循环` / `Loop` — visible as `规划中`, not clickable yet.
* The previous left-side `添加条件节点` button is removed to keep one clear add-node entry point.

Canvas condition node design:

* Header: `条件分支` + node ID/name.
* Body summary:
  * If input empty: show `未选择判断输入` warning style.
  * If input set: show shortened input path, e.g. `steps.inspect.stdout`.
  * Show case count: `2 个分支 + default`.
* Footer/status:
  * Show `配置不完整` when input/cases are invalid.
  * Show `可运行` when validation passes.
* Handles/edges:
  * One normal input handle.
  * Outgoing edges are labeled by selected case; MVP may keep a single output handle plus edge-label selector rather than one physical port per case.

Inspector condition editor design:

* Section 1: 基本信息 — node ID/name.
* Section 2: 判断输入 — upstream source selector plus manual template input.
* Section 3: 分支规则 — editable case list with ID/name/operator/values.
* Section 4: 默认分支 — enable/disable default, explain that default runs when no case matches.
* Section 5: 连线检查 — list outgoing edges and their selected case; flag missing or deleted case references.

### CLI and menu compatibility

Condition nodes must be compatible with workflow-level CLI and interactive menu commands:

```bash
./bin/opsctl.exe validate
./bin/opsctl.exe help-auto workflow <workflow-id>
./bin/opsctl.exe run workflow <workflow-id> --set key=value --no-prompt
./bin/opsctl.exe start
./bin/opsctl.exe menu
```

Expected behavior:

* `validate`: validates condition node schema, case IDs, default branch, and condition edge cases.
* `help-auto workflow`: prints condition nodes as orchestration/control nodes, including input, cases, default branch, and outgoing case labels.
* `run workflow`: executes condition nodes inside the workflow and records condition input, matched case, and skipped branches.
* `start` / `menu`: users can browse/select workflows that contain condition nodes, enter workflow parameters, pass high-risk confirmations for tool nodes, and execute the same branch semantics as CLI/API/Web.
* Interactive workflow details should label condition nodes as `编排节点/条件分支`, not as plugin tools.
* `run tool`: does not accept condition nodes because condition nodes are not plugin tools.


* `internal/config/load.go`: preserve backward compatibility and normalize legacy nodes.
* `internal/registry/validate.go`: typed validation + branch edge validation + existing DAG cycle checks.
* `internal/runner/runner.go`: branch-aware execution state, condition evaluation, skipped records.
* `internal/server/*`: ensure API catalog/detail/save/run responses preserve new fields.
* `web/src/main.jsx`: React Flow condition node, inspector fields, branch edge labels, save/load mapping.
* Tests: config/registry/runner/server/web build coverage for round-trip and execution branch behavior.

## Orchestrator-native control node design

### Product model

The workflow editor should distinguish two node families:

* **Tool nodes**: plugin-contributed operational actions. They execute external commands through `internal/runner` and require a registered `tool` ID.
* **Control nodes**: orchestrator-native flow-control semantics. They do not come from plugins and do not launch shell commands. They are interpreted by the runner.

Condition/Switch nodes belong to **control nodes**, not plugin tools.

### Initial built-in control node catalog

For this task, only `condition` is in implementation scope. The UI/architecture should leave room for these future built-in control nodes without implementing them now:

* `condition`: multi-branch Switch/case routing based on workflow params or previous node output.
* `merge`: explicit fan-in/merge behavior after branches.
* `approval`: human confirmation gate before continuing.
* `delay`: wait for a configured duration before continuing.
* `sub_workflow`: call another workflow as a node.

### Editor UX direction

* The workflow editor should have a clear **node palette split**:
  * **插件工具**: searchable/tag-filtered tools from plugin catalog.
  * **编排节点**: built-in control nodes such as 条件分支.
* First version uses a **two-tab node palette**: `插件工具` and `编排节点`. This keeps executable plugin tools separate from orchestrator-native control nodes while staying simple for users.
* Control nodes should use distinct visual treatment from tool nodes: color, icon/title, compact semantic summary, and specialized inspector controls.
* Control node configuration should be schema-driven enough to avoid one-off ad hoc UI for every new built-in node, but not so abstract that the first version becomes a form-engine project.

### Runtime direction

* Runner should treat built-in control nodes as internal evaluators.
* Control node execution records should be visible in run details, even when they do not run shell commands.
* Control nodes must define their skip/fan-in/failure behavior explicitly because they affect downstream execution.

### Design decision

**Decision**: Keep plugin tools and orchestrator-native control nodes as separate concepts. Plugins provide operational capabilities; the orchestrator provides flow semantics.

**Why**: This keeps unsafe or high-risk command execution in tool nodes, while allowing flow-control nodes to be validated, visualized, and executed safely inside the runner.

**Consequence**: The editor needs a built-in control-node palette and each control node type needs an explicit backend contract.



**Decision**: Implement explicit multi-branch Switch/case condition nodes in the first version, with structured predicates and `case`-labeled branch edges. Reuse existing workflow context for previous output references.

**Consequences**: The canvas becomes easy to understand for operators and closer to n8n/Node-RED routing. Schema and runner complexity are higher than a binary If, so frontend controls, validation, run records, and skipped/fan-in semantics must be designed together. Arbitrary expressions remain out of scope to keep the feature safe and testable.

## Canvas Control Node Shape Refinement

### New user feedback

* 用户希望画布里的编排节点样式可以调整，节点形状要符合节点用途，而不是所有节点都使用类似工具卡片的圆角矩形。
* 当前代码里 `conditionNode` 主要是紫色圆角卡片；它视觉上能区分工具节点，但还不够像流程图里的“判断/分支”节点。

### Requirements (evolving)

* 画布中的编排节点应使用符合流程图语义的视觉形状。
* `条件分支` 节点应明确表达“判断/路由/Switch Case”，优先考虑菱形或带菱形语义的混合形态。
* 工具节点仍保持动作/执行语义的圆角矩形，避免和条件节点混淆。
* 节点仍需保留可读的名称、输入摘要、case 数量/状态，以及删除按钮和连线 handles。
* 样式调整不应改变 workflow schema、runner 语义或 API payload。

### Acceptance Criteria (evolving)

* [ ] 条件节点在画布中一眼能看出是判断/分支节点，而不是普通工具节点。
* [ ] 条件节点仍能正常选择、删除、连线、显示 case 摘要和状态。
* [ ] 工具节点和条件节点视觉语义区分明确。
* [ ] Web 构建通过并更新 embedded assets。

### Decision

**Decision**: 采用 **菱形语义 + 信息卡混合节点**。

**Why**: 纯菱形最符合传统流程图判断节点，但承载不了运维编排器需要展示的输入来源、case 摘要和配置状态；混合节点既保留“判断/分流”的视觉语义，又能保持可读信息密度。

**Requirements**:

* 条件节点主体应明显带有菱形判断语义，例如菱形徽标、菱形头部或菱形背景结构。
* 条件节点仍保留紧凑信息卡区域，用于展示节点名、输入摘要、分支摘要和配置状态。
* 工具节点继续保持圆角矩形动作卡片，形成“动作节点 vs 判断节点”的视觉区分。
* 不改变 workflow schema、runner 语义、API payload 或并行/合流/循环的规划中状态。

### Options considered

1. **纯菱形判断节点**：最符合流程图，但文字空间较小，case 摘要可能拥挤。
2. **已选择：菱形语义 + 信息卡混合节点**：兼顾条件判断语义和配置可读性。
3. **胶囊/六边形控制节点**：比卡片更像控制节点，但“条件判断”语义不如菱形直观。

## Out of Scope (explicit)

* Arbitrary script/expression execution in conditions.
* Real parallel execution scheduling / concurrency limits.
* Rich structured JSONPath/JQ extraction from stdout unless later explicitly requested.
* Long-running workflow pause/resume semantics.

## Research References

* [`research/conditional-routing-patterns.md`](research/conditional-routing-patterns.md) — n8n/Node-RED favor explicit router nodes; YAML-first systems favor guards; this repo should use typed condition nodes with safe structured predicates.
* [`../04-27-04-27-workflow-control-flow-nodes/research/control-flow-patterns.md`](../04-27-04-27-workflow-control-flow-nodes/research/control-flow-patterns.md) — prior broader research on serial/parallel/condition control-flow patterns.

## Technical Notes

* Existing schema: `internal/config/types.go` `WorkflowConfig`, `WorkflowNode`, `WorkflowEdge`.
* Existing validation: `internal/registry/validate.go` `ValidateWorkflow`, `OrderWorkflow`.
* Existing execution: `internal/runner/runner.go` `RunWorkflowWithConfirmation`, `addStepContext`.
* Existing Web mapping: `web/src/main.jsx` `workflowNodeToFlowNode`, `buildWorkflowDraft`, edge load/save code.
* Graphify report highlights workflow-related communities: server/catalog, condition editor helper traces, runner workflow context, validation, and tests.

## Expansion Sweep

### Future evolution

* Expand Switch/case with compound `all/any` groups if single-condition cases are not enough.
* Add JSON output extraction (`json_path`/`jq`-like safe selectors) if tools start emitting structured stdout.

### Related scenarios

* Plugin-provided workflow YAML must work the same as Web-created workflows.
* CLI/API/Web run details should all explain condition result and skipped branch behavior consistently.

### Failure & edge cases

* Missing upstream output should produce deterministic condition result or validation/runtime error depending on operator.
* Fan-in after conditional branches must not hang or execute prematurely.
* Failed upstream tool should still follow existing stop-on-failure behavior unless `optional/on_failure` is explicitly expanded.

## Open Questions

* None — user chose first-version multi-branch Switch/case and requested careful frontend node control design.

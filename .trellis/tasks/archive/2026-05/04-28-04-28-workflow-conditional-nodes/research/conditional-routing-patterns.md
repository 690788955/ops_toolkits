# Research: conditional routing node patterns

- **Query**: Research conditional/routing node conventions in tools like n8n, Node-RED, GitHub Actions/Argo/Tekton, then map them to this repo's constraints for adding n8n-like workflow condition nodes that evaluate previous node output and choose outgoing branches. Include 2-4 comparable patterns, common conventions and why, recommended MVP schema/execution semantics for this repo, risks/edge cases.
- **Scope**: mixed
- **Date**: 2026-04-28

## Findings

### Files Found

| File Path | Description |
|---|---|
| `internal/config/types.go` | Current workflow YAML/JSON model: `WorkflowConfig` has `nodes`, `edges`, legacy `steps`; `WorkflowNode` is currently tool-centric; `WorkflowEdge` only has `from`/`to` (`internal/config/types.go:141-170`). |
| `internal/config/load.go` | Workflow normalization maps legacy `steps` into `nodes`, derives `edges` from `depends_on`, and defaults empty edges to a serial chain (`internal/config/load.go:122-149`). |
| `internal/registry/validate.go` | Workflow validation currently requires every node to have `tool`, validates edge endpoints, and topologically orders the DAG (`internal/registry/validate.go:19-43`, `internal/registry/validate.go:46-95`). |
| `internal/runner/runner.go` | Runner currently topologically orders nodes, then executes them sequentially; workflow context is accumulated from previous step params/stdout/stderr (`internal/runner/runner.go:78-123`, `internal/runner/runner.go:253-267`). |
| `web/src/main.jsx` | React Flow editor currently loads/saves only tool nodes and simple `{from,to}` edges; mapping UI already exposes previous node stdout/stderr/params as template sources (`web/src/main.jsx:410-425`, `web/src/main.jsx:820-833`, `web/src/main.jsx:912-923`). |
| `.trellis/spec/backend/quality-guidelines.md` | Backend execution constraints: CLI/menu/API/Web all go through `internal/runner`; high-risk confirmation remains a runner backstop; MVP workflows stop on first failed step (`.trellis/spec/backend/quality-guidelines.md:22-35`). |
| `.trellis/spec/backend/directory-structure.md` | Plugin workflow contributions are normalized into `WorkflowConfig`; nodes reference registered tool IDs; YAML structures stay in `internal/config` (`.trellis/spec/backend/directory-structure.md:112-147`). |
| `.trellis/spec/frontend/directory-structure.md` | UI should be generated from backend YAML/plugin metadata and should not hard-code tools/workflows (`.trellis/spec/frontend/directory-structure.md:82-87`). |
| `.trellis/tasks/04-27-04-27-workflow-control-flow-nodes/prd.md` | Prior planning notes already chose typed `tool`/`condition` nodes, branch-labeled edges, structured predicates, and phased runtime skip semantics (`.trellis/tasks/04-27-04-27-workflow-control-flow-nodes/prd.md:77-90`, `.trellis/tasks/04-27-04-27-workflow-control-flow-nodes/prd.md:120-145`). |
| `.trellis/tasks/04-27-04-27-workflow-control-flow-nodes/research/control-flow-patterns.md` | Related prior research comparing DAG topology, node guards, explicit condition nodes, edge conditions, and block syntax (`.trellis/tasks/04-27-04-27-workflow-control-flow-nodes/research/control-flow-patterns.md:161-227`, `.trellis/tasks/04-27-04-27-workflow-control-flow-nodes/research/control-flow-patterns.md:228-235`). |

### Code Patterns

#### Current persisted workflow shape is already DAG-based

`WorkflowConfig` already has graph fields, but they are not yet branch-aware:

```go
// internal/config/types.go:141-170
type WorkflowConfig struct {
    Nodes []WorkflowNode `yaml:"nodes" json:"nodes"`
    Edges []WorkflowEdge `yaml:"edges" json:"edges"`
    Steps []WorkflowNode `yaml:"steps" json:"-"`
}

type WorkflowNode struct {
    ID        string                 `yaml:"id" json:"id"`
    Name      string                 `yaml:"name" json:"name"`
    Tool      string                 `yaml:"tool" json:"tool"`
    DependsOn []string               `yaml:"depends_on" json:"depends_on"`
    Params    map[string]interface{} `yaml:"params" json:"params"`
    Optional  bool                   `yaml:"optional" json:"optional"`
    Timeout   string                 `yaml:"timeout" json:"timeout"`
    Confirm   bool                   `yaml:"confirm" json:"confirm"`
    OnFailure string                 `yaml:"on_failure" json:"on_failure"`
}

type WorkflowEdge struct {
    From string `yaml:"from" json:"from"`
    To   string `yaml:"to" json:"to"`
}
```

`NormalizeWorkflow` already translates legacy/shortcut representations into DAG edges:

- `steps` becomes `nodes` when `nodes` is empty (`internal/config/load.go:122-125`).
- empty `edges` becomes edges derived from `depends_on` (`internal/config/load.go:126-140`).
- if there are no explicit edges and at least two nodes, nodes are chained in list order (`internal/config/load.go:143-149`).

This means conditional routing can extend the existing DAG instead of adding a separate block or nested workflow representation.

#### Current validation assumes every node is a tool node

`ValidateWorkflow` rejects non-tool nodes today:

```go
// internal/registry/validate.go:35-40
if node.Tool == "" {
    return fmt.Errorf("节点 %s 的工具必填", node.ID)
}
if _, ok := r.Tools[node.Tool]; !ok {
    return fmt.Errorf("节点 %s 引用了不存在的工具 %s", node.ID, node.Tool)
}
```

`OrderWorkflow` validates edge endpoints and cycles generically (`internal/registry/validate.go:46-95`), so the topological sort itself is reusable for typed nodes if `ValidateWorkflow` no longer treats all nodes as tools.

#### Current runner has previous-output context that condition nodes can reuse

Workflow execution currently runs ordered tool nodes in a single loop:

```go
// internal/runner/runner.go:92-123
ordered, err := registry.OrderWorkflow(wf.Config)
...
workflowContext := copyParams(finalParams)
for _, node := range ordered {
    stepParams := resolveStepParams(finalParams, workflowContext, node.Params)
    tool, toolErr := r.Registry.Tool(node.Tool)
    stepRecord := StepRecord{ID: node.ID, Tool: node.Tool, Status: "running", StartedAt: time.Now()}
    ...
    addStepContext(workflowContext, node.ID, stepParams, stepRunDir)
    stepRecord.Status = "succeeded"
    record.Steps = append(record.Steps, stepRecord)
}
```

The context populated after a successful tool node is string-keyed and already includes the data needed by output-based conditions:

```go
// internal/runner/runner.go:261-267
func addStepContext(context map[string]string, nodeID string, params map[string]string, runDir string) {
    prefix := "steps." + nodeID + "."
    for k, v := range params {
        context[prefix+"params."+k] = v
    }
    context[prefix+"stdout"] = strings.TrimSpace(readTextFile(filepath.Join(runDir, "stdout.log")))
    context[prefix+"stderr"] = strings.TrimSpace(readTextFile(filepath.Join(runDir, "stderr.log")))
}
```

The React editor already mirrors these context paths in the parameter mapping dropdown (`web/src/main.jsx:912-923`), e.g. `{{ .steps.<nodeID>.stdout }}` and `{{ .steps.<nodeID>.stderr }}`.

#### Current React Flow round-trip drops branch metadata

The UI currently loads edges as plain React Flow edges and saves them as `{from,to}` only:

```jsx
// web/src/main.jsx:424-425
setNodes((config.nodes || []).map((node, index) => workflowNodeToFlowNode(node, index, removeNode)))
setEdges((config.edges || []).map(edge => ({id: `${edge.from}-${edge.to}`, source: edge.from, target: edge.to, type: 'smoothstep', animated: true})))
```

```jsx
// web/src/main.jsx:820-833
function buildWorkflowDraft(workflow, nodes, edges, category, parameters) {
  return {
    ...workflow,
    category: workflow.category || category || '',
    parameters: parameters || workflow.parameters || [],
    nodes: nodes.map(node => ({
      id: node.id,
      name: node.data.name || node.id,
      tool: node.data.tool,
      params: node.data.params || {},
      on_failure: node.data.on_failure || 'stop'
    })),
    edges: edges.map(edge => ({from: edge.source, to: edge.target}))
  }
}
```

Any branch `outcome`/`case` metadata must therefore be represented in both loaded edge data and saved YAML/JSON to round-trip.

### Comparable Patterns

#### 1. n8n: explicit If/Switch router nodes with true/false or case outputs

**Convention**

- n8n exposes visible routing nodes: an `If` node for two-way true/false branching and a `Switch` node for multiple cases.
- The condition belongs to the router node.
- Connections represent output streams/branches from that router node.
- Separate streams can later be combined with merge behavior.

**Why it exists**

- Visual workflow users can see the decision point as a node rather than hunting for hidden guards on downstream tasks.
- Edge labels/output ports keep branch meaning close to the canvas connector.
- A router node avoids duplicating the same expression across multiple downstream edges.

**Shape mapped to this repo**

```yaml
nodes:
  - id: inspect
    type: tool
    tool: plugin.demo.inspect
  - id: route_ok
    type: condition
    condition:
      input: "{{ .steps.inspect.stdout }}"
      operator: contains
      values: ["OK"]
  - id: apply
    type: tool
    tool: plugin.demo.apply
  - id: notify_skip
    type: tool
    tool: plugin.demo.greet
edges:
  - from: inspect
    to: route_ok
  - from: route_ok
    to: apply
    outcome: true
  - from: route_ok
    to: notify_skip
    outcome: false
```

**External references**

- [n8n If node documentation](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.if/) — relevant as the direct visual true/false router precedent.
- [n8n Switch node documentation](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.switch/) — relevant for later multi-case routing beyond MVP true/false.
- [n8n merging data](https://docs.n8n.io/flow-logic/merging/) — relevant for fan-in/merge behavior after branches.

#### 2. Node-RED: Switch node routes a message to numbered output ports

**Convention**

- Node-RED flows are visual nodes connected by wires.
- The `Switch` node owns a list of rules and routes a message to one or more output ports.
- A rule can compare against message properties, flow/global context, or other structured values.

**Why it exists**

- In message-flow systems, each message reaches a router; the router chooses which outgoing wire(s) receive the message.
- Numbered/labeled outputs are easier to debug in a visual canvas than embedding conditions in arbitrary downstream nodes.
- Router nodes make the "no matching branch" case visible: a message may go to no output.

**Shape mapped to this repo**

- This repo does not pass an n8n/Node-RED-style message object; it has workflow params and prior step context.
- The closest equivalent to message properties is the existing template context: `{{ .steps.<id>.stdout }}`, `{{ .steps.<id>.stderr }}`, and `{{ .steps.<id>.params.<name> }}`.
- For MVP, a binary condition node is enough; multi-output switch/case can be a compatible extension using `case` or string `outcome` edge labels.

**External references**

- [Node-RED concepts](https://nodered.org/docs/user-guide/concepts) — explains visual nodes, wires, and node output ports.
- [Node-RED cookbook: route on context](https://cookbook.nodered.org/basic/route-on-context) — shows using a Switch node to route or stop messages based on context.

#### 3. GitHub Actions: dependency topology plus `if` guards on jobs/steps

**Convention**

- Jobs are independent units; `needs` creates dependency edges and fan-in.
- Jobs/steps use `if` metadata for conditional execution.
- Skipped/failed dependencies affect downstream jobs unless expressions such as `always()` override defaults.

**Why it exists**

- YAML-first CI workflows prioritize concise declarations over visual routing nodes.
- A condition next to the job/step makes each unit self-contained.
- It avoids adding non-execution router nodes to the graph.

**Comparable lesson for this repo**

- This is a good comparison for node guard semantics, but it is less aligned with the requested n8n-like visual condition nodes.
- It reinforces that serial/parallel should stay dependency topology (`edges`/`depends_on`), not special "serial" or "parallel" nodes.
- It also shows that skipped/fan-in semantics must be explicit, because downstream behavior after skipped branches is user-visible.

**External references**

- [Workflow syntax for GitHub Actions](https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions) — relevant for `needs`, `if`, and dependency/skipped behavior conventions.

#### 4. Argo Workflows / Tekton Pipelines: DAG dependencies plus structured `when`

**Convention**

- Argo DAG tasks and Tekton tasks express ordering with dependency metadata (`dependencies`, `runAfter`, or result references).
- Conditional execution lives on a task/step as `when`.
- Tekton uses a structured predicate shape (`input`, `operator`, `values`) rather than arbitrary shell code.
- Argo conditionals often reference previous step outputs.

**Why it exists**

- Kubernetes-native workflow systems need validation-friendly YAML/CRD schemas.
- Structured predicates can be validated and surfaced in status more safely than arbitrary scripting.
- Output references are first-class because many pipeline decisions depend on previous task results.

**Comparable lesson for this repo**

- The repo should keep dependency topology separate from condition evaluation.
- The condition predicate should be data-only and enumerated, even if the visual shape is n8n-like.
- Previous-output conditions should reference the repo's existing context paths rather than introducing a new output model first.

**External references**

- [Argo DAG walkthrough](https://argo-workflows.readthedocs.io/en/latest/walk-through/dag/) — relevant for DAG dependency and parallel-readiness behavior.
- [Argo conditionals walkthrough](https://argo-workflows.readthedocs.io/en/latest/walk-through/conditionals/) — relevant for output-based `when` expressions.
- [Tekton Pipelines documentation](https://tekton.dev/docs/pipelines/pipelines/) — relevant for `runAfter`, task `when` expressions, and structured `input`/`operator`/`values` predicates.

### Common Conventions

1. **Serial execution is topology, not a node type.**
   - GitHub Actions uses `needs`; Argo uses DAG dependencies; Tekton uses `runAfter` or result references; n8n/Node-RED use direct connections.
   - This repo already has `edges` and `depends_on`, so serial should remain `A -> B`.

2. **Parallel shape is topology; actual parallelism is scheduler behavior.**
   - Fan-out/fan-in (`A -> B`, `A -> C`, `B/C -> D`) is the common graph representation.
   - The current runner executes topological order sequentially (`internal/runner/runner.go:92-123`), so runtime parallelism is a separate scheduler concern from adding condition nodes.

3. **Visual tools favor explicit router nodes.**
   - n8n and Node-RED expose If/Switch-style nodes because visual users need to see the decision point.
   - Edge labels/ports carry simple route identity (`true`, `false`, or a case name), while the condition itself belongs to the router node.

4. **YAML/CI tools favor guarded tasks.**
   - GitHub Actions, Argo, and Tekton attach `if`/`when` to jobs/tasks.
   - This is smaller in schema terms, but branch intent is less visible in React Flow.

5. **Safe predicates are structured and enumerable.**
   - Tekton's `input`/`operator`/`values` style is a useful precedent.
   - For this ops framework, MVP conditions should not execute shell, JS, Python, or arbitrary expressions.
   - Operators should be explicit and testable: `eq`, `neq`, `contains`, `not_contains`, `in`, `not_in`, `exists`, `empty` are enough for initial output routing.

6. **Skipped and fan-in behavior must be part of the contract.**
   - Conditional routing creates inactive branches.
   - Downstream nodes need deterministic rules for whether skipped/inactive upstreams block execution.
   - Run records need to expose skipped status and condition results so CLI/API/Web reports are explainable.

### Recommended MVP Schema / Execution Semantics for This Repo

#### MVP schema

Use typed workflow nodes and branch-labeled edges. Keep existing tool-node fields for backward compatibility by treating missing `type` as `tool` when `tool` is present.

```yaml
nodes:
  - id: inspect
    type: tool
    name: Inspect target
    tool: plugin.demo.inspect
    params:
      target: "{{ .target }}"

  - id: route_healthy
    type: condition
    name: Route by inspect output
    condition:
      input: "{{ .steps.inspect.stdout }}"
      operator: contains
      values: ["healthy"]

  - id: apply
    type: tool
    tool: plugin.demo.apply
    params:
      target: "{{ .target }}"

  - id: notify_skip
    type: tool
    tool: plugin.demo.greet
    params:
      message: "inspect did not report healthy"

edges:
  - from: inspect
    to: route_healthy
  - from: route_healthy
    to: apply
    outcome: true
  - from: route_healthy
    to: notify_skip
    outcome: false
```

Suggested Go model extension shape:

```go
type WorkflowNode struct {
    ID        string                 `yaml:"id" json:"id"`
    Type      string                 `yaml:"type" json:"type"` // default: tool when Tool is set
    Name      string                 `yaml:"name" json:"name"`
    Tool      string                 `yaml:"tool" json:"tool"`
    Condition *Condition             `yaml:"condition" json:"condition"`
    DependsOn []string               `yaml:"depends_on" json:"depends_on"`
    Params    map[string]interface{} `yaml:"params" json:"params"`
    Optional  bool                   `yaml:"optional" json:"optional"`
    Timeout   string                 `yaml:"timeout" json:"timeout"`
    Confirm   bool                   `yaml:"confirm" json:"confirm"`
    OnFailure string                 `yaml:"on_failure" json:"on_failure"`
}

type Condition struct {
    Input    string   `yaml:"input" json:"input"`
    Operator string   `yaml:"operator" json:"operator"`
    Values   []string `yaml:"values" json:"values"`
}

type WorkflowEdge struct {
    From    string `yaml:"from" json:"from"`
    To      string `yaml:"to" json:"to"`
    Outcome string `yaml:"outcome" json:"outcome"` // only meaningful when From is a condition node
}
```

MVP validation rules:

- Node type defaults:
  - `type` empty + `tool` set => `tool`.
  - `type` empty + `condition` set => `condition` may be accepted only if desired for YAML ergonomics; otherwise require explicit `type: condition`.
- `tool` node:
  - `tool` is required and must reference a registered tool.
  - `condition` should be empty.
  - high-risk confirmation behavior remains unchanged.
- `condition` node:
  - `tool` must be empty.
  - `condition.input` and `condition.operator` are required.
  - `condition.operator` must be in an allowlist.
  - for binary MVP, outgoing edges from a condition node should use `outcome: true` or `outcome: false`.
  - at most one outgoing edge per outcome unless multi-cast is explicitly allowed; if multi-cast is allowed, document that all matching edges activate.
- Edge rules:
  - all `from`/`to` endpoints must exist, as today.
  - cycles remain invalid, as today.
  - edges from normal tool nodes should not set `outcome` in MVP, or validation should ignore/forbid it consistently.
  - edges from condition nodes should set an allowed `outcome`.

#### MVP condition evaluation

Condition inputs should be rendered from the existing workflow context, then compared as strings for MVP:

- `input: "{{ .target }}"` references workflow params.
- `input: "{{ .steps.inspect.stdout }}"` references prior stdout.
- `input: "{{ .steps.inspect.stderr }}"` references prior stderr.
- `input: "{{ .steps.inspect.params.target }}"` references prior resolved params.

Operator semantics:

| Operator | MVP behavior |
|---|---|
| `eq` | rendered input equals any value in `values` after string conversion |
| `neq` | rendered input equals none of `values` |
| `contains` | rendered input contains any value in `values` |
| `not_contains` | rendered input contains none of `values` |
| `in` | alias/same behavior as `eq` with multiple values |
| `not_in` | alias/same behavior as `neq` with multiple values |
| `exists` | input reference resolves to a non-missing value; for rendered strings, non-empty is acceptable if missing-vs-empty is not tracked yet |
| `empty` | rendered input is empty after trimming |

Avoid arbitrary expression strings such as `{{ .steps.inspect.stdout }} == "healthy" && ...` in MVP. If compound conditions are needed later, add structured `all`/`any` arrays rather than allowing script execution.

#### MVP execution semantics

The runner can still use topological order initially, but execution must become branch-aware:

1. Build incoming/outgoing edge indexes from `wf.Edges`.
2. Track node state: `pending`, `running`, `succeeded`, `failed`, `skipped`.
3. Track active edges:
   - edges out of a successful `tool` node are active by default.
   - edges out of a `condition` node are active only when their `outcome` matches the condition result.
   - non-matching condition edges are inactive and should be recorded as skipped route decisions.
4. A node is runnable when:
   - all active incoming predecessor nodes have completed successfully, and
   - at least one incoming edge is active, or the node has no incoming edges and is a start node.
5. A node is skipped when:
   - all incoming edges are resolved and none are active, or
   - an active required predecessor failed and current MVP stop-on-first-failure semantics abort the workflow.
6. Condition node execution:
   - does not call a shell tool.
   - renders/evaluates its `condition` against the current workflow context.
   - records a `StepRecord` (or equivalent) with `Status: succeeded`, a `Type: condition`/`Tool: ""`, and condition result details if the run record model is extended.
   - activates matching outgoing branch edge(s).
7. Tool node execution:
   - remains under `internal/runner`.
   - preserves existing parameter merge and `resolveStepParams` behavior.
   - calls `addStepContext` only after success so downstream conditions use completed outputs.
8. Fan-in default:
   - active incoming edges are required; inactive/skipped incoming edges do not block.
   - this allows `true` and `false` branches to merge into a common downstream node after whichever branch ran.
   - if a node has two active incoming edges, it waits for both.
9. Failure default:
   - keep current MVP stop-on-first-failed-step behavior unless `on_failure`/`optional` already has documented runtime semantics.
   - skipped due to false branch should not be treated as failure.

This gives n8n-like branch selection while staying compatible with the repo's current DAG model and sequential topological runner. True concurrent scheduling can remain separate from conditional routing.

### Related Specs

- `.trellis/spec/backend/quality-guidelines.md` — execution must go through `internal/runner`; high-risk confirmations remain runner-enforced; workflows currently stop on first failed step.
- `.trellis/spec/backend/directory-structure.md` — workflow YAML contributed by plugins is normalized into `WorkflowConfig`; YAML model belongs in `internal/config`.
- `.trellis/spec/frontend/directory-structure.md` — Web UI must be generated from backend YAML/plugin metadata and not hard-code tools/workflows.
- `.trellis/tasks/04-27-04-27-workflow-control-flow-nodes/prd.md` — prior control-flow planning already selected typed condition nodes and branch-labeled edges for visual canvas compatibility.
- `.trellis/tasks/04-27-04-27-workflow-control-flow-nodes/research/control-flow-patterns.md` — related research on serial/parallel/conditional control-flow patterns.

### External References

- [n8n If node](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.if/) — direct reference for visual true/false conditional routing.
- [n8n Switch node](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.switch/) — reference for future multi-case route selection.
- [n8n merging data](https://docs.n8n.io/flow-logic/merging/) — reference for combining streams after branch execution.
- [Node-RED concepts](https://nodered.org/docs/user-guide/concepts) — reference for nodes, wires, and output-port-based visual flow routing.
- [Node-RED cookbook: route on context](https://cookbook.nodered.org/basic/route-on-context) — reference for Switch-node routing based on context.
- [GitHub Actions workflow syntax](https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions) — comparable YAML-first pattern for `needs` topology and `if` guards.
- [Argo Workflows DAG walkthrough](https://argo-workflows.readthedocs.io/en/latest/walk-through/dag/) — comparable DAG dependency and parallel-readiness model.
- [Argo Workflows conditionals walkthrough](https://argo-workflows.readthedocs.io/en/latest/walk-through/conditionals/) — comparable output-based conditional execution model.
- [Tekton Pipelines documentation](https://tekton.dev/docs/pipelines/pipelines/) — comparable structured `when` expression and `runAfter` dependency model.

## Caveats / Not Found

- No external web-search tool was available in this agent session, so external references are based on known public documentation URLs and the repo's prior persisted research file rather than freshly fetched pages.
- The specified task directory had no existing PRD/research markdown at the time of this research; only `task.json` was present.
- Current repo code does not yet contain condition-node schema fields or branch-aware runner logic; existing validation rejects nodes without `tool`.
- Current run record `StepRecord` has no fields for node type, skipped reason, condition result, or activated branch; those are needed for explainable condition-node runtime reporting.
- Current workflow context stores stdout/stderr as strings read from log files; structured JSON output routing would need an additional parsing contract if required later.
- Fan-in semantics are the main ambiguity: the recommended MVP treats inactive conditional edges as non-blocking and active incoming edges as required, but this must be documented for CLI/API/Web consistency.

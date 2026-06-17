# Workflow Conditional Nodes

## Scenario: Multi-branch Switch/case condition nodes

### 1. Scope / Trigger

- Trigger: workflow schema, runner behavior, API round-trip, and React Flow editor all changed to support condition-based routing.
- Applies when adding or modifying workflow node schema, workflow validation, runner scheduling/skip behavior, server workflow save/detail/run APIs, or workflow editor node/edge mapping.
- Condition routing must stay data-only and validation-friendly; do not execute arbitrary Shell/JS/Python expressions.

### 2. Signatures

Backend structs in `internal/config/types.go`:

```go
type WorkflowNode struct {
    ID        string                 `yaml:"id" json:"id"`
    Type      string                 `yaml:"type" json:"type"`
    Name      string                 `yaml:"name" json:"name"`
    Tool      string                 `yaml:"tool" json:"tool"`
    Condition WorkflowCondition      `yaml:"condition" json:"condition"`
    Params    map[string]interface{} `yaml:"params" json:"params"`
}

type WorkflowCondition struct {
    Input       string          `yaml:"input" json:"input"`
    Cases       []ConditionCase `yaml:"cases" json:"cases"`
    DefaultCase string          `yaml:"default_case" json:"default_case"`
}

type ConditionCase struct {
    ID       string   `yaml:"id" json:"id"`
    Name     string   `yaml:"name" json:"name"`
    Operator string   `yaml:"operator" json:"operator"`
    Values   []string `yaml:"values" json:"values"`
}

type WorkflowEdge struct {
    From string `yaml:"from" json:"from"`
    To   string `yaml:"to" json:"to"`
    Case string `yaml:"case" json:"case"`
}
```

Runtime record fields in `internal/runner.StepRecord`:

```go
Type           string `json:"type,omitempty"`
ConditionInput string `json:"condition_input,omitempty"`
MatchedCase    string `json:"matched_case,omitempty"`
SkippedReason  string `json:"skipped_reason,omitempty"`
```

### 3. Contracts

YAML/API workflow payload:

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

Contract details:

- `type` omitted + `tool` set means `tool` for backward compatibility.
- `type: tool` requires `tool` and must not use any `condition` fields, including `condition.input`, `condition.cases`, or `condition.default_case`.
- `type: condition` must not set `tool`; it owns `condition.input`, `condition.cases`, and optional `condition.default_case`.
- Condition input is rendered with existing workflow context: `{{ .param }}`, `{{ .steps.<id>.stdout }}`, `{{ .steps.<id>.stderr }}`, `{{ .steps.<id>.params.<name> }}`.
- Supported operators: `eq`, `neq`, `contains`, `not_contains`, `in`, `not_in`, `exists`, `empty`.
- Edges from condition nodes must set `case` to a known case ID or `default` when `default_case` is configured.
- Condition nodes may have zero outgoing edges; this represents a terminal routing/control node and must pass validation when the node's own condition schema is valid.
- Edges from non-condition nodes must not set `case`.
- Runner evaluates cases in order and activates the first matching case; if none match, it activates `default` when configured.
- If no case matches and no default branch is configured, the condition step succeeds with an empty `matched_case`; all outgoing condition branches stay inactive and are recorded as skipped when they become reachable records.
- Inactive conditional branches are recorded as `skipped` and must not fail the workflow.
- Fan-in rule: inactive condition edges do not block; active incoming edges must complete successfully.
- CLI/menu presentation must render condition step records in a human-readable summary while keeping `RunRecord` JSON fields unchanged: show step type `编排节点/条件分支`, `condition_input`, `matched_case`, and `skipped_reason` when present.
- Workflow-level confirmation scans must ignore condition/control nodes and only prompt for registered tool nodes.
- `type: loop` is an executable control node that owns its repeated tool config in `loop.tool`, `loop.params`, and `loop.max_iterations`; new workflows must not model loop execution by pointing `loop.target` at a separate canvas tool node.
- Loop nodes must not set top-level `tool` or `condition`. `loop.tool` must reference a registered tool, and `loop.max_iterations` must be between 1 and 20.
- Loop nodes must not set `loop.tool` and `loop.target` at the same time; mixed configuration is rejected to avoid ambiguous execution sources.
- Runner executes `loop.tool` sequentially for `loop.max_iterations`; `loop.params` are rendered with the same workflow context as tool-node params. Any failed iteration stops the loop and fails the workflow.
- Defensive backward compatibility may read old `loop.target` drafts, but examples, UI guidance, validation tests, and docs should use embedded `loop.tool` + `loop.params` only.

### 4. Validation & Error Matrix

| Condition | Expected behavior |
|---|---|
| tool node has empty `tool` | reject with node-specific Chinese validation error |
| tool node references missing tool | reject before execution |
| tool node has any `condition` field populated | reject; tool nodes cannot carry dormant control-node config |
| condition node sets `tool` | reject; condition nodes are not shell execution nodes |
| condition node missing `condition.input` | reject |
| condition node has no cases | reject |
| duplicate case ID | reject |
| case ID is `default` | reject; `default` is reserved for fallback routing |
| illegal operator | reject; only allow listed operators |
| valid condition node has no outgoing edges | accept; terminal condition nodes are allowed |
| edge from condition missing `case` | reject |
| edge from condition references unknown case | reject |
| edge uses `case: default` without `default_case` | reject |
| edge from non-condition has `case` | reject |
| graph has missing endpoint or cycle | reject via existing DAG validation |

### 5. Good/Base/Bad Cases

- Good: tool emits stdout `OK`; condition `contains ["OK"]` matches `ok`; only `case: ok` outgoing branch runs, other branches are skipped.
- Base: no case matches and `default_case: default` exists; only `case: default` branch runs.
- Base: no case matches and no default is configured; the condition step succeeds with empty `matched_case`, and no conditional outgoing branch runs.
- Base: a valid condition node has no outgoing edges; validation accepts it as a terminal control node.
- Bad: condition edge has `case: warn` but node cases only contain `ok`; validation fails before save/run.

### 6. Tests Required

- Registry validation tests:
  - tool nodes without `type` remain valid.
  - tool nodes reject any populated `condition` field, including `default_case` alone.
  - condition node rejects missing input, empty cases, duplicate IDs, illegal operators.
  - condition outgoing edges require valid `case`; non-condition edges reject `case`.
  - valid condition node with zero outgoing edges remains valid.
- Runner tests:
  - previous stdout renders into condition input.
  - first matching case activates only matching branch.
  - default branch executes when no case matches.
  - no-match/no-default leaves `matched_case` empty and activates no outgoing branch.
  - inactive branches produce `skipped` records and do not fail workflow.
  - fan-in after an active branch executes deterministically.
- Server/API tests:
  - workflow save/detail round-trips `type`, `condition`, cases, `default_case`, and edge `case`.
- Frontend build:
  - `npm run build --prefix web` must pass and update embedded assets.

### 7. Wrong vs Correct

#### Wrong

```yaml
nodes:
  - id: route
    type: condition
    tool: plugin.demo.greet
    condition:
      input: "{{ .steps.inspect.stdout }}"
edges:
  - from: route
    to: notify
```

Why wrong:

- Condition node incorrectly sets `tool`.
- It has no cases.
- The outgoing condition edge has no `case`.

#### Correct

```yaml
nodes:
  - id: route
    type: condition
    condition:
      input: "{{ .steps.inspect.stdout }}"
      cases:
        - id: ok
          name: 正常
          operator: contains
          values: ["OK"]
      default_case: default
edges:
  - from: route
    to: apply
    case: ok
  - from: route
    to: notify
    case: default
```

Why correct:

- Condition logic is data-only and visible in the workflow schema.
- Every condition edge maps to a valid case label.
- Default routing is explicit and testable.

## Frontend editor contract

- Condition nodes use React Flow node type `conditionNode` and must be visually distinct from tool nodes.
- The editor must expose controls for condition input, case ID/name/operator/values, and default branch configuration.
- Outgoing edges from condition nodes must display their selected case label.
- Save/run/validate preflight should catch missing input, empty cases, duplicate case IDs, illegal operators, missing edge case, and dangling case references before calling the backend.
- Load/save mapping must preserve all condition fields and edge `case` labels; do not drop unknown branch metadata during round-trip.

## Scenario: Rerun a Workflow Node in an Existing Run

### 1. Scope / Trigger

- Trigger: adding or changing node-level workflow rerun behavior, run record reuse, or Web/API rerun actions.
- Applies when changing `/api/runs/{runID}/nodes/{nodeID}/rerun`, `runner.RerunWorkflowNode*`, run record persistence, or canvas rerun UI.

### 2. Signatures

- API: `POST /api/runs/{runID}/nodes/{nodeID}/rerun`
- Request body reuses the workflow run request shape: `workflow?`, `params?`, and `confirm`.
- Runner entrypoints:
  - `RerunWorkflowNode(ctx, wf, record, runDir, nodeID, confirmed, out, errOut)`
  - `RerunWorkflowNodeWithParams(ctx, wf, record, runDir, nodeID, params, confirmed, out, errOut)`

### 3. Contracts

- The route is POST-only; non-POST mutating methods must return `405 method not allowed`.
- The target run must already exist and must not currently be active or recorded as `running`.
- If a workflow draft is supplied, `workflow.id` must match the original run record target.
- Rerun reuses the same `runID` and run directory, removes records/log directories for the selected node and its downstream nodes, then executes only that affected subgraph.
- Unaffected successful upstream steps rebuild workflow context from existing step logs and upload result files.
- Upload nodes inside the affected subgraph require an existing reusable upload result; rerun must fail clearly instead of silently waiting for a new upload.
- Confirmation checks still apply to workflow/tool confirmation requirements.

### 4. Validation & Error Matrix

| Condition | Expected behavior |
|---|---|
| Non-POST mutating rerun request | `405 method not allowed` |
| Missing run ID or node ID | `404 not found` |
| Run is active or `running` | reject with readable Chinese conflict/error |
| Run record is not a workflow | reject before execution |
| Requested node does not exist in workflow | reject with node-specific Chinese error |
| Supplied workflow ID differs from run target | reject with workflow mismatch error |
| Required workflow params are missing after override merge | reject before modifying affected steps |
| Affected upload node has no reusable upload result | fail that node and return a clear upload-result error |

### 5. Good/Base/Bad Cases

- Good: rerun node `second` in a completed `first -> second -> third` workflow; `first` logs remain unchanged and `second`/`third` are rewritten.
- Base: rerun with new workflow params; affected nodes resolve the new params while unaffected upstream logs remain intact.
- Bad: `PUT /api/runs/run-1/nodes/step-1/rerun` must not trigger rerun execution or request-body decoding.

### 6. Tests Required

- API test asserts rerun overwrites the same run ID and returns updated run detail.
- API test asserts params override affects rerun output.
- API test asserts non-POST mutating methods return `405`.
- Runner test asserts only selected/downstream nodes are removed and rerun.
- Runner test asserts affected upload nodes without reusable upload results fail explicitly.

### 7. Wrong vs Correct

#### Wrong

```go
if strings.Contains(path, "/rerun") {
    handleRunNodeRerun(w, req, state, reg, runID, nodeID)
}
```

Why wrong:

- It can route non-POST methods into a mutating operation.
- It does not preserve the existing `/api/runs/{id}` read contract.

#### Correct

```go
if req.Method == http.MethodPost {
    if runID, nodeID, ok := parseRunNodeRerunPath(id); ok {
        handleRunNodeRerun(w, req, state, reg, runID, nodeID)
        return
    }
}
```

Why correct:

- Rerun remains an explicit mutating POST action.
- GET requests keep their existing run-detail behavior.

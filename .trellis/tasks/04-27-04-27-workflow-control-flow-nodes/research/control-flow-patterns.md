# Research: workflow control-flow modeling patterns

- **Query**: Research workflow engine/control-flow modeling patterns for a YAML + DAG based ops automation tool with a React Flow visual editor. Focus on serial, parallel, conditional branching; whether conditions live on edges, nodes, or step metadata; keeping YAML/JSON schema simple and safe; and MVP approach best fitting this repo.
- **Scope**: mixed
- **Date**: 2026-04-27

## Findings

### Files Found

| File Path | Description |
|---|---|
| `internal/config/types.go` | Current workflow schema: `WorkflowConfig` has `nodes`, `edges`, legacy `steps`; `WorkflowNode` is tool-centric; `WorkflowEdge` is only `from`/`to` (`internal/config/types.go:141-170`). |
| `internal/config/load.go` | Workflow normalization maps legacy `steps` to `nodes`, builds edges from `depends_on`, and defaults empty edges to sequential node order (`internal/config/load.go:122-149`). |
| `internal/registry/validate.go` | Workflow validation requires every node to have a tool, validates edge endpoints, then topologically orders the DAG (`internal/registry/validate.go:19-43`, `internal/registry/validate.go:46-95`). |
| `internal/runner/runner.go` | Runner topologically orders nodes then executes them sequentially in a `for` loop; workflow context accumulates step output after each successful node (`internal/runner/runner.go:92-123`). |
| `web/src/main.jsx` | React Flow editor uses tool nodes only, stores React Flow edges as workflow `{from,to}`, and builds workflow draft without condition fields (`web/src/main.jsx:369-413`, `web/src/main.jsx:820-833`). |
| `workflows/demo-mytest.yaml` | Current saved workflow example with tool nodes and simple `edges: [{from,to}]` (`workflows/demo-mytest.yaml:8-32`). |
| `.trellis/spec/backend/quality-guidelines.md` | Project constraints: all execution through `internal/runner`; high-risk confirmation is a runner backstop; MVP workflows stop on first failed step (`.trellis/spec/backend/quality-guidelines.md:22-34`). |
| `.trellis/spec/backend/directory-structure.md` | Plugin-first workflow contract: plugin workflow YAML is normalized into `WorkflowConfig`; nodes reference registered tool IDs; YAML structures stay in `internal/config` (`.trellis/spec/backend/directory-structure.md:112-147`). |
| `.trellis/spec/frontend/directory-structure.md` | UI contract: React UI is generated from backend YAML/plugin metadata and should not hard-code tools/workflows (`.trellis/spec/frontend/directory-structure.md:82-87`). |

### Current Repo Baseline

- The persisted model already represents serial and fan-out/fan-in structure as a DAG: `nodes` plus `edges`.
- Current schema keeps control-flow out of edges: `WorkflowEdge` has only `From` and `To` (`internal/config/types.go:167-170`).
- Current node schema is tool-only: validation rejects a node with empty `tool` (`internal/registry/validate.go:35-40`).
- `NormalizeWorkflow` already treats explicit `depends_on` as edge metadata and defaults a list of nodes into a serial chain when no edges are provided (`internal/config/load.go:122-149`).
- Current execution is deterministic sequential topological order, not concurrent parallel execution (`internal/runner/runner.go:92-123`).
- React Flow is already the right UI primitive for fan-out/fan-in: `onConnect` adds a visual edge (`web/src/main.jsx:410-413`), `loadWorkflow` maps `{from,to}` into React Flow `source`/`target` (`web/src/main.jsx:424-425`), and `buildWorkflowDraft` maps them back to YAML (`web/src/main.jsx:820-833`).

### Comparable Patterns

#### 1. GitHub Actions: job metadata for dependency and condition

**Model**

- Serial/parallel:
  - Jobs run in parallel by default.
  - Sequential order is declared with `jobs.<job_id>.needs`.
  - A job can need multiple jobs, creating fan-in.
- Conditional branching:
  - Conditions live on job or step metadata via `if`.
  - Dependency behavior matters: skipped/failed jobs skip dependent jobs unless the dependent job uses expressions such as `always()`.

**Relevant external documentation**

- [Workflow syntax for GitHub Actions](https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions) — docs state that a workflow run is made up of jobs that run in parallel by default, and sequential execution is declared with `jobs.<job_id>.needs`; docs also describe `jobs.<job_id>.if` for conditional execution.

**Pattern shape**

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
  test:
    needs: build
    if: ${{ success() }}
  deploy:
    needs: [build, test]
    if: ${{ github.ref == 'refs/heads/main' }}
```

**Implications for this repo**

- This is closest to the current `depends_on`/`edges` model if conditions are added as node metadata.
- It keeps edges simple and makes each node responsible for its own guard.
- It is less visually explicit than router/gateway nodes because the branch decision is hidden inside downstream nodes.

#### 2. Argo Workflows: DAG task dependencies plus task/step `when`

**Model**

- Serial/parallel:
  - Argo supports a DAG template where each task specifies dependencies.
  - Tasks with no unmet dependencies can run concurrently; the Argo DAG documentation uses a diamond example: A first, B/C in parallel, D after B/C.
  - Argo also has a `steps` syntax where nested lists can represent serial stages and parallel steps within a stage.
- Conditional branching:
  - Conditions live on the step/task metadata with `when`.
  - Expressions may reference previous step outputs, for example `{{steps.flip-coin.outputs.result}} == heads`.

**Relevant external documentation**

- [Argo DAG walkthrough](https://argo-workflows.readthedocs.io/en/latest/walk-through/dag/) — describes DAGs as dependencies of tasks, simpler for complex workflows and enabling maximum parallelism.
- [Argo conditionals walkthrough](https://argo-workflows.readthedocs.io/en/latest/walk-through/conditionals/) — describes conditional execution with `when`, including output-based conditions.

**Pattern shape**

```yaml
dag:
  tasks:
    - name: A
      template: echo
    - name: B
      dependencies: [A]
    - name: C
      dependencies: [A]
    - name: D
      dependencies: [B, C]
```

```yaml
- name: heads
  template: heads
  when: "{{steps.flip-coin.outputs.result}} == heads"
```

**Implications for this repo**

- Argo supports the repo’s current DAG data shape well: dependencies are still graph structure, not separate “parallel” declarations.
- Conditional `when` on nodes would be a small schema extension.
- Argo expressions are powerful; for an ops tool, a restricted condition DSL is safer than importing a broad expression language for MVP.

#### 3. Tekton Pipelines: task `runAfter` plus task `when`

**Model**

- Serial/parallel:
  - A pipeline is a list of tasks.
  - Specific ordering uses `runAfter` when there is no data-result dependency.
  - Result references can also imply ordering because a task consumes another task’s result.
  - Tasks without ordering constraints can be scheduled in parallel.
- Conditional branching:
  - Conditions live on task metadata as `when` expressions.
  - `when` uses a structured shape: `input`, `operator`, `values`.
  - Tekton docs explicitly show skipped tasks when `when` evaluates false.

**Relevant external documentation**

- [Tekton Pipelines documentation](https://tekton.dev/docs/pipelines/pipelines/) — `runAfter` indicates a task must execute after one or more other tasks; `when` expressions guard task execution and use structured `input`/`operator`/`values` fields.

**Pattern shape**

```yaml
tasks:
  - name: test-app
    taskRef:
      name: make-test
  - name: build-app
    taskRef:
      name: kaniko-build
    runAfter:
      - test-app
```

```yaml
tasks:
  - name: first-create-file
    when:
      - input: "$(params.path)"
        operator: in
        values: ["README.md"]
```

**Implications for this repo**

- Tekton’s `when` shape is a useful schema-safety precedent: avoid free-form scripts; keep operators enumerable.
- A similar shape fits YAML/JSON and can be validated in Go without evaluating arbitrary code.
- Tekton’s distinction between ordering (`runAfter`) and guards (`when`) maps cleanly to the repo’s current `edges` plus potential node `when`.

#### 4. Node-RED / n8n: visual router nodes with output ports

**Model**

- Serial/parallel:
  - Nodes are connected by wires/edges; a node can emit to multiple downstream wires, creating fan-out.
  - Fan-in/merge is represented by connecting multiple streams into a downstream node or explicit merge node.
- Conditional branching:
  - Conditions usually live in explicit control-flow nodes such as Node-RED `Switch` or n8n `If` / `Switch` nodes.
  - Edges/wires are often associated with output ports such as true/false or numbered switch cases, but the condition definition is owned by the router node.

**Relevant external documentation**

- [Node-RED concepts](https://nodered.org/docs/user-guide/concepts) — nodes are connected by wires and wires represent how messages pass through the flow; nodes may have as many output ports as required.
- [Node-RED route on context cookbook](https://cookbook.nodered.org/basic/route-on-context) — uses a `Switch` node to route a message to different flows or stop it entirely according to context.
- [n8n If node](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.if/) — `If` node splits workflow conditionally, with true/false data streams; more than two outputs use a `Switch` node.
- [n8n merging data](https://docs.n8n.io/flow-logic/merging/) — separate streams can be combined again with a Merge node.

**Pattern shape**

```json
{
  "nodes": [
    {"id": "check", "type": "condition", "condition": {"left": "{{ .env }}", "operator": "eq", "right": "prod"}},
    {"id": "apply", "type": "tool", "tool": "plugin.demo.apply"},
    {"id": "skip", "type": "tool", "tool": "plugin.demo.greet"}
  ],
  "edges": [
    {"from": "check", "to": "apply", "outcome": "true"},
    {"from": "check", "to": "skip", "outcome": "false"}
  ]
}
```

**Implications for this repo**

- This fits a React Flow visual editor best because users can see a decision node and labeled true/false outgoing branches.
- It requires a schema change from “all nodes are tools” to typed workflow nodes.
- It also requires skip propagation, fan-in semantics, and validation rules for condition nodes and labeled outgoing edges.

### Common Conventions

1. **Serial is normally dependency topology, not a special node.**
   - GitHub Actions uses `needs`.
   - Argo uses DAG task dependencies.
   - Tekton uses `runAfter` or result references.
   - This repo already has `depends_on` and `edges`.

2. **Parallel is usually implicit from DAG readiness.**
   - Jobs/tasks without dependencies, or with the same completed parent, are parallel candidates.
   - A diamond (`A -> B`, `A -> C`, `B/C -> D`) is the common visual and YAML convention.
   - YAML does not usually need a separate `parallel: true`; the scheduler determines readiness from dependencies.

3. **Conditional branching has two dominant styles.**
   - **Node/task guard metadata**: GitHub `if`, Argo `when`, Tekton `when`. The downstream node decides whether it runs.
   - **Explicit router/gateway node**: Node-RED Switch, n8n If/Switch, BPMN gateway-like modeling. The routing node decides which outgoing branch is active.

4. **Conditions on edges are less common in code-first YAML systems but common in visual notation.**
   - Edge labels are useful in a canvas (`true`, `false`, `case=prod`).
   - Letting arbitrary expressions live directly on edges can make validation, skip propagation, and debugging harder.
   - A safer compromise is: condition definition belongs to a condition node; outgoing edges carry a simple `outcome`/`case` label.

5. **Safe schemas prefer structured predicates over arbitrary scripts.**
   - Tekton’s `input` / `operator` / `values` shape is a good precedent.
   - For an ops automation tool, MVP operators can be small and enumerable: `eq`, `neq`, `in`, `not_in`, `exists`, `empty`, optionally numeric comparisons later.
   - Inputs should be limited to workflow params and prior step outputs already exposed by the repo’s template context, e.g. `{{ .target }}` and `{{ .steps.inspect.stdout }}`.

### Trade-offs

| Option | Where condition lives | Serial/parallel model | Benefits | Costs / constraints |
|---|---|---|---|---|
| Keep current DAG; add node `when` | Tool node metadata | Existing `edges` / `depends_on`; parallel inferred from DAG readiness | Small backend/schema change; close to GitHub/Argo/Tekton; keeps `WorkflowEdge` simple | Branch intent is less visible on canvas; each downstream node needs its own guard; false guard and downstream skip semantics must be defined |
| Add explicit `condition` control node with labeled edges | Condition node owns predicate; edges carry `outcome` | Existing `edges` with optional edge label | Best visual fit for React Flow; conditions appear as canvas nodes; natural true/false and switch cases | Requires typed nodes; validation currently requires `tool`; runner must handle non-tool nodes and branch activation |
| Put expressions directly on edges | Edge metadata | Existing graph plus conditional edge activation | Branch labels are close to canvas connectors; no separate condition node | Conditions become duplicated across edges; harder to validate; fan-in and multiple conditional parents can be ambiguous |
| Add block syntax (`steps` stages with nested parallel arrays) | Step metadata or block metadata | Serial/parallel explicit by nested lists | Easy to read for hand-written YAML | Poor fit for current React Flow DAG and saved `{nodes,edges}`; introduces a second graph representation |

### Recommended Options for This Repo

#### MVP recommendation: model serial/parallel as DAG topology; add one safe conditional mechanism

1. **Do not add separate “serial” or “parallel” node types for MVP.**
   - Serial: `A -> B` edges.
   - Parallel candidate: fan-out/fan-in edges such as `A -> B`, `A -> C`, `B -> D`, `C -> D`.
   - This preserves the existing `nodes` + `edges` model and aligns with GitHub/Argo/Tekton conventions.

2. **For execution, choose a conservative MVP interpretation of parallel.**
   - The schema can represent parallel-ready branches immediately.
   - True concurrent execution can be a scheduler enhancement: execute all currently-ready nodes up to a `max_parallel` limit.
   - If safety is prioritized, the runner can keep deterministic sequential topological execution initially while the UI labels the shape as “parallel branch / fan-out” rather than promising concurrency.

3. **If the product goal is “canvas control-flow nodes”, prefer explicit `condition` nodes over edge expressions.**
   - This follows Node-RED/n8n visual conventions.
   - It keeps the predicate in one node and lets edges carry only simple labels such as `outcome: true`, `outcome: false`, or `case: prod`.
   - Required schema direction:

```yaml
nodes:
  - id: check_prod
    type: condition
    name: 是否生产环境
    condition:
      left: "{{ .env }}"
      operator: eq
      right: prod
  - id: apply
    type: tool
    tool: plugin.demo.confirmed
  - id: notify
    type: tool
    tool: plugin.demo.greet
edges:
  - from: check_prod
    to: apply
    outcome: true
  - from: check_prod
    to: notify
    outcome: false
```

4. **If minimizing backend changes is more important than canvas expressiveness, use Tekton-style `when` on tool nodes first.**
   - This can be added without introducing non-tool nodes:

```yaml
nodes:
  - id: apply
    tool: plugin.demo.confirmed
    when:
      - input: "{{ .env }}"
        operator: in
        values: ["prod"]
edges:
  - from: inspect
    to: apply
```

   - This option is closer to current validation but less visible as branching in React Flow.

5. **Keep the condition language small and data-only.**
   - Avoid shell snippets, JavaScript, Python, or unrestricted expression evaluation in workflow YAML.
   - Suggested MVP predicate schema:

```yaml
condition:
  left: "{{ .steps.inspect.stdout }}"
  operator: contains
  right: "OK"
```

   - Or Tekton-like list form:

```yaml
when:
  - input: "{{ .target }}"
    operator: in
    values: ["prod", "staging"]
```

   - Validate operators, operand types, and referenced node IDs in Go before running.

6. **Preserve compatibility with current `depends_on`.**
   - Keep `edges` as the persisted canonical graph for React Flow.
   - Continue accepting `depends_on` as author-friendly shorthand, normalized into edges.
   - If edge labels are introduced, `depends_on` can remain unconditional-only.

### Risks

1. **Parallel terminology risk**
   - A visual fan-out looks parallel, but the current runner executes sequentially. UI/help text should distinguish “parallel branch topology” from “concurrent execution” unless the scheduler is changed.

2. **Skip propagation risk**
   - If a condition skips a node, dependent nodes need deterministic rules:
     - default: skip descendants unless all required parents succeeded;
     - later option: allow fan-in policies such as `all_success`, `any_success`, or `always`.
   - GitHub Actions and Tekton both show that skipped dependency behavior becomes user-visible quickly.

3. **Fan-in ambiguity risk**
   - If one branch runs and another is skipped, a downstream merge node/tool must know whether skipped parents are acceptable.
   - MVP should use the safest rule: a node runs only when all unconditional required upstream nodes succeeded and all conditional upstream requirements that were selected have completed.

4. **Expression safety risk**
   - Arbitrary expressions can become an injection and portability problem for an ops tool.
   - Structured conditions with enumerable operators are safer and easier to validate, serialize, and render in the UI.

5. **Typed node migration risk**
   - Explicit condition nodes require changing validation that currently rejects nodes without `tool` (`internal/registry/validate.go:35-40`).
   - Existing workflows should default to `type: tool` when `type` is omitted to preserve compatibility.

6. **Run record clarity risk**
   - Conditional and skipped nodes need run-record states beyond `running`, `failed`, and `succeeded`, e.g. `skipped`, with skip reason and evaluated condition for operator diagnostics.

### External References

- [GitHub Actions workflow syntax](https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions) — job parallel-by-default behavior, `needs` dependencies, and `if` conditions.
- [Argo Workflows DAG walkthrough](https://argo-workflows.readthedocs.io/en/latest/walk-through/dag/) — DAG task dependencies and maximum parallelism from graph readiness.
- [Argo Workflows conditionals](https://argo-workflows.readthedocs.io/en/latest/walk-through/conditionals/) — task/step `when` expressions using prior outputs.
- [Tekton Pipelines](https://tekton.dev/docs/pipelines/pipelines/) — `runAfter` ordering and structured `when` expressions with `input`, `operator`, and `values`.
- [Node-RED concepts](https://nodered.org/docs/user-guide/concepts) — node/wire visual flow model and multi-output nodes.
- [Node-RED route on context cookbook](https://cookbook.nodered.org/basic/route-on-context) — Switch node routes messages to different flows or blocks them.
- [n8n If node](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.if/) — visual true/false conditional split.
- [n8n merging data](https://docs.n8n.io/flow-logic/merging/) — merging separate data streams after a split.

### Related Specs

- `.trellis/spec/backend/quality-guidelines.md` — runner reuse, confirmation backstop, stop-on-first-failure MVP behavior.
- `.trellis/spec/backend/directory-structure.md` — plugin-first workflow contributions and module boundaries.
- `.trellis/spec/frontend/directory-structure.md` — UI generated from backend YAML/plugin metadata.

## Caveats / Not Found

- The current repo does not contain a dedicated workflow-control-flow spec beyond general backend/frontend guidelines.
- The current runner does not implement true parallel concurrency; it only topologically orders then executes sequentially.
- The current schema does not contain condition fields, typed control-flow nodes, edge labels, or skip statuses.
- External documentation was sampled from official docs pages available on 2026-04-27; exact APIs may evolve by version.

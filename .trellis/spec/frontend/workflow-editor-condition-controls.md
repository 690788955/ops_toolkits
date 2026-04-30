# Workflow Editor Condition Controls

## Scenario: Editing Switch/case condition nodes

### 1. Scope / Trigger

- Trigger: workflow editor supports `type: condition` nodes and case-labeled branch edges.
- Applies when changing `web/src/main.jsx` React Flow node mapping, condition node inspector controls, save/run/validate preflight, or edge label behavior.
- The frontend mirrors backend contracts but the backend remains source of truth for workflow validation.

### 2. Signatures

React Flow node shape:

```js
{
  id: 'route_status',
  type: 'conditionNode',
  data: {
    name: '按巡检结果分支',
    condition: {
      input: '{{ .steps.inspect.stdout }}',
      cases: [
        {id: 'ok', name: '正常', operator: 'contains', values: ['OK']}
      ],
      default_case: 'default'
    },
    onRemove
  },
  position: {x: 80, y: 120}
}
```

React Flow edge shape:

```js
{
  id: 'route_status-apply-ok',
  source: 'route_status',
  target: 'apply',
  data: {case: 'ok'},
  label: 'ok'
}
```

Saved workflow draft shape:

```js
{
  nodes: [
    {id, type: 'condition', name, condition: {input, cases, default_case}}
  ],
  edges: [
    {from: edge.source, to: edge.target, case: edge.data.case}
  ]
}
```

### 3. Contracts

- Tool nodes and condition nodes must both round-trip through `workflowNodeToFlowNode` and `buildWorkflowDraft`.
- `插件工具` and `编排节点` tabs must use one unified compact grid card system with the same density, radius, and spacing. Cards should show a plain flowchart shape cue plus the node name; do not use label-like badges, chips, decorative status panels, or pasted-on tag treatments to explain the node type.
- The workflow editor palette must use two explicit tabs: `插件工具` for plugin tools with search/tag filters, and `编排节点` for built-in control nodes. The `编排节点` tab exposes `条件分支` / `Switch / Case`, `并行分支` / `Parallel`, `合流` / `Join`, and `循环` / `Loop` as enabled cards that support click and drag-to-canvas. Do not provide duplicate condition-node add buttons in the workflow operation toolbar, and do not mix control nodes into the plugin tool list.
- Loop nodes must round-trip as `type: 'loop'` with `loop: {tool, params, max_iterations}`. The node configuration modal must allow editing display name, embedded tool selection, tool parameter mappings, and max iterations; preflight must reject missing/unknown tool and iteration counts outside 1..20. The UI must not require or guide users to create a separate target tool node on the canvas.
- Workflow editor sidebar filtering must be driven by the active sidebar category, not by the draft workflow save category: a plugin category shows only that category's tools/workflows, global shows all tools/workflows, and built-in `编排节点` cards remain visible/addable in every category because they are editor primitives rather than plugin-scoped catalog entries.
- Workflow canvas nodes should be visually simple flowchart shapes: tool nodes use a process rounded rectangle, condition nodes use a compact decision diamond cue, and parallel/join nodes use compact gateway diamond cues with simple text markers such as `+` for parallel and `∧`/equivalent concise join markers. Avoid SVG icon libraries, BPMN-level shape taxonomies, nested frames, label-like badges, chips, or pasted-on status decorations unless the runtime contract explicitly adds that semantics.
- Node details such as tool IDs, condition input summaries, case counts, status/help text, and control-node descriptions should be revealed on hover/focus/selection (for example through title text or expanding secondary text) instead of being always-visible chip-like annotations. The node name and the simple shape cue should remain the primary always-visible content.
- Condition branch case/default rows and their React Flow source handles are exceptions to hover reveal: case/default branch labels, metadata, and handles must remain always visible and clickable so users can connect branches without discovering hidden controls.
- Palette and node-picker orchestration cards should mirror the same simple shape vocabulary with small plain previews, while disabled/planned cards still must not register click, drag, or saveable payload handlers.
- Condition branch handles must remain unobstructed and easy to target; decorative decision/gateway elements must not overlap React Flow source/target handles or case row handles.
- Node parameter editors must preserve readable vertical rhythm in modal/inspector surfaces: mapping rows should stack parameter identity, source selector, and manual input on narrow panels instead of compressing selector/input controls into one tight row.
- Workflow auto layout is a frontend-only canvas operation: it may recompute React Flow `nodes[].position` for the current editor session and call `fitView`, but it must not add position fields to `buildWorkflowDraft`, change workflow schema, or affect save/run/validation semantics.
- Workflow run overlay is frontend-only canvas state: execution may temporarily inject `data.run` into display nodes and edge `className`/`data.run` into display edges, but this state must be derived from the current run response/detail and must never be written into `nodes`, `edges`, `buildWorkflowDraft`, or saved workflow files. Loading/creating/editing workflow structure or node params must clear stale run overlays.
- Canvas run overlay maps `record.steps[].id` to current node IDs when present; loop iteration IDs such as `loopID#1-targetID` are aggregate display details only and must not require matching canvas node IDs for every iteration target, because loop runtime/UI contracts may evolve independently of canvas node structure.
- The inspector for a selected condition node must let users edit:
  - display name
  - condition input template
  - cases: ID, name, operator, values
  - default branch enabled/disabled
- Case values in the UI may be edited as newline/comma-separated text, but saved state must be `[]string`.
- Outgoing condition edges must expose a case selector and render a visible label.
- If cases are renamed or deleted, existing edges must either update predictably or be flagged by preflight validation.

### 4. Validation & Error Matrix

| UI condition | Expected preflight behavior |
|---|---|
| condition input empty | block save/run/validate and show readable Chinese error |
| no cases | block |
| case ID empty | block |
| duplicate case ID | block |
| operator outside allowlist | block |
| outgoing condition edge has no case | block |
| outgoing condition edge references deleted case | block |
| non-condition edge has case metadata | block or strip consistently before save; current contract prefers block |

### 5. Good/Base/Bad Cases

- Good: user adds a condition node, creates `ok` and `warn` cases, connects outgoing edges, selects edge labels, saves, reloads, and sees the same cases and labels.
- Base: user enables default branch and connects `case: default`; workflow can save even if no normal case matches at runtime.
- Bad: user deletes case `warn` but an edge still references `warn`; preflight blocks with the affected edge/node identified.

### 6. Tests Required

- Production build must pass: `npm run build --prefix web`.
- Manual browser verification should cover:
  - add condition node
  - edit cases/default
  - connect outgoing edge and select case
  - save workflow
  - reload workflow and confirm labels survive
  - run/validate preflight errors for invalid cases
- Backend round-trip tests should cover the persisted payload because frontend tests are not yet configured.
- Manual browser verification should confirm loop control cards are enabled, can be added from the palette/node picker, open the node configuration modal, and save/reload `loop.tool`, `loop.params`, and `loop.max_iterations` unchanged.
- Source review must confirm disabled/planned control cards do not register `onClick`, `onDragStart`, or drag payload handlers; use conditional props rather than no-op handlers.
- Manual browser verification should confirm node parameter editor surfaces remain readable at their normal width: parameter mapping rows and case editors should not squeeze selector/input/textarea controls into one cramped line.

### 7. Wrong vs Correct

#### Wrong

```js
edges: edges.map(edge => ({from: edge.source, to: edge.target}))
```

Why wrong:

- Drops `edge.data.case`, so condition branch labels disappear after save/load.

#### Correct

```js
edges: edges.map(edge => ({
  from: edge.source,
  to: edge.target,
  ...(edge.data?.case ? {case: edge.data.case} : {})
}))
```

Why correct:

#### Wrong

```js
<button
  disabled={disabled}
  onDragStart={event => {
    if (disabled) return
    handleControlDragStart(event, control)
  }}
  onClick={() => control.type === 'condition' && addConditionNode()}
>
```

Why wrong:

- Disabled/planned cards still register click and drag handlers, which violates the roadmap-only contract and can drift into saveable node payload behavior.

#### Correct

```js
<button
  disabled={disabled}
  onDragStart={disabled ? undefined : event => handleControlDragStart(event, control)}
  onClick={!disabled && control.type === 'condition' ? () => addConditionNode() : undefined}
>
```

Why correct:

- Planned cards are visibly disabled and have no registered click/drag payload path until backend/runtime schemas exist.

### Design Decision: Lightweight condition node and inspector styling

**Context**: The workflow editor needs condition nodes to be easy to distinguish from tool nodes without making the canvas and inspector feel dense.

**Options Considered**:
1. Heavy visual metaphor: diamond icons, dashed inner frames, nested cards, and dense branch controls.
2. Lightweight visual distinction: small condition label, subtle accent border, compact readable summary, and branch list.

**Decision**: Use lightweight visual distinction. Keep semantic controls and branch visibility, but avoid decorative shapes and nested frames unless they carry interaction meaning.

**Consequence**: Condition nodes remain recognizable while the canvas is calmer; future style changes must not remove required condition editing fields or branch labels.

## Scenario: Canvas node configuration modal

### 1. Scope / Trigger

- Trigger: workflow editor opens node configuration from the React Flow canvas instead of making the narrow right inspector the primary editor.
- Applies when changing `WorkflowEditor` node click behavior, selected-node state, node parameter editing, condition editing, or modal/inspector layout.
- This is a frontend interaction contract only; it must not change the saved workflow schema or backend runner semantics.

### 2. Signatures

Modal state and selected node relationship:

```js
const [selectedNodeID, setSelectedNodeID] = useState('')
const [selectedEdgeID, setSelectedEdgeID] = useState('')
const [nodeConfigModalOpen, setNodeConfigModalOpen] = useState(false)
const [edgeConfigModalOpen, setEdgeConfigModalOpen] = useState(false)
const selectedNode = nodes.find(node => node.id === selectedNodeID)
const selectedEdge = edges.find(edge => edge.id === selectedEdgeID)
```

Tool node config payload remains:

```js
{
  id: 'notify_user',
  type: 'toolNode',
  data: {
    tool: 'plugin.demo.notify',
    name: '通知用户',
    params: {message: '{{ .steps.inspect.stdout }}'},
    onRemove
  }
}
```

Condition node config payload remains the condition-node shape from this spec: `data.condition.input`, `data.condition.cases`, and `data.condition.default_case`.

### 3. Contracts

- `onNodeClick` must select the clicked node, clear `selectedEdgeID`, close the node picker, and open the node configuration modal.
- The modal must render tool-node configuration with the existing parameter mapping component and condition-node configuration with the existing condition editor; do not fork a second parameter/condition editing implementation.
- Tool parameter mappings must update `node.data.params` and keep mapping sources from `buildMappingSources(workflowParameters, selectedNodeID, nodes, edges)`.
- Condition edits must update `node.data.condition` and still call edge-label synchronization for outgoing condition edges.
- The right inspector may be removed to give space back to the canvas. If removed, condition edge editing must move to a modal or canvas-local surface that reuses `EdgeInspector`.
- Workflow editor layout should prefer two primary columns after node editing moves into modals: left workflow controls plus a wide canvas. Do not keep an empty right detail column just for selection summaries.
- Context switches must close the node modal: load workflow, create workflow, click pane, click edge, delete selected node, clear selection, or open the node picker.
- Node delete buttons inside React Flow nodes must stop propagation so delete does not also trigger `onNodeClick` and reopen the modal.
- Advanced JSON editing for tool params may be explicit/apply-based; invalid JSON must not write to `data.params` and must not close the modal.

### 4. Validation & Error Matrix

| UI condition | Expected behavior |
|---|---|
| clicked tool node | select node, open modal, show tool ID and parameter mappings |
| clicked condition node | select node, open modal, show display name/input/cases/default controls |
| clicked edge while node modal open | close node modal, select edge, open edge config modal, keep case selector available for condition edges |
| clicked pane while any config modal open | close node/edge modals and clear node/edge selection |
| node deleted while modal open | close modal and remove connected edges |
| advanced params JSON invalid | show readable Chinese error, keep modal open, keep previous `data.params` |
| selected node no longer exists | reset params text and close modal |

### 5. Good/Base/Bad Cases

- Good: user clicks a tool node, maps a required param to an upstream stdout value, saves, validates, saves workflow, reloads, and sees the same param.
- Base: user clicks a condition node, edits case names/default branch in the modal, and existing outgoing edge labels update predictably.
- Bad: user clicks a node delete button and the modal opens for a node that is being deleted; this means event propagation was not stopped.

### 6. Tests Required

- Production build must pass: `npm run build --prefix web`.
- Source review must confirm node modal content reuses `ParamMappingEditor` and `ConditionEditor` rather than duplicating their logic.
- Manual browser verification should cover:
  - click tool node opens modal and edits `data.params`
  - click condition node opens modal and edits `data.condition`
  - invalid advanced JSON keeps modal open and leaves prior params intact
  - click edge opens edge config modal and still allows case selection
  - opening node picker clears node/edge selection and closes config modals
  - click node delete button removes the node without reopening modal
  - save/reload preserves tool params and condition edge labels

### 7. Wrong vs Correct

#### Wrong

```js
onNodeClick={(_, node) => {
  setSelectedNodeID(node.id)
}}
```

Why wrong:

- It selects the node but leaves edge selection and node-picker/modal state ambiguous, so the UI can show stale editors at the same time.

#### Correct

```js
function openNodeConfigModal(nodeID) {
  setSelectedNodeID(nodeID)
  setSelectedEdgeID('')
  setNodeConfigModalOpen(true)
  closeNodePicker()
}
```

Why correct:

- Node configuration has one active context: selected node plus open modal, with edge selection and node picker cleared.

#### Wrong

```js
<button onClick={() => data.onRemove(id)}>×</button>
```

Why wrong:

- The click can bubble to React Flow's node click handler and open a modal for a node that is being removed.

#### Correct

```js
<button
  className="nodeDelete nodrag nopan"
  onMouseDown={event => event.stopPropagation()}
  onClick={event => { event.stopPropagation(); data.onRemove(id) }}
>
  ×
</button>
```

Why correct:

- Delete remains a node-local action and does not trigger selection/configuration side effects.

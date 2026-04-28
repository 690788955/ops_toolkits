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
- `插件工具` and `编排节点` tabs must use one unified compact grid card system: same density, radius, spacing, and chip treatment. Orchestration cards may use only a subtle control accent/status badge, not a separate full-width panel style.
- The workflow editor palette must use two explicit tabs: `插件工具` for plugin tools with search/tag filters, and `编排节点` for built-in control nodes. The `编排节点` tab exposes `条件分支` / `Switch / Case` as the only enabled card, supporting click and drag-to-canvas; it also shows future cards `并行分支`, `合流`, and `循环` as disabled/planned so users understand the control-node roadmap. Do not provide duplicate condition-node add buttons in the workflow operation toolbar, and do not mix control nodes into the plugin tool list.
- Disabled/planned control cards must not register click handlers, drag payloads, or any saveable node payload; they are roadmap hints only until backend/runtime schemas exist.
- `conditionNode` cards must be visually distinct from `toolNode` cards.
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
- Manual browser verification should confirm planned control cards show disabled/planned state and cannot be clicked or dragged into the canvas.

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

- Preserves backend branch routing contract without adding metadata to normal edges.

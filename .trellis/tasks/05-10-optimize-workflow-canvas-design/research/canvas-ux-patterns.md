# Research: Workflow canvas UX patterns

- **Query**: Research workflow canvas UX patterns for an ops automation / DAG editor that currently uses @xyflow/react. Compare 2-4 relevant patterns/tools: ProductFlow-style card canvas, React Flow/XyFlow examples, n8n/Node-RED-style automation editors if helpful. Focus on what to borrow for our repo: node card information hierarchy, side panels vs modals, edge labels/delete affordances, run-status overlays, palette/search, auto-layout, and what NOT to do.
- **Scope**: mixed
- **Date**: 2026-05-10

## Findings

### Files Found

| File Path | Description |
|---|---|
| `web/src/main.jsx` | Current React + @xyflow/react workflow editor implementation: node components, palette, node picker, modals, edge config, run overlay, auto-layout, save/run preflight. |
| `web/src/styles.css` | Current visual treatment for canvas, node cards, hover disclosure, run status badges, edge run classes, node picker, and bottom canvas dock. |
| `web/package.json` | Declares `@xyflow/react` dependency version `^12.10.2`. |
| `.trellis/spec/frontend/workflow-editor-condition-controls.md` | Frontend contracts for condition/control nodes, card hierarchy, palette split, modal editing, auto-layout and run-overlay persistence boundaries. |
| `.trellis/spec/backend/workflow-conditional-nodes.md` | Backend schema/runtime contract for condition edges, case labels, loop node semantics, skipped branch records, and validation expectations. |

### Code Patterns

#### Current repository baseline: compact XyFlow DAG editor

- `web/package.json:5-12` uses `@xyflow/react` `^12.10.2` alongside React/Vite.
- `web/src/main.jsx:99-110` defines tool nodes as a rounded card with left/right handles, a run badge, delete affordance, primary node name, and tool ID revealed as secondary metadata.
- `web/src/main.jsx:115-158` defines condition nodes with a decision cue, always-visible branch rows, per-branch source handles, and matched-branch styling hooks.
- `web/src/main.jsx:159-179` defines control nodes with a flowchart shape cue plus concise title/help text.
- `web/src/main.jsx:1587-1618` centralizes editor state: selected node/edge, modal state, palette tab, flow instance, node picker, React Flow nodes/edges, and transient canvas run state.
- `web/src/main.jsx:1701-1729` creates `smoothstep` edges, deduplicates source/target/handle pairs, and derives condition-edge labels from source handles/cases.
- `web/src/main.jsx:2188-2405` shows the layout hierarchy: left workflow toolbar, node palette, central `ReactFlow`, empty-canvas callout, node picker, bottom canvas dock, node config modal, and edge config modal.
- `web/src/main.jsx:2476-2533` implements an in-canvas node picker plus bottom dock actions: zoom, fit view, auto-layout, add node, and run workflow.
- `web/src/main.jsx:2535-2573` uses a modal for edge configuration; non-condition edges show deletion guidance, condition edges expose a case selector and visible edge label contract.
- `web/src/main.jsx:2746-2961` builds transient run overlays from run records, injects `data.run` into display nodes, decorates edges with run classes, and keeps overlay separate from saved graph state.
- `web/src/main.jsx:3304-3390` implements frontend-only auto-layout by layering nodes by DAG depth, estimating node dimensions, and updating only `nodes[].position`.

#### Current styling hierarchy

- `web/src/styles.css:593-600` makes the canvas card a large embedded React Flow surface with a neutral background.
- `web/src/styles.css:608-649` uses simple cards with progressive disclosure: secondary tool IDs, condition summaries, and control descriptions are hidden until hover/selection.
- `web/src/styles.css:697-819` keeps condition branch rows and branch handles always visible, making branch connections discoverable.
- `web/src/styles.css:827-877` displays run status as small badges and status-colored borders/backgrounds for succeeded/failed/skipped/running states.
- `web/src/styles.css:890-905` styles run-state edges: matched/succeeded edges turn green, dimmed condition branches fade/dash, failed edges turn red.
- `web/src/styles.css:908-925` provides node delete buttons as small round controls at the card corner.
- `web/src/styles.css:995-1063` styles the node picker as a centered in-canvas overlay with search and compact item cards.
- `web/src/styles.css:1064-1095` styles a bottom floating canvas dock for navigation, layout, add-node, and run actions.

#### Existing spec constraints relevant to UX decisions

- `.trellis/spec/frontend/workflow-editor-condition-controls.md:60-68` requires a unified compact card system, two explicit palette tabs, simple flowchart shape vocabulary, and hover/focus/selection reveal for secondary metadata; branch rows/handles are exceptions and must remain always visible.
- `.trellis/spec/frontend/workflow-editor-condition-controls.md:70-73` states auto-layout and run overlays are frontend-only state and must not be written into saved workflow drafts.
- `.trellis/spec/frontend/workflow-editor-condition-controls.md:184-190` records the decision to use a node configuration modal instead of making a narrow right inspector the primary editor.
- `.trellis/spec/backend/workflow-conditional-nodes.md:95-104` defines condition input rendering, case-labeled edges, skipped inactive branches, and human-readable condition step records.
- `.trellis/spec/backend/workflow-conditional-nodes.md:105-110` requires confirmation scans to ignore condition/control nodes and loop nodes to embed their repeated tool config.

### Pattern / Tool Comparison

#### 1. ProductFlow-style card canvas pattern

Observed as a general product/workflow design pattern rather than a directly accessible primary source in this environment. The accessible search results surfaced card-flow resources, but primary ProductFlow pages returned 403.

What this pattern contributes:

- Treat nodes as readable cards, not database rows: a card should show one primary action/name, one type cue, and only a small amount of secondary context.
- Prefer progressive disclosure for metadata: show expanded details on hover/selection or in an edit surface, not as always-visible chips everywhere.
- Use clear card grouping and whitespace so non-technical users can scan the workflow as a narrative.
- Make empty states and add-node prompts part of the canvas, not only a sidebar action.

What maps well to this repo:

- The current `ToolNode`, `ConditionNode`, and `ControlNode` already follow a card-first hierarchy (`web/src/main.jsx:99-179`) and CSS hides secondary metadata until hover/selection (`web/src/styles.css:625-649`).
- The empty canvas callout and node picker (`web/src/main.jsx:2337-2355`, `web/src/styles.css:973-1063`) match card-canvas onboarding.

What not to do:

- Do not turn each node into a dashboard panel with badges, tags, logs, parameter tables, and status widgets always visible; the spec explicitly says to avoid label-like badges/chips/decorative panels for node type explanations (`.trellis/spec/frontend/workflow-editor-condition-controls.md:60-66`).
- Do not hide condition branch handles behind hover states; branch rows are functional routing controls and must remain always visible (`.trellis/spec/frontend/workflow-editor-condition-controls.md:67-69`).

#### 2. React Flow / XyFlow example pattern

Relevant React Flow/XyFlow examples and components:

- Node Toolbar example: contextual actions anchored to selected/hovered nodes.
- Edge Label Renderer example: HTML labels/actions rendered along edges, useful for editable edge labels or delete buttons.
- Dagre Tree layout example: external layout engines can compute node positions and then call fit view.
- Drag and Drop example: palette-to-canvas insertion with pointer-position placement.
- Built-ins already used or mirrored in this repo: `<MiniMap />`, `<Controls />`, `<Background />`, custom node types, handles, and `useNodesState`/`useEdgesState`.

What this pattern contributes:

- Node/edge actions can be contextual instead of permanently occupying the canvas.
- Edge labels can support richer interaction than plain SVG text when using an edge label renderer or edge toolbar.
- Layout should be an operation over current view state, not a persisted workflow schema concern.
- React Flow patterns support a dual add flow: sidebar drag/drop and in-canvas picker/insert action.

What maps well to this repo:

- Current code already has drag/drop insertion (`web/src/main.jsx:1962-1991`), a bottom dock (`web/src/main.jsx:2522-2533`), and frontend-only auto-layout (`web/src/main.jsx:1889-1894`, `3304-3390`).
- Current edge labels are visible for condition cases (`web/src/main.jsx:3288-3301`) and modally editable (`web/src/main.jsx:2535-2573`).
- Current run overlays use display-node/display-edge transforms rather than mutating saved graph data (`web/src/main.jsx:2911-2951`).

What not to do:

- Do not persist React Flow-only fields such as `position`, `data.run`, edge `className`, viewport, or layout artifacts into the workflow YAML; the frontend spec forbids that (`.trellis/spec/frontend/workflow-editor-condition-controls.md:71-73`).
- Do not add always-visible delete buttons/toolbar clutter for every edge if edge labels become dense; contextual edge actions are a better match for sparse ops DAGs.
- Do not rely solely on drag/drop; in-canvas add/search matters for laptop/trackpad users and remote desktops.

#### 3. n8n-style automation editor pattern

Relevant public n8n docs pages:

- Nodes docs describe adding nodes to empty/existing workflows, node operations, node controls, node settings, and connections.
- Sticky Notes docs show n8n treats explanatory canvas annotations as first-class workflow canvas components.
- Executions docs separate manual/partial/production executions and execution/debug views from workflow construction.

What this pattern contributes:

- Strong separation between build mode and execution/debug information: runtime results should overlay or sit in execution panels, not become saved node configuration.
- Canvas annotations/sticky notes can help teams document automation intent without overloading node labels. For this repo, this is only a pattern observation; no current schema found for note nodes.
- Node settings are commonly opened in a focused editing surface because node configuration can be form-heavy.
- Execution state is most useful when it visually marks success/failure/skips at the node and edge level and links to details elsewhere.

What maps well to this repo:

- The node config modal matches form-heavy settings needs (`web/src/main.jsx:2407-2474`, `2575-2677`).
- Existing run overlay badges/classes map run record state into canvas affordances without persistence (`web/src/main.jsx:2746-2961`, `.trellis/spec/frontend/workflow-editor-condition-controls.md:72-73`).
- Condition matched/dimmed branches resemble automation execution traces (`web/src/styles.css:884-905`).

What not to do:

- Do not copy n8n-level feature breadth such as sticky notes, credential panels, partial-execution controls, and node operation catalogs unless the repo has matching backend/runtime contracts.
- Do not make execution/debug history compete with workflow editing controls in the same node card; keep node card content compact and move details to result/detail views or focused surfaces.

#### 4. Node-RED-style automation editor pattern

Relevant Node-RED docs pages:

- Workspace docs: flows are built by dragging nodes from the palette and wiring them together; the workspace includes flow tabs, zoom controls, and a minimap.
- Palette docs: installed nodes are organized into categories, categories can expand/collapse, and a filter input narrows the node list.
- Sidebar docs: editor sidebars provide information/help/debug/config/context panels and can be opened by icons or dropdown selection.

What this pattern contributes:

- Palette/search/category organization is central for automation tools with many installed nodes.
- A persistent sidebar works well for read-only help, debug, and configuration context; focused modals work better for editing dense node settings.
- Workspace navigation affordances (zoom, reset/fit, minimap) are expected for large DAGs.
- Wiring remains direct and visible; node cards stay compact because details live in sidebar/dialog surfaces.

What maps well to this repo:

- Current palette has two tabs plus tool search and tag filters (`web/src/main.jsx:2248-2318`), which fits Node-RED’s categorized palette idea while respecting plugin/control separation.
- Current React Flow view already uses `<MiniMap />`, `<Controls />`, and `<Background />` (`web/src/main.jsx:2334-2336`).
- Current result panel outside the editor can serve the role of debug/output panel; selected node editing uses modal rather than narrow sidebar.

What not to do:

- Do not expose every plugin category and control-node family in one long undifferentiated list; current two-tab separation is important for ops teammates distinguishing plugin tools from built-in workflow primitives.
- Do not make a right sidebar the only configuration path for dense parameter mapping; the spec documents that the modal is the primary node configuration surface (`.trellis/spec/frontend/workflow-editor-condition-controls.md:184-190`).

### What to borrow for this repo

#### Node card information hierarchy

- Keep the always-visible layer minimal: shape/type cue, node name, condition branch rows where routing depends on them, and a small runtime badge only during/after execution.
- Keep secondary metadata progressive: tool IDs, condition input summary, case counts, loop/help text, and runtime detail text should be hover/focus/selection/title/modal content.
- Preserve distinct but simple shape vocabulary: process rounded rectangle for tools, decision/gateway cues for condition/parallel/join, concise marker for loop/control.

#### Side panels vs modals

- Use modal/focused surfaces for node configuration because ops tool params, mapping sources, loop tool selection, and condition cases are form-heavy.
- Use side panels or persistent result areas for non-blocking read-only context: validation summary, run result, logs/debug output, help/info.
- Avoid a permanent inspector that compresses parameter mapping into tight rows unless it is read-only or a quick summary; current spec requires readable vertical rhythm for parameter editors.

#### Edge labels and delete affordances

- Keep condition edge labels visible because they encode routing semantics.
- Keep edge configuration discoverable via edge click; condition edges need a case selector, non-condition edges need clear deletion guidance.
- Consider contextual edge actions as a pattern source from React Flow EdgeLabelRenderer/EdgeToolbar; do not clutter all edges with always-visible controls unless labels/actions remain legible.

#### Run-status overlays

- Continue treating status as transient overlay state derived from run records.
- Show node-level status with compact badges and border/background changes.
- Show path-level state on edges: matched/succeeded emphasized, inactive branches dimmed/dashed, failures red.
- Keep loop iteration detail in tooltips/results rather than expanding node cards into log panels.

#### Palette/search

- Keep explicit plugin-tools vs orchestration-nodes tabs.
- Keep search/tag filtering for plugin tools and small shape-preview cards for control nodes.
- Keep the in-canvas node picker for empty canvas and quick insertion without dragging.

#### Auto-layout

- Keep auto-layout as a manual canvas operation with fit-view afterward.
- Preserve current schema boundary: positions and layout artifacts are editor state only.
- DAG-depth layering is sufficient for ops workflows unless graph complexity grows; external dagre/elk-style layout is a known React Flow pattern if current heuristic becomes inadequate, but would still be frontend-only.

### External References

- [React Flow Node Toolbar example](https://reactflow.dev/examples/nodes/node-toolbar) — contextual node actions anchored to selected/hovered nodes; relevant for delete/config/run actions without permanent card clutter.
- [React Flow Edge Label Renderer example](https://reactflow.dev/examples/edges/edge-label-renderer) — richer edge labels/actions; relevant for condition labels and possible edge delete affordances.
- [React Flow Dagre Tree example](https://reactflow.dev/examples/layout/dagre) — external layout pattern for DAG/tree positioning; relevant to auto-layout alternatives while keeping persistence separate.
- [React Flow Drag and Drop example](https://reactflow.dev/examples/interaction/drag-and-drop) — palette-to-canvas insertion pattern; relevant because the repo already implements drag/drop and in-canvas insertion.
- [n8n Nodes docs](https://docs.n8n.io/workflows/components/nodes/) — automation editor pattern for adding nodes, node controls/settings, and connections.
- [n8n Sticky Notes docs](https://docs.n8n.io/workflows/components/sticky-notes/) — canvas annotation pattern; useful as a reference for documenting flows without overloading node card content, but no current repo contract found.
- [n8n Executions docs](https://docs.n8n.io/workflows/executions/) — execution/debug separation pattern; relevant to transient run overlays and result/detail views.
- [Node-RED Workspace docs](https://nodered.org/docs/user-guide/editor/workspace/) — drag nodes from palette, wire them together, and provide zoom/minimap workspace controls.
- [Node-RED Palette docs](https://nodered.org/docs/user-guide/editor/palette/) — categorized palette with filter input; relevant to plugin categories, search, and tag filtering.
- [Node-RED Sidebar docs](https://nodered.org/docs/user-guide/editor/sidebar/) — sidebars for information/help/debug/config/context panels; relevant to separating read-only context from node edit modals.

### Related Specs

- `.trellis/spec/frontend/workflow-editor-condition-controls.md` — primary frontend canvas UX/schema contract for condition/control nodes, palette tabs, simple card hierarchy, modal editing, auto-layout, run overlays, and branch labels.
- `.trellis/spec/backend/workflow-conditional-nodes.md` — backend runtime/schema contract that gives condition branch labels and run overlay states their semantics.

## Caveats / Not Found

- Active Trellis task resolution returned `Current task: (none)`, so the user-provided task path was used for persistence.
- `python ./.trellis/scripts/get_context.py --mode packages` failed with `ImportError: cannot import name 'get_current_task_source' from 'common.paths'`; package context could not be collected.
- `graphify-out/GRAPH_REPORT.md` and `graphify-out/wiki/index.md` were not found in this worktree, so no graphify report was available.
- Primary ProductFlow pages attempted during research returned HTTP 403; ProductFlow-style notes above are based on the accessible general card-canvas/product-flow pattern, not direct ProductFlow documentation.
- n8n docs pages were accessible for Nodes/Sticky Notes/Executions, but the specific `/workflows/components/canvas/` URL returned 404 in this environment.

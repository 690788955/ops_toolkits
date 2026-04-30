# Research: flowchart / BPMN-like node shape conventions

- **Query**: Research standard industry flowchart / BPMN-like node shape conventions for a workflow editor UI. Focus on: process rectangle, decision diamond, parallel gateway diamond/plus, merge/join gateway, terminator, data/document shapes; which shapes are helpful vs too heavy for web workflow editors; how to adapt with CSS/React Flow without changing behavior.
- **Scope**: external
- **Date**: 2026-04-30

## Findings

### Files Found

| File Path | Description |
|---|---|
| N/A | External convention research only; no repository code was inspected for this topic. |

### Code Patterns

No internal code patterns were inspected. UI adaptation notes below are implementation-neutral and focus on CSS/React Flow rendering conventions without workflow behavior changes.

#### Industry / flowchart shape conventions

| Concept | Common shape | Meaning / typical use | Notes for a web workflow editor |
|---|---|---|---|
| Process / task / activity | Rectangle, often rounded rectangle in modern UIs | A step that performs work. In BPMN, activities are rounded rectangles. In classic flowcharts, process is a plain rectangle. | Highly helpful. This should usually be the default node because it is familiar, compact, label-friendly, and easy to scan. |
| Decision / exclusive branch | Diamond | A conditional choice or branch point; one outgoing path is selected according to a condition. BPMN exclusive gateways are also diamond-shaped, usually with an optional X marker. | Highly helpful when the editor has condition controls or branch logic. It visually separates “routing” nodes from executable task nodes. |
| Parallel gateway / fork / join | Diamond with plus marker | BPMN parallel gateway is a diamond with a plus sign. It is used to split into parallel paths, synchronize parallel incoming paths, or both. | Helpful if the workflow supports parallel execution or explicit synchronization. The plus marker is more important than the diamond alone because diamonds are otherwise read as decisions. |
| Merge / join gateway | Diamond, with marker depending on semantics | In BPMN, gateway shape stays diamond; the internal marker communicates semantics. A parallel join uses diamond-plus. An exclusive merge may be an unmarked or X-marked exclusive gateway. | Helpful only if the runtime/editor distinguishes merge semantics. If merge behavior is implicit from graph topology, a separate merge shape may be unnecessary visual weight. |
| Terminator / start / end | Pill / oval / rounded capsule | Start and end points in classic flowcharts use oval/terminator shapes. BPMN uses start/end events as circles, but many lightweight workflow editors use pill-shaped start/end labels. | Helpful for anchors such as Start and End because it improves diagram orientation. Use sparingly; avoid adding event taxonomy unless the product needs BPMN-level expressiveness. |
| Data / input-output | Parallelogram | Classic flowchart input/output shape. | Potentially helpful for data-entry or parameter nodes, but can become heavy if most nodes already carry forms/config. Better as an icon/badge or special node type only when data movement is first-class. |
| Document | Rectangle with wavy bottom | Classic flowchart document/report artifact. Multiple documents may be stacked. | Usually too heavy for a general web workflow editor unless document generation/review is a core domain concept. Prefer an icon or badge on a process node. |
| Database / data store | Cylinder | Stored data or database. | Useful when the workflow editor models persistence, external stores, or data lineage. Otherwise it can distract from control flow. |

#### Helpful vs. too heavy for modern web workflow editors

Helpful baseline set:

1. **Process rectangle / rounded rectangle** for executable work nodes.
2. **Decision diamond** for conditional branching.
3. **Parallel gateway diamond with plus** only where fork/join semantics are supported or displayed.
4. **Terminator pill/oval** for Start and End nodes.

Potentially useful but domain-dependent:

1. **Data/input-output parallelogram** when the editor has explicit data input/output nodes.
2. **Database cylinder** when persistent stores are represented in the graph.
3. **Document shape** when document generation, approval, or file artifacts are first-class workflow steps.

Likely too heavy for a lightweight web workflow editor:

1. Full BPMN event taxonomy (message/timer/error events, boundary events, compensation markers) unless the runtime actually supports BPMN semantics.
2. Many distinct artifact shapes for documents, manual operations, preparation, display, tape, etc.; these increase legend burden and reduce scan speed.
3. Separate merge/join visuals when the only actual behavior is ordinary edge convergence and the runtime does not treat joins differently.

The practical convention is to borrow the high-signal visual grammar from BPMN/flowcharts while avoiding shapes whose semantics the product does not execute or validate.

#### CSS / React Flow adaptation without behavior changes

React Flow supports rendering custom nodes by mapping node `type` values to React components. A custom node receives the node data and renders arbitrary JSX, while React Flow still owns positioning, edges, handles, selection, and interaction. This allows shape-only changes without changing workflow execution behavior.

Implementation-neutral adaptation patterns:

1. Keep existing node IDs, edge IDs, graph data, node execution types, and runtime condition semantics unchanged.
2. Change only the node renderer / CSS class chosen for each displayed node kind.
3. Use CSS shapes for visual semantics:
   - Process: rounded rectangle, e.g. `border-radius: 10px`.
   - Decision/gateway: square container rotated `45deg`, with inner label rotated back `-45deg`; or use `clip-path: polygon(50% 0, 100% 50%, 50% 100%, 0 50%)`.
   - Parallel gateway: same diamond plus a centered `+` marker.
   - Terminator: capsule, e.g. `border-radius: 999px`.
   - Data: parallelogram via `transform: skewX(-12deg)` on container and reverse skew on content, or `clip-path`.
   - Document: rectangle with a lightweight wavy-bottom SVG or pseudo-element; often better as a badge/icon than a full node silhouette.
4. Preserve React Flow handles (`<Handle />`) and their logical positions so edge connectivity and validation remain the same.
5. For diamond nodes, place handles at top/right/bottom/left visual points, or keep existing top/bottom/left/right positions if changing handle geometry would affect user expectations.
6. If labels inside rotated/skewed shapes become hard to read, render an untransformed inner wrapper.
7. Prefer icons/badges inside the existing rectangle for secondary concepts (document, database, manual task) when screen density and readability matter more than strict notation.

### External References

- [Lucidchart: Flowchart Symbols and Meaning](https://www.lucidchart.com/pages/flowchart-symbols-meaning-explained) — Summarizes standard flowchart symbols including process rectangle, decision diamond, terminator, input/output parallelogram, document, and database/data store.
- [Camunda Docs: BPMN gateways](https://docs.camunda.io/docs/components/modeler/bpmn/gateways/) — Documents BPMN gateway conventions; gateways are diamond-shaped and parallel gateways use a plus marker to fork or join parallel flows.
- [React Flow docs: Custom Nodes](https://reactflow.dev/learn/customization/custom-nodes) — Describes custom node rendering through node type mappings, suitable for shape-only UI changes.
- [React Flow docs: Handles](https://reactflow.dev/learn/customization/handles) — Describes source/target handles and handle customization, relevant when changing node silhouettes while preserving graph connections.

### Related Specs

- N/A — No repository specs were inspected for this external research task.

## Caveats / Not Found

- This research intentionally does not inspect or modify repository code.
- BPMN gateway shapes carry semantics. Reusing BPMN visuals without matching runtime semantics can confuse users; the safe lightweight subset is process, decision, optional parallel gateway, and start/end.
- Exact CSS class names and React components should be mapped to the existing codebase by the implementation agent; this document only records shape conventions and behavior-neutral adaptation principles.

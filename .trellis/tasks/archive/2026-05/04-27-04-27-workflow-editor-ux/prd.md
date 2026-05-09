# brainstorm: Workflow Editor UX Improvements

## Goal

Redesign the Web UI workflow editor so operators can build pipelines more reliably: save failures should explain exactly what is missing, tool selection should be easier to browse and drag from the top, and selected tool/node parameters should be edited in the right-side inspector.

## What I already know

* The user found that saving a pipeline can fail without a clear prompt about which required fields are missing.
* The user wants tool parameters moved to the right-side panel for adjustment.
* The user wants tool selection moved to the top and filtered by tags to make drag-and-drop easier.
* The current editor is implemented in `web/src/main.jsx` as `WorkflowEditor` using `@xyflow/react`.
* Current `WorkflowEditor` has a left `editorToolbar`, central canvas, and right `nodeInspector`.
* Current tool palette is inside the toolbar and filters tools only by active category, not by tags/search inside editor.
* Current node parameter editing already exists in the right `nodeInspector`, but workflow metadata and tool selection share the left toolbar, making the layout crowded.
* Current `saveDraft()` posts to `/api/workflows/{id}/save`; if `draft.id` is missing, the URL becomes `/api/workflows//save` or save errors are shown only as raw string/JSON in the result panel.
* Backend workflow validate/save uses `validateWorkflow(reg, wf)` and returns validation results or JSON error responses.

## Assumptions (temporary)

* MVP should stay within the existing React single-file UI structure unless implementation naturally benefits from extracting small components.
* MVP should not add a database or separate workflow designer service.
* Tag filtering should reuse existing catalog `tags` metadata.
* Save validation should happen client-side before POST for obvious missing fields, then backend validation errors should still be shown clearly.

## Open Questions

* None for MVP.

## Requirements (evolving)

* Save should not silently fail when required workflow fields are missing.
* Before save, the editor should validate at least workflow ID, workflow name, node presence, workflow parameter JSON, and required node tool parameters, then show readable missing-field messages.
* Missing required node tool parameters should block save.
* Backend validation/save errors should be displayed as readable messages, not only raw JSON.
* Workflow run output should be displayed in one combined log window instead of listing every node separately.
* Node-by-node workflow logs should not be expanded by default in the result panel because it makes the output visually noisy.
* Move tool selection to a top palette area inside the editor.
* Top palette should support tag filtering and search/filtering for tools.
* Tool cards in the top palette should remain draggable to the canvas and clickable to add.
* Keep node/tool parameter adjustment in the right-side inspector.
* Right-side inspector should clearly show the selected node, its tool, required parameters, defaults/current values, and mapping options.
* The editor should keep existing validate/run/save actions.

## Acceptance Criteria (evolving)

* [ ] Saving without workflow ID shows a clear message such as `请先填写工作流 ID`.
* [ ] Saving without workflow name shows a clear message.
* [ ] Saving without any nodes shows a clear message.
* [ ] Invalid workflow parameter JSON shows a clear message and does not send a save request.
* [ ] Saving with a node missing required tool parameters shows the node and parameter names, and does not send a save request.
* [ ] Backend validation errors are summarized in readable Chinese in the UI.
* [ ] Workflow execution output is shown in one combined log window.
* [ ] Workflow execution result does not list every node as separate visible log sections by default.
* [ ] Tool selection appears above the canvas/editor content, not buried in the left toolbar.
* [ ] Tool selection supports tags and search text.
* [ ] Dragging a top-palette tool to the canvas still creates a node at the drop position.
* [ ] Clicking a top-palette tool still creates a node.
* [ ] Selecting a node shows editable tool parameters in the right-side inspector.
* [ ] Existing checks pass: `GOTOOLCHAIN=local go test ./...`, `npm run build --prefix web`, `GOTOOLCHAIN=local go build -o "bin/opsctl.exe" "./cmd/opsctl"`, `./bin/opsctl.exe validate`.

## Definition of Done (team quality bar)

* Web UI builds successfully.
* Existing workflow editor behavior remains functional: load, new, validate, run, save, drag/drop, connect edges, delete nodes/edges.
* Validation logic is simple and localized; no duplicate backend workflow semantics beyond obvious client-side required-field checks.
* Docs/specs updated if new reusable UX conventions emerge.

## Out of Scope (explicit)

* Multi-user collaborative editing.
* Workflow version history.
* Auto-layout engine.
* Full schema-driven form builder for every parameter type.
* Backend workflow storage redesign.
* Permission/approval workflow for running pipelines.

## Technical Notes

* `WorkflowEditor` currently lives in `web/src/main.jsx` around the editor tab.
* Existing helper functions include `tagsForEntries`, `filterEntries`, `defaultParams`, `buildMappingSources`, `newToolFlowNode`, and `buildWorkflowDraft`.
* Existing backend workflow save path is in `internal/server/server.go` `handleWorkflowSave`, `validateWorkflow`, `saveWorkflow`, and `workflowPath`.
* Relevant frontend spec requires React state for selected category, active tab, selected entry, params, and result output, and UI generated from backend metadata.
* Relevant backend spec requires Web/API execution through `internal/runner`; this task should not add execution paths.

## Research References

* [`research/current-editor-surface.md`](research/current-editor-surface.md) — Current editor has toolbar/palette stacked left of canvas, node inspector also left on desktop, save errors surface in the generic result panel, and editor palette lacks tag/search filtering.
* [`research/editor-ux-patterns.md`](research/editor-ux-patterns.md) — Common DAG builders use a top command/palette zone, central canvas, right properties inspector, concise validation summaries, and progressive disclosure for advanced JSON.

## Research Notes

### Current constraints

* `WorkflowEditor` already supports drag/drop and click-to-add; the main change is moving and filtering the palette.
* `ParamMappingEditor` already generates fields from selected tool parameters; the main change is making the right inspector the primary parameter editing surface.
* Save currently does not guard empty workflow ID before POST, so missing ID can become a bad URL instead of a useful prompt.
* Backend validation returns one error string, not structured field errors, so MVP should combine client-side preflight for obvious missing fields with readable backend error summaries.

### Feasible approaches here

**Approach A: Top palette + right inspector + validation summary** (Recommended)

* How it works: keep a top editor toolbar for workflow actions/metadata and searchable/tag-filtered tool palette; keep canvas in the center; move inspector to the right column; add client-side preflight and a validation summary banner/card.
* Pros: Matches the user's requested layout and can reuse existing helpers/components.
* Cons: Still keeps workflow metadata and palette in one top area, so careful spacing is needed.

**Approach B: Separate top command bar and collapsible tool drawer**

* How it works: command bar stays top, tool palette opens as a collapsible drawer above/over the canvas, inspector stays right.
* Pros: Better for large tool catalogs later.
* Cons: More UI state and interaction complexity for MVP.

**Approach C: Minimal validation-only fix**

* How it works: only add save preflight messages and improve backend error display, no layout change.
* Pros: Lowest risk.
* Cons: Does not satisfy the requested top tool selection/right inspector redesign.

## Expansion Sweep

### Future evolution

* The top palette can later support favorites/recent tools and plugin grouping without changing the canvas/inspector model.
* Validation can later become structured field/node errors if backend starts returning field paths.

### Related scenarios

* Validate, Save, and Run should share the same preflight message style where applicable.
* Workflow result rendering should use the same single-window log style from both normal run panel and editor run flow.
* Drag/drop and click-to-add should both remain supported for desktop and touch/trackpad users.

### Failure and edge cases

* Empty tool palette after category/tag/search filter should show a clear empty state.
* Missing required node params should block save/run in the editor preflight, while backend remains source of truth for graph structure.
* Workflow logs should be combined in display; detailed per-node logs can remain available only as raw/full details if needed later, not as default visible sections.

## Decision (ADR-lite)

**Context**: The editor needs clearer save validation and a cleaner workflow execution result display. The user prefers strict required-parameter validation at save time and does not want workflow logs split by node in the visible result UI.

**Decision**: Use Approach A. The editor will block save when required workflow fields or required node tool parameters are missing. Workflow execution output will render as a single combined log window by default instead of visible per-node log sections.

**Consequences**: Saved workflows are more likely to be runnable because missing node parameters are caught early. The result panel becomes cleaner for operators, but node-level debugging detail is less prominent in the default UI.

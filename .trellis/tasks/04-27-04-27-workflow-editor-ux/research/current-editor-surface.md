# Research: current workflow editor UX and save validation surface

- **Query**: Research the current workflow editor UX and save validation surface in this repo. Context: saving currently fails without clear missing-field hints; tool parameters should be adjusted on the right; tool selection should move to the top and support tag filtering for easier drag-and-drop. Cover current WorkflowEditor layout/state, save/validate/run flow, tool palette/tag support and drag/drop, right-side node parameter editing, backend workflow save/validate behavior and error shape, implementation surfaces and constraints.
- **Scope**: internal
- **Date**: 2026-04-27

## Findings

### Files Found

| File Path | Description |
|---|---|
| `web/src/main.jsx` | Single-file React app containing `WorkflowEditor`, catalog/run panel tag filtering, drag/drop behavior, right-side node inspector, API helpers, and draft serialization helpers. |
| `web/src/styles.css` | CSS for editor grid layout, toolbar, palette, canvas, node inspector, tag chips/list, result panel, and responsive breakpoints. |
| `internal/server/server.go` | HTTP API handlers for workflow get/run/validate/save, response/error shape, catalog metadata including tags/parameters/confirm, and YAML persistence. |
| `internal/registry/validate.go` | Backend workflow validation rules for required workflow/node fields, duplicate nodes, missing tools, bad edges, and cycles. |
| `internal/config/types.go` | Workflow/tool/parameter/tag schema used by API JSON and YAML persistence. |
| `internal/config/load.go` | Workflow normalization and load-time checks; fills `nodes` from `steps`, derives edges from `depends_on`, defaults node `on_failure`. |
| `internal/config/params.go` | Required parameter validation used by run endpoints; errors are single strings such as `缺少必填参数 name`. |
| `internal/server/server_test.go` | Tests covering workflow validate/save APIs and expected status/body behavior. |
| `.trellis/spec/frontend/directory-structure.md` | Frontend spec notes that UI is generated from backend metadata, build output is embedded, and `web/src/main.jsx` is the React app/API integration surface. |
| `.trellis/spec/backend/quality-guidelines.md` | Backend constraints around runner usage, required parameter validation reuse, plugin-first runtime paths, and Web UI build verification. |
| `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md` | Runtime path constraints: avoid legacy root `tools/`, `workflows/`, root `ops.yaml`, and root `opsctl.exe`. |

### Code Patterns

#### 1. Current `WorkflowEditor` layout and state in `web/src/main.jsx`

`WorkflowEditor` is defined at `web/src/main.jsx:354` and receives `catalog`, `activeCategory`, `setResult`, and `refreshCatalog` from `App` (`web/src/main.jsx:156-158`). It is rendered only when the active tab is `editor`; otherwise `RunPanel` is used (`web/src/main.jsx:150-160`).

State and derived values are local to `WorkflowEditor`:

- `toolOptions`: filtered by active category from `catalog.tools` (`web/src/main.jsx:355-357`).
- `workflowOptions`: filtered by active category from `catalog.workflows` (`web/src/main.jsx:358-360`).
- `workflow`: current draft object, initialized from `emptyWorkflow(activeCategory)` (`web/src/main.jsx:361`).
- `selectedWorkflowID`, `selectedNodeID`, `selectedEdgeID`: selection state (`web/src/main.jsx:362-363`, `web/src/main.jsx:375`).
- `workflowParamsText`, `runParamsText`, `nodeParamsText`: JSON text buffers for workflow parameters, run parameters, and selected node params (`web/src/main.jsx:364-366`).
- `flowInstance`: React Flow instance used to convert drop coordinates (`web/src/main.jsx:367`, `web/src/main.jsx:618`).
- `nodes`, `edges`: React Flow state from `useNodesState` / `useEdgesState` (`web/src/main.jsx:368-369`).
- `selectedNode`, `selectedEdge`, `selectedTool`, `workflowParameters`, `mappingSources`: memoized derived state (`web/src/main.jsx:376-380`).

The JSX layout has three editor sections inside `.editorLayout` (`web/src/main.jsx:548-657`):

1. Left/top `section.card.editorToolbar` with load selector, new/validate/run/save/delete/clear controls, workflow ID/name/description inputs, workflow/run JSON textareas, and the tool palette (`web/src/main.jsx:550-605`).
2. Main `section.card.canvasCard` containing `<ReactFlow>` with minimap/controls/background and node/edge/pane handlers (`web/src/main.jsx:607-625`).
3. `section.card.nodeInspector` for selected edge info or selected node parameter editing (`web/src/main.jsx:627-656`).

CSS defines the desktop grid as a two-column layout with toolbar and inspector stacked on the left and canvas on the right:

```css
.editorLayout {
  display: grid;
  grid-template-columns: minmax(320px, 380px) minmax(560px, 1fr);
  grid-template-areas:
    "toolbar canvas"
    "inspector canvas";
}
```

This appears at `web/src/styles.css:352-360`. The areas are assigned at `web/src/styles.css:362-363`; a responsive breakpoint at `web/src/styles.css:444-452` changes to a single-column order: toolbar, canvas, inspector.

#### 2. Current save/validate/run flow and where errors surface

The editor serializes the current draft through `currentDraft()` (`web/src/main.jsx:499-505`). It parses `workflowParamsText` with `JSON.parse`; if the parsed value is not an array, it throws `工作流参数必须是 JSON 数组` (`web/src/main.jsx:499-503`). It then calls `buildWorkflowDraft(...)`.

`buildWorkflowDraft` preserves existing workflow fields and sets:

- `category`: existing workflow category or active category (`web/src/main.jsx:730-733`).
- `parameters`: parsed workflow parameters (`web/src/main.jsx:733`).
- `nodes`: each React Flow node becomes `{id, name, tool, params, on_failure}` (`web/src/main.jsx:734-740`).
- `edges`: each React Flow edge becomes `{from, to}` (`web/src/main.jsx:741`).

Validate flow:

- `validateDraft()` builds the draft, stores it with `setWorkflow(draft)`, posts to `/api/workflows/${draft.id || 'draft'}/validate`, then writes the full JSON response to the shared result panel with `setResult({message: JSON.stringify(body, null, 2)})` (`web/src/main.jsx:507-515`).
- Client-side thrown errors are caught and surfaced through `setResult({message: String(err)})` (`web/src/main.jsx:513-515`).

Save flow:

- `saveDraft()` builds the draft, posts to `/api/workflows/${draft.id}/save`, sets `selectedWorkflowID`, refreshes catalog, then writes the full JSON response to the shared result panel (`web/src/main.jsx:518-528`).
- If `draft.id` is empty, the path becomes `/api/workflows//save` from the template literal; there is no explicit frontend `if (!draft.id)` guard in `saveDraft()` (`web/src/main.jsx:518-523`).
- Caught errors also surface only through the shared result panel (`web/src/main.jsx:526-528`).

Run flow from the editor:

- `runDraft()` builds the draft and parses `runParamsText` with `JSON.parse` (`web/src/main.jsx:531-534`).
- It explicitly checks `if (!draft.id) throw new Error('请先填写工作流 ID')` (`web/src/main.jsx:535`).
- It posts only to `/api/workflows/${draft.id}/run` with `{params: runParams}`; it does not send the unsaved draft body (`web/src/main.jsx:536-537`).
- If response has `body.id`, it loads run details via `fetchRunDetail(body.id)` and sets `result` to `{run, detail}` (`web/src/main.jsx:538-540`); otherwise it prints the JSON response in the result panel (`web/src/main.jsx:542`).

Result/error surface:

- `WorkflowEditor` does not render inline validation errors near fields; it uses the shared `setResult` prop.
- `ResultView` prints `result.message` in a `<pre>` unless run details are present (`web/src/main.jsx:238-245`).
- The result card is rendered below the active tab panel at `web/src/main.jsx:162-167`, outside `WorkflowEditor`.
- API helper behavior is visible through consumers: caught errors are stringified with `String(err)` in `runSelected`, `loadWorkflow`, `validateDraft`, `saveDraft`, and `runDraft` (`web/src/main.jsx:106-108`, `web/src/main.jsx:410-412`, `web/src/main.jsx:513-515`, `web/src/main.jsx:526-528`, `web/src/main.jsx:543-545`).

#### 3. Current tool palette/tag support and drag/drop behavior

There is tag filtering in the non-editor `RunPanel`, not in `WorkflowEditor`'s tool palette.

RunPanel tag support:

- Global app state contains `searchText` and `activeTag` (`web/src/main.jsx:37-38`).
- `sourceEntries` are active-tab entries filtered by category (`web/src/main.jsx:65-69`).
- `availableTags` uses `tagsForEntries(sourceEntries)` (`web/src/main.jsx:71`, helper at `web/src/main.jsx:785-789`).
- `entries` uses `filterEntries(sourceEntries, searchText, activeTag)` (`web/src/main.jsx:73-75`, helper at `web/src/main.jsx:791-800`).
- `RunPanel` renders a search box and tag chips (`web/src/main.jsx:301-308`) and displays tags on entry cards (`web/src/main.jsx:310-317`).
- `filterEntries` matches `id`, `name`, `description`, `category`, and tags; if `activeTag` is set, entries without that tag are excluded (`web/src/main.jsx:791-800`).

WorkflowEditor palette behavior:

- `toolOptions` are filtered only by category (`web/src/main.jsx:355-357`).
- The palette is rendered inside the toolbar after workflow/run parameter JSON textareas (`web/src/main.jsx:595-604`).
- Each tool button is `draggable`, starts drag via `handleToolDragStart`, and also supports click-to-add via `onClick={() => addToolNode(tool)}` (`web/src/main.jsx:597-602`).
- Palette items currently show name/ID and `拖到画布，或点击添加`; they do not render `TagList` or tag chips in the editor palette (`web/src/main.jsx:598-602`).

Drag/drop behavior:

- `handleToolDragStart` stores the tool id as `application/ops-tool` and sets `effectAllowed = 'move'` (`web/src/main.jsx:444-447`).
- `canvasCard` handles drag-over and drop at the section level (`web/src/main.jsx:607`).
- `handleCanvasDragOver` calls `preventDefault()` and sets `dropEffect = 'move'` (`web/src/main.jsx:449-452`).
- `handleCanvasDrop` reads `application/ops-tool`, finds it in `toolOptions`, requires a `flowInstance`, and calls `addToolNode(tool, flowInstance.screenToFlowPosition({x: event.clientX, y: event.clientY}))` (`web/src/main.jsx:454-460`).
- `addToolNode` creates a unique node ID and adds a React Flow node; if no drop position is given, it places nodes at a computed offset (`web/src/main.jsx:436-442`).
- The node ID generation replaces `.` and `-` with `_`, starts at `nodes.length + 1`, and increments until unique (`web/src/main.jsx:803-808`).

CSS surfaces:

- `.toolPalette` is currently a scrollable grid with max height 280px (`web/src/styles.css:368-375`).
- `.paletteItem` styles the draggable/clickable tool buttons (`web/src/styles.css:377-390`).
- `.tagFilters`, `.tagChip`, and `.tagList` styles already exist for run panel tags (`web/src/styles.css:183-200`).

#### 4. Current right-side node parameter editing behavior

The node inspector is visually named “节点参数” and is the lower-left area on wide screens due to CSS grid areas (`web/src/main.jsx:627-656`, `web/src/styles.css:352-363`). It moves below the canvas at `max-width: 1400px` (`web/src/styles.css:444-452`).

Selection behavior:

- React Flow node click sets `selectedNodeID` and clears edge selection (`web/src/main.jsx:615`).
- Edge click sets `selectedEdgeID` and clears node selection (`web/src/main.jsx:616`).
- Pane click clears both selections (`web/src/main.jsx:617`).
- The delete button inside `ToolNode` calls `data.onRemove(id)` and stops propagation (`web/src/main.jsx:19-27`).

When no node/edge is selected, the inspector shows: `点击画布节点后编辑参数，点击连线后可删除依赖。` (`web/src/main.jsx:633-634`). When an edge is selected, it shows that the selected edge can be deleted via the left controls (`web/src/main.jsx:633`).

For a selected node, the inspector shows disabled node ID and tool ID inputs (`web/src/main.jsx:636-643`) and `ParamMappingEditor` (`web/src/main.jsx:644`). It also includes an advanced JSON editor in `<details>` (`web/src/main.jsx:645-653`).

`ParamMappingEditor` behavior:

- It receives the selected tool definition, current node params, mapping sources, and an `onChange` callback (`web/src/main.jsx:661`).
- If no tool definition is found, it shows `未找到当前节点工具定义。` (`web/src/main.jsx:662-664`).
- If the tool declares no parameters, it shows `当前工具没有声明输入参数。` (`web/src/main.jsx:664`).
- For each declared parameter, it renders parameter display text, a select box for mapping source, and a freeform input bound to the same `params[param.name]` value (`web/src/main.jsx:668-679`).
- Both the select and input call `onChange(param.name, value)` (`web/src/main.jsx:674-678`).
- The select options include `手动输入 / 不设置` and available mapping sources (`web/src/main.jsx:674-677`).

Parameter source construction:

- `workflowParameters` is parsed from `workflowParamsText` through `parseJSONList`, which returns `[]` on parse failure and does not surface the parse error itself (`web/src/main.jsx:379`, `web/src/main.jsx:753-760`).
- `buildMappingSources` adds workflow parameter expressions like `{{ .paramName }}` (`web/src/main.jsx:762-766`).
- It then adds direct upstream node stdout/stderr and existing upstream node params as expressions (`web/src/main.jsx:767-775`).
- `upstreamNodeIDs` only returns direct incoming edge sources for the selected node (`web/src/main.jsx:779-782`).

Advanced JSON editor behavior:

- `nodeParamsText` is synchronized from the selected node's `data.params` with a `useEffect`; if no selected node, it becomes `{}` (`web/src/main.jsx:382-388`).
- `applyNodeParams` parses `nodeParamsText` as JSON and updates selected node params; parse errors surface in the shared result panel as `参数（JSON） 无效: ...` (`web/src/main.jsx:479-487`).
- `updateSelectedNodeParams` writes the new params into the selected node's `data.params` and also formats `nodeParamsText` (`web/src/main.jsx:489-492`).
- `updateMappedParam` merges a single key into selected node params (`web/src/main.jsx:494-497`).

#### 5. Backend workflow save/validate behavior and error shape

API routes:

- `workflowsHandler` routes `POST /api/workflows/{id}/run`, `POST /api/workflows/{id}/validate`, and `POST /api/workflows/{id}/save` based on suffixes (`internal/server/server.go:360-385`).
- `handleWorkflowValidate` decodes `{workflow: ...}` and always returns HTTP 200 with `response{Data: validateWorkflow(...)}` when JSON decoding succeeds (`internal/server/server.go:448-455`).
- `handleWorkflowSave` decodes `{workflow: ...}`, checks path/body ID mismatch, validates, writes YAML, updates `reg.Workflows`, and returns `status: "saved"` on success (`internal/server/server.go:457-478`).

Response shape:

- Generic API response is `{id?, status?, data?, error?}` (`internal/server/server.go:253-258`).
- Workflow validation shape is `{valid: boolean, error?: string}` (`internal/server/server.go:237-240`).
- Validate success response example shape: `{"data":{"valid":true}}` from `handleWorkflowValidate` (`internal/server/server.go:448-455`).
- Validate failure response shape still uses HTTP 200: `{"data":{"valid":false,"error":"..."}}` (`internal/server/server.go:448-455`, tests at `internal/server/server_test.go:81-95`).
- Save validation failure returns HTTP 400 with both `data` and top-level `error`: `response{Data: result, Error: result.Error}` (`internal/server/server.go:467-470`).
- Save path/body ID mismatch returns HTTP 400 with top-level `error: "workflow id does not match path"` (`internal/server/server.go:463-465`).
- JSON decode errors return HTTP 400 with top-level `error` (`internal/server/server.go:448-452`, `internal/server/server.go:457-461`).

Backend validation rules:

- `validateWorkflow` wraps `reg.ValidateWorkflow` into `workflowValidation` (`internal/server/server.go:649-654`). It contains no list of fields or structured field errors.
- `Registry.ValidateWorkflow` checks:
  - workflow ID required: `工作流 ID 必填` (`internal/registry/validate.go:19-22`),
  - at least one node required: `节点必填` (`internal/registry/validate.go:23-25`),
  - node ID required: `节点 ID 必填` (`internal/registry/validate.go:27-30`),
  - duplicate node ID: `节点 ID 重复: <id>` (`internal/registry/validate.go:31-33`),
  - node tool required: `节点 <id> 的工具必填` (`internal/registry/validate.go:35-37`),
  - referenced tool exists: `节点 <id> 引用了不存在的工具 <tool>` (`internal/registry/validate.go:38-40`),
  - then calls `OrderWorkflow` for edge/cycle validation (`internal/registry/validate.go:42-43`).
- `OrderWorkflow` checks node duplicates again and edge shape/existence:
  - `工作流依赖的 from/to 必填` (`internal/registry/validate.go:60-63`),
  - `工作流依赖引用了不存在的节点 <id>` (`internal/registry/validate.go:64-69`),
  - `工作流存在环形依赖` (`internal/registry/validate.go:92-94`).

Save persistence:

- `decodeWorkflow` JSON-decodes into `workflowSaveRequest`, normalizes the workflow, and returns it (`internal/server/server.go:639-647`).
- `saveWorkflow` creates parent directories, YAML-encodes the workflow with indent 2, and writes it to disk with mode `0644` (`internal/server/server.go:656-670`).
- `workflowPath` reuses an existing registered workflow path when present; otherwise it defaults to `workflows` unless `reg.Root.Paths.Workflows[0]` is configured, then writes `<id>.yaml` there (`internal/server/server.go:673-679`).
- The project-level runtime guidance says the latest structure is plugin-first and legacy root `workflows/` is intentionally not part of latest runtime layout; specs also warn not to add workflows under root `workflows/` (`.trellis/spec/backend/directory-structure.md:47`, `.trellis/spec/backend/quality-guidelines.md:37-40`). The current `workflowPath` fallback still names `workflows` if no configured workflow path exists (`internal/server/server.go:673-679`).

Run endpoint validation:

- `handleWorkflowRun` loads an existing registered workflow by URL ID; it does not use the editor draft body (`internal/server/server.go:403-417`).
- It merges workflow parameter defaults and request params, then calls `config.ValidateRequired`; missing required params return HTTP 400 top-level error strings (`internal/server/server.go:418-421`, `internal/config/params.go:54-60`).
- It checks workflow/tool confirmation requirements before calling runner (`internal/server/server.go:423-431`).

Catalog metadata used by frontend:

- `buildCatalog` includes `Tags`, `Parameters`, `Confirm`, and `Source` for tools and workflows (`internal/server/server.go:610-618`).
- Tool/workflow catalog entry structs expose tags and parameters for frontend filtering/forms (`internal/server/server.go:212-226`).
- `config.ToolConfig` and `config.WorkflowConfig` both include `Tags []string` and `Parameters []Parameter` (`internal/config/types.go:90-100`, `internal/config/types.go:141-149`).

#### 6. Specific implementation surfaces and constraints

Implementation surfaces identified by inspection:

| Surface | Current role |
|---|---|
| `web/src/main.jsx:354-659` | Main `WorkflowEditor` component: editor state, layout, validation/save/run handlers, drag/drop, node selection, node inspector. |
| `web/src/main.jsx:661-683` | `ParamMappingEditor`: declared tool parameter UI for selected node. |
| `web/src/main.jsx:729-742` | `buildWorkflowDraft`: serialization boundary from React Flow state to backend workflow schema. |
| `web/src/main.jsx:745-800` | Helpers for defaults, parameter parsing, mapping sources, tag extraction/filtering. |
| `web/src/main.jsx:293-351` | Existing searchable/tag-filtered list pattern in `RunPanel`. |
| `web/src/styles.css:352-452` | Workflow editor layout, palette, canvas, node, and responsive layout CSS. |
| `web/src/styles.css:183-200` | Existing tag filter/chip/list CSS used outside editor. |
| `internal/server/server.go:448-478` | Validate/save API behavior and save error shape. |
| `internal/server/server.go:237-240`, `253-258` | Validation and generic response JSON shapes. |
| `internal/registry/validate.go:19-96` | Backend workflow validation rules and single-string error messages. |
| `internal/server/server_test.go:65-128` | API regression tests for validate/save status and response behavior. |

Constraints from specs and project instructions:

- UI is generated from backend YAML/plugin metadata; React should not hard-code tools/workflows (`.trellis/spec/frontend/directory-structure.md:82-87`).
- Web UI source is `web/src/main.jsx` and `web/src/styles.css`; Vite build output is embedded into `internal/server/web/` (`.trellis/spec/frontend/directory-structure.md:17-27`, `.trellis/spec/frontend/directory-structure.md:32-44`).
- After UI changes, `npm run build --prefix web` is the build contract and embedded server assets need rebuilding (`.trellis/spec/frontend/directory-structure.md:32-44`).
- API routes currently documented for UI are catalog/run/run-detail; workflow editor validate/save endpoints exist in code but are not listed in the frontend API contract section (`.trellis/spec/frontend/directory-structure.md:60-67`, `internal/server/server.go:360-385`).
- Backend required parameter validation belongs in `internal/config` and should be reused across entrypoints (`.trellis/spec/backend/quality-guidelines.md:24-30`).
- Runtime tools/workflows are plugin-first; specs caution not to add new runtime tools under root `tools/` or workflows under root `workflows/` (`.trellis/spec/backend/quality-guidelines.md:37-40`, `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md:119-126`).
- User-facing text in this repository is Simplified Chinese per project instructions.
- Graphify project instructions apply to code/architecture questions; `graphify-out/GRAPH_REPORT.md` was read before this research. It identifies `WorkflowEditor()` and related helpers (`buildMappingSources`, `defaultParams`, `emptyWorkflow`, `fetchJSON`, `newToolFlowNode`, `upstreamNodeIDs`) as a frontend community (`graphify-out/GRAPH_REPORT.md:103-106`). No `graphify-out/wiki/index.md` file exists.

### External References

None. This was an internal repository inspection task.

### Related Specs

- `.trellis/spec/frontend/directory-structure.md` — frontend source/build organization, metadata-driven UI, embedded build contract, documented UI API routes.
- `.trellis/spec/backend/quality-guidelines.md` — backend validation/runner constraints, plugin-first runtime paths, testing/build verification notes.
- `.trellis/spec/backend/directory-structure.md` — current runtime layout and legacy root path exclusions.
- `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md` — cautions against legacy root runtime path assumptions and path-related verification checklist.

## Caveats / Not Found

- No inline workflow-editor field error state was found; current validation/save/run errors route through the shared `result` panel.
- No editor-palette search or tag filter was found; tag filtering exists in `RunPanel` for tools/workflows but not in `WorkflowEditor`'s tool palette.
- Backend validation returns one error string, not a structured list of missing fields or field paths.
- `saveDraft()` does not contain the same explicit empty-workflow-ID guard as `runDraft()`.
- `runDraft()` runs the registered workflow by ID; it does not submit or execute the unsaved draft body.
- No external docs were needed for this repository-surface research.

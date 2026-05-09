# Research: Workflow/DAG editor UX patterns

- **Query**: Research common workflow/DAG editor UX patterns for validation feedback, top tool palettes with tag filters, and right-side node inspectors. Context: React + @xyflow/react Web UI. User wants: save workflow should clearly say what fields are missing; tool parameters adjusted on right; tool selection moved to top with tag selection for drag/drop.
- **Scope**: mixed
- **Date**: 2026-04-27

## Findings

### Files Found

| File Path | Description |
|---|---|
| `graphify-out/GRAPH_REPORT.md` | Project graph overview; Community 8 identifies `WorkflowEditor()`, `newToolFlowNode()`, `buildMappingSources()`, and related editor helpers as the relevant frontend cluster. |
| `web/src/main.jsx` | Current React + `@xyflow/react` single-file Web UI, including workflow editor, palette, canvas, right-side/current inspector, filtering helpers, and API calls. |
| `web/src/styles.css` | Current layout and visual styles for tabs, tag chips, palette, editor grid, canvas, React Flow nodes/edges, and responsive breakpoints. |
| `internal/registry/validate.go` | Backend workflow validation rules and Chinese error strings returned through validate/save endpoints. |
| `internal/server/server.go` | Workflow validate/save API handlers and catalog payload assembly for tags/parameters/confirm/source. |
| `internal/config/types.go` | Shared tool/workflow JSON shape; tools and workflows expose `tags`, `parameters`, `nodes`, `edges`, and `confirm`. |
| `internal/config/params.go` | Required-parameter validation behavior and existing `缺少必填参数 <name>` error style. |
| `plugins/plugin.demo/plugin.yaml` | Demo plugin tools include `tags` and declared `parameters`; useful realistic data for palette tag filters and node inspector fields. |
| `configs/ops.yaml` | Runtime config currently uses plugin-first structure and empty legacy root tool/workflow paths. |
| `.trellis/spec/frontend/component-guidelines.md` | Frontend component spec placeholder; no concrete constraints found. |
| `.trellis/spec/frontend/state-management.md` | Frontend state-management spec placeholder; no concrete constraints found. |
| `.trellis/spec/frontend/quality-guidelines.md` | Frontend quality spec placeholder; no concrete constraints found. |

### Code Patterns

#### Current editor layout and interaction model

- `web/src/main.jsx:354` defines `WorkflowEditor({catalog, activeCategory, setResult, refreshCatalog})` with local React state for workflow metadata, selected node/edge, workflow params JSON, run params JSON, node params JSON, React Flow nodes/edges, and `flowInstance`.
- `web/src/styles.css:352` currently lays out the editor as a two-column grid with toolbar and inspector on the left and canvas on the right:

```css
.editorLayout {
  display: grid;
  grid-template-columns: minmax(320px, 380px) minmax(560px, 1fr);
  grid-template-areas:
    "toolbar canvas"
    "inspector canvas";
}
```

- `web/src/main.jsx:550` renders the current `editorToolbar` card with workflow load/create/validate/run/save actions and workflow metadata fields.
- `web/src/main.jsx:595` renders the current tool palette inside the toolbar card, after workflow metadata fields. Palette items support both drag-to-canvas and click-to-add:

```jsx
<button key={tool.id} className="paletteItem" draggable onDragStart={event => handleToolDragStart(event, tool)} onClick={() => addToolNode(tool)}>
```

- `web/src/main.jsx:607` renders the `ReactFlow` canvas with `MiniMap`, `Controls`, `Background`, node/edge selection, pane-click clear selection, and drop handling.
- `web/src/main.jsx:627` renders `nodeInspector`; despite the requested “right-side node inspector”, the current CSS places it in the left column under toolbar. It already edits selected node params via `ParamMappingEditor` and has an advanced JSON editor.

#### Existing tag filtering patterns

- Catalog entries already include tags. `internal/server/server.go:610-617` adds `Tags` and `Parameters` to tool/workflow catalog entries.
- Tool configs and workflow configs expose tags in shared types: `internal/config/types.go:90-100` and `internal/config/types.go:141-152`.
- The current run panel has reusable tag filter logic:
  - `web/src/main.jsx:785-789` builds available tags from entries.
  - `web/src/main.jsx:791-800` filters entries by active tag and search keyword.
  - `web/src/styles.css:183-197` styles `.tagFilters`, `.tagChip`, and active chip state.
- Demo tools already have tags: `plugins/plugin.demo/plugin.yaml:18` uses `[plugin, demo, shell]`; `plugins/plugin.demo/plugin.yaml:45` uses `[plugin, demo, confirm]`.

#### Existing node inspector and parameter mapping patterns

- `web/src/main.jsx:378` derives selected tool metadata from the selected node’s `data.tool`, enabling inspector fields to be generated from the original tool parameter schema.
- `web/src/main.jsx:644` renders `ParamMappingEditor` in the inspector using tool parameters, current params, and mapping sources.
- `web/src/main.jsx:661-683` implements parameter rows with:
  - parameter display name / description;
  - required marker `*`;
  - a mapping dropdown for workflow/upstream sources;
  - an input for manual values.
- `web/src/main.jsx:762-777` builds mapping source options from workflow parameters and direct upstream nodes, including stdout/stderr and upstream node params.

#### Existing validation patterns and API behavior

- `web/src/main.jsx:507-516` validates drafts by POSTing to `/api/workflows/{id || 'draft'}/validate`; the UI currently displays the whole JSON response via `setResult({message: JSON.stringify(body, null, 2)})`.
- `web/src/main.jsx:518-528` saves drafts by POSTing to `/api/workflows/{draft.id}/save`; save errors are currently surfaced as `String(err)` in the generic result card.
- `internal/server/server.go:448-454` returns validate results as HTTP 200 with `{data: {valid, error?}}`.
- `internal/server/server.go:457-470` returns save validation failures as HTTP 400 with both `data: result` and `error: result.Error`.
- `internal/registry/validate.go:19-43` validates only structural workflow issues:
  - missing workflow ID: `工作流 ID 必填`;
  - no nodes: `节点必填`;
  - missing node ID: `节点 ID 必填`;
  - duplicate node ID: `节点 ID 重复: <id>`;
  - missing node tool: `节点 <id> 的工具必填`;
  - unknown tool: `节点 <id> 引用了不存在的工具 <tool>`;
  - invalid dependency and cycles via `OrderWorkflow`.
- `internal/registry/validate.go:60-93` validates edge `from/to`, missing edge endpoint nodes, and cycles.
- Required tool/workflow runtime params are validated elsewhere as a single first error: `internal/config/params.go:54-60` returns `缺少必填参数 <name>`.

### Comparable UX Patterns

#### Workflow/DAG builders

Common workflow/DAG editors such as Node-RED, n8n, Zapier Canvas, Make/Integromat, Airflow-style DAG viewers, GitHub Actions visualizations, and low-code builders generally converge on the same editor anatomy:

1. **Tool/source palette**: a searchable and categorized list of actions/nodes. When the catalog is large, filters are usually chips/tabs/tags near the palette search, not buried in individual cards.
2. **Central canvas**: nodes and edges are the primary working area; selection state drives inspector contents.
3. **Inspector/properties panel**: selected node fields and parameters are edited in a side panel, usually right-side because left/top are reserved for navigation and creation.
4. **Top command bar**: global workflow actions such as Save, Validate/Test, Run, Undo/Redo, and status are visually separate from node configuration.
5. **Inline validation**: save/validate failures show a summary and point to the affected field/node; mature editors also decorate invalid nodes on the canvas.
6. **Progressive disclosure**: simple parameter forms are default; raw JSON/expression editing is an advanced fallback.

#### React Flow / @xyflow/react-compatible patterns

The current implementation already uses React Flow patterns that map well to common DAG editors:

- draggable external nodes: `handleToolDragStart`, `handleCanvasDrop`, `screenToFlowPosition` (`web/src/main.jsx:444-459`);
- selected node/edge details outside the canvas (`web/src/main.jsx:615-617`, `web/src/main.jsx:627-656`);
- overview controls via `MiniMap`, `Controls`, and `Background` (`web/src/main.jsx:621-623`).

For this repo, the comparable UX pattern can be implemented using existing local state and CSS primitives without introducing a separate editor framework.

### Recommended MVP Layout for This Repo

The requested MVP is consistent with a three-zone workflow editor:

| Zone | Content | Relevant existing code |
|---|---|---|
| Top command/palette zone | Load workflow, new/validate/run/save, workflow ID/name/description, tool search/tag chips, draggable tool cards. | Current toolbar at `web/src/main.jsx:550-605`; tag helpers at `web/src/main.jsx:785-800`; tag styles at `web/src/styles.css:183-197`. |
| Center canvas | React Flow canvas with nodes/edges, minimap/controls/background, drag-drop target. | `web/src/main.jsx:607-625`; canvas styles at `web/src/styles.css:391-442`. |
| Right inspector | Selected node parameters, mapping dropdowns, manual values, advanced JSON; selected edge actions; empty-state instructions. | Current inspector at `web/src/main.jsx:627-656`; `ParamMappingEditor` at `web/src/main.jsx:661-683`. |

MVP layout notes grounded in current implementation:

- The existing `nodeInspector` already has the right content; the main layout change is positional: move it to a right column instead of the current left-under-toolbar grid area (`web/src/styles.css:352-357`).
- The existing `toolPalette` already supports drag/drop and click-to-add (`web/src/main.jsx:595-603`). The main behavior gap is palette filtering inside the workflow editor. Existing `tagsForEntries` and `filterEntries` functions can describe the required logic because they already filter run-panel entries by tag/search (`web/src/main.jsx:785-800`).
- Tool cards can remain compact. MVP should prioritize showing name, id, tags, and required-param count over adding rich previews.
- Keep raw JSON editing behind `<details className="advancedParams">` as currently implemented (`web/src/main.jsx:645-653`), because it supports power users without making the main form harder to scan.

### Validation Feedback Patterns Useful Without Overbuilding

Useful MVP validation feedback for this repo should map backend errors to clear UI messages and obvious locations without requiring a full schema/error model.

#### 1. Summary banner/result near Save/Validate

Pattern:

- show success as concise Chinese text such as `校验通过，可以保存`;
- show failure as `保存失败：请补全以下内容` or `校验失败：<reason>`;
- list actionable items if they can be inferred from known error strings.

Current gap: validate/save currently dumps raw JSON (`web/src/main.jsx:512`, `web/src/main.jsx:525`) or `String(err)` (`web/src/main.jsx:527`). For the user requirement “save workflow should clearly say what fields are missing”, the research-relevant finding is that backend already returns Chinese missing-field errors, but the frontend does not currently translate them into field-specific UI.

#### 2. Field-level messages for workflow metadata

Known backend structural errors can be shown next to the fields/areas users can fix:

| Backend message source | User-facing target |
|---|---|
| `工作流 ID 必填` (`internal/registry/validate.go:20-22`) | Workflow ID field. |
| `节点必填` (`internal/registry/validate.go:23-25`) | Canvas / empty workflow area. |
| `节点 ID 必填` (`internal/registry/validate.go:28-30`) | Node inspector if a selected node exists; otherwise validation summary. |
| `节点 ID 重复: <id>` (`internal/registry/validate.go:31-33`) | Validation summary and affected node cards if matched by id. |
| `节点 <id> 的工具必填` (`internal/registry/validate.go:35-37`) | Affected node / inspector. |
| `节点 <id> 引用了不存在的工具 <tool>` (`internal/registry/validate.go:38-40`) | Affected node / inspector. |
| `工作流依赖的 from/to 必填` (`internal/registry/validate.go:60-63`) | Edge validation summary. |
| `工作流依赖引用了不存在的节点 <id>` (`internal/registry/validate.go:64-69`) | Edge validation summary. |
| `工作流存在环形依赖` (`internal/registry/validate.go:92-94`) | Canvas-level summary; optionally mark involved edges if cycle detection is later expanded. |

#### 3. Canvas/node badges for structural failures

Without overbuilding, the UI can use a simple validation state keyed by node id and global area:

- global errors: workflow ID missing, no nodes, cycle, malformed workflow params JSON;
- node errors: duplicate id, missing/unknown tool;
- edge errors: missing endpoint, nonexistent node reference.

This only requires matching existing error strings or client-side preflight checks. It does not require changing backend response format.

#### 4. Client-side preflight before save

Some missing-field feedback can be generated before calling save:

- workflow ID empty;
- workflow parameters JSON invalid or not an array (`web/src/main.jsx:499-504` already throws `工作流参数必须是 JSON 数组`);
- no nodes;
- selected node required params empty, using tool schemas available from catalog (`tool.parameters`, `required` in `internal/config/types.go:133-139`).

This is useful because runtime required params currently validate at execution time, while workflow save validation currently checks structural graph validity only.

#### 5. Preserve backend validation as source of truth

Even with client preflight, save should still rely on `/save` behavior from `internal/server/server.go:467-470`, because backend validation catches unknown tools, duplicate IDs, missing edges, and cycles.

### Failure / Edge Cases to Include or Defer

#### Include in MVP

| Case | Why include |
|---|---|
| Empty workflow ID on save | Directly matches user request; backend already has `工作流 ID 必填`. |
| No nodes on save/validate | Backend already has `节点必填`; common blank-canvas failure. |
| Invalid workflow params JSON | Current `currentDraft()` can throw; users need a clear message near workflow params. |
| Missing required node tool parameter values | Tool parameter schemas are in catalog and inspector already renders required markers. Even if backend save does not enforce these, the UX can warn before save/run. |
| Unknown/missing tool after plugin removal or catalog refresh | Backend detects unknown tool; relevant to plugin-first repo. |
| Duplicate node IDs | Backend detects; unique ID helper exists, but loaded/edited workflows could still contain duplicates. |
| Invalid edges and cycles | Backend detects; crucial for DAG editors. |
| Palette empty after category/tag/search filter | Current run panel has empty-state precedent; top palette should say no matching tools. |
| Drag/drop not available on touch/mobile | Click-to-add already exists and should remain as fallback. |

#### Defer beyond MVP

| Case | Reason to defer |
|---|---|
| Full schema-driven form renderer for all parameter types | Current `Parameter` type only exposes name/type/description/required/default; no enum/min/max/secret/file metadata found. |
| Multi-select bulk edit and keyboard shortcuts | Not required for requested UX; adds interaction complexity. |
| Full cycle visualization with highlighted cycle path | Backend returns only a generic cycle error, not cycle members. |
| Live validation on every drag/key stroke | Could create noisy feedback; validate on save/validate and optionally on blur is enough. |
| Auto-layout and large-DAG navigation beyond MiniMap/Controls | React Flow controls already exist; large graph tooling can wait. |
| Undo/redo history | Useful in mature DAG builders, but not needed for clear save feedback/top palette/right inspector MVP. |
| Per-parameter type widgets beyond text/select mapping | Current tool parameter schema is minimal, so richer widgets would need schema changes. |
| Collaborative editing / locking | Out of scope for local ops console. |

### External References

No live external search tool was available in this environment. The comparable-product notes above are based on general product knowledge of workflow/DAG builders and on the repo’s current React Flow implementation.

Relevant library/API already present in code:

- `@xyflow/react` is imported in `web/src/main.jsx:10-14` and used through `ReactFlow`, `MiniMap`, `Controls`, `Background`, `addEdge`, `useEdgesState`, and `useNodesState`.

### Related Specs

- `.trellis/spec/frontend/component-guidelines.md` — placeholder only; no concrete component conventions found.
- `.trellis/spec/frontend/state-management.md` — placeholder only; no concrete state-management constraints found.
- `.trellis/spec/frontend/quality-guidelines.md` — placeholder only; no concrete validation/testing constraints found.

## Caveats / Not Found

- `.trellis/.current-task` currently points to `.trellis/tasks/00-join-cjg`, but the user explicitly requested persistence at `.trellis/tasks/04-27-04-27-workflow-editor-ux/research/editor-ux-patterns.md`; this file was written to the requested task directory.
- `graphify-out/wiki/index.md` was not found, so raw graph report and source files were used.
- No concrete frontend specs beyond placeholders were found in `.trellis/spec/frontend/`.
- Current backend workflow save validation does not appear to validate missing required tool node parameter values; required runtime parameters are validated in config/runner paths. If the UX must block save for missing node params, that behavior is client-side unless backend validation is expanded.
- External web search tools named in the task were not available in this tool environment, so no URLs are cited.

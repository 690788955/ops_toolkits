# Research: current devkit/template/download surface

- **Query**: Research the current devkit/template/download surface in this repo for updating Web development package download from old `tools/` layout to plugin-first, and for download plugin template + upload plugin.
- **Scope**: internal
- **Date**: 2026-04-27

## Findings

### Files Found

| File Path | Description |
|---|---|
| `internal/server/server.go` | HTTP API registration, current devkit zip constants, zip builder, workflow save surface. |
| `internal/server/server_test.go` | Existing test for `/api/dev/toolkit.zip`, currently asserting legacy `tools/demo/sample-tool` zip entries. |
| `web/src/main.jsx` | React Web UI entrypoint; contains floating download link and workflow editor save/validate/run actions. No upload control found. |
| `web/src/styles.css` | Styling for `.downloadKit` and `.floatingDownload`. |
| `internal/scaffold/scaffold.go` | Existing scaffold helper still creates legacy root `tools/<category>/<tool>` and `workflows/` examples. |
| `internal/plugin/types.go` | Current plugin manifest schema (`Manifest`, `Contributes`, plugin `Tool`, plugin `Workflow`). |
| `plugins/plugin.demo/plugin.yaml` | Concrete plugin-first sample manifest with `plugin.demo.*` tool IDs and `scripts/*.sh` commands. |
| `CLAUDE.md` | Project runtime instructions: plugin-first, no new runtime tools under root `tools/` or workflows under root `workflows/`. |
| `.trellis/spec/backend/directory-structure.md` | Backend spec for plugin-first runtime layout and plugin contracts. |
| `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md` | Guide explicitly calling out stale dev-kit/scaffold/docs references to legacy paths. |
| `.trellis/spec/guides/index.md` | Checklist entry says updating plugin/runtime docs, dev-kit examples, scaffolds, or shell execution behavior should use cross-platform runtime guide. |
| `README.md` | User-facing plugin-first documentation and plugin manifest example. |

### Code Patterns

#### Backend devkit download endpoint and zip builder

- `internal/server/server.go:232-240` creates the server mux and registers the devkit endpoint:
  - `/api/catalog`
  - `/api/dev/toolkit.zip`
  - `/api/tools/`
  - `/api/workflows/`
  - `/api/runs/`
- `internal/server/server.go:439-455` implements `toolDevKitHandler()`:
  - Allows only `GET`; non-GET returns `methodNotAllowed`.
  - Calls `buildToolDevKitZip()`.
  - Responds with `Content-Type: application/zip`.
  - Responds with `Content-Disposition: attachment; filename="ops-tool-devkit.zip"`.
- `internal/server/server.go:500-526` implements `buildToolDevKitZip()` as an in-memory `map[string]string` of zip entry path to content, then writes each entry to an archive.
- Current zip entries in `buildToolDevKitZip()` are legacy root-tool layout:
  - `README.md`
  - `SPEC.md`
  - `tools/demo/sample-tool/tool.yaml`
  - `tools/demo/sample-tool/bin/run.sh`
  - `tools/demo/sample-tool/README.md`
  - `tools/demo/sample-tool/examples/in.yaml`
- Current devkit constants are embedded in `internal/server/server.go:27-170`:
  - `toolDevKitReadme` instructs maintainers to copy `tools/demo/sample-tool` into `tools/<分类>/<工具>/` (`internal/server/server.go:27-39`).
  - `toolDevKitSpec` says tools must live under `tools/<分类>/<工具>/` and uses `tool.yaml`, `bin/run.sh`, `execution.entry` (`internal/server/server.go:41-84`).
  - `sampleToolYAML` uses legacy tool schema with `id: demo.sample-tool`, `execution.entry: bin/run.sh`, and `pass_mode` (`internal/server/server.go:86-124`).
  - `sampleRunScript`, `sampleToolReadme`, and `sampleParamsYAML` provide sample shell/readme/params content (`internal/server/server.go:126-170`).

#### Existing backend tests for devkit download

- `internal/server/server_test.go:129-158` defines `TestToolDevKitDownloadAPI`.
- Test behavior:
  - Sends `GET /api/dev/toolkit.zip` through `NewHandler(reg)`.
  - Expects HTTP 200.
  - Expects `Content-Type` exactly `application/zip`.
  - Expects `Content-Disposition` containing `ops-tool-devkit.zip`.
  - Opens the response body as zip and asserts these legacy entries exist:
    - `README.md`
    - `SPEC.md`
    - `tools/demo/sample-tool/tool.yaml`
    - `tools/demo/sample-tool/bin/run.sh`
    - `tools/demo/sample-tool/README.md`

#### Frontend download controls

- `web/src/main.jsx:138-146` renders a floating download link in the main content:
  - `<a className="downloadKit floatingDownload" href="/api/dev/toolkit.zip">下载工具开发包</a>`
- `web/src/styles.css:114-127` styles `.hint, .downloadKit` and `.downloadKit:hover`.
- `web/src/styles.css:128-134` positions `.floatingDownload` fixed at bottom-right.

#### Frontend upload-like controls/actions

- Search in `web/src` found no upload-specific UI/API usage:
  - No `upload` / `Upload` matches.
  - No `type="file"` matches.
  - No `FormData` matches.
  - No `multipart` matches.
- Existing write-like Web UI actions are workflow editor actions, not plugin upload:
  - `web/src/main.jsx:451-460` `saveDraft()` posts workflow JSON to `/api/workflows/${draft.id}/save`.
  - `web/src/main.jsx:442-445` `validateDraft()` posts workflow JSON to `/api/workflows/${draft.id || 'draft'}/validate`.
  - `web/src/main.jsx:468-477` `runDraft()` posts to `/api/workflows/${draft.id}/run`.
  - `web/src/main.jsx:496-505` renders buttons for 新建 / 校验 / 执行 / 保存 and node/edge deletion.
- Existing fetch helpers:
  - `web/src/main.jsx:748-756` `fetchJSON(path)` assumes JSON response.
  - `web/src/main.jsx:759+` `postJSON(path, payload)` posts JSON; current frontend has no binary/multipart helper.

#### Backend upload-like/write surface

- Search in `internal/server/server.go` found no upload/multipart plugin endpoint.
- Existing write-like endpoint is workflow save:
  - `internal/server/server.go:304-328` routes workflow `POST` suffixes `/run`, `/validate`, `/save`.
  - `internal/server/server.go:399-420` `handleWorkflowSave()` validates a workflow and writes it using `saveWorkflow()`.
  - `internal/server/server.go:579-586` `decodeWorkflow()` decodes JSON body shape `{ workflow: ... }`.
  - `internal/server/server.go:596-610` `saveWorkflow()` writes YAML to disk.
  - `internal/server/server.go:613-621` `workflowPath()` defaults new workflows to root `workflows` if no configured workflow path exists; this is also a legacy path surface.

#### Plugin-first schema and sample references

- `internal/plugin/types.go:5-13` defines `Manifest` fields: `id`, `name`, `version`, `description`, `author`, `compatibility`, `contributes`.
- `internal/plugin/types.go:19-23` defines plugin contributions: categories, tools, workflows.
- `internal/plugin/types.go:25-40` defines plugin tool fields:
  - `id`, `name`, `description`, `version`, `category`, `tags`, `help`, `command`, `args`, `workdir`, `timeout`, `parameters`, `confirm`, `env`.
- `internal/plugin/types.go:42-44` defines plugin workflow contribution as `path`.
- `plugins/plugin.demo/plugin.yaml:1-61` is the current concrete sample plugin:
  - Plugin ID: `plugin.demo`.
  - Category: `plugin-demo`.
  - Tools: `plugin.demo.greet` and `plugin.demo.confirmed`.
  - Commands use plugin-local `scripts/greet.sh` and `scripts/confirmed.sh`.
  - Args use template values like `"{{ .name }}"`.
  - `workdir: .`, `timeout: 1m`, parameters, and confirm metadata are declared in `plugin.yaml`.

### Legacy Path References

Likely replacement surfaces during implementation:

| File Path | Lines | Legacy reference / behavior |
|---|---:|---|
| `internal/server/server.go` | 27-39 | `toolDevKitReadme` says copy `tools/demo/sample-tool` into `tools/<分类>/<工具>/`. |
| `internal/server/server.go` | 41-84 | `toolDevKitSpec` says tools must be under `tools/<分类>/<工具>/`, uses `tool.yaml` and `execution.entry`. |
| `internal/server/server.go` | 86-124 | `sampleToolYAML` uses legacy standalone tool schema (`id: demo.sample-tool`, `execution`, `pass_mode`). |
| `internal/server/server.go` | 500-508 | `buildToolDevKitZip()` emits `tools/demo/sample-tool/...` entries. |
| `internal/server/server_test.go` | 153-157 | `TestToolDevKitDownloadAPI` expects legacy `tools/demo/sample-tool/...` zip entries. |
| `internal/scaffold/scaffold.go` | 10-74 | `NewTool` creates `tools/<category>/<tool>` with `tool.yaml`, `bin/run.sh`, legacy schema. |
| `internal/scaffold/scaffold.go` | 76-110 | `NewWorkflow` creates root `workflows/<id>.yaml` and references `demo.hello`. |
| `internal/server/server.go` | 613-621 | `workflowPath()` default for new workflows is root `workflows` when no configured path exists. |
| `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md` | 29-38 | Lists root `tools/`, `tools/demo/sample-tool`, root `workflows/`, and `workflows/demo-hello` as legacy paths. |

### Related Specs

- `CLAUDE.md:49-61` says the repo is plugin-first, and new runtime tools must not be added under legacy root `tools/` or workflows under root `workflows/`; current runtime structure is `bin/`, `configs/ops.yaml`, `plugins/<plugin-id>/`, `runs/`, `dist/`.
- `CLAUDE.md:79-88` says plugin loader parses `plugins/*/plugin.yaml`, validates IDs and plugin-local paths; registry normalizes plugin tools into existing `config.ToolConfig` and registers source metadata.
- `CLAUDE.md:102-138` gives the plugin package contract:
  - Each plugin lives in `plugins/<plugin-id>/`.
  - Manifest contains plugin metadata and `contributes` categories/tools.
  - Tool IDs must be prefixed by plugin ID plus dot.
  - `command` and `workdir` must stay inside the plugin directory.
  - `plugins.strict` controls bad-plugin handling.
  - `plugins.disabled` accepts plugin ID or directory name.
- `.trellis/spec/backend/directory-structure.md:7-10` says concrete operations are developed as plugin packages wired through `plugins/<plugin-id>/plugin.yaml`.
- `.trellis/spec/backend/directory-structure.md:47` says legacy root `ops.yaml`, `tools/`, `workflows/`, and root `opsctl.exe` are intentionally not part of the latest runtime layout.
- `.trellis/spec/backend/directory-structure.md:53-74` says `paths.tools` and `paths.workflows` stay empty in the latest structure; tools and workflows are contributed by plugins.
- `.trellis/spec/backend/directory-structure.md:75-123` documents plugin manifest and workflow contribution shape.
- `.trellis/spec/backend/directory-structure.md:140-148` says to keep HTTP transport and embedded UI serving in `internal/server`, keep plugin parsing/path validation in `internal/plugin`, and avoid duplicating parameter parsing in UI-specific code.
- `.trellis/spec/guides/index.md:51-59` says updating plugin/runtime docs, dev-kit examples, scaffolds, or shell execution behavior should use the cross-platform runtime guide.
- `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md:20-38` defines current runtime contract and explicitly labels root `tools/`, root `workflows/`, `tools/demo/sample-tool`, and `workflows/demo-hello` as legacy.
- `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md:75-96` says to search docs, tests, scaffold examples, generated dev kits, and local command notes for stale runtime path references before changing runtime behavior.

### External References

- None. This research was internal repository inspection only.

## Concise Implementation Surface Map

| Area | Existing surface | Files / functions |
|---|---|---|
| Backend devkit download route | Current single zip download endpoint for Web devkit. | `internal/server/server.go:232-240`, `toolDevKitHandler()`, `buildToolDevKitZip()` |
| Backend devkit content | Embedded string constants and in-memory zip entry map; currently legacy `tools/demo/sample-tool`. | `internal/server/server.go:27-170`, `internal/server/server.go:500-526` |
| Backend devkit tests | One API test validates status, headers, and zip entries; currently legacy entries. | `internal/server/server_test.go:129-158` |
| Frontend download control | Floating anchor points directly to `/api/dev/toolkit.zip`. | `web/src/main.jsx:138-140`; styles in `web/src/styles.css:114-134` |
| Frontend upload baseline | No plugin upload UI or binary/multipart helper found. Existing JSON post helper and workflow editor buttons are available patterns for POST actions. | `web/src/main.jsx:451-477`, `web/src/main.jsx:748+` |
| Backend upload baseline | No plugin upload endpoint found. Existing POST/write endpoint pattern is workflow save. | `internal/server/server.go:304-328`, `handleWorkflowSave()`, `decodeWorkflow()`, `saveWorkflow()` |
| Plugin schema/template source | Current plugin manifest structs and concrete sample plugin can inform template shape. | `internal/plugin/types.go`, `plugins/plugin.demo/plugin.yaml` |
| Plugin-first constraints | Specs/project instructions forbid new runtime content under root `tools/` / `workflows/`; plugin IDs and paths have prefix/safe-path rules. | `CLAUDE.md`, `.trellis/spec/backend/directory-structure.md`, `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md` |

## Caveats / Not Found

- `.trellis/.current-task` currently contains `.trellis/tasks/00-join-cjg`, but the explicit requested output path is `.trellis/tasks/04-27-04-27-plugin-template-download-upload/research/current-devkit-surface.md`; this file was persisted to the explicit requested task path.
- No existing Web plugin upload control, file input, `FormData`, multipart request, or backend plugin upload endpoint was found.
- Existing devkit download is entirely generated in `internal/server/server.go`; no separate template files for the devkit were found.
- Generated/build artifacts under `dist/` and embedded built frontend assets may contain old references, but implementation source surfaces are the files listed above.

# Research: Workflow plugin export patterns

- **Query**: Research patterns for saving user-created workflows as portable plugin/package assets; compare 2-4 tools such as n8n, Node-RED, GitHub Actions, plugin systems/local package managers; map conventions to this repo's plugin-first YAML runtime and Web workflow editor.
- **Scope**: mixed
- **Date**: 2026-04-29

## Findings

### Files Found

| File Path | Description |
|---|---|
| `graphify-out/GRAPH_REPORT.md` | Architecture navigation snapshot; relevant communities include server catalog/dev-kit, plugin ZIP upload, package build, config workflow schema, and plugin manifest types (`Community 5`, `6`, `9`, `13`, `20`, `23`). |
| `configs/ops.yaml` | Runtime config currently uses `plugins.paths: [plugins]`, `paths.workflows: []`, and empty `menu.categories`, making plugin discovery the current configured source of runtime assets. |
| `internal/config/types.go` | Defines workflow catalog refs and workflow YAML schema: `WorkflowRef` has `category` and `tags` (`lines 80-87`); `WorkflowConfig` also has `category`, `tags`, `parameters`, `nodes`, `edges`, and `confirm` (`lines 140-152`). |
| `internal/plugin/types.go` | Plugin manifest schema: `Manifest` includes `id/name/version/description/author/compatibility/contributes` (`lines 4-12`); `Contributes` includes `categories`, `tools`, and `workflows` (`lines 18-22`); current plugin workflow contribution has only `path` (`lines 41-43`). |
| `internal/plugin/load.go` | Plugin loader validates plugin IDs, required metadata, tool ID prefixing, safe `command`/`workdir` paths, and workflow paths inside plugin root (`lines 73-160`); `SafePath` rejects absolute paths and path escape (`lines 163-179`). |
| `internal/registry/registry.go` | Registry loads plugin workflows from `contributes.workflows[].path`, reads workflow YAML, normalizes entry metadata from workflow YAML, detects duplicate workflow IDs, and records source as plugin (`lines 150-167`, `327-345`). |
| `internal/server/server.go` | Web API exposes `/api/workflows/{id}/save`; save validates then writes YAML to `workflowPath`, updates in-memory registry, and returns saved workflow (`lines 799-820`); catalog includes workflow tags, parameters, confirmation, and source (`lines 954-960`). |
| `internal/server/plugin_upload.go` | Plugin ZIP upload supports raw ZIP or multipart, validates one plugin root, prevents zip-slip/symlinks/special files, enforces size/file limits, installs to first plugin root, and requires version increase on replacement (`lines 20-24`, `61-88`, `120-176`, `178-289`). |
| `internal/packagebuild/packagebuild.go` | Offline package builder copies `configs/` and `plugins/`, optionally current executable, then zips the output (`lines 10-31`, `82-109`). |
| `internal/server/server.go` | Built-in plugin dev-kit documents plugin directory conventions, ZIP root options, workflow path contribution, tags, confirm, validation commands, and sample workflow YAML (`lines 27-77`, `118-124`, `398-442`, `489-493`). |
| `web/src/main.jsx` | Workflow editor builds data-only workflow drafts and calls `/api/workflows/{id}/save`; empty workflow contains `category` and `tags: []`, but visible form fields are ID/name/description/parameters/run params (`lines 766-822`, `878-897`, `1242-1254`, `1356-1386`). |
| `.trellis/spec/backend/workflow-conditional-nodes.md` | Backend/frontend contract for workflow schema round-trip, including data-only condition nodes, edge `case`, and save/detail/run API preservation (`lines 53-107`, `207-214`). |
| `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md` | Project runtime contract states plugin-first layout: `configs/ops.yaml`, `bin/`, `plugins/<plugin-id>/`, and no legacy root `tools/` / `workflows/` assumptions (`lines 19-37`). |

### Code Patterns

#### 1. Current workflow persistence is direct YAML save, not plugin asset creation

- `internal/server/server.go:799-820` handles `POST /api/workflows/{id}/save`: decode, validate, write workflow YAML, then add to `reg.Workflows` with source metadata omitted/defaulted.
- `internal/server/server.go:1007-1021` serializes `config.WorkflowConfig` with `yaml.NewEncoder` and writes it to disk.
- `internal/server/server.go:1024-1032` chooses the path for new workflows: existing workflow path if present; otherwise first `reg.Root.Paths.Workflows`; otherwise fallback `workflows/<id>.yaml`.
- Caveat from current config/spec: `configs/ops.yaml:5-7` sets `paths.workflows: []`, while `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md:21-37` says root `workflows/` is legacy and not part of current runtime structure. This means current Web save behavior has a legacy-path fallback when no workflow path root is configured.

#### 2. Plugin workflows are portable by reference from `plugin.yaml`

- `internal/plugin/types.go:18-22` allows plugins to contribute categories, tools, and workflows.
- `internal/plugin/types.go:41-43` models each contributed workflow as `{ path }`; category/tags/name live in the referenced workflow YAML, not in the manifest entry.
- `internal/plugin/load.go:143-160` validates the workflow file exists and is inside the plugin directory.
- `internal/registry/registry.go:150-167` loads the workflow YAML from the plugin path, normalizes the catalog entry, and records plugin source.
- `internal/registry/registry.go:327-345` fills workflow catalog `id/category/name/description/tags` from the YAML when the `WorkflowRef` is empty.

Current convention already maps portable workflow assets to:

```text
plugins/<plugin-id>/
  plugin.yaml                 # contributes.workflows[].path
  workflows/<workflow>.yaml    # WorkflowConfig with id/name/category/tags/nodes/edges/confirm
```

#### 3. Categories and tags are split between manifest-level categories and workflow YAML metadata

- `internal/config/types.go:66-70` defines `Category` with `id/name/description`.
- `internal/config/types.go:80-87` defines `WorkflowRef` with `category` and `tags` for catalog display.
- `internal/config/types.go:140-147` defines workflow YAML with `category` and `tags`.
- `internal/server/server.go:954-960` puts workflow tags and source into the catalog response.
- `web/src/main.jsx:158-171`, `1609-1622` filter entries by active category/search/tag.
- `web/src/main.jsx:1242-1250` initializes workflow drafts with `category` and `tags: []`.
- `web/src/main.jsx:878-897` shows editor fields for ID/name/description/parameters/run params; there is no visible tag/category editor field in the read range beyond category defaulting from active category.

#### 4. Existing ZIP patterns favor single plugin directory ZIPs

- Plugin upload accepts either ZIP root directly containing `plugin.yaml` or one top-level plugin directory containing `plugin.yaml`; `findUploadedPluginRoot` rejects zero or multiple plugin roots (`internal/server/plugin_upload.go:249-289`).
- Upload extraction rejects unsafe paths, absolute paths, path traversal, symlinks/special files, excessive file count, and excessive uncompressed size (`internal/server/plugin_upload.go:178-246`).
- Replacement requires `replace=true` and a strictly higher plugin version (`internal/server/plugin_upload.go:146-160`).
- Dev-kit text says recommended ZIP structures are either root `plugin.yaml` or one plugin directory with `plugin.yaml`, and only the completed single plugin directory should be delivered (`internal/server/server.go:65-77`, `489-493`).
- Offline package build is host-package oriented: it copies whole `configs/` and `plugins/`, not a single workflow/plugin export (`internal/packagebuild/packagebuild.go:10-31`).

### Comparable Patterns

#### Pattern A: n8n workflow export/import JSON + separate node packages

Observed convention:

- Workflows are exported/imported as data documents, usually JSON.
- Workflow files carry graph structure, nodes, connections, names, settings, and metadata.
- Credentials/secrets are handled separately and are not normally portable as usable secrets in plain workflow export.
- Workflows can reference node types that must exist in the target instance; community nodes are installed as separate packages.
- Tags are instance/catalog metadata used for grouping workflows.

Mapping to this repo:

| n8n convention | Repo equivalent |
|---|---|
| Workflow JSON as data-only graph | `WorkflowConfig` YAML (`id/name/category/tags/parameters/nodes/edges/confirm`) in `internal/config/types.go:140-152` |
| Node type/package dependency must exist on target | Workflow node `tool` values must resolve to registered tools; validation rejects missing tools (`internal/registry/validate.go:57-68`) |
| Credentials separated from exported workflow | Repo parameters are schema/defaults in YAML; runtime values come from CLI/API/Web overrides, params files, or prompts; exported workflow/package should not need run-specific secrets in YAML |
| Tags as catalog grouping | Workflow `tags` are present in YAML and catalog (`internal/config/types.go:146`, `internal/server/server.go:959-960`) |
| Import validates missing nodes/packages | Plugin upload validates plugin manifest and reloads registry (`internal/server/plugin_upload.go:136-174`) |

#### Pattern B: Node-RED flow library/export + npm-packaged custom nodes

Observed convention:

- Flows are portable JSON snippets/files that describe nodes and wires.
- Node-RED custom nodes are npm packages with a manifest (`package.json`) that declares contributed node modules.
- Flow import can expose missing node types when the target runtime lacks the package.
- Reusable flow assets are commonly stored under a library structure, while installable runtime capabilities are packages.

Mapping to this repo:

| Node-RED convention | Repo equivalent |
|---|---|
| Flow JSON with nodes/wires | Workflow YAML with `nodes` and `edges`; condition edge metadata is data-only (`.trellis/spec/backend/workflow-conditional-nodes.md:53-107`) |
| `package.json` declares Node-RED nodes | `plugin.yaml` declares `contributes.tools` and `contributes.workflows` (`internal/plugin/types.go:18-22`) |
| Missing node type after import | Missing `tool` ID during workflow validation (`internal/registry/validate.go:66-68`) |
| Package-local files are installed together | Plugin loader requires workflow paths and tool command/workdir paths remain under plugin directory (`internal/plugin/load.go:126-139`, `151-160`, `163-179`) |

#### Pattern C: GitHub Actions workflow YAML + versioned action/reusable-workflow references

Observed convention:

- Workflows are YAML files in a conventional directory (`.github/workflows`).
- Workflow steps reference external actions by stable identifiers and versions (`owner/repo@ref`).
- Reusable workflows are also YAML and are invoked by reference with a version/ref.
- Secrets and environment-specific variables are supplied by the target repository/environment rather than embedded in portable workflow files.
- Marketplace/action packages carry their own action metadata (`action.yml`) and versioned releases/tags.

Mapping to this repo:

| GitHub Actions convention | Repo equivalent |
|---|---|
| Conventional workflow file path | Plugin convention already uses `workflows/<workflow>.yaml` under plugin root and `plugin.yaml` references it (`internal/server/server.go:44-47`, `118-124`) |
| Steps reference action IDs/version refs | Workflow nodes reference stable tool IDs like `plugin.template.inspect`; plugin has `id` and `version` (`internal/server/server.go:398-442`) |
| Action metadata file | `plugin.yaml` manifest with metadata and contributed tools/workflows (`internal/plugin/types.go:4-22`) |
| Secrets supplied at run/install environment | Workflow parameters/defaults define inputs; runtime params are supplied at execution time, not necessarily embedded in exported workflow YAML |
| Versioned package update behavior | Upload replacement requires higher plugin version (`internal/server/plugin_upload.go:146-160`) |

#### Pattern D: VS Code extension/VSIX-style contributed assets

Observed convention:

- Extension package has one manifest (`package.json`) with metadata, version, categories/keywords, engines compatibility, and `contributes` sections.
- Runtime assets are installed as one package archive (`.vsix`); metadata drives catalog display.
- Contributions reference package-local files/code; the host loads and indexes contributions.

Mapping to this repo:

| VS Code extension convention | Repo equivalent |
|---|---|
| Single manifest with package metadata/version/compatibility | `plugin.yaml` fields `id/name/version/description/author/compatibility` (`internal/plugin/types.go:4-15`) |
| `contributes` declares host-visible features | `contributes.categories/tools/workflows` (`internal/plugin/types.go:18-22`) |
| Marketplace categories/keywords | `contributes.categories` plus workflow/tool `tags` (`internal/config/types.go:66-70`, `89-96`, `140-147`) |
| Installable archive with one extension root | Plugin upload expects exactly one plugin root or root `plugin.yaml` (`internal/server/plugin_upload.go:249-289`) |
| Compatibility metadata | `compatibility.opsctl` exists in manifest type (`internal/plugin/types.go:14-16`) |

### External References

> External web tools were not available in this agent environment; URLs below are stable public documentation locations to verify during implementation planning.

- [n8n workflow export/import documentation](https://docs.n8n.io/workflows/export-import/) — Relevant for treating workflows as portable data documents while keeping credentials/runtime secrets separate.
- [n8n CLI export/import commands](https://docs.n8n.io/hosting/cli-commands/) — Relevant for bulk workflow portability and target-instance import validation.
- [Node-RED import/export user guide](https://nodered.org/docs/user-guide/editor/workspace/import-export) — Relevant for graph-as-data export/import and missing node-type behavior.
- [Node-RED node packaging guide](https://nodered.org/docs/creating-nodes/packaging) — Relevant for package manifest conventions that declare contributed runtime node types.
- [GitHub Actions workflow syntax](https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions) — Relevant for conventional workflow YAML files and step references.
- [GitHub Actions reusable workflows](https://docs.github.com/en/actions/using-workflows/reusing-workflows) — Relevant for workflows as portable referenced assets.
- [GitHub Actions metadata syntax for actions](https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions) — Relevant for action metadata as a manifest analogous to `plugin.yaml`.
- [VS Code extension manifest documentation](https://code.visualstudio.com/api/references/extension-manifest) — Relevant for manifest metadata/categories/keywords and contribution points in packaged extensions.
- [VS Code publishing extensions](https://code.visualstudio.com/api/working-with-extensions/publishing-extension) — Relevant for packaged extension archive conventions and versioned distribution.

### Related Specs

- `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md` — Establishes plugin-first runtime layout and legacy root `workflows/` avoidance.
- `.trellis/spec/backend/workflow-conditional-nodes.md` — Defines workflow YAML/API round-trip expectations, condition nodes, edge cases, and Web editor preservation.
- `.trellis/spec/frontend/workflow-editor-condition-controls.md` — Related frontend editor contract for condition controls and round-trip behavior (found by spec glob; not read in detail for this topic).
- `.trellis/spec/frontend/state-management.md` — Related frontend state conventions (found by spec glob; not read in detail for this topic).

## Caveats / Not Found

- `.trellis/.current-task` in the repository root currently points to `.trellis/tasks/04-28-04-28-workflow-conditional-nodes`; this research was written to the task directory explicitly provided in the request: `.trellis/tasks/04-29-workflow-persistence-plugin-export/research/`.
- No existing endpoint or function named as workflow plugin export was found in the searched code. Existing ZIP functionality covers plugin upload, plugin dev-kit generation, and whole offline package build.
- Current `plugin.Workflow` manifest contribution only has `path`; workflow category/tags/name/description are sourced from the workflow YAML itself after loading.
- Current Web editor initializes and preserves `tags` in the workflow object, but the visible editor form area read for this task only exposes ID/name/description/parameters/run params; tag/category UI controls beyond active category defaulting were not found in the inspected ranges.
- External references should be re-verified against live docs before committing product copy or strict compatibility rules, because no live web search tool was available in this run.

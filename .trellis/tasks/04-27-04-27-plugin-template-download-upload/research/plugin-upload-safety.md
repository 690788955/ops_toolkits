# Research: Plugin Upload Safety

- **Query**: Research common safe patterns for uploading local plugin/template ZIP packages and map them to this repo. Cover safe ZIP extraction checks, atomic install, duplicate plugin ID behavior, validation/reload flow, and recommended MVP approach for Web UI plugin upload.
- **Scope**: mixed
- **Date**: 2026-04-27

## Findings

### Files Found

| File Path | Description |
|---|---|
| `graphify-out/GRAPH_REPORT.md` | Graphify overview; identifies relevant communities for server/devkit, registry/plugin normalization, and plugin loader/path safety. |
| `.trellis/tasks/04-27-04-27-plugin-template-download-upload/prd.md` | Current task requirements and acceptance criteria for plugin template download/upload. |
| `.trellis/spec/backend/quality-guidelines.md` | Backend quality rules: plugin-oriented runtime, path-safety requirements, required checks, and forbidden legacy paths. |
| `configs/ops.yaml` | Runtime plugin configuration; plugin paths currently include `plugins`, strict mode defaults to `false`, disabled list is empty. |
| `internal/plugin/load.go` | Plugin discovery, manifest loading, `ValidatePackage`, strict/lenient load handling, and `SafePath` path containment checks. |
| `internal/plugin/types.go` | Plugin manifest/package/load result data structures. |
| `internal/plugin/load_test.go` | Existing tests for strict/lenient plugin loading, disabled plugins, and path escape rejection. |
| `internal/registry/registry.go` | Registry load/reload mechanics, plugin contribution normalization, category/tool/workflow registration, and conflict handling. |
| `internal/registry/plugin_test.go` | Existing tests for plugin tool/category registration and duplicate tool ID rejection in strict mode. |
| `internal/server/server.go` | HTTP handler setup, catalog API, current devkit ZIP generation, workflow save logic, and in-memory registry usage. |
| `internal/server/server_test.go` | Existing server tests including `/api/dev/toolkit.zip`, catalog metadata, workflow validate/save, run APIs. |
| `internal/packagebuild/packagebuild.go` | Existing ZIP creation code for distributable package output; useful contrast for write-side archive generation, not upload extraction. |
| `web/src/main.jsx` | Current React UI entry; floating download link points to `/api/dev/toolkit.zip`, catalog refresh function already exists. |

### Code Patterns

#### Existing plugin discovery and validation

- `internal/plugin/load.go:14-55` loads plugin packages by scanning configured plugin roots from `config.PluginsConfig.Paths`; for each child directory it reads `plugin.yaml`, validates it, and appends it to `LoadResult.Packages`.
- `internal/plugin/load.go:17-18` resolves configured plugin roots relative to the server/base directory: `rootDir := filepath.Join(baseDir, filepath.FromSlash(root))`.
- `internal/plugin/load.go:34-40` skips disabled plugins by either directory name or manifest ID.
- `internal/plugin/load.go:45-49` applies strict/lenient behavior: strict returns the first plugin error; lenient records a warning and continues.
- `internal/plugin/load.go:70-93` validates required manifest fields (`id`, `name`, `version`) and validates all contributed tools/workflows.
- `internal/plugin/load.go:95-127` validates contributed tools: tool ID required, unique within package, must be prefixed by `<plugin-id>.`, category required, command required, command path safe, command file exists and is not a directory, workdir path safe when set.
- `internal/plugin/load.go:129-147` validates contributed workflows: workflow path required, unique within package, safe path, exists, not directory.
- `internal/plugin/load.go:149-165` implements `SafePath(root, rel)` with these checks:
  - rejects absolute paths via `filepath.IsAbs(rel)`;
  - joins `rel` under the absolute plugin root;
  - rejects resulting paths that are not equal to root and do not have `root + path separator` prefix.

Relevant snippet from `internal/plugin/load.go:149-165`:

```go
func SafePath(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("不允许绝对路径 %s", rel)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	if pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径逃逸插件目录 %s", rel)
	}
	return pathAbs, nil
}
```

#### Registry load/reload and conflict behavior

- `internal/registry/registry.go:41-62` creates a fresh registry by reading `configs/ops.yaml`, loading built-in tools/workflows, loading plugin contributions, then calling `Validate()`.
- `internal/registry/registry.go:104-131` calls `plugin.Load(r.BaseDir, r.Root.Plugins)` and prints lenient warnings to stderr. It then builds plugin contributions and either fails (strict) or skips bad plugin contribution packages (lenient).
- `internal/registry/registry.go:134-170` normalizes plugin tools and workflows into registry entries.
- `internal/registry/registry.go:143-148` rejects duplicate tool IDs against already loaded tools and within the same plugin package.
- `internal/registry/registry.go:161-166` rejects duplicate workflow IDs against already loaded workflows and within the same plugin package.
- `internal/registry/registry.go:246-256` appends plugin categories by category ID and silently ignores duplicate category IDs.
- `internal/registry/plugin_test.go:68-97` verifies duplicate plugin tool ID conflicts can fail registry loading in strict mode.

#### HTTP server and catalog state

- `internal/server/server.go:228-242` constructs handlers from an already-loaded `*registry.Registry`. Existing handlers mutate this registry in memory for saved workflows (`handleWorkflowSave`) but there is no current plugin upload route or registry reload route.
- `internal/server/server.go:236-237` exposes `/api/catalog` and `/api/dev/toolkit.zip`.
- `internal/server/server.go:439-455` returns the current devkit ZIP on `GET /api/dev/toolkit.zip`.
- `internal/server/server.go:500-526` builds the devkit ZIP in memory with `archive/zip`, currently with legacy `tools/demo/sample-tool/...` paths.
- `internal/server/server.go:550-559` builds catalog data from the registry object currently held by the server handler.
- `web/src/main.jsx:41-48` already has `refreshCatalog()`, which fetches `/api/catalog` and updates UI state; this is directly reusable after a successful upload.
- `web/src/main.jsx:139` has the current floating download link: `<a className="downloadKit floatingDownload" href="/api/dev/toolkit.zip">下载工具开发包</a>`.

#### Existing package ZIP generation

- `internal/packagebuild/packagebuild.go:83-111` writes ZIP entries using relative paths from a known local source tree. This is ZIP creation only; upload extraction needs different safety checks because ZIP entry names are untrusted.

### Safe ZIP Extraction Checks

Common safe archive upload patterns for local plugin ZIPs:

1. **Constrain request body size before parsing**
   - Use a server-side maximum upload size, e.g. `http.MaxBytesReader` for HTTP requests.
   - Reject empty files and files above the configured maximum with a readable `400`/`413` response.
   - Keep a separate limit for total uncompressed bytes, because ZIP compressed size can be small while extracted content is large.

2. **Limit ZIP file count and directory depth**
   - Reject archives with too many entries.
   - Reject entries with excessive name length or suspicious deep paths.
   - This limits resource exhaustion and avoids pathological archive traversal costs.

3. **Normalize every ZIP entry name before extraction**
   - ZIP names use slash-separated paths. Treat every `zip.File.Name` as untrusted.
   - Reject blank names.
   - Reject absolute paths. In Go, check both slash and platform forms where relevant:
     - names beginning with `/`;
     - Windows drive paths such as `C:/...` or `C:\...` after conversion;
     - UNC-like paths where applicable.
   - Reject path traversal segments (`..`) after cleaning.
   - Join only under a controlled staging/extraction root, then verify the absolute target remains under that root using the same containment style as `plugin.SafePath`.
   - Avoid trusting `filepath.Clean` alone; validate containment after joining.

4. **Reject or tightly handle symlinks**
   - ZIP entries can encode symlinks in mode bits (`zip.File.FileInfo().Mode()&os.ModeSymlink`).
   - For MVP plugin upload, safest option is to reject symlink entries entirely.
   - Reason: existing `plugin.SafePath` checks lexical path containment, but symlinks can redirect command/workdir or workflow files outside the plugin directory at runtime if extracted and later followed by OS calls. Current `ValidatePackage` uses `os.Stat`, which follows symlinks.
   - If symlinks are ever allowed later, extraction must validate link target containment and avoid following unsafe links; MVP should not do this.

5. **Reject unsafe file modes and special files**
   - Only regular files and directories should be accepted.
   - Reject device files, named pipes, sockets, and symlinks.
   - Preserve executable bit only as needed for plugin scripts; otherwise write normal files with controlled permissions (`0644` for files, `0755` for dirs). On Windows this is less meaningful but safe across platforms.

6. **Track total uncompressed bytes while copying**
   - `zip.File.UncompressedSize64` can be inspected before extraction, but still count actual copied bytes while reading.
   - Reject if cumulative extracted bytes exceed configured maximum.
   - Reject individual files above per-file maximum if desired.

7. **Require expected plugin structure after extraction**
   - The extracted archive should contain exactly one plugin package root for MVP.
   - Acceptable package shapes can be one of:
     - `plugin.yaml` at archive root; or
     - a single top-level directory containing `plugin.yaml`.
   - Normalize both into a staging plugin directory before validation.
   - Reject archives with multiple top-level plugin directories or no `plugin.yaml`.
   - Reject archives that try to include `configs/`, root `ops.yaml`, `tools/`, root `workflows/`, or other project-level paths as active content for MVP, because current runtime package contract is `plugins/<plugin-id>/plugin.yaml` plus plugin-owned files.

8. **Do not execute uploaded content during upload**
   - Upload flow should only parse, validate, install, and reload catalog.
   - Out of scope in the PRD: automatic execution and dependency installation.

Mapping to this repo:

- Reuse `internal/plugin.ValidatePackage` after extraction to enforce manifest-required fields and plugin-local command/workdir/workflow paths (`internal/plugin/load.go:70-147`).
- Add ZIP-level checks before validation because `ValidatePackage` does not inspect arbitrary archive entries, extraction traversal, symlinks, compressed/uncompressed size, or file count.
- Use `plugin.SafePath` or an equivalent staging-root containment helper for each archive entry target. Existing `SafePath` is designed for plugin-local manifest paths; upload extraction needs the same root-containment concept for every ZIP entry.

### Atomic Install Strategy

Common safe install flow for ZIP plugin uploads:

1. Receive upload into memory or a temporary file under a controlled temp directory.
2. Extract into a temporary staging directory outside active plugin roots, e.g. under OS temp or a hidden staging area, not directly under `plugins/`.
3. Normalize archive shape into a staging plugin package directory that contains `plugin.yaml`.
4. Load/parse the staging `plugin.yaml` and validate the staging package with `plugin.ValidatePackage`.
5. Determine final active directory from the manifest ID, e.g. `plugins/<plugin-id>` or a sanitized directory name derived from the ID.
6. Check duplicate behavior before touching active plugin dirs.
7. Move/rename staging directory into final active directory only after validation succeeds.
8. After final install, reload/validate registry before reporting success.
9. If reload fails, remove the newly installed directory or move previous version back if replacing.
10. Clean all temporary files on failure.

Atomicity details:

- On the same filesystem, `os.Rename` is atomic for moving a staged directory into place when destination does not already exist.
- For replacement, a safer two/three-step strategy is:
  1. rename existing `plugins/<plugin-id>` to `plugins/.<plugin-id>.backup-<timestamp>`;
  2. rename staged plugin dir to `plugins/<plugin-id>`;
  3. reload registry;
  4. delete backup on success, or roll back by removing new dir and renaming backup back on failure.
- Do not extract directly into `plugins/<plugin-id>` because a failed extraction or failed validation would leave a partially active plugin visible to `plugin.Load`/`registry.Load`.
- Do not update the in-memory catalog before the on-disk install and registry reload have both succeeded.

Mapping to this repo:

- Active plugin root comes from `configs/ops.yaml` `plugins.paths`, currently `plugins` (`configs/ops.yaml:20-24`). MVP can use the first configured plugin path for installation.
- Current plugin loader scans child directories under each plugin root (`internal/plugin/load.go:17-37`), so any partially extracted child directory under `plugins/` may become visible on the next registry load. This makes staging outside active roots important.
- Server currently receives a fixed `*registry.Registry` in `NewHandler(reg)` and `buildCatalog(reg)` reads from that object. A reload after install must either mutate/replace the in-memory registry used by handlers or have the handler access a registry holder that can be swapped.

### Duplicate Plugin ID Behavior Options

Potential behaviors when uploaded manifest ID already exists:

| Option | Behavior | Notes for this repo |
|---|---|---|
| Reject by default | If `plugins/<plugin-id>` or any loaded package with same manifest ID exists, return conflict. | Safest MVP. Avoids accidental overwrite and avoids rollback complexity. |
| Explicit replace | Accept only with `replace=true` or equivalent UI confirmation; backup existing dir, install staged version, reload, roll back on failure. | Useful after MVP. Requires careful atomic replace and clear UI warning. |
| Version-aware update | Compare existing `manifest.version` with uploaded version; allow newer only or ask on same/downgrade. | Requires version comparison semantics that are not currently enforced. |
| Side-by-side install under unique directory | Install into `plugins/<plugin-id>-<version>` or upload timestamp. | Conflicts with manifest ID uniqueness assumptions and can still collide on contributed tool/workflow IDs. Not ideal for current loader. |
| Merge into existing directory | Overlay uploaded files into existing plugin dir. | Unsafe for MVP; can leave stale files and partial state. Avoid. |

Current repo behavior relevant to duplicates:

- `plugin.Load` does not currently reject duplicate plugin manifest IDs directly; it loads plugin directories and validates each package independently (`internal/plugin/load.go:14-55`).
- `registry.buildPluginPackage` rejects duplicate contributed tool IDs and workflow IDs against already-loaded entries (`internal/registry/registry.go:143-148`, `161-166`).
- Category ID duplicates are ignored/merged by ID (`internal/registry/registry.go:246-256`).
- Disabled plugin handling can reference either directory name or manifest ID (`internal/plugin/load.go:34-40`).

Recommended duplicate semantics for upload MVP:

- Reject duplicate plugin manifest ID by default with HTTP `409 Conflict` or `400 Bad Request` and a readable Chinese message.
- Also reject if target directory `plugins/<plugin-id>` already exists, even if not currently loaded, unless explicit replace is implemented.
- Defer replacement/update behavior to a later iteration.

### Validation / Reload Flow Using Existing Mechanisms

Recommended validation/reload sequence mapped to repo mechanisms:

1. **HTTP upload parse**
   - Add a new backend route near existing dev routes in `internal/server/server.go` (for example `POST /api/plugins/upload` or `POST /api/dev/plugins/upload`).
   - Limit request body size and accept only a ZIP multipart field or raw `application/zip` body, depending on UI choice.

2. **Safe extraction to staging**
   - Extract with the ZIP checks listed above into a temp staging directory.
   - Normalize package root to a directory with `plugin.yaml`.

3. **Manifest and package validation**
   - Load the staging package manifest. `loadPackage` is currently unexported (`internal/plugin/load.go:57`), so implementation choices are:
     - add an exported helper in `internal/plugin` for loading a package from a directory; or
     - parse `plugin.yaml` in server/package upload code and construct `plugin.Package` before calling `plugin.ValidatePackage`.
   - Reuse `plugin.ValidatePackage` (`internal/plugin/load.go:70-93`) and `plugin.SafePath` behavior.

4. **Duplicate checks before install**
   - Check existing plugin directories and/or call `plugin.Load` on the active registry root to identify existing manifest IDs.
   - Reject duplicate plugin ID or final directory existence for MVP.

5. **Atomic install**
   - Move staging plugin dir into the selected active plugin path, e.g. `plugins/<plugin-id>`.
   - Use same-filesystem staging if possible to make final `os.Rename` atomic.

6. **Full registry reload validation**
   - Call `registry.Load(reg.BaseDir)` after install to validate the whole runtime with existing strict/lenient plugin behavior and registry conflict checks.
   - This catches conflicts not visible to per-package validation, such as duplicate contributed tool/workflow IDs with existing packages.
   - If reload fails, roll back/remove the just-installed plugin and return readable error.

7. **Swap active catalog state**
   - Current handlers close over `reg *registry.Registry` (`internal/server/server.go:228-242`), so reloading into a separate `newReg` is not sufficient unless the handler state can be updated.
   - For MVP implementation, introduce a small in-memory registry holder or mutate/swap the registry used by handlers after successful reload. This research does not modify code; this is the flow implication from current server design.
   - After successful swap, Web UI can call `refreshCatalog({keepCategory: true})` (`web/src/main.jsx:41-48`).

8. **Response shape**
   - Return JSON consistent with existing `response` struct (`internal/server/server.go:221-226`), e.g. `status: "installed"`, plugin metadata in `data`, and `error` on failure.

### External References

General platform/library guidance used from common Go and archive-upload practice:

- Go `archive/zip` package behavior: ZIP file entry names are untrusted; callers are responsible for validating names before joining them to filesystem paths, checking file modes, and limiting extracted size/count.
- Go `net/http` upload practice: `http.MaxBytesReader` is the standard request-body guard for server-side upload limits.
- General archive extraction guidance (Zip Slip prevention): reject absolute paths and `..` traversal, clean paths, then verify the final absolute path remains under the intended extraction root. Do not rely on string cleanup alone.
- General symlink guidance: archives can contain symlink entries; rejecting symlinks is the simplest safe policy when extracted content later participates in execution or file validation.

No external web search MCP tool was available in this environment; the above external references are general implementation knowledge and should be verified against current Go docs if exact API signatures are needed during implementation.

### Related Specs

- `.trellis/spec/backend/quality-guidelines.md` — requires plugin-oriented runtime, validates plugin command/workdir paths inside plugin directory, forbids legacy root `tools/`/`workflows/` patterns, and lists test/build checks.
- `.trellis/tasks/04-27-04-27-plugin-template-download-upload/prd.md` — task-specific requirements for plugin template download/upload, safe extraction, validation before activation, catalog refresh, and rollback consideration.

## Recommended MVP Approach for This Repo

1. **Template download**
   - Replace old `/api/dev/toolkit.zip` contents with plugin-first template files under `plugins/<sample-plugin-id>/plugin.yaml`, scripts, examples, and README.
   - Remove active legacy `tools/demo/sample-tool` instructions from current template content.

2. **Upload endpoint**
   - Add a backend `POST` endpoint for plugin ZIP upload using existing JSON response conventions.
   - Limit upload body size, extracted file count, and extracted total uncompressed bytes.
   - Accept a single plugin package per ZIP.

3. **Extraction policy**
   - Extract only to staging outside active `plugins/` roots.
   - Reject absolute paths, traversal paths, symlinks, non-regular/special files, excessive size/count, and archives without one clear `plugin.yaml` root.
   - Reject project-level archive contents outside the plugin package contract.

4. **Validation policy**
   - Reuse `plugin.ValidatePackage` for manifest-level package validation.
   - After installing to active plugin root, call `registry.Load(reg.BaseDir)` to validate the complete runtime and catch ID conflicts.

5. **Install policy**
   - Use first configured plugin path (`configs/ops.yaml` currently has `plugins.paths: [plugins]`) as the install destination root.
   - Install to `plugins/<plugin-id>` after validation.
   - Reject duplicate plugin IDs and existing target directories in MVP. Do not overlay existing directories.
   - If final reload fails, remove the newly installed directory so it is not partially active.

6. **Catalog/UI flow**
   - Only update/swap server registry after successful install and full reload.
   - UI should upload the ZIP, show the readable backend result, then call existing `refreshCatalog({keepCategory: true})` to display new plugin contributions.

## Caveats / Not Found

- No current plugin upload endpoint exists in `internal/server/server.go`.
- No current registry hot-reload holder exists; handlers currently close over a fixed `*registry.Registry`.
- `internal/plugin.loadPackage` is unexported, so upload code cannot directly reuse it outside the `plugin` package without adding an exported helper or duplicating manifest parse logic.
- `plugin.ValidatePackage` checks manifest-declared command/workdir/workflow paths but does not protect ZIP extraction itself, inspect all archive entries, reject symlinks, or enforce upload limits.
- Duplicate plugin manifest IDs are not explicitly rejected by the current loader; conflicts are mainly caught through contributed tool/workflow ID collisions.
- Existing server test for `/api/dev/toolkit.zip` still expects legacy `tools/demo/sample-tool` entries (`internal/server/server_test.go:153-157`), which will need update during implementation.

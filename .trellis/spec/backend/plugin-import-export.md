# Plugin Import and Export Contracts

> Executable backend/API contracts for plugin ZIP import, export, catalog exposure, and safety boundaries.

---

## Scenario: Export an Installed Plugin ZIP

### 1. Scope / Trigger

- Trigger: adding or changing platform plugin export, plugin upload compatibility, catalog plugin metadata, or Web plugin-management download behavior.
- This is a cross-layer API contract: backend route + ZIP structure + catalog payload + Web UI link generation must stay aligned.

### 2. Signatures

- Catalog API: `GET /api/catalog`
  - Response body wraps `catalogResponse` under `data`.
  - `data.categories[]` entries use:
    - category fields from `config.Category` (`id`, `name`, `description`)
    - `disabled: boolean`
    - `source?: {type:string, plugin_id?:string, plugin_name?:string, plugin_version?:string}` when a disabled plugin owns the unavailable category
  - `data.plugins[]` entries use:
    - `id: string`
    - `name: string`
    - `version: string`
    - `description?: string`
    - `disabled: boolean`
- Plugin export API: `GET /api/plugins/{pluginID}.zip`
  - `pluginID` is the installed plugin manifest ID, not a file path.
  - Success headers:
    - `Content-Type: application/zip`
    - `Content-Disposition: attachment; filename="{pluginID}.zip"`
- Reserved plugin routes must remain more specific than the catch-all download route:
  - `GET /api/plugins/user-workflows.zip`
  - `POST /api/plugins/upload`
  - `POST /api/plugins/{pluginID}/disable`
  - `POST /api/plugins/{pluginID}/enable`
  - `DELETE /api/plugins/{pluginID}`
  - `GET /api/plugins/{pluginID}.zip`

### 3. Contracts

- Export is allowed only for registry-known installed plugins:
  - The plugin must be loadable from configured plugin roots.
  - The plugin must contribute at least one currently registered plugin tool or plugin workflow.
- Export ZIP structure must be compatible with upload discovery:
  - The archive contains exactly one plugin package root.
  - The top-level directory name is the plugin directory base name.
  - `plugin.yaml` is inside that root.
  - Plugin-owned regular files are included, including scripts, workflow YAML files, README, and other package-local resources.
- Export must not read or include files outside the plugin directory.
- Export must not rewrite manifest IDs, tool IDs, workflow IDs, or workflow content.

### 4. Validation & Error Matrix

| Condition | Expected behavior |
|---|---|
| Non-GET request to `/api/plugins/{pluginID}.zip` | `405 method not allowed` |
| Path does not end in `.zip` | `404` JSON error `not found` |
| Empty plugin ID | `404` JSON error `not found` |
| Unknown installed plugin | `404` JSON error containing plugin ID |
| Plugin ID contains `/`, `\\`, `%`, quotes, semicolon, spaces, newline, `.` or `..` | reject before filesystem lookup |
| Plugin directory is outside configured plugin roots after symlink evaluation | reject with error |
| Plugin package contains symlink or special file | reject export |
| ZIP entry would be absolute, escaped, or contain unsafe path segment | reject export |

### 5. Good/Base/Bad Cases

- Good: `GET /api/plugins/vendor.backup.zip` exports `vendor.backup/plugin.yaml`, `vendor.backup/scripts/*.sh`, and `vendor.backup/workflows/*.yaml`.
- Base: a plugin with only workflow contributions still appears in `data.plugins[]` and can be exported.
- Bad: `GET /api/plugins/vendor%2Fevil.zip` or `GET /api/plugins/../evil.zip` must not resolve to a filesystem path.

### 6. Tests Required

- HTTP export success test asserts status, ZIP headers, and expected entries.
- ZIP compatibility test exports a plugin, extracts with upload extraction logic, and asserts upload root discovery finds one package.
- Workflow inclusion test asserts contributed workflow YAML files are present and unchanged.
- Unknown plugin test asserts `404`.
- Illegal plugin ID tests assert no path-like or header-unsafe ID is accepted.
- Symlink/special-file safety tests assert export rejects unsafe plugin contents.
- Catalog test asserts `GET /api/catalog` includes exportable plugins with `id`, `name`, and `version`.
- Route precedence test must preserve `/api/plugins/upload` behavior when adding any `/api/plugins/` catch-all route.

### 7. Wrong vs Correct

#### Wrong

```go
mux.HandleFunc("/api/plugins/", pluginDownloadHandler(state))
mux.HandleFunc("/api/plugins/upload", pluginUploadHandler(state))

func buildPluginExportZip(baseDir, pluginID string) ([]byte, error) {
    return zipDir(filepath.Join(baseDir, "plugins", pluginID))
}
```

Why wrong:
- A catch-all route registered without respecting specific routes can shadow upload behavior in some routing setups.
- Treating `pluginID` as a path allows traversal and host file exposure.
- It does not prove the plugin is registry-known or inside a configured plugin root.

#### Correct

```go
mux.HandleFunc("/api/plugins/upload", pluginUploadHandler(state))
mux.HandleFunc("/api/plugins/", pluginDownloadHandler(state))

func buildPluginExportZip(reg *registry.Registry, pluginID string) ([]byte, error) {
    if !isSafePluginExportID(pluginID) {
        return nil, fmt.Errorf("插件 ID 包含不安全路径字符")
    }
    pkg, ok := installedPlugin(reg, pluginID)
    if !ok || !registryKnowsPlugin(reg, pkg) {
        return nil, fmt.Errorf("%w: %s", errPluginNotFound, pluginID)
    }
    if err := ensurePluginDirInConfiguredRoot(reg, pkg.Dir); err != nil {
        return nil, err
    }
    return zipPluginDir(pkg.Dir)
}
```

Why correct:
- Plugin ID is validated as an identifier before filesystem use.
- Export source is derived from installed plugin metadata, not request path concatenation.
- Symlink-resolved plugin directory is checked against configured plugin roots before zipping.

---

## Design Decision: Plugin Export Reuses Upload Package Shape

**Context**: The platform already imports ZIPs by scanning for exactly one `plugin.yaml`; export should create packages that the same importer accepts.

**Options Considered**:
1. Export raw plugin files at ZIP root.
2. Export one top-level plugin directory containing `plugin.yaml`.
3. Export only selected workflow files and synthesize a manifest.

**Decision**: Export one top-level plugin directory containing the existing plugin package. This keeps import/export round-trip behavior simple and avoids rewriting plugin manifests.

**Consequence**: Partial plugin export and multi-plugin bundles are out of scope unless a separate manifest-synthesis contract is introduced.

---

## Design Decision: Runtime-Only Fields Must Not Persist to Disk Config

**Context**: The registry merges built-in categories with plugin-contributed categories at runtime. When `SaveRoot` writes `configs/ops.yaml`, it must exclude runtime-only fields to prevent plugin-contributed categories from persisting to disk config after plugin deletion.

**Problem**: Without exclusion, deleting a plugin leaves its contributed categories in `configs/ops.yaml menu.categories`, causing stale category residue.

**Solution**: Mark runtime-only fields with `yaml:"-"` struct tags and clear them before YAML encoding.

**Implementation Pattern**:

```go
// RootConfig includes runtime-only fields marked with yaml:"-"
type RootConfig struct {
    Menu              MenuConfig `yaml:"menu"`
    RuntimeCategories []Category `yaml:"-" json:"-"`  // Runtime-only, never persisted
    // ... other fields
}

// SaveRoot clears runtime-only fields before encoding
func SaveRoot(path string, cfg *RootConfig) error {
    disk := *cfg
    disk.RuntimeCategories = nil  // Clear runtime-only field
    return encodeYAML(path, &disk)
}
```

**Why This Works**:
- `yaml:"-"` tag tells the YAML encoder to skip the field during marshaling
- Explicit `nil` assignment before encoding provides defense-in-depth
- Struct copy (`disk := *cfg`) prevents mutating the live registry config

**Related**: This pattern applies to any registry-computed field that must not persist (e.g., merged tool lists, computed workflow graphs).

---

## Scenario: Disable and Delete an Installed Plugin

### 1. Scope / Trigger

- Trigger: adding or changing plugin disable/enable/delete APIs, `plugins.disabled` persistence, catalog disabled-state exposure, or Web plugin-management destructive actions.
- This is a cross-layer API contract: backend routes, `configs/ops.yaml`, registry reload behavior, catalog payload, and Web UI state/actions must stay aligned.
- Plugin deletion is a local filesystem operation and must keep the same path-safety bar as plugin export.

### 2. Signatures

- Disable API: `POST /api/plugins/{pluginID}/disable`
  - `pluginID` is the installed plugin manifest ID, not a file path and not a `.zip` route suffix.
  - Request body is currently ignored; callers should send `{}`.
  - Success response:
    ```json
    {"status":"disabled","data":{"plugin_id":"vendor.backup","status":"disabled"}}
    ```
- Enable API: `POST /api/plugins/{pluginID}/enable`
  - `pluginID` is the disabled installed plugin manifest ID, not a file path and not a `.zip` route suffix.
  - Request body is currently ignored; callers should send `{}`.
  - Success response:
    ```json
    {"status":"enabled","data":{"plugin_id":"vendor.backup","status":"enabled"}}
    ```
- Delete API: `DELETE /api/plugins/{pluginID}`
  - Deletes only an already-disabled installed plugin directory.
  - Success response:
    ```json
    {"status":"deleted","data":{"plugin_id":"vendor.backup","status":"deleted"}}
    ```
- Catalog API: `GET /api/catalog`
  - `data.plugins[]` includes both enabled installed plugins and disabled-but-still-installed plugins.
  - Disabled plugins set `disabled: true` and must remain visible so Web UI can show the delete action after disable.
  - Disabled plugins' contributed tools/workflows must not appear in `data.tools[]` or `data.workflows[]`.
  - `data.categories[]` keeps disabled plugin-owned categories visible when no enabled tool/workflow still uses that category.
  - Disabled category entries set `disabled: true` and include `source.plugin_id`, `source.plugin_name`, and `source.plugin_version` so Web UI can render a grey disabled category with plugin context.
- Reserved plugin routes must remain more specific than the catch-all plugin route:
  - `GET /api/plugins/user-workflows.zip`
  - `POST /api/plugins/upload`
  - `POST /api/plugins/{pluginID}/disable`
  - `POST /api/plugins/{pluginID}/enable`
  - `DELETE /api/plugins/{pluginID}`
  - `GET /api/plugins/{pluginID}.zip`

### 3. Contracts

- Disable is required before delete:
  - `POST /api/plugins/{pluginID}/disable` appends the manifest ID to `configs/ops.yaml` `plugins.disabled` if not already present.
  - Registry reload after disable must succeed before the API returns success.
  - If registry reload fails after writing `plugins.disabled`, the previous disabled list must be restored before returning an error.
- Enable restores a disabled installed plugin:
  - `POST /api/plugins/{pluginID}/enable` removes both the manifest ID and plugin directory basename from `configs/ops.yaml` `plugins.disabled`.
  - Registry reload after enable must succeed before the API returns success.
  - If registry reload fails after writing `plugins.disabled`, the previous disabled list must be restored before returning an error.
- Delete is allowed only for installed plugins that are already disabled:
  - The plugin may be located by currently installed package metadata or by a disabled installed package scan.
  - The request path must never be concatenated directly into a deletion target.
  - Deletion must remove only the plugin package directory; it must not delete run logs, user workflow records, or plugin-created data outside the package directory.
  - After successful directory removal, remove matching manifest ID and directory-name entries from `plugins.disabled`, then reload registry.
  - If deletion fails before the directory is removed, keep `plugins.disabled` unchanged.
- Safety boundaries:
  - Reuse the same safe plugin ID constraints as export: trimmed, non-empty, no path separators, no `%`, no `.`/`..`, no spaces/control/header-unsafe characters.
  - Symlink-resolved plugin directory must be strictly under a configured `plugins.paths` root.
  - The plugin directory must be a real directory, not a symlink or special file, and `plugin.yaml` must be a regular file before deletion.
- Web UI behavior:
  - Enabled plugins show a `禁用` action.
  - Disabled plugins show `启用` and `删除` actions.
  - Disabled plugin-owned sidebar categories remain visible but greyed out, are not clickable, and show Chinese disabled-plugin context.
  - If the active category becomes disabled after catalog refresh, Web UI must leave that category context instead of showing an empty usable state.
  - Workflow editor category selectors must exclude disabled categories.
  - Delete must require an explicit Chinese confirmation before sending `DELETE`.
  - After disable/enable/delete succeeds, refresh catalog and show a readable Chinese result.

### 4. Validation & Error Matrix

| Condition | Expected behavior |
|---|---|
| Non-POST request to `/api/plugins/{pluginID}/disable` | `405 method not allowed` |
| Non-POST request to `/api/plugins/{pluginID}/enable` | `405 method not allowed` |
| Unknown plugin on disable | `404` JSON error containing plugin ID |
| Unknown plugin on enable | `404` JSON error containing plugin ID |
| Enable disabled plugin | remove matching manifest ID and directory-name entries from `plugins.disabled`, reload registry, return enabled status |
| Enable path uses `{pluginID}.zip` instead of manifest ID | reject with readable Chinese error |
| Unknown plugin on delete | `404` JSON error containing plugin ID |
| Plugin ID contains `/`, `\\`, `%`, quotes, semicolon, spaces, newline, `.` or `..` | reject before filesystem lookup |
| Delete path uses `{pluginID}.zip` instead of manifest ID | reject with readable Chinese error |
| Delete enabled plugin | `400` JSON error telling caller to disable first |
| Plugin directory is outside configured plugin roots after symlink evaluation | reject with error |
| Plugin directory or `plugin.yaml` is symlink/special/missing before delete | reject; keep disabled config |
| Disable writes config but registry reload fails | restore previous `plugins.disabled`; return error |
| Enable writes config but registry reload fails | restore previous `plugins.disabled`; return error |
| Delete removes directory but disabled cleanup or registry reload fails | return error explaining partial state |

### 5. Good/Base/Bad Cases

- Good: user disables `vendor.backup`; catalog no longer lists `vendor.backup.*` tools/workflows, still lists plugin `{id:"vendor.backup", disabled:true}`, and keeps plugin-only category `{id:"backup", disabled:true, source:{plugin_id:"vendor.backup", ...}}` for greyed sidebar display; user confirms delete; plugin directory is removed and disabled entry is cleaned.
- Base: user disables a plugin and leaves it installed. It remains listed as disabled in plugin management and its plugin-only categories remain visible as disabled until delete.
- Bad: user calls `DELETE /api/plugins/vendor.backup` while the plugin is enabled; backend rejects the request even if the Web UI would normally hide delete.

### 6. Tests Required

- Disable success test asserts HTTP status, `plugins.disabled` persistence, registry/catalog refresh, and disappearance of plugin tools/workflows.
- Catalog test asserts disabled installed plugins remain in `data.plugins[]` with `disabled:true`, and plugin-only disabled categories remain in `data.categories[]` with `disabled:true` and plugin `source` metadata.
- Catalog test asserts shared categories remain enabled when any enabled tool/workflow still references the category.
- Enable success test asserts HTTP status, removal of manifest ID and directory-name entries from `plugins.disabled`, registry/catalog refresh, and restored plugin tools/workflows.
- Enable unknown, unsafe ID, `.zip` path, non-POST method, and registry-reload rollback tests assert `404`/`400`/`405` behavior and restored `plugins.disabled`.
- Delete success test asserts enabled delete is rejected first, disabled delete removes directory, cleans disabled config, removes plugin-only stale disk categories that match the deleted plugin contribution, and refreshes catalog.
- Delete category tests assert user-authored categories that only share a plugin category ID are not removed, and shared categories remain when another enabled tool/workflow still references them.
- Unknown plugin tests assert `404` for disable/delete.
- Illegal plugin ID tests assert no path-like or header-unsafe ID is accepted for disable/delete.
- Failure tests assert reload failure after disable rolls back config, and delete failure before directory removal preserves disabled config.
- Route precedence tests assert upload, user-workflow export, plugin export, disable, enable, and delete routes do not shadow one another.
- Web build test must pass: `npm run build --prefix web`.

### 7. Wrong vs Correct

#### Wrong

```go
func deletePlugin(baseDir, pluginID string) error {
    return os.RemoveAll(filepath.Join(baseDir, "plugins", pluginID))
}
```

Why wrong:
- Treats request text as a filesystem path.
- Allows destructive deletion without first verifying installed plugin metadata, disabled state, and configured plugin-root containment.
- Provides no rollback/consistency handling for `plugins.disabled`.

#### Correct

```go
func deleteDisabledPlugin(state *serverState, pluginID string) (pluginActionResult, error) {
    if !isSafePluginExportID(pluginID) {
        return result, fmt.Errorf("插件 ID 包含不安全路径字符")
    }
    pkg, ok := installedAnyPlugin(reg, pluginID)
    if !ok || !pluginDisabled(reg.Root.Plugins.Disabled, pkg) {
        return result, fmt.Errorf("插件未禁用，请先禁用插件 %s", pluginID)
    }
    if err := ensurePluginDirInConfiguredRoot(reg, pkg.Dir); err != nil {
        return result, err
    }
    if err := ensureDeletablePluginDir(pkg.Dir); err != nil {
        return result, err
    }
    // remove package dir, then clean disabled config and reload registry
}
```

Why correct:
- Plugin identity is validated as an ID before filesystem access.
- Delete target is derived from installed plugin metadata/scans, not request path concatenation.
- Disabled state is a backend-enforced precondition, and config cleanup happens only after successful deletion.

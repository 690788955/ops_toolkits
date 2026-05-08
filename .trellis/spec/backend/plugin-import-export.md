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

## Scenario: Host Absolute Config File Mapping

### 1. Scope / Trigger

- Trigger: adding or changing `tools[].config_files`, host-side plugin mapping, plugin config file API, or Web plugin configuration editing.
- This is a backend/API/UI safety contract: plugin manifests, `configs/ops.yaml`, `configs/plugins/<plugin-id>.mapping.yaml`, registry normalization, HTTP routes, and Web editing behavior must remain aligned.

### 2. Signatures

- Global whitelist config in `configs/ops.yaml`:
  ```yaml
  host_config_files:
    allowed_dirs:
      - /etc/myapp
      - C:\\ProgramData\\Vendor
  ```
  - `allowed_dirs` accepts directory prefixes only.
  - Single-file whitelist entries are invalid.
- Plugin manifest `plugin.yaml` may use a relative or absolute `config_dir` plus relative `config_files`; relative `config_dir` is resolved from the plugin directory and may intentionally point outside the plugin directory for internal shared-config scenarios, while absolute `config_dir` is used directly as the configuration base directory:
  ```yaml
  config_dir: ../../shared-config
  config_files:
    - app.conf
  ```
  ```yaml
  config_dir: /etc/myapp
  config_files:
    - app.conf
  ```
- Host-side plugin mapping may use structured entries. `config_dir` is the base directory and `path` is the relative `config_files` item:
  ```yaml
  tools:
    vendor.app.reload:
      config_files:
        - id: app-main
          label: App 主配置
          scope: host_absolute
          config_dir: /etc/myapp
          path: app.conf
          access: read_write
          create: false
  ```
- Config file API:
  - `GET /api/plugins/{pluginID}/files` returns `data.files[]` objects containing at least `id`, `path`, `scope`, `access`, `create`, `exists`, `readable`, `writable`, and `reason`.
  - `GET /api/plugins/{pluginID}/files/{fileID}` reads only a registry-declared ID or legacy plugin-local path.
  - `PUT /api/plugins/{pluginID}/files/{fileID}` writes only a registry-declared ID or legacy plugin-local path.

### 3. Contracts

- Plugin tool `command` supports two safe forms:
  - Path-like commands containing `/` or `\\` must stay inside the plugin directory.
  - Bare command names such as `java` or `ansible-playbook` may execute through the runtime PATH only when explicitly listed in `plugins.allowed_commands`.
- Plugin package config-file safety boundary is based on the resolved `config_dir`:
  - `plugin.yaml` `config_dir` may be relative to the plugin directory or an absolute path recognized by the current platform.
  - Internal deployments may use `..` in relative `config_dir`, for example `../../shared-config`, with care.
  - Absolute `config_dir` is used directly as the configuration base directory; plugin packages still cannot grant arbitrary file access through `config_files` because entries must stay under that final base.
- `config_dir + config_files` is the only path construction rule:
  - `config_dir` defaults to plugin `config/` for new structured declarations.
  - `path`/`config_files` entries are always relative file or directory items; absolute paths and `..` escape from the resolved `config_dir` are invalid.
  - A directory item expands to one-level regular files only; recursion is not part of the MVP.
- Host absolute files are enabled only by host-side mapping plus global whitelist:
  - `scope: host_absolute` requires `config_dir` to be an absolute directory recognized by the current platform.
  - The cleaned absolute `config_dir`, and the symlink-resolved final base when it exists, must be inside one configured `host_config_files.allowed_dirs` directory.
  - For non-existing files under an existing base, the nearest existing parent directory must be symlink-resolved before the final path is checked against the whitelist.
- Application access is separate from OS permissions:
  - `access: read` allows GET when OS checks pass and rejects PUT.
  - `access: read_write` allows PUT only when OS checks pass.
  - Non-existing files can be created only when `create: true` and the parent directory is writable.
- File type and size checks are mandatory:
  - GET/PUT must reject directories and special files; only regular files are editable.
  - GET must reject files above the configured server-side size limit.
- URL path is not a filesystem path:
  - The API must resolve `{fileID}` through registry declarations.
  - URL-encoded traversal such as `%2e%2e`, slash-bearing undeclared values, or arbitrary absolute paths must not be read or written.
- Delete behavior:
  - Plugin-local config files may keep existing delete semantics.
  - Host absolute config files must reject DELETE by default.
- Plugin export ZIPs must include only plugin-owned files from the plugin directory and must not read or include host absolute config file targets.

### 4. Validation & Error Matrix

| Condition | Expected behavior |
|---|---|
| `plugin.yaml` config_dir is relative and config file stays inside that config_dir | load succeeds; file can be listed/read/written through plugin-scope declaration, even when config_dir points outside plugin dir |
| `plugin.yaml` config_dir is absolute and config file stays inside that config_dir | load succeeds; file can be listed/read/written through plugin-scope declaration using the absolute base |
| `plugin.yaml` config file is absolute or escapes resolved config_dir | reject as unsafe plugin config file path |
| mapping structured host `config_dir` is inside whitelist and `path` is relative | registry registers it and API lists structured status |
| mapping host `config_dir` is outside whitelist | registry rejects mapping with readable Chinese whitelist error |
| whitelist entry points to a file | registry rejects; whitelist supports directories only |
| host mapping is `access: read` | GET allowed if OS-readable; PUT rejects with read-only error |
| host mapping is `access: read_write` | PUT allowed only if file or parent directory is OS-writable |
| missing host file with `create: false` | list reports not readable/not writable and PUT rejects |
| missing host file with `create: true` and writable parent | PUT creates the file |
| host `config_dir` resolves through symlink outside whitelist | registry/API rejects before editing |
| file item resolves to directory or special file | list reports reason; GET/PUT reject, except declared directory items expand one-level in list |
| undeclared `{fileID}` or encoded traversal | reject; do not touch filesystem path from URL |
| DELETE host absolute config file | reject by default |

### 5. Good/Base/Bad Cases

- Good: `configs/ops.yaml` whitelists `/etc/myapp`, mapping declares `id: app-main`, `scope: host_absolute`, `config_dir: /etc/myapp`, `path: app.conf`, `access: read_write`; API lists status and PUT updates the regular file when the process can write it.
- Base: plugin manifest declares `config/example.conf`; API remains backward compatible and legacy URL `/files/configs%2Fapp.conf` continues to resolve only when declared.
- Bad: caller sends `/api/plugins/vendor.app/files/%2Fetc%2Fpasswd`; backend rejects because the URL value is not a declared ID.

### 6. Tests Required

- Registry test for structured host mapping accepted inside whitelist.
- Registry test for host mapping rejected outside whitelist and for single-file whitelist rejection when applicable.
- Server test for structured list status including `scope`, `access`, `exists`, `readable`, `writable`, and `reason`.
- Server tests for read-only GET/PUT rejection, read-write PUT success, `create:false` missing rejection, `create:true` creation, directory/special-file rejection, undeclared/encoded traversal rejection, and host DELETE rejection.
- Existing plugin-local config file read/write tests must continue to pass, including plugin-scope absolute `config_dir` list/read/write and escape rejection.
- Web build test must pass after list API response shape changes.

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

---

## Scenario: Plugin Integration Warnings

### 1. Scope / Trigger

- Trigger: adding or changing plugin integration quality checks, `opsctl validate` output, plugin upload response payloads, or catalog warning fields.
- This is a cross-layer contract: plugin loader generates warnings, registry stores them, CLI/API expose them, and runtime tool/workflow execution must ignore them.

### 2. Signatures

- Warning JSON shape:
  - `code: string`
  - `plugin_id: string`
  - `tool_id?: string`
  - `field?: string`
  - `message: string`
  - `suggestion?: string`
- CLI: `opsctl validate`
  - Success still returns exit code `0` when only warnings exist.
  - Output includes a `Warning: <count>` section after counts.
- Upload API: `POST /api/plugins/upload`
  - Success response `data.warnings?: Warning[]`.
- Catalog API: `GET /api/catalog`
  - Response `data.warnings?: Warning[]` for registry-level enabled-plugin warnings.
  - Response `data.plugins[].warnings?: Warning[]` for each enabled plugin.

### 3. Contracts

- Warnings are non-blocking integration-quality findings; they must not reject plugin load, validate, upload, or tool/workflow execution.
- `plugins.strict` only controls errors. It must not convert integration warnings into load failures.
- First-version warning codes:
  - `PLUGIN_README_MISSING`
  - `PLUGIN_DESCRIPTION_MISSING`
  - `TOOL_DESCRIPTION_MISSING`
  - `PARAM_DESCRIPTION_MISSING`
  - `CONFIRM_MESSAGE_TOO_SHORT`
  - `CONFIG_FILE_MISSING`
  - `PLUGIN_LOAD_SKIPPED` for non-strict skipped invalid plugins
- Warning generation checks only plugin package contract and declared files; it must not inspect or constrain script business logic.
- Disabled plugin contributions do not appear in runtime `tools`/`workflows`; catalog may list disabled plugin status, but disabled-package quality warnings should not make tests or UI treat disabled tools as active contributions.

### 4. Validation & Error Matrix

| Condition | Expected behavior |
|---|---|
| Valid plugin lacks `README.md` | warning, load succeeds |
| Valid plugin or tool lacks `description` | warning, load succeeds |
| Tool parameter lacks `description` | warning, load succeeds |
| `confirm.required=true` and message is empty or too short | warning, load succeeds |
| `config_files` path is declared but file is missing | warning, load/upload succeeds |
| `config_files` path is unsafe or directory | error, follows `plugins.strict` behavior |
| Only warnings during `opsctl validate` | exit code `0` |
| Tool/workflow run after warnings exist | no warning prompt or warning output during run |

### 5. Good/Base/Bad Cases

- Good: plugin uploads successfully with `data.warnings` describing missing README and missing config file, then catalog exposes the same warnings for the installed enabled plugin.
- Base: a warning-free plugin returns no `warnings` field or an empty warning list depending on JSON omitempty behavior.
- Bad: making `plugins.strict: true` fail validation because README is missing.

### 6. Tests Required

- Plugin unit test asserts all first-version warning codes can be generated without `ValidatePackage` failure.
- Upload API test asserts successful upload response includes structured warnings.
- Catalog API test asserts installed enabled plugin warnings are exposed.
- Full Go test suite must pass; build and validate commands must still succeed with only warnings.

### 7. Wrong vs Correct

#### Wrong

```go
if len(plugin.PackageWarnings(pkg)) > 0 && cfg.Strict {
    return fmt.Errorf("plugin warnings found")
}
```

#### Correct

```go
result.Packages = append(result.Packages, pkg)
result.Warnings = append(result.Warnings, plugin.PackageWarnings(pkg)...)
```

Why correct:
- Warning collection is side-channel metadata.
- Strict error policy remains reserved for invalid manifests, unsafe paths, and missing required runnable assets.
- Runtime execution stays focused on parameters, confirmation, and script exit status.

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
  - `data.plugins[]` entries use:
    - `id: string`
    - `name: string`
    - `version: string`
    - `description?: string`
- Plugin export API: `GET /api/plugins/{pluginID}.zip`
  - `pluginID` is the installed plugin manifest ID, not a file path.
  - Success headers:
    - `Content-Type: application/zip`
    - `Content-Disposition: attachment; filename="{pluginID}.zip"`
- Reserved plugin routes must remain more specific than the catch-all download route:
  - `GET /api/plugins/user-workflows.zip`
  - `POST /api/plugins/upload`
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

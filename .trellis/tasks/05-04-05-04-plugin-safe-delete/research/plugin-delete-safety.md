# Research: plugin deletion safety

- **Query**: Research safe plugin deletion/disable conventions for local plugin-based developer/admin tools. Current repo context: F:\ccb\ops_toolkits is a local ops automation platform with plugins under `plugins/<plugin-id>/`, config `configs/ops.yaml` has `plugins.disabled: []`, Web plugin manager already supports upload/export. User wants plugin deletion, preferably disable before delete. Research 2-4 comparable patterns focused on: requiring disable before delete, confirmation UX, backend safety boundaries for filesystem deletion, and whether deletion should remove config disabled entries.
- **Scope**: mixed
- **Date**: 2026-05-04

## Findings

### Files Found

| File Path | Description |
|---|---|
| `configs/ops.yaml:19-23` | Runtime plugin config uses `plugins.paths: [plugins]`, `strict: false`, and `disabled: []`. |
| `.trellis/spec/backend/directory-structure.md:52-70` | Documents the canonical runtime config shape, including `plugins.disabled: []`. |
| `.trellis/spec/backend/directory-structure.md:104-109` | Documents plugin package rules and states that `disabled` accepts plugin IDs or plugin directory names. |
| `.trellis/spec/backend/plugin-import-export.md:15-30` | Defines catalog and plugin export routes, including route precedence for `/api/plugins/upload` and `/api/plugins/{pluginID}.zip`. |
| `.trellis/spec/backend/plugin-import-export.md:34-57` | Defines export safety boundaries: installed/registry-known plugin only, plugin ID is not a path, symlink-resolved directory must remain inside configured plugin roots, reject symlinks/special files and unsafe ZIP paths. |
| `internal/plugin/load.go:13-18` | Loader builds a disabled set from `cfg.Disabled` before scanning configured plugin roots. |
| `internal/plugin/load.go:29-40` | Loader skips a plugin when either the directory name or manifest ID appears in `plugins.disabled`. |
| `internal/plugin/load.go:98-107` | Plugin ID validation rejects empty IDs, `.`/`..`, and slash/backslash path characters. |
| `internal/plugin/load.go:163-179` | `SafePath` rejects absolute paths and relative paths escaping the plugin directory. |
| `internal/server/server.go:638-646` | HTTP route registration places `/api/plugins/user-workflows.zip` and `/api/plugins/upload` before catch-all `/api/plugins/`. |
| `internal/server/server.go:915-940` | Current catch-all plugin route is GET-only export; no delete route exists in the read code. |
| `internal/server/plugin_upload.go:121-177` | Upload installs into the first configured plugin root, detects duplicates, requires `replace=true` for update, reloads registry after install/update, and rolls back new install if reload fails. |
| `internal/server/plugin_upload.go:179-248` | Upload ZIP extraction rejects too many files, oversized content, symlinks/special files, absolute paths, unsafe path segments, and path escape. |
| `internal/server/plugin_upload.go:313-335` | Plugin replace flow renames existing install to a backup, copies new files, reloads registry, restores backup on failure, and removes backup on success. |
| `internal/server/plugin_upload.go:384-399` | Export resolves plugin by safe ID, requires registry-known plugin, checks directory under configured plugin root, then zips plugin directory. |
| `internal/server/plugin_upload.go:401-415` | Export-safe plugin IDs must be trimmed, non-empty, not `.`/`..`, contain no path separators or `%`, match clean path, and use only letters, digits, `.`, `_`, `-`. |
| `internal/server/plugin_upload.go:448-475` | Export verifies symlink-resolved plugin directory is strictly under a configured plugin root. |
| `internal/server/server_test.go:480-500` | Tests reject unsafe plugin IDs in direct export and HTTP export requests. |
| `web/src/main.jsx:342-405` | Web plugin manager modal supports template download, plugin ZIP upload, duplicate update confirmation, user-workflow export, and installed-plugin export modal. |
| `web/src/main.jsx:409-435` | Installed-plugin export modal lists catalog plugins and links to `/api/plugins/{pluginID}.zip`; no delete UI exists in the read code. |

### Code Patterns

- Disabled plugin semantics are loader-level filtering, not filesystem deletion. `plugin.Load` constructs `disabled := disabledSet(cfg.Disabled)` then skips first by directory name and later by manifest ID (`internal/plugin/load.go:13-40`).
- The existing loader supports both tombstone styles: `plugins.disabled` can contain the directory basename before manifest parsing or the manifest ID after manifest parsing (`internal/plugin/load.go:33-39`; `.trellis/spec/backend/directory-structure.md:104-109`).
- Existing upload/export filesystem boundaries are identifier-first and registry/config-root based. Export does not concatenate untrusted route text into a path; it validates `pluginID`, resolves installed package metadata, verifies registry visibility, and checks the symlink-real plugin directory under configured plugin roots before reading (`internal/server/plugin_upload.go:384-475`).
- Existing destructive/update operations use a mutex and registry reload validation. Upload install/update holds `state.mu`, writes under the configured plugin root, reloads the registry after changing files, and rolls back on reload failure (`internal/server/plugin_upload.go:121-177`, `internal/server/plugin_upload.go:313-335`).
- Existing web upload has a second-step confirmation only for duplicate replacement: server returns `409` with existing version; UI then shows `确认更新` (`web/src/main.jsx:348-389`).

### External References

- Jenkins Managing Plugins — removing and disabling plugins: Jenkins distinguishes uninstall from disable. Disable is softer retirement; uninstall does not remove plugin-created configuration automatically.
- WordPress Manage Plugins / source: normal admin deletion requires deactivation first; server-side deletion refuses active plugins and shows an explicit confirmation screen.
- Drupal module uninstall docs: uninstall/disable semantics happen before code folder removal.
- Azure CLI extension remove: local extensions are removed by name after resolving installed metadata, with safety restrictions for system/dev extensions.

### Comparable Patterns

| System | Disable before delete? | Confirmation UX | Backend deletion boundary | Config/tombstone handling observed |
|---|---|---|---|---|
| Jenkins plugins | Disable is alternative/softer retirement; uninstall can also be direct. | UI/CLI surfaces enable/disable and warns about dependencies/old config. | Plugin paths are under `JENKINS_HOME/plugins`; disable marker is file tombstone. | Uninstall does not purge plugin-created config automatically. |
| WordPress plugins | Yes for normal admin deletion. | Two-step deactivate then delete, with explicit verification. | Deletes validated plugin paths under known plugins dir. | Active-state entry is removed by deactivation; data removal is separate. |
| Drupal modules | Yes conceptually: uninstall before folder removal. | Admin uninstall tab or Drush command. | Code removal follows uninstall. | Uninstall removes module config; folder removal follows. |
| Azure CLI extensions | No disable-before-delete convention in public docs. | `az extension remove --name`. | Resolves installed extension by name and removes resolved path. | No disabled-list cleanup pattern found. |

### Related Specs

- `.trellis/spec/backend/directory-structure.md` — Defines current plugin-first layout, `configs/ops.yaml` plugin config, and `plugins.disabled` semantics.
- `.trellis/spec/backend/plugin-import-export.md` — Defines plugin import/export API contracts, catalog payload, route precedence, and filesystem/ZIP safety boundaries adjacent to deletion.
- `.trellis/spec/backend/quality-guidelines.md` — Mentions plugin export safety test coverage and plugin upload route preservation requirements.

## Caveats / Not Found

- No existing plugin delete/disable HTTP route or Web delete UI was found; current Web plugin manager supports upload/update confirmation and export only.
- Public Azure CLI docs/source snippets did not show a disabled-entry or tombstone cleanup convention for extension removal.

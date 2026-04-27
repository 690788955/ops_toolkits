# brainstorm: Plugin Template Download and Upload

## Goal

Update the Web UI development package flow from the old root `tools/` layout to the current plugin-first model, and add user-facing actions to download a plugin template and upload a plugin package.

## What I already know

* The current Web UI has a floating link labeled `下载工具开发包` that points to `/api/dev/toolkit.zip`.
* The backend route `/api/dev/toolkit.zip` is implemented by `toolDevKitHandler()` and `buildToolDevKitZip()` in `internal/server/server.go`.
* The current downloaded ZIP still describes and generates legacy paths such as `tools/demo/sample-tool/tool.yaml` and `tools/demo/sample-tool/bin/run.sh`.
* The project runtime is now plugin-first: `configs/ops.yaml`, `plugins/<plugin-id>/plugin.yaml`, and `bin/opsctl.exe`.
* Existing plugin loading and validation live in `internal/plugin` and `internal/registry`.
* The Web UI already consumes plugin source metadata from `/api/catalog`.
* Graphify points at relevant areas: server/catalog/devkit, registry/plugin normalization, runner execution, and plugin loader/SafePath.
* The downloaded plugin template should be developer-facing: it should explain only how to create, validate, package, upload, and run plugins in this platform.

## Scope Decisions

* "Download plugin template" should replace or supersede the old tool devkit ZIP.
* "Upload plugin" means uploading a local ZIP from the Web UI and installing it into `plugins/<plugin-id>/`, then refreshing the catalog.
* Uploaded plugins should reuse the existing plugin manifest validation and path-safety rules.
* This task targets a product-grade feature, not a minimal MVP: UX, documentation, safety checks, rollback behavior, and test coverage should be complete enough for real developer handoff.
* Product-grade scope still excludes remote marketplace installation, dependency installation, plugin signing/trust store, and automatic plugin execution because those are separate product capabilities rather than polish for template download/upload.

## Open Questions

* None for the current product-grade scope.

## Requirements

* Replace old tool devkit download content with a plugin-first template package.
* Keep the existing download endpoint stable or provide a clearly named replacement for the Web UI.
* Remove the floating bottom-right download button from the page.
* Add a `+` action below the category list in the left sidebar.
* Clicking `+` opens a modal dialog for plugin management.
* The modal provides two actions: download plugin template and upload plugin ZIP.
* Add a Web UI action for downloading a plugin template inside the modal.
* Add a Web UI action for uploading a plugin package inside the modal.
* Uploaded plugin packages must be validated before becoming active.
* Upload should install exactly one plugin package per ZIP in the product-grade upload flow, with clear error messaging when a ZIP contains no plugin or multiple plugin packages.
* Catalog should refresh after a successful plugin upload.
* If uploaded plugin ID already exists, the UI should prompt that the plugin already exists and ask whether to update.
* Update flow should compare versions and only allow replacement when the uploaded plugin version is higher than the installed plugin version.
* Same-version or lower-version uploads for an existing plugin should be rejected with a readable message after the user chooses to update.
* Legacy `tools/` / `workflows/` / root `opsctl.exe` examples must not appear in the current template as active instructions.
* Downloaded plugin template must be usable as a plugin-developer handoff package, not as platform user documentation.
* Template documentation must describe only the plugin package itself: directory structure, `plugin.yaml` fields, tool/workflow contribution rules, parameter passing, confirmation/safety conventions, local validation expectations, packaging expectations, and common mistakes.
* Template documentation must not mention the platform page, Web UI, catalog refresh, upload endpoint, platform internals, or platform source code; plugin developers should not need to know the host platform product exists beyond the plugin contract.
* Template documentation should be concise and focused on plugin authoring only: no Go/React/framework internals, no generic engineering standards, and no broad AI prompt-engineering guide.
* Template demo plugin must be a规范插件样板, not a toy hello-world sample: it should be safe to copy as a starting point for real plugin development.
* Demo plugin must include a complete `plugin.yaml`, meaningful parameters, robust script behavior, workflow example, confirmation example, README handoff notes, and examples that demonstrate expected plugin authoring practices.
* Template package should include a complete plugin authoring guide, example parameters, sample script, and plugin README template so a user can modify it into a usable plugin ZIP.
* Template package should document both tool-only plugins and workflow-capable plugins, including when to add `contributes.workflows`, how workflow YAML references plugin tools, and how high-risk workflow confirmation is handled.
* Template package should include plugin versioning/upload guidance: uploaded updates for an existing plugin must use a higher version.
* Template package should include plugin-focused security and operations guidance: no bundled secrets, no path escape assumptions, timeout selection, log hygiene, and not executing during upload.
* Template package should include troubleshooting guidance for common validation/upload/run failures with clear remediation steps.

## Research References

* [`research/current-devkit-surface.md`](research/current-devkit-surface.md) — Current download is `/api/dev/toolkit.zip`, generated in `internal/server/server.go`, and still emits legacy `tools/demo/sample-tool` entries.
* [`research/plugin-upload-safety.md`](research/plugin-upload-safety.md) — Recommended product-grade baseline is safe ZIP staging, reject unsafe entries/symlinks/duplicates, validate with plugin package rules, install atomically, reload registry, refresh catalog.

## Research Notes

### Constraints from current repo

* Download template content is currently embedded in `internal/server/server.go` and tested in `internal/server/server_test.go`.
* Current `internal/server/server.go` already has `toolDevKitReadme`, `toolDevKitSpec`, `samplePluginYAML`, `sampleRunScript`, `samplePluginReadme`, and examples; the next refinement should expand this content into a true developer/AI handoff package rather than a thin sample.
* Frontend currently has only a direct anchor download link; no `FormData`, file input, or upload helper exists.
* Server handlers currently close over one loaded `*registry.Registry`; plugin upload needs a reload/swap strategy before `/api/catalog` can show newly installed plugins.
* `internal/plugin.loadPackage` is unexported; upload implementation needs either an exported package loader helper or a small manifest parse path before calling `plugin.ValidatePackage`.

### Feasible approaches here

**Approach A: Sidebar plugin management modal with version-aware update** (Selected)

* How it works: replace devkit ZIP with plugin template ZIP, remove the bottom-right floating download link, add a `+` button below the sidebar category list, and open a modal with download-template and upload-plugin actions. Upload extracts to staging, validates, detects duplicate plugin IDs, prompts for update when a duplicate exists, only allows replacement when uploaded version is higher than installed version, then installs/replaces safely and refreshes catalog.
* Pros: matches requested UI, keeps plugin actions close to the category/plugin navigation, supports update workflow without silent overwrite.
* Cons: requires version parsing/comparison, update confirmation, backup/rollback, and registry reload/swap behavior.

**Approach B: Sidebar modal with reject-on-duplicate**

* How it works: same UI as Approach A, but duplicate plugin IDs are rejected.
* Pros: safer and simpler.
* Cons: does not match the requested update behavior.

**Approach C: Template download only first**

* How it works: only fix the old devkit ZIP and UI label now; defer upload.
* Pros: very low risk and quick.
* Cons: does not satisfy the requested upload plugin workflow.

## Acceptance Criteria (evolving)

* [ ] `/api/dev/toolkit.zip` no longer contains `tools/demo/sample-tool` or legacy root runtime instructions.
* [ ] Downloaded template ZIP contains `plugins/<plugin-id>/plugin.yaml`, scripts, examples, and README content aligned with the plugin contract.
* [ ] Downloaded template ZIP includes plugin-developer-facing authoring guidance that explains how to create, validate, package, and hand off a plugin.
* [ ] Template guidance does not mention the platform page, Web UI, catalog refresh, upload endpoint, backend API, or platform internals.
* [ ] Demo plugin is a规范插件样板 that can be copied to start a real plugin, not only a toy greeting sample.
* [ ] Demo plugin includes complete manifest metadata, category/tool/workflow contributions, realistic parameters, confirm usage, robust script handling, examples, and README handoff notes.
* [ ] Template guidance includes a plugin author checklist that can be handed to研发 or AI for implementation.
* [ ] Template package documents workflow-capable plugins, plugin versioning/packaging rules, plugin security/operations rules, troubleshooting, and compatibility constraints.
* [ ] Template package contains no placeholder-only critical section: every required plugin authoring topic has actionable guidance or an explicit field for the plugin author to fill.
* [ ] Web UI removes the bottom-right floating `下载工具开发包` action.
* [ ] Web UI shows a `+` action below the left sidebar category list.
* [ ] Clicking `+` opens a plugin management modal.
* [ ] The modal exposes plugin template download and plugin ZIP upload actions.
* [ ] Uploading a new plugin validates and installs it, then refreshes catalog.
* [ ] Uploading an existing plugin prompts `插件已存在，是否更新？` or equivalent before replacement.
* [ ] Existing-plugin update only proceeds when uploaded version is higher than installed version.
* [ ] Existing-plugin update rejects same-version or lower-version uploads with a readable message.
* [ ] Backend rejects invalid plugin uploads with a readable error.
* [ ] Backend prevents path traversal/unsafe ZIP entries during upload extraction.
* [ ] Backend prevents unvalidated plugin content from being partially activated.
* [ ] After successful upload, `/api/catalog` includes the new plugin contributions.
* [ ] Existing checks pass: `GOTOOLCHAIN=local go test ./...`, `npm run build --prefix web`, `GOTOOLCHAIN=local go build -o "bin/opsctl.exe" "./cmd/opsctl"`, `./bin/opsctl.exe validate`.

## Definition of Done (team quality bar)

* Tests added/updated for backend ZIP contents, documentation completeness, and upload behavior.
* Web UI builds successfully.
* Backend validation and path-safety rules are reused, not duplicated ad hoc.
* Docs/specs updated if public behavior changes.
* Rollback behavior is considered for failed uploads.

## Out of Scope (explicit)

* Remote plugin marketplace or URL-based plugin installation.
* Plugin signing/trust store.
* Dependency installation for uploaded plugins.
* Running uploaded plugin tools automatically.
* Multi-user authorization model.

## Technical Notes

* Existing backend endpoint: `GET /api/dev/toolkit.zip`.
* Existing frontend link: `web/src/main.jsx` floating download link.
* Existing old ZIP generator: `internal/server/server.go` `buildToolDevKitZip()`.
* Existing plugin contract: `plugins/<plugin-id>/plugin.yaml` with `contributes.categories`, `contributes.tools`, and optional workflows.
* Existing plugin validation: `internal/plugin.Load`, `ValidatePackage`, `SafePath`.
* Relevant spec: `.trellis/spec/backend/quality-guidelines.md` forbids documenting or verifying against legacy runtime paths.

## Decision (ADR-lite)

**Context**: Plugin template download and upload need clear UI placement and duplicate-plugin behavior before implementation, because upload/update semantics affect rollback complexity and user safety.

**Decision**: Use a left-sidebar `+` action below categories to open a plugin management modal. Remove the bottom-right floating download button. The modal contains plugin template download and plugin ZIP upload. For duplicate plugin IDs, prompt the user that the plugin already exists and ask whether to update. If the user confirms update, only allow replacement when the uploaded plugin version is higher than the installed version; reject same-version or lower-version uploads.

**Consequences**: The UI keeps plugin management close to plugin/category navigation and avoids the old floating download affordance. Backend implementation must support safe replacement with backup/rollback, version comparison, duplicate detection, registry reload, and clear errors when update is not allowed. Development iterations that reuse the same version must bump the plugin version before upload. The downloaded template must be treated as a plugin authoring package for this platform: enough for users to develop and package plugins, but intentionally excluding generic coding standards and framework-internal documentation.

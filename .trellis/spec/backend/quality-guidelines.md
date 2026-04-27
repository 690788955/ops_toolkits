# Quality Guidelines

> Code quality standards for the ops automation framework.

---

## Required Checks

Run these checks before handing off work:

```bash
GOTOOLCHAIN=local go test ./...
npm run build --prefix web
GOTOOLCHAIN=local go build -o "bin/opsctl.exe" "./cmd/opsctl"
./bin/opsctl.exe validate
```

If the Web UI changes, `npm run build --prefix web` must be run so `internal/server/web/` contains the latest embedded assets.

---

## Required Patterns

- CLI, menu, HTTP API, and Web UI must all execute tools/workflows through `internal/runner`.
- Parameter precedence is defaults < parameter file < CLI/API/Web overrides.
- Required parameter validation belongs in `internal/config` and must be reused across entrypoints.
- Runtime tools must be plugin-oriented: `plugins/<plugin-id>/plugin.yaml` plus plugin-owned implementation files.
- Plugin upload ZIPs must contain exactly one plugin package, discovered by scanning for `plugin.yaml`; reject archives with zero or multiple plugin manifests.
- Plugin ZIP extraction must accept normal directory entries such as `vendor.backup/` while still rejecting traversal, absolute paths, symlinks, and special files.
- Plugin template/dev-kit documentation must focus on plugin authoring only: plugin structure, manifest fields, script parameters, workflows, confirmation, validation, packaging, upload, and troubleshooting.
- Plugin template demo content must be a copyable standard plugin sample, not a toy hello-world: include complete manifest metadata, normal and high-risk tool examples, workflow contribution, robust script patterns, confirm metadata, README handoff notes, and examples.
- Plugin `command` and `workdir` paths must be validated to stay inside the plugin directory.
- High-risk tools must use `confirm.required`; runner must reject unconfirmed workflow tool nodes as a safety backstop.
- Workflows stop on first failed step in MVP.
- Run records must be written under `runs/logs/<run_id>/` with `result.json`, `stdout.log`, and `stderr.log`.

---

## Forbidden Patterns

- Do not add a new execution path that calls Shell directly outside `internal/runner`.
- Do not add new runtime tools under legacy root `tools/` or workflows under root `workflows/`.
- Do not restore root `ops.yaml` or root `opsctl.exe`; use `configs/ops.yaml` and `bin/opsctl.exe`.
- Do not document or verify commands against legacy runtime paths such as `tools/demo/hello`, `tools/demo/sample-tool`, or `workflows/demo-hello`.
- Do not include generic Go/React/framework-source coding standards in plugin template/dev-kit documentation; those packages are for plugin authors, not platform contributors.
- Do not expose host product internals in plugin template/dev-kit content: avoid Web UI/page/catalog/API/backend/frontend/source-code wording in ZIP documentation intended for plugin developers.
- Do not hard-code tool IDs or workflow IDs in Go code except demo/sample data.
- Do not require a database for MVP run history.
- Do not require a separate frontend server in production; Web assets must be embedded into the Go binary.
- Do not log secrets intentionally. Parameter values are currently recorded for operator diagnostics, so secret-like parameters should not be used until masking is added.

---

## Regression Prevention Checks

### Plugin Runtime Migration

After any change that touches runtime execution, validation commands, dev-kit generation, scaffolds, or docs, search for stale pre-plugin runtime references before handoff:

```bash
tools/demo/hello
tools/demo/sample-tool
workflows/demo-hello
root opsctl.exe
./opsctl.exe
```

If any result remains, verify it is explicitly describing legacy behavior. Current examples and manual checks must use `./bin/opsctl.exe`, `configs/ops.yaml`, and plugin IDs such as `plugin.demo.greet` or `plugin.demo.confirmed`.

### Windows `.sh` Execution Failures

When a `.sh` command fails on Windows, classify the issue before changing framework code:

- If the error mentions `WSL getpwnam(<user>) failed`, check WSL default-user health first.
- If `/bin/bash` reports a malformed `C:Users...` path, determine whether `bash` resolved to WSL or Git Bash and whether path conversion is required.
- If the failing path contains legacy `tools/` or `workflows/` segments, look for stale commands, old binaries, old configs, scaffold output, or documentation before blaming the runner.

Manual reproduction must use plugin-first commands:

```bash
./bin/opsctl.exe validate
./bin/opsctl.exe list
./bin/opsctl.exe run tool plugin.demo.greet --set name=Tester --set message=Hello --no-prompt
./bin/opsctl.exe run tool plugin.demo.confirmed --set target=demo --no-prompt
```

For more detail, read `../guides/cross-platform-runtime-thinking-guide.md`.

---

## Testing Requirements

- Pure config/parameter functions need unit tests in `internal/config`.
- Plugin parsing and path-safety logic need unit tests in `internal/plugin`.
- Plugin template/dev-kit tests must assert both positive standard-sample content (complete manifest, confirm example, workflow, robust script snippets, README/examples) and negative host-internal wording (Web UI/page/catalog/API/backend/frontend/source-code/product terms).
- Plugin upload tests must cover at least: new plugin install, duplicate prompt, higher-version replace, same/lower-version rejection, traversal rejection, explicit directory entries, invalid ZIP, unsafe plugin ID, and multi-plugin ZIP rejection.
- Registry plugin normalization needs tests in `internal/registry`.
- Any change to parameter merge or validation should include regression tests.
- Any change to API routes should be manually verified with `opsctl serve --port <port>` and `/api/catalog`.
- Any change to Web UI should build successfully and be verified through the embedded server.

---

## Manual Verification Examples

```bash
./bin/opsctl.exe list
./bin/opsctl.exe run tool plugin.demo.greet --set name=Tester --set message=Hello --no-prompt
printf '确认\n' | ./bin/opsctl.exe run tool plugin.demo.confirmed --set target=demo --no-prompt
./bin/opsctl.exe serve --port 8080
```

Browser URL:

```text
http://127.0.0.1:8080/
```

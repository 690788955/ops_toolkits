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
- Plugin `command` and `workdir` paths must be validated to stay inside the plugin directory.
- High-risk tools must use `confirm.required`; runner must reject unconfirmed workflow tool nodes as a safety backstop.
- Workflows stop on first failed step in MVP.
- Run records must be written under `runs/logs/<run_id>/` with `result.json`, `stdout.log`, and `stderr.log`.

---

## Forbidden Patterns

- Do not add a new execution path that calls Shell directly outside `internal/runner`.
- Do not add new runtime tools under legacy root `tools/` or workflows under root `workflows/`.
- Do not restore root `ops.yaml` or root `opsctl.exe`; use `configs/ops.yaml` and `bin/opsctl.exe`.
- Do not hard-code tool IDs or workflow IDs in Go code except demo/sample data.
- Do not require a database for MVP run history.
- Do not require a separate frontend server in production; Web assets must be embedded into the Go binary.
- Do not log secrets intentionally. Parameter values are currently recorded for operator diagnostics, so secret-like parameters should not be used until masking is added.

---

## Testing Requirements

- Pure config/parameter functions need unit tests in `internal/config`.
- Plugin parsing and path-safety logic need unit tests in `internal/plugin`.
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

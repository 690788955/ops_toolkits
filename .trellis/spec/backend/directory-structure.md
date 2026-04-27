# Directory Structure

> Backend and ops-tool framework layout for this project.

---

## Overview

This project is a Go-based ops automation framework. The compiled binary is `opsctl`; concrete operations are developed as plugin packages and wired through `plugins/<plugin-id>/plugin.yaml`.

---

## Directory Layout

```text
bin/
  opsctl.exe                      # local build output, ignored by git
cmd/opsctl/
  main.go                         # binary entrypoint
configs/
  ops.yaml                        # main runtime config
internal/
  app/                            # cobra commands and CLI option handling
  config/                         # configs/ops.yaml, normalized tool/workflow schemas, parameter merge logic
  plugin/                         # plugin.yaml parsing, validation, safe path checks
  registry/                       # loads plugin-contributed tools/workflows into executable registries
  runner/                         # Shell execution, parameter file generation, run records
  menu/                           # numbered interactive console
  server/                         # HTTP API and embedded Web UI
  scaffold/                       # template generation helpers
  packagebuild/                   # opsctl package build
plugins/<plugin-id>/
  plugin.yaml                     # plugin metadata and contributions
  scripts/                        # plugin-owned executable scripts
  README.md                       # plugin maintenance notes
web/
  src/                            # React source, built by Vite
internal/server/web/
  index.html, assets/             # Vite build output embedded into Go binary
runs/logs/<run_id>/
  params.yaml                     # final merged parameters when param file mode is enabled
  stdout.log
  stderr.log
  result.json
```

Legacy root `ops.yaml`, `tools/`, `workflows/`, and root `opsctl.exe` are intentionally not part of the latest runtime layout.

---

## Configuration Contracts

### Main `configs/ops.yaml`

Defines display, runtime paths, server defaults, and plugin loading.

```yaml
app:
  name: Shell 运维框架
  version: 0.1.0
paths:
  tools: []
  workflows: []
  runs: runs
  logs: runs/logs
plugins:
  paths:
    - plugins
  strict: false
  disabled: []
```

`paths.tools` and `paths.workflows` stay empty in the latest structure. Tools and workflows are contributed by plugins.

### Plugin package `plugin.yaml`

Defines one plugin package and its contributions.

```yaml
id: vendor.backup
name: 备份工具集
version: 1.0.0
contributes:
  categories:
    - id: backup
      name: 备份恢复
  tools:
    - id: vendor.backup.full
      name: 全量备份
      category: backup
      command: scripts/backup.sh
      args:
        - --target
        - '{{ .target }}'
      workdir: .
      timeout: 30m
      parameters:
        - name: target
          required: true
      confirm:
        required: true
        message: 确认执行全量备份？
```

Plugin rules:

- Tool IDs must start with `<plugin-id>.`.
- `command` and `workdir` must stay inside the plugin directory.
- `strict: false` skips bad plugins with warnings; `strict: true` fails validation/startup.
- `disabled` accepts plugin IDs or plugin directory names.

### Workflow contributions

A plugin may contribute workflow YAML files via:

```yaml
contributes:
  workflows:
    - path: workflows/full-backup.yaml
```

The workflow file is still normalized into `WorkflowConfig`; nodes reference registered tool IDs.

---

## Command Entry Points

```bash
./bin/opsctl.exe list
./bin/opsctl.exe start
./bin/opsctl.exe menu
./bin/opsctl.exe serve --port 8080
./bin/opsctl.exe run tool <tool-id> --set key=value --no-prompt
./bin/opsctl.exe run workflow <workflow-id> --params params.yaml
./bin/opsctl.exe package build
```

---

## Module Organization

- Keep command parsing in `internal/app`.
- Keep YAML structures and parameter merge/validation in `internal/config`.
- Keep plugin parsing and plugin-local path validation in `internal/plugin`.
- Keep registration and normalization in `internal/registry`.
- Keep execution logic in `internal/runner`; CLI, menu, HTTP, and Web must reuse it.
- Keep HTTP transport and embedded UI serving in `internal/server`.
- Do not duplicate parameter parsing in UI-specific or command-specific code.

---

## Naming Conventions

- Plugin IDs use dot notation and should be globally stable, e.g. `vendor.backup`.
- Plugin tool IDs must be prefixed by plugin ID, e.g. `vendor.backup.full`.
- Plugin package paths use `plugins/<plugin-id>/`.
- Run directories are generated under `runs/logs/<run_id>/`.

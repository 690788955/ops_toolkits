# Directory Structure

> Backend and ops-tool framework layout for this project.

---

## Overview

This project is a Go-based ops automation framework. The compiled binary is `opsctl`; concrete operations are developed as Shell tool packages and wired through YAML configuration.

---

## Directory Layout

```text
cmd/opsctl/
  main.go                         # binary entrypoint
internal/
  app/                            # cobra commands and CLI option handling
  config/                         # ops.yaml, tool.yaml, workflow YAML schemas and parameter merge logic
  registry/                       # loads tools/workflows into executable registries
  runner/                         # Shell execution, parameter file generation, run records
  menu/                           # numbered interactive console
  server/                         # HTTP API and embedded Web UI
  scaffold/                       # opsctl new tool/workflow templates
  packagebuild/                   # opsctl package build
tools/<category>/<tool>/
  tool.yaml                       # tool metadata, parameters, entrypoint, pass mode
  bin/run.sh                      # executable Shell entrypoint
  lib/                            # optional helper scripts
  conf/                           # optional default configs
  examples/                       # optional sample parameter files
workflows/
  <workflow-id>.yaml              # ordered workflow steps
web/
  src/                            # React source, built by Vite
internal/server/web/
  index.html, assets/             # Vite build output embedded into Go binary
runs/logs/<run_id>/
  params.yaml                     # final merged parameters when pass_mode.param_file is enabled
  stdout.log
  stderr.log
  result.json
```

---

## Configuration Contracts

### Root `ops.yaml`

Defines display and entry wiring only: categories, tool references, workflow references, and HTTP defaults.

Required fields for tool references:

```yaml
tools:
  - id: demo-hello
    category: demo
    path: tools/demo/hello
    name: Hello Demo
    description: Print a greeting
```

Required fields for workflow references:

```yaml
workflows:
  - id: demo-hello
    category: demo
    path: workflows/demo-hello.yaml
```

### Tool package `tool.yaml`

Defines how one Shell tool runs.

```yaml
id: demo-hello
name: Hello Demo
entry: bin/run.sh
parameters:
  - name: name
    description: Name to greet
    required: true
    default: World
pass_mode:
  env: true
  args: true
  param_file: true
  file_name: params.yaml
timeout: 1m
confirmation:
  required: false
```

Parameter delivery contract:

- `env: true` sets `OPS_PARAM_<UPPER_NAME>` environment variables.
- `args: true` appends `--key value` arguments.
- `param_file: true` writes the merged parameter YAML into the run directory and exposes it as `OPS_PARAM_FILE` plus `--params-file <path>` when args are enabled.

### Workflow `workflows/*.yaml`

Defines ordered steps. MVP executes steps sequentially and stops on the first failure.

```yaml
id: demo-hello
parameters:
  - name: name
    required: true
steps:
  - id: hello
    tool: demo-hello
    params:
      name: ${name}
```

---

## Command Entry Points

```bash
opsctl list
opsctl start
opsctl menu
opsctl serve --port 8080
opsctl serve --addr 0.0.0.0:8080
opsctl run tool <tool-id> --set key=value --no-prompt
opsctl run workflow <workflow-id> --params params.yaml
opsctl new tool <category>/<tool>
opsctl new workflow <workflow-id>
opsctl package build
```

---

## Module Organization

- Keep command parsing in `internal/app`.
- Keep YAML structures and parameter merge/validation in `internal/config`.
- Keep execution logic in `internal/runner`; CLI, menu, HTTP, and Web must reuse it.
- Keep HTTP transport and embedded UI serving in `internal/server`.
- Do not duplicate parameter parsing in UI-specific or command-specific code.

---

## Naming Conventions

- Tool IDs use kebab case and should remain stable because CLI/API/workflows reference them.
- Tool package paths use `tools/<category>/<tool>/`.
- Workflow files use `workflows/<workflow-id>.yaml`.
- Run directories are generated under `runs/logs/<run_id>/`.

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Shell 运维框架** - A YAML-driven ops automation framework that unifies external tool plugins, parameter definitions, workflow DAGs, and Web UI. The project provides the `opsctl` CLI for plugin discovery, config validation, tool/workflow execution, interactive menus, HTTP console, and offline package generation.

## Build & Development Commands

### Build
```bash
GOTOOLCHAIN=local go build -o bin/opsctl.exe ./cmd/opsctl
```

### Testing
```bash
GOTOOLCHAIN=local go test ./...
./bin/opsctl.exe validate
./bin/opsctl.exe list
```

### Demo Plugin Tools
```bash
./bin/opsctl.exe run tool plugin.demo.greet --set name=Tester --set message=Hello --no-prompt
./bin/opsctl.exe run tool plugin.demo.confirmed --set target=demo --no-prompt
```

The confirmed demo prompts for confirmation; accepted answers are `yes`, `确认`, `是`, or `继续`.

### Web UI Development
```bash
npm install --prefix web
npm run dev --prefix web      # Dev server at http://127.0.0.1:5173
npm run build --prefix web    # Build embedded assets into internal/server/web
```

## Common Operations

```bash
./bin/opsctl.exe help-auto
./bin/opsctl.exe help-auto tool <id>
./bin/opsctl.exe help-auto workflow <id>
./bin/opsctl.exe start
./bin/opsctl.exe serve --port 8080
./bin/opsctl.exe package build
```

## Current Runtime Structure

This repo is plugin-first. Do not add new runtime tools under legacy root `tools/` or workflows under root `workflows/`.

```text
bin/                        Local build output, ignored by git
configs/ops.yaml            Main runtime config
plugins/<plugin-id>/        External tool plugin packages
runs/                       Runtime logs, ignored by git
dist/                       Package output, ignored by git
```

The root `ops.yaml`, root `opsctl.exe`, root `tools/`, and root `workflows/` are intentionally not part of the latest structure.

## Architecture

### Core Components

**CLI Layer** (`cmd/opsctl/`)
- Entry point using Cobra command framework

**Application Layer** (`internal/app/`)
- Command definitions and help generation
- Orchestrates config, registry, runner, server
- Performs CLI confirmation prompts before high-risk runs

**Configuration** (`internal/config/`)
- YAML parsing for `configs/ops.yaml`, plugin-normalized tool configs, workflow definitions, and parameter handling
- Parameter merging from CLI flags, YAML files, defaults, and interactive prompts

**Plugin Loader** (`internal/plugin/`)
- Parses `plugins/*/plugin.yaml`
- Validates plugin IDs, tool IDs, command paths, workflow paths, disabled plugins, and strict/lenient loading behavior
- Enforces plugin-local `command` / `workdir` paths to prevent path escape

**Registry** (`internal/registry/`)
- Loads built-in references only when explicitly configured, then loads plugin contributions
- Normalizes plugin tools into existing `config.ToolConfig`
- Registers plugin categories, tools, workflows, and source metadata

**Runner** (`internal/runner/`)
- Tool execution: spawns scripts with parameters via env vars and declared args
- Workflow execution: DAG-based node scheduling with dependency resolution
- Rejects workflows that contain unconfirmed high-risk tools unless the entrypoint confirms them

**Server** (`internal/server/`)
- HTTP API for tool/workflow execution
- Embedded static assets for Web UI from `internal/server/web`
- Catalog includes plugin source and confirm metadata

**Package Build** (`internal/packagebuild/`)
- Creates distributable packages with `configs/`, `plugins/`, and the current `opsctl` executable

## Plugin Package Contract

Each plugin lives in `plugins/<plugin-id>/` and contains a `plugin.yaml` manifest plus implementation files.

Example:
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
          type: string
          required: true
      confirm:
        required: true
        message: 确认执行全量备份？
```

Rules:
- Tool IDs must be prefixed by plugin ID plus dot, e.g. `vendor.backup.full`.
- `command` and `workdir` must stay inside the plugin directory.
- `plugins.strict: false` skips bad plugins with warnings; `true` fails validation/startup.
- `plugins.disabled` accepts either plugin ID or plugin directory name.

## Key Patterns

### Parameter Resolution
Parameters are merged in order:
1. Tool/workflow defaults
2. YAML param file (`--params`)
3. CLI/API/Web overrides (`--set key=value` or request body)
4. Interactive prompts (unless `--no-prompt`)

### Tool Execution
1. Registry resolves tool ID to normalized metadata
2. Entrypoint enforces `confirm` when required
3. Runner validates parameters
4. Runner spawns plugin script under plugin directory
5. Output is captured to `runs/logs/<run_id>/`

### Workflow Execution
1. Parse DAG from `nodes` and `edges`
2. Topological sort for execution order
3. Entrypoint confirms workflow and any high-risk tool nodes
4. Runner rejects unconfirmed high-risk tool nodes as a safety backstop
5. Execute nodes respecting dependencies

## Dependencies

- **Go 1.21+**
- **Cobra** (`github.com/spf13/cobra`): CLI framework
- **yaml.v3** (`gopkg.in/yaml.v3`): YAML parsing
- **React + Vite**: Web UI (in `web/`)
- **@xyflow/react**: Workflow DAG visualization

## Notes

- User-facing text is Simplified Chinese.
- Web UI is embedded into the binary at build time; run `npm run build --prefix web` after Web changes.
- After modifying code files, run graphify rebuild:
  `python3 -c "from graphify.watch import _rebuild_code; from pathlib import Path; _rebuild_code(Path('.'))"`

## graphify

This project has a graphify knowledge graph at graphify-out/.

Rules:
- Before answering architecture or codebase questions, read graphify-out/GRAPH_REPORT.md for god nodes and community structure
- If graphify-out/wiki/index.md exists, navigate it instead of reading raw files
- After modifying code files in this session, run `python3 -c "from graphify.watch import _rebuild_code; from pathlib import Path; _rebuild_code(Path('.'))"` to keep the graph current

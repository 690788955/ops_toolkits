# brainstorm: 运维工具自动化框架

## Goal

构建一个可维护的运维工具自动化开发框架，让用户可以按统一规范接入自己的运维工具和工作流。框架应通过 YAML 配置声明菜单、工具、参数、步骤和工作流，把 HBase WAL 复制、数据迁移、环境搭建、Elasticsearch 数据迁移等脚本或工具统一接入一个入口，支持本机 CLI、HTTP API、菜单式执行、内置工作流和自定义步骤执行。

## What I already know

* 用户已确认框架使用 Go 实现，具体运维工具主要使用 Shell 开发。
* 核心目标是提供一个可复用的开发框架，让用户按规范接入多类运维脚本/工具，而不是为单一任务写一次性脚本。
* 工具场景包含 HBase WAL 复制、HBase/其他数据迁移、集群/组件搭建、Elasticsearch 数据迁移。
* 用户希望有单一入口，既可以通过界面/选项菜单操作，也可以执行内置工作流。
* 用户希望菜单、工具、参数和工作流主要通过 YAML 配置声明。
* 用户倾向使用配置分层：根 `ops.yaml` 管理菜单分类和入口编排，每个工具包使用自己的 `tool.yaml` 管理工具细节，独立 `workflows/*.yaml` 管理复杂工作流步骤。
* 用户确认框架实现语言使用 Go。
* 用户接受使用成熟 Go 第三方库，因为交付形态是编译后的包，运行端不直接受开发依赖影响。
* 用户希望主要用 Shell 开发具体运维工具，框架负责分类、菜单、执行和工作流编排。
* 用户开发的工具可能是一个规范化目录，而不只是单个脚本；工具目录内可能包含多个 Shell 文件、配置、模板、依赖脚本和说明。
* 用户希望按运维对象分类组织，例如 HBase 分类下选择 HBase 相关操作或工作流，Elasticsearch 分类下选择 ES 相关操作或工作流。
* 用户确认工作流采用 DAG 有向无环图模型：节点是工具调用，边是执行依赖。
* 用户强调整个项目目录结构要提前设计好，框架代码和工具库必须分离；工具库面向其他维护者扩展和维护。
* 用户希望 MVP 同时支持 CLI、HTTP API 和简单编号菜单；菜单不需要复杂 TUI，优先稳定易实现。
* 用户希望参数既可以来自文件，也可以来自命令行/API 指定，并能根据参数定义自动生成交互式输入流程。
* 用户希望重新设计工具和工作流的关系：工具是最小可执行单元，工作流是对多个工具调用的组合编排。
* 用户希望工作流步骤优先引用已注册工具，而不是直接写任意脚本命令；每个步骤配置工具 ID、参数映射、执行条件和失败策略。
* 用户希望工作流既能按预设步骤执行，也能让用户自定义选择步骤。
* 用户确认需要完整页面能力，用于可视化拖拽编辑、修改、保存和管理工作流定义。
* 用户希望接入的工具也能基于配置自动生成 help/usage 信息，而不是每个工具手写独立 help 命令。
* 用户希望其他机器可以调用主机上的工具，最好支持 `curl` 方式触发。
* 当前仓库主要是 Trellis 工作区骨架，暂未发现业务代码或既有运维脚本目录。

## Assumptions (temporary)

* 该框架优先面向内部运维人员使用，运行环境大概率是 Linux 服务器或跳板机。
* 现有工具可能混合存在 Shell 脚本、二进制工具、Java/HBase/ES 命令行调用。
* MVP 应优先解决“统一入口、任务编排、参数收集、执行日志、失败可定位”，而不是一开始做完整 Web 平台。

## Open Questions

* None pending; waiting for final confirmation before implementation planning.

## Requirements (evolving)

* 提供统一入口执行运维工具。
* 提供根 `ops.yaml` 配置规范，用于声明分类菜单、工具包引用和工作流入口引用。
* 提供独立 `workflows/*.yaml` 配置规范，用于声明由工具调用组成的 DAG 工作流，包括节点、边、工具引用、步骤依赖、可选步骤、参数映射、超时和失败策略。
* 提供工具包 `tool.yaml` 配置规范，用于声明工具参数、入口脚本、命令适配器、传参方式、校验规则和说明。
* 提供清晰目录规范，将框架源代码、运行配置、工具库、工作流定义、运行日志和发布产物分离。
* 提供工具包目录规范，让一个工具可以包含入口脚本、子脚本、配置、模板、示例参数和说明文件。
* 框架启动时加载 `ops.yaml` 并生成 CLI、简单编号菜单和 HTTP API 可执行项。
* 支持按运维对象分类展示菜单，例如 HBase、Elasticsearch、Data Migration、Environment Setup。
* 支持分类下同时包含单个工具操作和组合工作流。
* 支持参数定义自动生成交互式输入，参数值可以来自用户输入、命令行参数、HTTP API 请求或参数文件。
* 支持基于工具包 `tool.yaml`、工作流 YAML 和根 `ops.yaml` 自动生成 help/usage 信息。
* 支持为工具、工作流、分类菜单和参数生成一致的帮助说明，包括用途、参数、默认值、是否必填、示例调用和配置来源。
* 支持将最终参数以命令行参数、环境变量或参数文件形式传递给 Shell 工具包入口。
* 支持简单编号菜单选择分类、工具或工作流。
* 支持 CLI 方式执行工具或工作流。
* 支持 HTTP API 方式执行工具或工作流。
* 支持页面方式查看工具列表、查看工作流列表、创建工作流、编辑工作流、保存工作流和触发工作流执行。
* 支持在页面工作流编辑器中将工具拖入画布形成 DAG 节点，并通过连线配置执行依赖、参数映射和失败策略。
* 支持内置 DAG 工作流按依赖顺序执行。
* 支持工作流步骤引用已注册工具，并复用工具自身的参数定义、校验规则、help 信息和执行适配器。
* 支持用户自定义选择步骤执行。
* 工作流步骤失败时，MVP 默认立即停止后续步骤执行。
* 支持接入 HBase、Elasticsearch 等运维脚本/工具。
* 支持以 HTTP API 方式从其他机器远程触发主机上的工具或工作流。
* HTTP API 应支持 `curl` 调用，返回任务 ID、状态和错误信息。
* MVP 使用本地文件目录保存运行记录、步骤日志、stdout/stderr 和最终结果。
* MVP 提供模板生成命令，用于创建标准工具包目录、`tool.yaml` 和工作流 YAML 模板。
* MVP 提供打包发布命令，用于生成包含二进制、配置、工具包和工作流的可交付目录或压缩包。
* MVP 内置示例工程，包括 demo Shell 工具包和示例工作流，用于验证 CLI、菜单、HTTP API、参数解析、日志和打包链路。

## Acceptance Criteria (evolving)

* [ ] 用户可以通过根 `ops.yaml` 新增分类、菜单项和工作流入口。
* [ ] 用户可以通过 `workflows/*.yaml` 定义由已注册工具组成的工作流步骤、步骤依赖和参数映射。
* [ ] 工作流步骤必须能引用工具 ID，并复用该工具的参数定义、校验规则和执行方式。
* [ ] 用户可以通过工具包 `tool.yaml` 定义一个 Shell 工具包的参数、入口和执行方式。
* [ ] 框架启动后可以从 `ops.yaml` 生成按分类展示的工具/工作流列表。
* [ ] 用户可以按规范创建一个包含多个文件的 Shell 工具包，并通过框架执行入口脚本。
* [ ] 框架可以根据参数定义生成交互式输入流程。
* [ ] 用户可以通过 `opsctl help`、`opsctl help <tool-or-workflow>` 或 `<command> --help` 查看自动生成的帮助信息。
* [ ] 自动生成的 help 至少包含描述、参数列表、必填/可选、默认值、示例调用和对应配置文件位置。
* [ ] 同一工具参数可以从交互输入、CLI/API 指定值或参数文件中解析。
* [ ] 用户可以在页面查看已注册工具，并把工具添加为工作流步骤。
* [ ] 用户可以在页面通过可视化方式调整工作流步骤顺序和依赖关系。
* [ ] 用户可以在页面配置步骤参数映射、必填参数和失败策略，并保存为工作流定义。
* [ ] 页面保存后的工作流可以被 CLI、菜单和 HTTP API 正常加载和执行。
* [ ] 用户可以选择一个内置工作流并按步骤执行。
* [ ] 用户可以跳过或手动选择工作流中的部分步骤。
* [ ] 工作流任一步骤失败时，默认立即停止后续步骤。
* [ ] 每个步骤有明确的参数输入、执行状态和失败信息。
* [ ] 新增一个工具/步骤不需要修改大量框架核心代码。
* [ ] 其他机器可以通过 `curl` 调用主机 API 启动一个工具或工作流。
* [ ] API 返回可查询的执行 ID，并可通过状态接口查看执行进度和结果。
* [ ] 每次工具或工作流执行都会在本地 runs/logs 目录生成独立运行记录。
* [ ] 用户可以通过命令生成标准 Shell 工具包模板，例如 `opsctl new tool hbase/wal-copy`。
* [ ] 模板生成的 `tool.yaml` 和工作流 YAML 包含 help 自动生成所需的 `name`、`description`、`usage`、`examples` 和参数说明字段。
* [ ] 用户可以通过命令生成标准工作流模板，例如 `opsctl new workflow hbase-migrate`。
* [ ] 用户可以通过命令生成可发布包，例如 `opsctl package build`。
* [ ] 框架内置 demo 工具和 demo 工作流，可以完整跑通 CLI、菜单、HTTP API 和日志记录。

## Definition of Done (team quality bar)

* Tests added/updated where appropriate.
* Lint / typecheck / build checks pass where applicable.
* Usage docs or operator notes updated if behavior changes.
* Failure handling, logs, rollback or retry boundaries considered for risky operations.

## Out of Scope (explicit)

* 暂不默认实现权限系统、审批流、多租户、审计平台等企业级能力。
* 页面工作流编辑器聚焦工作流编排，不扩展为完整运维平台或复杂审批系统。
* 页面工作流编辑器 MVP 不要求实现高级协同编辑、版本分支对比、复杂权限和审计平台能力。
* MVP 暂不实现远程 API 认证；仅假设部署在可信内网，后续可增加 Token 或 IP 白名单。
* MVP 暂不实现复杂失败策略，例如自动重试、失败后继续执行、回滚编排；默认失败即停止。
* 暂不重写所有现有运维脚本；优先包装和编排已有工具。

## Research Notes

### What similar tools do

* Internal ops frameworks commonly provide a CLI entrypoint first, because CLI commands are easy to document, audit, copy, automate, and run over SSH.
* Menu/TUI interaction is useful for guided operations, but should not be the only interface for risky production tasks.
* Reliable workflow tools usually separate precheck, parameter validation, execution, logging, failure summary, and retry/resume guidance.
* Existing Shell scripts are often wrapped instead of rewritten immediately, especially for HBase, Elasticsearch, and migration operations where operators already trust known scripts.

### Feasible approaches here

#### Workflow/orchestrator reuse options

**React Flow + Go execution engine** (Recommended for this framework)

* React Flow 提供前端拖拽画布、节点、连线和 DAG 编辑能力，许可证友好且适合内嵌二次开发。
* Go 后端继续负责工具注册、参数校验、工作流保存、执行调度、日志和 CLI/HTTP/API 复用。
* 优点是轻量、可控、能保持 `opsctl` 单一产品形态；缺点是执行引擎仍需自研。

**Node-RED as workflow layer**

* Node-RED 是成熟可视化流程编排工具，可把每个运维工具封装成自定义节点。
* 优点是编辑器和运行时现成；缺点是引入 Node.js 运行时，产品形态受 Node-RED 影响较大。

**Rundeck / Kestra / Argo / Temporal / Airflow 类平台**

* Rundeck 更贴近内网运维作业平台，权限、日志、节点执行较成熟，但拖拽编排不是核心优势，整体偏重。
* Kestra、Argo、Temporal、Airflow 更适合云原生、数据平台或强后端工作流场景，对轻量 Shell 运维框架偏重。

Conclusion: 如果目标是 Go `opsctl` + Shell 原子工具 + 页面拖拽 + CLI/HTTP/API 统一执行，优先采用 React Flow 作为页面编排器，Go 自研统一执行引擎；如果用户更看重“完全现成运行时”，再评估 Node-RED 或 Rundeck。

**Approach A: Hybrid Go orchestrator + Shell adapters** (Recommended)

* How it works: Go provides `opsctl` entrypoint, workflow registry, parameter validation, logs, step execution, dry-run, and failure summaries; existing scripts live under adapters and follow a stable input/output/exit-code contract.
* Pros: balances long-term maintainability with reuse of existing Shell tools; supports CLI and later TUI/Web; centralizes logs and errors; reduces rewrite risk.
* Cons: requires defining adapter conventions; maintains both Go and Shell layers.

**Approach B: Go CLI first, optional TUI later**

* How it works: implement most framework logic natively in Go with subcommands such as `opsctl hbase wal-replicate`, `opsctl es migrate`, and `opsctl workflow run`.
* Pros: best long-term structure, type safety, testability, single binary deployment, clean help docs and completion.
* Cons: higher initial cost; existing Shell scripts need wrapping or rewriting.

**Approach C: Pure Shell menu transitional MVP**

* How it works: one `ops.sh` menu calls task scripts and shared helpers for logging/precheck.
* Pros: fastest to start, easiest for existing ops users, minimal dependencies.
* Cons: long-term maintainability, parameter validation, logging, and error handling degrade as tasks grow.

## Technical Approach

Decision: use Approach A, a hybrid Go orchestrator with Shell tool packages, and use React Flow as the visual workflow editor. Mature Go libraries are acceptable for CLI, YAML parsing, and HTTP server support because the delivery artifact is a compiled package. Concrete tools are developed in Shell, while Go loads YAML configuration to provide CLI execution, simple numbered category menus, workflow orchestration, execution records, a lightweight HTTP server mode so other machines can trigger host-side tools with `curl`, and a React Flow based page editor for visual workflow creation and modification. Keep execution logic in Go so CLI, menu, HTTP API, and page-created workflows share the same runtime.

Core abstractions to validate:

* YAML-driven registry: root `ops.yaml` defines menus/categories/tool entries/workflow entries; `workflows/*.yaml` defines workflow steps and orchestration; each package `tool.yaml` defines parameters, adapters, execution mode, timeouts, and confirmation requirements.
* `ops.yaml` is the global entry configuration. Proposed minimal schema:

```yaml
app:
  name: opsctl
  description: Internal operations automation toolkit.
  version: 1.0.0

paths:
  tools:
    - tools
  workflows:
    - workflows
  runs: runs
  logs: runs/logs

server:
  enabled: true
  host: 0.0.0.0
  port: 8080

menu:
  categories:
    - id: hbase
      name: HBase
      description: HBase operation tools and workflows.
    - id: elasticsearch
      name: Elasticsearch
      description: Elasticsearch migration and maintenance tools.
    - id: migration
      name: Data Migration
      description: General data migration workflows.

registry:
  include_tools:
    - hbase.*
    - elasticsearch.*
  include_workflows:
    - hbase.*
    - migration.*

ui:
  enabled: true
  title: Ops Workflow Console
```

* Tool package layout: each tool may have `tool.yaml`, `bin/`, `lib/`, `conf/`, `templates/`, `examples/`, and `README`-style usage notes.
* Scaffolding commands: `opsctl new tool <category>/<tool>` creates a standard tool package; `opsctl new workflow <workflow-id>` creates a workflow YAML template.
* Packaging command: `opsctl package build` creates a deliverable directory or archive containing `opsctl`, `ops.yaml`, `tools/`, `workflows/`, and `configs/`.
* Demo project: include a simple `tools/demo/hello` tool package and a demo workflow to validate the framework before real HBase/ES tools are added.
* Parameter resolver: merge defaults, parameter files, CLI/API values, and interactive prompts into one final parameter object.
* Tool/task registry: loaded from YAML with name, description, parameters, adapter or native executor.
* `tool.yaml` should be the stable contract for external tool maintainers. Proposed minimal schema:

```yaml
id: hbase.wal-copy
name: HBase WAL Copy
description: Copy HBase WAL files between clusters.
version: 1.0.0
category: hbase
tags: [hbase, wal, migration]

help:
  usage: opsctl run hbase.wal-copy --source <path> --target <path>
  examples:
    - opsctl run hbase.wal-copy --source /hbase/WALs --target /backup/WALs

parameters:
  - name: source
    type: string
    required: true
    description: Source WAL path.
  - name: target
    type: string
    required: true
    description: Target WAL path.
  - name: dry_run
    type: bool
    required: false
    default: false
    description: Print planned actions without copying files.

execution:
  type: shell
  entry: bin/run.sh
  args:
    - --source={{ .source }}
    - --target={{ .target }}
    - --dry-run={{ .dry_run }}
  timeout: 30m
  workdir: .

confirm:
  required: true
  message: This operation may copy large WAL files. Continue?
```

* Workflow definition: loaded from YAML with DAG nodes/edges, optional steps, precheck steps, timeout, confirmation requirement, and references to registered tool IDs. Proposed minimal schema:

```yaml
id: hbase.migrate-with-wal
name: HBase Migration With WAL Copy
description: Run precheck, copy WAL files, and verify migration result.
version: 1.0.0
category: hbase

parameters:
  - name: source_cluster
    type: string
    required: true
    description: Source HBase cluster name.
  - name: target_cluster
    type: string
    required: true
    description: Target HBase cluster name.

nodes:
  - id: precheck
    name: Precheck source cluster
    tool: hbase.precheck
    params:
      cluster: "{{ .source_cluster }}"
    on_failure: stop

  - id: wal_copy
    name: Copy WAL files
    tool: hbase.wal-copy
    params:
      source: "/hbase/{{ .source_cluster }}/WALs"
      target: "/hbase/{{ .target_cluster }}/WALs"
      dry_run: false
    on_failure: stop

  - id: verify
    name: Verify target cluster
    tool: hbase.verify
    params:
      cluster: "{{ .target_cluster }}"
    on_failure: stop

edges:
  - from: precheck
    to: wal_copy
  - from: wal_copy
    to: verify

confirm:
  required: true
  message: This workflow will run HBase migration steps. Continue?
```

* Workflow editor model: represent workflow definitions as structured graph data that can round-trip between YAML, HTTP API, and visual page editor without losing fields.
* Visual workflow editor: use React Flow to support dragging tools into a workflow canvas, connecting ordered/dependent steps, configuring parameter mappings, editing failure policy, saving back to workflow definitions, and validating before execution.
* Adapter contract: arguments/config input, stdout/stderr handling, exit code mapping, and result metadata.
* Execution record: task id, operator, host, start/end time, step statuses, logs, final result.
* HTTP API mode: submit workflow/tool execution, query status, fetch logs or summary, cancel if safe.
* MVP remote API security: internal network only, no authentication in the first version; keep the API boundary simple enough to add token authentication and IP allowlist later.

## Decision (ADR-lite)

**Context**: The framework must let the user develop structured Shell-based ops tools and workflows, expose them through local and remote entrypoints, and keep delivery simple for Linux machines.

**Decision**: Build a Go-based `opsctl` framework with YAML-driven menus, Shell tool packages, workflow definitions, CLI execution, simple numbered menu, HTTP API, local file run records, scaffolding commands, packaging command, demo project, and a React Flow based visual workflow editor.

**Consequences**: This keeps concrete ops tools easy to write in Shell while moving orchestration, validation, logging, remote triggering, and packaging into Go. MVP intentionally excludes Web UI, authentication, database persistence, automatic retry, and rollback orchestration.

## Implementation Plan

* Phase 1: scaffold repository layout, Go module, `cmd/opsctl`, `internal/`, `web/`, `configs/`, `tools/`, `workflows/`, `examples/`, `runs/`, and `dist/` conventions.
* Phase 2: implement YAML config models and loaders for `ops.yaml`, `tool.yaml`, and `workflow.yaml`, including validation and registry construction.
* Phase 3: implement Shell tool execution, parameter resolution, generated help, local run records, stdout/stderr logging, and confirmation handling.
* Phase 4: implement DAG workflow validation and execution, including node dependency ordering, failure handling, and step-level logs.
* Phase 5: implement CLI commands, simple numbered menu, and HTTP API for listing, running, and querying tools/workflows.
* Phase 6: implement React + React Flow workflow editor for listing tools, composing DAG nodes/edges, editing params, saving workflows, and triggering execution.
* Phase 7: add demo tools, demo workflows, package build command, and end-to-end verification.

## Technical Notes

* Repo inspected: current project appears to contain Trellis workflow/config files only, no business implementation yet.
* Relevant constraint: framework should keep tool integration simple and avoid over-engineering early.
* Recommended repository/development structure:
  * `cmd/opsctl/`: Go CLI entrypoint.
  * `internal/`: framework core implementation, including config loading, registry, executor, workflow DAG engine, API server, logging, packaging, and scaffolding.
  * `web/`: React + React Flow workflow editor source code.
  * `configs/`: root `ops.yaml` and environment-level configuration examples.
  * `tools/`: externally maintained tool packages, separated from framework source code.
  * `workflows/`: workflow YAML definitions that compose registered tools.
  * `examples/`: demo tools, demo workflows, and sample parameter files.
  * `runs/` or `logs/`: local execution records; generated at runtime and excluded from source control.
  * `dist/`: packaging output; generated and excluded from source control.
* Recommended tool package structure for external maintainers:
  * `tool.yaml`: tool metadata, parameters, help text, execution adapter, timeout, confirmation requirement.
  * `bin/`: executable Shell entry scripts.
  * `lib/`: reusable Shell helper scripts.
  * `conf/`: tool-specific default configuration.
  * `templates/`: generated config or command templates.
  * `examples/`: parameter files and usage examples.
  * `README.md`: human-facing maintenance notes.
* Recommended deployment shape for MVP: one Go binary plus embedded or packaged web assets, `tools/`, `workflows/`, `configs/`, and `runs/` or `logs/` directories.
* MVP persistence: local filesystem only; SQLite or external database is out of scope for the first version.

# brainstorm: 外部工具菜单接入

## Goal

让其他用户开发的 tool 能以较低接入成本接入本框架：外部工具可以保留自己的开发格式，本项目侧只需要把它们接到菜单/入口中即可发现、展示和执行。

## What I already know

* 用户希望支持“其他用户开发的 tool”接入。
* 外部工具可能有自己的开发格式，不希望强制迁移为当前 `tools/<category>/<name>/tool.yaml + bin/run.sh` 格式。
* 本项目当前通过 `ops.yaml` 配置工具目录、工作流目录、菜单分类等。
* 当前 CLI 已支持 `opsctl list`、`opsctl validate`、`opsctl run tool`、`opsctl serve`、交互菜单和 Web UI。
* 当前架构已经是 `manifest + registry + runner + menu/server` 模式，适合增加适配层。
* 目前 `registry` 只会扫描 `tool.yaml` 并解析成本项目的 `ToolConfig`；不支持直接读取外部自有格式。
* 菜单和 Web Catalog 都从 registry 读取工具列表，因此只要把外部工具转换/注册为 `ToolConfig`，现有菜单和 API 基本可复用。

## Assumptions (temporary)

* MVP 目标不是提供完整插件市场，而是允许本地/外部目录中的工具通过适配层进入现有菜单和执行链路。
* 需要尽量复用现有 registry、runner、menu/server 机制，避免重写一套执行框架。
* 外部工具的“开发格式”差异可能主要体现在目录结构、参数定义、执行入口、帮助文档格式上。

## Open Questions

* 无。用户已确认按当前 MVP 范围实现。

## Requirements (evolving)

* 支持外部工具以低成本出现在现有菜单中。
* 尽量不要求外部工具改造成当前内置工具目录格式。
* 采用 plugin/collection 方向：外部用户按插件包规范交付目录和 `plugin.yaml`，框架读取 YAML 后快速接入菜单、Web catalog、执行入口和工作流引用。
* MVP 优先支持本地插件目录扫描、插件 YAML 校验、工具贡献、工作流贡献、菜单分类贡献、ID 冲突检查、基础路径安全检查。
* 插件工具执行声明采用更友好的 `command + args + workdir + timeout` 模型，而不是要求插件作者直接理解现有 `execution/pass_mode` 格式；框架内部负责转换为现有 `ToolConfig.Execution`。
* MVP 安全边界采用推荐版：校验 `plugin.yaml`、ID 冲突、命令文件存在、路径不能逃逸插件目录；支持插件启用/禁用；支持高风险工具 `confirm` 确认提示。
* 插件加载失败策略可配置，默认宽松：失败插件被跳过并输出告警，其他插件和内置工具继续可用；配置 strict 时加载失败会阻断 validate/启动。

## Acceptance Criteria (evolving)

* [x] 能说明当前架构是否支持扩展以及需要改哪些模块。
* [x] 给出 2–3 个可行接入方案及取舍。
* [x] 明确 MVP 范围和不做的内容。
* [x] 插件作者可以通过 `plugin.yaml` 的 `command/args/workdir/timeout` 声明工具执行方式。
* [x] 框架可以把插件工具声明转换为内部 `ToolConfig` 并复用现有菜单、Web catalog、API 和 runner。
* [x] 插件工具的命令路径和工作目录不能逃逸插件目录。
* [x] 可通过配置启用/禁用插件。
* [x] 高风险插件工具能通过 `confirm` 要求执行前确认。
* [x] 插件加载失败默认跳过并告警；配置 strict 后失败会阻断加载/校验。

## Definition of Done (team quality bar)

* Tests added/updated (unit/integration where appropriate)
* Lint / typecheck / CI green
* Docs/notes updated if behavior changes
* Rollout/rollback considered if risky

## Out of Scope (explicit)

* 暂不默认实现远程插件仓库、插件自动下载/升级、权限沙箱。
* 暂不默认支持所有第三方工具格式的自动推断。

## Research Notes

### What similar tools do

* GitHub Actions uses per-action `action.yml` manifests to declare inputs and execution (`runs`), so runners can discover and execute actions without knowing their internals.
* Ansible modules/collections use stable namespaces and metadata/docs around modules; the framework consumes a normalized interface rather than arbitrary code layout.
* Backstage catalog/plugins separate registration descriptors from implementation; catalog descriptors are the integration contract for UI/service discovery.
* VS Code extensions use `package.json` contribution points (`commands`, menus, activation events) so the host can add UI entries declaratively.

### Constraints from our repo/project

* `internal/registry/registry.go` currently discovers only files named `tool.yaml` under configured `paths.tools`.
* `internal/config/types.go` defines `ToolEntry` and `ToolConfig`; registry/menu/server already consume these normalized structs.
* `internal/runner/runner.go` executes `tool.Config.Execution.Entry` under `tool.Dir` and passes params through env/args/param_file.
* `internal/menu/menu.go` and `internal/server/server.go` do not care about physical tool layout once registry has a normalized tool.
* `ops.yaml` has `paths.tools`, root-level `tools`, and `menu.categories`, which can support explicit external registrations.

### Feasible approaches here

**Approach A: Sidecar adapter manifest** (Recommended MVP)

* How it works: external tools keep their own files. This project adds a small adapter YAML (for example `external_tools/*.yaml` or root `tools:` entries) that maps menu metadata, parameters, working directory, command, args/env mapping into normalized `ToolConfig`.
* Pros: simplest; no need to parse arbitrary external formats; reuses registry/runner/menu/server; easy to test.
* Cons: every external tool still needs one adapter manifest maintained by this project.

**Approach B: Import/convert command**

* How it works: add commands such as `opsctl import tool <path> --type github-action|custom` to generate local `tool.yaml` adapters from known external manifests.
* Pros: friendly onboarding; generated adapters are explicit and reviewable.
* Cons: more code and format-specific mapping rules; still not truly zero-config.

**Approach C: Plugin/collection package spec**

* How it works: define first-class plugin packages with their own manifest, contribution points, metadata, permissions, and possibly multiple tools/workflows.
* Pros: strongest long-term ecosystem path; supports many tools per package and richer UI.
* Cons: larger design surface; more validation/security/versioning work; likely overkill for MVP.

#### Expanded Approach C: Plugin/collection system

A plugin/collection is a distributable directory package owned by an external tool author. It contains one top-level manifest (for example `plugin.yaml`) that declares package metadata, contributed menu entries, tools, workflows, permissions, assets, and compatibility constraints. The framework loads plugin manifests, validates them, normalizes each contributed tool/workflow into existing registry models, then menu/Web/API consume them through the same registry path as built-in tools.

Example package shape:

```text
plugins/vendor.backup/
  plugin.yaml
  tools/
    backup/run.sh
    restore/run.py
  workflows/
    full-backup.yaml
  assets/
    icon.svg
  README.md
```

Example manifest concept:

```yaml
id: vendor.backup
name: Vendor Backup Tools
version: 1.2.0
compatibility:
  opsctl: ">=0.2.0"
contributes:
  categories:
    - id: backup
      name: 备份恢复
  tools:
    - id: vendor.backup.full
      name: 全量备份
      category: backup
      command: tools/backup/run.sh
      parameters:
        - name: target
          type: string
          required: true
      permissions:
        filesystem:
          read: ["/data"]
          write: ["/backup"]
        network: false
  workflows:
    - path: workflows/full-backup.yaml
```

Key implementation idea: add a plugin loader before/inside registry loading. The loader reads configured plugin directories, validates package manifests, expands contributed tools/workflows into `ToolConfig` / `WorkflowConfig`, assigns source metadata such as `source=plugin`, `plugin_id`, `plugin_version`, then registers them in the existing registry.

Important capabilities unlocked:

* One package can contribute multiple menu categories, tools, and workflows.
* External authors can keep package-local implementation files while exposing a stable framework contract.
* Future UI can show package name/version/author/icon, not just individual tools.
* Compatibility and permissions can be declared centrally before execution.
* Package-level validation can catch ID conflicts, missing files, incompatible versions, unsupported permissions, and unsafe paths.

Likely phases:

1. Minimal plugin discovery: `plugins/*/plugin.yaml` -> registry tools/workflows.
2. Validation and conflict handling: duplicate IDs, missing commands, invalid categories, path traversal.
3. UX polish: list plugins, show plugin source in Web UI/menu, package-level enable/disable.
4. Security/operations: permissions, confirm policies, trust model, signed packages or checksums.
5. Distribution: import/install/update/export commands.

## Technical Approach

Implemented a first-class plugin/collection loader while preserving the existing execution architecture:

* `internal/plugin` parses and validates `plugins/*/plugin.yaml` packages.
* `RootConfig.Plugins` configures plugin paths, `strict`, and `disabled` plugins.
* `registry.Load` loads built-in tools first, then converts plugin-contributed tools/workflows into existing `ToolConfig` / `WorkflowConfig` models.
* Plugin tools use friendly `command/args/workdir/timeout` fields, normalized internally to `ExecutionConfig`.
* Menu, CLI, HTTP API, Web catalog, and workflow runner continue to execute through `internal/runner`.
* Confirm handling is enforced at CLI, menu, HTTP API, and runner boundaries; workflows containing unconfirmed high-risk tools are rejected unless confirmed by the entrypoint.
* Package builder now includes `plugins/` in distributable output.

## Decision (ADR-lite)

**Context**: External users need to develop tools in their own directory format but still expose them in this framework's menu/API/workflow system.

**Decision**: Implement a local plugin/collection package spec with `plugin.yaml` as the integration contract. Use default-lenient plugin loading (`strict: false`) so bad plugins are skipped with warnings, while CI/release can opt into strict mode.

**Consequences**: External authors get a stable YAML contract and the framework reuses existing runner/menu/server code. The first version intentionally does not provide remote installation, signing, permission sandboxing, or automatic parsing of arbitrary third-party formats.

## Technical Notes

* Trellis task created with explicit assignee because no developer was initialized in this repo session.
* Graphify report/wiki were initially not present; after code changes the graph was rebuilt via graphify watch.
* Verification run: `GOTOOLCHAIN=local go test ./...`, `npm run build --prefix web`, `GOTOOLCHAIN=local go build -o "bin/opsctl.exe" "./cmd/opsctl"`.

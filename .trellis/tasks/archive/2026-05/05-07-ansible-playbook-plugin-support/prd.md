# brainstorm: ansible playbook plugin support

## Goal

探索并定义插件是否可以支持以 Ansible playbook 作为实现载体，让运维同学能够把已有 playbook 按 ops_toolkits 的插件规范打包、校验、执行和分发。

## What I already know

* 用户想确认“插件能不能支持 ansible 的 playbook 来制作”。
* 当前项目是 plugin-first 运维框架，插件通过 `plugins/<plugin-id>/plugin.yaml` 声明工具、参数、命令和工作目录。
* 既有长期目标是让运维同学能通过模板和 `plugin.yaml` 自助打包脚本，同时不限制脚本内部逻辑。
* 现有插件工具已经支持 `command`、模板化 `args`、`workdir`、`parameters`、`config_dir`、`config_files`、`confirm`，足以把 playbook 包装成普通插件工具执行。
* Runner 会执行插件声明的 `command` 和 `args`；用户希望 `command` 不仅能指向插件内脚本，也能调用运行环境 PATH 中的命令，例如 `java` 或 `ansible-playbook`。

## Assumptions (temporary)

* Ansible playbook 支持应尽量作为插件工具的一种实现方式，而不是引入一套完全独立的运行体系。
* MVP 优先依赖运行环境已有 `ansible-playbook` 命令，而不是内置 Ansible 运行时。
* playbook、inventory、vars、ansible.cfg 默认应放在插件目录内，保持插件包可分发、可校验、可审计。

## Open Questions

* 无。

## Requirements (evolving)

* 插件作者可以把 Ansible playbook 放进插件目录并通过 opsctl 执行。
* 插件边界仍需保持安全：命令、工作目录、配置路径不能逃逸插件目录或明确的配置目录边界。
* 框架配置应支持声明允许的运行时命令，内部管理员可以把 `java`、`ansible-playbook` 或其他内部命令加入允许列表，插件作者即可用裸 `command` 调用。
* `plugin.yaml` 中的 `config_dir` 支持相对插件目录解析，允许 `config`、`../../xxx` 这类内部共享配置目录；也支持当前平台可识别的绝对路径作为配置基准目录。
* `config_files` 仍必须是相对项，并且不能逃逸最终解析出的 `config_dir`。
* `command` 支持两类安全形式：带路径命令必须位于插件目录内；无路径命令名（如 `java`、`ansible-playbook`）可在命中框架允许策略后通过运行环境 PATH 执行。
* `command` 不支持 shell 字符串拼接执行；参数必须继续通过 `args` 数组声明，避免命令注入。
* 常见 Ansible 入参应能通过 opsctl 参数映射：`inventory`、`limit`、`tags`、`skip_tags`、`extra_vars`、`check`、`diff`、verbosity 等。
* 可编辑的 inventory、vars、ansible.cfg 可以放在 `config/` 并通过 `config_dir` / `config_files` 暴露给 Web/API；内部共享配置目录或当前平台可识别的绝对 `config_dir` 也可作为配置基准，但 `config_files` 仍必须是相对项并留在最终基准目录内；若需要白名单、只读/可写权限或稳定文件 ID 管控，则通过 host-side mapping 暴露。
* SSH key、vault password、宿主绝对路径 inventory 等敏感或宿主侧资源不应默认随插件包交付，需通过运行时参数、环境、或管理员白名单映射处理。

## Acceptance Criteria (evolving)

* [x] 明确 Ansible playbook 插件的推荐目录结构和 `plugin.yaml` 写法。
* [x] 明确 CLI/API/Web 传参如何映射到 `ansible-playbook` 的 extra vars 或参数文件。
* [x] 明确不新增 manifest 字段，复用现有 `command`/`args`/wrapper 能力。
* [x] 明确 Ansible/Python/collections/roles 等运行时依赖不由 `opsctl package build` 自动安装。
* [x] 明确高风险 playbook 应配置 `confirm.required: true`，并建议提供 `--check` / `--diff` dry-run 参数。
* [x] 支持 `config_dir: config`、`config_dir: ../../xxx` 这类相对插件目录配置基准，也支持当前平台可识别的绝对 `config_dir`。
* [x] 支持 `command: java` / `command: ansible-playbook` 这类 PATH 命令，同时继续拒绝带路径命令逃逸插件目录。
* [x] 支持在框架配置中声明允许的 PATH 命令，便于内部扩展其他运行时命令。

## Definition of Done (team quality bar)

* Tests added/updated (unit/integration where appropriate)
* Lint / typecheck / CI green
* Docs/notes updated if behavior changes
* Rollout/rollback considered if risky

## Research References

* [`research/ansible-plugin-patterns.md`](research/ansible-plugin-patterns.md) — 可比系统普遍把 playbook 作为项目/工作区内受约束执行单元，本仓库最贴合插件本地 wrapper script + `command/args/parameters/config_files` 的映射方式。

## Technical Approach

### Approach A: 文档化现有能力（推荐 MVP）

* How: 在插件模板/README 中新增 Ansible playbook 插件目录结构、`plugin.yaml` 示例和 wrapper script 示例；不新增 runtime 字段。
* Pros: 代码改动小，贴合“不限制脚本逻辑”的插件目标，风险低，现有校验/打包/执行链路全部可复用。
* Cons: 框架不会原生理解 playbook/inventory/extra-vars，只是提供规范模板。

### Approach B: 增加一个 Ansible 示例插件

* How: 在 `plugins/` 或模板示例中提供可运行的 `plugin.ansible-example`，包含 playbook、inventory、vars、wrapper。
* Pros: 运维同学复制成本最低，能用 `opsctl validate/list/run` 直接验证。
* Cons: 会增加示例维护成本，并要求测试环境对 Ansible 缺失有清晰处理。

### Approach C: 新增原生 `ansible_playbook` manifest 字段

* How: 在插件 manifest 中增加类似 `ansible:` 的结构化字段，由框架生成 `ansible-playbook` 命令。
* Pros: UI/API 可以更原生地展示 playbook、inventory、check/diff 等字段。
* Cons: 改动涉及 schema、loader、registry、runner、测试和文档；也会约束插件作者的 Ansible 用法，暂不适合作为第一步。

## Decision (ADR-lite)

**Context**: Ansible playbook 可以作为插件实现载体，但需要在插件边界、宿主路径、凭据和运行时依赖之间保持清晰分工。
**Decision**: 采用内部简化方案：复用现有插件能力；`command` 裸命令通过 `plugins.allowed_commands` 配置允许；`config_dir` 支持相对插件目录解析（包括 `../../xxx`）和当前平台可识别的绝对路径。
**Consequences**: 内部插件接入更直接，适合共享配置目录、绝对配置基准目录和系统运行时命令；仍保留 `config_files` 不逃逸最终 `config_dir`、`args` 数组传参等基础防误操作约束。

## Out of Scope (explicit)

* 暂不内置安装 Ansible、Python、collections 或 roles。
* 暂不实现完整 Ansible Tower/AWX 能力。
* 暂不把任意宿主绝对路径 playbook/inventory 作为插件包默认能力。
* 暂不提供凭据库或 SSH/vault 密钥托管。

## Technical Notes

* 现有 `plugins/plugin.template/plugin.yaml` 展示了 `command`、`args`、`workdir`、`parameters`、`config_dir`、`config_files` 的插件工具写法。
* `internal/runner/runner.go` 中工具执行会解析插件内入口、设置插件内工作目录、渲染参数、注入 `OPS_PARAM_*` 环境变量。
* `config_files` 只声明最终 `config_dir` 下可编辑配置文件，不会自动传参、复制或生成文件；wrapper script 需要显式读取或转发这些路径。
* `opsctl package build` 打包 `configs/`、`plugins/` 和当前可执行文件，不打包系统级 Ansible 依赖。

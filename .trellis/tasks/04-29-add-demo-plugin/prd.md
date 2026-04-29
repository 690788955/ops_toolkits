# brainstorm: 新增 demo 插件

## Goal

新增一个独立的 demo 插件，用于演示多插件、多分类、多标签，以及与工作流编辑器中“全局/分类工具范围”的交互，方便验证和展示插件体系。

## What I already know

* 用户希望“制作多一个 demo 插件”。
* 现有 demo 插件位于 `plugins/plugin.demo/`，ID 为 `plugin.demo`。
* 现有 demo 插件提供 `plugin.demo.greet` 普通工具和 `plugin.demo.confirmed` 高风险确认工具。
* 当前仓库运行结构是 plugin-first，插件应放在 `plugins/<plugin-id>/` 并通过 `plugin.yaml` 声明 categories/tools/workflows。
* 当前工作区已有上一项 workflow persistence 功能的未提交改动，新增 demo 插件应避免修改这些改动的行为。

## Assumptions (temporary)

* 新 demo 插件应是独立插件目录，不是在现有 `plugin.demo` 里追加工具。
* 新插件应用于展示多插件/多分类，而不是连接真实外部系统。
* 所有脚本应只输出日志，不修改系统状态。

## Open Questions

* 无。

## Requirements (evolving)

* 新增独立巡检 demo 插件，推荐目录 `plugins/plugin.inspect/`，插件 ID `plugin.inspect`。
* 插件包含巡检分类，例如 `plugin-inspect`，用于与现有 `plugin-demo` 分类区分。
* 插件至少提供 `plugin.inspect.check` 工具，模拟输出目标、CPU、磁盘、服务状态等巡检结果。
* 插件必须包含 `plugin.yaml`、脚本和 README。
* 工具 ID 必须以插件 ID 为前缀。
* 脚本必须使用 `set -euo pipefail`，支持参数解析，遇到未知参数返回非 0。
* 插件应可通过 `./bin/opsctl.exe validate` 和 `./bin/opsctl.exe list` 识别。

## Acceptance Criteria (evolving)

* [ ] 新插件出现在插件目录下且结构符合现有插件约定。
* [ ] `./bin/opsctl.exe validate` 通过。
* [ ] `./bin/opsctl.exe list` 能看到新插件分类/工具。
* [ ] 至少一个新工具可通过 `./bin/opsctl.exe run tool plugin.inspect.check --set target=demo --set service=nginx --set status=OK --no-prompt` 成功运行。
* [ ] README 包含插件用途、工具说明和验证命令。

## Definition of Done (team quality bar)

* Tests added/updated if needed
* Lint / typecheck / CI green
* Docs/notes updated if behavior changes
* Rollout/rollback considered if risky

## Out of Scope (explicit)

* 不接入真实外部系统。
* 不新增真实高风险操作。
* 不改动现有 `plugin.demo` 行为，除非后续明确要求。

## Technical Notes

* Existing plugin manifest: `plugins/plugin.demo/plugin.yaml`。
* Existing scripts pattern: `plugins/plugin.demo/scripts/greet.sh` and `plugins/plugin.demo/scripts/confirmed.sh`。
* Existing README pattern: `plugins/plugin.demo/README.md`。
* Project plugin-first runtime: `configs/ops.yaml` has `plugins.paths: [plugins]`.

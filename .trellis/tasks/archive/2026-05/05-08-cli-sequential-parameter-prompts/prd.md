# CLI 参数逐个交互确认

## Goal

让命令行交互体验对齐 Web UI 的执行配置面板：用户在 CLI 菜单或交互式运行中选择工具/工作流后，不需要记住 `--set key=value`，而是对所有声明参数按顺序逐个确认；直接回车保留当前值或默认值，输入新内容则覆盖。

## What I already know

* 用户希望 Web UI 中的入参体验也能在命令行里通过交互方式输入。
* 用户明确选择更简单的方式：不要复杂参数编辑菜单，而是所有参数一个一个确认。
* 当前代码已有 `config.PromptMissing`，但只提示缺失/空值参数；已有默认值、参数文件值、`--set` 值时会跳过。
* 菜单执行路径当前会先合并默认值再 `PromptMissing`，因此有默认值的参数会直接执行，用户没有机会确认或修改。

## Requirements

* CLI 交互场景中，工具/工作流的所有声明参数都应按定义顺序逐个提示确认。
* 每个提示应展示参数名/描述、类型、必填或可选、默认值/当前值等已有元信息。
* 用户直接回车时，保留当前值；如果没有当前值但有默认值，则使用默认值；如果必填且仍为空，则报缺少必填参数。
* 用户输入非空内容时，用该输入覆盖该参数值。
* `--set` 和 `--params` 提供的值作为当前值显示，用户在交互模式下仍可回车保留或输入覆盖。
* `--no-prompt` 时不进入逐个确认，保持脚本/CI 非交互行为。
* 高风险确认仍在参数确认之后执行。
* HTTP API 和 Web UI 不做交互式提示改变。

## Acceptance Criteria

* [ ] `opsctl start`/`opsctl menu` 选择工具后，所有参数都会逐个提示确认，即使已有默认值。
* [ ] `opsctl start tool <id>` / `opsctl start workflow <id>` 交互快捷执行时，所有参数都会逐个提示确认。
* [ ] `opsctl run tool <id>` / `opsctl run workflow <id>` 在未指定 `--no-prompt` 时，对所有参数逐个确认。
* [ ] 直接回车保留当前值/默认值。
* [ ] 输入新值会覆盖当前值，并用于本次执行。
* [ ] `--no-prompt` 下不提示，缺必填参数仍报错。
* [ ] 参数提示相关单元测试覆盖默认值、`--set` 覆盖、回车保留、输入覆盖、必填缺失。

## Definition of Done

* `GOTOOLCHAIN=local go test ./...` 通过。
* `GOTOOLCHAIN=local go build -o bin/opsctl.exe ./cmd/opsctl` 通过。
* 通过至少一个 CLI 命令手动验证逐个确认体验。
* 若修改代码文件，完成 graphify rebuild。

## Technical Approach

新增或扩展参数提示函数，使其支持“确认所有参数”模式，而不是复用只补缺失值的 `PromptMissing` 语义。菜单和交互式 CLI 执行路径使用新模式；`--no-prompt` 保持跳过提示。必要时保留 `PromptMissing` 供只补缺失参数的兼容场景使用。

## Decision (ADR-lite)

**Context**: 当前 CLI 已能提示缺失参数，但有默认值的参数会被静默使用，用户在菜单中选择工具后无法像 Web UI 一样确认或修改所有入参。

**Decision**: 采用最简单的逐个确认向导：每个参数问一次，回车保留当前/默认值，输入则覆盖，不做复杂编辑菜单。

**Consequences**: 交互菜单更可控、学习成本更低；带参数较多的工具会多几次回车，但逻辑简单且符合用户预期。脚本化场景通过 `--no-prompt` 保持不受影响。

## Out of Scope

* 不实现参数编辑菜单、批量预览页或返回修改上一项。
* 不改变 Web UI 表单。
* 不改变 HTTP API 参数协议。
* 不改变工具/工作流 YAML 参数 schema。

## Technical Notes

* 相关规范：`.trellis/spec/backend/cli-interaction-patterns.md`、`.trellis/spec/backend/quality-guidelines.md`。
* 现有代码探索结论：`internal/config/params.go` 的 `PromptMissing` 只提示空值；`internal/menu/menu.go` 和 `internal/app/app.go` 调用该逻辑。

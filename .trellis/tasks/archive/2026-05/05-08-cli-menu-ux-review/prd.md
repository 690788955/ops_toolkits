# brainstorm: 命令行和菜单使用体验优化

## Goal

审查 opsctl CLI 和交互式菜单的用户体验，识别可优化的点，提升运维人员的日常使用效率。

## What I already know

### 当前 CLI 命令结构
- `opsctl list` - 列出工具和工作流
- `opsctl validate` - 校验配置
- `opsctl run tool <id>` / `opsctl run workflow <id>` - 执行
- `opsctl help-auto [tool|workflow <id>]` - 显示帮助
- `opsctl start` / `opsctl menu` - 交互式菜单（两个命令功能相同）
- `opsctl serve` - HTTP 服务
- `opsctl new tool|workflow` - 创建模板
- `opsctl package build` - 打包

### 当前菜单实现
- 两级菜单：分类选择 → 工具/工作流选择
- 数字编号选择，支持 `q` 退出、`b` 返回
- 有"全局/全部"分类显示所有项目
- 执行后按回车返回主菜单

### 当前参数提示
- 逐个参数询问，只显示 description 或 name
- 没有显示参数类型、默认值、可选值等元信息

## 发现的优化点

### P1 - 高优先级（影响日常使用效率）

| # | 问题 | 现状 | 建议 |
|---|------|------|------|
| 1 | 命令冗余 | `start` 和 `menu` 功能完全相同 | 保留一个，另一个作为别名或废弃 |
| 2 | 参数提示信息不足 | 只显示 description，不显示类型/默认值/可选值 | 提示时显示 `参数名 (类型, 默认=X): ` |
| 3 | list 输出无分组 | 所有工具平铺，无分类 | 按分类分组显示，或加 `--by-category` 选项 |
| 4 | 列表对齐问题 | 用 `\t` 分隔，长 ID 会错位 | 使用 tabwriter 或固定列宽 |

### P2 - 中优先级（提升体验）

| # | 问题 | 现状 | 建议 |
|---|------|------|------|
| 5 | 菜单无搜索 | 只能数字选择 | 支持输入关键字过滤 |
| 6 | 无快捷执行 | 必须进菜单选择 | 支持 `opsctl start <tool-id>` 直接执行 |
| 7 | help 命令混淆 | `help` 和 `help-auto` 区别不明 | 统一为一个，或明确区分用途 |
| 8 | 无 JSON 输出 | 只有文本输出 | 加 `--json` 选项便于脚本集成 |

### P3 - 低优先级（锦上添花）

| # | 问题 | 现状 | 建议 |
|---|------|------|------|
| 9 | 无最近使用 | 每次从头选择 | 记录最近执行的工具，菜单顶部显示 |
| 10 | 工作流无 DAG 图 | 文本列表显示节点 | ASCII 或简单图形化显示依赖关系 |
| 11 | 无参数预设 | 每次手动输入 | 支持保存常用参数组合 |
| 12 | 无补全支持 | 手动输入完整 ID | 提供 bash/zsh 补全脚本 |

## Open Questions

无。

## Decision (ADR-lite)

### 交互入口命名

**Context**: `opsctl start` 和 `opsctl menu` 当前功能完全相同，用户容易困惑哪个才是推荐入口。

**Decision**: 保留 `opsctl start` 作为主入口，`opsctl menu` 作为兼容别名。

**Consequences**: 新用户统一引导使用 `start`；现有依赖 `menu` 的习惯和脚本不被破坏。

### 菜单搜索入口

**Context**: 菜单当前只能数字选择，工具多时定位困难；搜索入口需要避免和数字选择、退出/返回指令冲突。

**Decision**: 采用显式 `s) 搜索` 入口。

**Consequences**: 交互最清楚、误操作低；代价是搜索前多一次选择。

## Requirements (evolving)

### P1 - 必做
- [ ] 合并 `start`/`menu` 命令冗余
- [ ] 参数提示增强：显示类型、默认值、可选值
- [ ] `list` 命令按分类分组显示
- [ ] 列表输出对齐优化

### P2 - 纳入 MVP
- [ ] 菜单搜索过滤
- [ ] 快捷执行 `opsctl start <tool-id>` / `opsctl start workflow <workflow-id>`
- [ ] help 命令整理

## Acceptance Criteria (evolving)

- [ ] `opsctl list` 输出按分类分组，列宽对齐，并保留工具/工作流区分。
- [ ] 参数交互提示展示参数名、类型、默认值、必填状态；有可选值时展示可选值。
- [ ] 交互式菜单支持关键字过滤，工具较多时能快速定位。
- [ ] 支持从交互入口快捷执行指定工具或工作流，不必逐级菜单选择。
- [ ] `help` / `help-auto` 的用户入口清晰，不再让用户困惑两个帮助系统的区别。
- [ ] 现有 `opsctl start`、`opsctl menu`、`opsctl run tool <id>`、`opsctl run workflow <id>` 的常用调用不被破坏。
- [ ] Go 测试通过，CLI golden path 手动验证通过。

## Definition of Done (team quality bar)

- Tests added/updated (unit/integration where appropriate)
- Lint / typecheck / CI green
- Docs/notes updated if behavior changes
- Rollout/rollback considered if risky

## Out of Scope (explicit)

- Web UI 的优化（另开任务）
- 插件系统本身的改动
- 新增运维工具
- JSON 输出支持（本次聚焦人机交互体验，暂不设计机器可读输出协议）

## Technical Approach

- `opsctl start` 作为推荐交互入口；`opsctl menu` 保留兼容，但 help 文案标注为别名。
- `opsctl start` 无参数时进入菜单；带参数时快捷执行：`opsctl start tool <id>` / `opsctl start workflow <id>`，并可考虑兼容 `opsctl start <id>` 自动按 ID 类型识别。
- 菜单在分类页和条目页增加 `s) 搜索`，搜索范围覆盖 ID、名称、描述；搜索结果仍用编号选择。
- `list` 使用 `text/tabwriter` 或等价方式对齐输出，并按分类组织工具/工作流。
- 参数提示统一由 `internal/config/params.go` 输出增强标签，包含参数名、描述、类型、必填、默认值、可选值。
- `help-auto` 保留兼容；默认 `help`/命令 help 文案引导用户使用统一入口，减少重复概念。

## Technical Notes

### 相关文件
- `internal/app/app.go` - CLI 命令定义
- `internal/app/help.go` - 帮助输出
- `internal/menu/menu.go` - 交互式菜单
- `internal/config/params.go` - 参数提示逻辑

### 约束
- 用户界面文本使用简体中文
- 保持向后兼容，不破坏现有脚本调用

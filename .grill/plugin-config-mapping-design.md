# Grill: 插件配置映射规则设计
Date: 2026-05-05

## Intent
用户要验证并落地“页面配置能映射成工具配置文件”的框架能力：页面不仅能维护业务 YAML，还能维护受限的配置映射规则；`plugin.demo` 作为教学/验收案例，展示配置如何生成原生配置文件并传给脚本。

## Constraints
- 不做兼容教材，只保留一种传递方式。
- 不使用 `OPS_PARAM_FILE` 作为 demo 主路径。
- 映射规则应是通用框架能力，不只服务 `plugin.demo`。
- 页面可修改映射规则，但不能允许任意宿主路径或任意文件写入。
- 模板内容本次不允许页面编辑，只能选择插件内已有模板。

## Key decisions
- Decision: demo 只展示 `config_templates` 生成原生配置文件，再通过 `--config <generated-file>` 传给脚本。 Reason: 直接对应“页面配置映射文件”的核心诉求，避免 `OPS_PARAM_FILE` 混淆。 Alternative considered: 同时展示 `OPS_PARAM_FILE` 和 `config_templates`，因用户明确不需要兼容而拒绝。
- Decision: 映射规则保存到宿主侧文件 `configs/plugins/<plugin-id>.mapping.yaml`，不修改插件目录的 `plugin.yaml`。 Reason: 避免插件升级覆盖宿主改动。 Alternative considered: 直接改插件包 manifest，被拒绝。
- Decision: 模板只能从插件目录内已有模板选择。 Reason: 防止页面变成任意宿主文件读取入口。 Alternative considered: 页面自由输入模板路径，被拒绝。
- Decision: `output` 和 `arg` 可改但受限校验。 Reason: 保留配置灵活性，同时防止 generated 目录逃逸和命令拼接。 Alternative considered: 完全固定不可改，灵活性不足。
- Decision: 映射规则按工具维护，支持一个工具多个映射文件。 Reason: `config_templates` 本身是工具级数组能力，真实工具可能需要多个配置文件。 Alternative considered: 插件级默认映射或一个工具一个文件，被拒绝。
- Decision: 宿主 mapping 对某个工具的 `config_templates` 整体替换插件默认声明；未出现的工具继续使用插件默认声明。 Reason: 避免字段级深合并导致运行契约不清。 Alternative considered: 逐字段合并，被拒绝。
- Decision: 页面使用结构化表单维护 mapping，不开放原始 mapping YAML 编辑。 Reason: 更容易限制 template/output/arg 的合法范围。 Alternative considered: 原始 YAML 编辑，被拒绝。

## Surfaced assumptions
- “配置映射文件”不是页面直接选择任意宿主文件，而是运行时在 `runs/logs/<run-id>/generated/` 下生成文件。
- 插件模板是插件交付物，页面只选择模板，不在线开发模板。
- `plugin.demo` 是教学案例，但能力必须通用到所有插件。

## Out of scope
- 不编辑模板内容。
- 不新增 HTTP 访问/真实巡检逻辑。
- 不做插件级默认 mapping。
- 不支持字段级 mapping 合并。

# 插件演示配置映射与页面可维护映射规则

## Goal

实现通用的插件配置映射规则维护能力：页面不仅能维护插件宿主业务配置 `configs/plugins/<plugin-id>.yaml`，还可以用结构化表单维护宿主侧映射规则 `configs/plugins/<plugin-id>.mapping.yaml`。运行工具时，框架按工具级映射规则把最终分层配置渲染成 `runs/logs/<run-id>/generated/<output>`，并通过受限命令行参数传给脚本。`plugin.demo` 作为验收案例展示这套能力。

## Requirements

### 通用框架能力

* 新增宿主侧插件映射规则文件：`configs/plugins/<plugin-id>.mapping.yaml`。
* 映射规则按工具 ID 维护，不做插件级默认映射。
* 一个工具支持多个映射文件，对应 `config_templates` 数组。
* 宿主 mapping 文件中出现某个工具 ID 时，该工具的 `config_templates` 整体替换插件 `plugin.yaml` 里的默认声明。
* 宿主 mapping 未出现的工具继续使用插件默认 `config_templates`。
* 页面用结构化表单维护 mapping，不开放原始 YAML 编辑。
* 页面可编辑：
  * 工具 ID（从当前插件贡献工具中选择）
  * template（只能从插件目录内已有模板中选择）
  * output（受限相对路径，只能输出到运行目录 `generated/` 下）
  * arg（受限命令行参数名，如 `--config`）
* 页面不可编辑模板内容。
* 映射规则保存后重新加载 registry，让后续运行立即生效。

### 安全边界

* 不允许 template 指向插件目录外。
* 不允许 output 逃逸 `generated/`。
* 不允许 arg 携带空格、shell 片段或值；只能是 `--xxx` 形式。
* 不允许页面直接选择或写入任意宿主文件。
* 不修改插件目录内 `plugin.yaml`。

### Demo 验收案例

* 在 `plugin.demo` 增加业务真实的配置映射演示工具，例如 `plugin.demo.config-file`。
* Demo 只保留一种方式：`config_templates` 生成原生配置文件，再通过 `--config <generated-file>` 传给脚本。
* 不展示 `OPS_PARAM_FILE`。
* Demo 配置贴近“服务连接 + 执行策略 + 通知信息”：
  * service.name
  * service.endpoint
  * service.timeout
  * policy.mode
  * policy.retry
  * policy.window
  * notification.channel
  * notification.owner
* Demo 脚本真实读取 `--config` 指向的生成配置文件，并输出读取到的内容/将执行动作；不实现新的框架能力专用 hack。
* README 说明页面维护业务配置和映射规则的入口。

## Acceptance Criteria

* [ ] 页面插件配置弹窗中能进入“映射规则”结构化编辑区域。
* [ ] 页面能列出当前插件工具和插件内可用模板。
* [ ] 页面能新增/删除某工具的多个映射项。
* [ ] 保存 mapping 后生成/更新 `configs/plugins/<plugin-id>.mapping.yaml`。
* [ ] 保存 mapping 后 registry 刷新，后续工具运行使用宿主 mapping 覆盖插件默认 `config_templates`。
* [ ] 未配置宿主 mapping 的工具继续使用插件默认 `config_templates`。
* [ ] unsafe template/output/arg 被后端拒绝且不写入文件。
* [ ] `plugin.demo.config-file` 出现在 `opsctl list`。
* [ ] 运行 `plugin.demo.config-file` 会生成 `runs/logs/<run-id>/generated/demo.conf` 并通过 `--config` 传给脚本。
* [ ] `GOTOOLCHAIN=local go test ./...`、`npm run build --prefix web`、`opsctl validate`、`opsctl list` 通过。

## Definition of Done

* 后端 API、registry 加载、runner 行为和安全校验有测试覆盖。
* 前端构建通过，页面结构化表单可用。
* `plugin.demo` README 有使用说明。
* graphify rebuild 已运行。

## Out of Scope

* 不编辑模板内容。
* 不支持原始 mapping YAML 自由编辑。
* 不做插件级默认 mapping。
* 不做字段级 mapping 合并。
* 不新增真实 HTTP 巡检/访问外部 endpoint 能力。

## Technical Approach

* 后端新增 mapping 读写 API，建议路径在现有插件配置 API 旁：
  * `GET /api/plugins/<plugin-id>/mapping`
  * `PUT /api/plugins/<plugin-id>/mapping`
* API 返回：插件工具列表、可选模板列表、当前宿主 mapping、目标路径。
* 新增 mapping 类型，例如：

```yaml
tools:
  plugin.demo.config-file:
    config_templates:
      - name: demo_conf
        template: configs/templates/demo.conf.tmpl
        output: demo.conf
        arg: --config
```

* registry 加载插件工具时，读取 `configs/plugins/<plugin-id>.mapping.yaml`，如果某工具有宿主 mapping，则整体替换 `ToolConfig.ConfigTemplates`。
* 可选模板扫描限制在插件目录下，优先 `configs/templates/**/*.tmpl`。
* 前端在插件配置弹窗内增加“业务配置 / 映射规则”切换或映射规则区块。
* Demo 插件新增：
  * `plugins/plugin.demo/configs/default.yaml`
  * `plugins/plugin.demo/configs/templates/demo.conf.tmpl`
  * `plugins/plugin.demo/scripts/config_file.sh`
  * `plugin.demo.config-file` 工具声明

## Decision Log

* 只保留 `config_templates + --config`，不展示 `OPS_PARAM_FILE`。
* 映射规则保存到宿主侧 `configs/plugins/<plugin-id>.mapping.yaml`，不改插件包 manifest。
* 映射规则通用于所有插件，`plugin.demo` 只作为示例。
* 页面使用结构化表单，不开放 mapping YAML 自由编辑。
* 宿主 mapping 按工具整体替换 `config_templates`。

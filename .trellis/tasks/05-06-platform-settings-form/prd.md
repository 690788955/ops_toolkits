# 平台设置表单化

## Goal

将平台设置从 YAML 文本编辑器改造为人性化的表单界面，提取常用配置字段为独立表单项，降低用户配置门槛，同时保留高级 YAML 编辑入口。

## What I already know

* 用户截图显示当前平台设置是纯 YAML 文本框，希望改为表单化设置
* `RootConfig` 结构包含以下主要配置：
  - `app.name`、`app.description`、`app.version`：应用基本信息
  - `server.enabled`、`server.host`、`server.port`：服务器配置
  - `plugins.paths`、`plugins.strict`、`plugins.disabled`：插件配置
  - `paths.runs`、`paths.logs`：路径配置
  - `ui.enabled`、`ui.title`：UI 配置
* 当前前端 `GlobalConfigPanel` 只有 YAML textarea
* 后端 API：`GET /api/config/global` 和 `PUT /api/config/global`

## Requirements

### 基础表单字段（MVP）

**应用信息**
- 应用名称（`app.name`）- 文本输入
- 应用描述（`app.description`）- 文本输入
- 应用版本（`app.version`）- 文本输入

**服务器配置**
- 启用服务器（`server.enabled`）- 复选框
- 监听地址（`server.host`）- 文本输入，默认 `0.0.0.0`
- 监听端口（`server.port`）- 数字输入，默认 `8080`

**插件配置**
- 插件目录（`plugins.paths`）- 文本输入（逗号分隔），默认 `plugins`
- 严格模式（`plugins.strict`）- 复选框，说明：启用后插件加载失败会中断启动

**路径配置**
- 运行目录（`paths.runs`）- 文本输入，默认 `runs`
- 日志目录（`paths.logs`）- 文本输入，默认 `runs/logs`

### 交互设计

1. 表单字段按模块分组（应用信息、服务器、插件、路径）
2. 每个字段显示标签、输入框、默认值提示
3. 底部保留"高级编辑"按钮，点击切换到 YAML 文本编辑模式
4. YAML 模式下可切换回表单模式
5. 保存时：
   - 表单模式：将表单值序列化为 YAML 后提交
   - YAML 模式：直接提交 YAML 文本

### 数据流

**加载配置**
1. 调用 `GET /api/config/global` 获取 YAML 文本
2. 解析 YAML 为 JSON 对象
3. 提取字段值填充表单

**保存配置**
1. 表单模式：收集表单值，构造配置对象，序列化为 YAML
2. YAML 模式：直接使用 textarea 内容
3. 调用 `PUT /api/config/global` 提交

## Acceptance Criteria

* [ ] 平台设置模态框默认显示表单界面
* [ ] 表单包含应用信息、服务器、插件、路径四个分组
* [ ] 每个字段有清晰标签和默认值提示
* [ ] 可切换到 YAML 高级编辑模式
* [ ] 表单保存时正确序列化为 YAML
* [ ] YAML 模式保存时直接提交文本
* [ ] 前端构建通过
* [ ] 服务器测试通过

## Definition of Done

* `npm run build --prefix web` 通过
* `GOTOOLCHAIN=local go test ./internal/server/...` 通过
* `GOTOOLCHAIN=local go build -o bin/opsctl.exe ./cmd/opsctl` 通过

## Out of Scope

* 后端 API 改造（继续使用现有 YAML 文本 API）
* 配置校验（依赖后端现有校验）
* 配置模板或预设
* 配置导入/导出
* 配置历史版本

## Technical Approach

### 前端实现

1. **新增 `PlatformSettingsForm` 组件**
   - 表单字段分组渲染
   - 状态管理：表单值、编辑模式（form/yaml）
   - YAML 解析/序列化逻辑

2. **修改 `GlobalConfigPanel`**
   - 根据 `editMode` 状态切换表单/YAML 视图
   - 表单模式：渲染 `PlatformSettingsForm`
   - YAML 模式：渲染现有 textarea

3. **YAML 处理**
   - 使用 `js-yaml` 库解析和序列化
   - 表单 → YAML：构造配置对象，调用 `yaml.dump()`
   - YAML → 表单：调用 `yaml.load()`，提取字段值

### 字段映射

```javascript
const formFields = {
  appName: 'app.name',
  appDescription: 'app.description',
  appVersion: 'app.version',
  serverEnabled: 'server.enabled',
  serverHost: 'server.host',
  serverPort: 'server.port',
  pluginsPaths: 'plugins.paths', // 数组，前端用逗号分隔字符串
  pluginsStrict: 'plugins.strict',
  pathsRuns: 'paths.runs',
  pathsLogs: 'paths.logs'
}
```

### 样式

- 复用现有 `.pluginConfigEditor` 样式
- 表单字段使用 `label` + `input`/`select`/`checkbox`
- 分组用 `<fieldset>` 或简单的标题分隔

## Technical Notes

* 主要文件：`web/src/main.jsx`
* 需要安装 `js-yaml` 依赖：`npm install js-yaml --prefix web`
* 参考现有 `PluginConfigPanel` 的表单布局

# 工具按需显示配置文件绑定

## Goal

只有在 `plugin.yaml` 中声明了需要配置文件的工具，前端才显示配置文件绑定功能。避免所有工具都显示不必要的配置选项，让 UI 更简洁、更符合实际需求。

## What I already know

* **当前配置模板机制**：
  - 工具可以在 `plugin.yaml` 中通过 `config_templates` 声明配置模板
  - 配置模板可以来自插件目录或全局配置文件库
  - 运行时框架会渲染模板并传递给工具

* **当前问题**：
  - 配置文件库是全局入口（右上角 📁 按钮）
  - 没有工具级别的配置文件绑定 UI
  - 用户无法在工具详情页看到该工具使用了哪些配置文件

* **用户需求**：
  - 工具在 `plugin.yaml` 中声明需要配置文件
  - 前端根据声明显示配置文件绑定区域
  - 只有声明了的工具才显示，其他工具不显示

## Requirements

### 1. 声明机制

**在 `plugin.yaml` 中声明配置文件需求**
- 使用现有的 `config_templates` 字段
- 如果工具定义了 `config_templates`，表示该工具需要配置文件
- 支持两种来源：
  - 插件目录内的模板（相对路径）
  - 全局配置文件库的模板（`configs/templates/` 开头）

**示例**：
```yaml
tools:
  - id: plugin.backup.full
    name: 全量备份
    config_templates:
      - name: backup_conf
        template: configs/templates/backup.conf.tmpl  # 全局配置文件库
        output: backup.conf
        arg: --config
```

### 2. 前端显示逻辑

**工具详情页**
- 检查工具是否有 `config_templates` 或绑定关系
- 如果有，显示"配置文件"区域
- 如果没有，不显示该区域

**配置文件区域内容**
- 显示已绑定的配置文件列表
- 每个配置文件显示：
  - 模板名称
  - 模板文件路径
  - 输出文件名
  - 传递方式（参数或环境变量）
- 操作按钮：
  - 编辑配置文件内容
  - 解除绑定
  - 新增绑定

### 3. API 支持

**工具详情 API 增强**
- `GET /api/tools/{id}` 返回工具的配置模板信息
- 包含 `config_templates` 字段
- 包含绑定关系（从 `mapping.yaml` 读取）

## Acceptance Criteria

* [ ] 工具详情页根据 `config_templates` 显示配置文件区域
* [ ] 没有声明配置文件的工具不显示该区域
* [ ] 可以查看工具绑定的配置文件列表
* [ ] 可以编辑配置文件内容
* [ ] 可以新增/删除配置文件绑定
* [ ] 前端构建通过
* [ ] 后端测试通过

## Definition of Done

* `npm run build --prefix web` 通过
* `GOTOOLCHAIN=local go test ./...` 通过
* `GOTOOLCHAIN=local go build -o bin/opsctl.exe ./cmd/opsctl` 通过

## Out of Scope

* 修改 `plugin.yaml` 的声明格式（使用现有的 `config_templates`）
* 配置文件的版本管理
* 配置文件的权限控制

## Technical Approach

### 后端实现

**1. 工具详情 API**
- 新增 `GET /api/tools/{id}` 端点
- 返回工具的完整信息，包括：
  - 基本信息（名称、描述、分类等）
  - 参数定义
  - 配置模板（`config_templates`）
  - 绑定关系（从 `mapping.yaml` 读取）

### 前端实现

**1. 工具详情页**
- 点击工具列表项时，显示工具详情页
- 包含：
  - 工具基本信息
  - 参数列表
  - 配置文件区域（按需显示）
  - 执行按钮

**2. 配置文件区域**
- 条件渲染：`tool.config_templates?.length > 0 || hasBindings`
- 显示配置文件列表
- 每个配置文件可以点击编辑

**3. 配置文件编辑**
- 复用现有的 `ConfigTemplateEditorModal`
- 点击配置文件打开编辑器

## Technical Notes

* 主要文件：
  - 后端：`internal/server/server.go`（新增工具详情 API）
  - 前端：`web/src/main.jsx`（工具详情页 + 配置文件区域）
* 复用现有的配置文件库 API 和组件
* 工具详情页可以作为独立页面或侧边栏

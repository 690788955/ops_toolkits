# 配置文件库与工具绑定

## Goal

建立全局配置文件库，支持在 Web UI 中管理配置模板文件，工具可以选择绑定配置文件，多个工具可以共用同一个配置文件。工作流调用工具时自动继承工具绑定的配置。

## What I already know

* **配置模板机制**：
  - 定义在 `plugin.yaml` 的 `tools[].config_templates` 中
  - 模板文件当前存储在插件目录：`plugins/{plugin-id}/configs/templates/*.tmpl`
  - 模板使用 Go template 语法，变量来自插件业务配置（`configs/plugins/{plugin-id}.yaml`）
  - 运行时框架渲染模板生成配置文件，通过 `--config` 或环境变量传给脚本

* **ConfigTemplate 结构**（`internal/config/types.go:127`）：
  ```go
  type ConfigTemplate struct {
      Name     string // 模板名称
      Template string // 模板文件路径
      Output   string // 生成的配置文件名
      Env      string // 环境变量名（可选）
      Arg      string // 命令行参数名（可选，如 --config）
  }
  ```

* **用户需求**：
  - 配置文件独立管理，不属于某个插件
  - 多个工具可以共用一个配置文件
  - 工作流调用工具时自动继承工具配置

## Requirements

### 1. 配置文件库（全局管理）

**配置文件存储**
- 配置文件统一存储在 `configs/templates/` 目录
- 文件命名：`{name}.tmpl`（如 `database.conf.tmpl`、`api.conf.tmpl`）

**配置文件库页面**
- 在"配置"标签页新增"配置文件库"子标签
- 列出所有配置文件：
  - 文件名
  - 被使用次数（被多少个工具绑定）
  - 最后修改时间
- 操作：
  - 新增配置文件
  - 编辑配置文件内容
  - 删除配置文件（需检查是否被使用）

### 2. 工具绑定配置文件

**工具配置页面增强**
- 在工具详情/编辑页面新增"配置文件绑定"区域
- 显示当前工具已绑定的配置文件列表
- 每个绑定显示：
  - 配置文件名
  - 输出文件名（运行时生成的文件名）
  - 传递方式（`--config` 参数或环境变量）
- 操作：
  - 从配置文件库选择绑定
  - 新建配置文件并绑定
  - 编辑绑定参数（输出文件名、传递方式）
  - 解除绑定

**绑定关系存储**
- 保存在 `configs/plugins/{plugin-id}.mapping.yaml`
- 结构：
  ```yaml
  tools:
    tool-id:
      config_templates:
        - name: database_conf
          template: database.conf.tmpl  # 引用全局配置文件
          output: database.conf
          arg: --config
  ```

### 3. 工作流自动继承

**运行时行为**
- 工作流调用工具时，框架自动：
  1. 读取工具绑定的配置文件
  2. 用插件业务配置渲染模板
  3. 生成配置文件到临时目录
  4. 通过参数或环境变量传递给工具脚本
- 无需在工作流节点额外配置

### 数据流

**读取配置文件库**
- `GET /api/config/templates` - 列出所有配置文件

**读取配置文件内容**
- `GET /api/config/templates/{name}` - 读取模板内容

**保存配置文件**
- `PUT /api/config/templates/{name}` - 保存模板内容
- `POST /api/config/templates` - 新增配置文件
- `DELETE /api/config/templates/{name}` - 删除配置文件

**工具绑定配置**
- `GET /api/plugins/{id}/tools/{tool-id}/bindings` - 读取工具绑定
- `POST /api/plugins/{id}/tools/{tool-id}/bindings` - 新增绑定
- `PUT /api/plugins/{id}/tools/{tool-id}/bindings/{name}` - 更新绑定
- `DELETE /api/plugins/{id}/tools/{tool-id}/bindings/{name}` - 删除绑定

## Acceptance Criteria

* [ ] 配置文件库页面可以列出、新增、编辑、删除配置文件
* [ ] 工具配置页面可以绑定配置文件
* [ ] 多个工具可以共用同一个配置文件
* [ ] 工作流调用工具时自动使用工具绑定的配置
* [ ] 前端构建通过
* [ ] 后端测试通过

## Definition of Done

* `npm run build --prefix web` 通过
* `GOTOOLCHAIN=local go test ./internal/server/...` 通过
* `GOTOOLCHAIN=local go build -o bin/opsctl.exe ./cmd/opsctl` 通过

## Out of Scope

* 配置文件版本管理
* 配置文件模板语法校验
* 配置文件预览/渲染
* 配置文件导入/导出
* 工作流节点级别覆盖配置（保持简单，只继承工具配置）

## Technical Approach

### 后端实现

**1. 配置文件库 API**
- 新增 `internal/server/` 中的配置文件库 handlers
- 文件操作：读取、写入、删除 `configs/templates/*.tmpl`
- 安全检查：防止路径穿越

**2. 工具绑定 API**
- 读取/写入 `configs/plugins/{plugin-id}.mapping.yaml`
- 合并 `plugin.yaml` 和 `mapping.yaml` 的配置模板

**3. 运行时配置渲染**
- 修改 `internal/runner/` 工具执行逻辑
- 读取工具绑定的配置模板
- 渲染模板并传递给工具

### 前端实现

**1. 配置文件库页面**
- 新增 `ConfigTemplateLibrary` 组件
- 列表 + 编辑器 + 新增/删除操作

**2. 工具配置绑定**
- 修改工具详情页面（或新增工具配置页）
- 新增 `ToolConfigBindings` 组件
- 选择配置文件 + 编辑绑定参数

### 数据结构

**配置文件库 API 响应**：
```json
{
  "data": {
    "templates": [
      {
        "name": "database.conf.tmpl",
        "size": 1024,
        "modified_at": "2026-05-06T10:00:00Z",
        "used_by_count": 3
      }
    ]
  }
}
```

**工具绑定 API 响应**：
```json
{
  "data": {
    "bindings": [
      {
        "name": "database_conf",
        "template": "database.conf.tmpl",
        "output": "database.conf",
        "arg": "--config"
      }
    ]
  }
}
```

## Technical Notes

* 主要文件：
  - 后端：`internal/server/server.go`（新增 API）
  - 后端：`internal/runner/runner.go`（配置渲染逻辑）
  - 前端：`web/src/main.jsx`（新增配置文件库和工具绑定组件）
* 配置文件路径：`configs/templates/`（全局共享）
* 绑定关系：`configs/plugins/{plugin-id}.mapping.yaml`
* 向后兼容：保持支持 `plugin.yaml` 中的 `config_templates`

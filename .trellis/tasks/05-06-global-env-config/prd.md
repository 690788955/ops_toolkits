# 实现全局环境配置功能

## Goal

新增全局环境配置文件 `configs/global-env.conf`，用于存储可被所有工具插件调用的全局参数（如数据库连接、API密钥、环境标识等）。框架负责将配置文件路径通过环境变量传递给插件，插件自主选择如何解析。同时框架提供 `.env` 格式的解析和配置合并支持。将现有的"全局配置"重命名为"框架设置"以区分用途。

## What I already know

* **当前配置系统**：
  - `configs/ops.yaml`：框架设置（应用名称、端口、插件路径等）
  - `configs/plugins/{plugin-id}.yaml`：插件业务配置
  - `RootConfig.ConfigDefaults`：已存在的全局配置默认值字段
  - 配置合并逻辑在 `toolConfigLayers()` 中实现

* **配置合并顺序**（从 `internal/app/app.go:430`）：
  1. 工具参数默认值
  2. `reg.Root.ConfigDefaults`（全局配置默认值）
  3. 插件共享配置
  4. 插件包默认配置
  5. 工具配置默认值
  6. 插件宿主配置

* **前端配置标签**：
  - 位于 `web/src/main.jsx` 的 `buildConfigItems()` 函数
  - 当前显示"全局配置"（`configs/ops.yaml`）和插件配置

* **用户需求**：
  - 全局环境配置**不需要**版本管理
  - 插件自主选择如何解析配置文件
  - 框架提供 `.env` 格式的解析和合并支持

## Assumptions (temporary)

* 全局环境配置文件默认为 `.env` 格式（`KEY=value`）
* 框架通过环境变量 `OPS_GLOBAL_ENV_FILE` 传递文件路径给插件
* 插件可以自己解析文件（Shell 可以 `source`，其他语言可以用对应的解析器）
* 框架解析 `.env` 格式并合并到配置层中

## Open Questions

无 - 需求已明确

## Requirements (evolving)

### 后端实现

1. **新增全局环境配置文件**：
   - 文件路径：`configs/global-env.conf`
   - 格式：`.env` 格式（`KEY=value`），支持注释（`#`）
   - 如果文件不存在，返回空配置（不报错）
   - 加载函数：`config.LoadGlobalEnv(baseDir string) (Values, error)`

2. **环境变量传递**：
   - 工具执行时设置环境变量：`OPS_GLOBAL_ENV_FILE=/absolute/path/to/configs/global-env.conf`
   - 插件脚本可以直接使用：`source $OPS_GLOBAL_ENV_FILE` 或自己解析

3. **配置合并逻辑调整**：
   - 在 `toolConfigLayers()` 中添加全局环境配置层
   - 框架解析 `.env` 格式并合并到配置中
   - 合并顺序调整为：
     1. 工具参数默认值
     2. **全局环境配置** (`global-env.conf`) ← 新增
     3. `reg.Root.ConfigDefaults`（保持向后兼容）
     4. 插件共享配置
     5. 插件包默认配置
     6. 工具配置默认值
     7. 插件宿主配置

4. **Registry 加载全局环境配置**：
   - 在 `registry.Load()` 中加载 `global-env.conf`
   - 存储到 `Registry.GlobalEnv` 字段

### 前端实现

5. **配置标签 UI 调整**：
   - "全局配置" → "框架设置"（`configs/ops.yaml`）
   - 新增 "全局环境"（`configs/global-env.conf`）
   - 两者都显示在配置标签页

6. **配置编辑功能**：
   - 框架设置：保持现有编辑功能（YAML 编辑器）
   - 全局环境：文本编辑器（`.env` 格式）

### API 实现

7. **新增全局环境配置 API**：
   - `GET /api/config/global-env` - 读取全局环境配置
   - `PUT /api/config/global-env` - 保存全局环境配置

## Acceptance Criteria (evolving)

* [ ] 创建 `configs/global-env.conf` 示例文件（`.env` 格式）
* [ ] 后端可以加载全局环境配置（文件不存在时返回空配置）
* [ ] 工具执行时设置 `OPS_GLOBAL_ENV_FILE` 环境变量
* [ ] 框架解析 `.env` 格式并合并到配置中
* [ ] 插件配置可以覆盖全局环境配置的值
* [ ] Shell 脚本可以直接 `source $OPS_GLOBAL_ENV_FILE`
* [ ] 前端配置标签显示"框架设置"和"全局环境"两项
* [ ] 可以通过 Web UI 编辑全局环境配置（文本编辑器）
* [ ] API 可以读取和保存全局环境配置
* [ ] 前端构建通过
* [ ] 后端测试通过

## Definition of Done (team quality bar)

* 前端构建 `npm run build --prefix web` 通过
* 后端测试 `go test ./...` 通过
* 全局环境配置功能在 CLI 和 Web UI 中正常工作
* 配置合并逻辑正确（优先级验证）

## Out of Scope (explicit)

* 全局环境配置的版本管理（用户明确不需要）
* 全局环境配置的加密存储（后续可以考虑）
* 全局环境配置的权限控制
* 全局环境配置的模板变量功能

## Technical Approach

### 配置合并优先级

```
工具执行时的参数来源（从低到高）：
1. 工具参数默认值
2. 全局环境配置 (global-env.yaml) ← 新增
3. reg.Root.ConfigDefaults (向后兼容)
4. 插件共享配置
5. 插件包默认配置
6. 工具配置默认值
7. 插件宿主配置
8. CLI/API 传入的参数
```

### 文件结构

```
configs/
├── ops.yaml              # 框架设置（应用名称、端口、插件路径）
├── global-env.conf       # 全局环境配置（数据库、API密钥等）← 新增
└── plugins/
    └── {plugin-id}.yaml  # 插件业务配置
```

### 示例配置

`configs/global-env.conf`:
```bash
# 全局环境配置示例
# 数据库配置
DB_HOST=localhost
DB_PORT=5432
DB_NAME=mydb
DB_USER=admin

# API 配置
API_BASE_URL=https://api.example.com
API_TIMEOUT=30s
API_KEY=your-api-key-here

# 环境标识
ENVIRONMENT=dev
LOG_LEVEL=info
```

### 插件使用方式

**Shell 脚本插件**：
```bash
#!/usr/bin/env bash
# 直接 source 全局环境配置
source "$OPS_GLOBAL_ENV_FILE"

# 使用全局配置
echo "连接数据库: $DB_HOST:$DB_PORT/$DB_NAME"
curl -H "Authorization: Bearer $API_KEY" "$API_BASE_URL/endpoint"
```

**其他语言插件**：
- Python: 使用 `python-dotenv` 库
- Go: 使用 `godotenv` 库
- Node.js: 使用 `dotenv` 包

### 配置优先级

插件可以覆盖全局环境配置：
```
全局环境配置: DB_HOST=localhost
插件配置: DB_HOST=prod-db.example.com
最终值: prod-db.example.com (插件配置优先)
```

## Technical Notes

* 相关文件：
  - 后端配置加载：`internal/config/load.go`
  - 配置类型定义：`internal/config/types.go`
  - 配置合并逻辑：`internal/app/app.go` (`toolConfigLayers()`)
  - Registry 加载：`internal/registry/registry.go`
  - 前端配置标签：`web/src/main.jsx` (`buildConfigItems()`)
  - 服务器 API：`internal/server/server.go`

* 需要新增的类型/函数：
  - `Registry.GlobalEnv` 字段
  - `config.LoadGlobalEnv(baseDir string) (Values, error)`
  - API handlers: `handleGlobalEnvGet()`, `handleGlobalEnvPut()`

* 向后兼容性：
  - `reg.Root.ConfigDefaults` 保持不变，继续支持
  - 全局环境配置文件不存在时不报错，返回空配置

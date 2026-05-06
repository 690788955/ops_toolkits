# 配置版本管理系统

## Goal

重构配置管理系统，支持配置文件的版本快照管理，移除映射规则，让工具运行时可以选择使用哪个版本的配置。

## What I already know

* 当前实现：
  - 配置分为：全局配置、插件业务配置、插件映射规则
  - 配置文件路径：`configs/ops.yaml`、`configs/plugins/{plugin-id}.yaml`、`configs/plugins/{plugin-id}.mapping.yaml`
  - 配置标签显示所有配置项，点击可编辑
  - 工具使用固定的配置文件

* 用户期望：
  - 移除映射规则功能（不需要 `mapping.yaml`）
  - 配置文件支持版本快照（保存多个版本）
  - 工具运行时可以指定使用哪个版本的配置
  - 配置标签中可以管理配置版本（保存快照、恢复、删除）

## Assumptions (temporary)

* 版本快照存储在同一目录下
* 默认使用无后缀的配置文件（如 `plugin.config-demo.yaml`）
* 工具运行时通过参数指定版本（如 `--config-version prod`）

## Design Decisions

### 版本命名规则
**决策**：用户自定义版本名称（选项 B）

**理由**：
- 语义化命名便于识别版本用途（如 `prod`, `dev`, `backup-20260506`）
- 灵活性高，适应不同使用场景
- 文件命名格式：`{config-file}.{version-name}`（如 `plugin.demo.yaml.prod`）

**实现要点**：
- 保存时检查版本名称冲突
- 版本名称限制：字母、数字、连字符、下划线
- 提供版本描述字段（可选）用于记录版本用途

### 版本存储位置
**决策**：元数据 + 版本目录（选项 C）

**理由**：
- 版本文件独立管理，目录结构清晰
- 元数据文件记录版本信息（创建时间、描述、标签等）
- 便于扩展（未来可以添加更多元数据字段）

**存储结构**：
```
configs/
├── ops.yaml                                    # 主配置（当前版本）
├── ops.yaml.meta.json                          # 全局配置版本元数据
├── .versions/
│   └── ops/
│       ├── prod.yaml                           # 版本快照文件
│       └── dev.yaml
└── plugins/
    ├── plugin.demo.yaml                        # 主配置（当前版本）
    ├── plugin.demo.yaml.meta.json              # 插件配置版本元数据
    └── .versions/
        └── plugin.demo/
            ├── prod.yaml                       # 版本快照文件
            └── dev.yaml
```

**元数据文件格式**（`{config-file}.meta.json`）：
```json
{
  "versions": [
    {
      "name": "prod",
      "description": "生产环境配置",
      "created_at": "2026-05-06T14:30:00Z",
      "created_by": "junguang.chen"
    },
    {
      "name": "dev",
      "description": "开发环境配置",
      "created_at": "2026-05-06T15:00:00Z",
      "created_by": "junguang.chen"
    }
  ]
}
```

**实现要点**：
- 保存版本时同时更新元数据文件和版本快照文件
- 删除版本时同时清理元数据和快照文件
- 元数据文件不存在时自动创建
- 保证元数据和实际文件的一致性

### 默认版本行为
**决策**：混合模式（选项 C）

**理由**：
- 兼顾灵活性和简单性
- 支持快速切换默认版本，同时保持向后兼容

**行为规则**：
1. 工具运行时不指定 `--config-version` 参数：
   - 如果元数据中有版本标记为 `default: true`，使用该版本
   - 如果没有 `default` 标记，使用主配置文件（如 `plugin.demo.yaml`）
2. 工具运行时指定 `--config-version <name>`：
   - 使用指定的版本快照文件
   - 如果版本不存在，报错

**元数据扩展**（添加 `default` 字段）：
```json
{
  "versions": [
    {
      "name": "prod",
      "description": "生产环境配置",
      "created_at": "2026-05-06T14:30:00Z",
      "created_by": "junguang.chen",
      "default": true
    },
    {
      "name": "dev",
      "description": "开发环境配置",
      "created_at": "2026-05-06T15:00:00Z",
      "created_by": "junguang.chen",
      "default": false
    }
  ]
}
```

**实现要点**：
- 同一配置文件最多只能有一个版本标记为 `default: true`
- 设置新的默认版本时，自动取消其他版本的 `default` 标记
- UI 中显示当前默认版本（如果有）

## Open Questions

无 - 核心设计决策已确认。

## Requirements (evolving)

### 核心功能

* **移除映射规则**：
  - 删除 `mapping.yaml` 相关代码（前端 + 后端）
  - 移除配置编辑界面的"映射规则"子标签
  - 只保留"业务配置"编辑
  - 移除 `internal/server/plugin_config.go` 中的映射规则 API
  - 移除 `web/src/main.jsx` 中的映射规则编辑器

* **配置版本快照**：
  - 版本命名：用户自定义（字母、数字、连字符、下划线）
  - 版本存储：`.versions/` 目录 + 元数据文件（`{config-file}.meta.json`）
  - 版本元数据：名称、描述、创建时间、创建者、是否默认
  - 版本操作：保存、加载、删除、设置默认

* **版本管理界面**（Web UI）：
  - 配置编辑界面显示版本列表（侧边栏或顶部区域）
  - 版本列表显示：版本名称、描述、创建时间、默认标记
  - 操作按钮：
    - "保存为新版本"：弹出对话框输入版本名称和描述
    - "加载版本"：点击版本项，加载到编辑器
    - "删除版本"：确认后删除版本
    - "设为默认"：标记该版本为默认版本
  - 当前编辑状态提示（是否有未保存的修改）

* **工具运行时版本选择**：
  - CLI 支持 `--config-version <name>` 参数
  - Web UI 执行配置中添加"配置版本"选择框（可选，默认为空）
  - 工作流节点配置中添加"配置版本"字段（可选）
  - 版本解析逻辑：
    1. 如果指定了版本，使用 `.versions/{config-id}/{version}.yaml`
    2. 如果未指定版本且元数据中有默认版本，使用默认版本
    3. 否则使用主配置文件

* **配置标签优化**：
  - 显示所有配置文件（移除 config_templates 过滤）
  - 配置项卡片显示版本数量（如"3 个版本"）
  - 全局配置（`configs/ops.yaml`）也支持版本管理

### 后端 API

* `GET /api/config/{type}/{id}/versions` - 获取配置的所有版本列表（从元数据文件读取）
* `POST /api/config/{type}/{id}/versions` - 保存当前配置为新版本
  - 请求体：`{ "name": "版本名称", "description": "版本描述" }`
  - 创建版本快照文件 + 更新元数据文件
* `GET /api/config/{type}/{id}/versions/{version}` - 获取指定版本的内容
* `DELETE /api/config/{type}/{id}/versions/{version}` - 删除指定版本
  - 删除版本快照文件 + 更新元数据文件
* `PUT /api/config/{type}/{id}/versions/{version}/default` - 设置默认版本
  - 更新元数据文件中的 `default` 标记
* 修改现有配置读取 API，支持 `?version=<name>` 参数
* 移除映射规则相关 API：
  - 删除 `GET /api/plugins/{id}/mapping`
  - 删除 `PUT /api/plugins/{id}/mapping`

## Acceptance Criteria (evolving)

* [ ] 移除所有映射规则相关代码（前端 + 后端）
* [ ] 配置编辑界面只显示"业务配置"（无"映射规则"子标签）
* [ ] 配置编辑界面显示版本列表（侧边栏或顶部区域）
* [ ] 可以保存当前配置为新版本（输入版本名称和描述）
* [ ] 可以加载历史版本到编辑器
* [ ] 可以删除历史版本
* [ ] 可以设置某个版本为默认版本
* [ ] 版本列表显示版本元数据（名称、描述、创建时间、默认标记）
* [ ] CLI 支持 `--config-version` 参数
* [ ] Web UI 执行配置中可以选择配置版本
* [ ] 工作流节点配置中可以指定配置版本
* [ ] 配置标签显示所有插件配置（移除 config_templates 过滤）
* [ ] 配置项卡片显示版本数量
* [ ] 版本文件正确存储到 `.versions/` 目录
* [ ] 元数据文件正确创建和更新
* [ ] 版本解析逻辑正确（指定版本 > 默认版本 > 主配置）
* [ ] 全局配置（`configs/ops.yaml`）支持版本管理

## Definition of Done (team quality bar)

* 前端构建 `npm run build --prefix web` 通过
* 后端测试 `go test ./...` 通过
* 配置版本功能在 CLI 和 Web UI 中正常工作
* 移除映射规则后现有功能不受影响

## Out of Scope (explicit)

* 配置版本的 diff 对比功能
* 配置版本的合并功能
* 配置版本的分支管理
* 配置版本的权限控制

## Technical Notes

* 相关文件：
  - 前端：`web/src/main.jsx`（配置标签和版本管理 UI）
  - 后端：`internal/server/server.go`（全局配置 API）
  - 后端：`internal/server/plugin_config.go`（插件配置 API，需要移除映射规则部分）
  - 后端：新增 `internal/server/config_version.go`（版本管理 API）
  - 配置加载：`internal/config/load.go`（需要支持版本参数）
  - 运行器：`internal/runner/runner.go`（需要传递版本参数）

* 元数据文件格式：
  - 文件名：`{config-file}.meta.json`（如 `plugin.demo.yaml.meta.json`）
  - 位置：与主配置文件同目录
  - 内容：JSON 格式，包含版本数组

* 版本快照文件：
  - 位置：`.versions/{config-id}/{version-name}.yaml`
  - 全局配置：`.versions/ops/{version-name}.yaml`
  - 插件配置：`.versions/plugin.{plugin-id}/{version-name}.yaml`

* 需要考虑的边界情况：
  - 版本名称冲突（保存时检查）
  - 版本文件损坏（读取时捕获错误）
  - 并发保存版本（文件锁或原子操作）
  - 删除正在使用的默认版本（自动取消默认标记）
  - 元数据文件与实际版本文件不一致（启动时校验或自动修复）
  - 版本名称非法字符（保存时验证）

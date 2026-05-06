# 实现总结：配置标签阶段二扩展功能

## 实现完成

已成功实现配置标签的阶段二扩展功能，支持全局配置、插件配置的统一管理，并实现分类过滤。

## 修改的文件

### 后端实现

1. **internal/server/server.go**
   - 新增 `globalConfigHandler` 处理全局配置 API
   - 新增 `handleGlobalConfigGet` 读取 `configs/ops.yaml` 内容
   - 新增 `handleGlobalConfigPut` 保存全局配置并重新加载运行时配置
   - 在 `NewHandler` 中注册 `/api/config/global` 路由
   - PUT 请求在写入前验证 YAML 格式，避免写入无效配置

2. **internal/server/global_config_test.go** (新文件)
   - `TestGlobalConfigGetAPI` - 测试读取全局配置
   - `TestGlobalConfigPutAPI` - 测试保存全局配置
   - `TestGlobalConfigPutRejectsInvalidYAML` - 测试拒绝无效 YAML
   - `TestGlobalConfigRejectsNonGetPut` - 测试拒绝非 GET/PUT 方法

### 前端实现

3. **web/src/main.jsx**
   - 修改 `ConfigPanel` 组件：
     - 新增 `activeCategory` 参数支持分类过滤
     - 新增 `buildConfigItems` 函数构建配置项列表
     - 配置项包含全局配置和插件配置
     - 实现分类过滤逻辑：全局配置始终显示，插件配置根据相关分类过滤
     - 配置项卡片显示类型标签、描述和多行文件路径
   - 新增 `GlobalConfigPanel` 组件：
     - 单个 YAML 编辑器（textarea）
     - 调用 `GET /api/config/global` 读取配置
     - 调用 `PUT /api/config/global` 保存配置
     - 保存成功后刷新 catalog
   - 修改 `App` 组件：
     - 传递 `activeCategory` 参数给 `ConfigPanel`
   - 修改 `ConfigPanel` 调用：
     - 根据 `configSelectedPlugin.type` 渲染不同编辑器
     - `type === 'global'` → `GlobalConfigPanel`
     - `type === 'plugin'` → `PluginConfigPanel`

4. **web/src/styles.css**
   - 新增 `.configTypeLabel` 样式：类型标签（全局/插件）
   - 新增 `.configFilePaths` 样式：文件路径容器
   - 新增 `.configFilePath` 样式：单个文件路径
   - 新增 `.fileLabel` 样式：文件标签（业务配置/映射规则）

## API 设计

### GET /api/config/global
读取全局配置文件内容。

**响应示例：**
```json
{
  "data": {
    "content": "name: 运维控制台\ndescription: ...",
    "path": "configs/ops.yaml"
  }
}
```

### PUT /api/config/global
保存全局配置文件并重新加载运行时配置。

**请求体：**
```json
{
  "content": "name: 运维控制台\n..."
}
```

**响应示例：**
```json
{
  "status": "saved",
  "data": {
    "message": "全局配置已保存"
  }
}
```

**错误处理：**
- 400 Bad Request：YAML 格式无效（写入前验证）
- 500 Internal Server Error：文件写入失败或重新加载失败

## 配置项数据结构

```typescript
type ConfigItem = {
  type: 'global' | 'plugin'
  id: string  // 全局为'global'，插件为plugin-id
  name: string  // 显示名称
  typeLabel: string  // 类型标签（全局/插件）
  description: string  // 用途说明
  files: Array<{
    path: string  // 配置文件路径
    label: string  // 文件标签（如"业务配置"、"映射规则"）
  }>
  disabled: boolean  // 插件是否已禁用
  relatedCategories?: string[]  // 相关分类ID列表（用于过滤）
}
```

## 分类过滤逻辑

1. **全局配置**：始终显示，不受分类过滤影响
2. **插件配置**：
   - 从 catalog.tools 和 catalog.workflows 提取插件相关的分类
   - 如果当前选中具体分类，只显示 relatedCategories 包含该分类的插件配置
   - 如果当前在全局视图（无分类或跨分类），显示所有插件配置

## 配置项卡片布局

```
┌─────────────────────────────────────┐
│ 配置名称 [类型标签]                  │
│ 用途说明                             │
│                                      │
│ 业务配置：configs/plugins/xxx.yaml   │
│ 映射规则：configs/plugins/xxx.mapping.yaml │
└─────────────────────────────────────┘
```

## 验证结果

### 后端测试
- ✅ 所有现有测试通过
- ✅ 新增 4 个全局配置 API 测试全部通过
- ✅ `go test ./...` 全部通过

### 前端构建
- ✅ `npm run build --prefix web` 成功
- ✅ 生成的资源文件正常嵌入

### 二进制编译
- ✅ `go build -o bin/opsctl.exe ./cmd/opsctl` 成功
- ✅ `./bin/opsctl.exe validate` 通过

## 实现亮点

1. **安全性**：PUT 请求在写入前验证 YAML 格式，避免写入无效配置
2. **一致性**：保存后自动重新加载运行时配置，确保前后端状态一致
3. **用户体验**：
   - 配置项卡片清晰显示类型、用途和文件路径
   - 分类过滤自动提取插件相关分类，无需手动配置
   - 全局配置始终可见，方便快速访问
4. **可扩展性**：配置项数据结构支持未来扩展工具级配置

## 完成的需求

### 基础功能（阶段一，已完成）
- ✅ 主内容区标签栏显示"工具/工作流/编排器/配置"四个标签
- ✅ 点击"配置"标签显示配置项列表
- ✅ 点击配置项后切换到配置界面
- ✅ 配置界面顶部仍显示标签栏
- ✅ 插件配置界面功能完整
- ✅ 左侧边栏插件管理模态框中不再显示"配置"按钮

### 扩展功能（阶段二，本次实现）
- ✅ 配置列表显示全局配置项（`configs/ops.yaml`）
- ✅ 配置列表显示所有插件配置项（`configs/plugins/{plugin-id}.yaml`）
- ✅ 配置列表显示插件映射配置（`configs/plugins/{plugin-id}.mapping.yaml`）
- ✅ 配置项卡片多行显示配置文件路径
- ✅ 配置项卡片显示配置类型和用途说明
- ✅ 在具体分类下，配置列表只显示该分类相关的配置项
- ✅ 在全局视图下，配置列表显示所有配置项
- ✅ 全局配置编辑界面支持编辑 `ops.yaml`
- ✅ 插件配置编辑界面保持现有功能（业务配置/映射规则）

## 未来扩展方向

1. **工具级配置**：支持单个工具的配置文件编辑
2. **配置模板**：提供常用配置模板，方便快速创建
3. **配置验证**：在前端提供 YAML 语法高亮和实时验证
4. **配置历史**：记录配置变更历史，支持回滚
5. **配置导入导出**：支持配置文件的批量导入导出

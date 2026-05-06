# brainstorm: 插件配置入口移至演示页标签栏

## Goal

将插件配置操作从左侧边栏的"已安装插件"列表移至主内容区的"插件演示"页面标签栏，与"工具"、"工作流"、"编排器"并列，形成"工具/工作流/编排器/配置"四个标签。

## What I already know

* 当前实现：
  - 左侧边栏有"插件演示"入口，点击后主内容区显示"工具/工作流/编排器"三个标签
  - 左侧边栏有"+"按钮，点击后弹出插件管理模态框
  - 插件管理模态框中的"已安装插件"列表每个插件有"配置"按钮
  - 点击"配置"按钮打开 `PluginConfigModal` 组件
* 用户期望：
  - 配置操作应该在主内容区的标签栏中，而不是在左侧边栏的模态框中
  - 配置标签应该与工具、工作流、编排器并列
* 相关代码文件：
  - `web/src/main.jsx` - 主界面组件，包含标签栏和插件管理模态框
  - `activeTab` 状态控制当前显示的标签（'tools', 'workflows', 'editor'）
  - `PluginConfigModal` 组件已实现配置界面

## Assumptions (temporary)

* 配置标签应该显示所有已安装插件的列表，每个插件可以点击进入配置界面
* 配置界面复用现有的 `PluginConfigModal` 内容，但不作为模态框，而是作为标签页内容
* 左侧边栏的"+"按钮和插件管理模态框保留，但移除其中的"配置"按钮
* 配置标签应该在用户选择"插件演示"分类时可见

## Open Questions

无 - 用户已确认所有交互方案和过滤逻辑。

**已确认的设计决策**：
1. 配置项分类过滤：在具体分类下，只显示贡献了该分类的插件的配置
2. 全局配置显示：在任何分类下都显示 `configs/ops.yaml`
3. 配置项路径显示：每个配置文件路径占一行（一个插件可能有多个配置文件）

## Requirements (evolving)

### 基础功能（已实现）
* 主内容区标签栏增加"配置"标签，与"工具"、"工作流"、"编排器"并列
* 点击"配置"标签显示配置项列表（占满整个内容区）
* 点击配置项后切换到配置编辑界面（占满整个内容区）
* 配置界面仍显示顶部标签栏（工具/工作流/编排器/配置），再次点击"配置"标签返回列表
* 左侧边栏插件管理模态框中移除"配置"按钮，保留"启用/禁用/删除"操作

### 扩展功能（新需求）
* **配置项类型扩展**：
  - 全局配置：`configs/ops.yaml`（主配置文件）
  - 插件配置：`configs/plugins/{plugin-id}.yaml`（每个插件的业务配置）
  - 插件映射：`configs/plugins/{plugin-id}.mapping.yaml`（插件工具配置模板映射）
  - 工具配置：工具级别的配置文件（如果存在）
* **配置项过滤规则**：
  - 全局配置：始终显示
  - 插件配置：**只显示包含使用 `config_templates` 的工具的插件**
  - 不显示没有配置需求的插件（所有工具都不使用 `config_templates`）
* **分类过滤**：
  - 在具体分类下（如"ES 备份恢复"）：只显示该分类相关的配置项
  - 在全局视图下（跨分类选择）：显示所有配置项
* **配置项展示**：
  - 配置项卡片显示配置文件路径（多行显示，清晰易读）
  - 显示配置类型（全局/插件/工具）
  - 显示配置用途说明
* **配置编辑界面**：
  - 根据配置类型显示不同的编辑界面
  - 全局配置：直接编辑 `ops.yaml`
  - 插件配置：显示"业务配置"和"映射规则"两个子标签
  - 工具配置：编辑工具特定的配置文件

## Acceptance Criteria (evolving)

### 基础功能（已完成）
* [x] 主内容区标签栏显示"工具/工作流/编排器/配置"四个标签
* [x] 点击"配置"标签显示配置项列表（全屏显示）
* [x] 点击配置项后切换到配置界面（全屏显示）
* [x] 配置界面顶部仍显示标签栏，再次点击"配置"标签返回列表
* [x] 插件配置界面功能完整（业务配置/映射规则编辑和保存）
* [x] 左侧边栏插件管理模态框中不再显示"配置"按钮
* [x] 配置保存后刷新 catalog 数据

### 扩展功能（待实现）
* [ ] 配置列表显示全局配置项（`configs/ops.yaml`）
* [ ] 配置列表显示所有插件配置项（`configs/plugins/{plugin-id}.yaml`）
* [ ] 配置列表显示插件映射配置（`configs/plugins/{plugin-id}.mapping.yaml`）
* [ ] 配置项卡片多行显示配置文件路径
* [ ] 配置项卡片显示配置类型和用途说明
* [ ] 在具体分类下，配置列表只显示该分类相关的配置项
* [ ] 在全局视图下，配置列表显示所有配置项
* [ ] 全局配置编辑界面支持编辑 `ops.yaml`
* [ ] 插件配置编辑界面保持现有功能（业务配置/映射规则）

## Definition of Done (team quality bar)

* 前端构建 `npm run build --prefix web` 通过
* 配置功能在新位置正常工作
* 原插件管理模态框的其他功能（上传/导出/启用/禁用/删除）不受影响

## Out of Scope (explicit)

* 不改变配置功能的实现逻辑，只改变入口位置
* 不改变插件管理模态框的其他功能
* 不添加新的配置功能

## Technical Notes

* 相关文件：`web/src/main.jsx`
* 当前标签状态：`activeTab` ('tools' | 'workflows' | 'editor')
* 需要扩展为：`activeTab` ('tools' | 'workflows' | 'editor' | 'config')
* 配置界面组件：`PluginConfigModal` - 需要提取其内容部分，去除模态框包装
* 插件列表数据来源：`catalog.plugins` 数组

## Technical Approach

### 阶段一：基础功能（已完成）
**状态管理**：
- 新增 `configSelectedPlugin` 状态，记录当前选中的插件（null 表示显示列表）
- 点击"配置"标签时：
  - 如果 `configSelectedPlugin` 为 null → 显示插件列表
  - 如果 `configSelectedPlugin` 不为 null → 清空并返回插件列表
- 点击插件列表中的某个插件 → 设置 `configSelectedPlugin` 为该插件

**组件结构**：
- 提取 `PluginConfigModal` 的内容部分为独立组件 `PluginConfigPanel`
- 在 `activeTab === 'config'` 时：
  - 如果 `configSelectedPlugin` 为 null → 渲染插件列表
  - 如果 `configSelectedPlugin` 不为 null → 渲染 `PluginConfigPanel`

**插件管理模态框**：
- 移除"配置"按钮（第 484 行）
- 保留其他功能不变

### 阶段二：扩展功能（待实现）
**配置项数据结构**：
```typescript
type ConfigItem = {
  type: 'global' | 'plugin' | 'tool'
  id: string  // 全局为'global'，插件为plugin-id，工具为tool-id
  name: string  // 显示名称
  description: string  // 用途说明
  files: Array<{
    path: string  // 配置文件路径
    label: string  // 文件标签（如"业务配置"、"映射规则"）
    exists: boolean  // 文件是否存在
  }>
  relatedCategories?: string[]  // 相关分类ID列表（用于过滤）
}
```

**配置项列表构建逻辑**：
1. 始终包含全局配置项（`configs/ops.yaml`）
2. 遍历 `catalog.plugins`，为每个插件创建配置项：
   - 业务配置：`configs/plugins/{plugin-id}.yaml`
   - 映射规则：`configs/plugins/{plugin-id}.mapping.yaml`
   - 从插件的工具/工作流贡献中提取 `relatedCategories`
3. 根据当前分类过滤：
   - 如果是全局视图（无分类或跨分类）：显示所有配置项
   - 如果是具体分类：只显示 `relatedCategories` 包含该分类的配置项 + 全局配置

**配置项卡片布局**：
```
┌─────────────────────────────────────┐
│ [类型标签] 配置名称                  │
│ 用途说明                             │
│                                      │
│ 配置文件：                           │
│ • configs/ops.yaml                   │
│ • configs/plugins/plugin.demo.yaml   │
│ • configs/plugins/plugin.demo.mapping.yaml │
└─────────────────────────────────────┘
```

**配置编辑界面扩展**：
- 新增 `configSelectedItem` 状态（替代 `configSelectedPlugin`）
- 根据 `configSelectedItem.type` 渲染不同的编辑器：
  - `type === 'global'`：渲染全局配置编辑器（单个 YAML 编辑器）
  - `type === 'plugin'`：渲染现有的 `PluginConfigPanel`（业务配置/映射规则）
  - `type === 'tool'`：渲染工具配置编辑器（待设计）

**后端 API 需求**（如果需要）：
- `GET /api/config/global`：读取全局配置
- `PUT /api/config/global`：保存全局配置
- 现有插件配置 API 保持不变

# 前端优化：框架设置移至齿轮图标 + 工具分页

## Goal

优化前端用户体验，将平台级的"框架设置"从配置标签页移至右上角齿轮图标（模态框形式），并为工具列表添加分页功能，提升大量工具场景下的性能和可用性。

## What I already know

* **当前配置标签页结构**：
  - 显示三类配置：框架设置（`configs/ops.yaml`）、全局环境（`configs/global-env.conf`）、插件配置
  - 框架设置是平台级配置，修改频率低，影响范围大
  - 全局环境和插件配置是业务级配置，属于日常运维操作
  
* **现有组件**：
  - `GlobalConfigPanel` - 框架设置编辑器（YAML 格式）
  - `GlobalEnvConfigPanel` - 全局环境配置编辑器（.env 格式）
  - `PluginConfigPanel` - 插件配置编辑器
  - `buildConfigItems()` - 构建配置项列表的函数
  
* **应用结构**（`web/src/main.jsx`）：
  - `App` 组件包含 `aside.sidebar` 和 `main.content`
  - `header.topbar` 显示当前分类名称和描述
  - `RunPanel` 组件显示工具/工作流列表
  - 当前工具列表没有分页，所有工具一次性显示
  
* **设计要求**：
  - 使用 Expo 设计风格（参考 `DESIGN.md`）
  - Cloud Gray 背景（`#f0f0f3`）
  - Pure White 卡片（`#ffffff`）
  - Pill-shaped 按钮（9999px radius）
  - Inter 字体，权重 400-900

## Assumptions (temporary)

* 齿轮图标使用 SVG 或 Unicode 字符（⚙️）
* 模态框使用半透明遮罩层 + 居中白色卡片
* 框架设置模态框可以扩展为"平台设置"，未来可能包含主题、语言等选项
* 工具分页默认每页显示 20 条
* 分页组件显示在工具列表底部

## Open Questions

无 - 所有设计决策已确认。

## Design Decisions

1. **齿轮图标样式**：使用 Unicode 字符 `⚙️` - 简单直观，无需额外资源
2. **模态框标题**：使用"平台设置" - 为未来扩展预留空间（主题、语言等）
3. **分页大小**：每页 10/20/50 可选 - 提供灵活性，默认 20 条
4. **分页控件样式**：简单的 "上一页 | 1/5 | 下一页" - 符合 Expo 简洁风格

## Requirements (evolving)

### 功能 1：框架设置移至齿轮图标

1. **齿轮图标按钮**：
   - 位置：`header.topbar` 右侧
   - 样式：Expo 风格，pill-shaped 按钮，Pure White 背景，Border Lavender 边框
   - 图标：Unicode 字符 `⚙️`
   - 点击：打开平台设置模态框

2. **平台设置模态框**：
   - 遮罩层：半透明黑色背景（`rgba(0,0,0,0.5)`）
   - 内容卡片：Pure White（`#ffffff`），居中显示，最大宽度 800px，comfortably rounded（8px）
   - 标题："平台设置"（Inter 20px weight 600）
   - 内容：复用 `GlobalConfigPanel` 组件的编辑器部分
   - 关闭：点击遮罩层或右上角关闭按钮（×）
   - 保存：保存后刷新 catalog 并关闭模态框

3. **配置标签页调整**：
   - 从 `buildConfigItems()` 中移除"框架设置"项
   - 保留"全局环境"和"插件配置"

### 功能 2：工具列表分页

1. **分页逻辑**：
   - 在 `RunPanel` 组件中添加分页状态（currentPage, pageSize）
   - 默认每页 20 条，可选 10/20/50
   - 计算总页数 = Math.ceil(entries.length / pageSize)
   - 只显示当前页的工具：entries.slice((currentPage - 1) * pageSize, currentPage * pageSize)

2. **分页控件**：
   - 位置：工具列表底部
   - 布局：左侧显示每页大小选择器，右侧显示分页导航
   - 每页大小选择器：下拉框，选项 10/20/50
   - 分页导航：上一页按钮 | 当前页/总页数 | 下一页按钮
   - 样式：Expo 风格，pill-shaped 按钮（9999px radius）

3. **交互优化**：
   - 切换分类/搜索/标签时重置到第一页
   - 选中工具后保持当前页码
   - 上一页按钮在第一页时禁用
   - 下一页按钮在最后一页时禁用

## Acceptance Criteria (evolving)

### 功能 1：框架设置移至齿轮图标
* [ ] 右上角显示齿轮图标按钮
* [ ] 点击齿轮图标打开模态框
* [ ] 模态框显示框架设置编辑器
* [ ] 可以编辑和保存框架设置
* [ ] 保存后刷新 catalog
* [ ] 点击遮罩层或关闭按钮关闭模态框
* [ ] 配置标签页不再显示"框架设置"项
* [ ] 使用 Expo 设计风格

### 功能 2：工具列表分页
* [ ] 工具列表底部显示分页控件
* [ ] 只显示当前页的工具
* [ ] 可以切换上一页/下一页
* [ ] 显示当前页码和总页数
* [ ] 切换分类/搜索/标签时重置到第一页
* [ ] 分页控件使用 Expo 设计风格

## Definition of Done (team quality bar)

* 前端构建 `npm run build --prefix web` 通过
* 功能在浏览器中正常工作
* 使用 Expo 设计风格（参考 `DESIGN.md`）
* 代码符合现有项目风格

## Out of Scope (explicit)

* 不添加其他平台设置选项（主题、语言等）
* 不实现工作流列表分页（只做工具列表）
* 不添加每页大小选择器（固定每页 20 条）
* 不添加页码跳转功能（只有上一页/下一页）

## Technical Notes

* 相关文件：`web/src/main.jsx`、`web/src/styles.css`
* 设计参考：`DESIGN.md` - Expo 设计系统
* 现有组件：`GlobalConfigPanel`、`RunPanel`、`buildConfigItems()`
* 模态框实现：可以参考现有的 `PluginManagerModal` 组件

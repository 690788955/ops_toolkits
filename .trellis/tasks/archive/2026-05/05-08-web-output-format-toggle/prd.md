# Web UI 添加输出格式切换控件

## Goal

在 Web UI 的工具执行结果界面添加一个交互式的输出格式切换控件，让用户可以在 text 和 json 两种格式之间切换查看执行结果。

## What I already know

### Web UI 架构
- 单文件 React 应用：`web/src/main.jsx` (155KB)
- 使用 React hooks 进行状态管理
- 使用 @xyflow/react 用于工作流 DAG 可视化

### 执行结果显示流程
1. **ResultView 组件** (main.jsx:1419-1430)
   - 根据 result 对象的类型显示不同内容
   - 如果 `result.detail.data` 存在，渲染 `RunDetail` 组件
   - 如果 `result.response` 存在，渲染 `MessageWithDetails` 组件
   - 否则显示 `result.message`

2. **RunDetail 组件** (main.jsx:1444-1469)
   - 显示运行 ID、状态、目标
   - 显示日志内容（通过 `LogBlock` 组件）
   - 有"查看完整运行记录"的 details 展开区域，显示 JSON 格式的完整数据

3. **LogBlock 组件** (main.jsx:1471-1478)
   - 简单的日志显示组件
   - 使用 `<pre>` 标签显示纯文本内容

### 结果显示区域
- 位置：`<section className="card resultCard">` (main.jsx:353-358)
- 标题："执行结果"
- 内容：`<ResultView result={result} />`

### 当前状态管理
- App 组件中有 `const [result, setResult] = useState({message: '等待执行...'})`
- 执行工具/工作流时，通过 `setResult()` 更新结果

## Assumptions (temporary)

- 用户希望在主要的日志显示区域（LogBlock）添加格式切换功能
- text 格式：当前的纯文本显示方式
- json 格式：可能需要将日志内容解析为 JSON 并格式化显示
- 切换控件应该放在日志标题旁边或上方
- 格式切换状态应该是组件级别的（不需要全局状态）

## Open Questions

### 1. ~~切换控件的位置和样式~~ ✓ 已解决
- **用户选择**：方案 A - LogBlock 标题旁边的 tab 切换
- 在每个 LogBlock 组件的标题旁边添加两个小 tab 按钮："text" 和 "json"
- 使用现有的 `.tab` 和 `.tab.active` 样式
- 每个 LogBlock 独立管理自己的格式状态

### 2. ~~JSON 格式的数据来源~~ ✓ 已解决
- **后端已经返回 JSON 格式的数据**：
  - API `/api/runs/{id}` 返回 `{ data: { record: {...}, logs: {...} } }`
  - `detail.record` 是 JSON 格式的运行记录（包含 id, status, target, steps 等）
  - `detail.logs.stdout/stderr` 是纯文本格式的日志
- **不需要修改后端 API**
- 前端只需要在 text 格式（`logs.stdout`）和 json 格式（`JSON.stringify(detail.record, null, 2)`）之间切换

### 3. 格式切换的范围
- **建议范围**：工具执行的日志（stdout/stderr）和工作流日志
- **不包括**："查看完整运行记录"的展开区域（已经是 JSON 格式）
- **实现位置**：在 `LogBlock` 组件中添加格式切换功能

### 4. 默认格式
- **建议默认**：text 格式（保持当前行为）
- **可选增强**：使用 localStorage 记住用户的选择（可以在后续迭代中添加）

## Feasible Approaches

基于现有的 UI 风格和组件，以下是 3 个可行的实现方案：

### 方案 A：LogBlock 标题旁边的 tab 切换（推荐）

**实现方式**：
- 在每个 LogBlock 组件的标题旁边添加两个小 tab 按钮："text" 和 "json"
- 使用现有的 `.tab` 和 `.tab.active` 样式
- 每个 LogBlock 独立管理自己的格式状态（使用 React useState）

**布局示意**：
```
┌─────────────────────────────────┐
│ 标准输出  [text] [json]         │
│ ─────────────────────────────── │
│ 日志内容...                      │
└─────────────────────────────────┘
```

**优点**：
- 每个日志块独立控制，灵活性高
- 使用现有的 tab 样式，与项目 UI 风格一致
- 实现简单，只需修改 LogBlock 组件

**缺点**：
- 如果有多个日志块，需要分别切换

---

### 方案 B：执行结果卡片 header 的全局切换

**实现方式**：
- 在"执行结果"标题旁边（`.cardHeader` 中）添加两个 tab 按钮
- 使用现有的 `.tab` 样式
- 在 ResultView 或 RunDetail 组件中管理全局格式状态
- 所有 LogBlock 共享同一个格式设置

**布局示意**：
```
┌─────────────────────────────────┐
│ 执行结果      [text] [json]     │
│ ─────────────────────────────── │
│ 标准输出                         │
│ 日志内容...                      │
│                                  │
│ 错误输出                         │
│ 日志内容...                      │
└─────────────────────────────────┘
```

**优点**：
- 一次切换影响所有日志块，操作简单
- 适合用户想要统一查看所有日志的场景

**缺点**：
- 不够灵活，无法针对不同日志块使用不同格式

---

### 方案 C：LogBlock 标题旁边的小图标切换

**实现方式**：
- 在每个 LogBlock 的标题旁边添加一个小图标按钮
- 使用 `{ }` 图标表示 json，`T` 或 `≡` 表示 text
- 点击图标在两种格式之间切换
- 使用现有的 `.tagChip` 样式或自定义小按钮样式

**优点**：
- 节省空间，界面更简洁
- 每个日志块独立控制

**缺点**：
- 图标可能不够直观，需要用户学习
- 需要额外的样式定义

## Requirements (evolving)

基于**方案 A**的具体需求：

- [ ] 修改 LogBlock 组件，添加格式切换功能
- [ ] 在 LogBlock 标题（h4）旁边添加两个 tab 按钮："text" 和 "json"
- [ ] 使用 React useState 管理每个 LogBlock 的格式状态（默认 "text"）
- [ ] text 格式：显示原始日志内容（当前行为，使用 `<pre>` 标签）
- [ ] json 格式：尝试解析日志内容为 JSON 并格式化显示；如果解析失败，显示原始内容
- [ ] 使用现有的 `.tab` 和 `.tab.active` 样式（已在 styles.css 中定义）
- [ ] 保持现有的执行结果显示功能不变
- [ ] 在浏览器中测试功能（启动 dev server，测试切换功能）

## Acceptance Criteria (evolving)

- [ ] 用户可以看到输出格式切换控件
- [ ] 点击切换控件时，格式立即切换
- [ ] text 格式显示纯文本日志（当前行为）
- [ ] json 格式显示格式化的 JSON 内容
- [ ] 切换格式不影响其他功能（执行工具、查看完整记录等）

## Definition of Done (team quality bar)

- Tests added/updated (unit/integration where appropriate)
- Lint / typecheck / CI green
- Docs/notes updated if behavior changes
- Rollout/rollback considered if risky
- **实际在浏览器中测试功能**（启动 dev server，测试切换功能）

## Out of Scope (explicit)

- 后端 API 修改（如果不需要）
- 其他格式支持（如 YAML、XML 等）
- 日志内容的编辑或下载功能
- 日志内容的搜索或过滤功能

## Technical Notes

### 文件位置
- Web UI 源代码：`web/src/main.jsx`
- 样式文件：`web/src/styles.css`
- 构建配置：`web/vite.config.js`

### 相关组件
- `ResultView` (main.jsx:1419-1430)
- `RunDetail` (main.jsx:1444-1469)
- `LogBlock` (main.jsx:1471-1478)
- `MessageWithDetails` (main.jsx:1432-1442)

### 开发命令
```bash
npm run dev --prefix web      # Dev server at http://127.0.0.1:5173
npm run build --prefix web    # Build embedded assets
```

### 现有的 JSON 显示
- `RunDetail` 组件中已经有"查看完整运行记录"的 details 展开区域
- 使用 `<pre>{JSON.stringify(detail, null, 2)}</pre>` 显示 JSON
- 这可以作为 JSON 格式显示的参考实现

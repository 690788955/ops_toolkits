# 交互式参数输入引导改进

## Goal

改进命令行工具的参数输入体验，在当前的参数提示基础上，增加可选值列表显示和默认值快捷输入支持，让用户能够更轻松地理解和输入参数。

## What I already know

* 当前 opsctl 已支持交互式参数输入（`PromptMissing` 函数）
* 参数提示已显示：参数名称、描述、类型、必填/可选、默认值（`parameterPromptLabel` 函数）
* 用户反馈：当工具有参数时，不清楚要怎么配合输入
* 用户希望：有引导填写功能，减少学习成本
* 相关文件：`internal/config/params.go`

## Assumptions (temporary)

* 参数定义中可能包含可选值列表（需要确认参数结构）
* 当前实现可能没有显示可选值列表
* 当前实现可能不支持直接回车使用默认值

## Requirements (evolving)

* **用户选择：添加参数输入向导**
* **触发方式：总是使用向导**（每次执行工具时自动进入向导模式）
* **选项处理：直接输入**（显示可选值提示，用户直接输入选项值）
* **默认值处理：直接回车使用默认值**（用户可以直接按回车键使用默认值）
* 参数提示显示可选值列表（如果参数有固定选项）
* 保持交互式的简单性
* 减少用户的学习成本

## Acceptance Criteria (evolving)

* [ ] 参数提示显示可选值列表（如果参数有固定选项）
* [ ] 用户可以直接回车使用默认值（对于可选参数）
* [ ] 参数提示格式清晰易懂
* [ ] 必填参数验证正确
* [ ] 手动测试确认用户体验改进

## Definition of Done (team quality bar)

* Tests added/updated where appropriate
* Lint / typecheck / CI green
* Docs/notes updated if behavior changes
* Manual testing confirms improved UX

## Out of Scope (explicit)

* 确认提示改进（"Y" 不被接受的问题）— 留待后续任务
* 输入验证增强（无效值重新提示）— 当前保持简单报错
* 参数间依赖关系
* 复杂参数类型（数组、对象、文件路径选择）
* 编号选项列表（用户选择直接输入方式）

## Technical Approach

### 当前实现分析

从 `internal/config/params.go` 来看：
- `PromptMissing` 函数负责交互式参数输入
- `parameterPromptLabel` 函数生成参数提示标签
- 当前提示格式：`name description (类型=type, 必填/可选, 默认值=default)`

### 需要改进的点

1. **显示可选值列表**：
   - 需要检查参数定义结构是否包含可选值字段
   - 在提示中显示可选值列表（如：`可选值: text, json`）

2. **支持直接回车使用默认值**：
   - 当前实现：必填参数必须输入，可选参数可以跳过
   - 需要改进：可选参数且有默认值时，允许直接回车使用默认值
   - 提示格式：`请输入 [默认: text]:`

### 实现步骤

1. 检查参数定义结构（`config.Parameter`），确认是否有可选值字段
2. 修改 `parameterPromptLabel` 函数，添加可选值列表显示
3. 修改 `PromptMissing` 函数，支持直接回车使用默认值
4. 添加/更新测试
5. 手动测试验证

## Technical Notes

* 当前实现：`internal/config/params.go:19-68`
* 参数提示格式：`name description (类型=type, 必填/可选, 默认值=default)`
* 需要确认：`config.Parameter` 结构是否包含可选值字段

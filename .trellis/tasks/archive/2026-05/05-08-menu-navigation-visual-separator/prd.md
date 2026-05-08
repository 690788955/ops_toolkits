# 菜单导航选项视觉分隔

## Goal

在交互式菜单中，将导航选项（s) 搜索、b) 返回上级、q) 退出）与上面的工具/工作流列表做更明显的视觉分隔，提升菜单可读性。

## What I already know

* 当前菜单在显示工具/工作流列表后，直接显示导航选项，没有明显的视觉分隔
* 用户希望在列表和导航选项之间添加更明显的区分
* 菜单代码位于 `internal/menu/menu.go`
* 从之前的 CLI/menu UX 优化任务中，我知道菜单的显示逻辑在 `selectItem` 函数中

## Assumptions (temporary)

* 使用空行作为视觉分隔符
* 不改变菜单的功能逻辑，只改变显示格式

## Open Questions

* 无

## Requirements (evolving)

* 在工具/工作流列表和导航选项之间添加视觉分隔
* 分隔应该明显但不过于突兀
* 保持菜单的整体风格一致

## Acceptance Criteria (evolving)

* [ ] 在 `selectItem` 函数中，工具/工作流列表和导航选项之间添加空行分隔
* [ ] 在 `selectCategory` 函数中，分类列表和导航选项之间添加空行分隔
* [ ] 手动测试菜单显示，确认视觉分隔明显
* [ ] 不影响菜单的功能逻辑

## Definition of Done (team quality bar)

* 代码修改后运行 `go build` 和 `go test` 通过
* 手动测试菜单显示，确认视觉效果符合预期
* 修改代码文件后重建 graphify

## Technical Approach

在 `internal/menu/menu.go` 中：
1. 在 `selectItem` 函数中，在显示完工具/工作流列表后、显示导航选项前，添加一个空行输出
2. 在 `selectCategory` 函数中，在显示完分类列表后、显示导航选项前，添加一个空行输出

## Decision (ADR-lite)

**Context**: 当前菜单的工具/工作流列表和导航选项之间没有明显的视觉分隔，用户希望添加更明显的区分。

**Decision**: 在列表和导航选项之间添加一个空行作为视觉分隔符。

**Consequences**: 
* 菜单显示更清晰，用户更容易区分列表项和导航选项
* 菜单高度会增加一行，但不会影响使用体验
* 实现简单，不需要引入复杂的格式化逻辑

## Technical Notes

* 菜单显示使用 `fmt.Fprintln` 输出到 `io.Writer`
* 需要在两个函数中添加空行：`selectItem` 和 `selectCategory`
* 空行应该在显示完所有列表项后、显示第一个导航选项前输出

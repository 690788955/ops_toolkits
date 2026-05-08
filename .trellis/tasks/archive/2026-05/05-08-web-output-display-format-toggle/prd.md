# 移除 Web UI 结果区 text/json 展示切换

## Goal

移除 Web UI 运行结果日志块上的 text/json 切换按钮，避免用户误以为工具执行完成后可以无成本把真实输出格式从 text 切换成 json。结果区应只展示本次运行实际产生的 stdout/stderr 内容。

## Requirements

* `标准输出`、`错误输出`、`工作流日志` 的日志块不再显示 text/json 切换按钮。
* 日志块直接显示原始日志内容；无日志时显示 `无日志内容`。
* 不改变运行前参数表单中的 `format` 参数行为。
* 不改后端 API、不改运行记录结构、不重新执行工具。

## Acceptance Criteria

* [ ] 运行结果区不再出现 text/json 切换按钮。
* [ ] 日志标题仍正常显示。
* [ ] stdout/stderr/工作流日志仍直接显示原始文本。
* [ ] Web 前端构建通过。

## Definition of Done

* 前端构建通过。
* 浏览器中验证结果区不再显示 text/json 切换按钮。
* 若修改代码文件，完成 graphify rebuild。

## Technical Approach

恢复 `LogBlock` 为无状态展示组件：接收 `title` 和 `value`，渲染标题和 `<pre>` 内容。删除只服务于结果区格式切换的 CSS。

## Decision (ADR-lite)

**Context**: 结果区 text/json 切换只能对已返回字符串做展示层处理，不能把普通文本转换成真实 JSON，容易造成误解。

**Decision**: 移除结果区 text/json 切换控件，保留运行前 `format` 参数作为真实输出格式选择入口。

**Consequences**: UI 更清晰，不再暗示运行后可转换真实输出格式；后续如要降低输入成本，应把运行前 `format` 文本框改成按钮/单选控件。

## Out of Scope

* 不实现运行后重新执行或重新获取另一种格式。
* 不保存 text/json 两份输出。
* 不调整工具脚本输出格式协议。
* 不改运行前参数输入控件。

## Technical Notes

* 相关前端组件：`web/src/main.jsx` 的 `LogBlock`。
* 相关样式：`web/src/styles.css` 的日志块样式。
* 相关规范：`.trellis/spec/frontend/component-guidelines.md`、`.trellis/spec/frontend/quality-guidelines.md`。

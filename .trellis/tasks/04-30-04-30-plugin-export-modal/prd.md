# brainstorm: 插件导出改为模态框选择

## Goal

将插件管理中的“导出已安装插件”从当前内嵌列表，调整为更清晰的模态框/子视图选择体验：用户先点击“导出已安装插件”，再在弹出的选择界面中选择具体插件导出 ZIP，降低主插件管理弹窗的信息密度。

## What I already know

* 用户截图标注希望“导出已安装插件”区域改成动态框来选择。
* 现有 `PluginManagerModal` 已经是模态框，位于 `web/src/main.jsx`。
* 当前导出实现直接渲染 `exportablePlugins` 列表，每个插件通过 `/api/plugins/{pluginID}.zip` 下载。
* 已有“导出用户工作流插件”是独立下载入口 `/api/plugins/user-workflows.zip`。
* 后端插件导出 API 和 ZIP 契约已有规范，前端应只改变交互，不改后端协议。

## Assumptions (temporary)

* “动态框/模态框”指在点击“导出已安装插件”后打开一个选择界面，而不是把所有插件常驻展示在主弹窗里。
* MVP 只改 Web UI，不新增后端接口。

## Open Questions

* None.

## Requirements (evolving)

* 插件管理主弹窗保留“下载插件模板”“上传插件 ZIP”“导出用户工作流插件”。
* “导出已安装插件”改为一个入口按钮，点击后打开二级模态框选择具体已安装插件。
* 二级导出模态框展示已安装插件名称、ID、版本、描述，并提供导出动作。
* 二级导出模态框可关闭，关闭后仍回到插件管理主弹窗。
* 无可导出插件时展示中文空状态。

## Acceptance Criteria (evolving)

* [ ] 插件管理主弹窗不再常驻展示完整已安装插件导出列表。
* [ ] 点击“导出已安装插件”后打开二级模态框，可选择具体插件下载 ZIP。
* [ ] 二级模态框关闭时不关闭插件管理主弹窗。
* [ ] 每个导出链接仍使用 `/api/plugins/{encodeURIComponent(plugin.id)}.zip`。
* [ ] 无插件可导出时显示明确空状态。
* [ ] `npm run build --prefix web` 通过。

## Definition of Done

* Tests added/updated where applicable.
* Lint / typecheck / build green.
* Docs/spec updated if behavior contract changes.
* Rollback considered: UI-only change，可通过恢复组件结构回滚。

## Technical Approach

在 `PluginManagerModal` 内增加一个 `exportModalOpen` 状态。主弹窗只展示“导出已安装插件”入口按钮；点击后渲染独立的 `PluginExportModal` 二级模态框。二级模态框复用现有 `exportablePlugins` 数据和 `/api/plugins/{pluginID}.zip` 下载链接，不触碰后端协议。

## Decision (ADR-lite)

**Context**: 当前插件管理弹窗同时承载模板下载、插件上传、用户工作流导出和已安装插件列表，已安装插件列表会占用主弹窗空间。

**Decision**: 采用二级模态框作为已安装插件导出选择界面。

**Consequences**: 主弹窗更简洁，导出选择更聚焦；代价是存在模态框叠层，需要确保关闭二级模态框不会误关闭主弹窗，并保持遮罩点击行为可控。

## Out of Scope (explicit)

* 不修改后端插件导出 API。
* 不改变 ZIP 包结构或插件导入/导出安全策略。
* 不做批量导出、多选打包或导出进度管理。

## Technical Notes

* `web/src/main.jsx` lines 290-369: `PluginManagerModal` 当前管理上传和导出。
* `web/src/styles.css` contains modal and plugin export styles around `.modal`, `.pluginSecondaryOptions`, `.pluginExportPanel`, `.pluginExportItem`.
* `.trellis/spec/backend/plugin-import-export.md` defines plugin export API contract and must remain compatible.

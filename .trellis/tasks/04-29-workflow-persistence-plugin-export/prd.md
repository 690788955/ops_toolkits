# brainstorm: 插件导出与工作流迁移

## Goal

让平台支持把已安装插件导出为可再次导入的标准 ZIP 包；插件包应完整包含其 `plugin.yaml` 声明的工具、脚本、工作流和相关资源，从而补齐当前“只有导入、没有导出”的插件迁移闭环。

## What I already know

* 用户发现页面新建工作流重启后消失，并期望工作流应进入插件体系。
* 用户澄清：真正需求是平台级“导出插件”，导出内容包含插件自身贡献的工作流；当前平台已有插件导入/上传，但没有通用插件导出入口。
* 当前 `POST /api/workflows/{id}/save` 会调用 `workflowPath(reg, wf.ID)` 后写 YAML 文件。
* 对于不存在的工作流，`workflowPath` 默认写到 `workflows/<id>.yaml`；如果 `paths.workflows` 非空，则写到第一个配置目录。
* 当前 `configs/ops.yaml` 中 `paths.workflows: []`，导致重启后 `registry.discoverWorkflowEntries()` 不扫描根目录 `workflows/`。
* 平台已有 `POST /api/plugins/upload` 和 Web 插件管理弹窗上传插件 ZIP。
* 当前 Web 只有“导出用户工作流插件”固定入口 `/api/plugins/user-workflows.zip`，没有按插件选择导出任意已安装插件的能力。

## Assumptions (temporary)

* 页面新增工作流应优先成为“可迁移资产”，而不是只存在于运行时内存。
* 用户说的“新的 tag”可能指工作流 tags 字段，也可能包含分类 category；需要确认 UX。
* 插件导出应复用现有插件目录作为包根，保留相对路径结构，生成后应能被现有上传导入逻辑识别。
* 对包含工作流的插件，导出 ZIP 自然包含 `plugin.yaml` 引用的 `workflows/*.yaml` 和相关资源。

## Open Questions

* 无。

## Requirements (evolving)

* 支持从 Web 插件管理界面选择任意已安装插件并导出为 ZIP。
* 导出的 ZIP 必须包含该插件目录下的 `plugin.yaml`、工具脚本、workflow YAML 和其他插件内资源。
* 导出 ZIP 的目录结构必须符合现有上传导入约束：包内恰好一个插件根，且能被 `findUploadedPluginRoot` 识别。
* 插件导出接口应拒绝不存在的插件 ID，并避免路径逃逸或导出插件目录外文件。
* 保留“导出用户工作流插件”作为快捷入口或在通用插件导出列表中呈现 `user.workflows`。
* 页面保存的新工作流默认保存到固定“用户工作流插件”，例如 `plugins/user.workflows/`。
* 保存时自动维护用户工作流插件的 `plugin.yaml contributes.workflows`，保证重启后通过插件加载。
* 用户工作流插件支持导出为 ZIP，迁移到其他环境后可通过现有插件上传安装。
* 页面新增/编辑工作流时支持设置 tags，可选择已有 tag，也可输入新 tag。
* 支持“全局工作流”：工作流不绑定某一个插件工具来源，可跨分类编排所有可见工具。
* 支持“插件/分类工作流”：直接从某个分类上下文创建或编辑时，只能选择当前分类下的工具。

## Acceptance Criteria (evolving)

* [ ] Web 页面保存一个新工作流后，重启服务仍能在工作流列表看到它。
* [ ] 新建/编辑工作流时可以选择已有 tag，也可以输入新 tag，并保存到 YAML。
* [ ] 全局工作流编辑器可选择所有分类的工具。
* [ ] 分类/插件上下文中的工作流编辑器只允许选择当前分类工具。
* [ ] Web 插件管理界面展示可导出的已安装插件列表。
* [ ] 用户选择任意插件后可下载对应插件 ZIP。
* [ ] 导出的 ZIP 可被现有插件上传/安装流程识别。
* [ ] 导出包含插件内 workflow YAML，导入后工作流 ID、名称、分类、tags、节点和边保持一致。
* [ ] 保存/导出失败时给出清晰错误提示，不静默丢失。

## Definition of Done (team quality bar)

* Tests added/updated (unit/integration where appropriate)
* Lint / typecheck / CI green
* Docs/notes updated if behavior changes
* Rollout/rollback considered if risky

## Out of Scope (explicit)

* 暂不设计复杂插件市场或远程同步。
* 暂不实现跨环境工具依赖自动修复；导入环境仍需已有对应外部依赖。
* 暂不支持把多个插件合并导出为一个包。
* 暂不支持只选择插件内部分工具/工作流导出。

## Research References

* [`research/workflow-plugin-export.md`](research/workflow-plugin-export.md) — 现有仓库最贴合“单插件 ZIP + plugin.yaml contributes.workflows[].path + 插件内 workflows/*.yaml”的可迁移资产模式。

## Research Notes

### What similar tools do

* n8n/Node-RED/GitHub Actions 都把 workflow/flow 作为数据文件导入导出，节点/工具依赖必须在目标环境存在。
* VS Code/Node-RED 插件体系都依赖 manifest 声明贡献项，包内文件由 manifest 引用，便于安装、迁移和版本管理。

### Constraints from our repo/project

* 当前项目已明确 plugin-first，根目录 `workflows/` 是 legacy，不应继续作为新资产默认落点。
* 现有插件上传已经支持单插件 ZIP，并要求更新已有插件时提升 version。
* `plugin.yaml` 的 workflow contribution 目前只声明 path，分类、tags、名称等元数据应保存在 workflow YAML 中。

### Feasible approaches here

**Approach A: 通用插件导出（Chosen）**

* How it works: Web 插件管理界面列出已安装插件；用户选择插件后调用 `/api/plugins/{id}.zip` 或等价接口，服务端把对应插件目录打包成单插件 ZIP，目录结构满足现有上传导入约束。
* Pros: 补齐导入/导出闭环，适用于工具插件和包含工作流的插件，符合 plugin-first，迁移路径清晰。
* Cons: 需要做插件 ID 到目录的安全映射、ZIP 路径安全处理、前端插件选择 UI。

**Approach B: 仅导出用户工作流插件**

* How it works: 页面保存的新工作流默认写入一个固定的用户插件，例如 `plugins/user.workflows/`；导出时只打包这个插件。
* Pros: 可快速覆盖用户自建工作流迁移。
* Cons: 不能解决平台“只有导入没有导出”的通用插件迁移问题。

**Approach C: 修复 legacy workflows 目录 + 另做导出**

* How it works: 将 `configs/ops.yaml paths.workflows` 改为包含 `workflows`，页面继续保存到根目录 `workflows/`；导出时临时生成插件 ZIP。
* Pros: 改动小，能快速解决重启丢失。
* Cons: 违背 plugin-first 结构，后续迁移/分类/版本管理会继续分裂。

## Decision (ADR-lite)

**Context**: 当前平台已有插件 ZIP 上传/导入能力，但 Web 插件管理缺少通用导出入口；同时用户工作流也需要通过插件包迁移。

**Decision**: 实现通用插件导出：用户可在 Web 插件管理中选择任意已安装插件并下载标准插件 ZIP；`user.workflows` 作为普通插件同样可导出。

**Consequences**: 插件导入/导出形成闭环，工作流迁移自然复用插件机制；需要保证插件目录定位和 ZIP 打包安全，首版不做多插件合并或部分资源选择。

## Technical Notes

* `internal/server/server.go:799` `handleWorkflowSave` 处理页面保存。
* `internal/server/server.go:1007` `saveWorkflow` 将 workflow YAML 落盘。
* `internal/server/server.go:1024` `workflowPath` 对新 workflow 默认使用 `workflows/<id>.yaml`。
* `configs/ops.yaml:5` 当前 `paths.workflows: []`，重启加载时不会扫描根目录 workflow。
* `internal/registry/registry.go:171` `loadWorkflows` 只扫描 `Root.Paths.Workflows` 或 root config 显式 workflow refs。
* `internal/registry/registry.go:150` 插件 workflow 从 `plugin.yaml contributes.workflows` 指向的路径加载。
* `internal/plugin/types.go:41` 插件 workflow 清单目前只有 `path` 字段。

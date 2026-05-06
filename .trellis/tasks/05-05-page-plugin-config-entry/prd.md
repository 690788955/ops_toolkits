# 页面维护插件配置文件入口

## Goal

在 Web 页面增加维护插件宿主配置文件的入口，让用户可以在页面上查看、编辑并保存 `configs/plugins/<plugin-id>.yaml`，配合已实现的插件分层配置能力使用，降低手工改 YAML 文件的成本。

## What I already know

* 用户希望“页面上维护配置文件”，并询问能否增加入口。
* 当前框架已支持插件分层配置，宿主插件配置文件固定读取 `configs/plugins/<plugin-id>.yaml`。
* 该能力应是通用插件配置维护入口，不应只服务 `lzyh.es_backup`。
* Web 当前是单页 React 应用，插件入口在侧边栏 `+` 打开 `PluginManagerModal`。
* `PluginManagerModal` 已有插件上传、导出、禁用、启用、删除能力，并在已安装插件列表中逐个渲染操作按钮。
* 后端已有 `/api/plugins/` catch-all、插件上传/导出/禁用/启用/删除等路由，并有 `serverState.swap` 支持刷新 registry。

## Assumptions (temporary)

* MVP 维护的是宿主侧插件配置：`configs/plugins/<plugin-id>.yaml`，不是插件包内 `plugins/<plugin-id>/configs/default.yaml`。
* 页面入口优先挂到插件管理弹窗的“已安装插件”列表，作为每个插件条目的“配置”按钮。
* 保存后应触发后端重新加载 registry，让运行工具立即使用新配置。

## Open Questions

* 页面入口范围：只编辑插件宿主配置，还是同时也编辑全局 `configs/ops.yaml` 的 `config_defaults`？

## Requirements (evolving)

* Web 页面提供插件配置维护入口。
* 支持读取、编辑、保存 `configs/plugins/<plugin-id>.yaml`。
* 保存后服务端校验 YAML 格式，写入安全路径并刷新运行时配置。
* 入口对所有插件通用。
* 已安装插件列表中每个插件提供“配置”操作。

## Acceptance Criteria (evolving)

* [ ] 用户能从页面进入某个插件的配置编辑界面。
* [ ] 无配置文件时页面能显示空模板或空 YAML，并可保存创建。
* [ ] YAML 语法错误不能写入文件，页面显示错误。
* [ ] 保存成功后 `opsctl` 后端重新加载配置，后续工具运行使用新值。
* [ ] API/后端测试覆盖安全路径、读写、错误回滚或失败处理。
* [ ] 前端构建通过，插件管理弹窗内能打开/关闭配置编辑器。

## Definition of Done (team quality bar)

* Tests added/updated (unit/integration where appropriate)
* Lint / typecheck / CI green
* Docs/notes updated if behavior changes
* Rollout/rollback considered if risky

## Out of Scope (explicit)

* 暂不直接修改插件包内自带默认配置 `plugins/<plugin-id>/configs/default.yaml`。
* 暂不做复杂表单 schema 生成，除非后续确认需要。
* 暂不做权限系统和审计日志，当前项目尚未引入登录/权限。

## Technical Approach

推荐先做“插件宿主配置编辑器”：

* 后端新增插件配置 API：
  * `GET /api/plugins/<plugin-id>/config`：读取 `configs/plugins/<plugin-id>.yaml`，不存在返回空内容和目标路径。
  * `PUT /api/plugins/<plugin-id>/config`：接收 YAML 文本，校验 YAML 语法与安全 plugin ID，写入 `configs/plugins/<plugin-id>.yaml`，然后重新 `registry.Load` 并 `state.swap`。
* 前端在 `PluginManagerModal` 的每个插件条目增加“配置”按钮。
* 点击后打开嵌套弹窗/编辑区，展示 YAML textarea、保存/取消、保存状态。
* 配置入口对启用/禁用插件都可见；禁用插件也可以预先维护宿主配置。

## Decision Options

**A. 只维护插件宿主配置（推荐）**

* 范围：`configs/plugins/<plugin-id>.yaml`。
* 优点：最贴合刚实现的分层配置能力；安全边界清晰；不会误改全局运行配置。
* 缺点：全局 `config_defaults` 仍需手工维护。

**B. 同时做插件配置 + 全局配置默认值**

* 范围：插件配置入口 + `configs/ops.yaml` 中 `config_defaults` 的页面编辑。
* 优点：配置链路更完整。
* 缺点：更容易误改主配置；需要更强的 UI 提示和回滚测试。

**C. 做完整配置中心**

* 范围：全局配置、插件宿主配置、可能还包括工具参数模板。
* 优点：长期体验最好。
* 缺点：会明显扩大任务，涉及导航、权限/审计、表单模型和更多安全边界。


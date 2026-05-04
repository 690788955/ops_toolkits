# brainstorm: 插件安全删除

## Goal

为插件管理增加删除能力，并采用“先禁用、再删除”的安全流程，避免用户误删正在启用、仍可能被工具/工作流引用的插件。

## What I already know

* 用户希望插件支持删除。
* 用户倾向于删除前先禁用插件。
* 项目已有插件上传、导出能力和插件加载/禁用配置语义。
* `configs/ops.yaml` 已有 `plugins.disabled: []`，loader 会按插件目录名或 manifest ID 跳过禁用插件。
* 当前 Web 插件管理只支持模板下载、ZIP 上传、用户工作流导出、已安装插件导出。
* 用户反馈：禁用后应该支持重新启用。
* 用户反馈：删除插件后分类仍残留，没有删干净。
* 当前实现已有禁用/删除 API，但没有启用 API。
* 当前 `SaveRoot`/配置写回路径需要确保只持久化磁盘配置字段，不能把 registry 运行期展示分类写入 `configs/ops.yaml menu.categories`，否则删除插件后分类会残留。

## Assumptions (temporary)

* 插件删除是本地文件系统操作，目标为 `plugins/<plugin-dir>/` 目录。
* “先禁用再删除”应作为后端强制约束，而不是只在 UI 上提示。
* 删除插件不应影响内置配置、其他插件目录、运行日志或历史执行记录。
* MVP 做单插件禁用/启用/删除，不做批量删除、回收站或依赖图分析。
* 删除后可以从 `plugins.disabled` 移除该插件 ID，避免留下不可见 tombstone；删除失败时必须保留禁用配置。

## Open Questions

* None — user confirmed the forced two-step flow on 2026-05-04.

## Requirements (evolving)

* 插件管理需要支持禁用、启用和删除插件。
* 删除前必须先禁用插件；后端拒绝删除未禁用插件。
* 禁用插件应写入 `configs/ops.yaml` 的 `plugins.disabled`，刷新 registry 后该插件贡献的工具/工作流不再出现在 catalog 中。
* 启用插件应从 `configs/ops.yaml` 的 `plugins.disabled` 移除对应插件 ID/目录名，刷新 registry 后该插件贡献重新出现在 catalog 中。
* 禁用插件对应的侧边栏分类应保留显示但置灰，提示该插件已禁用，避免用户误以为分类丢失。
* 删除插件应只允许删除已禁用、可定位到配置插件根目录内的插件目录。
* 删除应有明确确认，错误信息使用中文且可读。
* 删除成功后插件目录不再存在，插件不再出现在 catalog 中。
* 删除成功后清理 `plugins.disabled` 中对应插件 ID/目录名，避免配置残留；删除失败时不清理禁用项。
* 删除成功后该插件独有分类不应继续出现在 catalog 或 `configs/ops.yaml menu.categories` 中。

## Acceptance Criteria (evolving)

* [x] 前端插件管理入口列出已安装插件，并能对启用插件执行“禁用”。
* [x] 前端对已禁用插件显示“启用”和“删除”操作，并在删除前二次确认。
* [x] 禁用插件对应的分类在侧边栏保留显示并置灰，不能进入正常可用状态。
* [x] 后端禁用 API 将插件 ID 加入 `configs/ops.yaml plugins.disabled`，刷新 registry 后 catalog 不再显示该插件贡献。
* [x] 后端启用 API 将插件 ID/目录名从 `configs/ops.yaml plugins.disabled` 移除，刷新 registry 后 catalog 重新显示该插件贡献。
* [x] 后端删除 API 拒绝删除未禁用插件，返回可读错误。
* [x] 后端删除已禁用插件目录后，catalog/validation 不再加载该插件。
* [x] 删除已禁用插件后，该插件独有分类不再出现在侧边栏或 `configs/ops.yaml menu.categories`。
* [x] 删除路径受限在配置的 `plugins.paths` 下，不能路径穿越或删除非插件目录。
* [x] 删除失败时不破坏插件禁用配置或其他插件目录。
* [x] 相关 Go 测试覆盖禁用成功、启用成功、删除成功、未禁用拒绝、未知插件、unsafe ID/path 错误、删除后分类不残留。

## Definition of Done (team quality bar)

* Tests added/updated for backend deletion behavior.
* `GOTOOLCHAIN=local go test ./...` 通过。
* 前端构建 `npm run build --prefix web` 通过（如涉及 UI）。
* graphify 重建。
* Docs/spec 更新判断完成。

## Research References

* [`research/plugin-delete-safety.md`](research/plugin-delete-safety.md) — WordPress/Drupal 等常见插件系统倾向先 deactivate/uninstall，再删除目录；本仓库可复用 `plugins.disabled` 作为禁用门槛。

## Research Notes

### What similar tools do

* WordPress 管理后台要求先停用插件，再允许删除，并在服务端拒绝删除启用插件。
* Drupal 模块流程也是先卸载/停用，再移除代码目录。
* Azure CLI 扩展删除偏 CLI 直删，但会先解析已安装扩展元数据并拒绝系统/开发模式扩展。

### Constraints from this repo

* 当前 loader 已支持 `plugins.disabled` 按目录名或插件 ID 禁用。
* 当前 catalog 只列出 registry-known 插件；禁用后插件不会出现在当前 `data.plugins[]`，因此 UI 若要展示已禁用插件，需要后端 catalog 或单独 API 暴露 disabled installed plugins。
* 当前导出已具备插件 ID 校验和“插件目录必须位于配置 plugin roots 内”的安全边界，删除应复用同等边界。
* 修改 `configs/ops.yaml` 需要新增配置写回逻辑；当前写 YAML 的示例主要在工作流保存路径。

### Feasible approaches here

**Approach A: 强制两步：禁用 API + 删除 API（推荐）**

* How it works: `POST /api/plugins/{id}/disable` 写入 `plugins.disabled` 并刷新 registry；`DELETE /api/plugins/{id}` 只删除已禁用插件目录，成功后清理 disabled 项并刷新 registry。
* Pros: 符合用户偏好和 WordPress/Drupal 安全惯例；后端可强制安全门槛；误删风险低。
* Cons: 需要 catalog/API 能让 UI 看见已禁用插件，否则禁用后用户找不到删除入口。

**Approach B: 单删除 API 内部自动禁用再删除**

* How it works: UI 只点“删除”，后端先写 disabled、刷新 registry，再删除目录。
* Pros: 操作少。
* Cons: “先禁用”不可见，用户没有观察禁用后的影响窗口；如果禁用成功但删除失败，状态较难解释。

**Approach C: 只做删除 API，UI 先确认**

* How it works: 后端直接删除 registry-known 插件目录。
* Pros: 最简单。
* Cons: 不满足用户“最好先禁用再删除”的安全意图，且破坏性更强。

## Technical Approach

推荐采用 Approach A：后端强制两步，前端以插件管理模态框呈现启用/禁用状态和下一步操作。

Planned backend shape:

* `POST /api/plugins/{pluginID}/disable`：禁用插件。
* `POST /api/plugins/{pluginID}/enable`：启用已禁用插件。
* `DELETE /api/plugins/{pluginID}`：删除已禁用插件。
* `GET /api/catalog` 的 `data.plugins[]` 增加 `disabled: boolean`，并包含已安装但禁用的插件，以便 UI 能在禁用后继续显示启用/删除入口。

Safety rules:

* `pluginID` 必须通过与导出相同的安全 ID 校验。
* 插件目录必须从已安装插件元数据/禁用插件扫描中解析，不能直接拼接请求路径作为删除目标。
* symlink-resolved 插件目录必须严格位于 configured plugin root 之下。
* 删除只删除插件目录，不删除运行日志、用户工作流插件导出、历史记录或插件产生的数据。

## Out of Scope (explicit)

* 不做批量删除插件。
* 不做插件回收站/恢复。
* 不删除运行日志或历史执行记录。
* 不实现用户权限/登录鉴权。
* 不做插件依赖图分析或引用扫描阻断；MVP 通过禁用后 catalog 消失来暴露影响。

## Technical Notes

* Relevant backend files: `internal/server/plugin_upload.go`, `internal/server/server.go`, `internal/server/server_test.go`, `internal/config/load.go`, `internal/config/types.go`, `internal/plugin/load.go`。
* Relevant frontend files: `web/src/main.jsx`, `web/src/styles.css`。
* Relevant specs: `.trellis/spec/backend/plugin-import-export.md`, `.trellis/spec/backend/directory-structure.md`。

# brainstorm: 导出插件解压即用

## Goal

让导出的插件支持两种交付形态：一种是保持现有导入兼容的标准插件 ZIP；另一种是包含 `opsctl`、必要基础配置和该插件的可部署运行包，让用户在服务器上解压后即可用 `opsctl` 加载并运行该插件工具。

## What I already know

* 用户希望“导出的插件是解压后可以直接当工具给服务器使用的”。
* 用户选择方案 3 的方向，并补充希望导出时可选择：既能导出纯插件，也能导出包含 `opsctl` 的插件包。
* 当前平台已是 plugin-first，运行时插件默认位于 `plugins/<plugin-id>/`。
* 现有通用插件导出接口是 `GET /api/plugins/{pluginID}.zip`，由 Web 插件管理下载使用。
* `.trellis/spec/backend/plugin-import-export.md` 规定当前导出 ZIP 结构为“恰好一个插件包根目录”，顶层目录名是插件目录 basename，`plugin.yaml` 在该根目录内。
* 现有设计重点是“导出包可被上传导入逻辑识别”，不是“从服务器根目录解压即可落到 plugins/ 下”。
* `buildPluginExportZip(reg, pluginID)` 当前从 registry 已知插件定位目录并打包插件目录，禁止读取目录外文件。
* `buildUserWorkflowPluginZip(reg)` 当前把 `user.workflows/` 作为 ZIP 顶层目录打包。
* 当前 `internal/packagebuild.Build` 会构建全量离线包：复制 `configs/`、`plugins/` 和当前可执行文件到 `dist/opsctl/`，再生成 `dist/opsctl.tar.gz`。
* GitHub Actions 已构建多平台二进制，命名为 `opsctl/bin/opsctl_<goos>_<goarch>[.exe]`，例如 `opsctl_linux_amd64`、`opsctl_windows_amd64.exe`。

## Assumptions (temporary)

* “解压后直接使用”至少应满足：包内存在完整 `plugin.yaml`、脚本、workflow YAML 和插件本地资源；标准插件包解压到 `plugins/` 后可加载，含 `opsctl` 运行包在服务器应用目录解压后可直接执行 `bin/opsctl`。
* 含 `opsctl` 的插件包应是“单插件运行包”，不是当前 `opsctl package build` 那种包含全部插件的全量环境包。
* 含 `opsctl` 的单插件运行包采用生成的最小配置，而不是复制当前完整 `configs/`。
* 运行包应支持选择目标平台，服务端从当前运行 base 目录的 `bin/opsctl_<goos>_<goarch>[.exe]` 解析本地多平台 `opsctl` 二进制产物；如果缺少目标平台二进制，应返回清晰错误而不是现场构建。
* 服务器仍需要具备插件声明所需的外部运行时依赖，例如系统命令、Ansible、Java、Python 包等；插件包不自动安装这些依赖。

## Open Questions

* None.

## Requirements (evolving)

* 导出的插件 ZIP 应保持单插件包完整性，包含插件目录内的 `plugin.yaml`、脚本、workflow YAML、README 和其他插件本地资源。
* Web 插件导出时提供两种下载选择：标准插件包、含 `opsctl` 的单插件运行包。
* 标准插件包继续保持现有上传/导入兼容结构：ZIP 顶层为单个插件目录。
* Web 插件导出采用一个“导出”按钮 + 下拉菜单展示 3 个下载动作，避免插件列表拥挤。
* 含 `opsctl` 的单插件运行包应同时提供 `.tar.gz` 和 `.zip` 两种格式。
* 含 `opsctl` 的单插件运行包应包含从服务端 base 目录 `bin/opsctl_<goos>_<goarch>[.exe]` 找到的目标平台 `opsctl` 可执行文件、生成的最小 `configs/ops.yaml`、以及仅被选择的插件目录。
* 最小 `configs/ops.yaml` 应保留应用名称/基础 server 配置、`paths.runs`、`paths.logs`、`plugins.paths: [plugins]`、`plugins.strict`、空 disabled/allowed_commands、空 host config 白名单等必要默认项；不复制本机业务配置或其他插件配置。
* 含 `opsctl` 的单插件运行包应包含 `README.md` 使用说明，写明解压、运行 `bin/opsctl validate`、`bin/opsctl list`、`bin/opsctl start/serve` 的基本步骤，以及外部依赖需在目标服务器自行安装。
* 含 `opsctl` 的单插件运行包解压后，用户可在包根目录执行 `bin/opsctl validate`、`bin/opsctl list` 或启动服务来加载该插件。
* 导出包解压后应能按插件加载器约定被服务器识别。
* 导出不得包含插件目录外文件、宿主绝对路径配置文件目标或运行日志。
* 导出行为应继续满足现有上传/导入安全契约。
* 用户界面或下载说明应明确“解压到哪里可直接使用”。

## Acceptance Criteria (evolving)

* [ ] 标准插件 ZIP 解压后能得到完整插件目录，目录内包含 `plugin.yaml` 和插件本地资源。
* [ ] 标准插件 ZIP 仍可被现有 `POST /api/plugins/upload` 导入流程识别。
* [ ] 含 `opsctl` 的单插件运行包可分别下载 `.tar.gz` 和 `.zip` 两种格式。
* [ ] Web 可选择目标平台，服务端从 base 目录下的 `bin/opsctl_<goos>_<goarch>[.exe]` 读取对应 `opsctl`。
* [ ] 缺少目标平台二进制时，下载失败并显示清晰中文错误。
* [ ] 含 `opsctl` 的单插件运行包包含 `bin/<opsctl>`、生成的最小 `configs/ops.yaml` 和 `plugins/<selected-plugin>/`。
* [ ] 含 `opsctl` 的单插件运行包不包含其他未选择插件。
* [ ] 含 `opsctl` 的单插件运行包解压后，执行 `bin/opsctl validate` 和 `bin/opsctl list` 能识别所选插件的工具/工作流。
* [ ] 含 `opsctl` 的单插件运行包使用最小配置，不复制当前完整 `configs/` 或其他插件配置。
* [ ] 含 `opsctl` 的单插件运行包包含中文 `README.md`，说明解压、验证、列出工具、启动服务和外部依赖要求。
* [ ] Web 中每个可导出插件显示一个“导出”按钮，点击后出现 3 个下载选项。
* [ ] 三个下载选项使用清晰中文文案区分“标准插件包 ZIP”“含 opsctl 运行包 tar.gz”“含 opsctl 运行包 ZIP”。
* [ ] 两种导出都不包含插件目录外文件或 host absolute config mapping 的目标文件。

## Definition of Done (team quality bar)

* Tests added/updated (unit/integration where appropriate).
* Lint / typecheck / CI green.
* Docs/notes updated if behavior changes.
* Rollout/rollback considered if risky.

## Out of Scope (explicit)

* 不自动安装插件依赖的系统命令、语言运行时或第三方包。
* 不把多个插件合并为一个 ZIP。
* 不做插件市场、远程同步或自动发布。
* 不导出运行日志、服务端私有配置或插件目录外文件。
* 不做包含所有插件的全量环境包；全量分发继续由 `opsctl package build` 负责。

## Technical Notes

* `graphify-out/GRAPH_REPORT.md` shows plugin export/import code concentrated around server community nodes such as `buildPluginExportZip()`, `buildUserWorkflowPluginZip()`, and catalog handlers.
* `.trellis/spec/backend/plugin-import-export.md` defines current ZIP/export/import contract and safety matrix.
* `.trellis/tasks/04-29-workflow-persistence-plugin-export/prd.md` chose通用插件导出，并明确导出包可被现有上传/安装流程识别。
* `.trellis/tasks/04-30-04-30-plugin-export-modal/prd.md` kept Web export link as `/api/plugins/{encodeURIComponent(plugin.id)}.zip` and did not change backend ZIP contract.
* `internal/server/plugin_upload.go:592` `buildPluginExportZip` validates plugin ID, requires registry-known installed plugin, checks configured plugin root, then zips plugin dir.
* `internal/server/server.go:1172` `pluginDownloadHandler` maps `/api/plugins/{pluginID}.zip` to `buildPluginExportZip`.
* `internal/server/server.go:1598` `buildUserWorkflowPluginZip` packages `user.workflows/` as a ZIP top-level directory.
* `internal/packagebuild/packagebuild.go:11` `Build` 当前复制全量 `configs/`、`plugins/` 和当前可执行文件，可作为单插件运行包打包逻辑参考，但不能直接复用全量插件复制行为。
* `internal/packagebuild/packagebuild_test.go:11` 覆盖现有全量交付包内容和 tar.gz 条目。
* `.github/workflows/build.yml:29` 构建 `linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64`，输出到 `opsctl/bin/opsctl_<goos>_<goarch>[.exe]`。
* `configs/ops.yaml:18` 当前 `plugins.paths` 指向 `plugins`，适合作为运行包内默认插件加载目录。
* 运行包配置决策：采用生成的最小 `configs/ops.yaml`，避免泄露或耦合本机完整配置。
* 运行包二进制来源决策：从服务端 base 目录解析本地多平台构建产物，不新增配置项；路径约定为 `bin/opsctl_<goos>_<goarch>[.exe]`。

## Candidate Approaches

### Approach A: 双导出选项（Chosen / Recommended）

* How it works: Web 插件导出为每个插件提供一个“导出”按钮，点击后通过下拉菜单展示三个下载动作：下载“标准插件包 ZIP”、下载“含 opsctl 运行包 tar.gz”、下载“含 opsctl 运行包 ZIP”。标准插件包继续使用现有 `/api/plugins/{pluginID}.zip`；运行包使用新增导出路径，包内结构类似 `opsctl/bin/<exe>`、`opsctl/configs/ops.yaml`、`opsctl/plugins/<plugin-dir>/...`，但只包含被选择的插件，并生成最小配置。
* Pros: 同时满足“可上传导入”和“服务器解压后直接运行”两个场景；不会破坏现有插件 ZIP 契约；比全量 `package build` 更聚焦。
* Cons: 需要新增后端打包函数/API、前端下载入口、目标平台参数校验、本地二进制缺失错误处理和运行包测试。

### Approach B: 只增强标准插件包文案

* How it works: 不新增包格式，只告诉用户把标准插件 ZIP 解压到服务器 `plugins/` 目录。
* Pros: 实现最小，完全复用现有契约。
* Cons: 不包含 `opsctl`，无法满足用户希望“包含 opsctl 的插件包”的要求。

### Approach C: 复用全量 `opsctl package build`

* How it works: 直接让用户运行或下载现有全量包，里面包含全部 `configs/`、全部 `plugins/` 和可执行文件。
* Pros: 已有实现，最接近完整环境迁移。
* Cons: 粒度过大，会把未选择插件一起带出，不符合按插件导出的产品语义。

## Decision (ADR-lite)

**Context**: 标准插件 ZIP 已能被上传导入识别，但用户还需要一种在服务器上解压后即可带 `opsctl` 使用的交付物。

**Decision**: 保留标准插件包，同时新增“含 `opsctl` 的单插件运行包”导出选项；运行包只包含被选插件、生成的最小配置、README 和所选目标平台的 `opsctl` 可执行文件，并同时提供 `.tar.gz` 与 `.zip` 两种格式，不替代全量 `opsctl package build`。

**Consequences**: 产品上覆盖两类迁移场景；技术上需要维护两种导出格式和清晰命名，避免用户把运行包误当作上传导入插件 ZIP。

# 支持 config_files 绝对路径映射

## Goal

支持运维人员把工具配置文件映射到宿主机器上的绝对路径，使 Web/API 可以查看和修改这些机器本地配置文件，同时保留明确的应用层授权、OS 权限检查和路径安全边界。

## What I already know

* 用户目标：`config_files` 希望支持绝对路径，可以指定机器上的任意位置做映射。
* 用户补充方案：可以在框架设置里配置白名单；只要路径落在白名单内，就允许在 mapping 中指定。
* 当前 `plugin.yaml` 的 `tools[].config_files` 是字符串列表，校验为插件目录内相对路径。
* 当前 `configs/plugins/<plugin-id>.mapping.yaml` 已支持宿主侧按工具整体覆盖 `config_files`，但也只允许插件内相对路径。
* 当前 Web/API 通过 `/api/plugins/{pluginID}/files` 和 `/api/plugins/{pluginID}/files/{filename}` 列出、读取、保存、删除配置文件。
* 用户新增 config_dir 需求：类似 `config_files`，可指定 `config_dir` 作为基准目录；`config_files` 里的条目可以是文件名或目录相对项，最终只与 `config_dir` 拼接。

## Research References

* [`research/absolute-path-config-mapping.md`](research/absolute-path-config-mapping.md) — 推荐把宿主绝对路径作为宿主侧显式授权能力，使用结构化条目、按声明 ID 访问、区分应用层读写权限和 OS 权限。

## Requirements (evolving)

* 保留插件包默认安全边界：`plugin.yaml` 中的普通字符串 `config_files` 仍表示插件内相对路径。
* 在框架全局配置中增加宿主配置文件目录白名单；白名单只支持目录前缀，不支持单文件白名单。
* 宿主侧 mapping 只有在目标绝对路径落入白名单目录时才允许注册和编辑。
* 白名单校验必须使用清理后的绝对路径，并考虑符号链接解析后的最终路径，避免通过软链逃逸授权目录。
* `config_dir` 默认值为插件目录下的 `config`；指定时可为插件内相对目录或白名单内宿主绝对目录。
* `config_files` 中的每个条目都按相对项处理，可以是文件名或带子目录的相对路径；禁止绝对路径和 `..` 逃逸。
* 最终配置文件路径只允许通过 `config_dir + config_files 条目` 得出，不允许 `config_files` 自己携带宿主绝对路径。
* 目录型 `config_files` 条目表示声明目录下的配置文件集合；MVP 可列出该目录下一级普通文件，不递归扫描。
* 每个绝对路径映射应至少表达：稳定 ID、显示名称、`config_dir` 基准目录、`config_files` 相对条目、访问模式（只读或读写）、是否允许创建。
* 插件模板和插件作者文档必须写清楚 `config_files` / `config_dir` / `host_absolute` / 白名单契约与安全边界，不能让插件模板默认获得宿主绝对路径能力；模板默认示例应使用推荐的 `config_dir: config` + 短文件名写法，旧 `config/example.conf` 写法仅作为兼容说明。
* 列表 API 应返回结构化文件状态，包括是否存在、是否可读、是否可写、失败原因。
* GET 读取前必须检查：已声明、应用层允许读取、目标是普通文件、当前进程有读取权限、文件大小在限制内。
* PUT 保存前必须检查：已声明、应用层允许写入、已存在文件为普通文件且可写；不存在时只有 `create: true` 才能创建，且父目录必须可写。
* 默认不支持删除宿主绝对路径文件，避免误删机器文件。
* 插件导出 ZIP 不应包含宿主绝对路径映射指向的文件。

## Acceptance Criteria (evolving)

* [x] 插件内相对路径 `config_files` 继续可列出、读取、保存。
* [x] 框架全局配置可声明宿主配置文件目录白名单。
* [x] `config_dir` 未指定时，默认使用插件目录下的 `config` 目录作为基准目录。
* [x] `config_dir` 指向白名单内宿主绝对目录时，`config_files` 相对文件项可映射到该目录下文件。
* [x] `config_files` 相对目录项可展开为该目录下一级普通文件，并拒绝递归逃逸。
* [x] 宿主 mapping 中白名单外的绝对路径被拒绝，并返回清晰错误。
* [x] 只读声明允许 GET，拒绝 PUT，并返回清晰错误。
* [x] 读写声明在 OS 权限允许时可以 PUT 保存。
* [x] 文件不存在且 `create: false` 时返回不可读/不可写状态，不自动创建。
* [x] 文件不存在且 `create: true` 且父目录可写时允许创建。
* [x] 目录、特殊文件、未声明 ID、URL 编码路径逃逸都不能被读取或写入。
* [x] Web 配置文件/插件配置区域按目录树展示插件内文件和宿主配置映射，并保留文件状态与查看/编辑入口。
* [x] 配置文件列表以目录树展示，便于查找和选择。
* [x] 目录树按插件根目录 `plugins/<pluginID>` 与宿主 `config_dir` 分组，目录默认展开并支持折叠。
* [x] 配置文件编辑页展示文件摘要、权限状态与不可保存原因。
* [x] Windows 绝对路径和当前平台路径语义有明确测试或说明。
* [x] 插件模板 `plugins/plugin.template/plugin.yaml` 默认示例使用安全的 `config_dir: config` + 相对 `config_files` 短文件名写法；旧 `config/example.conf` 写法仅作为兼容说明，不启用宿主绝对路径。
* [x] 插件模板 README 写清 manifest 只声明插件内配置、`config_dir` 默认/兼容规则、目录项一级展开、宿主绝对路径需管理员配置 `host_config_files.allowed_dirs` 与 host-side mapping、`access` / `create` 含义、白名单和 symlink 边界。
* [x] 插件模板提供 host-side `scope: host_absolute` mapping 示例供管理员参考，但不默认加载。
* [x] 本次插件模板规范补充已运行 `./bin/opsctl.exe validate`；未修改 Go 代码或 Web 前端，因此未运行 `go test ./...` / Web build。
* [x] `go test ./...`、Web build、`opsctl validate` 通过。

## Definition of Done

* Tests added/updated where behavior changes.
* Backend API contract and Web UI behavior aligned.
* Build/test/validate green.
* Spec updated for absolute host path mapping contract.
* Rollout/rollback considered because this enables editing host files.

## Technical Approach (recommended)

推荐采用“全局白名单 + 宿主 mapping 显式声明”的方案：

* `plugin.yaml` 保持插件内相对路径，避免插件包自带任意宿主文件访问能力。
* 在 `configs/ops.yaml` 增加宿主配置文件访问白名单，用来声明允许 Web/API 维护的宿主目录或文件范围。
* 扩展宿主侧 `configs/plugins/<plugin-id>.mapping.yaml`，增加结构化配置声明，例如 `config_dir/config_files/access/create/label`。
* Registry 解析配置声明时只把 `config_files` 当相对项，最终路径由 `config_dir` 拼接；host absolute `config_dir` 必须命中全局白名单。
* Registry 将旧字符串和新结构归一化为统一条目给 Server/UI 使用。
* Server 按声明 ID 解析文件，分别处理 `plugin` 和 `host_absolute` 两类路径。
* UI 从字符串列表升级为结构化列表，展示宿主路径、白名单状态、只读/可写状态和风险提示。

## Decision (ADR-lite, pending)

**Context**: 绝对路径文件编辑是宿主文件系统能力，风险高于插件包内文件编辑；直接把绝对路径写进 `config_files` 会让条目语义混乱。  
**Decision**: 采用“全局白名单 + `config_dir` 基准目录 + `config_files` 相对项”：插件包不能直接声明宿主绝对路径；管理员先在框架设置中配置允许目录，再在插件 mapping 中用 `config_dir` 指向白名单内目录，`config_files` 只声明该目录下文件或目录相对项。  
**Consequences**: 安全边界比任意绝对路径更清晰；`config_files` 始终是相对项，老插件兼容；需要新增全局配置字段、mapping 校验、目录项展开和 UI 状态展示。

## Out of Scope (proposed)

* 不支持单文件白名单，白名单粒度只做目录前缀。
* 不支持白名单外宿主路径，即使 mapping 显式声明也不允许访问。
* 不支持通过 URL 直接输入任意宿主路径进行读取/写入。
* 不支持删除宿主绝对路径文件。
* 不做配置文件格式校验/reload hook/服务重启编排。
* 不做多用户鉴权模型；权限检查基于当前 opsctl/server 进程的 OS 权限和声明权限。
* 不把宿主绝对路径文件打包进插件导出 ZIP。

## Technical Notes

* 插件模板 `plugins/plugin.template/plugin.yaml` 默认示例使用推荐的 `config_dir: config` + 短文件名写法，同时以注释说明旧 `config/example.conf` 写法兼容，不启用宿主绝对路径。
* `plugins/plugin.template/README.md` 补充配置文件功能规范，明确插件包不能直接声明宿主绝对路径，宿主路径能力必须由管理员在 `configs/ops.yaml` 与 `configs/plugins/<plugin-id>.mapping.yaml` 中显式启用。
* `plugins/plugin.template/examples/host-absolute.mapping.yaml` 是管理员参考示例，不会随插件模板自动启用宿主路径能力。
* 根 `README.md` 的插件规则同步补充配置文件契约摘要。

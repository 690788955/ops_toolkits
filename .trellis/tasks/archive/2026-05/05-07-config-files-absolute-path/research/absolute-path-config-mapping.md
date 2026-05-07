# Research: absolute-path config_files mapping

- **Query**: Research safe patterns for allowing an ops web UI/API to view and edit host absolute-path configuration files declared by plugins. Context: repo is F:\ccb\ops_toolkits; task dir is .trellis/tasks/05-07-config-files-absolute-path. Current behavior only allows plugin-local config_files; user wants absolute paths to arbitrary machine locations with read/write permission checks. Identify comparable patterns, recommended safety controls, and how to map them to this repo.
- **Scope**: mixed
- **Date**: 2026-05-07

## Findings

### Files Found

| File Path | Description |
|---|---|
| `internal/plugin/types.go` | 插件 manifest 与工具声明类型；`Tool.ConfigFiles []string` 当前只表达路径字符串，`SensitivePaths` 已存在但不参与 config_files 权限模型。 |
| `internal/plugin/load.go` | 插件加载校验；当前对 `config_files` 强制使用 `SafePath(pkg.Dir, cf)`，因此拒绝绝对路径与插件目录逃逸。 |
| `internal/registry/mapping.go` | 宿主侧 `configs/plugins/<plugin-id>.mapping.yaml` 映射规则校验；当前映射 `config_files` 同样调用 `plugin.SafePath(pkg.Dir, cf)`，因此也只能是插件内相对路径。 |
| `internal/registry/registry.go` | 注册表加载插件默认配置、宿主配置和 mapping；mapping 中某工具的 `config_files` 会整体覆盖插件 manifest 默认值。 |
| `internal/config/types.go` | `PluginConfigMapping` / `PluginToolConfigMapping` 类型目前仅包含 `ConfigFiles []string`，没有 mode、label、绝对路径权限、是否允许创建等结构化字段。 |
| `internal/server/server.go` | HTTP API 路由和 handler：`GET /api/plugins/{id}/files` 列表，`GET/PUT/DELETE /api/plugins/{id}/files/{filename}` 读取、保存、删除配置文件。 |
| `web/src/main.jsx` | Web UI 的插件配置文件列表与编辑器；当前文案和路径展示假设文件在 `plugins/{pluginID}/{file}` 下。 |
| `.trellis/spec/backend/plugin-import-export.md` | 已有插件导入/导出安全契约，可复用“ID 不是路径”“来源必须来自注册表元数据”“符号链接解析后检查范围”等安全思路。 |
| `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md` | Windows/Git Bash/WSL 路径差异指南；绝对路径能力需要兼容 Windows 盘符、POSIX 路径和 shell 边界。 |
| `.grill/plugin-config-mapping-design.md` | 早期设计明确曾拒绝“页面任意宿主路径写入”，并决定 mapping 存在宿主侧、按工具整体覆盖。 |

### Code Patterns

#### 1. 当前 `config_files` 的边界是“插件内相对路径”

- `internal/plugin/types.go:42`：插件工具声明字段是纯字符串数组：`ConfigFiles []string \`yaml:"config_files" json:"config_files"\``。
- `internal/plugin/load.go:143-153`：加载插件时逐个校验 `tool.ConfigFiles`，空字符串报错，`SafePath(pkg.Dir, cf)` 不通过则报“路径不安全”，已存在且为目录时报错。
- `internal/plugin/load.go:277-292`：`SafePath(root, rel)` 明确拒绝 `filepath.IsAbs(rel)`，然后把路径拼到 `rootAbs` 下并检查 `pathAbs` 必须等于根或以根目录前缀开头。
- `internal/plugin/load.go:242-255`：配置文件不存在不会阻断加载，而是生成 `CONFIG_FILE_MISSING` warning，建议“把该配置文件放入插件目录”。

含义：插件 manifest 里的 `config_files` 目前不只是 UI 声明字段，也是一个插件包本地资源边界。直接放开绝对路径会改变 loader、registry、server、UI 的共同契约。

#### 2. 宿主 mapping 已经是“宿主侧覆盖插件默认声明”的入口

- `internal/registry/registry.go:190-206`：注册表会读取 `configs/plugins/<plugin-id>.yaml` 宿主配置和 `configs/plugins/<plugin-id>.mapping.yaml` mapping。
- `internal/registry/registry.go:215-217`：如果 mapping 中存在某工具规则，`toolCfg.ConfigFiles = append([]string{}, rule.ConfigFiles...)`，即整体覆盖插件默认 `config_files`。
- `internal/registry/mapping.go:37-45`：mapping 只能引用当前插件 manifest 中已有工具 ID。
- `internal/registry/mapping.go:50-58`：mapping 中的 `config_files` 目前同样要求非空并通过 `plugin.SafePath(pkg.Dir, cf)`，所以仍不能是绝对路径。
- `internal/registry/plugin_test.go:243-290`：测试覆盖“mapping 未配置的工具不受影响”和“配置工具整体覆盖默认 config_files”。

含义：如果要允许机器本地绝对路径，比起修改插件包 `plugin.yaml`，宿主 mapping 是更接近现有设计的承载点；但现有类型太弱，无法表达只读/读写、创建策略、标签和权限检查结果。

#### 3. 当前 API 通过“声明匹配 + SafePath”防止任意文件访问

- `internal/server/server.go:1157-1172`：插件文件路由是 `/api/plugins/{id}/files` 和 `/api/plugins/{id}/files/{filename}`。
- `internal/server/server.go:1677-1699`：列表接口从 registry 中收集该插件所有工具的 `tool.Config.ConfigFiles` 并去重排序。
- `internal/server/server.go:1715-1731`：读取接口先调用 `declaredPluginConfigFilePath`，文件不存在时返回空内容，否则 `os.ReadFile`。
- `internal/server/server.go:1734-1758`：保存接口先解析 JSON `{content}`，再 `MkdirAll(filepath.Dir(filePath), 0755)`，最后 `os.WriteFile(filePath, ..., 0644)`。
- `internal/server/server.go:1761-1773`：删除接口同样解析声明路径后 `os.Remove(filePath)`。
- `internal/server/server.go:1776-1793`：`declaredPluginConfigFilePath` 必须找到 registry 中同插件声明的同名 `fileName`，再用 `plugin.SafePath(tool.Config.PluginConfig.Dir, declared)` 解析到插件目录内绝对路径。

含义：服务端当前安全核心是“不信任 URL 路径，只信任 registry 中已声明的文件名，并且最终落在插件目录内”。绝对路径能力应保留“请求只引用声明 ID/路径，不由 URL 任意传入宿主路径”的模式。

#### 4. 全局配置编辑已有“写入后 reload，失败回滚”的流程

- `internal/server/server.go:779-821`：`PUT /api/config` 先 YAML 解析校验，再写入 `configs/ops.yaml`，随后 `registry.Load`；如果 reload 失败，则写回旧内容或删除新文件并返回错误。
- `internal/config/load.go:25-42`：`SaveRoot` 用 YAML encoder 生成内容后 `os.WriteFile(filepath.Clean(path), ..., 0644)`，没有 atomic rename。

含义：对于需要影响运行时配置的文件，仓库已有“保存后重新加载并失败回滚”模式；对于任意宿主配置文件，是否需要 reload、是否能回滚、是否需要写前备份，需要作为文件声明的元数据或 API 语义明确。

#### 5. Web UI 当前把配置文件展示为插件目录路径

- `web/src/main.jsx:1034-1093`：`PluginConfigFilesPanel` 从 `/api/plugins/{pluginID}/files` 拉取字符串列表，显示说明“页面会直接读取和保存插件目录内对应路径的文件”，并渲染 `plugins/{pluginID}/{file}`。
- `web/src/main.jsx:1096-1151`：`PluginConfigFileEditor` 以 `fileName` 作为 URL segment，调用 `GET/PUT /api/plugins/{pluginID}/files/{fileName}`，只传 `{content}`。
- `web/src/main.jsx:437-479`：配置管理入口也从 catalog 的 `tool.config_files` 聚合，并展示为 `plugins/${plugin.id}/${file}`。

含义：如果 `config_files` 支持绝对路径，前端需要从“字符串路径列表”升级为带显示路径、权限、可读/可写状态、风险提示的结构化响应，否则 UI 会继续误导为插件内文件。

### Comparable Patterns

#### Docker bind mounts

External reference: [Docker docs — Bind mounts](https://docs.docker.com/engine/storage/bind-mounts/)

相关模式：Docker 将宿主路径显式声明为 bind mount。文档强调 bind mount 默认可写，容器内进程可以修改或删除宿主重要文件；可用 `readonly` 或 `ro` 选项阻止写入。

可映射点：

- 宿主绝对路径访问应是显式声明，而不是请求时自由传入。
- 权限应区分 `read-only` 和 `read-write`，默认更适合只读。
- UI/API 需要把“这是宿主文件系统访问”作为风险信息暴露出来。
- 读写行为应由声明控制，不能因为文件可被 OS 用户写入就自动允许 Web 保存。

#### Kubernetes hostPath volumes

External reference: [Kubernetes docs — Volumes / hostPath](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath)

相关模式：`hostPath` 允许 Pod 挂载节点文件或目录。文档提示即使只读也可能暴露特权系统凭据或 API；如果允许不可信 Pod 读写任意 host path，可能绕过安全边界。`hostPath.type` 还要求声明期望对象类型，例如 `File`、`FileOrCreate`、`Directory`、`DirectoryOrCreate` 等。

可映射点：

- 对 host absolute path 应声明对象类型：文件必须存在、文件可创建、目录不允许、特殊文件不允许。
- read-write 应比 read-only 更严格，且可单独要求确认。
- “路径存在并可读”不等于“安全可读”；系统凭据、runtime socket、密钥等路径应被显式阻断或要求更高权限。
- Windows 与 Linux 上同一声明可能行为不同，需要在校验结果里报告当前主机路径类型和权限。

#### OWASP Path Traversal

External reference: [OWASP — Path Traversal](https://owasp.org/www-community/attacks/Path_Traversal)

相关模式：路径遍历攻击通过 `../`、编码、双重编码、反斜杠等方式访问 web root 外文件。OWASP 建议不要直接使用用户输入做文件系统 API；如必须使用，应先规范化路径，并用隔离/访问策略限制可读写范围。

可映射点：

- URL 中不要直接承载任意绝对路径；应使用 registry/mapping 中的声明 ID 或索引解析路径。
- 对请求参数做解码后校验，处理 `/`、`\`、`%2e%2e`、双重编码、Windows 盘符、UNC 路径等。
- 即使绝对路径功能允许“任意机器位置”，也要把“任意”限制为“插件/宿主配置显式声明且通过策略检查的路径”。
- 对符号链接和最终路径做二次检查，避免声明路径看似安全但实际指向敏感位置。

#### OWASP File Upload / File Storage defense-in-depth

External reference: [OWASP Cheat Sheet — File Upload](https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html)

相关模式：文件写入类能力没有单一防线，建议多层防护：扩展名/类型/内容校验、文件名安全、存储位置、用户权限、文件系统权限、大小限制等。

可映射点：

- 配置文件编辑也属于“用户通过 Web 写文件”，应限制大小、类型/扩展名或允许的文本格式。
- 保存内容前可做编码/文本校验，避免二进制或超大文件写入。
- 文件系统权限检查和应用层声明权限都要通过，缺一不可。
- 错误信息要说明是“未声明”“应用层不允许写”“OS 权限不足”“路径类型不匹配”等哪类失败。

#### Ansible copy/template file modules

External reference: [Ansible builtin copy module](https://docs.ansible.com/ansible/latest/collections/ansible/builtin/copy_module.html), [Ansible builtin template module](https://docs.ansible.com/ansible/latest/collections/ansible/builtin/template_module.html)

相关模式：Ansible 在目标主机写文件时通常要求声明目标路径、owner/group/mode、backup、validate 等；`template`/`copy` 支持写前校验和备份，失败时不应留下不一致文件。

可映射点：

- 绝对路径写入声明可包含 `backup`、`mode`/保留权限、`validate` 或保存后 reload 钩子。
- 保存应尽量采用临时文件 + 校验 + 原子替换，而不是直接覆盖。
- 对运维配置文件，写前备份和失败恢复是可观察的安全控制。

### Recommended Safety Controls

以下控制按“声明模型、路径解析、权限检查、写入语义、API/UI 表达、审计与测试”分组，便于映射到现有代码。

#### A. 声明模型

1. 将绝对路径作为宿主侧显式授权，而不是插件包默认能力。
   - 现有 `.grill/plugin-config-mapping-design.md:16` 已说明 mapping 存到宿主侧而非修改插件目录。
   - 插件包可声明建议路径或占位信息，但真正的主机绝对路径应由宿主 mapping/平台管理员启用。
2. 从 `[]string` 升级为结构化条目，至少表达：
   - stable `id` 或 `name`：供 URL/API 引用，避免 URL 中传绝对路径。
   - `path`：原始显示路径。
   - `scope` 或 `kind`：`plugin` / `host_absolute`。
   - `access`：`read` / `read_write`；默认 `read`。
   - `required`：文件不存在是否报错。
   - `create`：是否允许不存在时创建；默认 false。
   - `file_type`：仅 regular file；可选 `file` / `file_or_create`，避免目录、socket、device。
   - `max_size`：读取和写入大小上限。
   - `sensitive` 或 `risk`：用于 UI 警告/隐藏内容/禁止 catalog 泄露完整路径。
   - `description` / `label`：UI 展示用途。
3. mapping 中某工具规则当前是整体覆盖（`internal/registry/registry.go:215-217`），绝对路径能力应保持同一合并语义，避免插件默认与宿主授权混合后难以判断权限来源。

#### B. 路径解析

1. URL 不传任意路径：沿用当前 `declaredPluginConfigFilePath` 的模式，但从“按 fileName 字符串匹配”改成“按声明 ID 匹配”。
2. 对 host absolute path 使用独立解析函数，不复用 `plugin.SafePath`：
   - `plugin.SafePath` 的职责是插件目录边界，`internal/plugin/load.go:277-292` 明确拒绝绝对路径。
   - 新函数应先判断 `filepath.IsAbs(path)`；Windows 下还要明确处理盘符路径、UNC 路径、Git Bash/MSYS 风格路径是否支持。
3. 清理和规范化路径：`filepath.Clean`、`filepath.Abs`，并保留原始显示路径用于 UI。
4. 符号链接处理：
   - 已存在文件：读取/写入前使用 `filepath.EvalSymlinks` 获取最终路径。
   - 不存在但允许创建：解析最近已存在父目录的 symlink 后再拼接最终文件名。
   - 若声明策略禁止 symlink，应拒绝任何路径链中含 symlink。
5. 明确拒绝：空路径、相对路径、目录、特殊文件、设备文件、socket、管道、路径中 NUL 字节、经过 URL 解码后出现未声明 ID 以外的路径成分。

#### C. 权限检查

1. 应用层权限先于 OS 权限：声明 `access: read` 时，即使 OS 可写也拒绝 PUT/DELETE。
2. OS 权限检查：
   - 读取：尝试 `os.Open` 或 `os.ReadFile` 前检查 regular file 和大小。
   - 写入已存在文件：检查 regular file、可写；避免跟随不允许的 symlink。
   - 创建新文件：检查父目录存在、是目录、可写，并且声明 `create: true`。
3. 敏感路径策略：可复用已有 `SensitivePaths` 字段概念，但目前 `internal/plugin/types.go:13` 和 `internal/plugin/types.go:43` 只是类型字段，需要明确是否参与 host config_files。策略可包括：
   - 默认拒绝典型系统敏感路径，如 SSH keys、系统密码/凭据、runtime sockets、Windows 系统目录等。
   - 允许管理员在 mapping 中显式标记 `sensitive: true`，UI 只显示风险和路径，不自动展示内容，或要求再次确认。
4. 读写权限结果应体现在列表 API，而不只在点击编辑时报错：`readable`、`writable`、`exists`、`reason`。

#### D. 写入语义

1. 默认禁用删除。当前 API 有 `DELETE /api/plugins/{id}/files/{filename}`（`internal/server/server.go:1761-1773`）；对 host absolute path，删除应单独声明 `allow_delete: true` 或完全不支持。
2. 写入建议使用：读旧内容/元数据 → 写临时文件 → 可选校验 → 原子替换 → 失败回滚/保留备份。
3. 对配置文件格式已知时，保存前执行语法校验；全局配置已有 YAML 预校验和 reload 回滚模式（`internal/server/server.go:792-817`）。
4. 写入保留或显式设置权限：避免 `os.WriteFile(..., 0644)` 无条件改变新文件权限；对已有文件尽量保留 mode。
5. 内容限制：文本编码、最大字节数、行尾策略、禁止二进制，避免 Web UI 写入大型或非文本宿主文件。
6. 并发控制：当前全局配置 PUT 使用 `state.mu.Lock()`（`internal/server/server.go:781-782`），插件配置文件保存没有显式全局锁。host absolute path 写入需要至少按目标路径串行化，避免两个请求互相覆盖。

#### E. API/UI 表达

1. 列表 API 从 `files: []string` 升级为结构化对象数组，例如：`id`, `path`, `display_path`, `scope`, `access`, `exists`, `readable`, `writable`, `size`, `sensitive`, `message`。
2. 编辑 API 以 `fileID` 为路径参数，不以绝对路径为 URL segment；例如 `/api/plugins/{pluginID}/files/{fileID}`。
3. Catalog 中如果继续暴露 `tool.config_files`，应避免直接暴露敏感绝对路径给所有页面；可只暴露 label/scope，详细路径在插件配置文件 API 中按权限返回。
4. UI 文案需要从“插件目录内对应路径”改为区分：插件内文件、宿主绝对路径文件、不可读/只读/可写。
5. 保存按钮应基于 `access` 和实时 `writable` 禁用，并在 host absolute path 上显示风险提示。

#### F. 审计、日志和测试

1. 审计记录：谁、何时、插件 ID、工具 ID、file ID、实际解析路径、操作 read/write/delete、结果；内容本身避免写入普通日志。
2. 测试矩阵：
   - plugin-local 相对路径继续通过。
   - manifest 中直接绝对路径按目标策略通过或拒绝；如果只允许宿主 mapping，则 manifest 绝对路径应拒绝。
   - mapping 中 host absolute path 只在结构化授权下通过。
   - 未声明 ID、URL 编码路径、`..`、反斜杠、双重编码不能访问任意文件。
   - 目录、symlink、特殊文件、超大文件、只读文件、父目录不可写分别返回可解释错误。
   - Windows 盘符路径、UNC 路径、POSIX 路径在当前平台行为明确。

### Mapping to This Repo

#### Minimal code areas affected by the capability

| Area | Current behavior | Mapping needed for absolute host paths |
|---|---|---|
| `internal/config/types.go` | `PluginToolConfigMapping.ConfigFiles []string` | 增加结构化 config file 类型，或新增并行字段以兼容旧 `[]string`。 |
| `internal/plugin/types.go` | 插件 manifest `Tool.ConfigFiles []string` | 决定 manifest 是否仍只允许插件内相对路径；若允许绝对路径，必须附带 access 元数据，否则不宜仅靠字符串。 |
| `internal/plugin/load.go` | `ValidatePackage` 对 `config_files` 调用 `SafePath(pkg.Dir, cf)` | 保持 plugin-local 校验；host absolute path 应绕开此函数并走 host policy 校验，避免削弱 `command`/`workdir` 的插件目录安全边界。 |
| `internal/registry/mapping.go` | mapping 校验未知工具、空路径、`SafePath` | 扩展为按条目类型校验：plugin-local 走 `SafePath`，host-absolute 走绝对路径/权限/策略校验。 |
| `internal/registry/registry.go` | mapping `config_files` 整体覆盖默认 | 保持整体覆盖，registry 中归一化成统一结构，供 server/UI 使用。 |
| `internal/server/server.go` | API 返回字符串列表，用 fileName 匹配并解析到插件目录 | 改为返回结构化条目；handler 按声明 ID 查找；解析函数分 plugin-local 与 host-absolute。 |
| `web/src/main.jsx` | 认为所有文件都在 `plugins/{pluginID}/{file}` | 展示 scope/access/status；只读禁用保存；host path 显示风险提示。 |
| `.trellis/spec/backend/plugin-import-export.md` | 插件导出严禁读取插件目录外文件 | host absolute config_files 不能被插件 ZIP 导出打包；该规范的“插件包只包含插件目录内文件”仍应保留。 |

#### Suggested data-shape direction for research consumers

为保持旧插件兼容，可把旧字符串视为：

```yaml
config_files:
  - config/example.conf
```

归一化为：

```yaml
config_files:
  - id: config/example.conf
    scope: plugin
    path: config/example.conf
    access: read_write
```

宿主绝对路径更适合放在 `configs/plugins/<plugin-id>.mapping.yaml`：

```yaml
tools:
  vendor.app.reload:
    config_files:
      - id: nginx-main
        scope: host_absolute
        path: /etc/nginx/nginx.conf
        access: read_write
        file_type: file
        create: false
        backup: true
        max_size: 1048576
        description: Nginx 主配置文件
```

Windows 示例需按当前进程平台处理：

```yaml
tools:
  vendor.app.windows:
    config_files:
      - id: app-conf
        scope: host_absolute
        path: C:\\ProgramData\\Vendor\\app.conf
        access: read
        file_type: file
```

#### Fit with existing repo decisions

- `.grill/plugin-config-mapping-design.md:11` 曾把“页面任意宿主路径或任意文件写入”列为不允许；本次需求改变了目标，但仍可保留“不能请求时任意输入，只能声明后访问”的核心限制。
- `.grill/plugin-config-mapping-design.md:16` 的“mapping 保存到宿主侧，不修改插件目录”与绝对路径授权天然匹配。
- `.trellis/spec/backend/plugin-import-export.md:51` 要求插件导出不读取或包含插件目录外文件；host absolute config_files 只能是运行时引用，不能成为插件包内容。
- `.trellis/spec/backend/plugin-import-export.md:62-65` 已要求非法 plugin ID、symlink、unsafe ZIP path 拒绝；同类思路可用于 host config_files 的声明 ID 与最终路径校验。
- `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md:65-73` 提醒 Windows 路径不能假设可直接传给 WSL/Git Bash；若配置文件路径随后会传给脚本，UI/API 保存路径与 runner 传参路径的 shell 表示要分开处理。

### External References

- [Docker Docs — Bind mounts](https://docs.docker.com/engine/storage/bind-mounts/) — 宿主路径映射默认可写会影响宿主文件系统；`readonly`/`ro` 是直接可比的 read-only 控制。
- [Kubernetes Docs — Volumes / hostPath](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath) — `hostPath` 暴露节点文件系统风险；`File`、`FileOrCreate` 等类型声明可映射到 config_files 的文件类型/创建策略。
- [OWASP — Path Traversal](https://owasp.org/www-community/attacks/Path_Traversal) — 不应把用户输入直接用于文件 API；需要规范化、解码后校验、访问策略限制。
- [OWASP Cheat Sheet — File Upload](https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html) — Web 写文件需要 defense-in-depth：文件名安全、存储位置、用户权限、文件系统权限、大小限制等。
- [Ansible builtin copy module](https://docs.ansible.com/ansible/latest/collections/ansible/builtin/copy_module.html) — 可比的运维文件写入能力，常见控制包括 owner/group/mode/backup/validate。
- [Ansible builtin template module](https://docs.ansible.com/ansible/latest/collections/ansible/builtin/template_module.html) — 可比的模板生成配置文件能力，强调目标路径、校验与备份语义。

### Related Specs

- `.trellis/spec/backend/plugin-import-export.md` — 插件 ID 不能当路径、插件目录需在配置 root 内、symlink/special file 拒绝、导出不得读取插件目录外文件。
- `.trellis/spec/guides/cross-platform-runtime-thinking-guide.md` — Windows/Git Bash/WSL 路径边界；绝对路径在不同 shell/平台上的表示需要明确。
- `.trellis/tasks/05-07-config-files-absolute-path/task.json` — 当前任务标题为“支持 config_files 绝对路径映射”，状态 `planning`。

## Caveats / Not Found

- `python ./.trellis/scripts/task.py current --source` 在当前工作树返回 `Current task: (none)`；本研究按用户显式提供的任务目录 `F:\ccb\ops_toolkits\.trellis\tasks\05-07-config-files-absolute-path` 写入。
- 当前代码未发现 host absolute-path config_files 的实现；所有实际读写仍通过插件目录 `SafePath`。
- 未发现认证/用户角色模型；因此“谁可以读写哪些 host config_files”只能映射为声明级权限与 OS 权限检查，无法映射到现有用户 RBAC。
- 未发现专用审计日志 API；当前运行日志目录存在，但配置文件编辑审计不是现有显式能力。
- 外部文档抓取中 Ansible/OWASP 部分页面存在 403 限制；引用基于公开文档主题和可访问摘要模式，落地前可人工复核具体参数名。

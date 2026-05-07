# 模板插件上传默认赋权 755

## Goal

上传插件 ZIP 后，安装到 `plugins/<plugin-id>` 的插件目录应默认具备可执行权限，避免插件脚本因为 ZIP 来源未保留 Unix 权限而在 Linux/macOS 部署后无法运行。

## Requirements

* 插件上传解压/安装时，不信任 ZIP 内部记录的权限位作为最终安装权限。
* 新增安装和替换安装都应将插件目录及其常规文件安装为 `0755` 权限。
* 保持现有上传安全边界：继续拒绝路径逃逸、符号链接和特殊文件，继续限制文件数量和解压大小。
* 不改变插件 manifest、工具参数、工作流注册和重复版本校验语义。

## Acceptance Criteria

* [x] 上传 ZIP 中脚本文件即使权限为 `0644`，安装后也是 `0755`。
* [x] 上传 ZIP 中普通资源文件安装后默认也是 `0755`，与“插件整体赋权 755”保持一致。
* [x] 替换已有插件时，新版本安装目录递归权限同样为 `0755`。
* [x] 现有上传安全测试继续通过。

## Definition of Done

* Tests added/updated (unit/integration where appropriate)
* Lint / typecheck / CI green
* Docs/notes updated if behavior changes
* Rollout/rollback considered if risky

## Technical Approach

将上传安装路径里的权限写入点收敛到后端：`extractPluginZip` 和/或 `copyDir` 不再保留 ZIP/临时目录中的文件权限，安装结果使用固定目录权限和文件权限 `0755`。优先在最终安装复制处保证权限，这样新增安装与替换安装路径一致。

## Decision (ADR-lite)

**Context**: 插件包可能来自 Windows、Web 上传或不同 ZIP 工具，ZIP 中脚本可执行位不可靠；运行器执行插件脚本时需要宿主文件系统具备可执行权限。  
**Decision**: 上传安装结果递归默认 `0755`，而不是只保留 ZIP 原权限或只给 `scripts/*.sh` 赋权。  
**Consequences**: 普通资源文件也会可执行，权限更宽但与当前打包产物 `chmod -R 755 plugins` 的行为一致；不在本任务引入细粒度权限策略。

## Out of Scope

* 不调整插件导出 ZIP 的权限写入策略。
* 不新增前端权限配置项。
* 不支持用户自定义上传权限模式。

## Technical Notes

* `internal/server/plugin_upload.go` 是上传安装入口。
* 当前 `extractPluginZip` 写文件使用 `file.FileInfo().Mode().Perm()`。
* 当前 `copyDir` 写最终安装文件使用 `info.Mode().Perm()`，因此会继承临时解压目录权限。
* `.trellis/spec/backend/plugin-import-export.md` 记录了插件导入导出、安全边界和路由契约。

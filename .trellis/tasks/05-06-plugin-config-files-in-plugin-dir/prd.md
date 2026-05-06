# 插件配置文件映射到插件目录

## Goal

将 `plugin.yaml` 中 `tools[].config_files` 的语义调整为“插件目录内配置文件路径声明”：页面配置功能根据声明直接读取和保存插件包内对应文件，而不是把内容存放到 `configs/plugins/{plugin}/files/`。

## What I already know

* 用户目标：页面的配置功能用于映射并修改插件指定配置路径的文件。
* `config_files` 当前已简化为字符串列表，例如 `- example.conf`。
* 当前后端 `handlePluginConfigFiles` 从 registry 收集声明文件名。
* 当前 `handleGetPluginConfigFile` / `handleSavePluginConfigFile` 通过 `config.LoadPluginConfigFile` / `SavePluginConfigFile` 访问 `configs/plugins/{plugin}/files/{file}`。
* 当前 runner 的 `passConfigFiles` 也从 `configs/plugins/{plugin}/files/{file}` 读取后复制到运行目录。

## Assumptions (temporary)

* `config_files` 路径应相对插件根目录，必须禁止绝对路径和 `..` 逃逸。
* 页面保存配置时直接写回插件目录内对应文件。
* 插件脚本默认也可以继续用插件内相对路径读取这些文件。

## Open Questions

* 是否需要保留 runner 运行前复制到 `runs/.../configs` 的兼容行为？

## Requirements (evolving)

* `tools[].config_files` 是插件目录内文件路径列表。
* 配置页面 GET/PUT/DELETE 直接操作插件目录内对应文件。
* 后端必须校验路径安全，不能允许逃逸插件目录。
* 插件模板示例应声明插件内实际存在的配置文件路径。

## Acceptance Criteria (evolving)

* [ ] 配置页面列出的文件来自 `plugin.yaml` 的 `config_files` 声明。
* [ ] 打开配置文件读取插件目录内真实文件内容。
* [ ] 保存配置文件写回插件目录内真实文件。
* [ ] 不再依赖 `configs/plugins/{plugin}/files` 存储插件配置文件内容。
* [ ] 路径逃逸/绝对路径被拒绝。
* [ ] `go test ./...` 与 `opsctl validate` 通过。

## Definition of Done

* Tests added/updated where behavior changes.
* Build/test/validate green.
* Plugin template updated to include the declared config file under plugin directory.
* Risky deletion/overwrite behavior is bounded by safe path validation.

## Out of Scope

* 不引入新的配置版本管理。
* 不恢复已废弃的 `config_templates`。
* 不增加额外的配置文件传参机制。

## Technical Notes

* Likely files: `internal/config/load.go`, `internal/server/server.go`, `internal/runner/runner.go`, `internal/plugin/load.go`, `internal/registry/plugin_test.go`, `internal/runner/runner_test.go`, `plugins/plugin.template/plugin.yaml`.

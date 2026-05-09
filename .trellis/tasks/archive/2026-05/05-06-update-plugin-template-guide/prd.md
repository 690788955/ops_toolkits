# 更新插件开发模板说明

## Goal

更新 `plugins/plugin.template` 的模板内容和“如何开发插件”说明，让新插件开发者看到的目录结构、`plugin.yaml` 示例、脚本读取配置方式、Web 配置页面说明都与当前机制一致：`config_files` 是插件目录内相对路径声明，页面直接编辑插件内对应文件，工具脚本自行读取配置。

## What I already know

* 用户要求更新插件开发模板内容，以及如何开发插件的说明。
* 当前机制：`tools[].config_files` 是插件内相对路径列表。
* 页面配置接口只允许编辑已声明的插件内文件。
* Runner 不再复制/传递 `config_files`，工具脚本自行读取配置。
* `plugin.template` 当前配置文件已移动到 `config/example.conf`。

## Assumptions (temporary)

* 主要更新范围是 `plugins/plugin.template/README.md`、`plugin.yaml`、示例脚本和可能的 dev toolkit 文档文本。
* 不需要新增功能，只修正文档/模板内容与当前行为一致。

## Open Questions

* 无阻塞问题，先按当前代码机制直接更新。

## Requirements

* README 不再描述 `config_templates`、`pass_via`、框架复制/生成配置文件。
* README 明确 `config_files` 只做页面编辑声明，路径相对插件根目录。
* README 给出推荐目录结构：`config/` 放插件可编辑配置文件，`scripts/` 放脚本。
* `plugin.yaml` 和脚本示例保持一致，使用 `config/example.conf`。
* 如果内置开发包说明里有旧机制，也同步更新。

## Acceptance Criteria

* [ ] 搜索不到模板文档中的旧 `configs/plugins/{plugin}/files` 存储说明。
* [ ] 搜索不到模板文档中旧 `pass_via` / `config_templates` 作为当前推荐机制的说明。
* [ ] `plugin.template` 通过 `opsctl validate`。
* [ ] 相关测试通过。

## Definition of Done

* 文档和模板一致。
* 测试/校验通过。
* 不引入新的配置机制或兼容层。

## Out of Scope

* 不改插件系统核心逻辑。
* 不恢复旧模板渲染机制。
* 不新增配置版本管理。

## Technical Notes

* 重点检查：`plugins/plugin.template/README.md`、`plugins/plugin.template/plugin.yaml`、`plugins/plugin.template/scripts/with-config.sh`、`internal/server/server.go` 中开发包说明字符串。

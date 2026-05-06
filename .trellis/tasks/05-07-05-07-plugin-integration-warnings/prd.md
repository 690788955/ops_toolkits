# 实现插件接入 warning 机制

## Goal

为插件接入增加非阻断 warning 机制，帮助内部运维同事根据模板和 `plugin.yaml` 自助完成插件接入质量修正，而不约束脚本内部业务逻辑。

## Requirements

* Warning 不阻断插件加载、validate、上传或运行，退出码保持成功。
* `plugins.strict` 只影响 Error，不影响 Warning。
* Warning 结构化，至少包含 code、plugin_id、可选 tool_id、field、message、suggestion。
* 第一版 Warning 覆盖：缺 README、插件/工具 description 缺失、参数 description 缺失、confirm.required 但 message 为空或太短、config_files 声明文件不存在。
* `opsctl validate` 输出 warning。
* 插件上传成功响应带 warnings。
* catalog/registry 暴露 warning，便于未来 Web 使用。

## Acceptance Criteria

* [ ] `opsctl validate` 校验通过时可输出 warnings 且退出码为 0。
* [ ] 插件上传成功时 response 包含 warnings。
* [ ] `/api/catalog` 包含插件 warnings。
* [ ] Warning 不影响工具/工作流运行。
* [ ] Go 测试、前端构建、二进制构建、validate 通过。

## Out of Scope

* 不实现脚手架。
* 不强制脚本内部编码规范。
* 不让 warning 在运行工具/工作流时提示。

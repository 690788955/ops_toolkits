# 移除配置文件库功能

## Goal

移除右上角“配置文件库”入口及其配套前后端能力，避免与当前 `config_files` 插件内配置文件维护机制混淆。

## Requirements

* Web 顶栏不再显示“配置文件库”按钮。
* 删除配置文件库弹窗、新增/编辑模板弹窗及 `/api/config/templates/` 前端调用。
* 删除后端 `/api/config/templates/` API 和只服务该功能的 handler。
* 保留“平台设置”“全局环境”和插件 `config_files` 配置页。

## Acceptance Criteria

* [ ] 搜索不到当前代码中的“配置文件库”入口和 `/api/config/templates/` 路由调用。
* [ ] 前端构建通过。
* [ ] Go 测试通过。
* [ ] `opsctl validate` 通过。

## Out of Scope

* 不移除插件目录内 `config_files` 编辑能力。
* 不移除全局环境配置。
* 不重新设计共享配置机制。

# 插件配置维护移除版本概念

## Goal

简化插件配置维护体验：插件配置页面只负责维护当前配置文件内容，不再展示、创建、切换或设置默认配置版本，降低用户理解和维护成本。

## What I already know

* 用户反馈：维护插件配置里的版本概念太复杂，只要配置能维护即可。
* 当前前端 `web/src/main.jsx` 的 `PluginConfigPanel` 包含版本相关状态和 API 调用：
  - `versions`
  - `showVersionPanel`
  - `showSaveVersionModal`
  - `newVersionName`
  - `newVersionDesc`
  - `/api/plugins/{id}/config/versions/`
* 后端已有配置版本 API，但本任务优先移除 UI 入口与前端复杂度。

## Requirements

* 插件配置面板只展示当前插件配置文件编辑能力。
* 移除插件配置面板中的“配置版本”按钮、版本列表、保存为版本、加载版本、设为默认等 UI。
* 保存行为保持不变：保存当前 `configs/plugins/{plugin-id}.yaml`。
* 不影响全局环境配置、平台设置、插件启用/禁用/删除等其他功能。
* 后端版本 API 本轮不删除，避免扩大影响。

## Acceptance Criteria

* [ ] 插件配置编辑页不再出现版本管理入口或版本列表。
* [ ] 插件配置可正常读取、编辑、保存。
* [ ] 前端构建通过。
* [ ] 服务端相关测试通过。

## Definition of Done

* `npm run build --prefix web` 通过。
* `GOTOOLCHAIN=local go test ./internal/server/...` 通过。
* `GOTOOLCHAIN=local go build -o bin/opsctl.exe ./cmd/opsctl` 通过。

## Out of Scope

* 删除后端配置版本 API。
* 删除已保存的历史版本文件。
* 重构配置 API 数据结构。

## Technical Notes

* 主要文件：`web/src/main.jsx`、`web/src/styles.css`。
* 当前版本样式可保留未使用，后续清理可另做。

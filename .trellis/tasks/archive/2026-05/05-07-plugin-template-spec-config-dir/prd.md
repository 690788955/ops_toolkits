# 修正插件模板 SPEC config_dir 说明

## Goal

修正 Web 下载的插件开发模板包内容，确保其中的 `SPEC.md`、示例 `plugin.yaml` 和模板 README 都说明并使用 `config_dir` 新契约，避免下载模板仍展示旧 `config_files: config/example.conf` 写法。

## Requirements

- 下载的插件开发模板包 `SPEC.md` 必须说明 `config_dir` 是插件内配置基准目录。
- 下载包示例应推荐：
  ```yaml
  config_dir: config
  config_files:
    - example.conf
  ```
- 文档必须说明旧 `config/example.conf` 写法兼容，但新模板推荐短文件名 + `config_dir`。
- 文档必须说明 `config_files`/`path` 只能是相对文件或相对目录项，禁止绝对路径和 `..` 逃逸。
- 文档必须说明目录项只一级展开普通文件，不递归。
- 文档必须说明插件包不能直接声明宿主绝对路径；宿主文件映射由管理员通过 `host_config_files.allowed_dirs` 和 host-side mapping 显式启用。
- 下载包内 README 和 `plugin.yaml` 示例必须与 SPEC 一致。
- 相关测试断言需要覆盖 `config_dir` 出现在下载包中。

## Acceptance Criteria

- [x] `/api/dev/toolkit.zip` 下载包内 `SPEC.md` 包含 `config_dir` 用法说明。
- [x] 下载包内示例 `plugin.yaml` 使用 `config_dir: config` + `config_files: [example.conf]`。
- [x] 下载包 README 说明 `config_dir` 与插件配置文件关系。
- [x] `go test ./internal/server` 通过。
- [x] `go test ./...` 通过。
- [x] `go build` 与 `opsctl validate` 通过。

## Technical Notes

- 下载包内容来自 `internal/server/server.go` 内嵌常量和 `buildToolDevKitZip()`，不是实体目录 `plugins/plugin.template/`。
- 需要同步更新 `internal/server/server_test.go` 的下载包断言。

# GitHub Actions 多平台打包 opsctl

## Goal

为 `opsctl` 增强 GitHub Actions 打包流程，让发布时自动产出 Windows、Linux、macOS 的 amd64/arm64 二进制，并把它们放入同一个总包中。总包内 `configs/` 和 `plugins/` 共用一份，`bin/` 下按平台架构区分多个 `opsctl` 可执行文件，减少手动分发成本。

## What I already know

* 用户希望把多个平台/架构的 `opsctl` 二进制都放在总包的 `bin/` 下，`configs/` 和 `plugins/` 内容共用一份。
* 产物需要能区分平台和架构，例如 x86/arm64。
* 仓库已有 `.github/workflows/build.yml`，当前会在 push 到 master 和 tag `v*` 时触发。
* 现有工作流已构建 linux/windows/darwin 的 amd64/arm64 矩阵，但排除了 `windows/arm64`。
* 现有每个平台架构会生成单独的 `opsctl-<goos>-<goarch>.tar.gz` artifact，并在 tag 时发布到 GitHub Release。
* 现有打包内容包含 `bin/opsctl`、`configs/`、`plugins/`。
* Web UI 需要先运行 `npm install --prefix web` 和 `npm run build --prefix web`，再构建 Go 二进制。

## Assumptions (temporary)

* 只生成一个总包，不保留多个单平台分发包。
* 总包用于下载/分发；包内目录结构固定为一份 `configs/`、一份 `plugins/`、一个包含多平台二进制的 `bin/`。
* 仍以 tag `v*` 发布 Release；push master 只上传 artifact。

## Open Questions

* 无。

## Requirements (evolving)

* GitHub Actions 自动构建 `opsctl` 的 linux/windows/darwin + amd64/arm64 组合。
* Windows 产物需要 `.exe` 后缀。
* 总包只包含一份 `configs/` 和一份 `plugins/`。
* 总包的 `bin/` 下包含多个按 OS/ARCH 命名的二进制文件。
* tag 发布时只把总包附加到 GitHub Release。

## Acceptance Criteria (evolving)

* [ ] `.github/workflows/build.yml` 覆盖 linux/windows/darwin 的 amd64/arm64。
* [ ] artifact/Release 只产出一个总包，例如 `opsctl-all-platforms.tar.gz`。
* [ ] 总包内有 `configs/`、`plugins/`、`bin/` 三类内容。
* [ ] `bin/` 下能看到按平台架构命名的多个二进制文件，例如 `opsctl_linux_amd64`、`opsctl_windows_arm64.exe`。
* [ ] Go 构建仍使用 `./cmd/opsctl` 入口。
* [ ] Web UI 构建仍在 Go 构建前执行，保证嵌入资源更新。

## Definition of Done (team quality bar)

* Tests added/updated where appropriate.
* CI YAML 语法和产物路径可解释、可维护。
* 不破坏现有 tag Release 流程。
* 修改代码文件后按项目要求重建 graphify；本任务若仅改 CI YAML，则无需重建代码图。

## Technical Approach

修改现有 `.github/workflows/build.yml`，将当前矩阵产出的多个单平台 tar.gz 改为单个打包流程：先构建 Web UI，再循环交叉编译 linux/windows/darwin 的 amd64/arm64，将所有二进制写入 `opsctl/bin/opsctl_<goos>_<goarch>[.exe]`，最后把一份 `configs/`、一份 `plugins/` 和完整 `bin/` 打成 `opsctl-all-platforms.tar.gz`。tag `v*` 发布时只上传这个总包。

## Decision (ADR-lite)

**Context**: 当前工作流已能构建大部分平台，但会生成多个单平台包，且排除了 Windows arm64；用户希望无需分别分发，插件和配置内容又是共用的。

**Decision**: 只产出一个总包 `opsctl-all-platforms.tar.gz`，其中 `configs/` 和 `plugins/` 共用一份，`bin/` 下放全部平台架构二进制。

**Consequences**: 分发更简单，但用户下载包会更大；如后续需要按平台精简下载，可再恢复单平台包或同时发布单平台包。

* 不改变 `opsctl package build` 的本地打包逻辑。
* 不新增安装脚本、自动更新器或包管理器发布。
* 不调整插件运行时规范。

## Technical Notes

* Existing workflow: `.github/workflows/build.yml`。
* Go module version: `go 1.21`。
* Existing build command pattern: `go build -o "dist/opsctl${ext}" ./cmd/opsctl`。
* Existing package layout: `opsctl/bin/`, `opsctl/configs/`, `opsctl/plugins/`。
* Desired total package layout: `opsctl/bin/opsctl_<goos>_<goarch>[.exe]`, `opsctl/configs/`, `opsctl/plugins/`。
* Existing release action: `softprops/action-gh-release@v2`。

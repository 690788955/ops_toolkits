# Shell 运维框架

YAML 驱动的运维自动化框架，用于把外部工具插件、参数定义、工作流 DAG 和 Web UI 统一组织起来。项目提供 `opsctl` CLI，可发现插件工具、校验配置、执行工具/工作流、启动交互菜单和 HTTP 控制台，并生成离线交付包。

## 功能特性

- 通过 `configs/ops.yaml` 统一配置运行记录、Web 服务和插件扫描目录。
- 通过 `plugins/<插件ID>/plugin.yaml` 接入外部工具插件。
- 支持插件贡献菜单分类、工具和工作流。
- 支持工具参数从 `--set`、YAML 参数文件和交互提示中合并。
- 支持工作流 DAG，按节点依赖串联多个工具。
- 支持 CLI、交互菜单、HTTP API 和 Web UI 多种入口。
- 支持生成包含 `configs/`、`plugins/` 和当前可执行文件的离线交付包。

## 快速开始

### 1. 构建 CLI

```bash
GOTOOLCHAIN=local go build -o bin/opsctl.exe ./cmd/opsctl
```

### 2. 校验配置

```bash
./bin/opsctl.exe validate
```

### 3. 查看插件工具

```bash
./bin/opsctl.exe list
./bin/opsctl.exe help-auto
./bin/opsctl.exe help-auto tool plugin.demo.greet
```

### 4. 运行示例插件工具

```bash
./bin/opsctl.exe run tool plugin.demo.greet --set name=Tester --set message=Hello --no-prompt
```

### 5. 运行确认流程示例

```bash
./bin/opsctl.exe run tool plugin.demo.confirmed --set target=demo --no-prompt
```

执行时输入 `yes`、`确认`、`是` 或 `继续` 完成确认。

### 6. 启动 Web UI

```bash
./bin/opsctl.exe serve --port 8080
```

启动后访问控制台输出中的 Web UI 地址。

## 常用命令

| 命令 | 说明 |
| --- | --- |
| `opsctl list` | 列出已发现的插件工具和工作流 |
| `opsctl validate` | 校验 `configs/ops.yaml` 和插件配置 |
| `opsctl help-auto` | 根据 YAML 元数据生成目录帮助 |
| `opsctl help-auto tool <id>` | 查看指定工具帮助 |
| `opsctl help-auto workflow <id>` | 查看指定工作流帮助 |
| `opsctl run tool <id>` | 执行指定工具 |
| `opsctl run workflow <id>` | 执行指定工作流 |
| `opsctl start` / `opsctl menu` | 启动交互式菜单 |
| `opsctl serve` | 启动 HTTP API 和 Web UI |
| `opsctl package build` | 生成交付包和 zip 文件 |

通用参数：

- `--base-dir`：指定项目根目录，默认当前目录。
- `--params`：指定 YAML 参数文件。
- `--set key=value`：覆盖或补充参数，可重复传入。
- `--no-prompt`：禁用缺失必填参数时的交互提示。

## 目录结构

```text
bin/                        本地构建产物目录（不提交）
cmd/opsctl/                 CLI 入口
configs/ops.yaml            框架主配置
internal/app/               Cobra 命令定义
internal/config/            YAML 配置和参数处理
internal/plugin/            插件清单解析与校验
internal/registry/          插件工具、工作流注册与校验
internal/runner/            工具和工作流执行器
internal/server/            HTTP API 与内嵌 Web 静态资源
internal/packagebuild/      交付包构建
plugins/<插件ID>/           插件包目录
web/                        Web UI 前端项目
runs/                       运行记录和日志目录（不提交）
dist/                       打包输出目录（不提交）
```

## 配置说明

### 主配置 `configs/ops.yaml`

`configs/ops.yaml` 定义应用信息、运行记录目录、服务监听地址、菜单分类、插件扫描目录和 UI 配置。

```yaml
app:
  name: Shell 运维框架
  description: 演示用 YAML 驱动运维自动化框架
  version: 0.1.0

paths:
  tools: []
  workflows: []
  runs: runs
  logs: runs/logs

plugins:
  paths:
    - plugins
  strict: false
  disabled: []
```

`paths.tools` 和 `paths.workflows` 显式为空，表示不再使用旧的独立 `tools/`、`workflows/` 目录。工具和工作流通过插件贡献。

## 插件接入

外部工具按插件包方式接入。插件目录放在 `plugins/<插件ID>/`，至少包含：

- `plugin.yaml`：插件元数据、菜单分类、工具/工作流贡献。
- 实际执行脚本：例如 `scripts/run.sh`。
- `README.md`：插件维护说明。

示例：

```text
plugins/plugin.demo/
  plugin.yaml
  scripts/
    greet.sh
    confirmed.sh
  README.md
```

### 插件清单示例

```yaml
id: vendor.backup
name: 备份工具集
version: 1.0.0
description: 第三方备份恢复工具
author: vendor-a

contributes:
  categories:
    - id: backup
      name: 备份恢复
      description: 第三方备份恢复工具
  tools:
    - id: vendor.backup.full
      name: 全量备份
      category: backup
      description: 执行一次全量备份
      command: scripts/backup.sh
      args:
        - --target
        - '{{ .target }}'
      workdir: .
      timeout: 30m
      parameters:
        - name: target
          type: string
          description: 备份目标
          required: true
      confirm:
        required: true
        message: 确认执行全量备份？
```

### 插件规则

- 插件工具 ID 必须以插件 ID 加点号开头，例如 `vendor.backup.full`。
- `command` 和 `workdir` 必须位于插件目录内，避免路径逃逸。
- `strict: false` 时坏插件会被跳过并输出告警；`strict: true` 时任意插件加载失败都会阻断校验/启动。
- `disabled` 可以按插件 ID 或插件目录名禁用插件。
- `confirm.required: true` 的工具在 CLI、菜单和 Web 执行前都需要确认。

## Web 前端

前端项目位于 `web/`，用于开发 Web UI。

```bash
npm install --prefix web
npm run dev --prefix web
npm run build --prefix web
```

后端运行时会从内嵌静态资源提供 Web UI。

## 打包交付

```bash
./bin/opsctl.exe package build
```

该命令会在 `dist/` 下生成目录和 zip 文件，包含：

- `configs/`
- `plugins/`
- 当前 `opsctl` 可执行文件

## 开发验证

```bash
GOTOOLCHAIN=local go test ./...
npm run build --prefix web
GOTOOLCHAIN=local go build -o bin/opsctl.exe ./cmd/opsctl
./bin/opsctl.exe validate
```

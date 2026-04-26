# Shell 运维框架

YAML 驱动的运维自动化框架，用于把 Shell 工具、参数定义、工作流 DAG 和 Web UI 统一组织起来。项目提供 `opsctl` CLI，可发现工具、校验配置、执行工具/工作流、启动交互菜单和 HTTP 控制台，并生成离线交付包。

## 功能特性

- 通过 `ops.yaml` 统一配置工具目录、工作流目录、运行记录和 Web 服务。
- 通过 `tools/<分类>/<工具>/tool.yaml` 描述工具元数据、参数、执行入口和确认策略。
- 支持工具参数从 `--set`、YAML 参数文件和交互提示中合并。
- 支持工作流 DAG，按节点依赖串联多个工具。
- 支持 CLI、交互菜单、HTTP API 和 Web UI 多种入口。
- 支持生成工具模板、工作流模板和可分发交付包。

## 快速开始

### 1. 构建 CLI

```bash
go build -o opsctl ./cmd/opsctl
```

Windows 环境也可以构建为：

```bash
go build -o opsctl.exe ./cmd/opsctl
```

### 2. 校验配置

```bash
./opsctl validate
```

### 3. 查看工具和工作流

```bash
./opsctl list
./opsctl help-auto
./opsctl help-auto tool demo.hello
./opsctl help-auto workflow demo.hello
```

### 4. 运行示例工具

```bash
./opsctl run tool demo.hello --set name=Tester --set message=Hello --no-prompt
```

### 5. 运行示例工作流

```bash
./opsctl run workflow demo.hello --set name=Workflow --no-prompt
```

### 6. 启动 Web UI

```bash
./opsctl serve --port 8080
```

启动后访问控制台输出中的 Web UI 地址。

## 常用命令

| 命令 | 说明 |
| --- | --- |
| `opsctl list` | 列出已发现的工具和工作流 |
| `opsctl validate` | 校验 `ops.yaml`、工具和工作流配置 |
| `opsctl help-auto` | 根据 YAML 元数据生成目录帮助 |
| `opsctl help-auto tool <id>` | 查看指定工具帮助 |
| `opsctl help-auto workflow <id>` | 查看指定工作流帮助 |
| `opsctl run tool <id>` | 执行指定工具 |
| `opsctl run workflow <id>` | 执行指定工作流 |
| `opsctl start` / `opsctl menu` | 启动交互式菜单 |
| `opsctl serve` | 启动 HTTP API 和 Web UI |
| `opsctl new tool <分类>/<工具>` | 创建工具模板 |
| `opsctl new workflow <workflow-id>` | 创建工作流模板 |
| `opsctl package build` | 生成交付包和 zip 文件 |

通用参数：

- `--base-dir`：指定项目根目录，默认当前目录。
- `--params`：指定 YAML 参数文件。
- `--set key=value`：覆盖或补充参数，可重复传入。
- `--no-prompt`：禁用缺失必填参数时的交互提示。

## 目录结构

```text
cmd/opsctl/                 CLI 入口
internal/app/               Cobra 命令定义
internal/config/            YAML 配置和参数处理
internal/registry/          工具、工作流注册与校验
internal/runner/            工具和工作流执行器
internal/server/            HTTP API 与内嵌 Web 静态资源
internal/scaffold/          工具和工作流模板生成
internal/packagebuild/      交付包构建
ops.yaml                    框架主配置
tools/                      工具目录
workflows/                  工作流目录
web/                        Web UI 前端项目
dist/                       打包输出目录
runs/                       运行记录和日志目录
```

## 配置说明

### 主配置 `ops.yaml`

`ops.yaml` 定义应用信息、工具目录、工作流目录、运行记录目录、服务监听地址、菜单分类、注册过滤规则和 UI 配置。

```yaml
app:
  name: Shell 运维框架
  description: 演示用 YAML 驱动运维自动化框架
  version: 0.1.0

paths:
  tools:
    - tools
  workflows:
    - workflows
  runs: runs
  logs: runs/logs
```

### 工具配置 `tool.yaml`

工具放在 `tools/<分类>/<工具>/` 下，至少包含：

- `tool.yaml`：工具元数据、参数、执行入口。
- `bin/run.sh`：实际执行脚本。
- `README.md`：工具维护说明、输入输出和回滚方式。

参数会按配置传递给脚本：

- 环境变量：`OPS_PARAM_<参数名大写>`。
- 参数文件：`OPS_PARAM_FILE` 指向生成的 YAML 参数文件。
- 命令参数：开启 `pass_mode.args` 后附加 `key=value` 参数。

## 新增工具

```bash
./opsctl new tool demo/example
./opsctl validate
```

生成后补齐：

1. `tools/demo/example/tool.yaml` 中的工具说明、参数和执行入口。
2. `tools/demo/example/bin/run.sh` 中的实际运维逻辑。
3. `tools/demo/example/README.md` 中的维护说明和回滚方案。

## 新增工作流

```bash
./opsctl new workflow demo-example
./opsctl validate
```

工作流通过 `nodes` 定义节点，通过 `edges` 定义依赖关系。节点的 `tool` 字段引用工具 ID，例如 `demo.hello`。

## Web 前端

前端项目位于 `web/`，用于开发 Web UI。

```bash
cd web
npm install
npm run dev
npm run build
```

后端运行时会从内嵌静态资源提供 Web UI。

## 打包交付

```bash
./opsctl package build
```

该命令会在 `dist/` 下生成目录和 zip 文件，包含：

- `ops.yaml`
- `tools/`
- `workflows/`
- `configs/`（如果存在）
- 当前 `opsctl` 可执行文件

## 开发验证

```bash
go test ./...
go build -o opsctl ./cmd/opsctl
./opsctl validate
```

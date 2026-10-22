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
| `opsctl market index` | 生成插件市场索引 `index.json` 和插件包 |

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

market:
  sources:
    - id: local
      name: 本地插件市场
      url: market
    - id: internal
      name: 内网插件市场
      url: http://plugins.internal/ops-market/index.json
```

`paths.tools` 和 `paths.workflows` 显式为空，表示不再使用旧的独立 `tools/`、`workflows/` 目录。工具和工作流通过插件贡献。`market.sources` 可选，用于在 Web 插件管理中浏览和安装市场插件；`url` 支持本地目录和 HTTP/HTTPS 地址，留空时默认使用项目根目录下的 `market/`。

### 全局变量 `configs/global-env.conf`

以 `.env` 格式维护工具和插件脚本运行时可引用的全局变量，可在 Web 控制台的“全局变量”中在线编辑，保存后立即生效。

```bash
DB_HOST=10.0.0.8
API_TOKEN=xxxxxx
```

变量会直接导出到工具进程环境，本地执行和远程下发的执行机都生效；同时框架会设置 `OPS_GLOBAL_ENV_FILE` 指向该文件，脚本可按需加载：

```bash
source "$OPS_GLOBAL_ENV_FILE"
```

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
      config_dir: config
      config_files:
        - example.conf
      confirm:
        required: true
        message: 确认执行全量备份？
```

### 插件规则

- 插件工具 ID 必须以插件 ID 加点号开头，例如 `vendor.backup.full`。
- 带路径的 `command` 和 `workdir` 必须位于插件目录内，避免路径逃逸；裸命令名（如 `java`、`ansible-playbook`）只有加入 `plugins.allowed_commands` 后才会通过运行环境 PATH 执行。
- `strict: false` 时坏插件会被跳过并输出告警；`strict: true` 时任意插件加载失败都会阻断校验/启动。
- `disabled` 可以按插件 ID 或插件目录名禁用插件。
- `confirm.required: true` 的工具在 CLI、菜单和 Web 执行前都需要确认。
- 插件 manifest 的 `config_dir` 是配置基准目录：相对路径按插件目录解析，也支持当前平台可识别的绝对路径。
- 未声明 `config_dir` 时，新短文件名默认以插件 `config/` 作为基准；旧 `config/example.conf` 写法保持兼容。
- `config_files` / 结构化 `path` 必须是相对文件或相对目录项，不能写绝对路径或使用 `..` 逃逸最终解析出的 `config_dir`；目录项在 Web/API 列表中只展开下一层普通文件，不递归。
- 如需更细粒度地管控宿主绝对路径配置文件，由管理员在 `configs/ops.yaml` 配置 `host_config_files.allowed_dirs` 目录白名单，并在 `configs/plugins/<plugin-id>.mapping.yaml` 中用 `scope: host_absolute`、`config_dir`、相对 `path`、`access: read/read_write`、`create` 显式启用；最终路径及符号链接解析后的真实路径都必须仍在白名单目录内。

宿主绝对路径配置文件映射示例（由管理员配置，不写入插件包默认清单）：

```yaml
# configs/ops.yaml
host_config_files:
  allowed_dirs:
    - /etc/myapp

# configs/plugins/vendor.backup.mapping.yaml
tools:
  vendor.backup.full:
    config_files:
      - id: backup-main
        label: 备份主配置
        scope: host_absolute
        config_dir: /etc/myapp
        path: backup.conf
        access: read_write
        create: false
```

`access: read` 只允许查看并拒绝保存；`access: read_write` 还需要当前进程具备 OS 写权限才允许保存。`create: false` 不自动创建缺失文件；`create: true` 仅在父目录可写时允许创建。白名单只支持目录前缀，不支持单文件白名单。

## 插件市场

插件市场由一个索引和若干插件 ZIP 包组成，可以放在本地目录，也可以发布到静态文件服务。Web UI 的“插件管理 → 插件市场”页签可以浏览市场、搜索插件，并一键安装或更新。

### 生成市场内容

```bash
./bin/opsctl.exe market index
```

默认输出到项目根目录的 `market/`，可用 `--output` 指定其他目录、`--name` 指定市场名称：

```text
market/
  index.json
  packages/
    vendor.backup-1.0.0.zip
```

`index.json` 中的插件包地址使用相对路径，并把 `sha256` 和 `size` 一并写入索引。生成过程会重新校验 `plugins/` 下的所有插件；存在加载失败或校验不通过的插件时命令会失败。插件包使用固定时间戳，相同插件内容可生成稳定 ZIP。本地 `user.workflows` 插件不会发布到市场，并会输出跳过提示；如需共享其中的工作流，请先导出用户工作流 ZIP 后作为独立插件发布。

### 本地市场

`opsctl market index` 默认输出到项目根目录的 `market/`，该目录可直接作为本地市场使用。未配置 `market.sources` 时，Web 端默认读取 `<项目根目录>/market/index.json`，无需启动额外的静态文件服务：

```yaml
market:
  sources: []
```

首次打开“插件市场”时，如果默认本地市场的索引不存在，服务会自动根据 `plugins/` 下已安装的插件生成索引和插件包；没有可发布插件时生成空索引。需要重新生成或修改市场名称时仍可运行 `opsctl market index`。

也可以在设置页或 `configs/ops.yaml` 中显式指定本地目录、`index.json` 文件路径或 `file://` URL：

```yaml
market:
  sources:
    - id: local
      name: 本地插件市场
      url: market
```

### 远程市场

把 `market/` 目录整体发布到内网静态文件服务（Nginx、对象存储或任意 HTTP 文件服务），然后配置索引地址：

```yaml
market:
  sources:
    - id: internal
      name: 内网插件市场
      url: http://plugins.internal/ops-market/index.json
```

配置支持多个市场源，Web 端可切换来源。`url` 可以是本地目录、`index.json` 文件路径、`file://` URL，或 `http` / `https` 地址。远程插件包必须与索引同源；本地插件包必须位于市场目录内。无论本地还是远程，安装前都会校验包大小和 `sha256`。

### 安装行为

- 安装前 Web 端会提示插件会写入本地插件目录并执行本地脚本，需要确认来源可信。
- 同一插件在索引中可以有多个版本；未指定版本时选择最高版本，安装接口也支持显式指定 `version`。
- 已安装且市场版本更高的插件显示“更新”；更新会用新版本整体替换插件目录，要求市场版本高于本地版本。
- 已安装同版本显示“已安装”，本地版本更高显示“本地较新”，禁用的插件显示“已禁用”且不能直接更新。
- 插件 `compatibility.opsctl` 声明了 `opsctl` 版本约束时，不满足约束的插件不允许安装。支持 `>=`、`>`、`<=`、`<`、`=`、`==`，空格或逗号分隔表示同时满足，`*` / `any` 表示不限制。
- 安装复用插件 ZIP 上传链路：校验清单、路径、脚本与工作流，校验通过后重载注册表。
- 远程市场索引默认缓存 60 秒；点击“刷新”或执行安装会强制重新拉取索引。本地市场每次直接读取磁盘，重新生成索引后立即生效。
- 索引最大 2 MB，单个插件包最大 20 MB；索引缺少合法 `sha256` 时拒绝安装，下载后会强制校验摘要和大小。

### 分享插件到远程市场

“插件管理 → 已安装”里每个已启用插件都有“分享”按钮，可把本机插件直接发布到远程插件市场（Nexus 3 Raw 仓库）：

- 服务端按当前插件目录重新打包（自动包含归属该插件的工作流），校验清单后写入 `<package_dir>/<id>-<version>.zip`，并更新仓库中的 `index.json`。
- 存在多个可用的 Nexus 来源时，弹窗里可选择目标来源；只有本地市场（未配置 Nexus）时按钮置灰不可点击，并在已安装列表上方提示配置入口。
- 远程已存在同名同版本插件包时返回冲突，勾选“覆盖同名同版本”后重试即可；覆盖会删除索引中指向的旧包。
- 相关接口：`GET /api/nexus/share/targets` 返回可用的分享目标，`POST /api/plugins/{id}/share?source=<来源ID>&replace=true` 执行分享。

### 导出用户工作流

在“插件管理 → 已安装”中点击“导出用户工作流”，可以勾选要导出的工作流：

- 导出的 ZIP 以 `user.workflows` 插件根目录组织，只包含所选工作流及其 `config/workflows/<id>` 配置目录，可直接上传安装。
- 勾选“同时导出引用的插件”后，工作流工具依赖的已安装插件会以独立 ZIP 放入 `dependencies/`，并生成 `export-manifest.json` 和 `README.md`；依赖插件需要单独上传安装。
- 如果工作流引用了当前环境未安装的工具，导出清单会在 `missing_tools` 中列出。

## Web 前端

前端项目位于 `web/`，用于开发 Web UI。

工作流编辑器中的“归属”决定工作流保存位置：选择 `user.workflows` 为全局工作流，选择业务插件则写入该插件的 `workflows/` 目录并随插件 ZIP 导出。归属业务插件时不再单独展示分类，保存时自动写入归属插件的 `contributes.categories`；插件声明多个分类时保留“插件分类”下拉，只能在其范围内选择；只有全局工作流（或内置工作流）才展示完整分类列表。

编辑侧栏只保留工作流加载、当前摘要和操作按钮；工作流 ID、名称、描述、归属、分类、共享给和参数定义在点击“保存”后弹出的“保存工作流”弹窗中填写，参数定义折叠在“高级设置”中。已保存工作流的 ID 只读，避免改 ID 时另存出重复工作流。

“共享给（可选）”可以再选多个插件，写入工作流 YAML 的 `shared_with`。共享只影响可见性：按分类筛选工作流时，该工作流会额外出现在这些插件的分类下并标注“共享自 …”。文件仍只保存在归属插件目录，不复制，也不随对方插件的 ZIP 导出；保存时会自动剔除归属插件与全局工作流自身，且目标插件必须已启用。

运行工作流是三步：填写参数 → 选择执行机器 → 确认运行。执行机器默认“跟随节点配置”，也可以统一选本机或某台 SSH 操作机；默认只作用于没有单独指定执行目标的节点，勾选“覆盖节点已配置的执行目标”后全部节点都改到所选机器执行。该选择只在本次运行生效，不会写回工作流 YAML。对应接口参数为 `target`（`follow_nodes` / `local` / 操作机 ID）与 `override_nodes`。

“配置”页的“工作流配置”里，除工作流自身的配置文件外，还会列出该工作流节点引用到的插件配置（`referenced_plugins`）。这些文件仍由插件持有、按插件路径编辑，工作流侧只做展示与跳转，不复制文件，避免同一份配置出现两个来源。

画布节点分为两类：内置编排节点与插件工具节点。内置编排节点按用途再分三族，颜色与形状一一对应，选择面板和悬浮工具栏也按族分区：

| 分类 | 形状 | 现有节点 |
| --- | --- | --- |
| 流程控制 `flow` | 菱形 | 条件分支、并行、合流、循环 |
| 数据与文件 `data` | 文件块 | 上传文件、提取配置 |
| 内置能力 `builtin` | 圆形 | HTTP 调用（CURL，已上线）、Shell 脚本与消息通知（规划中，面板内禁用） |

节点的分类、形状、符号、标题和帮助文案集中在 `web/src/workflow/catalog.js` 的 `controlNodeCatalog`。新增内置节点时只需在该表追加一条元数据并把 `status` 改为 `ready`，画布渲染、选择面板分组、悬浮工具栏分区与搜索会自动跟随；运行侧再补对应的执行器与 Go 侧 `config.WorkflowNodeType*` 常量即可。

### 内置 HTTP 调用节点

`http_request` 节点等价于一条可参数化的 `curl`：在本机（不走操作机）发起请求，URL、Header 与 Body 都支持运行期模板，响应写入节点运行目录并进入工作流上下文。

```yaml
nodes:
  - id: deploy
    type: http_request
    name: 触发发布
    http:
      method: POST
      url: "{{ .base_url }}/api/deploy"
      headers:
        Authorization: "Bearer {{ .token }}"
      body_type: json
      body: '{"env":"{{ .env }}"}'
      expect_status: [200, 201]
      fail_on_error: true
      save_response: response/deploy.json
      timeout: 15s
```

- 方法支持 GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS，GET 不允许携带请求体；URL 只允许 `http://` 或 `https://`。
- `expect_status` 留空时按 2xx 判定；`fail_on_error: false` 时状态码不符只记录不中断，节点仍记为成功。
- `save_response` 只能是节点运行目录下的相对路径；响应体本身始终写入 `stdout.log`。
- 下游可引用：`{{ .steps.<id>.stdout }}`（响应体）、`{{ .steps.<id>.http.status }}`、`{{ .steps.<id>.http.duration_ms }}`、`{{ .steps.<id>.http.saved_path }}`。
- 校验在 `config.ValidateHTTPConfig`（`internal/config/types.go`），执行器在 `internal/runner/http.go`，Go 测试覆盖请求渲染、状态码失败与容错分支。

选择面板中每个分类分组可点击标题折叠，默认全部展开，输入搜索词时强制展开。画布左上角的分类图例显示各类节点数量，点击某一类会在画布上仅高亮该类、其余节点淡化（再点取消）；该高亮只作用于显示层，不修改节点数据与连线。

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

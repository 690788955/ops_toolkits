# brainstorm: 插件工具配置文件传入

## Goal

探索插件工具是否应支持“整份配置文件传入”，让外部工具可以通过一个配置文件接收复杂参数，而不是只依赖逐个 `--set` 参数或交互输入；判断它是否符合当前 YAML 驱动、插件优先的运维框架设计。

## What I already know

* 用户希望插件工具能够支持配置文件传入。
* 用户的目标是“整个配置文件来让其他工具调用”。
* 当前框架已有插件工具 manifest、参数定义、`--params` YAML 参数文件、`--set key=value` 覆盖、默认值和交互提示机制。
* 当前插件运行会把参数渲染到 `args` 并执行插件目录内脚本。
* `internal/config/load.go` 已有 `LoadParamsFile(path)`，CLI 的 `--params` 会把 YAML 参数文件读成参数 map。
* `internal/config/types.go` 已有 `PassMode.ParamFile` 和 `PassMode.FileName` 字段。
* `internal/runner/runner.go` 会在 `pass_mode.param_file: true` 时，把最终合并后的参数写到 run 目录，并通过 `--params-file <path>` 和 `OPS_PARAM_FILE=<path>` 传给工具。
* 当前 `Parameter.Default` 是 interface，但 `LoadParamsFile` 最终会 `fmt.Sprint` 成 `map[string]string`，因此复杂嵌套 YAML 还不是一等能力。
* 新增的 `lzyh.es_backup` 插件已经暴露了 `config` 参数，但这是工具原生配置路径，不等同于框架统一参数文件能力。

## Assumptions (temporary)

* 用户说的“配置文件”可能有两层含义：一是 opsctl 的参数文件，二是直接生成/传入工具原生配置文件。
* 这个能力适合复杂工具，例如备份、巡检、批量任务、迁移任务。
* MVP 应避免让任意外部路径配置文件绕过插件目录安全边界。

## Open Questions

* 待根据现有机制和方案收敛后询问一个偏好问题。

## Requirements (evolving)

* 支持讨论插件工具以配置文件作为输入的合理性。
* 需要保持插件目录安全边界和参数可追踪性。
* 需要兼容当前 `--params` / `--set` 的参数合并模型。
* 推荐把“框架参数配置文件”和“工具原生配置文件”分层处理：前者由 opsctl 统一合并、审计和传递，后者只在插件需要时由 wrapper 安全使用。
* 需要考虑同一组工具共享一份配置文件的场景，例如同一 ES 备份恢复分类下的备份、查询、恢复工具共用 ES 地址、认证、NAS 路径、保留策略等配置。
* 分类本身当前主要是 UI/菜单分组，不应直接当作强安全/配置边界；更稳妥的模型是“插件级或工具组级共享配置”，再映射到分类展示。
* 配置优先级采用用户确认的三层覆盖模型：全局配置 < 插件级共享配置 < 工具级配置；后续运行时输入（如 `--params` / `--set` / API params）应继续作为更高优先级覆盖层。

## Research Notes

### What similar tools do

* Terraform 的 `-var-file`、Helm 的 `values.yaml`、Ansible 的 vars/extra-vars 都支持配置文件 + 命令行覆盖的组合，核心价值是可复用、可审计、适合复杂参数。
* 这些工具通常保持覆盖优先级清晰：默认值 < 文件 < 命令行显式覆盖。
* 对敏感信息通常不鼓励直接写入普通配置文件，而是使用环境变量、secret 文件或外部凭据管理。

### Constraints from this repo

* 当前框架已经有 `--params` 作为“调用方输入配置文件”。
* 当前 runner 已经有 `pass_mode.param_file` 作为“传给工具的最终参数文件”。
* 当前参数内部模型是 `map[string]string`，不适合直接承载深层嵌套结构、数组和强类型对象。
* 插件 `command`/`workdir` 必须在插件目录内，不能为了配置文件能力开放任意路径执行或路径逃逸。

### Feasible approaches here

**Approach A: 先正式化现有参数文件能力** (Recommended MVP)

* How it works: 文档化并完善 `--params` + `pass_mode.param_file`；插件声明 `pass_mode.param_file: true` 后，opsctl 把最终合并参数写成运行目录内 `params.yaml`，工具通过 `OPS_PARAM_FILE` 或 `--params-file` 读取。
* Pros: 最符合当前框架；实现小；保留参数定义、默认值、Web 表单、CLI `--set` 覆盖和运行审计。
* Cons: 暂时只适合扁平参数；工具如果需要原生复杂配置，还要 wrapper 转换。

**Approach B: 增加插件原生配置模板**

* How it works: `plugin.yaml` 允许声明 `config_templates` 或工具级 `generated_config`，由 opsctl 根据参数渲染成工具原生配置文件，再把文件路径传给脚本。
* Pros: 适合 ES/备份/巡检这类复杂工具；工具可以消费自己的配置格式。
* Cons: 设计面更大；需要模板安全、文件落点、敏感信息、Web/API 展示和测试。

**Approach D: 插件级/工具组级共享配置** (Good fit for category-like tools)

* How it works: 在插件或工具组层声明一份共享配置，例如 `shared_config`，同插件内多个工具默认继承；运行时可由 `--params` 或 Web 表单选择/覆盖，最终仍以 `OPS_PARAM_FILE` 或生成配置文件传给工具。
* Pros: 非常适合 ES 备份这种“一组工具共用连接信息/路径/策略”的场景；减少重复参数；比“按分类”更安全，因为插件是明确边界，分类只是展示分组。
* Cons: 需要定义继承和覆盖规则；如果不同插件复用同一个分类，不能误共享配置。


## Acceptance Criteria (evolving)

* [ ] 明确这是通用框架能力，不绑定 `lzyh.es_backup`。
* [ ] 支持插件包自带默认配置 `plugins/<plugin-id>/configs/default.yaml`。
* [ ] 支持宿主覆盖配置 `configs/plugins/<plugin-id>.yaml`。
* [ ] 最终运行参数文件包含全局、插件、工具和运行时参数合并结果。
* [ ] 给出清晰目录职责、加载顺序、安全边界和 MVP 范围。

## Definition of Done (team quality bar)

* Tests added/updated if implementation follows.
* `GOTOOLCHAIN=local go test ./...` passes if implementation follows.
* Docs/spec updated if framework behavior changes.
* Security boundary and rollback considerations documented.

## Decision (ADR-lite)

**Context**: 多个工具可能属于同一个业务工具组，需要复用连接信息、路径、认证方式和默认策略；但分类只是 UI 分组，不能作为可靠配置边界。

**Decision**: 采用分层配置覆盖模型：全局配置 < 插件级共享配置 < 工具级配置；运行时输入继续作为更高优先级覆盖层。插件是共享配置的主要边界，工具可以覆盖插件默认值。

**Consequences**: 这能减少插件内重复参数并保持工具差异化；需要实现清晰的合并规则、冲突可见性和安全限制，避免不同插件因共享分类而误用配置。


## Technical Approach

### Recommended Scope

采用通用的“分层配置 + 深合并 + 运行时覆盖 + 最终参数文件 + 原生配置模板”能力，不绑定任何特定插件；`lzyh.es_backup` 只是示例验证对象。

配置优先级（完整版本）：

```text
Parameter.default < 全局 config_defaults < 插件 shared_config < 插件自带 configs/default.yaml < 工具 config_defaults < 宿主插件配置文件 < 运行时 --params/API params/workflow params < --set
```

说明：宿主插件配置放在工具默认之后，这样部署人员可以用 `configs/plugins/<plugin-id>.yaml` 统一覆盖插件作者和工具作者提供的默认策略；运行时参数仍然最高。

一步到位范围：

* 支持嵌套 YAML 深合并。
* 支持点路径覆盖，例如 `--set es.host=prod-es.local`。
* 支持敏感字段声明、运行记录/API/Web 脱敏。
* 支持密钥引用字符串作为普通配置值透传，例如 `secret:file:/path`、`secret:env:ES_PASSWORD`；是否解析为真实密钥由后续 secret resolver 决定，MVP 至少保证脱敏和不展开到日志。
* 支持工具原生配置模板生成，把最终合并配置渲染成插件工具需要的配置文件，例如 `backup.conf` 或 `values.yaml`。
* 仍不支持按 UI 分类共享配置，因为分类不是安全边界。


### Config file convention

配置文件分为两类：

```text
plugins/<plugin-id>/configs/default.yaml      # 插件包自带默认配置，可随插件分发
configs/plugins/<plugin-id>.yaml              # 宿主环境覆盖配置，不随插件分发
```

例如：

```text
plugins/lzyh.es_backup/configs/default.yaml
configs/plugins/lzyh.es_backup.yaml
```

插件包自带默认配置用于让插件开箱可读、可示例化；宿主环境覆盖配置用于生产环境差异，例如真实 ES 地址、NAS 路径、账号等。

框架加载插件 `lzyh.es_backup` 时：

1. 自动尝试读取插件目录内默认配置：`plugins/lzyh.es_backup/configs/default.yaml`。
2. 自动尝试读取宿主覆盖配置：`configs/plugins/lzyh.es_backup.yaml`。
3. 两者都不存在时跳过；存在但解析失败时返回可读错误。

不推荐让插件在 `plugin.yaml` 中声明任意宿主配置路径，因为那会把插件包变成宿主文件系统路径的决策者，安全边界不清晰。

### YAML shape

全局配置放在 `configs/ops.yaml`：

```yaml
config_defaults:
  log_dir: runs/logs
  nas_root: /nas/ops

plugins:
  paths:
    - plugins
```

插件包内共享默认配置放在 `plugins/<plugin-id>/plugin.yaml`，这是随插件发布的默认值：

```yaml
id: lzyh.es_backup
shared_config:
  es_host: 127.0.0.1
  es_port: "9200"
  nas_path: "{{ .nas_root }}/es_backup"

contributes:
  tools:
    - id: lzyh.es_backup.create
      config_defaults:
        action: backup
        on_exist: skip
        retain_snapshots: "7"
      pass_mode:
        param_file: true
        file_name: params.yaml
```

宿主环境覆盖放在统一配置目录，而不是插件包内：

```yaml
# configs/plugins/lzyh.es_backup.yaml
es_host: es-prod.local
es_port: "9200"
es_user: elastic
nas_path: /mnt/nas/es_backup
```

### Generated native config templates

插件工具可以声明一个或多个原生配置模板，由框架在运行目录生成文件，再通过参数或环境变量传给脚本。

示例：

```yaml
contributes:
  tools:
    - id: lzyh.es_backup.create
      config_templates:
        - name: backup_conf
          template: configs/templates/backup.conf.tmpl
          output: backup.conf
          env: ES_BACKUP_CONFIG
          arg: --config
      pass_mode:
        param_file: true
        file_name: params.yaml
```

模板规则：

* `template` 必须是插件目录内相对路径，不能逃逸插件目录。
* `output` 只允许文件名或安全相对路径，生成到 `runs/logs/<run-id>/generated/` 下。
* 生成文件路径可通过指定 `env` 注入环境变量，也可通过 `arg` 自动追加到命令参数。
* 模板只使用最终合并配置作为上下文。
* 生成文件若包含敏感字段，不写入 stdout/stderr；运行记录只记录生成文件相对路径和脱敏摘要。


### Directory layout

推荐目录结构：

```text
configs/
  ops.yaml                              # 框架主配置：全局 config_defaults、插件扫描路径、server/ui 等
  plugins/                              # 宿主环境插件配置目录，不随插件包分发
    <plugin-id>.yaml                    # 某个插件的宿主覆盖配置，例如生产环境地址/账号/路径

plugins/
  <plugin-id>/                          # 插件包目录，可导入/导出/分发
    plugin.yaml                         # 插件 manifest：元数据、shared_config、工具定义、tool config_defaults
    configs/
      default.yaml                      # 插件自带默认配置，随插件分发，用于示例/默认值
      examples/                         # 可选：示例配置，不自动加载
        dev.yaml
        prod.example.yaml
    scripts/                            # 插件脚本/wrapper，只读取最终 params 或插件内安全资源
      run.sh
    workflows/                          # 可选：插件贡献工作流
      *.yaml
    README.md                           # 可选：插件使用说明

runs/
  logs/
    <run-id>/
    generated/                         # 本次运行生成的工具原生配置文件目录
      backup.conf
      params.yaml                       # 本次运行最终合并配置，仅运行产物
      stdout.log
      stderr.log
      result.json
```

自动加载规则：

1. `configs/ops.yaml` 总是加载。
2. 对每个插件 `<plugin-id>`，框架自动尝试加载 `plugins/<plugin-id>/configs/default.yaml`。
3. 框架自动尝试加载 `configs/plugins/<plugin-id>.yaml` 作为宿主覆盖。
4. 运行时如果开启 `pass_mode.param_file`，最终合并结果写入 `runs/logs/<run-id>/params.yaml`。

目录职责：

* `plugins/<plugin-id>/configs/default.yaml`：插件作者提供的默认配置，可随插件包导入/导出。
* `configs/plugins/<plugin-id>.yaml`：部署人员维护的环境配置，不随插件包导出，优先级高于插件默认配置。
* `runs/logs/<run-id>/params.yaml`：执行时生成的最终有效配置，供工具读取和审计。
* `runs/logs/<run-id>/generated/`：执行时生成的工具原生配置文件目录，例如由模板渲染出的 `backup.conf`、`values.yaml`。

### Config merge behavior

* 嵌套 YAML 使用 map 深合并：同一路径下 map 与 map 递归合并，标量/数组由高优先级整体覆盖低优先级。
* `--set` 支持点路径覆盖，例如 `--set es.host=prod-es.local`；需要保留现有扁平 key 兼容，若 key 中包含点号则按点路径解析。
* 最终 `params.yaml` 保留嵌套结构。
* 传给 env/args 的单值参数仍需要字符串化；复杂值只通过 `OPS_PARAM_FILE` 或模板使用。
* 工具 `parameters` 仍用于声明可交互/可展示/可校验的运行时参数，配置文件中的额外 key 允许存在。

### Sensitive config behavior

* `Parameter` 可增加 `sensitive: true`。
* 工具/插件可声明敏感路径，例如 `sensitive_paths: [es.password, token]`。
* 常见敏感 key 默认脱敏：包含 `password`、`passwd`、`secret`、`token`、`key` 的路径。
* 运行记录、API 返回、Web 展示、错误摘要中不展示敏感值。
* secret 引用值例如 `secret:env:ES_PASSWORD`、`secret:file:configs/secrets/es_password` 默认按敏感值处理；第一版可以只透传引用，不解析真实密钥。

### Data flow

1. CLI/API/Web/Workflow 解析目标 tool ID。
2. Registry 定位工具所属插件，并携带插件目录、插件默认配置和工具默认配置元数据。
3. Config/Runner 合并：参数默认值、全局默认、插件 `shared_config`、插件自带 `configs/default.yaml`、宿主 `configs/plugins/<plugin-id>.yaml`、工具默认配置、运行时参数。
4. Runner 写入 run 目录 `params.yaml`。
5. Runner 通过环境变量 `OPS_PARAM_FILE` 和参数 `--params-file` 传给脚本。
6. 运行记录保存最终参数摘要；敏感字段脱敏作为后续增强。

### Security rules

* 插件内 `shared_config` 和 `configs/default.yaml` 可以随插件发布，但只能作为默认值。
* 插件默认配置文件固定为插件目录内 `configs/default.yaml`，不能通过 manifest 指向插件目录外。
* 宿主插件配置文件固定从 `configs/plugins/<plugin-id>.yaml` 读取。
* 不支持插件声明任意宿主配置文件路径。
* 插件 ID 必须先通过已有安全校验，才能映射到配置文件名。
* 禁止绝对路径、`..`、路径分隔符等影响配置文件定位的插件 ID。
* 模板文件必须位于插件目录内，模板输出只能写入当前 run 目录的 `generated/` 子目录。
* 敏感字段必须脱敏，secret 引用默认不在日志或 API 响应中展开。
* 最终 run 目录参数文件和生成配置文件建议权限 `0600`。

### Implementation impact

Backend:

* `internal/config/types.go`：给 `RootConfig` 增加 `ConfigDefaults`，给 plugin manifest 类型增加 `SharedConfig`、`SensitivePaths`，给 tool 增加 `ConfigDefaults`、`ConfigTemplates`，给 `Parameter` 增加 `Sensitive`。
* `internal/config/load.go`：加载全局默认，提供嵌套 YAML 加载、深合并、点路径覆盖、脱敏 helper，读取 `configs/plugins/<plugin-id>.yaml`。
* `internal/plugin/*`：解析并校验插件级共享配置、插件默认配置文件、模板路径；确保插件 ID 到配置文件路径的映射安全。
* `internal/registry/registry.go`：归一化工具时保留插件 ID、共享配置、默认配置、工具配置默认值、敏感路径和模板元数据。
* `internal/runner/runner.go`：执行前合并配置、写最终 `params.yaml`、渲染工具原生配置模板、注入模板 env/arg。
* `internal/server/server.go`：API/Web 执行路径复用同一合并逻辑，并返回脱敏后的参数/配置摘要。

Tests:

* 全局 < 插件 shared_config < 插件默认文件 < 工具默认 < 宿主插件配置 < 运行时覆盖顺序。
* 嵌套 YAML 深合并和数组/标量覆盖行为。
* `--set` 点路径覆盖和原有扁平参数兼容。
* `plugins/<plugin-id>/configs/default.yaml` 与 `configs/plugins/<plugin-id>.yaml` 自动加载。
* unsafe plugin ID、模板路径、模板输出路径不会造成路径逃逸。
* `pass_mode.param_file` 输出最终合并后的嵌套参数。
* 工具原生配置模板渲染到 `runs/logs/<run-id>/generated/`，并可通过 env/arg 传给工具。
* 敏感字段和 secret 引用在运行记录/API/Web 摘要中脱敏。
* CLI `--params` / `--set` 与 API params 行为一致。
* Workflow node params 覆盖工具默认配置。

Frontend:

* MVP 可不做复杂 UI，只展示最终参数默认值。
* 后续可在插件管理页展示 `configs/plugins/<plugin-id>.yaml` 是否存在。

### Example: lzyh.es_backup

ES 连接、认证方式、NAS 路径适合放 `configs/plugins/lzyh.es_backup.yaml`；插件包内 `shared_config` 只提供开发/示例默认值；备份保留策略、恢复重命名规则适合放工具级默认；恢复日期、是否确认、dry-run 适合运行时参数。

### Out of Scope

* 按 UI 分类共享配置。
* 插件在 `plugin.yaml` 中声明任意宿主配置文件路径。
* 自动连接外部 Secret Manager；第一版只支持 secret 引用透传和脱敏。
* 图形化配置编辑器；第一版可以通过文件和参数运行。


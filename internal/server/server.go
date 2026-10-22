package server

import (
	"embed"
)

const (
	userWorkflowPluginID      = "user.workflows"
	userWorkflowPluginName    = "用户工作流"
	userWorkflowPluginVersion = "1.0.0"
)

//go:embed web/*
var webFiles embed.FS

const toolDevKitReadme = `# 插件开发包

这个压缩包面向插件开发者，帮助你制作可交付给宿主运行环境或插件接入方的插件 ZIP。文档只描述插件包本身的目录、清单、脚本、参数、工作流、确认、安全、版本、打包交付、验证预期和常见错误。

## 开发流程

1. 解压开发包。
2. 复制 plugins/plugin.template，并重命名为你的插件 ID，例如 plugins/vendor.backup。
3. 修改插件目录内的 plugin.yaml、scripts/run.sh、README.md 和 examples/ 示例。
4. 在本地验证环境中把插件目录放到 plugins/<plugin-id>/ 下。
5. 运行 ./bin/opsctl.exe validate，确认 plugin.yaml、脚本路径和 workflow 引用有效。
6. 运行 ./bin/opsctl.exe run tool <插件工具ID> --set key=value --no-prompt 验证普通工具行为。
7. 对 confirm.required=true 的工具或工作流，按接入方流程完成确认后再执行。
8. 只压缩完成后的单个插件目录，并将 ZIP 交付给宿主运行环境或插件接入方。

## 开发包内容

- plugins/plugin.template/plugin.yaml：插件清单，声明分类、普通工具、高风险工具、workflow、参数和确认策略。
- plugins/plugin.template/scripts/run.sh：示例工具脚本，包含 usage、参数解析、未知参数拒绝、必填校验和错误返回。
- plugins/plugin.template/workflows/maintenance-flow.yaml：插件内 workflow 示例，引用本插件工具并展示 depends_on 依赖。
- plugins/plugin.template/config/example.conf：插件内配置文件示例，推荐由工具声明 config_dir: config 和 config_files: [example.conf] 后由脚本读取。
- plugins/plugin.template/examples/params.yaml：本地验证参数示例。
- plugins/plugin.template/README.md：插件开发者交付给使用方的说明模板。

## 模板定位

本模板用于复制插件结构和编写习惯，不代表任何真实业务逻辑。请替换插件 ID、分类、参数、脚本动作、风险说明和回滚说明后再交付。

## 插件开发者交付清单

- plugin.yaml 的 id、name、version、description、author 和 compatibility 已替换为真实插件信息。
- contributes.categories、contributes.tools、contributes.workflows 只声明本插件提供的能力。
- 每个工具的 command、workdir、workflow path 都留在插件目录内部。
- parameters 与脚本读取的环境变量、args 名称一致，必填参数已设置 required: true。
- 高风险工具或工作流已配置 confirm.required 和清晰的 confirm.message。
- scripts/run.sh 能解析参数、拒绝未知参数、校验必填参数，并在失败时返回非 0。
- README.md 已写清输入、输出、风险、回滚方式和联系人。
- examples/ 中的参数可直接用于 validate 后的 run tool / run workflow 验证。

## 打包与交付

推荐 ZIP 结构二选一：

- ZIP 根目录直接包含 plugin.yaml。
- ZIP 根目录只包含一个插件目录，插件目录内包含 plugin.yaml。

不要把整个开发包原样交付；请交付你完成后的单个插件目录 ZIP。不要假设交付时会执行脚本；脚本只应在宿主运行环境按工具或 workflow 调用时执行。

交付前建议在本地验证环境运行 ./bin/opsctl.exe validate、./bin/opsctl.exe run tool 和必要的 ./bin/opsctl.exe run workflow。需要生成离线分发包时再运行 ./bin/opsctl.exe package build。

更新已存在插件时必须提升 version；同版本或更低版本通常应被拒绝，避免误覆盖已安装插件。
`

const toolDevKitSpec = `# 插件开发规范

本规范只约束插件开发者如何制作可交付、可运行的插件包；不涉及宿主实现细节。

## 1. 插件目录

每个插件是一个独立目录，推荐结构：

- plugin.yaml：必需，插件元数据和 contributes 声明。
- scripts/：工具脚本目录，脚本必须留在插件目录内部。
- workflows/：可选，插件贡献的 workflow YAML。
- examples/：可选，参数文件和运行命令示例。
- README.md：建议提供，说明功能、输入、输出、风险和回滚。

插件目录通常安装到宿主运行环境的 plugins/<plugin-id>/ 下。

## 2. plugin.yaml

关键字段：

- id：稳定的点分命名，例如 vendor.backup。不要使用斜杠、反斜杠，也不要使用单独的 . 或 ..。
- name/version/description/author：用于识别插件和比较版本。
- compatibility：声明适配的运行工具版本范围。
- contributes.categories：插件提供的分类。
- contributes.tools：插件工具列表。
- contributes.workflows：插件 workflow 文件列表。

工具字段要求：

- 工具 id 必须以插件 id 加点号开头，例如 vendor.backup.full。
- category 应引用 contributes.categories 中的分类 ID。
- command 必填；带路径命令必须指向插件目录内部文件，例如 scripts/run.sh；裸命令名需由管理员加入 plugins.allowed_commands 后才会走运行环境 PATH。
- workdir 可选，默认 .，也必须留在插件目录内部。
- args 可选，支持 '{{ .参数名 }}' 模板；模板名应与 parameters 中的 name 一致。
- timeout 建议显式填写，例如 1m、30m，避免长时间挂起。
- tags 建议填写，便于接入方理解工具用途和风险类别。
- parameters 必须列出脚本需要的输入，并包含 type、description、required、default。
- config_dir 可选，推荐填写相对插件目录的配置基准目录，例如 config；内部场景可谨慎填写 ../../shared-config 这类共享配置目录，也可以填写当前平台可识别的绝对路径。
- config_files 可选，推荐声明相对 config_dir 的短文件名或相对目录项，例如 example.conf；脚本是否读取该文件由 args 或脚本默认逻辑决定。
- confirm.required=true 用于高风险工具；message 应写清楚影响范围、目标环境和是否可回滚。

workflow 引用要求：

- contributes.workflows[].path 指向插件目录内的 workflow YAML，例如 workflows/maintenance-flow.yaml。
- workflow 节点的 tool 字段可引用本插件工具 ID。
- 节点可以用 depends_on 描述依赖，形成清晰的 DAG 执行顺序。
- 如果 workflow 包含高风险工具，可以在 workflow.confirm 或节点 confirm 中表达确认策略。

## 3. 参数传递

宿主运行环境会把参数传给插件工具：

- 环境变量：OPS_PARAM_<参数名大写>，参数名中的 - 会转成 _。
- 命令参数：plugin.yaml 中 args 模板渲染后附加到脚本命令。
- 参数文件：当宿主运行环境生成参数文件时，OPS_PARAM_FILE 指向该 YAML 文件。

脚本应当同时能处理环境变量和 args，至少要对必填参数做校验。校验失败时输出简短错误到 stderr，并返回非 0 退出码。

## 4. 配置文件

如果工具需要插件内配置文件，推荐把文件放在插件目录内的 config/ 目录，并在 plugin.yaml 的工具声明中使用 config_dir 作为配置基准目录，config_files 只写相对 config_dir 的短文件名或相对目录项：

` + "```yaml" + `
config_dir: config
config_files:
  - example.conf
` + "```" + `

含义与约束：

- config_dir 是配置基准目录：相对路径按插件目录解析，上例中 example.conf 会按插件目录内的 config/example.conf 处理；内部共享配置也可以写成 ../../shared-config；当前平台可识别的绝对路径会直接作为配置基准目录。
- 旧写法仍兼容：未声明 config_dir 时，config_files: [config/example.conf] 会继续按插件目录内路径识别，便于旧插件平滑升级；新模板推荐短文件名 + config_dir。
- config_files 中的字符串条目，以及结构化条目的 path，只能是相对文件或相对目录项，禁止绝对路径，禁止使用 .. 或其他方式逃逸最终解析出的 config_dir。
- 如果 config_files/path 指向目录，目录项只会一级展开普通文件，不递归进入子目录，也不会包含目录、符号链接或特殊文件。
- 需要更细粒度的宿主路径白名单、只读/可写权限或稳定文件 ID 管控时，可由管理员通过 host_config_files.allowed_dirs 与 host-side mapping 显式启用 scope: host_absolute。

config_files 只用于声明哪些文件可被配置维护；宿主不会自动生成、复制或传参。工具脚本应通过 args、默认路径或自己的逻辑读取这些文件。

## 5. 脚本可靠性

- 使用 set -euo pipefail，避免忽略失败。
- 明确解析参数，遇到未知参数返回非 0。
- 必填参数为空时返回非 0。
- 使用 dry-run 或 action 参数表达真实执行意图。
- 参数错误写 stderr，正常进度写 stdout，便于排障。
- 不要在 stdout/stderr 输出密码、令牌、密钥、完整连接串等敏感信息。
- 不要假设交付或接入时会执行脚本；所有运行前检查都应放在 validate 或工具启动时完成。
- 输出应聚焦执行进度和结果，方便通过运行日志排障。
- 修改外部系统的工具必须在 README.md 写清楚影响范围和回滚方式。

## 6. 验证、运行、打包、交付

在本地验证环境中安装到 plugins/<plugin-id>/ 后验证：

` + "```bash" + `
./bin/opsctl.exe validate
./bin/opsctl.exe list
./bin/opsctl.exe run tool plugin.template.inspect --set target=demo --set action=inspect --set dry_run=true --no-prompt
./bin/opsctl.exe run workflow plugin.template.maintenance-flow --set target=demo --set action=inspect --set dry_run=true --no-prompt
./bin/opsctl.exe package build
` + "```" + `

打包交付建议：

1. 只压缩单个插件目录，确保 ZIP 中有且只有一个 plugin.yaml 所在插件根目录。
2. 交付前先本地运行 validate 和至少一次 run tool。
3. 记录预期的分类、工具、workflow、confirm 信息，便于接入方核对。
4. 更新已存在插件时提升 version；同版本或更低版本通常应被拒绝更新。

## 7. 常见问题

- validate 提示 command 不存在：确认 command 路径相对插件目录，且文件已打入 ZIP。
- validate 提示路径不安全：不要使用绝对路径或 ../ 跳出插件目录。
- 工具未被识别：确认插件目录位于宿主运行环境约定的插件目录下，工具 id 前缀与插件 id 一致。
- workflow 找不到工具：确认 workflow 节点 tool 使用完整插件工具 ID，且该工具在同一个 plugin.yaml 中声明。
- 参数为空：确认 parameters 名称、args 模板和脚本读取的 OPS_PARAM_ 名称一致。
- 需要二次确认：将 confirm.required 设为 true，并提供清晰 message。
- 接入方提示已有插件：如果确实要更新，提升 version 后重新交付。
- ZIP 结构无效：确认 ZIP 不是整个开发包，且根目录直接是插件目录或 plugin.yaml。
`

const samplePluginYAML = `id: plugin.template
name: 规范插件模板
version: 1.0.0
description: 可复制的规范插件模板，展示清单、脚本、参数、确认和工作流写法
author: your-team
compatibility:
  opsctl: ">=0.1.0"
contributes:
  categories:
    - id: plugin-template
      name: 插件模板
      description: 插件模板示例分类
  tools:
    - id: plugin.template.inspect
      name: 目标检查
      description: 普通只读工具示例，检查目标状态并输出摘要
      category: plugin-template
      tags: [plugin, template, readonly]
      command: scripts/run.sh
      args:
        - --target
        - '{{ .target }}'
        - --action
        - inspect
        - --dry-run
        - '{{ .dry_run }}'
        - --config
        - config/example.conf
      config_dir: config
      config_files:
        - example.conf
      workdir: .
      timeout: 1m
      parameters:
        - name: target
          type: string
          description: 目标标识，例如主机组、实例名或环境名
          required: true
          default: demo
        - name: action
          type: string
          description: 执行动作，普通工具固定为 inspect
          required: false
          default: inspect
        - name: dry_run
          type: bool
          description: 是否仅预览动作，不修改外部系统
          required: false
          default: true
      confirm:
        required: false
        message: ""
    - id: plugin.template.apply
      name: 变更执行
      description: 高风险工具示例，展示 confirm.required 和 dry-run 保护
      category: plugin-template
      tags: [plugin, template, change, high-risk]
      command: scripts/run.sh
      args:
        - --target
        - '{{ .target }}'
        - --action
        - '{{ .action }}'
        - --dry-run
        - '{{ .dry_run }}'
        - --config
        - config/example.conf
      config_dir: config
      config_files:
        - example.conf
      workdir: .
      timeout: 5m
      parameters:
        - name: target
          type: string
          description: 目标标识，例如主机组、实例名或环境名
          required: true
          default: demo
        - name: action
          type: string
          description: 执行动作，示例支持 apply 或 inspect
          required: true
          default: apply
        - name: dry_run
          type: bool
          description: 是否仅预览动作；生产变更建议先保持 true
          required: false
          default: true
      confirm:
        required: true
        message: 确认对目标执行变更示例？请确认目标、动作和回滚方案已核对。
  workflows:
    - path: workflows/maintenance-flow.yaml
`

const sampleRunScript = `#!/usr/bin/env bash
set -euo pipefail

target="${OPS_PARAM_TARGET:-}"
action="${OPS_PARAM_ACTION:-inspect}"
dry_run="${OPS_PARAM_DRY_RUN:-true}"
config_file="config/example.conf"

usage() {
  cat >&2 <<'EOF'
用法: run.sh --target <target> [--action inspect|apply] [--dry-run true|false] [--config config/example.conf]

参数:
  --target    必填。目标标识，例如主机组、实例名或环境名。
  --action    可选。inspect 只读检查；apply 表示执行变更示例。
  --dry-run   可选。true 仅预览；false 表示执行真实动作。
  --config    可选。插件内配置文件路径，默认 config/example.conf。
EOF
}

error() {
  echo "错误: $*" >&2
}

info() {
  echo "$*"
}

normalize_bool() {
  case "${1,,}" in
    true|yes|1|on) echo "true" ;;
    false|no|0|off) echo "false" ;;
    *) return 1 ;;
  esac
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      if [[ $# -lt 2 || -z "${2:-}" ]]; then
        error "--target 需要非空参数"
        usage
        exit 2
      fi
      target="$2"
      shift 2
      ;;
    --action)
      if [[ $# -lt 2 || -z "${2:-}" ]]; then
        error "--action 需要非空参数"
        usage
        exit 2
      fi
      action="$2"
      shift 2
      ;;
    --dry-run)
      if [[ $# -lt 2 || -z "${2:-}" ]]; then
        error "--dry-run 需要 true 或 false"
        usage
        exit 2
      fi
      if ! dry_run="$(normalize_bool "$2")"; then
        error "--dry-run 只接受 true/false、yes/no、1/0、on/off"
        usage
        exit 2
      fi
      shift 2
      ;;
    --config)
      if [[ $# -lt 2 || -z "${2:-}" ]]; then
        error "--config 需要文件路径"
        usage
        exit 2
      fi
      config_file="$2"
      shift 2
      ;;
    --params-file)
      if [[ $# -lt 2 || -z "${2:-}" ]]; then
        error "--params-file 需要文件路径"
        usage
        exit 2
      fi
      export OPS_PARAM_FILE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      error "未知参数: $1"
      usage
      exit 2
      ;;
  esac
done

if [[ -z "$target" ]]; then
  error "缺少必填参数 target"
  usage
  exit 1
fi

case "$action" in
  inspect|apply) ;;
  *)
    error "action 只支持 inspect 或 apply"
    usage
    exit 2
    ;;
esac

if ! dry_run="$(normalize_bool "$dry_run")"; then
  error "dry-run 只接受 true/false、yes/no、1/0、on/off"
  usage
  exit 2
fi

if [[ -n "${OPS_PARAM_FILE:-}" ]]; then
  info "已接收参数文件"
fi

if [[ ! -f "$config_file" ]]; then
  error "配置文件不存在: $config_file"
  exit 1
fi

# 不要输出密码、令牌、密钥、完整连接串等敏感信息。
info "插件工具开始执行"
info "目标: ${target}"
info "动作: ${action}"
info "dry-run: ${dry_run}"
info "配置文件: ${config_file}"

if [[ "$action" == "inspect" ]]; then
  info "检查完成: 示例状态正常"
  info "插件工具执行完成"
  exit 0
fi

if [[ "$dry_run" == "true" ]]; then
  info "预览完成: 将执行变更示例，但未修改任何外部系统"
else
  info "变更完成: 已执行示例动作，请根据真实插件 README 核对结果"
fi

info "插件工具执行完成"
`

const samplePluginWorkflowYAML = `id: plugin.template.maintenance-flow
name: 插件模板维护流程
description: 演示插件内 workflow 如何引用本插件工具，并用 depends_on 描述 DAG 依赖
version: 1.0.0
category: plugin-template
tags: [plugin, template, workflow]
parameters:
  - name: target
    type: string
    description: 目标标识，例如主机组、实例名或环境名
    required: true
    default: demo
  - name: action
    type: string
    description: 变更动作，示例默认 apply
    required: true
    default: apply
  - name: dry_run
    type: bool
    description: 是否仅预览动作，不修改外部系统
    required: false
    default: true
nodes:
  - id: inspect
    name: 变更前检查
    tool: plugin.template.inspect
    params:
      target: "{{ .target }}"
      action: inspect
      dry_run: true
  - id: apply
    name: 执行变更示例
    tool: plugin.template.apply
    depends_on: [inspect]
    params:
      target: "{{ .target }}"
      action: "{{ .action }}"
      dry_run: "{{ .dry_run }}"
edges:
  - from: inspect
    to: apply
confirm:
  required: true
  message: 确认执行插件模板维护流程？请确认目标、动作和回滚方案已核对。
`

const samplePluginReadme = `# 规范插件模板

## 功能

这个插件提供普通工具 plugin.template.inspect、高风险工具 plugin.template.apply 和 workflow plugin.template.maintenance-flow。它是可复制的规范模板，用来展示插件目录、manifest 字段、脚本参数解析、confirm 配置、depends_on 依赖和交付说明；它不是业务逻辑。

复制模板后，请把插件 ID、分类、工具 ID、脚本逻辑、风险说明、回滚说明和联系人改成你的真实插件含义。

## 目录说明

- plugin.yaml：声明插件元数据、分类、工具、workflow、参数和 confirm。
- scripts/run.sh：工具脚本，演示 usage、参数解析、未知参数拒绝、必填校验、dry-run、错误返回和安全输出。
- workflows/maintenance-flow.yaml：插件内 workflow 示例，引用本插件工具并使用 depends_on 表达依赖。
- config/example.conf：插件内可编辑配置文件示例，plugin.yaml 中通过 config_dir: config 与 config_files: [example.conf] 声明。
- examples/params.yaml：本地运行参数示例。

## 输入

- target：目标标识，必填。
- action：执行动作，示例支持 inspect 或 apply。
- dry_run：是否仅预览动作，默认 true。
- config/example.conf：插件内配置文件示例，工具脚本通过 --config 读取。

## 配置文件

plugin.yaml 推荐使用 config_dir: config 作为相对插件目录的配置基准目录，再在 config_files 中声明相对该目录的短文件名或相对目录项，例如 example.conf 对应插件目录内的 config/example.conf。内部共享配置场景可以谨慎使用 config_dir: ../../shared-config。旧的 config_files: [config/example.conf] 写法仍兼容，但新模板推荐短文件名 + config_dir。

config_dir 支持相对插件目录或当前平台可识别的绝对路径；config_files 的字符串条目和结构化条目的 path 只能是相对文件或相对目录项，禁止绝对路径和 .. 逃逸最终解析出的 config_dir。目录项只一级展开普通文件，不递归。需要更细粒度的宿主绝对路径配置文件映射时，可由管理员通过 host_config_files.allowed_dirs 和宿主侧 mapping 中的 scope: host_absolute、config_dir、path 显式启用。

## 输出

stdout 会输出执行进度和结果；stderr 只输出参数错误或执行错误。不要输出密码、令牌、密钥、完整连接串等敏感信息。

## 风险与确认

plugin.template.inspect 是普通只读示例，confirm.required 为 false。

plugin.template.apply 是高风险示例，confirm.required 为 true。真实插件如果会删除、覆盖、重启、变更生产配置或影响业务，请保留确认策略，并把 confirm.message 写清目标、动作、影响范围和回滚要求。

workflow plugin.template.maintenance-flow 包含高风险节点，因此 workflow 自身也配置 confirm.required: true。

## 本地验证

将插件目录安装到本地验证环境的 plugins/plugin.template 后运行：

` + "```bash" + `
./bin/opsctl.exe validate
./bin/opsctl.exe list
./bin/opsctl.exe run tool plugin.template.inspect --set target=demo --set action=inspect --set dry_run=true --no-prompt
printf '确认\n' | ./bin/opsctl.exe run tool plugin.template.apply --set target=demo --set action=apply --set dry_run=true --no-prompt
printf '确认\n确认\n' | ./bin/opsctl.exe run workflow plugin.template.maintenance-flow --set target=demo --set action=apply --set dry_run=true --no-prompt
` + "```" + `

## 打包交付

只压缩这个插件目录。ZIP 根目录可以直接是 plugin.yaml，也可以是 plugin.template/plugin.yaml。不要把上层开发包目录或无关文件一起交付。

如果交付的是已存在插件的新版本，请先提升 plugin.yaml 的 version；只有版本高于已安装版本时才应替换。

## 安全与运维

- 不要把密码、令牌、密钥或生产连接串打进 ZIP。
- 不要依赖插件目录外的相对路径；command、workdir、workflow path 都应留在插件目录内部。
- 根据实际耗时设置 timeout，避免长时间占用运行队列。
- 不要假设交付或接入时会自动执行工具；上线前仍需手动 run tool / run workflow 验证。
- 高风险动作必须先 dry-run，再按变更窗口和回滚方案执行。

## 回滚

如果插件工具会修改系统状态，请在这里写清楚回滚步骤、影响范围和联系人。
`

const sampleConfigFile = `[service]
name = TemplateService
endpoint = https://api.example.com
timeout = 30s

[options]
debug = false
dry_run_default = true
`

const sampleParamsYAML = `target: demo
action: apply
dry_run: true
`

const sampleExamplesReadme = `# 示例

## 参数文件

params.yaml 是模板工具参数示例，可用于参数输入或命令行 --set 对照。

## 本地命令

` + "```bash" + `
./bin/opsctl.exe validate
./bin/opsctl.exe run tool plugin.template.inspect --set target=demo --set action=inspect --set dry_run=true --no-prompt
printf '确认\n' | ./bin/opsctl.exe run tool plugin.template.apply --set target=demo --set action=apply --set dry_run=true --no-prompt
printf '确认\n确认\n' | ./bin/opsctl.exe run workflow plugin.template.maintenance-flow --set target=demo --set action=apply --set dry_run=true --no-prompt
` + "```" + `

## 交付前检查

交付 ZIP 前，请核对：

- 分类：插件模板
- 普通工具：plugin.template.inspect
- 高风险工具：plugin.template.apply
- 工作流：plugin.template.maintenance-flow
- confirm.required 示例已按真实风险调整
`

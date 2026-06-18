package server

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"shell_ops/internal/config"
	"shell_ops/internal/plugin"
	"shell_ops/internal/registry"
	"shell_ops/internal/runbundle"
	"shell_ops/internal/runner"
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

type catalogResponse struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Categories  []categoryCatalogEntry `json:"categories"`
	Tools       []toolCatalogEntry     `json:"tools"`
	Workflows   []workflowCatalogEntry `json:"workflows"`
	Plugins     []pluginCatalogEntry   `json:"plugins"`
	Warnings    []plugin.Warning       `json:"warnings,omitempty"`
}

type categoryCatalogEntry struct {
	config.Category
	Disabled bool             `json:"disabled"`
	Source   *registry.Source `json:"source,omitempty"`
}

type toolCatalogEntry struct {
	config.ToolEntry
	Tags           []string               `json:"tags"`
	Execution      config.ExecutionConfig `json:"execution"`
	Parameters     []config.Parameter     `json:"parameters"`
	Outputs        []config.ToolOutput    `json:"outputs"`
	ConfigFiles    []string               `json:"config_files"`
	ConfigFileRefs []config.ConfigFileRef `json:"config_file_entries,omitempty"`
	Confirm        config.Confirmation    `json:"confirm"`
	Source         registry.Source        `json:"source"`
}

type workflowCatalogEntry struct {
	config.WorkflowRef
	Tags        []string               `json:"tags"`
	Parameters  []config.Parameter     `json:"parameters"`
	ConfigFiles []config.ConfigFileRef `json:"config_files,omitempty"`
	Confirm     config.Confirmation    `json:"confirm"`
	Source      registry.Source        `json:"source"`
}

type pluginCatalogEntry struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	Description string           `json:"description,omitempty"`
	Disabled    bool             `json:"disabled"`
	Warnings    []plugin.Warning `json:"warnings,omitempty"`
}

type runRequest struct {
	Params        map[string]interface{}                 `json:"params"`
	Confirm       bool                                   `json:"confirm"`
	Workflow      *config.WorkflowConfig                 `json:"workflow,omitempty"`
	Uploads       map[string]config.WorkflowUploadResult `json:"uploads,omitempty"`
	ConfigVersion string                                 `json:"config_version,omitempty"`
}

type workflowSaveRequest struct {
	Workflow config.WorkflowConfig `json:"workflow"`
}

type workflowValidation struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

type runLogs struct {
	Stdout string             `json:"stdout,omitempty"`
	Stderr string             `json:"stderr,omitempty"`
	Steps  map[string]runLogs `json:"steps,omitempty"`
	Items  []runLogItem       `json:"items,omitempty"`
}

type runLogItem struct {
	ID        string       `json:"id"`
	Kind      string       `json:"kind"`
	Title     string       `json:"title"`
	Status    string       `json:"status,omitempty"`
	Type      string       `json:"type,omitempty"`
	Tool      string       `json:"tool,omitempty"`
	Stdout    string       `json:"stdout,omitempty"`
	Stderr    string       `json:"stderr,omitempty"`
	Children  []runLogItem `json:"children,omitempty"`
	Iteration int          `json:"iteration,omitempty"`
}

type runLogEvent struct {
	Seq       int    `json:"seq"`
	RunID     string `json:"run_id"`
	ItemID    string `json:"item_id"`
	Kind      string `json:"kind"`
	StepID    string `json:"step_id,omitempty"`
	Iteration int    `json:"iteration,omitempty"`
	Stream    string `json:"stream"`
	Text      string `json:"text"`
	Status    string `json:"status"`
}

type runDetail struct {
	Record runner.RunRecord `json:"record"`
	Logs   runLogs          `json:"logs"`
}

type response struct {
	ID     string      `json:"id,omitempty"`
	Status string      `json:"status,omitempty"`
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
}

type serverState struct {
	mu     sync.RWMutex
	reg    *registry.Registry
	runMu  sync.Mutex
	active map[string]context.CancelFunc
}

func newServerState(reg *registry.Registry) *serverState {
	return &serverState{reg: reg, active: map[string]context.CancelFunc{}}
}

func (s *serverState) registry() *registry.Registry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reg
}

func (s *serverState) swap(reg *registry.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reg = reg
}

func (s *serverState) registerRun(id string, cancel context.CancelFunc) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.active[id] = cancel
}

func (s *serverState) finishRun(id string) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	delete(s.active, id)
}

func (s *serverState) hasActiveRun(id string) bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	return s.active[id] != nil
}

func (s *serverState) cancelRun(id string) bool {
	s.runMu.Lock()
	cancel := s.active[id]
	s.runMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (s *serverState) reconcileRunRecord(reg *registry.Registry, id string) (runner.RunRecord, error) {
	record, err := runbundle.LoadRecord(reg, id)
	if err != nil {
		return runner.RunRecord{}, err
	}
	if record.Status != "running" || s.hasActiveRun(record.ID) {
		return record, nil
	}
	record.EndedAt = time.Now()
	record.Status = "failed"
	record.Error = "运行任务已异常退出或服务已重启，当前进程没有该任务的运行句柄。"
	for index := range record.Steps {
		if record.Steps[index].Status != "running" && record.Steps[index].Status != "waiting" {
			continue
		}
		record.Steps[index].Status = "failed"
		record.Steps[index].EndedAt = record.EndedAt
		record.Steps[index].Error = "运行任务已异常退出或服务已重启。"
	}
	if err := saveRunRecord(reg, &record); err != nil {
		return runner.RunRecord{}, err
	}
	return record, nil
}

func (s *serverState) reconcileOrphanRuns(reg *registry.Registry) {
	if s == nil || reg == nil {
		return
	}
	items, err := runbundle.List(reg)
	if err != nil {
		return
	}
	for _, item := range items {
		if item.Status != "running" || s.hasActiveRun(item.ID) {
			continue
		}
		_, _ = s.reconcileRunRecord(reg, item.ID)
	}
}

func (s *serverState) startRunReconciler() {
	go func() {
		s.reconcileOrphanRuns(s.registry())
	}()
}

func saveRunRecord(reg *registry.Registry, record *runner.RunRecord) error {
	if record == nil || strings.TrimSpace(record.ID) == "" {
		return fmt.Errorf("运行记录 ID 不能为空")
	}
	runDir, err := runbundle.RunDir(reg, record.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	tempPath := filepath.Join(runDir, fmt.Sprintf("result.json.%d.tmp", time.Now().UnixNano()))
	resultPath := filepath.Join(runDir, "result.json")
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < 25; attempt++ {
		if err := os.Rename(tempPath, resultPath); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if err := os.Remove(resultPath); err != nil && !os.IsNotExist(err) {
			lastErr = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if err := os.Rename(tempPath, resultPath); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = os.Remove(tempPath)
	return lastErr
}

func ListenAndServe(addr string, reg *registry.Registry) error {
	return http.ListenAndServe(addr, NewHandler(reg))
}

func ListenAndServeWithToken(addr string, reg *registry.Registry, token string) error {
	return http.ListenAndServe(addr, tokenMiddleware(NewHandler(reg), token))
}

func NewHandler(reg *registry.Registry) http.Handler {
	state := newServerState(reg)
	state.startRunReconciler()
	mux := http.NewServeMux()
	registerWeb(mux)
	mux.HandleFunc("/api/catalog", catalogHandler(state))
	mux.HandleFunc("/api/ui/preferences", uiPreferencesHandler(state))
	mux.HandleFunc("/api/config/global", globalConfigHandler(state))
	mux.HandleFunc("/api/config/global-env", globalEnvConfigHandler(state))
	mux.HandleFunc("/api/config/global/versions/", func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/api/config/global/versions/")
		if path == "" {
			// /api/config/global/versions
			handleConfigVersions(w, req, state, "global", "ops")
			return
		}
		parts := strings.Split(path, "/")
		if len(parts) == 2 && parts[1] == "default" {
			// /api/config/global/versions/{version}/default
			handleSetDefaultVersion(w, req, state, "global", "ops", parts[0])
			return
		}
		if len(parts) == 1 {
			// /api/config/global/versions/{version}
			handleConfigVersion(w, req, state, "global", "ops", parts[0])
			return
		}
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
	})
	mux.HandleFunc("/api/files/upload", fileUploadHandler(state))
	mux.HandleFunc("/api/dev/toolkit.zip", toolDevKitHandler())
	mux.HandleFunc("/api/plugins/user-workflows.zip", userWorkflowPluginExportHandler(state))
	mux.HandleFunc("/api/plugins/upload", pluginUploadHandler(state))
	mux.HandleFunc("/api/plugins/", pluginDownloadHandler(state))
	mux.HandleFunc("/api/tools/", toolsHandler(state))
	mux.HandleFunc("/api/workflows/", workflowsHandler(state))
	mux.HandleFunc("/api/runs/", runsHandler(state))
	return mux
}

func tokenMiddleware(next http.Handler, token string) http.Handler {
	if strings.TrimSpace(token) == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if tokenAuthorized(req, token) {
			if req.URL.Query().Get("token") == token {
				http.SetCookie(w, &http.Cookie{Name: "ops_token", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
			}
			next.ServeHTTP(w, req)
			return
		}
		if req.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("缺少或无效的本地访问 token，请使用启动命令输出的 Web UI 地址访问。"))
			return
		}
		writeJSON(w, http.StatusUnauthorized, response{Error: "unauthorized"})
	})
}

func tokenAuthorized(req *http.Request, token string) bool {
	if req.URL.Query().Get("token") == token {
		return true
	}
	if req.Header.Get("X-Ops-Token") == token {
		return true
	}
	cookie, err := req.Cookie("ops_token")
	return err == nil && cookie.Value == token
}

func catalogHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, response{Data: buildCatalog(state.registry())})
	}
}

func uiPreferencesHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, response{Data: uiPreferencesFromConfig(state.registry().Root.UI)})
	}
}

func uiPreferencesFromConfig(ui config.UIConfig) map[string]interface{} {
	return map[string]interface{}{
		"log_font_size": uiLogFontSize(ui.LogFontSize),
	}
}

func uiLogFontSize(value int) int {
	const (
		defaultLogFontSize = 14
		minLogFontSize     = 12
		maxLogFontSize     = 20
	)
	if value < minLogFontSize || value > maxLogFontSize {
		return defaultLogFontSize
	}
	return value
}

func globalConfigHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reg := state.registry()
		if req.Method == http.MethodGet {
			handleGlobalConfigGet(w, reg)
			return
		}
		if req.Method == http.MethodPut {
			handleGlobalConfigPut(w, req, state)
			return
		}
		methodNotAllowed(w)
	}
}

func handleGlobalConfigGet(w http.ResponseWriter, reg *registry.Registry) {
	path := config.RootPath(reg.BaseDir)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, response{Error: "框架设置文件不存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{
		"content": string(content),
		"path":    relativePath(reg.BaseDir, path),
	}})
}

func handleGlobalConfigPut(w http.ResponseWriter, req *http.Request, state *serverState) {
	defer req.Body.Close()
	state.mu.Lock()
	defer state.mu.Unlock()
	reg := state.reg
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	// 先验证 YAML 是否有效
	var testRoot config.RootConfig
	if err := yaml.Unmarshal([]byte(body.Content), &testRoot); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: fmt.Sprintf("YAML 格式无效: %v", err)})
		return
	}

	path := config.RootPath(reg.BaseDir)
	oldContent, readErr := os.ReadFile(path)
	existed := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		writeJSON(w, http.StatusInternalServerError, response{Error: readErr.Error()})
		return
	}
	if err := os.WriteFile(path, []byte(body.Content), 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	newReg, err := registry.Load(reg.BaseDir)
	if err != nil {
		if existed {
			_ = os.WriteFile(path, oldContent, 0o644)
		} else {
			_ = os.Remove(path)
		}
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("重新加载注册表失败，已回滚配置文件: %v", err)})
		return
	}
	state.reg = newReg
	writeJSON(w, http.StatusOK, response{Status: "saved", Data: map[string]string{"message": "框架设置已保存"}})
}

func globalEnvConfigHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reg := state.registry()
		if req.Method == http.MethodGet {
			handleGlobalEnvConfigGet(w, reg)
			return
		}
		if req.Method == http.MethodPut {
			handleGlobalEnvConfigPut(w, req, state)
			return
		}
		methodNotAllowed(w)
	}
}

func handleGlobalEnvConfigGet(w http.ResponseWriter, reg *registry.Registry) {
	path := config.GlobalEnvPath(reg.BaseDir)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{
				"content": "",
				"path":    relativePath(reg.BaseDir, path),
			}})
			return
		}
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{
		"content": string(content),
		"path":    relativePath(reg.BaseDir, path),
	}})
}

func handleGlobalEnvConfigPut(w http.ResponseWriter, req *http.Request, state *serverState) {
	defer req.Body.Close()
	state.mu.Lock()
	defer state.mu.Unlock()
	reg := state.reg
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	path := config.GlobalEnvPath(reg.BaseDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	oldContent, readErr := os.ReadFile(path)
	existed := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		writeJSON(w, http.StatusInternalServerError, response{Error: readErr.Error()})
		return
	}
	if err := os.WriteFile(path, []byte(body.Content), 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	newReg, err := registry.Load(reg.BaseDir)
	if err != nil {
		if existed {
			_ = os.WriteFile(path, oldContent, 0o644)
		} else {
			_ = os.Remove(path)
		}
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("重新加载注册表失败，已回滚配置文件: %v", err)})
		return
	}
	state.reg = newReg
	writeJSON(w, http.StatusOK, response{Status: "saved", Data: map[string]string{"message": "全局环境配置已保存"}})
}

func toolsHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reg := state.registry()
		r := runner.New(reg)
		path := strings.TrimPrefix(req.URL.Path, "/api/tools/")
		if req.Method == http.MethodGet {
			id := strings.Trim(path, "/")
			if id == "" {
				writeJSON(w, http.StatusNotFound, response{Error: "not found"})
				return
			}
			tool, err := reg.Tool(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, response{Data: tool})
			return
		}
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		id, ok := strings.CutSuffix(path, "/run")
		if !ok || id == "" {
			writeJSON(w, http.StatusNotFound, response{Error: "not found"})
			return
		}
		reqBody, err := decodeRunRequest(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		tool, err := reg.Tool(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		params := config.ValuesToStringMap(config.MergeParamsValues(tool.Config.Parameters, nil, config.InterfaceMapToValues(reqBody.Params)))
		if err := config.ValidateRequired(tool.Config.Parameters, params); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		if tool.Config.Confirm.Required && !reqBody.Confirm {
			writeJSON(w, http.StatusBadRequest, response{Error: "该工具需要确认后执行"})
			return
		}
		if isAsyncRunRequest(req) {
			ctx, cancel := context.WithCancel(context.Background())
			record, err := r.StartToolValues(ctx, id, config.InterfaceMapToValues(reqBody.Params), io.Discard, io.Discard)
			if err != nil {
				cancel()
				writeRunResponse(w, record, err)
				return
			}
			state.registerRun(record.ID, cancel)
			go watchRunCompletion(state, reg, record.ID, cancel)
			writeRunResponse(w, record, err)
			return
		}
		record, err := r.RunToolValues(context.Background(), id, config.InterfaceMapToValues(reqBody.Params), io.Discard, io.Discard)
		writeRunResponse(w, record, err)
	}
}

func workflowsHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reg := state.registry()
		r := runner.New(reg)
		path := strings.TrimPrefix(req.URL.Path, "/api/workflows/")
		if req.Method == http.MethodGet {
			if strings.HasSuffix(path, "/files") || strings.Contains(path, "/files/") {
				handleWorkflowFilesRoute(w, req, state, path)
				return
			}
			handleWorkflowGet(w, reg, path)
			return
		}
		if (req.Method == http.MethodPut || req.Method == http.MethodDelete) && (strings.HasSuffix(path, "/files") || strings.Contains(path, "/files/")) {
			handleWorkflowFilesRoute(w, req, state, path)
			return
		}
		if req.Method == http.MethodDelete {
			handleWorkflowDelete(w, reg, path)
			return
		}
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if strings.HasSuffix(path, "/run") {
			handleWorkflowRun(w, req, state, reg, r, strings.TrimSuffix(path, "/run"))
			return
		}
		if strings.HasSuffix(path, "/files") || strings.Contains(path, "/files/") {
			handleWorkflowFilesRoute(w, req, state, path)
			return
		}
		if strings.HasSuffix(path, "/validate") {
			handleWorkflowValidate(w, req, reg)
			return
		}
		if strings.HasSuffix(path, "/save") {
			handleWorkflowSave(w, req, reg, strings.TrimSuffix(path, "/save"))
			return
		}
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
	}
}

func handleWorkflowFilesRoute(w http.ResponseWriter, req *http.Request, state *serverState, path string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[1] != "files" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	workflowID := parts[0]
	if len(parts) == 2 {
		handleWorkflowConfigFiles(w, req, state, workflowID)
		return
	}
	fileID, err := url.PathUnescape(strings.Join(parts[2:], "/"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件 ID 格式错误"})
		return
	}
	handleWorkflowConfigFile(w, req, state, workflowID, fileID)
}

func handleWorkflowGet(w http.ResponseWriter, reg *registry.Registry, path string) {
	id := strings.Trim(path, "/")
	if id == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	wf, err := reg.Workflow(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: wf})
}

func handleWorkflowRun(w http.ResponseWriter, req *http.Request, state *serverState, reg *registry.Registry, r *runner.Runner, id string) {
	if id == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	reqBody, err := decodeRunRequest(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	workflowConfig := reqBody.Workflow
	if workflowConfig == nil {
		wf, err := reg.Workflow(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		workflowConfig = wf.Config
	}
	params := config.ValuesToStringMap(config.MergeParamsValues(workflowConfig.Parameters, nil, config.InterfaceMapToValues(reqBody.Params)))
	if err := config.ValidateRequired(workflowConfig.Parameters, params); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if workflowConfig.Confirm.Required && !reqBody.Confirm {
		writeJSON(w, http.StatusBadRequest, response{Error: "该工作流需要确认后执行"})
		return
	}
	if err := confirmWorkflowTools(reg, workflowConfig, reqBody.Confirm); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if isAsyncRunRequest(req) {
		ctx, cancel := context.WithCancel(context.Background())
		record, err := r.StartWorkflowConfigWithUploads(ctx, workflowConfig, params, reqBody.Confirm, reqBody.Uploads, io.Discard, io.Discard)
		if err != nil {
			cancel()
			writeRunResponse(w, record, err)
			return
		}
		state.registerRun(record.ID, cancel)
		go watchRunCompletion(state, reg, record.ID, cancel)
		writeRunResponse(w, record, err)
		return
	}
	if reqBody.Workflow != nil {
		record, err := r.RunWorkflowConfigWithUploads(context.Background(), workflowConfig, params, reqBody.Confirm, reqBody.Uploads, io.Discard, io.Discard)
		writeRunResponse(w, record, err)
		return
	}
	record, err := r.RunWorkflowConfigWithUploads(context.Background(), workflowConfig, params, reqBody.Confirm, reqBody.Uploads, io.Discard, io.Discard)
	writeRunResponse(w, record, err)
}

func watchRunCompletion(state *serverState, reg *registry.Registry, runID string, cancel context.CancelFunc) {
	readFailures := 0
	for {
		time.Sleep(100 * time.Millisecond)
		detail, err := loadRunDetail(nil, reg, runID)
		if err != nil {
			readFailures++
			if readFailures < 5 {
				continue
			}
			state.finishRun(runID)
			cancel()
			return
		}
		readFailures = 0
		if detail.Record.Status != "running" {
			state.finishRun(runID)
			cancel()
			return
		}
	}
}

func isAsyncRunRequest(req *http.Request) bool {
	value := strings.ToLower(strings.TrimSpace(req.URL.Query().Get("async")))
	return value == "1" || value == "true" || value == "yes"
}

func confirmWorkflowTools(reg *registry.Registry, wf *config.WorkflowConfig, confirmed bool) error {
	for _, node := range wf.Nodes {
		nodeType := node.Type
		if nodeType == "" && node.Tool != "" {
			nodeType = config.WorkflowNodeTypeTool
		}
		if nodeType == "" && (node.Loop.Tool != "" || node.Loop.Target != "" || node.Loop.MaxIterations != 0 || len(node.Loop.Params) > 0) {
			nodeType = config.WorkflowNodeTypeLoop
		}
		toolID := node.Tool
		if nodeType == config.WorkflowNodeTypeLoop {
			toolID = node.Loop.Tool
		}
		if nodeType != config.WorkflowNodeTypeTool && nodeType != config.WorkflowNodeTypeLoop {
			continue
		}
		if toolID == "" {
			continue
		}
		tool, err := reg.Tool(toolID)
		if err != nil {
			return err
		}
		if tool.Config.Confirm.Required && !node.Confirm && !confirmed {
			return fmt.Errorf("工作流节点 %s 引用的工具 %s 需要确认", node.ID, toolID)
		}
	}
	return nil
}

func handleWorkflowValidate(w http.ResponseWriter, req *http.Request, reg *registry.Registry) {
	wf, err := decodeWorkflow(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: validateWorkflow(reg, wf)})
}

func handleWorkflowSave(w http.ResponseWriter, req *http.Request, reg *registry.Registry, id string) {
	wf, err := decodeWorkflow(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if id != "" && wf.ID != id {
		writeJSON(w, http.StatusBadRequest, response{Error: "workflow id does not match path"})
		return
	}
	result := validateWorkflow(reg, wf)
	if !result.Valid {
		writeJSON(w, http.StatusBadRequest, response{Data: result, Error: result.Error})
		return
	}
	path, source, err := saveWorkflowAsset(reg, wf)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	reg.Workflows[wf.ID] = &registry.Workflow{Entry: workflowEntryForSavedWorkflow(reg, path, wf), Config: wf, Path: path, Source: source}
	writeJSON(w, http.StatusOK, response{Status: "saved", Data: reg.Workflows[wf.ID]})
}

func handleWorkflowDelete(w http.ResponseWriter, reg *registry.Registry, path string) {
	id := strings.Trim(path, "/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	wf, err := reg.Workflow(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	if wf.Source.PluginID != userWorkflowPluginID || !isUserWorkflowPluginPath(reg, wf.Path) {
		writeJSON(w, http.StatusBadRequest, response{Error: "只能删除 Web 页面保存的用户工作流"})
		return
	}
	if err := os.Remove(wf.Path); err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	if err := os.RemoveAll(workflowConfigDir(reg, id)); err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	delete(reg.Workflows, id)
	if err := maintainUserWorkflowPluginManifest(reg); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "deleted", ID: id})
}

func handleWorkflowConfigFiles(w http.ResponseWriter, req *http.Request, state *serverState, workflowID string) {
	if req.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	reg := state.registry()
	wf, err := userWorkflow(reg, workflowID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	files, err := scanWorkflowConfigFiles(reg, wf.Config.ID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"files": files}})
}

func handleWorkflowConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, workflowID, fileID string) {
	switch req.Method {
	case http.MethodGet:
		handleGetWorkflowConfigFile(w, state, workflowID, fileID)
	case http.MethodPut:
		handleSaveWorkflowConfigFile(w, req, state, workflowID, fileID)
	case http.MethodDelete:
		handleDeleteWorkflowConfigFile(w, state, workflowID, fileID)
	default:
		methodNotAllowed(w)
	}
}

func handleGetWorkflowConfigFile(w http.ResponseWriter, state *serverState, workflowID, fileID string) {
	reg := state.registry()
	entry, _, err := declaredWorkflowConfigFile(reg, workflowID, fileID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	filePath, err := resolvedWorkflowConfigFilePath(reg, workflowID, entry)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	content, err := readConfigFileContent(filePath)
	if err != nil {
		if os.IsNotExist(err) && entry.Create {
			writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"content": ""}})
			return
		}
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"content": content}})
}

func handleSaveWorkflowConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, workflowID, fileID string) {
	defer req.Body.Close()
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "请求格式错误"})
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	entry, _, err := declaredWorkflowConfigFile(state.reg, workflowID, fileID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if entry.Access != config.ConfigFileAccessReadWrite {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件声明为只读，不能保存"})
		return
	}
	if int64(len([]byte(body.Content))) > maxPluginConfigFileBytes {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件内容超过大小限制"})
		return
	}
	filePath, err := resolvedWorkflowConfigFilePath(state.reg, workflowID, entry)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if err := writeConfigFileContent(filePath, body.Content, entry.Create); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"message": "工作流配置文件已保存"}})
}

func handleDeleteWorkflowConfigFile(w http.ResponseWriter, state *serverState, workflowID, fileID string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	entry, wf, err := declaredWorkflowConfigFile(state.reg, workflowID, fileID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	filePath, err := resolvedWorkflowConfigFilePath(state.reg, workflowID, entry)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if wf != nil && wf.Config != nil && len(wf.Config.ConfigFiles) > 0 {
		filtered := wf.Config.ConfigFiles[:0]
		for _, item := range wf.Config.ConfigFiles {
			normalized := normalizeWorkflowConfigFileRef(wf.Config.ID, item)
			if normalized.ID == fileID {
				continue
			}
			filtered = append(filtered, item)
		}
		wf.Config.ConfigFiles = filtered
		if err := saveWorkflowAssetForWorkflow(state.reg, wf.Config); err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("更新工作流配置声明失败: %v", err)})
			return
		}
		state.reg.Workflows[wf.Config.ID] = wf
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("删除工作流配置文件失败: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"message": "工作流配置文件已删除"}})
}

func runsHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reg := state.registry()
		id := strings.TrimPrefix(req.URL.Path, "/api/runs/")
		if req.Method == http.MethodPost && strings.Trim(id, "/") == "cleanup" {
			handleRunsCleanup(w, req, reg)
			return
		}
		if req.Method == http.MethodPost {
			if runID, ok := strings.CutSuffix(strings.Trim(id, "/"), "/cancel"); ok {
				handleRunCancel(w, state, reg, strings.Trim(runID, "/"))
				return
			}
			if runID, nodeID, ok := parseRunNodeRerunPath(id); ok {
				handleRunNodeRerun(w, req, state, reg, strings.Trim(runID, "/"), strings.Trim(nodeID, "/"))
				return
			}
			if runID, nodeID, action, ok := parseRunUploadPath(id); ok {
				handleRunUploadNode(w, req, state, reg, strings.Trim(runID, "/"), strings.Trim(nodeID, "/"), action)
				return
			}
			methodNotAllowed(w)
			return
		}
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if strings.Trim(id, "/") == "" {
			items, err := listRuns(state, reg)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"runs": items}})
			return
		}
		if runID, ok := strings.CutSuffix(id, "/support.zip"); ok {
			handleRunSupportZip(w, reg, strings.Trim(runID, "/"))
			return
		}
		if runID, ok := strings.CutSuffix(id, "/events"); ok {
			handleRunEvents(w, req, state, reg, strings.Trim(runID, "/"))
			return
		}
		detail, err := loadRunDetail(state, reg, id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Data: detail})
	}
}

func parseRunNodeRerunPath(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 4 || parts[1] != "nodes" || parts[3] != "rerun" {
		return "", "", false
	}
	return parts[0], parts[2], true
}

func parseRunUploadPath(path string) (string, string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if (len(parts) != 3 && len(parts) != 4) || len(parts) >= 2 && parts[1] != "uploads" {
		return "", "", "", false
	}
	action := ""
	if len(parts) == 4 {
		action = parts[3]
	}
	return parts[0], parts[2], action, true
}

func handleRunCancel(w http.ResponseWriter, state *serverState, reg *registry.Registry, id string) {
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	detail, err := loadRunDetail(state, reg, id)
	if err == nil && detail.Record.Status != "running" {
		writeJSON(w, http.StatusBadRequest, response{Status: detail.Record.Status, Data: detail, Error: "只能取消运行中的任务"})
		return
	}
	if state.hasActiveRun(id) {
		if !state.cancelRun(id) {
			writeJSON(w, http.StatusConflict, response{Error: "当前进程没有该运行任务的取消句柄，可能已结束或由其他进程启动"})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "cancelling", Data: map[string]string{"id": id, "status": "cancelling"}})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	if !state.cancelRun(id) {
		writeJSON(w, http.StatusConflict, response{Error: "当前进程没有该运行任务的取消句柄，可能已结束或由其他进程启动"})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "cancelling", Data: map[string]string{"id": id, "status": "cancelling"}})
}

func handleRunNodeRerun(w http.ResponseWriter, req *http.Request, state *serverState, reg *registry.Registry, runID, nodeID string) {
	if req.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(nodeID) == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	if state.hasActiveRun(runID) {
		writeJSON(w, http.StatusConflict, response{Error: "运行任务正在执行，不能重跑节点"})
		return
	}
	reqBody, err := decodeRunRequest(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	detail, err := loadRunDetail(state, reg, runID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	if detail.Record.Status == "running" {
		writeJSON(w, http.StatusBadRequest, response{Error: "运行中的工作流不能重跑节点"})
		return
	}
	workflowConfig := reqBody.Workflow
	if workflowConfig == nil {
		wf, err := reg.Workflow(detail.Record.Target)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		workflowConfig = wf.Config
	}
	if workflowConfig.ID != detail.Record.Target {
		writeJSON(w, http.StatusBadRequest, response{Error: "请求工作流与运行记录不一致"})
		return
	}
	if workflowConfig.Confirm.Required && !reqBody.Confirm {
		writeJSON(w, http.StatusBadRequest, response{Error: "该工作流需要确认后执行"})
		return
	}
	if err := confirmWorkflowTools(reg, workflowConfig, reqBody.Confirm); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	rerunParams := config.ValuesToStringMap(config.MergeParamsValues(workflowConfig.Parameters, config.InterfaceMapToValues(detail.Record.Config), config.InterfaceMapToValues(reqBody.Params)))
	if len(detail.Record.Config) == 0 {
		rerunParams = config.ValuesToStringMap(config.MergeParamsValues(workflowConfig.Parameters, config.StringMapToValues(detail.Record.Params), config.InterfaceMapToValues(reqBody.Params)))
	}
	if err := config.ValidateRequired(workflowConfig.Parameters, rerunParams); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	runDir, err := runbundle.RunDir(reg, runID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	r := runner.New(reg)
	record := detail.Record
	ctx, cancel := context.WithCancel(context.Background())
	state.registerRun(runID, cancel)
	defer func() {
		state.finishRun(runID)
		cancel()
	}()
	updated, err := r.RerunWorkflowNodeWithParams(ctx, workflowConfig, &record, runDir, nodeID, rerunParams, reqBody.Confirm, io.Discard, io.Discard)
	if err != nil {
		writeRunResponse(w, updated, err)
		return
	}
	rerunDetail, err := loadRunDetail(state, reg, runID)
	if err != nil {
		writeJSON(w, http.StatusOK, response{ID: updated.ID, Status: updated.Status, Data: runDetail{Record: *updated}})
		return
	}
	writeJSON(w, http.StatusOK, response{ID: updated.ID, Status: updated.Status, Data: rerunDetail})
}

func handleRunUploadNode(w http.ResponseWriter, req *http.Request, state *serverState, reg *registry.Registry, runID, nodeID, action string) {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(nodeID) == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	if !state.hasActiveRun(runID) {
		writeJSON(w, http.StatusConflict, response{Error: "运行任务未处于活动状态，无法上传节点文件"})
		return
	}
	detail, err := loadRunDetail(state, reg, runID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	if detail.Record.Status != "running" {
		writeJSON(w, http.StatusBadRequest, response{Error: "只能向运行中的工作流上传节点文件"})
		return
	}
	step, ok := findRunStep(detail.Record, nodeID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, response{Error: "上传节点尚未进入等待状态"})
		return
	}
	if step.Type != config.WorkflowNodeTypeUpload {
		writeJSON(w, http.StatusBadRequest, response{Error: "目标步骤不是上传节点"})
		return
	}
	if step.Status != "waiting" {
		writeJSON(w, http.StatusBadRequest, response{Error: "上传节点当前不在等待上传状态"})
		return
	}
	if action != "" {
		handleRunUploadNodeChunked(w, req, reg, runID, nodeID, action)
		return
	}
	req.Body = http.MaxBytesReader(w, req.Body, maxPlatformFileUploadBytes+1)
	result, err := savePlatformUpload(reg, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	runDir, err := runbundle.RunDir(reg, runID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	if err := runner.WriteWorkflowUploadResult(runDir, nodeID, result); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("写入上传节点结果失败: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "uploaded", Data: result})
}

func handleRunUploadNodeChunked(w http.ResponseWriter, req *http.Request, reg *registry.Registry, runID, nodeID, action string) {
	switch action {
	case "start":
		var body chunkedUploadStartRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		session, err := startChunkedPlatformUpload(reg, body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "started", Data: map[string]interface{}{"id": session.ID, "chunk_size": maxPlatformUploadChunkBytes}})
	case "chunk":
		sessionID := strings.TrimSpace(req.URL.Query().Get("session_id"))
		fileIndex, err := strconv.Atoi(req.URL.Query().Get("file_index"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "上传文件索引无效"})
			return
		}
		offset, err := strconv.ParseInt(req.URL.Query().Get("offset"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "上传分片偏移无效"})
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, maxPlatformUploadChunkBytes+1)
		session, err := appendChunkedPlatformUpload(reg, sessionID, fileIndex, offset, req.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "chunked", Data: map[string]interface{}{"id": session.ID, "received_size": session.ReceivedSize, "received": session.Received}})
	case "finish":
		var body chunkedUploadFinishRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		result, err := finishChunkedPlatformUpload(reg, body.SessionID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		runDir, err := runbundle.RunDir(reg, runID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		if err := runner.WriteWorkflowUploadResult(runDir, nodeID, result); err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("写入上传节点结果失败: %v", err)})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "uploaded", Data: result})
	default:
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
	}
}

func findRunStep(record runner.RunRecord, nodeID string) (runner.StepRecord, bool) {
	for _, step := range record.Steps {
		if step.ID == nodeID {
			return step, true
		}
	}
	return runner.StepRecord{}, false
}

func handleRunsCleanup(w http.ResponseWriter, req *http.Request, reg *registry.Registry) {
	var body runbundle.CleanupOptions
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	result, err := runbundle.Cleanup(reg, body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "ok", Data: result})
}

func handleRunEvents(w http.ResponseWriter, req *http.Request, state *serverState, reg *registry.Registry, id string) {
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	detail, err := loadRunDetail(state, reg, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	runDir, err := runbundle.RunDir(reg, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, response{Error: "streaming is not supported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	tailer := newRunLogTailer(id, runDir, detail.Record)
	seq := 0
	lastBySource := map[string]string{}
	sendLogEvents := func() bool {
		events := tailer.read(detail.Record.Status)
		for _, event := range events {
			key := event.ItemID + ":" + event.Stream
			if lastBySource[key] == event.Text {
				continue
			}
			lastBySource[key] = event.Text
			seq++
			event.Seq = seq
			if err := writeSSEEvent(w, "log", event); err != nil {
				return false
			}
		}
		flusher.Flush()
		return true
	}
	if !sendLogEvents() {
		return
	}
	if detail.Record.Status != "running" {
		_ = writeSSEEvent(w, "complete", map[string]string{"run_id": id, "status": detail.Record.Status})
		flusher.Flush()
		return
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-req.Context().Done():
			return
		case <-ticker.C:
			next, err := loadRunDetail(nil, reg, id)
			if err == nil {
				detail = next
				tailer.record = next.Record
			}
			if !sendLogEvents() {
				return
			}
			if err != nil || detail.Record.Status != "running" {
				status := detail.Record.Status
				if status == "" {
					status = "unknown"
				}
				_ = writeSSEEvent(w, "complete", map[string]string{"run_id": id, "status": status})
				flusher.Flush()
				return
			}
		}
	}
}

func writeSSEEvent(w io.Writer, event string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

func handleRunSupportZip(w http.ResponseWriter, reg *registry.Registry, id string) {
	data, err := runbundle.ExportZip(reg, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, runbundle.Filename(id)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func toolDevKitHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		data, err := buildToolDevKitZip()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="ops-plugin-template.zip"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func userWorkflowPluginExportHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		data, err := buildUserWorkflowPluginZip(state.registry())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="user.workflows.zip"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func pluginDownloadHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		name := strings.TrimPrefix(req.URL.Path, "/api/plugins/")

		// 配置文件路由：/api/plugins/{id}/files
		if strings.HasSuffix(name, "/files") || strings.Contains(name, "/files/") {
			parts := strings.Split(strings.Trim(name, "/"), "/")
			if len(parts) >= 2 && parts[1] == "files" {
				pluginID := parts[0]
				if len(parts) == 2 {
					// /api/plugins/{id}/files - 列出配置文件
					handlePluginConfigFiles(w, req, state, pluginID)
				} else {
					// /api/plugins/{id}/files/{fileID} - 读取/保存/删除配置文件；legacy 插件内路径允许编码后的斜杠，但必须命中声明。
					fileID, err := url.PathUnescape(strings.Join(parts[2:], "/"))
					if err != nil {
						writeJSON(w, http.StatusBadRequest, response{Error: "配置文件 ID 格式错误"})
						return
					}
					handlePluginConfigFile(w, req, state, pluginID, fileID)
				}
				return
			}
		}

		if pluginID, ok := strings.CutSuffix(name, "/disable"); ok {
			handlePluginDisable(w, req, state, pluginID)
			return
		}
		if pluginID, ok := strings.CutSuffix(name, "/enable"); ok {
			handlePluginEnable(w, req, state, pluginID)
			return
		}
		// 版本管理路由已移除，业务配置已废弃
		if req.Method == http.MethodDelete {
			handlePluginDelete(w, req, state, strings.Trim(name, "/"))
			return
		}
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if strings.HasSuffix(name, "/runtime.zip") || strings.HasSuffix(name, "/runtime.tar.gz") {
			handlePluginRuntimeDownload(w, req, state, name)
			return
		}
		pluginID, ok := strings.CutSuffix(name, ".zip")
		if !ok || strings.TrimSpace(pluginID) == "" {
			writeJSON(w, http.StatusNotFound, response{Error: "not found"})
			return
		}
		data, err := buildPluginExportZip(state.registry(), pluginID)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errPluginNotFound) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, response{Error: err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, pluginID))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func handlePluginRuntimeDownload(w http.ResponseWriter, req *http.Request, state *serverState, name string) {
	format := "zip"
	pluginID, ok := strings.CutSuffix(name, "/runtime.zip")
	if !ok {
		format = "tar.gz"
		pluginID, ok = strings.CutSuffix(name, "/runtime.tar.gz")
	}
	if !ok || strings.TrimSpace(pluginID) == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	goos := strings.TrimSpace(req.URL.Query().Get("goos"))
	goarch := strings.TrimSpace(req.URL.Query().Get("goarch"))
	data, err := buildPluginRuntimePackage(state.registry(), strings.Trim(pluginID, "/"), goos, goarch, format)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errPluginNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, response{Error: err.Error()})
		return
	}
	contentType := "application/zip"
	fileName := fmt.Sprintf("%s-opsctl-%s-%s.zip", pluginID, goos, goarch)
	if format == "tar.gz" {
		contentType = "application/gzip"
		fileName = fmt.Sprintf("%s-opsctl-%s-%s.tar.gz", pluginID, goos, goarch)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func listRuns(state *serverState, reg *registry.Registry) ([]runbundle.Summary, error) {
	items, err := runbundle.List(reg)
	if err != nil {
		return nil, err
	}
	out := make([]runbundle.Summary, 0, len(items))
	for _, item := range items {
		if item.Status != "running" || state == nil || state.hasActiveRun(item.ID) {
			out = append(out, item)
			continue
		}
		record, err := state.reconcileRunRecord(reg, item.ID)
		if err != nil {
			out = append(out, item)
			continue
		}
		out = append(out, runbundle.Summary{
			ID:        record.ID,
			Kind:      record.Kind,
			Target:    record.Target,
			Status:    record.Status,
			StartedAt: record.StartedAt,
			EndedAt:   record.EndedAt,
			Params:    record.Params,
			Error:     record.Error,
		})
	}
	return out, nil
}

func loadRunDetail(state *serverState, reg *registry.Registry, id string) (runDetail, error) {
	var record runner.RunRecord
	var err error
	if state != nil {
		record, err = state.reconcileRunRecord(reg, id)
	} else {
		record, err = runbundle.LoadRecord(reg, id)
	}
	if err != nil {
		return runDetail{}, err
	}
	runDir, err := runbundle.RunDir(reg, id)
	if err != nil {
		return runDetail{}, err
	}
	return runDetail{Record: record, Logs: loadRunLogs(runDir, record)}, nil
}

func loadRunLogs(runDir string, record runner.RunRecord) runLogs {
	logs := runLogs{
		Stdout: readTextFile(filepath.Join(runDir, "stdout.log")),
		Stderr: readTextFile(filepath.Join(runDir, "stderr.log")),
	}
	if len(record.Steps) == 0 {
		if logs.Stdout != "" || logs.Stderr != "" || record.Kind == "tool" {
			logs.Items = []runLogItem{{
				ID:     record.ID,
				Kind:   "tool_run",
				Title:  nonEmpty(record.Target, record.ID),
				Status: record.Status,
				Type:   "tool",
				Tool:   record.Target,
				Stdout: logs.Stdout,
				Stderr: logs.Stderr,
			}}
		}
		return logs
	}
	logs.Steps = map[string]runLogs{}
	for _, step := range record.Steps {
		logs.Steps[step.ID] = runLogs{
			Stdout: readTextFile(filepath.Join(runDir, step.ID, "stdout.log")),
			Stderr: readTextFile(filepath.Join(runDir, step.ID, "stderr.log")),
		}
	}
	logs.Items = buildRunLogItems(runDir, record, logs.Steps)
	return logs
}

func readTextFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func buildRunLogItems(runDir string, record runner.RunRecord, steps map[string]runLogs) []runLogItem {
	items := make([]runLogItem, 0, len(record.Steps))
	for _, step := range record.Steps {
		stepLogs := steps[step.ID]
		item := runLogItem{
			ID:     step.ID,
			Kind:   "workflow_step",
			Title:  stepLogTitle(step),
			Status: step.Status,
			Type:   step.Type,
			Tool:   step.Tool,
			Stdout: stepLogs.Stdout,
			Stderr: stepLogs.Stderr,
		}
		if step.Type == config.WorkflowNodeTypeLoop {
			item.Children = loopLogItems(runDir, step.ID, step.LoopIterations, step.Status)
		}
		items = append(items, item)
	}
	return items
}

func stepLogTitle(step runner.StepRecord) string {
	if step.Type == config.WorkflowNodeTypeLoop && step.Tool != "" {
		return fmt.Sprintf("%s / 循环 / %s", step.ID, step.Tool)
	}
	if step.Tool != "" {
		return fmt.Sprintf("%s / %s", step.ID, step.Tool)
	}
	if step.Type != "" {
		return fmt.Sprintf("%s / %s", step.ID, step.Type)
	}
	return step.ID
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func loopLogItems(runDir, stepID string, iterations int, status string) []runLogItem {
	if iterations <= 0 {
		iterations = countLoopIterationDirs(runDir, stepID)
	}
	items := []runLogItem{}
	for iteration := 1; iteration <= iterations; iteration++ {
		dir := filepath.Join(runDir, stepID, fmt.Sprintf("%d", iteration))
		stdout := readTextFile(filepath.Join(dir, "stdout.log"))
		stderr := readTextFile(filepath.Join(dir, "stderr.log"))
		if stdout == "" && stderr == "" {
			continue
		}
		itemID := fmt.Sprintf("%s#%d", stepID, iteration)
		items = append(items, runLogItem{
			ID:        itemID,
			Kind:      "loop_iteration",
			Title:     fmt.Sprintf("%s / 第 %d 次", stepID, iteration),
			Status:    status,
			Type:      config.WorkflowNodeTypeLoop,
			Stdout:    stdout,
			Stderr:    stderr,
			Iteration: iteration,
		})
	}
	return items
}

func countLoopIterationDirs(runDir, stepID string) int {
	entries, err := os.ReadDir(filepath.Join(runDir, stepID))
	if err != nil {
		return 0
	}
	maxIteration := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var iteration int
		if _, err := fmt.Sscanf(entry.Name(), "%d", &iteration); err == nil && iteration > maxIteration {
			maxIteration = iteration
		}
	}
	return maxIteration
}

type runLogTailer struct {
	runID  string
	runDir string
	record runner.RunRecord
	offset map[string]int64
}

type runLogSource struct {
	path      string
	itemID    string
	kind      string
	stepID    string
	iteration int
	stream    string
}

func newRunLogTailer(runID, runDir string, record runner.RunRecord) *runLogTailer {
	return &runLogTailer{runID: runID, runDir: runDir, record: record, offset: map[string]int64{}}
}

func (t *runLogTailer) read(status string) []runLogEvent {
	events := []runLogEvent{}
	for _, source := range t.sources() {
		lines, nextOffset := readNewLogLines(source.path, t.offset[source.path])
		t.offset[source.path] = nextOffset
		for _, line := range lines {
			events = append(events, runLogEvent{
				RunID:     t.runID,
				ItemID:    source.itemID,
				Kind:      source.kind,
				StepID:    source.stepID,
				Iteration: source.iteration,
				Stream:    source.stream,
				Text:      line,
				Status:    status,
			})
		}
	}
	return events
}

func (t *runLogTailer) sources() []runLogSource {
	if len(t.record.Steps) == 0 {
		return []runLogSource{
			{path: filepath.Join(t.runDir, "stdout.log"), itemID: t.record.ID, kind: "tool_run", stream: "stdout"},
			{path: filepath.Join(t.runDir, "stderr.log"), itemID: t.record.ID, kind: "tool_run", stream: "stderr"},
		}
	}
	sources := []runLogSource{}
	for _, step := range t.record.Steps {
		stepDir := filepath.Join(t.runDir, step.ID)
		sources = append(sources,
			runLogSource{path: filepath.Join(stepDir, "stdout.log"), itemID: step.ID, kind: "workflow_step", stepID: step.ID, stream: "stdout"},
			runLogSource{path: filepath.Join(stepDir, "stderr.log"), itemID: step.ID, kind: "workflow_step", stepID: step.ID, stream: "stderr"},
		)
		if step.Type == config.WorkflowNodeTypeLoop {
			iterations := step.LoopIterations
			if iterations <= 0 {
				iterations = countLoopIterationDirs(t.runDir, step.ID)
			}
			for iteration := 1; iteration <= iterations; iteration++ {
				itemID := fmt.Sprintf("%s#%d", step.ID, iteration)
				iterationDir := filepath.Join(stepDir, fmt.Sprintf("%d", iteration))
				sources = append(sources,
					runLogSource{path: filepath.Join(iterationDir, "stdout.log"), itemID: itemID, kind: "loop_iteration", stepID: step.ID, iteration: iteration, stream: "stdout"},
					runLogSource{path: filepath.Join(iterationDir, "stderr.log"), itemID: itemID, kind: "loop_iteration", stepID: step.ID, iteration: iteration, stream: "stderr"},
				)
			}
		}
	}
	return sources
}

func readNewLogLines(path string, offset int64) ([]string, int64) {
	file, err := os.Open(path)
	if err != nil {
		return nil, offset
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, offset
	}
	if info.Size() < offset {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, offset
	}
	nextOffset := offset + int64(len(data))
	if len(data) == 0 {
		return nil, nextOffset
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nextOffset
}

func buildToolDevKitZip() ([]byte, error) {
	files := map[string]string{
		"README.md":                              toolDevKitReadme,
		"SPEC.md":                                toolDevKitSpec,
		"plugins/plugin.template/plugin.yaml":    samplePluginYAML,
		"plugins/plugin.template/scripts/run.sh": sampleRunScript,
		"plugins/plugin.template/workflows/maintenance-flow.yaml": samplePluginWorkflowYAML,
		"plugins/plugin.template/README.md":                       samplePluginReadme,
		"plugins/plugin.template/config/example.conf":             sampleConfigFile,
		"plugins/plugin.template/examples/params.yaml":            sampleParamsYAML,
		"plugins/plugin.template/examples/README.md":              sampleExamplesReadme,
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		file, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := file.Write([]byte(content)); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func registerWeb(mux *http.ServeMux) {
	assets, err := fs.Sub(webFiles, "web")
	if err != nil {
		return
	}
	fileServer := http.FileServer(http.FS(assets))
	mux.Handle("/assets/", fileServer)
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		data, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	})
}

func buildCatalog(reg *registry.Registry) catalogResponse {
	out := catalogResponse{Name: reg.Root.DisplayName(), Description: reg.Root.DisplayDescription(), Categories: buildCatalogCategories(reg), Warnings: reg.Warnings}
	for _, item := range catalogPluginEntries(reg) {
		out.Plugins = append(out.Plugins, item)
	}
	for _, tool := range reg.OrderedTools() {
		out.Tools = append(out.Tools, toolCatalogEntry{ToolEntry: tool.Entry, Tags: tool.Config.Tags, Execution: tool.Config.Execution, Parameters: tool.Config.Parameters, Outputs: tool.Config.Outputs, ConfigFiles: tool.Config.ConfigFiles, ConfigFileRefs: declaredConfigFiles(tool.Config), Confirm: tool.Config.Confirm, Source: tool.Source})
	}
	for _, wf := range reg.Workflows {
		out.Workflows = append(out.Workflows, workflowCatalogEntry{WorkflowRef: wf.Entry, Tags: wf.Config.Tags, Parameters: wf.Config.Parameters, ConfigFiles: workflowConfigFilesForCatalog(reg, wf.Config), Confirm: effectiveWorkflowConfirm(reg, wf.Config), Source: wf.Source})
	}
	return out
}

func buildCatalogCategories(reg *registry.Registry) []categoryCatalogEntry {
	active := activeCategoryIDs(reg)
	out := make([]categoryCatalogEntry, 0, len(reg.Root.DisplayCategories()))
	seen := map[string]bool{}
	for _, category := range reg.Root.DisplayCategories() {
		entry := categoryCatalogEntry{Category: category}
		if !active[category.ID] {
			if source, ok := disabledCategorySource(reg, category.ID); ok {
				entry.Disabled = true
				entry.Source = &source
			}
		}
		out = append(out, entry)
		if category.ID != "" {
			seen[category.ID] = true
		}
	}
	for _, pkg := range installedAnyPluginPackages(reg) {
		if !pluginDisabled(reg.Root.Plugins.Disabled, pkg) {
			continue
		}
		source := registry.Source{Type: "plugin", PluginID: pkg.Manifest.ID, PluginName: pkg.Manifest.Name, PluginVersion: pkg.Manifest.Version}
		for _, category := range pkg.Manifest.Contributes.Categories {
			if category.ID == "" || seen[category.ID] || active[category.ID] {
				continue
			}
			out = append(out, categoryCatalogEntry{Category: category, Disabled: true, Source: &source})
			seen[category.ID] = true
		}
	}
	return out
}

func disabledCategorySource(reg *registry.Registry, categoryID string) (registry.Source, bool) {
	for _, pkg := range installedAnyPluginPackages(reg) {
		if !pluginDisabled(reg.Root.Plugins.Disabled, pkg) {
			continue
		}
		for _, category := range pkg.Manifest.Contributes.Categories {
			if category.ID == categoryID {
				return registry.Source{Type: "plugin", PluginID: pkg.Manifest.ID, PluginName: pkg.Manifest.Name, PluginVersion: pkg.Manifest.Version}, true
			}
		}
	}
	return registry.Source{}, false
}

func effectiveWorkflowConfirm(reg *registry.Registry, wf *config.WorkflowConfig) config.Confirmation {
	if wf.Confirm.Required {
		return wf.Confirm
	}
	for _, node := range wf.Nodes {
		nodeType := node.Type
		if nodeType == "" && node.Tool != "" {
			nodeType = config.WorkflowNodeTypeTool
		}
		if nodeType != config.WorkflowNodeTypeTool {
			continue
		}
		tool, err := reg.Tool(node.Tool)
		if err != nil || !tool.Config.Confirm.Required || node.Confirm {
			continue
		}
		message := tool.Config.Confirm.Message
		if message == "" {
			message = "工作流包含需要确认的工具"
		}
		return config.Confirmation{Required: true, Message: message}
	}
	return wf.Confirm
}

func decodeWorkflow(req *http.Request) (*config.WorkflowConfig, error) {
	defer req.Body.Close()
	var body workflowSaveRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, err
	}
	config.NormalizeWorkflow(&body.Workflow)
	return &body.Workflow, nil
}

func validateWorkflow(reg *registry.Registry, wf *config.WorkflowConfig) workflowValidation {
	if err := reg.ValidateWorkflow(wf); err != nil {
		return workflowValidation{Valid: false, Error: err.Error()}
	}
	return workflowValidation{Valid: true}
}

func saveWorkflow(path string, wf *config.WorkflowConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(wf); err != nil {
		_ = enc.Close()
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func saveWorkflowAsset(reg *registry.Registry, wf *config.WorkflowConfig) (string, registry.Source, error) {
	path := workflowPath(reg, wf.ID)
	if err := saveWorkflow(path, wf); err != nil {
		return "", registry.Source{}, err
	}
	if isUserWorkflowPluginPath(reg, path) || reg.Workflows[wf.ID] == nil {
		if err := maintainUserWorkflowPluginManifest(reg); err != nil {
			return "", registry.Source{}, err
		}
	}
	return path, workflowSource(reg, path), nil
}

func workflowPath(reg *registry.Registry, id string) string {
	if wf, ok := reg.Workflows[id]; ok && wf.Path != "" {
		return wf.Path
	}
	filename := workflowFilename(id)
	return filepath.Join(userWorkflowPluginDir(reg), "workflows", filename)
}

func workflowFilename(id string) string {
	clean := strings.TrimSpace(id)
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	clean = replacer.Replace(clean)
	clean = strings.Trim(clean, ". ")
	if clean == "" {
		clean = "workflow"
	}
	return clean + ".yaml"
}

func userWorkflowPluginDir(reg *registry.Registry) string {
	return filepath.Join(reg.BaseDir, filepath.FromSlash(firstPluginRoot(reg)), userWorkflowPluginID)
}

func isUserWorkflowPluginPath(reg *registry.Registry, path string) bool {
	pluginDir, err := filepath.Abs(userWorkflowPluginDir(reg))
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return absPath == pluginDir || strings.HasPrefix(absPath, pluginDir+string(os.PathSeparator))
}

func workflowEntryForSavedWorkflow(reg *registry.Registry, path string, wf *config.WorkflowConfig) config.WorkflowRef {
	return config.WorkflowRef{ID: wf.ID, Category: wf.Category, Path: relativePath(reg.BaseDir, path), Name: wf.Name, Description: wf.Description, Tags: wf.Tags}
}

func workflowSource(reg *registry.Registry, path string) registry.Source {
	for _, existing := range reg.Workflows {
		if existing.Path == path && existing.Source.Type != "" {
			return existing.Source
		}
	}
	if isUserWorkflowPluginPath(reg, path) {
		return registry.Source{Type: "plugin", PluginID: userWorkflowPluginID, PluginName: userWorkflowPluginName, PluginVersion: userWorkflowPluginVersion}
	}
	return registry.Source{Type: "builtin"}
}

func maintainUserWorkflowPluginManifest(reg *registry.Registry) error {
	pluginDir := userWorkflowPluginDir(reg)
	workflowsDir := filepath.Join(pluginDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		return err
	}
	manifest, err := loadOrDefaultUserWorkflowManifest(filepath.Join(pluginDir, "plugin.yaml"))
	if err != nil {
		return err
	}
	manifest.ID = userWorkflowPluginID
	manifest.Name = userWorkflowPluginName
	if strings.TrimSpace(manifest.Version) == "" {
		manifest.Version = userWorkflowPluginVersion
	}
	if strings.TrimSpace(manifest.Description) == "" {
		manifest.Description = "Web 页面创建和维护的用户工作流集合"
	}
	manifest.Contributes.Tools = nil
	paths := map[string]bool{}
	if err := filepath.WalkDir(workflowsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		rel, err := filepath.Rel(pluginDir, path)
		if err != nil {
			return err
		}
		paths[filepath.ToSlash(rel)] = true
		return nil
	}); err != nil {
		return err
	}
	workflowPaths := make([]string, 0, len(paths))
	for path := range paths {
		workflowPaths = append(workflowPaths, path)
	}
	sort.Strings(workflowPaths)
	manifest.Contributes.Workflows = manifest.Contributes.Workflows[:0]
	for _, path := range workflowPaths {
		manifest.Contributes.Workflows = append(manifest.Contributes.Workflows, plugin.Workflow{Path: path})
	}
	return savePluginManifest(filepath.Join(pluginDir, "plugin.yaml"), manifest)
}

func loadOrDefaultUserWorkflowManifest(path string) (plugin.Manifest, error) {
	manifest := plugin.Manifest{
		ID:          userWorkflowPluginID,
		Name:        userWorkflowPluginName,
		Version:     userWorkflowPluginVersion,
		Description: "Web 页面创建和维护的用户工作流集合",
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return plugin.Manifest{}, fmt.Errorf("解析用户工作流插件清单失败: %w", err)
		}
		return manifest, nil
	}
	if os.IsNotExist(err) {
		return manifest, nil
	}
	return plugin.Manifest{}, err
}

func savePluginManifest(path string, manifest plugin.Manifest) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(manifest); err != nil {
		_ = enc.Close()
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func buildUserWorkflowPluginZip(reg *registry.Registry) ([]byte, error) {
	pluginDir := userWorkflowPluginDir(reg)
	if err := maintainUserWorkflowPluginManifest(reg); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "plugin.yaml")); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := filepath.WalkDir(pluginDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("不支持导出特殊文件: %s", path)
		}
		rel, err := filepath.Rel(pluginDir, path)
		if err != nil {
			return err
		}
		name := userWorkflowPluginID + "/" + filepath.ToSlash(rel)
		entry, err := zw.Create(name)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = entry.Write(data)
		return err
	}); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func relativePath(baseDir, path string) string {
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func writeRunResponse(w http.ResponseWriter, record *runner.RunRecord, err error) {
	if record == nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: errorText(err)})
		return
	}
	status := http.StatusOK
	if err != nil {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, response{ID: record.ID, Status: record.Status, Error: errorText(err)})
}

func decodeRunRequest(req *http.Request) (*runRequest, error) {
	defer req.Body.Close()
	var body runRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Params == nil {
		body.Params = map[string]interface{}{}
	}
	return &body, nil
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, response{Error: "method not allowed"})
}

func writeJSON(w http.ResponseWriter, status int, body response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

const maxPluginConfigFileBytes int64 = 1024 * 1024

type pluginConfigFileStatus struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	ConfigDir   string `json:"config_dir,omitempty"`
	DisplayRoot string `json:"display_root,omitempty"`
	Path        string `json:"path"`
	DisplayPath string `json:"display_path,omitempty"`
	Scope       string `json:"scope"`
	Access      string `json:"access"`
	Create      bool   `json:"create"`
	Exists      bool   `json:"exists"`
	Readable    bool   `json:"readable"`
	Writable    bool   `json:"writable"`
	Reason      string `json:"reason,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

func handlePluginConfigFiles(w http.ResponseWriter, req *http.Request, state *serverState, pluginID string) {
	if req.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	reg := state.registry()
	seen := map[string]bool{}
	var files []pluginConfigFileStatus
	for _, tool := range reg.Tools {
		if tool.Source.PluginID != pluginID {
			continue
		}
		for _, declared := range declaredConfigFiles(tool.Config) {
			expanded, err := expandDeclaredConfigFiles(reg.Root, tool.Config, declared)
			if err != nil {
				entry := normalizeServerConfigFileRef(declared)
				id := entry.ID
				if id == "" {
					id = entry.Path
				}
				if id == "" || seen[id] {
					continue
				}
				seen[id] = true
				files = append(files, pluginConfigFileStatus{ID: id, Label: entry.Label, ConfigDir: entry.ConfigDir, Path: entry.Path, Scope: entry.Scope, Access: entry.Access, Create: entry.Create, Reason: err.Error()})
				continue
			}
			for _, entry := range expanded {
				if entry.ID == "" || seen[entry.ID] {
					continue
				}
				seen[entry.ID] = true
				filePath, err := resolvedConfigFilePath(reg.Root, tool.Config, entry)
				if err != nil {
					files = append(files, pluginConfigFileStatus{ID: entry.ID, Label: entry.Label, ConfigDir: entry.ConfigDir, Path: entry.Path, Scope: entry.Scope, Access: entry.Access, Create: entry.Create, Reason: err.Error()})
					continue
				}
				files = append(files, configFileStatus(tool.Config, entry, filePath))
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ID < files[j].ID })

	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"files": files}})
}

func handlePluginConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, pluginID, fileID string) {
	switch req.Method {
	case http.MethodGet:
		handleGetPluginConfigFile(w, req, state, pluginID, fileID)
	case http.MethodPut:
		handleSavePluginConfigFile(w, req, state, pluginID, fileID)
	case http.MethodDelete:
		handleDeletePluginConfigFile(w, req, state, pluginID, fileID)
	default:
		methodNotAllowed(w)
	}
}

func handleGetPluginConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, pluginID, fileID string) {
	entry, toolCfg, err := declaredPluginConfigFile(state.registry(), pluginID, fileID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	filePath, err := resolvedConfigFilePath(state.registry().Root, toolCfg, entry)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) && entry.Scope == config.ConfigFileScopePlugin {
			writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"content": ""}})
			return
		}
		writeJSON(w, http.StatusBadRequest, response{Error: fmt.Sprintf("配置文件不可读: %v", err)})
		return
	}
	if !info.Mode().IsRegular() {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件不是普通文件"})
		return
	}
	if info.Size() > maxPluginConfigFileBytes {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件超过大小限制"})
		return
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("读取配置文件失败: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"content": string(data)}})
}

func handleSavePluginConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, pluginID, fileID string) {
	defer req.Body.Close()
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "请求格式错误"})
		return
	}

	entry, toolCfg, err := declaredPluginConfigFile(state.registry(), pluginID, fileID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if entry.Access != config.ConfigFileAccessReadWrite {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件声明为只读，不能保存"})
		return
	}
	if int64(len([]byte(body.Content))) > maxPluginConfigFileBytes {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件内容超过大小限制"})
		return
	}
	filePath, err := resolvedConfigFilePath(state.registry().Root, toolCfg, entry)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	info, err := os.Stat(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("检查配置文件失败: %v", err)})
			return
		}
		if entry.Scope == config.ConfigFileScopeHostAbsolute {
			if !entry.Create {
				writeJSON(w, http.StatusBadRequest, response{Error: "配置文件不存在且声明不允许创建"})
				return
			}
			if !parentWritable(filepath.Dir(filePath)) {
				writeJSON(w, http.StatusBadRequest, response{Error: "配置文件父目录不可写或不存在"})
				return
			}
		} else if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("创建配置目录失败: %v", err)})
			return
		}
	} else {
		if !info.Mode().IsRegular() {
			writeJSON(w, http.StatusBadRequest, response{Error: "配置文件不是普通文件"})
			return
		}
		file, err := os.OpenFile(filePath, os.O_WRONLY, 0)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: fmt.Sprintf("配置文件不可写: %v", err)})
			return
		}
		_ = file.Close()
	}
	if err := os.WriteFile(filePath, []byte(body.Content), 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("保存配置文件失败: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"message": "配置文件已保存"}})
}

func handleDeletePluginConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, pluginID, fileID string) {
	entry, toolCfg, err := declaredPluginConfigFile(state.registry(), pluginID, fileID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if entry.Scope == config.ConfigFileScopeHostAbsolute {
		writeJSON(w, http.StatusBadRequest, response{Error: "宿主绝对路径配置文件默认不支持删除"})
		return
	}
	filePath, err := resolvedConfigFilePath(state.registry().Root, toolCfg, entry)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("删除配置文件失败: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"message": "配置文件已删除"}})
}

func declaredPluginConfigFile(reg *registry.Registry, pluginID, fileID string) (config.ConfigFileRef, *config.ToolConfig, error) {
	if strings.TrimSpace(fileID) == "" || strings.Contains(fileID, "\x00") {
		return config.ConfigFileRef{}, nil, fmt.Errorf("配置文件 ID 不安全")
	}
	for _, tool := range reg.Tools {
		if tool.Source.PluginID != pluginID {
			continue
		}
		for _, declared := range declaredConfigFiles(tool.Config) {
			declared = normalizeServerConfigFileRef(declared)
			if declared.ID == fileID {
				return declared, tool.Config, nil
			}
			expanded, err := expandDeclaredConfigFiles(reg.Root, tool.Config, declared)
			if err != nil {
				continue
			}
			for _, entry := range expanded {
				if entry.ID != fileID {
					continue
				}
				return entry, tool.Config, nil
			}
		}
	}
	return config.ConfigFileRef{}, nil, fmt.Errorf("配置文件 %s 未在插件 %s 中声明", fileID, pluginID)
}

func declaredConfigFiles(toolCfg *config.ToolConfig) []config.ConfigFileRef {
	if toolCfg == nil {
		return nil
	}
	if len(toolCfg.ConfigFileRefs) > 0 {
		out := make([]config.ConfigFileRef, 0, len(toolCfg.ConfigFileRefs))
		for _, entry := range toolCfg.ConfigFileRefs {
			out = append(out, normalizeServerConfigFileRef(entry))
		}
		return out
	}
	out := make([]config.ConfigFileRef, 0, len(toolCfg.ConfigFiles))
	for _, path := range toolCfg.ConfigFiles {
		out = append(out, config.NewPluginConfigFileRef(path))
	}
	return out
}

func workflowConfigDirRef(workflowID string) string {
	return filepath.ToSlash(filepath.Join("config", "workflows", workflowFilenameBase(workflowID)))
}

func workflowFilenameBase(id string) string {
	return strings.TrimSuffix(workflowFilename(id), ".yaml")
}

func workflowConfigDir(reg *registry.Registry, workflowID string) string {
	return filepath.Join(userWorkflowPluginDir(reg), "config", "workflows", workflowFilenameBase(workflowID))
}

func resolvedWorkflowConfigFilePath(reg *registry.Registry, workflowID string, entry config.ConfigFileRef) (string, error) {
	entry = normalizeWorkflowConfigFileRef(workflowID, entry)
	if entry.Scope != config.ConfigFileScopePlugin {
		return "", fmt.Errorf("工作流配置文件只支持 plugin scope")
	}
	if entry.Legacy {
		return joinConfigFilePath(workflowConfigDir(reg, workflowID), entry.Path)
	}
	baseDir, err := resolvedWorkflowConfigDir(reg, entry.ConfigDir)
	if err != nil {
		return "", err
	}
	return joinConfigFilePath(baseDir, entry.Path)
}

func workflowConfigFilePath(reg *registry.Registry, workflowID, path string) (string, error) {
	return resolvedWorkflowConfigFilePath(reg, workflowID, config.ConfigFileRef{
		ID:        path,
		ConfigDir: workflowConfigDirRef(workflowID),
		Path:      path,
		Scope:     config.ConfigFileScopePlugin,
		Access:    config.ConfigFileAccessReadWrite,
		Create:    true,
	})
}

func declaredWorkflowConfigFile(reg *registry.Registry, workflowID, fileID string) (config.ConfigFileRef, *registry.Workflow, error) {
	if strings.TrimSpace(fileID) == "" || strings.Contains(fileID, "\x00") {
		return config.ConfigFileRef{}, nil, fmt.Errorf("配置文件 ID 不安全")
	}
	wf, err := userWorkflow(reg, workflowID)
	if err != nil {
		return config.ConfigFileRef{}, nil, err
	}
	if len(wf.Config.ConfigFiles) > 0 {
		for _, entry := range declaredWorkflowConfigFiles(reg, wf) {
			if entry.ID == fileID {
				return entry, wf, nil
			}
		}
		return config.ConfigFileRef{}, nil, fmt.Errorf("配置文件 %s 未在工作流 %s 中声明", fileID, workflowID)
	}
	files, err := scanWorkflowConfigFiles(reg, workflowID)
	if err != nil {
		return config.ConfigFileRef{}, nil, err
	}
	for _, entry := range files {
		if entry.ID == fileID {
			return workflowStatusToConfigRef(workflowID, entry), wf, nil
		}
	}
	return config.ConfigFileRef{}, nil, fmt.Errorf("配置文件 %s 未在工作流 %s 中声明", fileID, workflowID)
}

func scanWorkflowConfigFiles(reg *registry.Registry, workflowID string) ([]pluginConfigFileStatus, error) {
	wf, err := userWorkflow(reg, workflowID)
	if err != nil {
		return nil, err
	}
	if len(wf.Config.ConfigFiles) > 0 {
		return workflowDeclaredConfigFileStatuses(reg, wf)
	}
	return scanWorkflowConfigDirFiles(reg, workflowID)
}

func scanWorkflowConfigDirFiles(reg *registry.Registry, workflowID string) ([]pluginConfigFileStatus, error) {
	root := workflowConfigDir(reg, workflowID)
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return []pluginConfigFileStatus{}, nil
		}
		return nil, err
	}
	files := []pluginConfigFileStatus{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		entry := pluginConfigFileStatus{
			ID:          rel,
			Label:       filepath.Base(rel),
			ConfigDir:   workflowConfigDirRef(workflowID),
			Path:        rel,
			DisplayPath: workflowConfigDisplayPath(config.ConfigFileRef{ConfigDir: workflowConfigDirRef(workflowID), Path: rel}),
			Scope:       config.ConfigFileScopePlugin,
			Access:      config.ConfigFileAccessReadWrite,
			Create:      true,
		}
		status := configFileStatus(nil, workflowStatusToConfigRef(workflowID, entry), path)
		status.ConfigDir = entry.ConfigDir
		status.DisplayPath = entry.DisplayPath
		files = append(files, status)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ID < files[j].ID })
	return files, nil
}

func workflowStatusToConfigRef(workflowID string, status pluginConfigFileStatus) config.ConfigFileRef {
	configDir := status.ConfigDir
	if configDir == "" {
		configDir = workflowConfigDirRef(workflowID)
	}
	return config.ConfigFileRef{
		ID:        status.ID,
		Label:     status.Label,
		ConfigDir: configDir,
		Path:      status.Path,
		Scope:     status.Scope,
		Access:    status.Access,
		Create:    status.Create,
		Legacy:    true,
	}
}

func declaredWorkflowConfigFiles(reg *registry.Registry, wf *registry.Workflow) []config.ConfigFileRef {
	if wf == nil || wf.Config == nil {
		return nil
	}
	out := make([]config.ConfigFileRef, 0, len(wf.Config.ConfigFiles))
	for _, entry := range wf.Config.ConfigFiles {
		out = append(out, normalizeWorkflowConfigFileRef(wf.Config.ID, entry))
	}
	return out
}

func workflowDeclaredConfigFileStatuses(reg *registry.Registry, wf *registry.Workflow) ([]pluginConfigFileStatus, error) {
	files := []pluginConfigFileStatus{}
	for _, entry := range declaredWorkflowConfigFiles(reg, wf) {
		filePath, err := resolvedWorkflowConfigFilePath(reg, wf.Config.ID, entry)
		if err != nil {
			files = append(files, workflowConfigFileStatus(entry, "", err.Error()))
			continue
		}
		files = append(files, workflowConfigFileStatus(entry, filePath, ""))
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ID < files[j].ID })
	return files, nil
}

func workflowConfigFileStatus(entry config.ConfigFileRef, filePath, reason string) pluginConfigFileStatus {
	status := pluginConfigFileStatus{
		ID:          entry.ID,
		Label:       entry.Label,
		ConfigDir:   entry.ConfigDir,
		DisplayRoot: workflowConfigDisplayRoot(entry),
		Path:        entry.Path,
		DisplayPath: workflowConfigDisplayPath(entry),
		Scope:       entry.Scope,
		Access:      entry.Access,
		Create:      entry.Create,
		Reason:      reason,
	}
	if filePath == "" {
		return status
	}
	next := configFileStatus(nil, entry, filePath)
	next.DisplayPath = workflowConfigDisplayPath(entry)
	if reason != "" {
		next.Reason = reason
	}
	return next
}

func workflowConfigFilesForCatalog(reg *registry.Registry, wf *config.WorkflowConfig) []config.ConfigFileRef {
	if wf == nil {
		return nil
	}
	if len(wf.ConfigFiles) > 0 {
		out := make([]config.ConfigFileRef, 0, len(wf.ConfigFiles))
		for _, entry := range wf.ConfigFiles {
			out = append(out, normalizeWorkflowConfigFileRef(wf.ID, entry))
		}
		return out
	}
	files, err := scanWorkflowConfigFiles(reg, wf.ID)
	if err == nil {
		out := make([]config.ConfigFileRef, 0, len(files))
		for _, file := range files {
			out = append(out, workflowStatusToConfigRef(wf.ID, file))
		}
		return out
	}
	out := make([]config.ConfigFileRef, 0, len(wf.ConfigFiles))
	for _, entry := range wf.ConfigFiles {
		out = append(out, normalizeWorkflowConfigFileRef(wf.ID, entry))
	}
	return out
}

func normalizeWorkflowConfigFileRef(workflowID string, entry config.ConfigFileRef) config.ConfigFileRef {
	legacy := entry.Legacy
	config.NormalizeConfigFileRef(&entry)
	entry.Legacy = legacy
	entry.Scope = config.ConfigFileScopePlugin
	if !entry.Legacy && entry.ConfigDir == workflowConfigDirRef(workflowID) {
		entry.ConfigDir = "."
	}
	if entry.Access == "" {
		entry.Access = config.ConfigFileAccessReadWrite
	}
	if entry.Path == "" {
		entry.Path = entry.ID
	}
	if entry.ID == "" {
		entry.ID = entry.Path
	}
	return entry
}

func resolvedWorkflowConfigDir(reg *registry.Registry, configDir string) (string, error) {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" {
		configDir = "config"
	}
	return joinWorkflowConfigDir(reg.BaseDir, configDir)
}

func joinWorkflowConfigDir(baseDir, configDir string) (string, error) {
	if strings.TrimSpace(configDir) == "" {
		return "", fmt.Errorf("工作流 config_dir 不能为空")
	}
	if strings.Contains(configDir, "://") || filepath.IsAbs(configDir) || strings.HasPrefix(configDir, "/") || strings.HasPrefix(configDir, "\\") || len(configDir) >= 2 && configDir[1] == ':' {
		return "", fmt.Errorf("工作流 config_dir 不能是绝对路径")
	}
	cleanDir := filepath.Clean(filepath.FromSlash(configDir))
	if cleanDir == "." {
		cleanDir = ""
	} else if cleanDir == ".." || strings.HasPrefix(cleanDir, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("工作流 config_dir 不能逃逸运行根目录")
	}
	for _, part := range strings.FieldsFunc(configDir, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "" || part == ".." {
			return "", fmt.Errorf("工作流 config_dir 包含不安全路径片段")
		}
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(baseAbs, cleanDir))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("工作流 config_dir 逃逸运行根目录")
	}
	return pathAbs, nil
}

func workflowConfigDisplayPath(entry config.ConfigFileRef) string {
	configDir := strings.TrimSpace(entry.ConfigDir)
	if configDir == "" || configDir == "." {
		return filepath.ToSlash(entry.Path)
	}
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(configDir), filepath.FromSlash(entry.Path)))
}

func workflowConfigDisplayRoot(entry config.ConfigFileRef) string {
	configDir := strings.TrimSpace(entry.ConfigDir)
	if configDir == "" || configDir == "." {
		return "运行根目录"
	}
	return filepath.ToSlash(configDir)
}

func userWorkflow(reg *registry.Registry, workflowID string) (*registry.Workflow, error) {
	if strings.TrimSpace(workflowID) == "" || strings.ContainsAny(workflowID, `/\`) || strings.Contains(workflowID, "\x00") {
		return nil, fmt.Errorf("工作流 ID 不安全")
	}
	wf, err := reg.Workflow(workflowID)
	if err != nil {
		return nil, err
	}
	if wf.Source.PluginID != userWorkflowPluginID || !isUserWorkflowPluginPath(reg, wf.Path) {
		return nil, fmt.Errorf("只能维护 Web 页面保存的用户工作流配置")
	}
	return wf, nil
}

func readConfigFileContent(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("配置文件不可读: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("配置文件不是普通文件")
	}
	if info.Size() > maxPluginConfigFileBytes {
		return "", fmt.Errorf("配置文件超过大小限制")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取配置文件失败: %w", err)
	}
	return string(data), nil
}

func writeConfigFileContent(filePath, content string, create bool) error {
	info, err := os.Stat(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("检查配置文件失败: %w", err)
		}
		if !create {
			return fmt.Errorf("配置文件不存在且声明不允许创建")
		}
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return fmt.Errorf("创建配置目录失败: %w", err)
		}
	} else {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("配置文件不是普通文件")
		}
		file, err := os.OpenFile(filePath, os.O_WRONLY, 0)
		if err != nil {
			return fmt.Errorf("配置文件不可写: %w", err)
		}
		_ = file.Close()
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("保存配置文件失败: %w", err)
	}
	return nil
}

func readWorkflowUploadResult(path string) (config.WorkflowUploadResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config.WorkflowUploadResult{}, fmt.Errorf("读取上传结果失败: %w", err)
	}
	var upload config.WorkflowUploadResult
	if err := json.Unmarshal(data, &upload); err != nil {
		return config.WorkflowUploadResult{}, fmt.Errorf("解析上传结果失败: %w", err)
	}
	return upload, nil
}

func saveWorkflowAssetForWorkflow(reg *registry.Registry, wf *config.WorkflowConfig) error {
	path := workflowPath(reg, wf.ID)
	if err := saveWorkflow(path, wf); err != nil {
		return err
	}
	if err := maintainUserWorkflowPluginManifest(reg); err != nil {
		return err
	}
	reg.Workflows[wf.ID] = &registry.Workflow{Entry: workflowEntryForSavedWorkflow(reg, path, wf), Config: wf, Path: path, Source: workflowSource(reg, path)}
	return nil
}

func expandDeclaredConfigFiles(root *config.RootConfig, toolCfg *config.ToolConfig, entry config.ConfigFileRef) ([]config.ConfigFileRef, error) {
	entry = normalizeServerConfigFileRef(entry)
	baseDir, err := resolvedConfigDir(root, toolCfg, entry)
	if err != nil {
		return nil, err
	}
	filePath, err := joinConfigFilePath(baseDir, entry.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []config.ConfigFileRef{entry}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return []config.ConfigFileRef{entry}, nil
	}
	items, err := os.ReadDir(filePath)
	if err != nil {
		return nil, err
	}
	out := make([]config.ConfigFileRef, 0, len(items))
	for _, item := range items {
		if item.Type()&os.ModeType != 0 {
			continue
		}
		child := entry
		child.Path = filepath.ToSlash(filepath.Join(entry.Path, item.Name()))
		child.ID = child.Path
		if entry.Scope == config.ConfigFileScopeHostAbsolute {
			child.ID = entry.ID + ":" + filepath.ToSlash(item.Name())
		}
		child.Label = item.Name()
		out = append(out, normalizeServerConfigFileRef(child))
	}
	return out, nil
}

func normalizeServerConfigFileRef(entry config.ConfigFileRef) config.ConfigFileRef {
	legacy := entry.Legacy
	config.NormalizeConfigFileRef(&entry)
	if legacy {
		entry.ConfigDir = "."
	}
	return entry
}

func resolvedConfigFilePath(root *config.RootConfig, toolCfg *config.ToolConfig, entry config.ConfigFileRef) (string, error) {
	entry = normalizeServerConfigFileRef(entry)
	baseDir, err := resolvedConfigDir(root, toolCfg, entry)
	if err != nil {
		return "", err
	}
	filePath, err := joinConfigFilePath(baseDir, entry.Path)
	if err != nil {
		return "", err
	}
	if entry.Scope != config.ConfigFileScopeHostAbsolute {
		return filePath, nil
	}
	allowedDirs, err := normalizeHostAllowedDirsForServer(root)
	if err != nil {
		return "", err
	}
	if err := ensureHostPathAllowedForServer(filePath, allowedDirs); err != nil {
		return "", err
	}
	resolved, err := resolveHostConfigFileForServer(filePath)
	if err != nil {
		return "", err
	}
	if err := ensureHostPathAllowedForServer(resolved, allowedDirs); err != nil {
		return "", fmt.Errorf("宿主配置文件符号链接最终路径未命中白名单: %w", err)
	}
	return filepath.Clean(resolved), nil
}

func resolvedConfigDir(root *config.RootConfig, toolCfg *config.ToolConfig, entry config.ConfigFileRef) (string, error) {
	entry = normalizeServerConfigFileRef(entry)
	switch entry.Scope {
	case config.ConfigFileScopePlugin:
		baseDir, err := plugin.ResolveConfigDir(toolCfg.PluginConfig.Dir, entry.ConfigDir)
		if err != nil {
			return "", fmt.Errorf("config_dir 不安全: %w", err)
		}
		return baseDir, nil
	case config.ConfigFileScopeHostAbsolute:
		if !filepath.IsAbs(entry.ConfigDir) {
			return "", fmt.Errorf("宿主 config_dir 必须是绝对路径")
		}
		allowedDirs, err := normalizeHostAllowedDirsForServer(root)
		if err != nil {
			return "", err
		}
		cleanAbs, err := filepath.Abs(filepath.Clean(entry.ConfigDir))
		if err != nil {
			return "", err
		}
		if err := ensureHostPathAllowedForServer(cleanAbs, allowedDirs); err != nil {
			return "", err
		}
		resolved, err := resolveHostConfigFileForServer(cleanAbs)
		if err != nil {
			return "", err
		}
		if err := ensureHostPathAllowedForServer(resolved, allowedDirs); err != nil {
			return "", fmt.Errorf("宿主 config_dir 符号链接最终路径未命中白名单: %w", err)
		}
		return filepath.Clean(resolved), nil
	default:
		return "", fmt.Errorf("配置文件 scope 不支持: %s", entry.Scope)
	}
}

func joinConfigFilePath(baseDir, item string) (string, error) {
	if strings.TrimSpace(item) == "" {
		return "", fmt.Errorf("config_files 条目不能为空")
	}
	if filepath.IsAbs(item) {
		return "", fmt.Errorf("config_files 条目不能是绝对路径")
	}
	cleanItem := filepath.Clean(filepath.FromSlash(item))
	if cleanItem == "." || cleanItem == ".." || strings.HasPrefix(cleanItem, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("config_files 条目不能逃逸 config_dir")
	}
	for _, part := range strings.FieldsFunc(item, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("config_files 条目包含不安全路径片段")
		}
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(baseAbs, cleanItem))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("config_files 条目逃逸 config_dir")
	}
	return pathAbs, nil
}
func normalizeHostAllowedDirsForServer(root *config.RootConfig) ([]string, error) {
	if root == nil || len(root.HostConfigFiles.AllowedDirs) == 0 {
		return nil, fmt.Errorf("未配置 host_config_files.allowed_dirs")
	}
	allowedDirs := make([]string, 0, len(root.HostConfigFiles.AllowedDirs))
	for index, dir := range root.HostConfigFiles.AllowedDirs {
		if strings.TrimSpace(dir) == "" {
			return nil, fmt.Errorf("host_config_files.allowed_dirs 第 %d 项不能为空", index+1)
		}
		if !filepath.IsAbs(dir) {
			return nil, fmt.Errorf("host_config_files.allowed_dirs 第 %d 项必须是当前平台可识别的绝对目录", index+1)
		}
		cleanAbs, err := filepath.Abs(filepath.Clean(dir))
		if err != nil {
			return nil, fmt.Errorf("host_config_files.allowed_dirs 第 %d 项无效: %w", index+1, err)
		}
		info, err := os.Stat(cleanAbs)
		if err != nil {
			return nil, fmt.Errorf("host_config_files.allowed_dirs 第 %d 项必须是已存在目录: %w", index+1, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("host_config_files.allowed_dirs 第 %d 项只支持目录白名单，不支持单文件", index+1)
		}
		resolved, err := filepath.EvalSymlinks(cleanAbs)
		if err != nil {
			return nil, fmt.Errorf("host_config_files.allowed_dirs 第 %d 项符号链接解析失败: %w", index+1, err)
		}
		allowedDirs = append(allowedDirs, filepath.Clean(resolved))
	}
	return allowedDirs, nil
}

func ensureHostPathAllowedForServer(path string, allowedDirs []string) error {
	cleanPath := filepath.Clean(path)
	for _, dir := range allowedDirs {
		cleanDir := filepath.Clean(dir)
		rel, err := filepath.Rel(cleanDir, cleanPath)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel) {
			return nil
		}
	}
	return fmt.Errorf("路径 %s 不在 host_config_files.allowed_dirs 白名单内", path)
}

func resolveHostConfigFileForServer(cleanAbs string) (string, error) {
	if _, err := os.Lstat(cleanAbs); err == nil {
		return filepath.EvalSymlinks(cleanAbs)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(cleanAbs)
	for {
		if info, err := os.Stat(parent); err == nil {
			if !info.IsDir() {
				return "", fmt.Errorf("父路径不是目录: %s", parent)
			}
			resolvedParent, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return "", err
			}
			rel, err := filepath.Rel(parent, cleanAbs)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolvedParent, rel), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", fmt.Errorf("未找到已存在父目录")
		}
		parent = next
	}
}

func configFileStatus(toolCfg *config.ToolConfig, entry config.ConfigFileRef, filePath string) pluginConfigFileStatus {
	entry = normalizeServerConfigFileRef(entry)
	status := pluginConfigFileStatus{ID: entry.ID, Label: entry.Label, ConfigDir: entry.ConfigDir, Path: entry.Path, DisplayPath: displayConfigFilePath(entry), Scope: entry.Scope, Access: entry.Access, Create: entry.Create}
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			status.Reason = "文件不存在"
			if entry.Scope == config.ConfigFileScopePlugin {
				status.Writable = entry.Access == config.ConfigFileAccessReadWrite
				return status
			}
			if entry.Create {
				if parentWritable(filepath.Dir(filePath)) {
					status.Writable = entry.Access == config.ConfigFileAccessReadWrite
				} else {
					status.Reason = "父目录不可写或不存在"
				}
			}
			return status
		}
		status.Reason = err.Error()
		return status
	}
	status.Exists = true
	status.Size = info.Size()
	if !info.Mode().IsRegular() {
		status.Reason = "不是普通文件"
		return status
	}
	if info.Size() > maxPluginConfigFileBytes {
		status.Reason = "超过大小限制"
		return status
	}
	if file, err := os.Open(filePath); err == nil {
		status.Readable = true
		_ = file.Close()
	} else {
		status.Reason = "不可读: " + err.Error()
	}
	if entry.Access == config.ConfigFileAccessReadWrite {
		if file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0); err == nil {
			status.Writable = true
			_ = file.Close()
		} else if status.Reason == "" {
			status.Reason = "不可写: " + err.Error()
		}
	} else if status.Reason == "" {
		status.Reason = "只读"
	}
	return status
}

func displayConfigFilePath(entry config.ConfigFileRef) string {
	entry = normalizeServerConfigFileRef(entry)
	if entry.ConfigDir == "" || entry.ConfigDir == "." {
		return filepath.ToSlash(entry.Path)
	}
	return filepath.ToSlash(filepath.Join(entry.ConfigDir, entry.Path))
}

func parentWritable(parent string) bool {
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return false
	}
	tmp, err := os.CreateTemp(parent, ".opsctl-perm-*")
	if err != nil {
		return false
	}
	name := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(name)
	return true
}

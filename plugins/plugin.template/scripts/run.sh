#!/usr/bin/env bash
set -euo pipefail

target="${OPS_PARAM_TARGET:-}"
action="${OPS_PARAM_ACTION:-inspect}"
dry_run="${OPS_PARAM_DRY_RUN:-true}"

usage() {
  cat >&2 <<'EOF'
用法: run.sh --target <target> [--action inspect|apply] [--dry-run true|false]

参数:
  --target    必填。目标标识，例如主机组、实例名或环境名。
  --action    可选。inspect 只读检查；apply 表示执行变更示例。
  --dry-run   可选。true 仅预览；false 表示执行真实动作。
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

# 不要输出密码、令牌、密钥、完整连接串等敏感信息。
info "插件工具开始执行"
info "目标: ${target}"
info "动作: ${action}"
info "dry-run: ${dry_run}"

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

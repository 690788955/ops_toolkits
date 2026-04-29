#!/usr/bin/env bash
set -euo pipefail

target="${OPS_PARAM_TARGET:-demo}"
service="${OPS_PARAM_SERVICE:-nginx}"
status="${OPS_PARAM_STATUS:-OK}"

require_value() {
  local flag="$1"
  local value="${2:-}"

  if [[ -z "$value" || "$value" == --* ]]; then
    echo "参数 ${flag} 需要提供值" >&2
    exit 1
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      require_value "$1" "${2:-}"
      target="$2"
      shift 2
      ;;
    --service)
      require_value "$1" "${2:-}"
      service="$2"
      shift 2
      ;;
    --status)
      require_value "$1" "${2:-}"
      status="$2"
      shift 2
      ;;
    *)
      echo "未知参数: $1" >&2
      exit 1
      ;;
  esac
done

echo "[plugin.inspect.check] 巡检开始"
echo "目标: ${target}"
echo "CPU: 18% used (模拟)"
echo "磁盘: 42% used (模拟)"
echo "服务: ${service}"
echo "状态: ${status}"
echo "说明: 本工具只输出模拟巡检日志，不修改任何系统状态。"
echo "[plugin.inspect.check] 巡检完成"

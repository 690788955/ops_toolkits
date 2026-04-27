#!/usr/bin/env bash
set -euo pipefail

target="${OPS_PARAM_TARGET:-demo-target}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      target="${2:-$target}"
      shift 2
      ;;
    *)
      echo "未知参数: $1" >&2
      exit 1
      ;;
  esac
done

echo "[plugin.demo.confirmed] 已通过框架确认流程"
echo "模拟操作目标: ${target}"
echo "这里只输出日志，不修改任何系统状态。"

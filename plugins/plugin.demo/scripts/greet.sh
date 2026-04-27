#!/usr/bin/env bash
set -euo pipefail

name="${OPS_PARAM_NAME:-World}"
message="${OPS_PARAM_MESSAGE:-Hello from plugin}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --name)
      name="${2:-$name}"
      shift 2
      ;;
    --message)
      message="${2:-$message}"
      shift 2
      ;;
    *)
      echo "未知参数: $1" >&2
      exit 1
      ;;
  esac
done

echo "[plugin.demo.greet] 工具开始执行"
echo "消息: ${message}"
echo "名称: ${name}"
echo "工作目录: $(pwd)"

if [[ -n "${OPS_PARAM_FILE:-}" ]]; then
  echo "参数文件: ${OPS_PARAM_FILE}"
fi

echo "[plugin.demo.greet] 工具执行完成"

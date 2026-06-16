#!/usr/bin/env bash
set -euo pipefail

# 宿主机执行命令
# 用法: host-command.sh <命令>
# 示例: host-command.sh "docker ps"

COMMAND="${1:-}"

if [[ -z "$COMMAND" ]]; then
  echo "错误: 未指定要执行的宿主机命令" >&2
  echo "用法: $0 <命令>" >&2
  exit 1
fi

echo "========================================="
echo "宿主机命令执行"
echo "========================================="
echo "命令: $COMMAND"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================="
echo ""

bash -lc "$COMMAND"
EXIT_CODE=$?

echo ""
echo "========================================="
if [[ $EXIT_CODE -eq 0 ]]; then
  echo "执行完成 (退出码: $EXIT_CODE)"
else
  echo "执行失败 (退出码: $EXIT_CODE)" >&2
fi
echo "========================================="

exit $EXIT_CODE

#!/usr/bin/env bash
set -uo pipefail

# 查看 k3s / k3s-agent 服务日志（只读）。
# 用法: logs.sh [service] [lines] [since]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

SERVICE_REQUESTED="${1:-auto}"
LINES="${2:-200}"
SINCE="${3:-1h}"

if ! has_command journalctl; then
  echo "错误: 未找到 journalctl，当前系统可能未使用 systemd" >&2
  exit 1
fi

if ! [[ "$LINES" =~ ^[0-9]+$ ]]; then
  echo "错误: lines 必须是正整数，当前值: $LINES" >&2
  exit 1
fi

SERVICE="$(pick_k3s_service "$SERVICE_REQUESTED")"
SINCE_VALUE="$(normalize_since "$SINCE")"

echo "========================================="
echo "服务日志: $SERVICE"
echo "行数: $LINES，起始: $SINCE_VALUE"
echo "========================================="
echo

if ! journalctl -u "$SERVICE" --since "$SINCE_VALUE" -n "$LINES" --no-pager; then
  echo "错误: 读取日志失败，请确认当前用户有权限（通常需要 root 或 sudo）" >&2
  exit 1
fi

exit 0
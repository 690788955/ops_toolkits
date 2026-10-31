#!/usr/bin/env bash
set -uo pipefail

# k3s / k3s-agent 服务控制。最后一行输出 JSON。
# 用法: service.sh <status|start|stop|restart> [auto|k3s|k3s-agent]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

ACTION="${1:-status}"
SERVICE_REQUESTED="${2:-auto}"

case "$ACTION" in
  status | start | stop | restart) ;;
  *)
    echo "错误: 不支持的操作 $ACTION，可选 status/start/stop/restart" >&2
    exit 1
    ;;
esac

if ! has_command systemctl; then
  echo "错误: 未找到 systemctl，当前系统可能未使用 systemd" >&2
  exit 1
fi

SERVICE="$(pick_k3s_service "$SERVICE_REQUESTED")"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "警告: 当前用户不是 root，systemctl 操作可能因权限不足失败" >&2
fi

echo "========================================="
echo "k3s 服务操作"
echo "服务: $SERVICE"
echo "操作: $ACTION"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================="
echo

EXIT_CODE=0
if [[ "$ACTION" == "status" ]]; then
  systemctl status "$SERVICE" --no-pager -l || true
else
  systemctl "$ACTION" "$SERVICE"
  EXIT_CODE=$?
  if [[ "$EXIT_CODE" -ne 0 ]]; then
    echo "错误: systemctl $ACTION $SERVICE 执行失败（退出码 $EXIT_CODE）" >&2
  fi
fi

ACTIVE="$(systemctl is-active "$SERVICE" 2>/dev/null || true)"
ACTIVE="${ACTIVE:-unknown}"
ENABLED="$(systemctl is-enabled "$SERVICE" 2>/dev/null || true)"
ENABLED="${ENABLED:-unknown}"

echo
echo "当前状态: $ACTIVE（开机自启: $ENABLED）"
echo "========================================="

printf '{"service":"%s","action":"%s","active":"%s","enabled":"%s"}\n' \
  "$(json_escape "$SERVICE")" \
  "$(json_escape "$ACTION")" \
  "$(json_escape "$ACTIVE")" \
  "$(json_escape "$ENABLED")"

exit "$EXIT_CODE"
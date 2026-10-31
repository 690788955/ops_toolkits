#!/usr/bin/env bash
set -uo pipefail

# Helm 仓库管理。
# 用法: helm-repo.sh <list|add|update> [名称] [地址]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

ACTION="${1:-list}"
NAME="${2:-}"
URL="${3:-}"

HELM="$(helm_bin)" || exit 1

case "$ACTION" in
  list)
    "$HELM" repo list
    ;;
  update)
    "$HELM" repo update
    ;;
  add)
    if [[ -z "$NAME" || -z "$URL" ]]; then
      echo "错误: action=add 必须提供仓库名称和地址" >&2
      exit 1
    fi
    "$HELM" repo add "$NAME" "$URL"
    ;;
  *)
    echo "错误: 不支持的操作 $ACTION，可选 list/add/update" >&2
    exit 1
    ;;
esac
EXIT_CODE=$?

if [[ "$EXIT_CODE" -ne 0 ]]; then
  echo "错误: helm repo $ACTION 执行失败（退出码 $EXIT_CODE）" >&2
  exit "$EXIT_CODE"
fi

printf '{"action":"%s","name":"%s","url":"%s"}\n' \
  "$(json_escape "$ACTION")" \
  "$(json_escape "$NAME")" \
  "$(json_escape "$URL")"

exit 0
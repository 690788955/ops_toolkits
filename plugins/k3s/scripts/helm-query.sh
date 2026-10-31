#!/usr/bin/env bash
set -uo pipefail

# Helm 发布查询（只读）。
# 用法: helm-query.sh <list|status|history> [release] [namespace] [yes|no]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

ACTION="${1:-list}"
RELEASE="${2:-}"
NAMESPACE="${3:-}"
ALL_NAMESPACES="${4:-no}"

HELM="$(helm_bin)" || exit 1
if [[ -z "${KUBECONFIG:-}" && -r /etc/rancher/k3s/k3s.yaml ]]; then
  export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
fi

ARGS=()
case "$ACTION" in
  list)
    ARGS=(list)
    if [[ "$ALL_NAMESPACES" == "yes" ]]; then
      ARGS+=(-A)
    elif [[ -n "$NAMESPACE" ]]; then
      ARGS+=(-n "$NAMESPACE")
    fi
    ;;
  status | history)
    if [[ -z "$RELEASE" ]]; then
      echo "错误: action=$ACTION 必须提供 release 名称" >&2
      exit 1
    fi
    ARGS=("$ACTION" "$RELEASE")
    if [[ -n "$NAMESPACE" ]]; then
      ARGS+=(-n "$NAMESPACE")
    fi
    ;;
  *)
    echo "错误: 不支持的操作 $ACTION，可选 list/status/history" >&2
    exit 1
    ;;
esac

echo "========================================="
echo "Helm 发布查询: $ACTION"
echo "========================================="
echo

"$HELM" "${ARGS[@]}"
EXIT_CODE=$?

if [[ "$EXIT_CODE" -ne 0 ]]; then
  echo "错误: helm ${ARGS[*]} 执行失败（退出码 $EXIT_CODE），请确认 release 与命名空间是否正确" >&2
  exit "$EXIT_CODE"
fi

exit 0
#!/usr/bin/env bash
set -uo pipefail

# 回滚 Helm 发布到指定版本。
# 用法: helm-rollback.sh <release> [revision] [namespace] [yes|no wait]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

RELEASE="${1:-}"
REVISION="${2:-0}"
NAMESPACE="${3:-default}"
WAIT="${4:-yes}"

if [[ -z "$RELEASE" ]]; then
  echo "错误: release 为必填参数" >&2
  exit 1
fi
if [[ -z "$NAMESPACE" ]]; then
  NAMESPACE="default"
fi
if ! [[ "$REVISION" =~ ^[0-9]+$ ]]; then
  echo "错误: revision 必须是整数（0 表示回滚到上一版本），当前值: $REVISION" >&2
  exit 1
fi

HELM="$(helm_bin)" || exit 1
if [[ -z "${KUBECONFIG:-}" && -r /etc/rancher/k3s/k3s.yaml ]]; then
  export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
fi

ARGS=(rollback "$RELEASE")
if [[ "$REVISION" -gt 0 ]]; then
  ARGS+=("$REVISION")
fi
ARGS+=(--namespace "$NAMESPACE")
if [[ "$WAIT" == "yes" ]]; then
  ARGS+=(--wait)
fi

echo "========================================="
echo "Helm 回滚"
echo "release: $RELEASE"
echo "目标 revision: $([[ "$REVISION" -gt 0 ]] && echo "$REVISION" || echo "上一版本")"
echo "namespace: $NAMESPACE"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================="
echo

"$HELM" "${ARGS[@]}"
EXIT_CODE=$?

if [[ "$EXIT_CODE" -ne 0 ]]; then
  echo "错误: helm rollback 失败（退出码 $EXIT_CODE），可通过 helm history 查看可用版本" >&2
  exit "$EXIT_CODE"
fi

printf '{"release":"%s","revision":"%s","namespace":"%s"}\n' \
  "$(json_escape "$RELEASE")" \
  "$(json_escape "$REVISION")" \
  "$(json_escape "$NAMESPACE")"

exit 0
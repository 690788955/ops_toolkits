#!/usr/bin/env bash
set -uo pipefail

# 卸载 Helm 发布。
# 用法: helm-uninstall.sh <release> [namespace] [yes|no 保留历史]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

RELEASE="${1:-}"
NAMESPACE="${2:-default}"
KEEP_HISTORY="${3:-no}"

if [[ -z "$RELEASE" ]]; then
  echo "错误: release 为必填参数" >&2
  exit 1
fi
if [[ -z "$NAMESPACE" ]]; then
  NAMESPACE="default"
fi

HELM="$(helm_bin)" || exit 1
if [[ -z "${KUBECONFIG:-}" && -r /etc/rancher/k3s/k3s.yaml ]]; then
  export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
fi

ARGS=(uninstall "$RELEASE" --namespace "$NAMESPACE")
if [[ "$KEEP_HISTORY" == "yes" ]]; then
  ARGS+=(--keep-history)
fi

echo "========================================="
echo "Helm 卸载"
echo "release: $RELEASE"
echo "namespace: $NAMESPACE"
echo "保留历史: $KEEP_HISTORY"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================="
echo

"$HELM" "${ARGS[@]}"
EXIT_CODE=$?

if [[ "$EXIT_CODE" -ne 0 ]]; then
  echo "错误: helm uninstall 失败（退出码 $EXIT_CODE），请确认 release 是否存在" >&2
  exit "$EXIT_CODE"
fi

printf '{"release":"%s","namespace":"%s","keep_history":"%s"}\n' \
  "$(json_escape "$RELEASE")" \
  "$(json_escape "$NAMESPACE")" \
  "$(json_escape "$KEEP_HISTORY")"

exit 0
#!/usr/bin/env bash
set -uo pipefail

# 通过 helm upgrade --install 安装或升级发布。
# 用法: helm-deploy.sh <release> <chart> [namespace] [version] [values_file] [set_values]
#                     [create_namespace] [atomic] [wait] [dry_run] [timeout]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

RELEASE="${1:-}"
CHART="${2:-}"
NAMESPACE="${3:-default}"
VERSION="${4:-}"
VALUES_FILE="${5:-}"
SET_VALUES="${6:-}"
CREATE_NAMESPACE="${7:-yes}"
ATOMIC="${8:-yes}"
WAIT="${9:-yes}"
DRY_RUN="${10:-no}"
TIMEOUT="${11:-10m}"

if [[ -z "$RELEASE" || -z "$CHART" ]]; then
  echo "错误: release 和 chart 均为必填参数" >&2
  exit 1
fi
if [[ -z "$NAMESPACE" ]]; then
  NAMESPACE="default"
fi
if [[ -n "$VALUES_FILE" && ! -f "$VALUES_FILE" ]]; then
  echo "错误: values 文件不存在: $VALUES_FILE（需为目标节点上的绝对路径）" >&2
  exit 1
fi

HELM="$(helm_bin)" || exit 1
if [[ -z "${KUBECONFIG:-}" && -r /etc/rancher/k3s/k3s.yaml ]]; then
  export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
fi

ARGS=(upgrade --install "$RELEASE" "$CHART" --namespace "$NAMESPACE")
if [[ "$CREATE_NAMESPACE" == "yes" ]]; then
  ARGS+=(--create-namespace)
fi
if [[ -n "$VERSION" ]]; then
  ARGS+=(--version "$VERSION")
fi
if [[ -n "$VALUES_FILE" ]]; then
  ARGS+=(--values "$VALUES_FILE")
fi
if [[ -n "$SET_VALUES" ]]; then
  ARGS+=(--set "$SET_VALUES")
fi
if [[ "$ATOMIC" == "yes" ]]; then
  ARGS+=(--atomic)
fi
if [[ "$WAIT" == "yes" ]]; then
  ARGS+=(--wait)
fi
if [[ "$DRY_RUN" == "yes" ]]; then
  ARGS+=(--dry-run)
fi
if [[ -n "$TIMEOUT" ]]; then
  ARGS+=(--timeout "$TIMEOUT")
fi

DISPLAY="helm $(printf '%s ' "${ARGS[@]}")"
if [[ -n "$SET_VALUES" ]]; then
  DISPLAY="${DISPLAY//$SET_VALUES/***}"
fi

echo "========================================="
echo "Helm 安装/升级"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "命令: $DISPLAY"
if [[ "$DRY_RUN" == "yes" ]]; then
  echo "模式: dry-run（仅渲染，不提交到集群）"
fi
echo "========================================="
echo

"$HELM" "${ARGS[@]}"
EXIT_CODE=$?

if [[ "$EXIT_CODE" -ne 0 ]]; then
  echo "错误: helm upgrade --install 失败（退出码 $EXIT_CODE），请结合上方输出排查" >&2
  exit "$EXIT_CODE"
fi

echo
echo "========================================="
echo "执行完成"
echo "========================================="

printf '{"release":"%s","chart":"%s","namespace":"%s","version":"%s","dry_run":"%s"}\n' \
  "$(json_escape "$RELEASE")" \
  "$(json_escape "$CHART")" \
  "$(json_escape "$NAMESPACE")" \
  "$(json_escape "$VERSION")" \
  "$(json_escape "$DRY_RUN")"

exit 0
#!/usr/bin/env bash
set -uo pipefail

# 按预设动作执行 kubectl 命令，避免自由填写整条命令。
# 用法: kubectl.sh <action> [namespace] [resource] [replicas] [lines]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

ACTION="${1:-}"
NAMESPACE="${2:-default}"
RESOURCE="${3:-}"
REPLICAS="${4:-1}"
LINES="${5:-200}"

require_kube
setup_kubeconfig ""

require_resource() {
  if [[ -z "$RESOURCE" ]]; then
    echo "错误: 动作 $ACTION 需要填写资源名称（节点名 / Pod 名 / Deployment 名）" >&2
    exit 1
  fi
}

require_namespace() {
  if [[ -z "$NAMESPACE" ]]; then
    echo "错误: 动作 $ACTION 需要指定命名空间" >&2
    exit 1
  fi
}

CMD=()

case "$ACTION" in
  get_nodes)
    CMD=(get nodes -o wide)
    ;;
  get_pods)
    require_namespace
    CMD=(get pods -n "$NAMESPACE" -o wide)
    ;;
  get_pods_all)
    CMD=(get pods -A -o wide)
    ;;
  get_deployments)
    require_namespace
    CMD=(get deployments -n "$NAMESPACE" -o wide)
    ;;
  get_services)
    require_namespace
    CMD=(get services -n "$NAMESPACE" -o wide)
    ;;
  get_events)
    require_namespace
    CMD=(get events -n "$NAMESPACE" --sort-by=.lastTimestamp)
    ;;
  describe_node)
    require_resource
    CMD=(describe node "$RESOURCE")
    ;;
  describe_pod)
    require_resource
    require_namespace
    CMD=(describe pod "$RESOURCE" -n "$NAMESPACE")
    ;;
  describe_deployment)
    require_resource
    require_namespace
    CMD=(describe deployment "$RESOURCE" -n "$NAMESPACE")
    ;;
  logs_pod)
    require_resource
    require_namespace
    if ! [[ "$LINES" =~ ^[0-9]+$ ]]; then
      echo "错误: lines 必须是正整数，当前值: $LINES" >&2
      exit 1
    fi
    CMD=(logs "$RESOURCE" -n "$NAMESPACE" --tail="$LINES")
    ;;
  rollout_restart)
    require_resource
    require_namespace
    CMD=(rollout restart "deployment/$RESOURCE" -n "$NAMESPACE")
    ;;
  rollout_status)
    require_resource
    require_namespace
    CMD=(rollout status "deployment/$RESOURCE" -n "$NAMESPACE")
    ;;
  scale_deployment)
    require_resource
    require_namespace
    if ! [[ "$REPLICAS" =~ ^[0-9]+$ ]]; then
      echo "错误: replicas 必须是非负整数，当前值: $REPLICAS" >&2
      exit 1
    fi
    CMD=(scale "deployment/$RESOURCE" --replicas="$REPLICAS" -n "$NAMESPACE")
    ;;
  delete_pod)
    require_resource
    require_namespace
    CMD=(delete pod "$RESOURCE" -n "$NAMESPACE")
    ;;
  cordon_node)
    require_resource
    CMD=(cordon "$RESOURCE")
    ;;
  uncordon_node)
    require_resource
    CMD=(uncordon "$RESOURCE")
    ;;
  drain_node)
    require_resource
    CMD=(drain "$RESOURCE" --ignore-daemonsets --delete-emptydir-data)
    ;;
  top_nodes)
    CMD=(top nodes)
    ;;
  top_pods)
    require_namespace
    CMD=(top pods -n "$NAMESPACE")
    ;;
  *)
    echo "错误: 不支持的动作 $ACTION" >&2
    echo "可选动作: get_nodes get_pods get_pods_all get_deployments get_services get_events describe_node describe_pod describe_deployment logs_pod rollout_restart rollout_status scale_deployment delete_pod cordon_node uncordon_node drain_node top_nodes top_pods" >&2
    exit 1
    ;;
esac

echo "========================================="
echo "kubectl 操作: $ACTION"
echo "命令: kubectl ${CMD[*]}"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================="
echo

if [[ "$ACTION" == "get_events" ]]; then
  kube "${CMD[@]}" | tail -n 50
else
  kube "${CMD[@]}"
fi
EXIT_CODE=$?

echo
echo "========================================="
echo "退出码: $EXIT_CODE"
echo "========================================="

printf '{"action":"%s","namespace":"%s","resource":"%s","replicas":"%s"}\n' \
  "$(json_escape "$ACTION")" \
  "$(json_escape "$NAMESPACE")" \
  "$(json_escape "$RESOURCE")" \
  "$(json_escape "$REPLICAS")"

exit "$EXIT_CODE"
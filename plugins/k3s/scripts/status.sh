#!/usr/bin/env bash
set -uo pipefail

# k3s 集群巡检（只读），最后一行输出 JSON 供工作流引用。
# 用法: status.sh [kubeconfig]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

KUBECONFIG_OVERRIDE="${1:-}"

require_kube
setup_kubeconfig "$KUBECONFIG_OVERRIDE"

echo "========================================="
echo "k3s 集群巡检（只读）"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "节点: $(hostname 2>/dev/null || uname -n)"
echo "========================================="
echo

VERSION=""
if has_command k3s; then
  VERSION="$(k3s --version 2>/dev/null | head -n1 | awk '{print $3}')"
fi
if [[ -z "$VERSION" ]]; then
  VERSION="$(kube version 2>/dev/null | awk '/Server Version/ {print $3; exit}' | tr -d 'v')"
else
  VERSION="${VERSION#v}"
fi
echo "Kubernetes 版本: ${VERSION:-未知}"
echo

SERVICES=""
SERVICE_OK=1
if has_command systemctl; then
  for svc in k3s k3s-agent; do
    if service_exists "$svc"; then
      state="$(systemctl is-active "$svc" 2>/dev/null || true)"
      state="${state:-unknown}"
      echo "服务 $svc: $state"
      SERVICES="${SERVICES}${svc}:${state} "
      if [[ "$state" != "active" ]]; then
        SERVICE_OK=0
      fi
    fi
  done
else
  echo "服务状态: 当前系统未使用 systemd，跳过服务检查"
fi
echo

NODES_TOTAL=0
NODES_READY=0
NODE_LINES="$(kube get nodes --no-headers 2>/dev/null || true)"
if [[ -n "$NODE_LINES" ]]; then
  read -r NODES_TOTAL NODES_READY <<<"$(printf '%s\n' "$NODE_LINES" | awk '{
    total++
    status = $2
    if (status == "Ready" || status ~ /^Ready,/) ready++
  } END { print total + 0, ready + 0 }')"
  echo "节点 ($NODES_READY/$NODES_TOTAL Ready):"
  printf '%s\n' "$NODE_LINES" | sed 's/^/  /'
else
  echo "节点: 无法获取，请检查 kubeconfig 是否存在且可读（通常需要 root）"
fi
echo

PODS_TOTAL=0
PODS_UNHEALTHY=0
PODS_QUERY_OK=0
if PODS_ALL="$(kube get pods -A --no-headers 2>/dev/null)"; then
  PODS_QUERY_OK=1
fi
if [[ -n "$PODS_ALL" ]]; then
  read -r PODS_TOTAL PODS_UNHEALTHY <<<"$(printf '%s\n' "$PODS_ALL" | awk '{
    if (NF == 0) next
    total++
    if ($4 != "Running" && $4 != "Completed") unhealthy++
  } END { print total + 0, unhealthy + 0 }')"
  echo "Pod: 共 $PODS_TOTAL 个，异常 $PODS_UNHEALTHY 个"
  if [[ "$PODS_UNHEALTHY" -gt 0 ]]; then
    echo "异常明细（最多 15 条）:"
    printf '%s\n' "$PODS_ALL" | awk 'NF > 0 && $4 != "Running" && $4 != "Completed" { printf "  %s/%s  %s\n", $1, $2, $4 }' | head -n 15
  fi
elif [[ "$PODS_QUERY_OK" -eq 1 ]]; then
  echo "Pod: 集群暂无 Pod"
else
  echo "Pod: 无法获取"
fi
echo

if command -v df >/dev/null 2>&1; then
  echo "根分区: $(df -P / 2>/dev/null | awk 'NR==2 {print $5" used, "$4" available"}')"
fi
if command -v free >/dev/null 2>&1; then
  echo "内存: $(free -m 2>/dev/null | awk 'NR==2 {print $3"MB / "$2"MB"}')"
fi
if [[ -r /proc/loadavg ]]; then
  echo "负载: $(awk '{print $1, $2, $3}' /proc/loadavg)"
fi
echo

CLUSTER_OK="false"
if [[ "$NODES_TOTAL" -gt 0 && "$NODES_READY" -eq "$NODES_TOTAL" && "$PODS_QUERY_OK" -eq 1 && "$PODS_UNHEALTHY" -eq 0 && "$SERVICE_OK" -eq 1 ]]; then
  CLUSTER_OK="true"
  echo "结论: 集群健康"
else
  echo "结论: 集群存在异常项，请结合上面的明细排查"
fi
echo "========================================="

printf '{"version":"%s","nodes_total":%d,"nodes_ready":%d,"pods_total":%d,"pods_unhealthy":%d,"services":"%s","cluster_ok":%s}\n' \
  "$(json_escape "$VERSION")" \
  "$NODES_TOTAL" \
  "$NODES_READY" \
  "$PODS_TOTAL" \
  "$PODS_UNHEALTHY" \
  "$(json_escape "${SERVICES% }")" \
  "$CLUSTER_OK"

exit 0

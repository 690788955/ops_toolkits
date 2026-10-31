#!/usr/bin/env bash
set -uo pipefail

# 收集 k3s 诊断信息并生成报告文件（只读），最后一行输出 JSON。
# 用法: diagnose.sh [报告目录] [kubeconfig]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

REPORT_DIR_INPUT="${1:-}"
KUBECONFIG_OVERRIDE="${2:-}"

require_kube
setup_kubeconfig "$KUBECONFIG_OVERRIDE"

resolve_report_dir() {
  local dir="${REPORT_DIR_INPUT:-${K3S_REPORT_DIR:-/var/log/opsctl/k3s}}"
  if mkdir -p "$dir" 2>/dev/null && [[ -w "$dir" ]]; then
    printf '%s\n' "$dir"
    return
  fi
  dir="${TMPDIR:-/tmp}/opsctl-k3s"
  mkdir -p "$dir" 2>/dev/null || {
    echo "错误: 无法创建报告目录 $dir" >&2
    exit 1
  }
  printf '%s\n' "$dir"
}

REPORT_DIR="$(resolve_report_dir)"
TIMESTAMP="$(date '+%Y%m%d-%H%M%S')"
HOSTNAME_VALUE="$(hostname 2>/dev/null || uname -n)"
REPORT="$REPORT_DIR/k3s-diagnose-${HOSTNAME_VALUE}-${TIMESTAMP}.txt"

if [[ "$REPORT_DIR" != "${REPORT_DIR_INPUT:-${K3S_REPORT_DIR:-/var/log/opsctl/k3s}}" ]]; then
  echo "提示: 目标报告目录不可写，已改用回退目录 $REPORT_DIR"
fi

{
  echo "k3s 诊断报告"
  echo "生成时间: $(date '+%Y-%m-%d %H:%M:%S')"
  echo "主机: $HOSTNAME_VALUE"
  echo "系统: $(uname -a)"
} >"$REPORT"

append_section() {
  local title="$1"
  shift
  {
    echo
    echo "##### $title"
  } >>"$REPORT"
  if ! "$@" >>"$REPORT" 2>&1; then
    echo "(命令执行失败: $*)" >>"$REPORT"
  fi
}

append_text() {
  local title="$1"
  local content="$2"
  {
    echo
    echo "##### $title"
    printf '%s\n' "$content"
  } >>"$REPORT"
}

VERSION=""
if has_command k3s; then
  VERSION="$(k3s --version 2>/dev/null | head -n1)"
fi
append_text "k3s 版本" "${VERSION:-未知}"

if has_command systemctl; then
  for svc in k3s k3s-agent; do
    if service_exists "$svc"; then
      append_section "服务 $svc 状态" systemctl status "$svc" --no-pager -l
      append_section "服务 $svc 最近日志(200 行)" journalctl -u "$svc" -n 200 --no-pager
    fi
  done
fi

append_section "节点概览" kube get nodes -o wide
append_section "全部 Pod(wide)" kube get pods -A -o wide
append_section "最近事件(60 条)" bash -o pipefail -c "k3s kubectl get events -A --sort-by=.lastTimestamp 2>/dev/null | tail -n 60 || kubectl get events -A --sort-by=.lastTimestamp 2>/dev/null | tail -n 60"
append_section "API 就绪检查" bash -c "k3s kubectl get --raw=/readyz 2>/dev/null || kubectl get --raw=/readyz 2>/dev/null"

if has_command df; then
  append_section "磁盘使用" df -h
fi
if has_command free; then
  append_section "内存使用(MB)" free -m
fi
if [[ -r /proc/loadavg ]]; then
  append_text "负载" "$(cat /proc/loadavg)"
fi

if has_command k3s; then
  append_section "etcd 快照列表" k3s etcd-snapshot ls
fi

HELM_PATH="$(helm_bin 2>/dev/null || true)"
if [[ -n "$HELM_PATH" ]]; then
  append_section "Helm 版本" "$HELM_PATH" version --short
  append_section "Helm 发布列表(全部命名空间)" "$HELM_PATH" list -A
fi

if has_command ls && [[ -d /var/lib/rancher/k3s ]]; then
  append_section "k3s 数据目录占用" du -sh /var/lib/rancher/k3s
fi

echo "已生成诊断报告: $REPORT"
echo "输出文件: $REPORT"
printf '{"report_dir":"%s","report_path":"%s"}\n' "$(json_escape "$REPORT_DIR")" "$(json_escape "$REPORT")"

exit 0

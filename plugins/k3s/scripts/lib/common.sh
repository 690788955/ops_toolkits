#!/usr/bin/env bash
# k3s 维护插件公共函数库，由各脚本 source 引入。

json_escape() {
  local raw="$1"
  raw="${raw//\\/\\\\}"
  raw="${raw//\"/\\\"}"
  raw="${raw//$'\n'/\\n}"
  raw="${raw//$'\r'/\\r}"
  raw="${raw//$'\t'/\\t}"
  printf '%s' "$raw"
}

has_command() {
  command -v "$1" >/dev/null 2>&1
}

kube_available() {
  has_command k3s || has_command kubectl
}

require_kube() {
  if ! kube_available; then
    echo "错误: 未找到 k3s 或 kubectl 命令，请在 k3s 节点上执行，或通过 kubeconfig 参数指定配置文件" >&2
    exit 1
  fi
}

kube() {
  if has_command k3s; then
    k3s kubectl "$@"
  else
    kubectl "$@"
  fi
}

setup_kubeconfig() {
  local override="${1:-}"
  if [[ -n "$override" ]]; then
    export KUBECONFIG="$override"
    return
  fi
  if [[ -n "${KUBECONFIG:-}" ]]; then
    return
  fi
  if [[ -r /etc/rancher/k3s/k3s.yaml ]]; then
    export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
  fi
}

service_exists() {
  if ! has_command systemctl; then
    return 1
  fi
  systemctl list-unit-files 2>/dev/null | awk 'NR>1 {print $1}' | grep -qx "$1.service"
}

pick_k3s_service() {
  local requested="${1:-auto}"
  if [[ "$requested" != "auto" ]]; then
    if ! service_exists "$requested"; then
      echo "错误: 服务 $requested 不存在（当前系统可能未使用 systemd 或未安装 k3s）" >&2
      exit 1
    fi
    printf '%s\n' "$requested"
    return
  fi
  if service_exists k3s; then
    printf 'k3s\n'
    return
  fi
  if service_exists k3s-agent; then
    printf 'k3s-agent\n'
    return
  fi
  echo "错误: 未找到 k3s 或 k3s-agent 服务" >&2
  exit 1
}

helm_bin() {
  if [[ -n "${HELM_BIN:-}" && -x "${HELM_BIN}" ]]; then
    printf '%s\n' "$HELM_BIN"
    return
  fi
  if has_command helm; then
    command -v helm
    return
  fi
  local candidate
  for candidate in /usr/local/bin/helm /usr/bin/helm "${HOME:-/root}/.local/bin/helm" /snap/bin/helm; do
    if [[ -x "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return
    fi
  done
  echo "错误: 未找到 helm 命令，请先在目标节点安装 Helm 3（https://helm.sh/docs/intro/install/）" >&2
  return 1
}

# 把 30m/2h/1d 这类相对时间转换为 journalctl --since 可识别的格式，其他输入原样返回。
normalize_since() {
  local value="$1"
  if [[ "$value" =~ ^[0-9]+[smhd]$ ]]; then
    local number="${value%?}"
    local unit="${value: -1}"
    case "$unit" in
      s) printf '%s seconds ago\n' "$number" ;;
      m) printf '%s minutes ago\n' "$number" ;;
      h) printf '%s hours ago\n' "$number" ;;
      d) printf '%s days ago\n' "$number" ;;
    esac
    return
  fi
  printf '%s\n' "$value"
}
#!/usr/bin/env bash
# PowerShell 不可用时（git-bash/MSYS/WSL）通过 sc.exe 操作 Windows 服务。
# 参数与服务名由 service.ps1 校验后通过环境变量传入，本脚本不直接对外暴露。

set -euo pipefail

NAME="${SC_SERVICE_NAME:-}"
ACTION="${SC_SERVICE_ACTION:-status}"
CHECK_EXISTS="${SC_CHECK_EXISTS:-0}"

json_escape() {
  local raw="$1"
  raw="${raw//\\/\\\\}"
  raw="${raw//\"/\\\"}"
  raw="${raw//$'\n'/\\n}"
  raw="${raw//$'\r'/\\r}"
  printf '%s' "$raw"
}

emit_error() {
  local message="$1"
  printf '{"service":"%s","action":"%s","status":"error","error":"%s"}\n' \
    "$(json_escape "$NAME")" \
    "$(json_escape "$ACTION")" \
    "$(json_escape "$message")"
  echo "错误: $message" >&2
  exit 1
}

if [[ -z "$NAME" ]]; then
  emit_error "--service 必填"
fi

SC_EXE="sc.exe"
if ! command -v "$SC_EXE" >/dev/null 2>&1; then
  SC_EXE="/c/Windows/System32/sc.exe"
fi
if ! command -v "$SC_EXE" >/dev/null 2>&1; then
  emit_error "未找到 sc.exe，确认当前目标为 Windows 管理机"
fi

run_sc() {
  local output status
  set +e
  output="$("$SC_EXE" "$@" 2>&1 | tr -d '\r')"
  status=$?
  set -e
  SC_OUTPUT="$output"
  return "$status"
}

if [[ "$CHECK_EXISTS" == "1" ]]; then
  run_sc query "$NAME" || true
  if printf '%s' "$SC_OUTPUT" | grep -q "1060"; then
    emit_error "服务不存在: $NAME"
  fi
fi

if [[ "$ACTION" == "restart" ]]; then
  run_sc stop "$NAME" || true
  if ! run_sc start "$NAME"; then
    emit_error "重启服务失败: $NAME"
  fi
  echo "服务已重启: $NAME"
  printf '{"service":"%s","action":"restart","status":"ok","manager":"sc"}\n' "$(json_escape "$NAME")"
  exit 0
fi

if ! run_sc query "$NAME"; then
  emit_error "查询服务状态失败: $NAME"
fi

if printf '%s' "$SC_OUTPUT" | grep -qi "STOPPED"; then
  emit_error "服务未运行: $NAME"
fi
if ! printf '%s' "$SC_OUTPUT" | grep -q "RUNNING"; then
  emit_error "服务状态异常: $NAME"
fi

echo "服务运行正常: $NAME"
printf '{"service":"%s","action":"status","status":"active","manager":"sc"}\n' "$(json_escape "$NAME")"

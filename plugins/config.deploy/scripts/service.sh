#!/usr/bin/env bash
set -euo pipefail

# 重启或检查关联服务，供工作流恢复动作使用。
# 用法: service.sh --service <服务名> --action restart|status [--manager auto|systemctl|service|rc-service]
# Windows 目标（git-bash/MSYS/WSL 或 PowerShell）自动改由 service.ps1 / sc.exe 处理。
# 支持 openrc 服务目录覆盖，例如 SERVICE_DIR=/etc/init.d（auto 模式按 systemctl、service、rc-service 顺序探测）。

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_NAME=""
ACTION="status"
MANAGER="auto"

usage() {
  cat <<'EOF'
用法: service.sh --service <服务名> [选项]

选项:
  --action restart|status  重启服务或检查服务状态（默认 status）
  --manager auto|systemctl|service|rc-service  服务管理方式（默认 auto）
EOF
}

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
    "$(json_escape "$SERVICE_NAME")" \
    "$(json_escape "$ACTION")" \
    "$(json_escape "$message")"
  echo "错误: $message" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --service | --action | --manager)
      if [[ $# -lt 2 ]]; then
        echo "错误: 参数 $1 缺少取值" >&2
        exit 1
      fi
      case "$1" in
        --service) SERVICE_NAME="$2" ;;
        --action) ACTION="$2" ;;
        --manager) MANAGER="$2" ;;
      esac
      shift 2
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "错误: 未知参数 $1" >&2
      exit 1
      ;;
  esac
done

if [[ "$ACTION" != "restart" && "$ACTION" != "status" ]]; then
  emit_error "--action 仅支持 restart 或 status，当前值: $ACTION"
fi
case "$MANAGER" in
  auto | systemctl | service | rc-service | sc) ;;
  *) emit_error "--manager 仅支持 auto、systemctl、service、rc-service，Windows 目标额外支持 sc，当前值: $MANAGER" ;;
esac

SERVICE_NAME="${SERVICE_NAME// /}"
if [[ -z "$SERVICE_NAME" ]]; then
  emit_error "--service 必填"
fi
if [[ "$SERVICE_NAME" == *[\*\?]* ]]; then
  emit_error "服务名不合法: $SERVICE_NAME"
fi

is_windows_target() {
  case "$(uname -s 2>/dev/null || true)" in
    MINGW* | MSYS* | CYGWIN*) return 0 ;;
  esac
  [[ -n "${SystemRoot:-}" || -n "${WINDIR:-}" ]]
}

find_powershell() {
  local candidate
  for candidate in pwsh.exe powershell.exe; do
    if command -v "$candidate" >/dev/null 2>&1; then
      command -v "$candidate"
      return 0
    fi
  done
  return 1
}

if is_windows_target; then
  if [[ "$MANAGER" != "auto" && "$MANAGER" != "sc" ]]; then
    emit_error "Windows 目标仅支持 --manager auto 或 sc，当前值: $MANAGER"
  fi
  if powershell=$(find_powershell); then
    SCRIPT_PATH="$SCRIPT_DIR/service/service.ps1"
    if command -v cygpath >/dev/null 2>&1; then
      SCRIPT_PATH="$(cygpath -w "$SCRIPT_PATH")"
    fi
    exec "$powershell" -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass \
      -File "$SCRIPT_PATH" -Service "$SERVICE_NAME" -Action "$ACTION" -Manager sc
  fi
  # WSL 等环境无法直接调用 pwsh.exe 时退回到 sc.exe。
  SC_CHECK_EXISTS=1
  if [[ -n "${SystemRoot:-}" || -n "${WINDIR:-}" ]]; then
    SC_CHECK_EXISTS=0
  fi
  export SC_CHECK_EXISTS SC_SERVICE_NAME="$SERVICE_NAME" SC_SERVICE_ACTION="$ACTION"
  exec bash "$SCRIPT_DIR/service/sc-service.sh"
fi

if [[ "$MANAGER" == "sc" ]]; then
  emit_error "--manager sc 仅用于 Windows 目标"
fi

warn_from_output() {
  local lower
  lower="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  if [[ "$lower" == *"could not be found"* || "$lower" == *"not found"* || "$lower" == *"no such file"* ]]; then
    emit_error "$1"
  fi
}

run_manager() {
  local output status
  set +e
  output="$("$@" 2>&1)"
  status=$?
  set -e
  warn_from_output "$output"
  if [[ $status -ne 0 ]]; then
    emit_error "${output:-$1 $2 执行失败（退出码 $status）}"
  fi
  printf '%s\n' "$output"
}

manager_available() {
  local name="$1" output status
  set +e
  output="$("$name" --version 2>&1)"
  status=$?
  set -e
  warn_from_output "$output"
  return "$status"
}

resolve_manager() {
  local candidate
  if [[ "$MANAGER" != "auto" ]]; then
    if ! manager_available "$MANAGER"; then
      emit_error "未找到服务管理命令: $MANAGER"
    fi
    printf '%s' "$MANAGER"
    return 0
  fi
  for candidate in systemctl service rc-service; do
    if manager_available "$candidate"; then
      printf '%s' "$candidate"
      return 0
    fi
  done
  return 1
}

if ! MANAGER="$(resolve_manager)"; then
  emit_error "未找到可用的服务管理命令（systemctl / service / rc-service）"
fi

if [[ "$ACTION" == "restart" ]]; then
  run_manager "$MANAGER" restart "$SERVICE_NAME"
  echo "服务已重启: $SERVICE_NAME"
  printf '{"service":"%s","action":"restart","status":"ok","manager":"%s"}\n' \
    "$(json_escape "$SERVICE_NAME")" "$MANAGER"
  exit 0
fi

state="$(run_manager "$MANAGER" is-active "$SERVICE_NAME")"
if [[ "$state" != "active" ]]; then
  emit_error "服务未运行: $SERVICE_NAME"
fi
echo "服务运行正常: $SERVICE_NAME"
printf '{"service":"%s","action":"status","status":"active","manager":"%s"}\n' \
  "$(json_escape "$SERVICE_NAME")" "$MANAGER"

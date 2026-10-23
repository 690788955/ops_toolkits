#!/usr/bin/env bash
set -euo pipefail

# 按运行记录中的备份元数据恢复目标文件。
# --action check    只校验目标文件和备份是否仍可用于回滚
# --action rollback 原子恢复备份，并在恢复前保留当前文件作为安全备份
# 用法: rollback-config.sh --action check|rollback --target <绝对路径> --backup <备份路径>
#                          [--expected-sha256 <下发后的目标 SHA256>] [--force yes|no]

ACTION="check"
TARGET_INPUT=""
BACKUP_INPUT=""
EXPECTED_SHA=""
FORCE="no"

TMP_FILE=""

usage() {
  cat <<'EOF'
用法: rollback-config.sh --target <目标绝对路径> --backup <备份绝对路径> [选项]

选项:
  --action check|rollback  只校验或执行恢复（默认 check）
  --expected-sha256 <sha>  期望的目标文件 SHA256；不一致说明文件被外部修改，默认拒绝回滚
  --force yes|no           目标被外部修改时仍执行回滚（会先保存当前文件，默认 no）
EOF
}

cleanup() {
  if [[ -n "$TMP_FILE" && -f "$TMP_FILE" ]]; then
    rm -f -- "$TMP_FILE" || true
  fi
}
trap cleanup EXIT

json_escape() {
  local raw="$1"
  raw="${raw//\\/\\\\}"
  raw="${raw//\"/\\\"}"
  raw="${raw//$'\n'/\\n}"
  raw="${raw//$'\r'/\\r}"
  printf '%s' "$raw"
}

normalize_bool() {
  case "${1,,}" in
    yes | y | true | 1) printf 'yes' ;;
    no | n | false | 0) printf 'no' ;;
    *) printf '' ;;
  esac
}

normalize_path() {
  local raw="$1"
  raw="${raw//\\:/:}"
  raw="${raw//\\//}"

  if [[ "$raw" =~ ^([A-Za-z]):(/.*)?$ ]]; then
    if command -v cygpath >/dev/null 2>&1; then
      cygpath -u "$raw"
      return
    fi
    if command -v wslpath >/dev/null 2>&1; then
      wslpath -u "$raw"
      return
    fi
    printf '/mnt/%s%s\n' "${BASH_REMATCH[1],,}" "${BASH_REMATCH[2]:-}"
    return
  fi

  printf '%s\n' "$raw"
}

sha256_of() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$file" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 -- "$file" | awk '{print $1}'
    return
  fi
  if command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 -- "$file" | awk '{print $NF}'
    return
  fi
  printf ''
}

emit_error() {
  local message="$1"
  printf '{"action":"%s","status":"error","target_path":"%s","backup_path":"%s","error":"%s"}\n' \
    "$(json_escape "$ACTION")" \
    "$(json_escape "$TARGET_INPUT")" \
    "$(json_escape "$BACKUP_INPUT")" \
    "$(json_escape "$message")"
  echo "错误: $message" >&2
  exit 1
}

emit_result() {
  local status="$1" sha="$2" safety_backup="$3"
  printf '{"action":"%s","status":"%s","target_path":"%s","backup_path":"%s","sha256":"%s","safety_backup":"%s"}\n' \
    "$(json_escape "$ACTION")" \
    "$(json_escape "$status")" \
    "$(json_escape "$TARGET_PATH")" \
    "$(json_escape "$BACKUP_PATH")" \
    "$(json_escape "$sha")" \
    "$(json_escape "$safety_backup")"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --action | --target | --backup | --expected-sha256 | --force)
      if [[ $# -lt 2 ]]; then
        echo "错误: 参数 $1 缺少取值" >&2
        exit 1
      fi
      case "$1" in
        --action) ACTION="$2" ;;
        --target) TARGET_INPUT="$2" ;;
        --backup) BACKUP_INPUT="$2" ;;
        --expected-sha256) EXPECTED_SHA="$2" ;;
        --force) FORCE="$2" ;;
      esac
      shift 2
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "错误: 未知参数 $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

ACTION="${ACTION,,}"
case "$ACTION" in
  check | rollback) ;;
  *)
    echo "错误: --action 仅支持 check 或 rollback，当前值: $ACTION" >&2
    exit 1
    ;;
esac

FORCE="$(normalize_bool "$FORCE")"
if [[ -z "$FORCE" ]]; then
  echo "错误: --force 仅支持 yes 或 no" >&2
  exit 1
fi

if [[ -z "$TARGET_INPUT" || -z "$BACKUP_INPUT" ]]; then
  echo "错误: --target 和 --backup 必填" >&2
  usage >&2
  exit 1
fi

TARGET_PATH="$(normalize_path "$TARGET_INPUT")"
BACKUP_PATH="$(normalize_path "$BACKUP_INPUT")"
for item in "$TARGET_PATH" "$BACKUP_PATH"; do
  case "$item" in
    /*) ;;
    *) emit_error "路径必须是绝对路径: $item" ;;
  esac
done

if [[ ! -f "$BACKUP_PATH" ]]; then
  emit_error "备份文件不存在: $BACKUP_PATH"
fi

if [[ -L "$TARGET_PATH" ]]; then
  link_resolved="$(readlink -f "$TARGET_PATH" 2>/dev/null || true)"
  if [[ -n "$link_resolved" ]]; then
    TARGET_PATH="$link_resolved"
  fi
fi

TARGET_EXISTS="no"
CURRENT_SHA=""
if [[ -e "$TARGET_PATH" ]]; then
  if [[ ! -f "$TARGET_PATH" ]]; then
    emit_error "目标路径已存在且不是普通文件: $TARGET_PATH"
  fi
  TARGET_EXISTS="yes"
  CURRENT_SHA="$(sha256_of "$TARGET_PATH")"
fi

if [[ -n "$EXPECTED_SHA" ]]; then
  if [[ "$TARGET_EXISTS" != "yes" ]]; then
    emit_error "目标文件已不存在，无法确认当前状态，默认拒绝回滚: $TARGET_PATH"
  fi
  if [[ "$CURRENT_SHA" != "$EXPECTED_SHA" && "$FORCE" != "yes" ]]; then
    emit_error "目标文件已被外部修改（当前 $CURRENT_SHA，期望 $EXPECTED_SHA），默认拒绝回滚；确认覆盖请传 --force yes"
  fi
elif [[ "$FORCE" != "yes" ]]; then
  emit_error "缺少下发后的目标 SHA256，无法确认是否发生外部修改，默认拒绝回滚"
fi

BACKUP_SHA="$(sha256_of "$BACKUP_PATH")"
if [[ -z "$BACKUP_SHA" ]]; then
  emit_error "无法计算备份文件 SHA256: $BACKUP_PATH"
fi

echo "========================================="
echo "配置回滚"
echo "========================================="
echo "动作: $ACTION"
echo "目标: $TARGET_PATH"
echo "备份: $BACKUP_PATH"
echo "备份 SHA256: $BACKUP_SHA"
echo "当前 SHA256: ${CURRENT_SHA:-不存在}"
echo "========================================="

if [[ "$ACTION" == "check" ]]; then
  emit_result "ready" "$BACKUP_SHA" ""
  exit 0
fi

PARENT_DIR="$(dirname "$TARGET_PATH")"
if [[ ! -d "$PARENT_DIR" ]]; then
  emit_error "目标目录不存在: $PARENT_DIR"
fi

SAFETY_BACKUP=""
if [[ "$TARGET_EXISTS" == "yes" ]]; then
  SAFETY_BACKUP="$TARGET_PATH.before-rollback.$(date '+%Y%m%d%H%M%S')"
  if ! cp -p -- "$TARGET_PATH" "$SAFETY_BACKUP"; then
    emit_error "保存回滚前文件失败: $SAFETY_BACKUP"
  fi
  echo "已保留回滚前文件: $SAFETY_BACKUP"
fi

if [[ "$TARGET_EXISTS" == "yes" ]] && [[ "$TARGET_PATH" -ef "$BACKUP_PATH" ]]; then
  emit_error "目标文件与备份文件是同一个文件: $TARGET_PATH"
fi

TMP_FILE="$PARENT_DIR/.$(basename "$TARGET_PATH").rollback.$$"
if ! cp -p -- "$BACKUP_PATH" "$TMP_FILE"; then
  emit_error "写入回滚临时文件失败: $TMP_FILE"
fi
if ! mv -f -- "$TMP_FILE" "$TARGET_PATH"; then
  emit_error "替换目标文件失败: $TARGET_PATH"
fi
TMP_FILE=""

RESTORED_SHA="$(sha256_of "$TARGET_PATH")"
if [[ "$RESTORED_SHA" != "$BACKUP_SHA" ]]; then
  emit_error "回滚后校验失败：目标 SHA256 $RESTORED_SHA 与备份 $BACKUP_SHA 不一致"
fi

echo "回滚完成，目标 SHA256: $RESTORED_SHA"
emit_result "rolled_back" "$RESTORED_SHA" "$SAFETY_BACKUP"

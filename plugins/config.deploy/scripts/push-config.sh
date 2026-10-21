#!/usr/bin/env bash
set -euo pipefail

# 将插件 config 目录中的配置文件内容写入目标路径。
# --action push  执行写入（默认先备份、原子替换、写入后校验）
# --action check 仅比对差异，不落盘
# 用法: push-config.sh --action push --target <绝对路径> [--source app.conf]
#                      [--create-dirs yes|no] [--backup yes|no] [--if-changed yes|no]
#                      [--mode 0644] [--owner user:group]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG_DIR="$PLUGIN_DIR/config"

ACTION="push"
TARGET_INPUT=""
SOURCE_INPUT="app.conf"
CREATE_DIRS="yes"
BACKUP="yes"
IF_CHANGED="yes"
MODE=""
OWNER=""

TMP_FILE=""

usage() {
  cat <<'EOF'
用法: push-config.sh --target <目标绝对路径> [选项]

选项:
  --action push|check     执行写入或仅比对差异（默认 push）
  --source <文件名>       插件 config 目录内的源配置文件（默认 app.conf）
  --create-dirs yes|no    目标父目录不存在时是否创建（默认 yes）
  --backup yes|no         目标存在时是否先备份（默认 yes）
  --if-changed yes|no     内容一致时是否跳过写入（默认 yes）
  --mode <0644>           写入后权限位，留空沿用原权限
  --owner <user:group>    写入后属主，留空不修改
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

# Windows 盘符路径在 Git Bash / WSL 下转换为可用路径
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

file_mode() {
  local file="$1"
  stat -c '%a' "$file" 2>/dev/null || stat -f '%OLp' "$file" 2>/dev/null || printf ''
}

emit_result() {
  local status="$1" changed="$2" backup_path="$3" sha="$4" bytes="$5"
  printf '{"action":"%s","status":"%s","changed":%s,"source_file":"%s","target_path":"%s","backup_path":"%s","sha256":"%s","bytes":%s}\n' \
    "$(json_escape "$ACTION")" \
    "$(json_escape "$status")" \
    "$changed" \
    "$(json_escape "$SOURCE_INPUT")" \
    "$(json_escape "$TARGET_PATH")" \
    "$(json_escape "$backup_path")" \
    "$(json_escape "$sha")" \
    "${bytes:-0}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --action | --target | --source | --create-dirs | --backup | --if-changed | --mode | --owner)
      if [[ $# -lt 2 ]]; then
        echo "错误: 参数 $1 缺少取值" >&2
        exit 1
      fi
      case "$1" in
        --action) ACTION="$2" ;;
        --target) TARGET_INPUT="$2" ;;
        --source) SOURCE_INPUT="$2" ;;
        --create-dirs) CREATE_DIRS="$2" ;;
        --backup) BACKUP="$2" ;;
        --if-changed) IF_CHANGED="$2" ;;
        --mode) MODE="$2" ;;
        --owner) OWNER="$2" ;;
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
  push | check) ;;
  *)
    echo "错误: --action 仅支持 push 或 check，当前值: $ACTION" >&2
    exit 1
    ;;
esac

for item in CREATE_DIRS BACKUP IF_CHANGED; do
  value="$(normalize_bool "${!item}")"
  if [[ -z "$value" ]]; then
    echo "错误: --$(printf '%s' "$item" | tr '[:upper:]_' '[:lower:]-') 仅支持 yes 或 no，当前值: ${!item}" >&2
    exit 1
  fi
  printf -v "$item" '%s' "$value"
done

if [[ -z "$TARGET_INPUT" ]]; then
  echo "错误: --target 必填，请传入目标文件路径" >&2
  usage >&2
  exit 1
fi

# ---------- 解析源配置文件（限制在插件 config 目录内） ----------
SOURCE_INPUT="${SOURCE_INPUT//\\//}"
case "$SOURCE_INPUT" in
  "" | /* | [A-Za-z]:*)
    echo "错误: --source 必须是插件 config 目录内的相对路径: $SOURCE_INPUT" >&2
    exit 1
    ;;
esac

SOURCE_CANDIDATE="$CONFIG_DIR/$SOURCE_INPUT"
if [[ ! -f "$SOURCE_CANDIDATE" ]]; then
  echo "错误: 源配置文件不存在: $SOURCE_CANDIDATE" >&2
  echo "提示: 可用文件:" >&2
  find "$CONFIG_DIR" -maxdepth 2 -type f -printf '  %P\n' 2>/dev/null || ls -1 "$CONFIG_DIR" >&2
  exit 1
fi

REAL_CONFIG_DIR="$(cd "$CONFIG_DIR" && pwd -P)"
SOURCE_DIR_REAL="$(cd "$(dirname "$SOURCE_CANDIDATE")" && pwd -P)"
SRC_PATH="$SOURCE_DIR_REAL/$(basename "$SOURCE_CANDIDATE")"
case "$SRC_PATH" in
  "$REAL_CONFIG_DIR"/*) ;;
  *)
    echo "错误: 源配置文件解析后不在 config 目录内: $SRC_PATH" >&2
    exit 1
    ;;
esac

# ---------- 解析目标路径 ----------
TARGET_PATH="$(normalize_path "$TARGET_INPUT")"
case "$TARGET_PATH" in
  /*) ;;
  *)
    echo "错误: 目标路径必须是绝对路径: $TARGET_INPUT" >&2
    echo "提示: Windows 可传 D:/etc/app.conf 或 D:\\etc\\app.conf；Linux 可传 /etc/app.conf" >&2
    exit 1
    ;;
esac

if [[ "$TARGET_PATH" == */ ]] || [[ -d "$TARGET_PATH" ]]; then
  TARGET_PATH="${TARGET_PATH%/}/$(basename "$SRC_PATH")"
fi

if [[ "$TARGET_PATH" == "/" || -z "$(basename "$TARGET_PATH")" ]]; then
  echo "错误: 目标路径不合法: $TARGET_INPUT" >&2
  exit 1
fi

LINK_DIRECT_WRITE="no"
if [[ -L "$TARGET_PATH" ]]; then
  link_resolved="$(readlink -f "$TARGET_PATH" 2>/dev/null || true)"
  if [[ -n "$link_resolved" ]]; then
    echo "提示: 目标为符号链接，实际写入: $link_resolved"
    TARGET_PATH="$link_resolved"
  else
    echo "提示: 目标为符号链接且无法解析，将直接写入链接指向的文件"
    LINK_DIRECT_WRITE="yes"
  fi
fi

PARENT_DIR="$(dirname "$TARGET_PATH")"

TARGET_EXISTS="no"
TARGET_IS_FILE="no"
if [[ -e "$TARGET_PATH" ]]; then
  TARGET_EXISTS="yes"
  if [[ ! -f "$TARGET_PATH" ]]; then
    echo "错误: 目标路径已存在且不是普通文件: $TARGET_PATH" >&2
    exit 1
  fi
  TARGET_IS_FILE="yes"
  if [[ "$TARGET_PATH" -ef "$SRC_PATH" ]]; then
    echo "错误: 目标文件与源配置文件是同一个文件: $TARGET_PATH" >&2
    exit 1
  fi
fi

echo "========================================="
echo "配置文件部署"
echo "========================================="
echo "动作: $ACTION"
echo "源配置: $SRC_PATH"
echo "目标: $TARGET_PATH"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================="
echo ""

# ---------- 比对 ----------
SRC_SHA="$(sha256_of "$SRC_PATH")"
SRC_BYTES="$(wc -c <"$SRC_PATH" | tr -d ' ')"
TARGET_SHA=""
if [[ "$TARGET_IS_FILE" == "yes" ]]; then
  TARGET_SHA="$(sha256_of "$TARGET_PATH")"
fi

if [[ -n "$SRC_SHA" && "$SRC_SHA" == "$TARGET_SHA" ]]; then
  CONTENT_STATUS="identical"
elif [[ "$TARGET_IS_FILE" == "yes" ]]; then
  CONTENT_STATUS="different"
else
  CONTENT_STATUS="missing"
fi

if [[ "$ACTION" == "check" ]]; then
  if [[ "$CONTENT_STATUS" == "identical" ]]; then
    echo "内容一致，无需更新。"
    emit_result "identical" "false" "" "$TARGET_SHA" "$SRC_BYTES"
    exit 0
  fi

  if [[ "$CONTENT_STATUS" == "missing" ]]; then
    echo "目标文件不存在: $TARGET_PATH"
    echo "执行 push 将新建该文件，大小 $SRC_BYTES bytes"
    emit_result "missing" "true" "" "$SRC_SHA" "$SRC_BYTES"
    exit 0
  fi

  echo "检测到内容差异（--- 目标文件 / +++ 源配置）:"
  echo "-----------------------------------------"
  diff -u --label "$TARGET_PATH" --label "$SRC_PATH" "$TARGET_PATH" "$SRC_PATH" 2>/dev/null | head -n 200 || true
  echo "-----------------------------------------"
  echo "执行 push 将覆盖目标文件。"
  emit_result "different" "true" "" "$SRC_SHA" "$SRC_BYTES"
  exit 0
fi

# ---------- push ----------
if [[ "$CONTENT_STATUS" == "identical" && "$IF_CHANGED" == "yes" ]]; then
  echo "内容一致，跳过写入。"
  emit_result "skipped" "false" "" "$TARGET_SHA" "$SRC_BYTES"
  exit 0
fi

if [[ ! -d "$PARENT_DIR" ]]; then
  if [[ "$CREATE_DIRS" != "yes" ]]; then
    echo "错误: 目标目录不存在且未允许自动创建: $PARENT_DIR" >&2
    exit 1
  fi
  if ! mkdir -p -- "$PARENT_DIR"; then
    echo "错误: 创建父目录失败: $PARENT_DIR" >&2
    exit 1
  fi
  echo "已创建父目录: $PARENT_DIR"
fi

BACKUP_PATH=""
if [[ "$TARGET_IS_FILE" == "yes" && "$BACKUP" == "yes" ]]; then
  BACKUP_PATH="$TARGET_PATH.bak.$(date '+%Y%m%d%H%M%S')"
  if ! cp -p -- "$TARGET_PATH" "$BACKUP_PATH"; then
    echo "错误: 备份目标文件失败: $BACKUP_PATH" >&2
    exit 1
  fi
  echo "已备份原文件: $BACKUP_PATH"
fi

if [[ "$LINK_DIRECT_WRITE" == "yes" ]]; then
  if ! cat -- "$SRC_PATH" >"$TARGET_PATH"; then
    echo "错误: 写入目标文件失败: $TARGET_PATH" >&2
    exit 1
  fi
else
  TMP_FILE="$PARENT_DIR/.$(basename "$TARGET_PATH").tmp.$$"
  if ! cp -- "$SRC_PATH" "$TMP_FILE"; then
    echo "错误: 写入临时文件失败: $TMP_FILE" >&2
    exit 1
  fi

  if [[ -n "$MODE" ]]; then
    if ! chmod "$MODE" "$TMP_FILE" 2>/dev/null; then
      echo "错误: 设置权限失败: $MODE" >&2
      exit 1
    fi
    case "$(uname -s 2>/dev/null || printf 'unknown')" in
      MINGW* | MSYS* | CYGWIN*)
        echo "提示: 当前为 Windows 兼容环境，POSIX 权限位不生效，请在目标 Linux 主机执行以应用 mode"
        ;;
    esac
  elif [[ "$TARGET_IS_FILE" == "yes" ]]; then
    ref_mode="$(file_mode "$TARGET_PATH")"
    if [[ -n "$ref_mode" ]]; then
      chmod "$ref_mode" "$TMP_FILE" 2>/dev/null || true
    fi
  else
    chmod 0644 "$TMP_FILE" 2>/dev/null || true
  fi

  if [[ -n "$OWNER" ]]; then
    if ! chown "$OWNER" "$TMP_FILE" 2>/dev/null; then
      echo "错误: 设置属主失败（通常需要 root 权限）: $OWNER" >&2
      exit 1
    fi
  fi

  if ! mv -f -- "$TMP_FILE" "$TARGET_PATH"; then
    echo "错误: 替换目标文件失败: $TARGET_PATH" >&2
    exit 1
  fi
  TMP_FILE=""
fi

FINAL_SHA="$(sha256_of "$TARGET_PATH")"
if [[ -n "$SRC_SHA" && -n "$FINAL_SHA" && "$SRC_SHA" != "$FINAL_SHA" ]]; then
  echo "错误: 写入校验失败，目标文件 SHA256 与源配置不一致" >&2
  echo "源: $SRC_SHA" >&2
  echo "目标: $FINAL_SHA" >&2
  exit 1
fi

FINAL_BYTES="$(wc -c <"$TARGET_PATH" | tr -d ' ')"
FINAL_MODE="$(file_mode "$TARGET_PATH")"

if [[ "$TARGET_EXISTS" == "yes" ]]; then
  FINAL_STATUS="updated"
else
  FINAL_STATUS="created"
fi

echo ""
echo "========================================="
echo "写入完成"
echo "目标文件: $TARGET_PATH"
echo "大小: $FINAL_BYTES bytes"
echo "权限: ${FINAL_MODE:-未知}"
echo "SHA256: ${FINAL_SHA:-未知}"
echo "========================================="

emit_result "$FINAL_STATUS" "true" "$BACKUP_PATH" "$FINAL_SHA" "$FINAL_BYTES"

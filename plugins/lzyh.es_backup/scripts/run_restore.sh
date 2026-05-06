#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
CONFIG_FILE="${SCRIPT_DIR}/backup.conf"
DATE=""
RENAME_PATTERN=""
RENAME_TO=""
YES="false"

resolve_plugin_config() {
    local cfg="${1:-${SCRIPT_DIR}/backup.conf}"
    case "$cfg" in
        backup.conf|./backup.conf|scripts/backup.conf|./scripts/backup.conf)
            cfg="${SCRIPT_DIR}/backup.conf"
            ;;
    esac
    if [[ "$cfg" != /* && ! "$cfg" =~ ^[A-Za-z]:[\\/] ]]; then
        cfg="$(pwd)/$cfg"
    fi

    local dir base abs
    dir="$(dirname "$cfg")"
    base="$(basename "$cfg")"
    if [[ ! -d "$dir" ]]; then
        echo "[FATAL] 配置文件目录不存在: $dir" >&2
        exit 1
    fi
    abs="$(cd "$dir" && pwd -P)/$base"

    case "$abs" in
        "$SCRIPT_DIR"/*) ;;
        *)
            echo "[FATAL] 配置文件必须位于插件 scripts 目录内: $cfg" >&2
            exit 1
            ;;
    esac

    if [[ ! -f "$abs" || -L "$abs" ]]; then
        echo "[FATAL] 配置文件必须是插件内普通文件: $abs" >&2
        exit 1
    fi

    printf '%s\n' "$abs"
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --date)
            DATE="${2:-}"
            if [[ -z "$DATE" ]]; then
                echo "[FATAL] --date 缺少参数"
                exit 1
            fi
            shift 2
            ;;
        --config)
            CONFIG_FILE="${2:-}"
            if [[ -z "$CONFIG_FILE" ]]; then
                echo "[FATAL] --config 缺少参数"
                exit 1
            fi
            shift 2
            ;;
        --rename-pattern)
            RENAME_PATTERN="${2:-}"
            shift 2
            ;;
        --rename-to)
            RENAME_TO="${2:-}"
            shift 2
            ;;
        --yes)
            YES="${2:-true}"
            shift 2
            ;;
        *)
            echo "[FATAL] 未识别参数: $1"
            exit 1
            ;;
    esac
done

if [[ -z "$DATE" ]]; then
    echo "[FATAL] --date 必填，格式 YYYY-MM-DD"
    exit 1
fi

CONFIG_FILE="$(resolve_plugin_config "$CONFIG_FILE")"

args=("restore" "$DATE")
if [[ -n "$RENAME_PATTERN" || -n "$RENAME_TO" ]]; then
    if [[ -z "$RENAME_PATTERN" || -z "$RENAME_TO" ]]; then
        echo "[FATAL] --rename-pattern 与 --rename-to 必须同时提供"
        exit 1
    fi
    args+=("--rename-pattern" "$RENAME_PATTERN" "--rename-to" "$RENAME_TO")
fi
if [[ "$YES" == "true" ]]; then
    args+=("--yes")
fi
args+=("$CONFIG_FILE")

exec bash "${SCRIPT_DIR}/es_restore.sh" "${args[@]}"

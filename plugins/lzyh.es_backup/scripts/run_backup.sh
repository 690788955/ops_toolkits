#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
CONFIG_FILE="${SCRIPT_DIR}/backup.conf"
ON_EXIST="skip"
DRY_RUN="false"
YES="true"

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
        --config)
            CONFIG_FILE="${2:-}"
            if [[ -z "$CONFIG_FILE" ]]; then
                echo "[FATAL] --config 缺少参数"
                exit 1
            fi
            shift 2
            ;;
        --on-exist)
            ON_EXIST="${2:-}"
            if [[ -z "$ON_EXIST" ]]; then
                echo "[FATAL] --on-exist 缺少参数"
                exit 1
            fi
            shift 2
            ;;
        --dry-run)
            DRY_RUN="${2:-false}"
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

CONFIG_FILE="$(resolve_plugin_config "$CONFIG_FILE")"

case "$ON_EXIST" in
    ask|skip|overwrite|fail) ;;
    *)
        echo "[FATAL] --on-exist 仅支持 ask|skip|overwrite|fail"
        exit 1
        ;;
esac

args=("--on-exist" "$ON_EXIST")
if [[ "$YES" == "true" ]]; then
    args+=("--yes")
fi
if [[ "$DRY_RUN" == "true" ]]; then
    args+=("--dry-run")
fi
args+=("$CONFIG_FILE")

exec bash "${SCRIPT_DIR}/es_backup.sh" "${args[@]}"

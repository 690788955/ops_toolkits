#!/bin/bash
# ============================================================
# ES Snapshot 恢复脚本 (按日期分目录版)
# 功能: 列出备份日期 → 选择日期 → 恢复快照
# 用法:
#   列出所有备份:       bash es_restore.sh list [配置文件]
#   查看某天备份详情:   bash es_restore.sh info <日期> [配置文件]
#   恢复某天的备份:     bash es_restore.sh restore <日期> [配置文件]
#   恢复并重命名索引:   bash es_restore.sh restore <日期> --rename-pattern <原> --rename-to <新> [配置文件]
# ============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# -------------------- 参数解析 --------------------
ACTION="${1:-help}"
TARGET_DATE=""
RENAME_PATTERN=""
RENAME_REPLACEMENT=""
CONFIG_FILE=""
NON_INTERACTIVE="false"
ASSUME_YES="false"
HAS_TTY="false"

shift || true

while [[ $# -gt 0 ]]; do
    case "$1" in
        --yes|-y|--non-interactive)
            NON_INTERACTIVE="true"
            ASSUME_YES="true"
            shift
            ;;
        --help|-h)
            ACTION="help"
            shift
            ;;
        --rename-pattern)
            if [[ $# -lt 2 ]]; then
                echo "[FATAL] --rename-pattern 缺少参数"
                exit 1
            fi
            RENAME_PATTERN="$2"
            shift 2
            ;;
        --rename-to)
            if [[ $# -lt 2 ]]; then
                echo "[FATAL] --rename-to 缺少参数"
                exit 1
            fi
            RENAME_REPLACEMENT="$2"
            shift 2
            ;;
        *)
            if [[ -z "$TARGET_DATE" && "$ACTION" != "list" ]]; then
                TARGET_DATE="$1"
            else
                CONFIG_FILE="$1"
            fi
            shift
            ;;
    esac
done

CONFIG_FILE="${CONFIG_FILE:-${SCRIPT_DIR}/backup.conf}"

# 加载公共函数库
source "${SCRIPT_DIR}/es_common.sh"

if [[ -t 0 && -t 1 && "$NON_INTERACTIVE" != "true" ]]; then
    HAS_TTY="true"
fi

if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "[FATAL] 配置文件不存在: $CONFIG_FILE"
    exit 1
fi

validate_config_permission "$CONFIG_FILE"

# shellcheck source=backup.conf
source "$CONFIG_FILE"

# -------------------- 初始化公共依赖 --------------------
init_curl_defaults
RESTORE_LOCK_FILE="${RESTORE_LOCK_FILE:-/tmp/es_restore.lock}"
RESTORE_TIMEOUT="${RESTORE_TIMEOUT:-3600s}"
LOCK_HELD="false"

# 加载 ES 凭据
load_es_credentials

# -------------------- 恢复专有函数 --------------------
validate_target_date() {
    local date_val="$1"
    if [[ ! "$date_val" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
        log_error "日期格式无效(${date_val})，必须为 YYYY-MM-DD"
        exit 1
    fi
}

acquire_restore_lock() {
    exec 8>"$RESTORE_LOCK_FILE" || {
        echo "[FATAL] 无法打开恢复锁文件: $RESTORE_LOCK_FILE"
        exit 1
    }
    if ! flock -n 8; then
        log_info "已有恢复任务在运行, 本次退出 (锁文件: $RESTORE_LOCK_FILE)"
        exit 0
    fi
    LOCK_HELD="true"
}

release_restore_lock() {
    if [[ "$LOCK_HELD" == "true" ]]; then
        flock -u 8 || true
        LOCK_HELD="false"
    fi
}

confirm_or_exit() {
    local prompt="$1"

    if [[ "$ASSUME_YES" == "true" ]]; then
        log_info "非交互模式: 自动确认 -> ${prompt}"
        return 0
    fi

    if [[ "$HAS_TTY" != "true" ]]; then
        log_error "检测到非交互环境且未使用 --yes，安全退出"
        exit 1
    fi

    local answer=""
    read -r -p "${prompt} (y/N): " answer
    if [[ "$answer" != "y" && "$answer" != "Y" ]]; then
        log_info "已取消"
        exit 0
    fi
}

# 确保指定日期的仓库已注册 (恢复时可能仓库已被清理)
ensure_repo_registered() {
    local date_tag="$1"
    local repo_name="${REPO_PREFIX}-${date_tag}"
    local backup_dir="${NAS_BASE_PATH}/${date_tag}"

    # 检查仓库是否已注册
    local existing
    existing=$(es_request "GET" "/_snapshot/${repo_name}")
    if echo "$existing" | grep -q "\"${repo_name}\""; then
        return 0
    fi

    # 仓库不存在, 重新注册
    log_info "仓库 [${repo_name}] 未注册, 正在注册..."
    local repo_body="{
        \"type\": \"fs\",
        \"settings\": {
            \"location\": \"${backup_dir}\",
            \"compress\": ${REPO_COMPRESS}
        }
    }"

    local result
    result=$(es_request "PUT" "/_snapshot/${repo_name}" "$repo_body")
    if ! check_response "$result" "仓库注册"; then
        return 1
    fi
    log_info "仓库 [${repo_name}] 注册成功"
    return 0
}

# -------------------- 命令: list --------------------
cmd_list() {
    log_info "扫描备份目录: ${NAS_BASE_PATH}"
    echo ""

    if [[ ! -d "$NAS_BASE_PATH" ]]; then
        log_error "备份基础路径不存在: ${NAS_BASE_PATH}"
        exit 1
    fi

    # 查找所有日期目录
    local dirs
    dirs=$(find "$NAS_BASE_PATH" -maxdepth 1 -mindepth 1 -type d -name "????-??-??" | sort -r)

    if [[ -z "$dirs" ]]; then
        echo "  (无备份)"
        return 0
    fi

    echo "================================================================"
    printf "%-15s %-15s %-12s %s\n" "备份日期" "目录大小" "快照状态" "索引模式"
    echo "================================================================"

    while IFS= read -r dir_path; do
        [[ -z "$dir_path" ]] && continue
        local date_tag
        date_tag=$(basename "$dir_path")
        local repo_name="${REPO_PREFIX}-${date_tag}"

        # 获取目录大小
        local dir_size
        dir_size=$(du -sh "$dir_path" 2>/dev/null | cut -f1 || echo "?")

        # 尝试从 ES 获取快照状态
        local snap_state="未注册"
        local indices_pattern="-"

        local snap_info
        snap_info=$(es_request "GET" "/_snapshot/${repo_name}/${SNAPSHOT_NAME}" 2>/dev/null)

        if ! echo "$snap_info" | grep -q '"error"'; then
            snap_state=$(json_val "$snap_info" "state")
            snap_state=${snap_state:-"未知"}
        fi

        printf "%-15s %-15s %-12s %s\n" "$date_tag" "$dir_size" "$snap_state" "${BACKUP_INDICES}"
    done <<< "$dirs"

    echo "================================================================"
    echo ""
    log_info "提示: 使用 'bash es_restore.sh info <日期>' 查看备份详情"
    log_info "提示: 使用 'bash es_restore.sh restore <日期>' 恢复备份"
    echo ""
    log_info "示例: bash es_restore.sh restore 2026-03-25"
}

# -------------------- 命令: info --------------------
cmd_info() {
    if [[ -z "$TARGET_DATE" ]]; then
        echo "用法: bash es_restore.sh info <日期>"
        echo "示例: bash es_restore.sh info 2026-03-25"
        exit 1
    fi
    validate_target_date "$TARGET_DATE"

    local backup_dir="${NAS_BASE_PATH}/${TARGET_DATE}"
    local repo_name="${REPO_PREFIX}-${TARGET_DATE}"

    if [[ ! -d "$backup_dir" ]]; then
        log_error "备份目录不存在: ${backup_dir}"
        exit 1
    fi

    log_info "查询备份 [${TARGET_DATE}] 详情..."
    echo ""

    # 目录信息
    echo "=== 目录信息 ==="
    echo "  路径: ${backup_dir}"
    echo "  大小: $(du -sh "$backup_dir" 2>/dev/null | cut -f1 || echo '?')"
    echo "  文件数: $(find "$backup_dir" -type f 2>/dev/null | wc -l || echo '?')"
    echo ""

    # 确保仓库已注册
    if ! ensure_repo_registered "$TARGET_DATE"; then
        log_error "无法注册仓库, 无法查看快照详情"
        exit 1
    fi

    # 快照详情
    echo "=== 快照详情 ==="
    local result
    result=$(es_request "GET" "/_snapshot/${repo_name}/${SNAPSHOT_NAME}")

    if ! check_response "$result" "快照查询"; then
        exit 1
    fi

    echo "$result"
}

# -------------------- 命令: restore --------------------
cmd_restore() {
    if [[ -z "$TARGET_DATE" ]]; then
        echo "用法: bash es_restore.sh restore <日期> [--rename-pattern <原> --rename-to <新>]"
        echo "示例: bash es_restore.sh restore 2026-03-25"
        exit 1
    fi
    validate_target_date "$TARGET_DATE"

    local backup_dir="${NAS_BASE_PATH}/${TARGET_DATE}"
    local repo_name="${REPO_PREFIX}-${TARGET_DATE}"

    acquire_restore_lock
    trap 'release_restore_lock' EXIT

    log_info "========== 开始恢复备份 [${TARGET_DATE}] =========="

    # 检查备份目录
    if [[ ! -d "$backup_dir" ]]; then
        log_error "备份目录不存在: ${backup_dir}"
        exit 1
    fi

    # 确保仓库已注册
    if ! ensure_repo_registered "$TARGET_DATE"; then
        log_error "仓库注册失败"
        exit 1
    fi

    # 检查快照状态
    local snap_info
    snap_info=$(es_request "GET" "/_snapshot/${repo_name}/${SNAPSHOT_NAME}")

    if ! check_response "$snap_info" "快照状态查询"; then
        log_error "快照不存在或查询失败"
        echo "$snap_info"
        exit 1
    fi

    local snap_state
    snap_state=$(json_val "$snap_info" "state")
    if [[ "$snap_state" != "SUCCESS" ]]; then
        log_warn "快照状态为 ${snap_state}, 非 SUCCESS 状态可能导致恢复不完整"
        confirm_or_exit "是否继续恢复"
    fi

    # 构造恢复请求
    local restore_body="{"
    restore_body+="\"ignore_unavailable\": true"

    if [[ -n "$RENAME_PATTERN" && -n "$RENAME_REPLACEMENT" ]]; then
        log_info "启用索引重命名: ${RENAME_PATTERN} -> ${RENAME_REPLACEMENT}"
        restore_body+=",\"rename_pattern\": \"${RENAME_PATTERN}\""
        restore_body+=",\"rename_replacement\": \"${RENAME_REPLACEMENT}\""
    fi

    restore_body+="}"

    # 安全确认
    log_info "即将恢复备份 [${TARGET_DATE}] 到集群 ${ES_HOST}:${ES_PORT}"
    if [[ -z "$RENAME_PATTERN" ]]; then
        log_warn "如果目标索引已存在且已打开, 恢复将失败。建议先关闭或删除目标索引。"
    fi
    confirm_or_exit "确认恢复"

    # 执行恢复
    log_info "提交恢复请求..."
    local result
    result=$(es_request "POST" "/_snapshot/${repo_name}/${SNAPSHOT_NAME}/_restore" "$restore_body")

    if ! check_response "$result" "提交恢复请求"; then
        log_error "恢复失败:"
        echo "$result"
        exit 1
    fi

    log_info "恢复请求已提交"

    # 等待恢复完成
    log_info "等待恢复完成..."
    local max_wait=${RESTORE_TIMEOUT%s}
    local waited=0

    while [[ $waited -lt $max_wait ]]; do
        sleep 10
        waited=$((waited + 10))

        local recovery
        recovery=$(es_request "GET" "/_cat/recovery?active_only=true&h=index,stage,files_percent")

        if [[ "$ES_LAST_HTTP_STATUS" -ne 200 ]]; then
            log_warn "恢复状态查询失败(HTTP ${ES_LAST_HTTP_STATUS})，继续重试..."
            continue
        fi

        local related
        related=$(printf '%s\n' "$recovery" | grep -E "^${TARGET_DATE}|${RENAME_REPLACEMENT}|${BACKUP_INDICES%%,*}" || true)

        if [[ -z "$related" ]]; then
            log_info "恢复完成!"
            break
        fi

        local recovering_count
        recovering_count=$(printf '%s\n' "$related" | grep -c "^" || true)
        log_info "进行中... 已等待 ${waited}s, 恢复中分片数: ${recovering_count}"
    done

    if [[ $waited -ge $max_wait ]]; then
        log_error "等待超时, 请手动检查恢复状态: GET /_cat/recovery"
        exit 1
    fi

    log_info "========== 备份恢复完成 =========="
}

# -------------------- 命令: help --------------------
cmd_help() {
    echo ""
    echo "ES Snapshot 恢复工具 (按日期分目录版)"
    echo "======================================="
    echo ""
    echo "用法:"
    echo "  bash es_restore.sh list                          列出所有备份"
    echo "  bash es_restore.sh info <日期>                    查看备份详情"
    echo "  bash es_restore.sh restore <日期>                 恢复备份"
    echo "  bash es_restore.sh restore <日期> \\               恢复并重命名索引"
    echo "      --rename-pattern <原前缀> \\"
    echo "      --rename-to <新前缀>"
    echo ""
    echo "示例:"
    echo "  bash es_restore.sh list"
    echo "  bash es_restore.sh info 2026-03-25"
    echo "  bash es_restore.sh restore 2026-03-25"
    echo "  bash es_restore.sh restore 2026-03-25 --rename-pattern '(.+)' --rename-to 'restored_\$1'"
    echo ""
    echo "指定配置文件 (放在最后):"
    echo "  bash es_restore.sh list /path/to/backup.conf"
    echo ""
}

# -------------------- 路由 --------------------
case "$ACTION" in
    list)    cmd_list    ;;
    info)    cmd_info    ;;
    restore) cmd_restore ;;
    help|*)  cmd_help    ;;
esac

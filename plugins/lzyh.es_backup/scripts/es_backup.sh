#!/bin/bash
# ============================================================
# ES Snapshot 备份脚本 (按日期分目录版)
# 功能: 注册独立仓库 → 创建快照 → 等待完成 → 清理旧备份
# 用法:
#   手动执行:   bash es_backup.sh [配置文件路径]
#   强制覆盖:   bash es_backup.sh --force [配置文件路径]
#   定时任务:   0 2 * * * /path/to/es_backup.sh --force /path/to/backup.conf
#
# 备份目录结构:
#   /nas_base_path/
#   ├── 2026-03-23/          ← 每天独立目录
#   │   ├── index-0
#   │   ├── indices/...
#   │   └── snap-xxx.dat
#   ├── 2026-03-24/
#   └── 2026-03-25/
# ============================================================

set -euo pipefail

# -------------------- 初始化 --------------------
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FORCE_MODE="false"
NON_INTERACTIVE="false"
DRY_RUN="false"
ON_EXIST_ACTION="ask"
CONFIG_FILE=""

show_help() {
    cat <<EOF
ES Snapshot 备份脚本 (按日期分目录版)

用法:
  bash $(basename "$0") [选项] [配置文件]

选项:
  -f, --force             等价于 --on-exist overwrite
  -y, --yes               非交互模式 (适合 crontab)
      --on-exist <策略>   遇到同名快照时的策略: ask|skip|overwrite|fail
      --dry-run           仅演练清理动作, 不执行删除
  -h, --help              显示帮助

示例:
  bash $(basename "$0") --yes --on-exist overwrite /path/to/backup.conf
  0 2 * * * /path/to/es_backup.sh --yes --on-exist overwrite /path/to/backup.conf
EOF
}

# 解析参数
while [[ $# -gt 0 ]]; do
    case "$1" in
        --force|-f)
            FORCE_MODE="true"
            shift
            ;;
        --yes|-y|--non-interactive)
            NON_INTERACTIVE="true"
            shift
            ;;
        --dry-run)
            DRY_RUN="true"
            shift
            ;;
        --on-exist)
            ON_EXIST_ACTION="${2:-}"
            if [[ -z "${ON_EXIST_ACTION}" ]]; then
                echo "[FATAL] --on-exist 缺少参数 (ask|skip|overwrite|fail)"
                exit 1
            fi
            shift 2
            ;;
        --on-exist=*)
            ON_EXIST_ACTION="${1#*=}"
            shift
            ;;
        --help|-h)
            show_help
            exit 0
            ;;
        *)
            if [[ -z "${CONFIG_FILE}" ]]; then
                CONFIG_FILE="$1"
            else
                echo "[FATAL] 未识别参数: $1"
                exit 1
            fi
            shift
            ;;
    esac
done

if [[ "${FORCE_MODE}" == "true" ]]; then
    ON_EXIST_ACTION="overwrite"
fi

if [[ ! "${ON_EXIST_ACTION}" =~ ^(ask|skip|overwrite|fail)$ ]]; then
    echo "[FATAL] --on-exist 仅支持 ask|skip|overwrite|fail"
    exit 1
fi

CONFIG_FILE="${CONFIG_FILE:-${SCRIPT_DIR}/backup.conf}"

# 加载公共函数库
source "${SCRIPT_DIR}/es_common.sh"

HAS_TTY="false"
if [[ -t 0 && -t 1 && "${NON_INTERACTIVE}" != "true" ]]; then
    HAS_TTY="true"
fi

if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "[FATAL] 配置文件不存在: $CONFIG_FILE"
    exit 1
fi

validate_config_permission "$CONFIG_FILE"

# shellcheck source=backup.conf
source "$CONFIG_FILE"

# 运行参数默认值 (可在 backup.conf 覆盖)
init_curl_defaults
CURL_STATUS_MAX_TIME="${CURL_STATUS_MAX_TIME:-180}"
LOCK_FILE="${LOCK_FILE:-/tmp/es_backup.lock}"

# 加载 ES 凭据
load_es_credentials

# 生成日期标识
DATE_TAG=$(date +"%Y-%m-%d")
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")

# 当天的备份路径和仓库名
BACKUP_DIR="${NAS_BASE_PATH}/${DATE_TAG}"
REPO_NAME="${REPO_PREFIX}-${DATE_TAG}"

# 初始化日志
mkdir -p "$LOG_DIR"
LOG_FILE="${LOG_DIR}/backup_${TIMESTAMP}.log"

# 锁状态
LOCK_HELD="false"

# -------------------- 备份专有函数 --------------------
acquire_lock() {
    exec 9>"$LOCK_FILE" || {
        echo "[FATAL] 无法打开锁文件: $LOCK_FILE"
        exit 1
    }
    if ! flock -n 9; then
        log_warn "已有备份任务在运行, 本次退出 (锁文件: $LOCK_FILE)"
        exit 0
    fi
    LOCK_HELD="true"
    log_info "已获取互斥锁: $LOCK_FILE"
}

release_lock() {
    if [[ "$LOCK_HELD" == "true" ]]; then
        flock -u 9 || true
        LOCK_HELD="false"
    fi
}

safe_dir_check() {
    local dir_path="$1"

    if [[ -z "$dir_path" || -z "${NAS_BASE_PATH:-}" ]]; then
        return 1
    fi

    if [[ "$NAS_BASE_PATH" == "/" || "$NAS_BASE_PATH" == "." ]]; then
        return 1
    fi

    case "$dir_path" in
        "$NAS_BASE_PATH"/????-??-??)
            ;;
        *)
            return 1
            ;;
    esac

    return 0
}

# 发送通知
send_notify() {
    local status="$1"
    local message="$2"

    if [[ "${ENABLE_NOTIFY}" != "true" ]]; then
        return 0
    fi

    if [[ "${NOTIFY_TYPE}" == "webhook" && -n "${WEBHOOK_URL}" ]]; then
        local escaped_message
        escaped_message=$(printf '%s' "$message" | sed 's/\\/\\\\/g; s/"/\\"/g')
        local payload="{\"msgtype\":\"text\",\"text\":{\"content\":\"[ES备份${status}] ${escaped_message}\"}}"
        curl -s -X POST "${WEBHOOK_URL}" \
            -H 'Content-Type: application/json' \
            -d "$payload" > /dev/null 2>&1 || true
        log_info "已发送 Webhook 通知"
    elif [[ "${NOTIFY_TYPE}" == "email" && -n "${MAIL_TO}" ]]; then
        echo "$message" | mail -s "[ES备份${status}] $(hostname) ${TIMESTAMP}" "${MAIL_TO}" || true
        log_info "已发送邮件通知"
    fi
}

# -------------------- 预检查 --------------------
preflight_check() {
    log_info "========== 开始预检查 =========="

    # 1. 检查 ES 连通性
    log_info "检查 ES 集群连通性..."
    local cluster_info
    cluster_info=$(es_request "GET" "/")
    if ! check_response "$cluster_info" "集群连接"; then
        log_error "无法连接 ES 集群: ${ES_HOST}:${ES_PORT}"
        return 1
    fi

    local cluster_name
    cluster_name=$(json_val "$cluster_info" "cluster_name")
    log_info "已连接集群: $cluster_name"

    # 2. 检查集群健康状态
    log_info "检查集群健康状态..."
    local health
    health=$(es_request "GET" "/_cluster/health")
    local status
    status=$(json_val "$health" "status")

    if [[ "$status" == "red" ]]; then
        log_error "集群状态为 RED, 不建议执行备份"
        return 1
    elif [[ "$status" == "yellow" ]]; then
        log_warn "集群状态为 YELLOW, 继续备份但部分分片可能未分配"
    else
        log_info "集群状态: GREEN"
    fi

    # 3. 检查 NAS 基础路径
    log_info "检查 NAS 基础路径: ${NAS_BASE_PATH}"
    if [[ ! -d "$NAS_BASE_PATH" ]]; then
        log_warn "本机 NAS 路径不存在: ${NAS_BASE_PATH} (若脚本不在 ES 节点运行可忽略)"
    fi

    log_info "========== 预检查完成 =========="
    return 0
}

# -------------------- 注册仓库 --------------------
register_repository() {
    log_info "========== 注册 Snapshot 仓库 =========="

    # 检查仓库是否已存在
    local existing
    existing=$(es_request "GET" "/_snapshot/${REPO_NAME}")

    if echo "$existing" | grep -q "\"${REPO_NAME}\""; then
        log_info "仓库 [${REPO_NAME}] 已存在, 跳过注册"
        return 0
    fi

    # 注册仓库, location 指向当天日期子目录
    log_info "注册仓库 [${REPO_NAME}], 路径: ${BACKUP_DIR}"
    local repo_body="{
        \"type\": \"fs\",
        \"settings\": {
            \"location\": \"${BACKUP_DIR}\",
            \"compress\": ${REPO_COMPRESS}
        }
    }"

    local result
    result=$(es_request "PUT" "/_snapshot/${REPO_NAME}" "$repo_body")

    if ! check_response "$result" "仓库注册"; then
        log_error "仓库注册失败, 请确认:"
        log_error "  1. elasticsearch.yml 中已配置 path.repo: [\"${NAS_BASE_PATH}\"]"
        log_error "  2. 所有数据节点的 NAS 已正确挂载"
        log_error "  3. elasticsearch 用户对 ${BACKUP_DIR} 有读写权限"
        return 1
    fi

    log_info "仓库 [${REPO_NAME}] 注册成功"

    # 验证仓库
    log_info "验证仓库可用性..."
    local verify
    verify=$(es_request "POST" "/_snapshot/${REPO_NAME}/_verify")
    if ! check_response "$verify" "仓库验证"; then
        log_warn "仓库验证有异常, 请检查各节点的 NAS 挂载状态"
    else
        log_info "仓库验证通过"
    fi

    return 0
}

# -------------------- 创建快照 --------------------
create_snapshot() {
    log_info "========== 创建快照: ${SNAPSHOT_NAME} =========="

    # 检查今天的快照是否已存在
    local existing_snap
    existing_snap=$(es_request "GET" "/_snapshot/${REPO_NAME}/${SNAPSHOT_NAME}")
    if ! echo "$existing_snap" | grep -q '"error"'; then
        local existing_state
        existing_state=$(json_val "$existing_snap" "state")
        log_warn "今天的快照 [${SNAPSHOT_NAME}] 已存在 (状态: ${existing_state})"

        case "$ON_EXIST_ACTION" in
            overwrite)
                log_info "策略 overwrite: 自动删除旧快照并重建"
                ;;
            skip)
                log_info "策略 skip: 保留已有快照, 本次退出"
                return 0
                ;;
            fail)
                log_error "策略 fail: 检测到同名快照, 任务失败退出"
                return 1
                ;;
            ask)
                if [[ "$HAS_TTY" == "true" ]]; then
                    log_warn "如需覆盖, 请选择操作:"
                    echo ""
                    echo "  1) 删除旧快照, 重新备份"
                    echo "  2) 跳过备份, 保留已有快照 (退出)"
                    echo ""
                    read -r -p "请选择 [1/2]: " choice
                    case "$choice" in
                        1)
                            log_info "用户选择: 删除旧快照, 重新备份"
                            ;;
                        *)
                            log_info "用户选择: 跳过备份, 保留已有快照"
                            return 0
                            ;;
                    esac
                else
                    log_warn "非交互环境下 ask 不可用, 自动降级为 skip"
                    return 0
                fi
                ;;
        esac

        # 删除旧快照
        log_info "删除旧快照..."
        local del_result
        del_result=$(es_request "DELETE" "/_snapshot/${REPO_NAME}/${SNAPSHOT_NAME}")
        if echo "$del_result" | grep -q '"error"'; then
            log_error "删除旧快照失败: $del_result"
            return 1
        fi
        log_info "旧快照已删除"
    fi

    local snapshot_body="{
        \"indices\": \"${BACKUP_INDICES}\",
        \"ignore_unavailable\": ${IGNORE_UNAVAILABLE},
        \"include_global_state\": ${INCLUDE_GLOBAL_STATE}
    }"

    # 发起异步快照
    local result
    result=$(es_request "PUT" "/_snapshot/${REPO_NAME}/${SNAPSHOT_NAME}?wait_for_completion=false" "$snapshot_body")

    if ! check_response "$result" "创建快照"; then
        return 1
    fi

    log_info "快照创建请求已提交, 等待完成..."

    # 轮询等待快照完成
    local max_wait=${SNAPSHOT_TIMEOUT%s}
    local waited=0
    local poll_interval=10

    while [[ $waited -lt $max_wait ]]; do
        sleep $poll_interval
        waited=$((waited + poll_interval))

        local status_resp
        status_resp=$(es_request "GET" "/_snapshot/${REPO_NAME}/${SNAPSHOT_NAME}")

        local snap_state
        snap_state=$(json_val "$status_resp" "state")

        case "$snap_state" in
            "SUCCESS")
                log_info "快照 [${SNAPSHOT_NAME}] 创建成功!"
                local total_shards
                total_shards=$(json_num "$status_resp" "total")
                local success_shards
                success_shards=$(json_num "$status_resp" "successful")
                log_info "分片信息: 成功 ${success_shards}/${total_shards}"
                return 0
                ;;
            "FAILED")
                log_error "快照 [${SNAPSHOT_NAME}] 创建失败!"
                log_error "详情: $status_resp"
                return 1
                ;;
            "PARTIAL")
                log_warn "快照 [${SNAPSHOT_NAME}] 部分完成 (存在失败分片)"
                return 1
                ;;
            "IN_PROGRESS")
                local progress_resp
                progress_resp=$(es_request "GET" "/_snapshot/${REPO_NAME}/${SNAPSHOT_NAME}/_status" "" "$CURL_STATUS_MAX_TIME")
                local shards_done
                shards_done=$(json_num "$progress_resp" "done" || echo "?")
                local shards_total
                shards_total=$(json_num "$progress_resp" "total" || echo "?")
                log_info "进行中... 已等待 ${waited}s, 分片进度: ${shards_done}/${shards_total}"
                ;;
            *)
                log_warn "未知状态: [${snap_state}], 继续等待... (原始响应: ${status_resp})"
                ;;
        esac
    done

    log_error "快照超时 (已等待 ${max_wait}s)"
    return 1
}

# -------------------- 清理旧备份 --------------------
cleanup_old_backups() {
    if [[ "${RETAIN_SNAPSHOTS}" -eq 0 ]]; then
        log_info "保留策略: 不清理旧备份"
        return 0
    fi

    log_info "========== 清理旧备份 (保留最近 ${RETAIN_SNAPSHOTS} 天) =========="

    # 列出所有日期目录, 按名称排序 (YYYY-MM-DD 格式天然有序)
    local all_dirs
    all_dirs=$(find "$NAS_BASE_PATH" -maxdepth 1 -mindepth 1 -type d -name "????-??-??" | sort)

    local total=0
    if [[ -n "$all_dirs" ]]; then
        total=$(printf '%s\n' "$all_dirs" | wc -l)
    fi

    if [[ $total -le $RETAIN_SNAPSHOTS ]]; then
        log_info "当前共 ${total} 个备份目录, 不需要清理"
        return 0
    fi

    local to_delete=$((total - RETAIN_SNAPSHOTS))
    log_info "当前共 ${total} 个备份目录, 需要删除最旧的 ${to_delete} 个"

    local delete_list
    delete_list=$(echo "$all_dirs" | head -n "$to_delete")

    local deleted=0
    local failed=0
    while IFS= read -r dir_path; do
        [[ -z "$dir_path" ]] && continue
        local dir_date
        dir_date=$(basename "$dir_path")
        local old_repo_name="${REPO_PREFIX}-${dir_date}"

        log_info "清理备份: ${dir_date}"

        # Step 1: 删除 ES 中该仓库的快照
        log_info "  删除仓库 [${old_repo_name}] ..."
        local del_result
        del_result=$(es_request "DELETE" "/_snapshot/${old_repo_name}")
        if check_response "$del_result" "删除仓库 ${old_repo_name}"; then
            log_info "  仓库 [${old_repo_name}] 已删除"
        else
            log_warn "  仓库 [${old_repo_name}] 删除失败 (可能已不存在)"
        fi

        # Step 2: 删除 NAS 上的目录
        if ! safe_dir_check "$dir_path"; then
            log_error "  跳过删除(安全校验失败): ${dir_path}"
            failed=$((failed + 1))
            continue
        fi

        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "  [DRY-RUN] 将删除目录: ${dir_path}"
            deleted=$((deleted + 1))
            continue
        fi

        log_info "  删除目录: ${dir_path}"
        if rm -rf "$dir_path" 2>/dev/null; then
            log_info "  目录已删除"
            deleted=$((deleted + 1))
        else
            log_warn "  目录删除失败 (权限问题?)"
            failed=$((failed + 1))
        fi
    done <<< "$delete_list"

    log_info "清理完成: 删除 ${deleted} 个, 失败 ${failed} 个"
    return 0
}

# -------------------- 清理旧日志 --------------------
cleanup_old_logs() {
    if [[ "${LOG_RETAIN_DAYS}" -gt 0 ]]; then
        log_info "清理 ${LOG_RETAIN_DAYS} 天前的日志文件..."
        find "$LOG_DIR" -name "backup_*.log" -mtime +"${LOG_RETAIN_DAYS}" -delete 2>/dev/null || true
    fi
}

cleanup_on_signal() {
    log_warn "接收到中断信号，尝试释放锁并取消进行中的快照..."
    if [[ -n "${REPO_NAME:-}" && -n "${SNAPSHOT_NAME:-}" ]]; then
        local cancel_resp
        cancel_resp=$(es_request "DELETE" "/_snapshot/${REPO_NAME}/${SNAPSHOT_NAME}" "" "$CURL_STATUS_MAX_TIME")
        if check_response "$cancel_resp" "取消进行中的快照"; then
            log_info "已发送快照取消请求"
        else
            log_warn "快照取消请求可能失败或无需取消"
        fi
    fi
    release_lock
    exit 130
}

# -------------------- 主流程 --------------------
main() {
    local start_time
    start_time=$(date +%s)

    acquire_lock
    trap 'release_lock' EXIT
    trap 'cleanup_on_signal' INT TERM

    log_info "============================================"
    log_info "  ES Snapshot 备份任务启动"
    log_info "  配置文件: ${CONFIG_FILE}"
    log_info "  ES 集群:  ${ES_HOST}:${ES_PORT}"
    log_info "  备份日期: ${DATE_TAG}"
    log_info "  备份目录: ${BACKUP_DIR}"
    log_info "  仓库名:   ${REPO_NAME}"
    log_info "  快照名:   ${SNAPSHOT_NAME}"
    log_info "  备份索引: ${BACKUP_INDICES}"
    log_info "============================================"

    # Step 1: 预检查
    if ! preflight_check; then
        log_error "预检查失败, 终止备份"
        send_notify "失败" "预检查失败, 请检查 ES 集群状态和网络连通性"
        exit 1
    fi

    # Step 2: 注册仓库
    if ! register_repository; then
        log_error "仓库注册失败, 终止备份"
        send_notify "失败" "仓库 [${REPO_NAME}] 注册失败, 请检查 path.repo 配置和 NAS 挂载"
        exit 1
    fi

    # Step 3: 创建快照
    if ! create_snapshot; then
        log_error "快照创建失败"
        send_notify "失败" "快照创建失败, 目录: ${BACKUP_DIR}"
        exit 1
    fi

    # Step 4: 清理旧备份 (目录 + 仓库)
    cleanup_old_backups

    # Step 5: 清理旧日志
    cleanup_old_logs

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))
    local duration_min=$((duration / 60))

    log_info "============================================"
    log_info "  备份任务完成!"
    log_info "  备份目录: ${BACKUP_DIR}"
    log_info "  耗时: ${duration}s (${duration_min}分钟)"
    log_info "============================================"

    send_notify "成功" "备份完成 [${DATE_TAG}], 目录: ${BACKUP_DIR}, 耗时 ${duration_min} 分钟"
    exit 0
}

main

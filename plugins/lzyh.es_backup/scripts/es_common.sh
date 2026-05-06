#!/bin/bash
# ============================================================
# ES Snapshot 公共函数库
# 被 es_backup.sh 和 es_restore.sh 共同引用
# 用法: source "$(dirname "$0")/es_common.sh"
# ============================================================

# -------------------- 配置文件权限校验 --------------------
validate_config_permission() {
    local cfg="$1"
    local perm=""

    if perm=$(stat -c '%a' "$cfg" 2>/dev/null); then
        :
    elif perm=$(stat -f '%Lp' "$cfg" 2>/dev/null); then
        :
    else
        echo "[WARN] 无法检查配置文件权限: $cfg"
        return 0
    fi

    local other=$((perm % 10))
    if (( (other & 2) != 0 )); then
        echo "[FATAL] 配置文件权限过大(${perm})，请去掉 other 写权限"
        exit 1
    fi
}

# -------------------- 密码加载 --------------------
# 凭据优先级: 环境变量 ES_PASSWORD > ES_PASSWORD_FILE > backup.conf 的 ES_PASSWORD
load_es_credentials() {
    if [[ -n "${ES_PASSWORD_FILE:-}" && -z "${ES_PASSWORD:-}" ]]; then
        if [[ -r "$ES_PASSWORD_FILE" ]]; then
            ES_PASSWORD=$(head -n 1 "$ES_PASSWORD_FILE")
        else
            echo "[FATAL] ES_PASSWORD_FILE 不可读: $ES_PASSWORD_FILE"
            exit 1
        fi
    fi

    if [[ -z "${ES_PASSWORD:-}" ]]; then
        echo "[WARN] ES_PASSWORD 为空，若集群开启认证将连接失败"
    fi
}

# -------------------- CURL 默认值 --------------------
init_curl_defaults() {
    CURL_CONNECT_TIMEOUT="${CURL_CONNECT_TIMEOUT:-5}"
    CURL_MAX_TIME="${CURL_MAX_TIME:-60}"
    CURL_RETRY="${CURL_RETRY:-2}"
    CURL_RETRY_DELAY="${CURL_RETRY_DELAY:-2}"
}

# -------------------- 日志函数 --------------------
# 全局变量 LOG_FILE: 若已设置则同时写入文件, 否则仅输出到终端
_log_impl() {
    local level="$1"
    shift
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] [${level}] $*"

    if [[ -n "${LOG_FILE:-}" ]]; then
        echo "$msg" | tee -a "$LOG_FILE"
    else
        echo "$msg"
    fi
}

log_info()  { _log_impl "INFO"  "$@"; }
log_warn()  { _log_impl "WARN"  "$@"; }
log_error() { _log_impl "ERROR" "$@"; }

# -------------------- URL 构造 --------------------
build_es_url() {
    local path="$1"
    echo "${ES_HOST}:${ES_PORT}${path}"
}

# -------------------- JSON 解析 --------------------
# 从 JSON 中提取指定 key 的第一个 string 值 (仅依赖默认命令)
json_val() {
    local json="$1"
    local key="$2"
    printf '%s\n' "$json" \
        | tr -d '\n' \
        | grep -oE "\"${key}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" \
        | head -1 \
        | sed -E 's/.*:[[:space:]]*"([^"]*)"/\1/'
}

# 从 JSON 中提取指定 key 的第一个数字值 (仅依赖默认命令)
json_num() {
    local json="$1"
    local key="$2"
    printf '%s\n' "$json" \
        | tr -d '\n' \
        | grep -oE "\"${key}\"[[:space:]]*:[[:space:]]*[0-9]+" \
        | head -1 \
        | sed -E 's/.*:[[:space:]]*([0-9]+)/\1/'
}

# -------------------- ES API 请求 --------------------
ES_LAST_HTTP_STATUS="0"

es_request() {
    local method="$1"
    local path="$2"
    local data="${3:-}"
    local max_time_override="${4:-$CURL_MAX_TIME}"

    local url
    url=$(build_es_url "$path")

    local -a curl_args=(
        -sS
        -X "$method"
        --connect-timeout "$CURL_CONNECT_TIMEOUT"
        --max-time "$max_time_override"
        --retry "$CURL_RETRY"
        --retry-delay "$CURL_RETRY_DELAY"
        --retry-all-errors
        -w "\nHTTP_STATUS:%{http_code}\n"
    )

    if [[ -n "${ES_USER:-}" && -n "${ES_PASSWORD:-}" ]]; then
        curl_args+=(-u "${ES_USER}:${ES_PASSWORD}")
    fi
    if [[ "${ES_SKIP_SSL_VERIFY:-false}" == "true" ]]; then
        curl_args+=(-k)
    fi
    if [[ -n "$data" ]]; then
        curl_args+=(-H "Content-Type: application/json" -d "$data")
    fi

    local resp
    resp=$(curl "${curl_args[@]}" "$url" 2>&1 || true)

    ES_LAST_HTTP_STATUS=$(printf '%s\n' "$resp" | awk -F: '/^HTTP_STATUS:[0-9]+$/ {code=$2} END {if (code=="") print 0; else print code}')
    printf '%s\n' "$resp" | awk '!/^HTTP_STATUS:[0-9]+$/'
}

# -------------------- 响应校验 --------------------
check_response() {
    local body="$1"
    local action="$2"

    if [[ "$ES_LAST_HTTP_STATUS" -lt 200 || "$ES_LAST_HTTP_STATUS" -ge 300 ]]; then
        log_error "${action} 失败: HTTP ${ES_LAST_HTTP_STATUS}, 响应: $body"
        return 1
    fi

    if echo "$body" | grep -qE '"error"[[:space:]]*:[[:space:]]*\{'; then
        log_error "${action} 失败: $body"
        return 1
    fi
    return 0
}

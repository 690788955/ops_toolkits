#!/usr/bin/env bash
set -euo pipefail

# 系统资源检查脚本 - 跨发行版兼容
# 支持 CentOS、Ubuntu、Debian、RHEL 等主流 Linux 发行版

# 默认参数
FORMAT="text"

# 解析参数
while [[ $# -gt 0 ]]; do
  case $1 in
    --format)
      FORMAT="$2"
      shift 2
      ;;
    *)
      echo "未知参数: $1" >&2
      echo "用法: $0 [--format text|json]" >&2
      exit 1
      ;;
  esac
done

# 验证格式参数
if [[ "$FORMAT" != "text" && "$FORMAT" != "json" ]]; then
  echo "错误: format 必须是 text 或 json" >&2
  exit 1
fi

# ============ 数据收集函数 ============

# 获取系统信息
get_system_info() {
  local os_name="" os_version="" kernel="" hostname="" arch=""

  # 读取 /etc/os-release
  if [[ -f /etc/os-release ]]; then
    source /etc/os-release
    os_name="${NAME:-Unknown}"
    os_version="${VERSION:-${VERSION_ID:-Unknown}}"
  elif [[ -f /etc/redhat-release ]]; then
    os_name=$(cat /etc/redhat-release)
    os_version=""
  else
    os_name="Unknown"
    os_version=""
  fi

  kernel=$(uname -r)
  hostname=$(hostname)
  arch=$(uname -m)

  echo "$os_name|$os_version|$kernel|$hostname|$arch"
}

# 获取 CPU 信息
get_cpu_info() {
  local model="" physical_cores="" logical_cores="" usage=""

  # CPU 型号
  model=$(grep -m1 "model name" /proc/cpuinfo | cut -d: -f2 | sed 's/^[ \t]*//')

  # 物理核心数
  physical_cores=$(grep "physical id" /proc/cpuinfo | sort -u | wc -l)
  if [[ $physical_cores -eq 0 ]]; then
    physical_cores=1
  fi

  # 逻辑核心数
  logical_cores=$(grep -c "^processor" /proc/cpuinfo)

  # CPU 使用率（1秒采样）
  if command -v top >/dev/null 2>&1; then
    usage=$(top -bn2 -d1 | grep "Cpu(s)" | tail -1 | awk '{print $2}' | cut -d'%' -f1)
  else
    usage="N/A"
  fi

  echo "$model|$physical_cores|$logical_cores|$usage"
}

# 获取内存信息
get_memory_info() {
  local total="" used="" available="" usage_percent=""

  if [[ -f /proc/meminfo ]]; then
    total=$(grep "MemTotal:" /proc/meminfo | awk '{print $2}')
    available=$(grep "MemAvailable:" /proc/meminfo | awk '{print $2}')
    if [[ -z "$available" ]]; then
      # 旧版本 Linux 没有 MemAvailable
      free_mem=$(grep "MemFree:" /proc/meminfo | awk '{print $2}')
      buffers=$(grep "Buffers:" /proc/meminfo | awk '{print $2}')
      cached=$(grep "^Cached:" /proc/meminfo | awk '{print $2}')
      available=$((free_mem + buffers + cached))
    fi
    used=$((total - available))
    usage_percent=$(awk "BEGIN {printf \"%.2f\", ($used/$total)*100}")
  else
    total="N/A"
    used="N/A"
    available="N/A"
    usage_percent="N/A"
  fi

  echo "$total|$used|$available|$usage_percent"
}

# 获取磁盘信息
get_disk_info() {
  df -h | grep -E '^/dev/' | awk '{print $1"|"$2"|"$3"|"$4"|"$5"|"$6}'
}

# 获取负载信息
get_load_info() {
  if [[ -f /proc/loadavg ]]; then
    read load1 load5 load15 _ < /proc/loadavg
    echo "$load1|$load5|$load15"
  else
    echo "N/A|N/A|N/A"
  fi
}

# ============ 输出函数 ============

# 文本格式输出
output_text() {
  echo "========================================="
  echo "系统资源检查报告"
  echo "========================================="
  echo ""

  # 系统信息
  IFS='|' read -r os_name os_version kernel hostname arch <<< "$(get_system_info)"
  echo "【系统信息】"
  echo "  操作系统: $os_name $os_version"
  echo "  内核版本: $kernel"
  echo "  主机名: $hostname"
  echo "  系统架构: $arch"
  echo ""

  # CPU 信息
  IFS='|' read -r cpu_model physical_cores logical_cores cpu_usage <<< "$(get_cpu_info)"
  echo "【CPU 信息】"
  echo "  型号: $cpu_model"
  echo "  物理核心: $physical_cores"
  echo "  逻辑核心: $logical_cores"
  echo "  使用率: ${cpu_usage}%"
  echo ""

  # 内存信息
  IFS='|' read -r mem_total mem_used mem_available mem_usage <<< "$(get_memory_info)"
  echo "【内存信息】"
  echo "  总内存: $(awk "BEGIN {printf \"%.2f GB\", $mem_total/1024/1024}")"
  echo "  已用内存: $(awk "BEGIN {printf \"%.2f GB\", $mem_used/1024/1024}")"
  echo "  可用内存: $(awk "BEGIN {printf \"%.2f GB\", $mem_available/1024/1024}")"
  echo "  使用率: ${mem_usage}%"
  echo ""

  # 磁盘信息
  echo "【磁盘信息】"
  printf "  %-20s %-10s %-10s %-10s %-10s %s\n" "设备" "总容量" "已用" "可用" "使用率" "挂载点"
  while IFS='|' read -r device size used avail usage mount; do
    printf "  %-20s %-10s %-10s %-10s %-10s %s\n" "$device" "$size" "$used" "$avail" "$usage" "$mount"
  done <<< "$(get_disk_info)"
  echo ""

  # 负载信息
  IFS='|' read -r load1 load5 load15 <<< "$(get_load_info)"
  echo "【系统负载】"
  echo "  1 分钟: $load1"
  echo "  5 分钟: $load5"
  echo "  15 分钟: $load15"
  echo ""

  echo "========================================="
  echo "检查完成"
  echo "========================================="
}

# JSON 格式输出
output_json() {
  # 系统信息
  IFS='|' read -r os_name os_version kernel hostname arch <<< "$(get_system_info)"

  # CPU 信息
  IFS='|' read -r cpu_model physical_cores logical_cores cpu_usage <<< "$(get_cpu_info)"

  # 内存信息
  IFS='|' read -r mem_total mem_used mem_available mem_usage <<< "$(get_memory_info)"

  # 负载信息
  IFS='|' read -r load1 load5 load15 <<< "$(get_load_info)"

  # 磁盘信息（构建 JSON 数组）
  disk_json="["
  first=true
  while IFS='|' read -r device size used avail usage mount; do
    if [[ "$first" == "true" ]]; then
      first=false
    else
      disk_json+=","
    fi
    disk_json+="{\"device\":\"$device\",\"size\":\"$size\",\"used\":\"$used\",\"available\":\"$avail\",\"usage\":\"$usage\",\"mount\":\"$mount\"}"
  done <<< "$(get_disk_info)"
  disk_json+="]"

  # 输出 JSON
  cat <<EOF
{
  "system": {
    "os_name": "$os_name",
    "os_version": "$os_version",
    "kernel": "$kernel",
    "hostname": "$hostname",
    "architecture": "$arch"
  },
  "cpu": {
    "model": "$cpu_model",
    "physical_cores": $physical_cores,
    "logical_cores": $logical_cores,
    "usage_percent": "$cpu_usage"
  },
  "memory": {
    "total_kb": $mem_total,
    "used_kb": $mem_used,
    "available_kb": $mem_available,
    "usage_percent": "$mem_usage"
  },
  "disk": $disk_json,
  "load": {
    "load_1min": "$load1",
    "load_5min": "$load5",
    "load_15min": "$load15"
  }
}
EOF
}

# ============ 主逻辑 ============

case "$FORMAT" in
  text)
    output_text
    ;;
  json)
    output_json
    ;;
esac

exit 0

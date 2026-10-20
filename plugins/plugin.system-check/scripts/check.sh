#!/bin/sh
set -eu

# 系统资源检查脚本 - POSIX 兼容
# 适用于 dash、BusyBox ash、bash 等常见 /bin/sh 实现

FORMAT="text"

while [ $# -gt 0 ]; do
  case "$1" in
    --format)
      if [ $# -lt 2 ]; then
        echo "错误: --format 需要一个参数" >&2
        exit 1
      fi
      FORMAT="$2"
      shift 2
      ;;
    -h|--help)
      echo "用法: $0 [--format text|json]"
      exit 0
      ;;
    *)
      echo "未知参数: $1" >&2
      echo "用法: $0 [--format text|json]" >&2
      exit 1
      ;;
  esac
done

case "$FORMAT" in
  text|json) ;;
  *)
    echo "错误: format 必须是 text 或 json" >&2
    exit 1
    ;;
esac

# ============ 数据收集 ============

collect_system_info() {
  os_name="Unknown"
  os_version="Unknown"
  if [ -r /etc/os-release ]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    os_name="${NAME:-Unknown}"
    os_version="${VERSION:-${VERSION_ID:-Unknown}}"
  elif [ -r /etc/redhat-release ]; then
    os_name=$(cat /etc/redhat-release)
    os_version=""
  fi
  kernel_version=$(uname -r)
  host_name=$(hostname 2>/dev/null || uname -n)
  arch=$(uname -m)
}

collect_cpu_info() {
  cpu_model=""
  if [ -r /proc/cpuinfo ]; then
    cpu_model=$(grep -E "model name|^Model|^Hardware|^cpu model" /proc/cpuinfo 2>/dev/null | head -n 1 | cut -d: -f2- | sed 's/^[[:space:]]*//')
  fi
  if [ -z "$cpu_model" ]; then
    cpu_model=$(uname -m)
  fi

  physical_cores=0
  if [ -r /proc/cpuinfo ]; then
    physical_cores=$(grep "physical id" /proc/cpuinfo 2>/dev/null | sort -u | wc -l)
  fi
  if [ "$physical_cores" -eq 0 ] 2>/dev/null; then
    physical_cores=1
  fi

  logical_cores=1
  if [ -r /proc/cpuinfo ]; then
    logical_cores=$(grep -c "^processor" /proc/cpuinfo 2>/dev/null || true)
    [ -n "$logical_cores" ] || logical_cores=1
    [ "$logical_cores" -gt 0 ] || logical_cores=1
  fi

  cpu_usage="N/A"
  if [ -r /proc/stat ]; then
    set -- $(awk '/^cpu / {print $2+$3+$4+$5+$6+$7+$8, $5+$6; exit}' /proc/stat)
    if [ $# -eq 2 ]; then
      prev_total=$1
      prev_idle=$2
      sleep 1
      set -- $(awk '/^cpu / {print $2+$3+$4+$5+$6+$7+$8, $5+$6; exit}' /proc/stat)
      if [ $# -eq 2 ]; then
        cpu_usage=$(awk -v t1="$prev_total" -v i1="$prev_idle" -v t2="$1" -v i2="$2" 'BEGIN {
          dt = t2 - t1
          if (dt <= 0) { print "N/A"; exit }
          printf "%.2f", ((dt - (i2 - i1)) / dt) * 100
        }')
      fi
    fi
  fi
}

collect_memory_info() {
  mem_total="N/A"
  mem_used="N/A"
  mem_available="N/A"
  mem_usage="N/A"
  if [ -r /proc/meminfo ]; then
    mem_total=$(awk '/^MemTotal:/ {print $2; exit}' /proc/meminfo)
    mem_available=$(awk '/^MemAvailable:/ {print $2; exit}' /proc/meminfo)
    if [ -z "$mem_available" ]; then
      # 旧版本内核没有 MemAvailable
      mem_free=$(awk '/^MemFree:/ {print $2; exit}' /proc/meminfo)
      mem_buffers=$(awk '/^Buffers:/ {print $2; exit}' /proc/meminfo)
      mem_cached=$(awk '/^Cached:/ {print $2; exit}' /proc/meminfo)
      mem_available=$((mem_free + mem_buffers + mem_cached))
    fi
    if [ -n "$mem_total" ] && [ "$mem_total" -gt 0 ]; then
      mem_used=$((mem_total - mem_available))
      mem_usage=$(awk -v used="$mem_used" -v total="$mem_total" 'BEGIN {printf "%.2f", (used / total) * 100}')
    fi
  fi
}

collect_load_info() {
  load1="N/A"
  load5="N/A"
  load15="N/A"
  if [ -r /proc/loadavg ]; then
    read -r load1 load5 load15 _rest </proc/loadavg || true
  fi
}

disk_rows() {
  df -P -h 2>/dev/null | awk 'NR > 1 && $1 ~ /^\/dev\//'
}

disk_json() {
  df -P -h 2>/dev/null | awk '
    BEGIN { printf "["; first = 1 }
    NR > 1 && $1 ~ /^\/dev\// {
      if (!first) printf ","
      printf "{\"device\":\"%s\",\"size\":\"%s\",\"used\":\"%s\",\"available\":\"%s\",\"usage\":\"%s\",\"mount\":\"%s\"}", $1, $2, $3, $4, $5, $6
      first = 0
    }
    END { printf "]" }
  '
}

# ============ 输出 ============

output_text() {
  echo "========================================="
  echo "系统资源检查报告"
  echo "========================================="
  echo ""

  echo "【系统信息】"
  echo "  操作系统: $os_name $os_version"
  echo "  内核版本: $kernel_version"
  echo "  主机名: $host_name"
  echo "  系统架构: $arch"
  echo ""

  echo "【CPU 信息】"
  echo "  型号: $cpu_model"
  echo "  物理核心: $physical_cores"
  echo "  逻辑核心: $logical_cores"
  echo "  使用率: ${cpu_usage}%"
  echo ""

  echo "【内存信息】"
  if [ "$mem_total" != "N/A" ]; then
    echo "  总内存: $(awk -v kb="$mem_total" 'BEGIN {printf "%.2f GB", kb / 1024 / 1024}')"
    echo "  已用内存: $(awk -v kb="$mem_used" 'BEGIN {printf "%.2f GB", kb / 1024 / 1024}')"
    echo "  可用内存: $(awk -v kb="$mem_available" 'BEGIN {printf "%.2f GB", kb / 1024 / 1024}')"
  else
    echo "  总内存: N/A"
    echo "  已用内存: N/A"
    echo "  可用内存: N/A"
  fi
  echo "  使用率: ${mem_usage}%"
  echo ""

  echo "【磁盘信息】"
  printf "  %-20s %-10s %-10s %-10s %-10s %s\n" "设备" "总容量" "已用" "可用" "使用率" "挂载点"
  disk_rows | awk '{printf "  %-20s %-10s %-10s %-10s %-10s %s\n", $1, $2, $3, $4, $5, $6}'
  echo ""

  echo "【系统负载】"
  echo "  1 分钟: $load1"
  echo "  5 分钟: $load5"
  echo "  15 分钟: $load15"
  echo ""

  echo "========================================="
  echo "检查完成"
  echo "========================================="
}

output_json() {
  if [ "$mem_total" = "N/A" ]; then
    mem_total_json=null
    mem_used_json=null
    mem_available_json=null
  else
    mem_total_json=$mem_total
    mem_used_json=$mem_used
    mem_available_json=$mem_available
  fi
  cat <<EOF
{
  "system": {
    "os_name": "$os_name",
    "os_version": "$os_version",
    "kernel": "$kernel_version",
    "hostname": "$host_name",
    "architecture": "$arch"
  },
  "cpu": {
    "model": "$cpu_model",
    "physical_cores": $physical_cores,
    "logical_cores": $logical_cores,
    "usage_percent": "$cpu_usage"
  },
  "memory": {
    "total_kb": $mem_total_json,
    "used_kb": $mem_used_json,
    "available_kb": $mem_available_json,
    "usage_percent": "$mem_usage"
  },
  "disk": $(disk_json),
  "load": {
    "load_1min": "$load1",
    "load_5min": "$load5",
    "load_15min": "$load15"
  }
}
EOF
}

# ============ 主逻辑 ============

collect_system_info
collect_cpu_info
collect_memory_info
collect_load_info

if [ "$FORMAT" = "json" ]; then
  output_json
else
  output_text
fi

exit 0

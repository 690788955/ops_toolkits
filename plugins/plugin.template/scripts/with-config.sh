#!/usr/bin/env bash
set -euo pipefail

CONFIG_FILE=""
ACTION="check"

while [[ $# -gt 0 ]]; do
  case $1 in
    --action)
      ACTION="$2"
      shift 2
      ;;
    --config)
      CONFIG_FILE="$2"
      shift 2
      ;;
    *)
      echo "未知参数: $1" >&2
      echo "用法: $0 --action <操作> --config <插件内配置文件路径>" >&2
      exit 1
      ;;
  esac
done

if [[ -z "$CONFIG_FILE" ]]; then
  echo "错误: --config 参数必填" >&2
  exit 1
fi

if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "错误: 配置文件不存在: $CONFIG_FILE" >&2
  exit 1
fi

echo "========================================="
echo "配置文件工具示例"
echo "========================================="
echo ""
echo "操作: $ACTION"
echo "配置文件路径: $CONFIG_FILE"
echo ""
echo "配置文件内容:"
echo "---"
cat "$CONFIG_FILE"
echo "---"
echo ""

SERVICE_NAME=$(grep "^name = " "$CONFIG_FILE" | cut -d= -f2 | tr -d ' ')
echo "服务名称: $SERVICE_NAME"
echo ""

echo "========================================="
echo "执行完成"
echo "========================================="

exit 0

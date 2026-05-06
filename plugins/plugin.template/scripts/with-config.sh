#!/usr/bin/env bash
set -euo pipefail

# 配置文件工具示例 - 使用配置模板
# 演示如何接收和使用框架生成的配置文件

CONFIG_FILE=""

# 解析参数
while [[ $# -gt 0 ]]; do
  case $1 in
    --config)
      CONFIG_FILE="$2"
      shift 2
      ;;
    *)
      echo "未知参数: $1" >&2
      echo "用法: $0 --config <配置文件路径>" >&2
      exit 1
      ;;
  esac
done

# 验证配置文件
if [[ -z "$CONFIG_FILE" ]]; then
  echo "错误: --config 参数必填" >&2
  exit 1
fi

if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "错误: 配置文件不存在: $CONFIG_FILE" >&2
  exit 1
fi

# 执行逻辑
echo "========================================="
echo "配置文件工具示例"
echo "========================================="
echo ""
echo "配置文件路径: $CONFIG_FILE"
echo ""
echo "配置文件内容:"
echo "---"
cat "$CONFIG_FILE"
echo "---"
echo ""

# 解析配置文件（示例：读取 service.name）
SERVICE_NAME=$(grep "^name = " "$CONFIG_FILE" | cut -d= -f2 | tr -d ' ')
echo "服务名称: $SERVICE_NAME"
echo ""

echo "========================================="
echo "执行完成"
echo "========================================="

exit 0

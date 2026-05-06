#!/usr/bin/env bash
set -euo pipefail

# 简单工具示例 - 不使用配置文件
# 演示基本的参数解析和错误处理

MESSAGE=""

# 解析参数
while [[ $# -gt 0 ]]; do
  case $1 in
    --message)
      MESSAGE="$2"
      shift 2
      ;;
    *)
      echo "未知参数: $1" >&2
      echo "用法: $0 --message <消息内容>" >&2
      exit 1
      ;;
  esac
done

# 验证参数
if [[ -z "$MESSAGE" ]]; then
  echo "错误: --message 参数必填" >&2
  exit 1
fi

# 执行逻辑
echo "========================================="
echo "简单工具示例"
echo "========================================="
echo ""
echo "消息: $MESSAGE"
echo ""
echo "========================================="
echo "执行完成"
echo "========================================="

exit 0

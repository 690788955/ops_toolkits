#!/usr/bin/env bash
set -euo pipefail

# Docker 容器内执行命令（非交互式）
# 用法: docker-exec.sh <容器名或ID> <命令>
# 示例: docker-exec.sh ansible ./ops/everisk/install_all.sh

CONTAINER="${1:-}"
COMMAND="${2:-}"

# 参数校验
if [[ -z "$CONTAINER" ]]; then
  echo "错误: 未指定容器名称或 ID" >&2
  echo "用法: $0 <容器名或ID> <命令>" >&2
  exit 1
fi

if [[ -z "$COMMAND" ]]; then
  echo "错误: 未指定要执行的命令" >&2
  echo "用法: $0 <容器名或ID> <命令>" >&2
  exit 1
fi

# 检查 docker 命令是否可用
if ! command -v docker &>/dev/null; then
  echo "错误: docker 命令未找到，请确认 Docker 已安装且在 PATH 中" >&2
  exit 1
fi

# 检查容器是否存在且正在运行
if ! docker inspect --format='{{.State.Running}}' "$CONTAINER" 2>/dev/null | grep -q 'true'; then
  echo "错误: 容器 [$CONTAINER] 不存在或未运行" >&2
  echo "提示: 使用 docker ps -a 查看所有容器状态" >&2
  exit 1
fi

echo "========================================="
echo "Docker 容器内执行命令"
echo "========================================="
echo "容器: $CONTAINER"
echo "命令: $COMMAND"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================="
echo ""

# 执行命令（非交互式，不使用 -t）
docker exec "$CONTAINER" $COMMAND

EXIT_CODE=$?

echo ""
echo "========================================="
if [[ $EXIT_CODE -eq 0 ]]; then
  echo "执行完成 (退出码: $EXIT_CODE)"
else
  echo "执行失败 (退出码: $EXIT_CODE)" >&2
fi
echo "========================================="

exit $EXIT_CODE

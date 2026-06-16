#!/usr/bin/env bash
set -euo pipefail

# 宿主机执行 Shell 脚本
# 用法: host-shell.sh <脚本路径> <脚本参数>
# 示例: host-shell.sh /opt/scripts/deploy.sh "--env prod --force"

SCRIPT_PATH="${1:-}"
SCRIPT_ARGS="${2:-}"

# 参数校验
if [[ -z "$SCRIPT_PATH" ]]; then
  echo "错误: 未指定脚本路径" >&2
  echo "用法: $0 <脚本路径> <脚本参数>" >&2
  exit 1
fi

# 检查脚本文件是否存在
if [[ ! -f "$SCRIPT_PATH" ]]; then
  echo "错误: 脚本文件不存在: $SCRIPT_PATH" >&2
  exit 1
fi

# 检查脚本是否可执行，若不可执行则尝试用 bash 执行
if [[ ! -x "$SCRIPT_PATH" ]]; then
  echo "提示: 脚本不可执行，将使用 bash 执行"
  echo ""
  echo "========================================="
  echo "宿主机 Shell 脚本执行"
  echo "========================================="
  echo "脚本: $SCRIPT_PATH"
  echo "参数: ${SCRIPT_ARGS:-(无)}"
  echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
  echo "========================================="
  echo ""

  bash "$SCRIPT_PATH" $SCRIPT_ARGS
else
  echo "========================================="
  echo "宿主机 Shell 脚本执行"
  echo "========================================="
  echo "脚本: $SCRIPT_PATH"
  echo "参数: ${SCRIPT_ARGS:-(无)}"
  echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
  echo "========================================="
  echo ""

  "$SCRIPT_PATH" $SCRIPT_ARGS
fi

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

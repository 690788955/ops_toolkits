#!/usr/bin/env bash
set -uo pipefail

# k3s 内置 etcd 快照维护。最后一行输出 JSON。
# 用法: etcd-snapshot.sh <list|save|prune> [快照名称] [保留数量]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/common.sh
source "$SCRIPT_DIR/lib/common.sh"

ACTION="${1:-list}"
NAME="${2:-}"
KEEP="${3:-5}"

if ! has_command k3s; then
  echo "错误: 未找到 k3s 命令，etcd 快照需要直接在 k3s server 节点执行" >&2
  exit 1
fi

case "$ACTION" in
  list)
    echo "etcd 快照列表:"
    k3s etcd-snapshot ls
    ;;
  save)
    SAVE_ARGS=(etcd-snapshot save)
    if [[ -n "$NAME" ]]; then
      SAVE_ARGS+=(--name "$NAME")
    fi
    echo "保存 etcd 快照${NAME:+: $NAME}..."
    k3s "${SAVE_ARGS[@]}"
    ;;
  prune)
    if ! [[ "$KEEP" =~ ^[0-9]+$ ]] || [[ "$KEEP" -lt 1 ]]; then
      echo "错误: keep 必须是大于 0 的整数，当前值: $KEEP" >&2
      exit 1
    fi
    echo "清理 etcd 快照，保留最近 $KEEP 份..."
    k3s etcd-snapshot prune --snapshot-retention "$KEEP"
    ;;
  *)
    echo "错误: 不支持的操作 $ACTION，可选 list/save/prune" >&2
    exit 1
    ;;
esac
EXIT_CODE=$?

if [[ "$EXIT_CODE" -ne 0 ]]; then
  echo "错误: etcd 快照操作失败（退出码 $EXIT_CODE），请确认当前节点为 server 且已启用 etcd 存储" >&2
  exit "$EXIT_CODE"
fi

printf '{"action":"%s","name":"%s","keep":"%s"}\n' \
  "$(json_escape "$ACTION")" \
  "$(json_escape "$NAME")" \
  "$(json_escape "$KEEP")"

exit 0
#!/usr/bin/env bash
set -euo pipefail

# 校验 md5.txt 中列出的分片文件，并按清单顺序合并为完整包，可选解压。
# 用法: verify-md5-and-merge.sh <上传文件路径或上传目录路径> [输出文件名] [yes|no]

INPUT_PATH="${1:-}"
OUTPUT_NAME="${2:-}"
EXTRACT="${3:-}"

if [[ -z "$INPUT_PATH" ]]; then
  echo "错误: 未指定上传文件路径或目录路径" >&2
  echo "用法: $0 <上传文件路径或上传目录路径> [输出文件名]" >&2
  exit 1
fi

if ! command -v md5sum >/dev/null 2>&1; then
  echo "错误: md5sum 命令未找到，请确认运行环境已安装 coreutils" >&2
  exit 1
fi

if [[ -d "$INPUT_PATH" ]]; then
  PACKAGE_DIR="$INPUT_PATH"
elif [[ -f "$INPUT_PATH" ]]; then
  PACKAGE_DIR="$(cd "$(dirname "$INPUT_PATH")" && pwd)"
else
  echo "错误: 路径不存在或不是普通文件/目录: $INPUT_PATH" >&2
  exit 1
fi

MD5_FILE="$PACKAGE_DIR/md5.txt"
if [[ ! -f "$MD5_FILE" ]]; then
  echo "错误: 未找到 MD5 清单文件: $MD5_FILE" >&2
  exit 1
fi

mapfile -t PART_FILES < <(
  awk '
    /^[[:space:]]*$/ || /^[[:space:]]*#/ { next }
    {
      checksum=$1
      $1=""
      sub(/^[[:space:]]+/, "", $0)
      sub(/^\*/, "", $0)
      if (checksum ~ /^[0-9a-fA-F]{32}$/ && $0 != "") {
        print $0
      }
    }
  ' "$MD5_FILE"
)

if [[ ${#PART_FILES[@]} -eq 0 ]]; then
  echo "错误: md5.txt 中未找到有效的 MD5 分片记录" >&2
  exit 1
fi

if [[ -z "$OUTPUT_NAME" ]]; then
  first_part="${PART_FILES[0]}"
  OUTPUT_NAME="$(printf '%s' "$first_part" | sed -E 's/\.[0-9]+$//')"
fi

if [[ -z "$OUTPUT_NAME" || "$OUTPUT_NAME" == */* || "$OUTPUT_NAME" == *\\* ]]; then
  echo "错误: 输出文件名不合法: $OUTPUT_NAME" >&2
  exit 1
fi

for part in "${PART_FILES[@]}"; do
  if [[ "$OUTPUT_NAME" == "$part" ]]; then
    echo "错误: 输出文件名不能和分片文件同名: $OUTPUT_NAME" >&2
    exit 1
  fi
done

OUTPUT_PATH="$PACKAGE_DIR/$OUTPUT_NAME"

echo "========================================="
echo "分片包 MD5 校验与合并"
echo "========================================="
echo "目录: $PACKAGE_DIR"
echo "清单: $MD5_FILE"
echo "输出: $OUTPUT_PATH"
echo "分片数: ${#PART_FILES[@]}"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================="
echo ""

echo "开始 MD5 校验..."
(
  cd "$PACKAGE_DIR"
  md5sum -c "md5.txt"
)
echo "MD5 校验通过。"
echo ""

echo "开始合并分片..."
tmp_output="${OUTPUT_PATH}.tmp.$$"
rm -f "$tmp_output"

for part in "${PART_FILES[@]}"; do
  part_path="$PACKAGE_DIR/$part"
  if [[ ! -f "$part_path" ]]; then
    echo "错误: 分片文件不存在: $part_path" >&2
    rm -f "$tmp_output"
    exit 1
  fi
  cat "$part_path" >> "$tmp_output"
done

mv -f "$tmp_output" "$OUTPUT_PATH"

echo ""
echo "========================================="
echo "合并完成"
echo "输出文件: $OUTPUT_PATH"
echo "输出大小: $(wc -c < "$OUTPUT_PATH") bytes"
echo "========================================="

# 解压逻辑
if [[ "${EXTRACT,,}" == "yes" ]]; then
  echo ""
  echo "开始解压 $OUTPUT_PATH ..."
  EXTRACT_DIR="$PACKAGE_DIR"
  tar xzf "$OUTPUT_PATH" -C "$EXTRACT_DIR"
  echo "解压完成，目录: $EXTRACT_DIR"
  echo "========================================="
  echo "全部完成"
  echo "合并文件: $OUTPUT_PATH"
  echo "解压目录: $EXTRACT_DIR"
  echo "========================================="
fi

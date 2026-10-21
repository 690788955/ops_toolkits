#!/usr/bin/env bash
# ============================================================================
# ops_toolkits 一键构建脚本（前端 + 后端）
#
# 流程：检查工具链 -> 构建 Web UI（Vite，产物内嵌进 Go 二进制）-> 编译 Go
#       ->（可选）单元测试 ->（可选）生成交付包
#
# 用法：
#   ./build.sh                      # 构建当前平台，产物 bin/opsctl[.exe]
#   ./build.sh --targets all        # 构建 CI 全部平台（linux/arm64 等 6 个目标）
#   ./build.sh --targets linux/amd64,linux/arm64
#   ./build.sh --skip-web           # 跳过前端，直接编译 Go（复用上次前端产物）
#   ./build.sh --test               # 构建前执行 go test ./...
#   ./build.sh --package            # 构建后调用 opsctl package build 生成交付包
#   ./build.sh --clean              # 构建前清空 bin/ 下的旧产物
#   ./build.sh --out dist/bin       # 自定义输出目录
#
# 说明：
#   - 支持 Git Bash / MSYS2 / Linux / macOS，Windows 目标自动追加 .exe 后缀。
#   - 产物命名与 CI 一致：本机目标为 opsctl[.exe]，交叉目标为 opsctl_<goos>_<goarch>[.exe]。
# ============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# --- 默认参数 ---------------------------------------------------------------
TARGETS=""                 # 空 = 仅当前平台；all = CI 全平台；也可显式列出
OUT_DIR="bin"
WEB_DIR="web"
EMBED_DIR="internal/server/web"
BUILD_WEB=1
RUN_TESTS=0
DO_PACKAGE=0
DO_CLEAN=0
ALL_TARGETS="linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64"

# --- 输出辅助 ---------------------------------------------------------------
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'; C_GREEN=$'\033[32m'
  C_YELLOW=$'\033[33m'; C_RED=$'\033[31m'; C_CYAN=$'\033[36m'
else
  C_RESET=""; C_BOLD=""; C_GREEN=""; C_YELLOW=""; C_RED=""; C_CYAN=""
fi

STEP=0
step()  { STEP=$((STEP + 1)); printf '\n%s[%d] %s%s\n' "$C_BOLD$C_CYAN" "$STEP" "$*" "$C_RESET"; }
info()  { printf '    %s\n' "$*"; }
ok()    { printf '    %s%s%s\n' "$C_GREEN" "$*" "$C_RESET"; }
warn()  { printf '    %s警告：%s%s\n' "$C_YELLOW" "$*" "$C_RESET" >&2; }
die()   { printf '\n%s错误：%s%s\n' "$C_RED" "$*" "$C_RESET" >&2; exit 1; }

usage() { awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "${BASH_SOURCE[0]}"; }

# --- 参数解析 ---------------------------------------------------------------
while [ $# -gt 0 ]; do
  case "$1" in
    --targets)   [ $# -ge 2 ] || die "--targets 需要一个参数"; TARGETS="$2"; shift 2 ;;
    --targets=*) TARGETS="${1#*=}"; shift ;;
    --out)       [ $# -ge 2 ] || die "--out 需要一个参数"; OUT_DIR="$2"; shift 2 ;;
    --out=*)     OUT_DIR="${1#*=}"; shift ;;
    --skip-web)  BUILD_WEB=0; shift ;;
    --test)      RUN_TESTS=1; shift ;;
    --package)   DO_PACKAGE=1; shift ;;
    --clean)     DO_CLEAN=1; shift ;;
    -h|--help)   usage; exit 0 ;;
    *)           die "未知参数：$1（使用 --help 查看用法）" ;;
  esac
done

# 目标列表归一化：逗号或空格分隔
TARGETS="${TARGETS//,/ }"
if [ -z "${TARGETS// /}" ]; then
  TARGETS="$(go env GOOS 2>/dev/null || echo "")/$(go env GOARCH 2>/dev/null || echo "")"
  [ "$TARGETS" = "/" ] && die "无法检测当前平台，请显式指定 --targets"
elif [ "$TARGETS" = "all" ]; then
  TARGETS="$ALL_TARGETS"
elif [ "$TARGETS" = "local" ]; then
  TARGETS="$(go env GOOS)/$(go env GOARCH)"
fi

# --- 环境准备 ---------------------------------------------------------------
for target in $TARGETS; do
  case "$target" in
    */*) [ -n "${target%%/*}" ] && [ -n "${target#*/}" ] \
           || die "目标格式错误：$target（应为 os/arch，例如 linux/amd64）" ;;
    *)   die "目标格式错误：$target（应为 os/arch，例如 linux/amd64）" ;;
  esac
done

HOST_OS="$(uname -s)"
case "$HOST_OS" in
  MINGW*|MSYS*|CYGWIN*) IS_WINDOWS_HOST=1 ;;
  *)                    IS_WINDOWS_HOST=0 ;;
esac

export CGO_ENABLED=0
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
NPM_BIN="${NPM_BIN:-$(command -v npm || command -v npm.cmd || true)}"
START_TS=$SECONDS

printf '%sops_toolkits 构建%s  目标：%s  输出：%s/\n' "$C_BOLD" "$C_RESET" "$TARGETS" "$OUT_DIR"

step "检查工具链"
command -v go >/dev/null 2>&1 || die "未找到 go，请先安装 Go 1.21+"
info "go      $(go version | awk '{print $3}')  ($(go env GOOS)/$(go env GOARCH))"
if [ "$BUILD_WEB" = "1" ]; then
  [ -n "$NPM_BIN" ] || die "未找到 npm，请先安装 Node.js 18+（或加 --skip-web 跳过前端）"
  info "node    $(node --version 2>/dev/null || echo '未知')"
  info "npm     $("$NPM_BIN" --version 2>/dev/null || echo '未知')"
fi
ok "工具链就绪"

if [ "$DO_CLEAN" = "1" ]; then
  step "清理旧产物"
  if [ -d "$OUT_DIR" ]; then
    find "$OUT_DIR" -maxdepth 1 -type f \( -name 'opsctl' -o -name 'opsctl.exe' -o -name 'opsctl_*' \) -print -delete | sed 's/^/    已删除 /'
  fi
  ok "清理完成"
fi

mkdir -p "$OUT_DIR"

# --- 前端构建 ---------------------------------------------------------------
if [ "$BUILD_WEB" = "1" ]; then
  step "构建 Web UI（$WEB_DIR/ -> $EMBED_DIR/）"
  [ -f "$WEB_DIR/package.json" ] || die "未找到 $WEB_DIR/package.json"
  if [ ! -d "$WEB_DIR/node_modules" ]; then
    info "首次构建，安装前端依赖（可能需要数分钟）..."
    if [ -f "$WEB_DIR/package-lock.json" ]; then
      ( cd "$WEB_DIR" && "$NPM_BIN" ci --no-audit --no-fund )
    else
      ( cd "$WEB_DIR" && "$NPM_BIN" install --no-audit --no-fund )
    fi
  else
    info "复用已有 node_modules（如需更新依赖请删除该目录）"
  fi
  info "执行 vite build ..."
  ( cd "$WEB_DIR" && "$NPM_BIN" run build )
  [ -f "$EMBED_DIR/index.html" ] || die "前端构建完成但未找到 $EMBED_DIR/index.html，请检查 vite.config.js 的 outDir"
  ok "前端产物就绪（$(find "$EMBED_DIR/assets" -type f 2>/dev/null | wc -l | tr -d ' ') 个资源文件）"
else
  step "跳过前端构建"
  [ -f "$EMBED_DIR/index.html" ] || die "内嵌资源不存在（$EMBED_DIR/index.html），无法编译；请去掉 --skip-web 重新构建"
  warn "使用已有前端产物编译，确认其与源码一致"
fi

# --- 后端编译 ---------------------------------------------------------------
step "编译 Go 二进制"
HOST_GOOS="$(go env GOOS)"
HOST_GOARCH="$(go env GOARCH)"
BUILT_LOCAL=""

for target in $TARGETS; do
  goos="${target%%/*}"
  goarch="${target#*/}"
  [ -n "$goos" ] && [ -n "$goarch" ] && [ "$goos" != "$target" ] \
    || die "目标格式错误：$target（应为 os/arch，例如 linux/amd64）"

  ext=""
  [ "$goos" = "windows" ] && ext=".exe"

  if [ "$goos" = "$HOST_GOOS" ] && [ "$goarch" = "$HOST_GOARCH" ]; then
    name="opsctl${ext}"
    BUILT_LOCAL="$OUT_DIR/$name"
  else
    name="opsctl_${goos}_${goarch}${ext}"
  fi

  info "-> ${goos}/${goarch}"
  GOOS="$goos" GOARCH="$goarch" go build -trimpath -o "$OUT_DIR/$name" ./cmd/opsctl
  size="$(wc -c < "$OUT_DIR/$name" | tr -d ' ')"
  ok "$OUT_DIR/$name  ($((size / 1024 / 1024)) MB)"
done

# --- 单元测试（可选） -------------------------------------------------------
if [ "$RUN_TESTS" = "1" ]; then
  step "执行单元测试"
  if [ "$IS_WINDOWS_HOST" = "1" ]; then
    warn "Windows 下 internal/server 的符号链接类用例（TestPluginExportRejectsSymlink 等）会失败，属平台限制"
  fi
  go test ./...
  ok "测试通过"
fi

# --- 交付包（可选） ---------------------------------------------------------
if [ "$DO_PACKAGE" = "1" ]; then
  step "生成交付包（dist/）"
  [ -n "$BUILT_LOCAL" ] || die "--package 需要先构建当前平台目标（请把本机 os/arch 加入 --targets）"
  case "$BUILT_LOCAL" in
    *.exe) RUN_BIN="$BUILT_LOCAL" ;;
    *)     RUN_BIN="./$BUILT_LOCAL" ;;
  esac
  info "调用 $RUN_BIN package build ..."
  "$RUN_BIN" package build
  ok "交付包已生成，见 dist/"
fi

# --- 汇总 -------------------------------------------------------------------
printf '\n%s构建完成%s，耗时 %d 秒。产物：\n' "$C_GREEN$C_BOLD" "$C_RESET" "$((SECONDS - START_TS))"
for target in $TARGETS; do
  goos="${target%%/*}"; goarch="${target#*/}"; ext=""
  [ "$goos" = "windows" ] && ext=".exe"
  if [ "$goos" = "$HOST_GOOS" ] && [ "$goarch" = "$HOST_GOARCH" ]; then
    printf '  %s/opsctl%s\n' "$OUT_DIR" "$ext"
  else
    printf '  %s/opsctl_%s_%s%s\n' "$OUT_DIR" "$goos" "$goarch" "$ext"
  fi
done
if [ -n "$BUILT_LOCAL" ]; then
  printf '\n提示：本机可直接运行  %s serve --port 8080\n' "$BUILT_LOCAL"
fi

package runner

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"shell_ops/internal/config"
)

func TestNativeCommandKeepsWindowsEnvironmentWhenBashUsesWSL(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows native command environment")
	}
	bashRuntimeMu.Lock()
	previous := bashRuntimeValue
	bashRuntimeValue = &bashRuntimeInfo{exe: `C:\Windows\System32\wsl.exe`, wsl: true, prefix: []string{"bash"}}
	bashRuntimeMu.Unlock()
	defer func() {
		bashRuntimeMu.Lock()
		bashRuntimeValue = previous
		bashRuntimeMu.Unlock()
	}()
	env := []string{`OPS_PARAM_FILE=E:\ops dir\params.yaml`, `CONFIG_PATH=C:\etc\app.conf`}
	cmd, err := buildCommand(context.Background(), "native-tool.exe", config.ToolConfig{}, nil, "", `E:\ops dir`, env)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Dir != `E:\ops dir` {
		t.Fatalf("working directory = %q", cmd.Dir)
	}
	for index, want := range env {
		if got := cmd.Env[len(cmd.Env)-len(env)+index]; got != want {
			t.Fatalf("native command environment = %q, want %q", got, want)
		}
	}
}

func TestGitBashCandidatesCoverBinAndUsrBin(t *testing.T) {
	root := filepath.Join(string(filepath.Separator)+"GitInstall", "Git")
	candidates := gitBashCandidates([]string{root})
	want := []string{
		filepath.Join(root, "bin", "bash.exe"),
		filepath.Join(root, "usr", "bin", "bash.exe"),
		filepath.Join(root, "bash.exe"),
	}
	for index, path := range want {
		if candidates[index] != path {
			t.Fatalf("gitBashCandidates()[%d] = %q, want %q", index, candidates[index], path)
		}
	}
}

func TestBashCandidatesDeduplicateAndKeepOrder(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("候选路径按 Windows 布局生成")
	}
	candidates := bashCandidates()
	if len(candidates) == 0 {
		t.Fatal("bashCandidates() 不应为空：至少应包含 PATH 与 Git 常见目录")
	}
	seen := map[string]bool{}
	for _, path := range candidates {
		key := strings.ToLower(path)
		if seen[key] {
			t.Fatalf("候选路径重复: %s", path)
		}
		seen[key] = true
	}
}

func TestLooksLikeWindowsAbsPath(t *testing.T) {
	for _, value := range []string{`C:\ops\app.yaml`, `E:/桌面/run.sh`, `d:\tmp\x`} {
		if !looksLikeWindowsAbsPath(value) {
			t.Fatalf("looksLikeWindowsAbsPath(%q) = false, want true", value)
		}
	}
	for _, value := range []string{"", "app.yaml", "/etc/ops.yaml", "C:relative", "1:\\x", "C:x"} {
		if looksLikeWindowsAbsPath(value) {
			t.Fatalf("looksLikeWindowsAbsPath(%q) = true, want false", value)
		}
	}
}

func TestDetectBashIsStableAndReportsPath(t *testing.T) {
	path, wsl, err := DetectBash()
	first, firstWSL, firstErr := DetectBash()
	if path != first || wsl != firstWSL || (err == nil) != (firstErr == nil) {
		t.Fatalf("DetectBash() 两次结果不一致: %q/%v/%v vs %q/%v/%v", path, wsl, err, first, firstWSL, firstErr)
	}
	if err == nil {
		if strings.TrimSpace(path) == "" {
			t.Fatal("DetectBash() 成功时路径不能为空")
		}
		return
	}
	if !strings.Contains(err.Error(), "bash") {
		t.Fatalf("缺少 bash 时的错误应说明原因: %v", err)
	}
}

// Git Bash 与 WSL 对 Windows 绝对路径的写法不同，转换函数必须各自正确，
// 否则插件脚本拿到 C:\... 时会把反斜杠当转义字符。
func TestBashPathForRespectsShellFlavor(t *testing.T) {
	if runtime.GOOS != "windows" {
		if got := bashPathFor("/srv/app"); got != "/srv/app" {
			t.Fatalf("bashPathFor() = %q, 非 Windows 平台应原样返回", got)
		}
		return
	}
	got := bashPathFor(`E:\ops dir\app.conf`)
	if isWSLBash() {
		if got != "/mnt/e/ops dir/app.conf" {
			t.Fatalf("WSL 路径 = %q", got)
		}
		return
	}
	if got != "E:/ops dir/app.conf" {
		t.Fatalf("Git Bash 路径 = %q, want E:/ops dir/app.conf", got)
	}
}

func TestNormalizeEnvForBashKeepsNonPathValues(t *testing.T) {
	env := []string{
		`PLAIN=hello`,
		`PATTERN=^C:\\temp\\x`,
		`OPS_PARAM_FILE=E:\ops\params.yaml`,
	}
	got := normalizeEnvForBash(env)
	if runtime.GOOS != "windows" {
		for index, item := range env {
			if got[index] != item {
				t.Fatalf("非 Windows 平台不应改写环境变量: %q -> %q", item, got[index])
			}
		}
		return
	}
	if got[0] != "PLAIN=hello" || got[1] != `PATTERN=^C:\\temp\\x` {
		t.Fatalf("非路径值被改写: %#v", got)
	}
	if !strings.HasPrefix(got[2], "OPS_PARAM_FILE=") || strings.Contains(got[2], `\`) {
		t.Fatalf("路径型环境变量应换成 bash 写法: %q", got[2])
	}
}

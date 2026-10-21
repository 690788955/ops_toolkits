package remoteexec

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"shell_ops/internal/runner"
	"shell_ops/internal/sshmgr"
)

const (
	maxBundleBytes = 64 << 20
	toolBundleDir  = "tool"
	paramBundleDir = "params"
	globalEnvName  = "global-env.conf"
)

type Executor struct {
	ssh *sshmgr.Manager
}

func New(manager *sshmgr.Manager) *Executor {
	return &Executor{ssh: manager}
}

func (e *Executor) DescribeHost(hostID string) (runner.RemoteHost, error) {
	if e == nil || e.ssh == nil {
		return runner.RemoteHost{}, fmt.Errorf("远程执行未配置 SSH 操作机管理器")
	}
	host, err := e.ssh.HostByID(hostID)
	if err != nil {
		return runner.RemoteHost{}, err
	}
	if !host.IsEnabled() {
		return runner.RemoteHost{}, fmt.Errorf("操作机 %s 已停用", host.Name)
	}
	return runner.RemoteHost{ID: host.ID, Name: host.Name}, nil
}

func (e *Executor) RunTool(ctx context.Context, req runner.RemoteToolRequest, stdout, stderr io.Writer) error {
	if e == nil || e.ssh == nil {
		return fmt.Errorf("远程执行未配置 SSH 操作机管理器")
	}
	bundle, err := buildBundle(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = bundle.Close()
		_ = os.Remove(bundle.Name())
	}()
	host, err := e.ssh.HostByID(req.HostID)
	if err != nil {
		return err
	}
	result, err := e.ssh.StreamExec(ctx, req.HostID, remoteCommand(host.BundleBaseDir, host.KeepBundle), bundle, stdout, stderr)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("远程工具执行失败（退出码 %d）", result.ExitCode)
	}
	return nil
}

func remoteCommand(bundleBaseDir string, keepBundle bool) string {
	// 外层包装只使用 POSIX 语法，目标机即使没有 GNU Bash 也能解包并启动。
	base := `"${TMPDIR:-/tmp}"`
	if baseDir := strings.TrimSpace(bundleBaseDir); baseDir != "" {
		base = shellQuote(baseDir)
	}
	script := `base=` + base + `
mkdir -p "$base" || exit 1
d=$(mktemp -d "$base/opsctl.XXXXXX") || exit 1
`
	if keepBundle {
		script += `echo "opsctl: 下发文件保留在 $d" >&2
`
	} else {
		script += `trap 'rm -rf "$d"' EXIT
`
	}
	script += `tar -xzf - -C "$d" || exit 1
/bin/sh "$d/run.sh"`
	return "/bin/sh -c " + shellQuote(script)
}

func buildBundle(req runner.RemoteToolRequest) (*os.File, error) {
	toolDir, err := filepath.Abs(req.ToolDir)
	if err != nil {
		return nil, fmt.Errorf("解析工具目录失败: %w", err)
	}
	info, err := os.Stat(toolDir)
	if err != nil {
		return nil, fmt.Errorf("读取工具目录 %s 失败: %w", req.ToolDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("工具目录不是目录: %s", req.ToolDir)
	}
	workdir, err := remoteWorkdir(toolDir, req.Workdir)
	if err != nil {
		return nil, err
	}
	entry, entryInBundle, err := remoteEntry(toolDir, req.Entry)
	if err != nil {
		return nil, err
	}
	paramName := ""
	if req.ParamFile != "" {
		paramName = filepath.Base(req.ParamName)
		if paramName == "" || paramName == "." {
			paramName = filepath.Base(req.ParamFile)
		}
		paramName = path.Base(strings.ReplaceAll(paramName, `\`, "/"))
		if paramName == "" || paramName == "." || paramName == "/" {
			return nil, fmt.Errorf("参数文件名无效")
		}
	}
	shellKind := ""
	if strings.EqualFold(filepath.Ext(req.Entry), ".sh") {
		localEntry := req.Entry
		if !filepath.IsAbs(localEntry) {
			localEntry = filepath.Join(toolDir, localEntry)
		}
		shellKind = scriptShellKind(localEntry)
	}
	script := renderRunScript(req, workdir, entry, entryInBundle, shellKind, paramName)

	file, err := os.CreateTemp("", "opsctl-exec-*.tar.gz")
	if err != nil {
		return nil, fmt.Errorf("创建执行包失败: %w", err)
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}
	_ = file.Chmod(0o600)
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	writeErr := addTarBytes(tw, "run.sh", []byte(script), 0o755)
	if writeErr == nil && paramName != "" {
		writeErr = addTarFile(tw, path.Join(paramBundleDir, paramName), req.ParamFile, 0o600)
	}
	if writeErr == nil && req.GlobalEnvFile != "" {
		writeErr = addGlobalEnv(tw, req.GlobalEnvFile)
	}
	if writeErr == nil {
		writeErr = addToolTree(tw, toolDir)
	}
	if closeErr := tw.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if closeErr := gz.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		cleanup()
		return nil, writeErr
	}
	stat, err := file.Stat()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("读取执行包大小失败: %w", err)
	}
	if stat.Size() > maxBundleBytes {
		cleanup()
		return nil, fmt.Errorf("执行包大小 %.1f MiB 超过上限 %d MiB", float64(stat.Size())/(1<<20), maxBundleBytes>>20)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, fmt.Errorf("读取执行包失败: %w", err)
	}
	return file, nil
}

// run.sh 本体也必须是 POSIX 语法：目标机的 sh 可能是 dash/BusyBox。
const bashPrelude = `OPS_BASH=""
for candidate in bash /bin/bash /usr/bin/bash /usr/local/bin/bash /opt/bin/bash /opt/homebrew/bin/bash; do
  if "$candidate" -c 'set -o pipefail' >/dev/null 2>&1; then
    OPS_BASH="$candidate"
    break
  fi
done
if [ -z "$OPS_BASH" ]; then
  echo 'opsctl: 目标机未找到可用的 GNU Bash（支持 pipefail），无法执行该工具的 bash 脚本。请安装 bash，或将脚本改写为 POSIX sh。' >&2
  exit 127
fi
`

func renderRunScript(req runner.RemoteToolRequest, workdir, entry string, entryInBundle bool, shellKind string, paramName string) string {
	var builder strings.Builder
	builder.WriteString("#!/bin/sh\n")
	builder.WriteString("set -e\n")
	builder.WriteString("BASE=\"$(cd \"$(dirname \"$0\")\" && pwd)\"\n")
	builder.WriteString("cd " + workdir + "\n")
	for _, item := range req.GlobalEnv {
		key, value, ok := strings.Cut(item, "=")
		if !ok || !isEnvName(key) {
			continue
		}
		builder.WriteString("export " + key + "=" + shellQuote(value) + "\n")
	}
	for _, item := range req.Env {
		key, value, ok := strings.Cut(item, "=")
		if !ok || !isEnvName(key) {
			continue
		}
		builder.WriteString("export " + key + "=" + shellQuote(value) + "\n")
	}
	if paramName != "" {
		builder.WriteString("export OPS_PARAM_FILE=\"$BASE\"/" + shellQuote(path.Join(paramBundleDir, paramName)) + "\n")
	}
	if req.GlobalEnvFile != "" {
		builder.WriteString("export OPS_GLOBAL_ENV_FILE=\"$BASE\"/" + shellQuote(globalEnvName) + "\n")
	}
	switch {
	case shellKind == "sh":
		builder.WriteString("exec /bin/sh " + entry)
	case shellKind == "bash":
		builder.WriteString(bashPrelude)
		builder.WriteString("exec \"$OPS_BASH\" " + entry)
	case entryInBundle:
		builder.WriteString("chmod +x " + entry + " 2>/dev/null || true\n")
		builder.WriteString("exec " + entry)
	default:
		builder.WriteString("exec " + entry)
	}
	for _, arg := range req.Args {
		builder.WriteString(" " + shellQuote(arg))
	}
	builder.WriteString("\n")
	return builder.String()
}

func scriptShellKind(localPath string) string {
	file, err := os.Open(localPath)
	if err != nil {
		return "bash"
	}
	defer file.Close()
	buffer := make([]byte, 256)
	count, _ := file.Read(buffer)
	line := string(buffer[:count])
	if index := strings.IndexByte(line, '\n'); index >= 0 {
		line = line[:index]
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "#!") {
		return "bash"
	}
	fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "#!")))
	if len(fields) == 0 {
		return "bash"
	}
	interpreter := path.Base(fields[0])
	if interpreter == "env" && len(fields) > 1 {
		interpreter = path.Base(fields[1])
	}
	switch interpreter {
	case "sh", "dash", "ash":
		return "sh"
	}
	return "bash"
}

func remoteWorkdir(toolDir, workdir string) (string, error) {
	if strings.TrimSpace(workdir) == "" {
		workdir = toolDir
	}
	if rel, ok := relativeToToolDir(toolDir, workdir); ok {
		return `"$BASE"/` + shellQuote(rel), nil
	}
	if hasWindowsVolume(workdir) {
		return "", fmt.Errorf("远程执行不支持 Windows 本机工作目录 %s，请把工作目录设置在工具目录内", workdir)
	}
	return shellQuote(filepath.ToSlash(workdir)), nil
}

func remoteEntry(toolDir, entry string) (string, bool, error) {
	if strings.TrimSpace(entry) == "" {
		return "", false, fmt.Errorf("工具未配置执行入口")
	}
	switch strings.ToLower(filepath.Ext(entry)) {
	case ".exe", ".bat", ".cmd", ".ps1":
		return "", false, fmt.Errorf("远程执行不支持 Windows 可执行文件 %s", entry)
	}
	if rel, ok := relativeToToolDir(toolDir, entry); ok {
		return `"$BASE"/` + shellQuote(rel), true, nil
	}
	if hasWindowsVolume(entry) {
		return "", false, fmt.Errorf("远程执行不支持 Windows 本机路径 %s", entry)
	}
	if filepath.IsAbs(entry) {
		return shellQuote(filepath.ToSlash(entry)), false, nil
	}
	return shellQuote(filepath.ToSlash(entry)), false, nil
}

func relativeToToolDir(toolDir, target string) (string, bool) {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(toolDir, absTarget)
	if err != nil {
		return "", false
	}
	if rel == "." {
		return toolBundleDir, true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return path.Join(toolBundleDir, filepath.ToSlash(rel)), true
}

func hasWindowsVolume(value string) bool {
	if len(value) >= 2 {
		letter := value[0]
		if (letter >= 'a' && letter <= 'z' || letter >= 'A' && letter <= 'Z') && value[1] == ':' {
			return true
		}
		if value[0] == '\\' && value[1] == '\\' {
			return true
		}
	}
	volume := filepath.VolumeName(value)
	return len(volume) == 2 && volume[1] == ':'
}

func addGlobalEnv(tw *tar.Writer, source string) error {
	data, err := os.ReadFile(source)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取全局环境配置 %s 失败: %w", source, err)
	}
	return addTarBytes(tw, globalEnvName, normalizeCRLF(data), 0o600)
}

func addToolTree(tw *tar.Writer, toolDir string) error {
	return filepath.WalkDir(toolDir, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("读取工具文件 %s 失败: %w", current, err)
		}
		if current == toolDir {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(toolDir, current)
		if err != nil {
			return err
		}
		name := path.Join(toolBundleDir, filepath.ToSlash(relative))
		if entry.IsDir() {
			return addTarDir(tw, name, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		mode := fs.FileMode(0o644)
		if info.Mode().Perm()&0o111 != 0 {
			mode = 0o755
		}
		return addTarFile(tw, name, current, mode)
	})
}

func addTarFile(tw *tar.Writer, name, source string, mode fs.FileMode) error {
	file, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("读取工具文件 %s 失败: %w", source, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if isTextScript(name, file) {
		data, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("读取工具文件 %s 失败: %w", source, err)
		}
		return addTarBytes(tw, name, normalizeCRLF(data), mode)
	}
	if err := addTarHeader(tw, name, info.Size(), mode); err != nil {
		return err
	}
	_, err = io.Copy(tw, file)
	return err
}

// Windows 工作区里的脚本常见 CRLF，远端 bash/sh 会把 \r 当成选项或参数的一部分。
func isTextScript(name string, file *os.File) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".sh", ".bash", ".zsh", ".ksh", ".dash", ".ash", ".py", ".pl", ".rb", ".conf", ".cfg", ".ini", ".env":
		return true
	}
	var head [2]byte
	count, _ := file.Read(head[:])
	_, _ = file.Seek(0, io.SeekStart)
	return count == 2 && head[0] == '#' && head[1] == '!'
}

func normalizeCRLF(data []byte) []byte {
	if !bytes.Contains(data, []byte("\r\n")) {
		return data
	}
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

func addTarBytes(tw *tar.Writer, name string, data []byte, mode fs.FileMode) error {
	if err := addTarHeader(tw, name, int64(len(data)), mode); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func addTarHeader(tw *tar.Writer, name string, size int64, mode fs.FileMode) error {
	header := &tar.Header{
		Name:    name,
		Mode:    int64(mode.Perm()),
		Size:    size,
		ModTime: time.Now(),
	}
	return tw.WriteHeader(header)
}

func addTarDir(tw *tar.Writer, name string, mode fs.FileMode) error {
	header := &tar.Header{
		Name:     strings.TrimSuffix(name, "/") + "/",
		Mode:     int64(mode.Perm()),
		Typeflag: tar.TypeDir,
		ModTime:  time.Now(),
	}
	return tw.WriteHeader(header)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func isEnvName(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

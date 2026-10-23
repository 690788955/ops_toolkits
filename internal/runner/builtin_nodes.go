package runner

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

const (
	defaultWaitInterval = 5 * time.Second
	defaultWaitMaxWait  = 5 * time.Minute
	waitStepMetaFile    = "wait.json"
	builtinNodeTimeout  = 30 * time.Minute
)

// builtinShellTool 保存独立脚本，复用 executeTool 对 .sh 的 WSL 路径转换和远程下发。
func builtinShellTool(nodeID, label, script, workdir, timeout, runDir, hostID string, env map[string]string) (*registry.Tool, error) {
	if hostID == "" && workdir == "" {
		var err error
		workdir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	if hostID == "" && isWSLBash() {
		workdir = windowsPathToWSLOrSlash(workdir)
	}
	if workdir != "" {
		script = "cd -- " + shellQuote(workdir) + " || exit $?\n" + script
	}
	toolDir := filepath.Join(runDir, ".builtin")
	if err := os.MkdirAll(toolDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(toolDir, "run.sh"), []byte("#!/usr/bin/env bash\n"+script+"\n"), 0o700); err != nil {
		return nil, err
	}
	cfg := &config.ToolConfig{
		ID:        "builtin." + nodeID,
		Name:      label,
		Execution: config.ExecutionConfig{Entry: "./run.sh", Timeout: timeout},
		Env:       env,
	}
	return &registry.Tool{Entry: config.ToolEntry{ID: cfg.ID, Category: "builtin"}, Config: cfg, Dir: toolDir}, nil
}

// executeShellNode 在节点目标机器上执行内联脚本。
func (r *Runner) executeShellNode(ctx context.Context, node config.WorkflowNode, workflowContext map[string]string, runDir, hostID string, out, errOut io.Writer) error {
	item, err := config.ValidateScriptConfig(node.ID, node.Script)
	if err != nil {
		return err
	}
	script := renderTemplate(item.Script, workflowContext)
	if item.FailOnExit != nil && !*item.FailOnExit {
		// 只忽略脚本本身的退出码，启动失败、超时和取消仍由 executeTool 返回。
		script = "(\n" + script + "\n)\nstatus=$?\nif [ \"$status\" -ne 0 ]; then printf '脚本退出码 %s（已配置继续）\\n' \"$status\" >&2; fi\nexit 0"
	}
	workdir := strings.TrimSpace(renderTemplate(item.Workdir, workflowContext))
	timeout := config.ParseTimeout(item.Timeout, config.ParseTimeout(node.Timeout, builtinNodeTimeout))
	env := map[string]string{}
	for name, value := range item.Env {
		env[name] = renderTemplate(value, workflowContext)
	}
	tool, err := builtinShellTool(node.ID, "Shell 节点", script, workdir, timeout.String(), runDir, hostID, env)
	if err != nil {
		return err
	}
	values := map[string]interface{}{}
	for name, value := range env {
		values[name] = value
	}
	return r.executeTool(ctx, tool, values, env, runDir, hostID, out, errOut)
}

// executeChecksumNode 计算文件摘要；配置了期望值时不一致即失败。
func (r *Runner) executeChecksumNode(ctx context.Context, node config.WorkflowNode, workflowContext map[string]string, runDir, hostID string, out, errOut io.Writer) error {
	item := node.Checksum
	item.Path = renderTemplate(item.Path, workflowContext)
	item.Expected = renderTemplate(item.Expected, workflowContext)
	if strings.Contains(item.Expected, "{{") {
		return fmt.Errorf("文件校验节点 %s 的期望摘要模板未解析", node.ID)
	}
	item, err := config.ValidateChecksumConfig(node.ID, item)
	if err != nil {
		return err
	}
	path, algorithm := item.Path, item.Algorithm
	executionPath := path
	if hostID == "" && isWSLBash() {
		executionPath = windowsPathToWSLOrSlash(path)
	}
	command := "sha256sum"
	helperName := "shasum"
	helper := "shasum -a 256"
	if algorithm == "md5" {
		command = "md5sum"
		helperName = "md5"
		helper = "md5 -r"
	}
	script := "set -euo pipefail\n" +
		"file=" + shellQuote(executionPath) + "\n" +
		"if [ ! -f \"$file\" ]; then echo \"文件不存在: $file\" >&2; exit 2; fi\n" +
		"if command -v " + strings.Fields(command)[0] + " >/dev/null 2>&1; then\n" +
		"  " + command + " \"$file\" | awk '{print $1}'\n" +
		"elif command -v " + helperName + " >/dev/null 2>&1; then\n" +
		"  " + helper + " \"$file\" | awk '{print $1}'\n" +
		"else\n" +
		"  echo '缺少 " + command + "/" + helperName + " 命令' >&2; exit 3\n" +
		"fi\n"
	tool, err := builtinShellTool(node.ID, "文件校验节点", script, "", config.ParseTimeout(node.Timeout, 5*time.Minute).String(), runDir, hostID, nil)
	if err != nil {
		return err
	}
	if err := r.executeTool(ctx, tool, nil, nil, runDir, hostID, out, errOut); err != nil {
		return err
	}
	raw := strings.TrimSpace(readTextFile(filepath.Join(runDir, "stdout.log")))
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return fmt.Errorf("文件校验节点 %s 没有输出摘要", node.ID)
	}
	actual := strings.ToLower(strings.TrimSpace(fields[len(fields)-1]))
	if _, err := config.ValidateChecksumConfig(node.ID, config.WorkflowChecksum{Path: path, Algorithm: algorithm, Expected: actual}); err != nil {
		return fmt.Errorf("文件校验节点 %s 输出了无效摘要: %w", node.ID, err)
	}
	expected := strings.ToLower(strings.TrimSpace(item.Expected))
	matched := expected == "" || expected == actual
	summary := map[string]interface{}{
		"path":      path,
		"algorithm": algorithm,
		"actual":    actual,
		"expected":  expected,
		"matched":   matched,
	}
	payload, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(filepath.Join(runDir, "checksum.json"), payload, 0o600); err != nil {
		return err
	}
	if !matched {
		return fmt.Errorf("文件校验节点 %s 的 %s 摘要不一致：期望 %s，实际 %s", node.ID, strings.ToUpper(algorithm), expected, actual)
	}
	return nil
}

// executeArchiveNode 解开 tar/tar.gz/zip；overwrite=false 时目标文件已存在会中止。
func (r *Runner) executeArchiveNode(ctx context.Context, node config.WorkflowNode, workflowContext map[string]string, runDir string) error {
	// 步骤输出常带换行，模板渲染后的路径要去掉首尾空白，否则格式识别和打开文件都会失败。
	item := node.Archive
	item.Path = renderTemplate(item.Path, workflowContext)
	item.TargetDir = renderTemplate(item.TargetDir, workflowContext)
	item, err := config.ValidateArchiveConfig(node.ID, item)
	if err != nil {
		return err
	}
	source, target := item.Path, item.TargetDir
	if err := ctx.Err(); err != nil {
		return err
	}
	format := item.Format
	if format == "" {
		format = detectArchiveFormat(source)
	}
	if mkErr := os.MkdirAll(target, 0o755); mkErr != nil {
		return fmt.Errorf("解压节点 %s 创建目标目录失败: %w", node.ID, mkErr)
	}
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return err
	}
	var (
		extracted  []string
		extractErr error
	)
	switch format {
	case "zip":
		extracted, extractErr = extractZip(ctx, source, target, item)
	case "tar":
		extracted, extractErr = extractTar(ctx, source, target, item, false)
	case "tar.gz", "tgz":
		extracted, extractErr = extractTar(ctx, source, target, item, true)
	default:
		return fmt.Errorf("解压节点 %s 无法识别压缩格式：%s", node.ID, source)
	}
	if extractErr != nil {
		return fmt.Errorf("解压节点 %s 失败: %w", node.ID, extractErr)
	}
	summary := map[string]interface{}{"source": source, "target_dir": target, "format": format, "files": extracted, "file_count": len(extracted)}
	payload, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(filepath.Join(runDir, "archive.json"), payload, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, "stdout.log"), []byte(strings.Join(extracted, "\n")), 0o600)
}

func detectArchiveFormat(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	default:
		return ""
	}
}

// archiveEntryEscape 判断压缩包条目是否用绝对路径或 .. 试图跳出目标目录。
// filepath.Clean 会吃掉 ..，所以必须在归一化之前检查原始名称。
func archiveEntryEscape(name string) bool {
	normalized := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(normalized, "/") || (len(normalized) > 1 && normalized[1] == ':') {
		return true
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func archiveTargetName(name string, strip int) (string, bool) {
	clean := filepath.ToSlash(filepath.Clean("/" + name))
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." {
		return "", false
	}
	parts := strings.Split(clean, "/")
	if strip >= len(parts) {
		return "", false
	}
	return filepath.Join(parts[strip:]...), true
}

func safeArchivePath(target, name string) (string, error) {
	base, baseErr := filepath.Abs(target)
	if baseErr != nil {
		return "", errors.New("无法解析解压路径")
	}
	// 允许用户选择的目标目录本身是别名，但拒绝包内路径经过已有符号链接。
	base, baseErr = filepath.EvalSymlinks(base)
	if baseErr != nil {
		return "", baseErr
	}
	full := filepath.Join(base, name)
	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("压缩包条目 %s 会逃逸目标目录", name)
	}
	relative, err := filepath.Rel(base, full)
	if err != nil {
		return "", err
	}
	current := base
	for _, part := range strings.Split(relative, string(os.PathSeparator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("压缩包条目 %s 经过已有符号链接，可能逃逸目标目录", name)
		}
	}
	return full, nil
}

// archiveContextReader 在复制大文件期间也响应取消，避免只在条目之间检查上下文。
type archiveContextReader struct {
	ctx context.Context
	src io.Reader
}

func (r archiveContextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.src.Read(data)
}

func writeArchiveFile(ctx context.Context, target, name string, src io.Reader, mode os.FileMode, overwrite bool) error {
	destination, err := safeArchivePath(target, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	// O_EXCL 保证 overwrite=false 的存在性检查与创建是同一个原子操作。
	flags := os.O_CREATE | os.O_WRONLY | os.O_EXCL
	if !overwrite {
		dst, err := os.OpenFile(destination, flags, mode.Perm()|0o600)
		if os.IsExist(err) {
			return fmt.Errorf("目标文件已存在: %s", name)
		}
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(dst, archiveContextReader{ctx: ctx, src: src})
		return errors.Join(copyErr, dst.Close())
	}
	// 先写临时文件再替换，避免截断已有硬链接指向的外部文件。
	dst, err := os.CreateTemp(filepath.Dir(destination), ".opsctl-extract-*")
	if err != nil {
		return err
	}
	defer os.Remove(dst.Name())
	if err := dst.Chmod(mode.Perm() | 0o600); err != nil {
		dst.Close()
		return err
	}
	_, copyErr := io.Copy(dst, archiveContextReader{ctx: ctx, src: src})
	if err := errors.Join(copyErr, dst.Close()); err != nil {
		return err
	}
	if _, err := safeArchivePath(target, name); err != nil {
		return err
	}
	return os.Rename(dst.Name(), destination)
}

func extractZip(ctx context.Context, source, target string, item config.WorkflowArchive) ([]string, error) {
	reader, err := zip.OpenReader(source)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	files := make([]string, 0, len(reader.File))
	for _, entry := range reader.File {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return files, ctxErr
		}
		if archiveEntryEscape(entry.Name) {
			return files, fmt.Errorf("压缩包条目 %s 会逃逸目标目录", entry.Name)
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return files, fmt.Errorf("压缩包条目 %s 是符号链接，不允许解压", entry.Name)
		}
		name, ok := archiveTargetName(entry.Name, item.Strip)
		if !ok {
			continue
		}
		destination, pathErr := safeArchivePath(target, name)
		if pathErr != nil {
			return files, pathErr
		}
		if entry.FileInfo().IsDir() {
			if mkErr := os.MkdirAll(destination, 0o755); mkErr != nil {
				return files, mkErr
			}
			continue
		}
		src, openErr := entry.Open()
		if openErr != nil {
			return files, openErr
		}
		writeErr := writeArchiveFile(ctx, target, name, src, entry.Mode(), item.Overwrite)
		if err := errors.Join(writeErr, src.Close()); err != nil {
			return files, err
		}
		files = append(files, filepath.ToSlash(name))
	}
	return files, nil
}

func extractTar(ctx context.Context, source, target string, item config.WorkflowArchive, gzipped bool) ([]string, error) {
	file, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var reader io.Reader = file
	if gzipped {
		gz, gzErr := gzip.NewReader(file)
		if gzErr != nil {
			return nil, gzErr
		}
		defer gz.Close()
		reader = gz
	}
	tarReader := tar.NewReader(reader)
	files := make([]string, 0)
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return files, ctxErr
		}
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return files, nextErr
		}
		if archiveEntryEscape(header.Name) {
			return files, fmt.Errorf("压缩包条目 %s 会逃逸目标目录", header.Name)
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return files, fmt.Errorf("压缩包条目 %s 是链接，不允许解压", header.Name)
		}
		name, ok := archiveTargetName(header.Name, item.Strip)
		if !ok {
			continue
		}
		destination, pathErr := safeArchivePath(target, name)
		if pathErr != nil {
			return files, pathErr
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if mkErr := os.MkdirAll(destination, 0o755); mkErr != nil {
				return files, mkErr
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := writeArchiveFile(ctx, target, name, tarReader, os.FileMode(header.Mode), item.Overwrite); err != nil {
				return files, err
			}
			files = append(files, filepath.ToSlash(name))
		}
	}
	return files, nil
}

// waitStepMeta 记录轮询节点的尝试次数与结果，便于失败后回溯。
type waitStepMeta struct {
	Mode      string `json:"mode"`
	Target    string `json:"target,omitempty"`
	Attempts  int    `json:"attempts"`
	Matched   bool   `json:"matched"`
	LastError string `json:"last_error,omitempty"`
	WaitedMS  int64  `json:"waited_ms"`
}

// executeWaitNode 执行固定等待或有限轮询。每次尝试都检查 ctx，取消可立即生效。
func (r *Runner) executeWaitNode(ctx context.Context, node config.WorkflowNode, workflowContext map[string]string, runDir string, out io.Writer) error {
	item := node.Wait
	item.PollURL = renderTemplate(item.PollURL, workflowContext)
	item.PollFile = renderTemplate(item.PollFile, workflowContext)
	item, err := config.ValidateWaitConfig(node.ID, item)
	if err != nil {
		return err
	}
	node.Wait = item
	meta := waitStepMeta{}
	started := time.Now()
	defer func() {
		meta.WaitedMS = time.Since(started).Milliseconds()
		writeWaitMeta(runDir, meta)
	}()
	if node.Wait.Seconds > 0 {
		meta.Mode = "sleep"
		timer := time.NewTimer(time.Duration(node.Wait.Seconds * float64(time.Second)))
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		meta.Matched = true
		if out != nil {
			fmt.Fprintf(out, "等待 %.1fs 完成\n", node.Wait.Seconds)
		}
		return nil
	}
	interval := defaultWaitInterval
	if node.Wait.Interval != "" {
		if parsed, err := time.ParseDuration(node.Wait.Interval); err == nil && parsed > 0 {
			interval = parsed
		}
	}
	maxWait := defaultWaitMaxWait
	if node.Wait.MaxWait != "" {
		if parsed, err := time.ParseDuration(node.Wait.MaxWait); err == nil && parsed > 0 {
			maxWait = parsed
		}
	}
	// HTTP 请求与轮询间隔共用真实截止时间，慢响应不能突破 max_wait。
	waitCtx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()
	maxAttempts := node.Wait.MaxAttempts
	modes := make([]string, 0, 2)
	if node.Wait.PollURL != "" {
		modes = append(modes, "http")
	}
	if node.Wait.PollFile != "" {
		modes = append(modes, "file")
	}
	meta.Mode = strings.Join(modes, "+")
	meta.Target = renderTemplate(node.Wait.PollURL+node.Wait.PollFile, workflowContext)
	client := &http.Client{Timeout: 30 * time.Second}
	for attempt := 1; maxAttempts == 0 || attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if waitCtx.Err() != nil {
			break
		}
		meta.Attempts = attempt
		matched, attemptErr := r.checkWaitCondition(waitCtx, node, workflowContext, client)
		// http.Client 超时返回的 net 错误不会包装 context.Canceled，先看上下文是否已结束。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if waitCtx.Err() != nil {
			break
		}
		if attemptErr != nil {
			meta.LastError = attemptErr.Error()
		} else if matched {
			meta.Matched = true
			meta.LastError = ""
			if out != nil {
				fmt.Fprintf(out, "轮询第 %d 次命中：%s\n", attempt, meta.Target)
			}
			return nil
		}
		if maxAttempts > 0 && attempt == maxAttempts {
			break
		}
		timer := time.NewTimer(interval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
		case <-timer.C:
		}
	}
	reason := meta.LastError
	if reason == "" {
		reason = "条件未在限定次数内满足"
	}
	return fmt.Errorf("等待节点 %s 轮询超时（%d 次）：%s", node.ID, meta.Attempts, reason)
}

func (r *Runner) checkWaitCondition(ctx context.Context, node config.WorkflowNode, workflowContext map[string]string, client *http.Client) (bool, error) {
	if node.Wait.PollFile != "" {
		path := strings.TrimSpace(node.Wait.PollFile)
		if _, err := os.Stat(path); err != nil {
			return false, err
		}
		return true, nil
	}
	target := strings.TrimSpace(node.Wait.PollURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, err
	}
	for name, value := range node.Wait.Headers {
		req.Header.Set(renderTemplate(name, workflowContext), renderTemplate(value, workflowContext))
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if len(node.Wait.ExpectStatus) == 0 {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return true, nil
		}
		return false, fmt.Errorf("状态码 %d", resp.StatusCode)
	}
	for _, code := range node.Wait.ExpectStatus {
		if resp.StatusCode == code {
			return true, nil
		}
	}
	return false, fmt.Errorf("状态码 %d 不在期望列表内", resp.StatusCode)
}

func writeWaitMeta(runDir string, meta waitStepMeta) {
	payload, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(runDir, waitStepMetaFile), payload, 0o600)
}

func readWaitMeta(path string) waitStepMeta {
	var meta waitStepMeta
	data, err := os.ReadFile(path)
	if err != nil {
		return meta
	}
	_ = json.Unmarshal(data, &meta)
	return meta
}

func addWaitContext(workflowContext map[string]string, nodeID string, runDir string) {
	meta := readWaitMeta(filepath.Join(runDir, waitStepMetaFile))
	prefix := "steps." + nodeID + ".wait."
	workflowContext[prefix+"matched"] = strconv.FormatBool(meta.Matched)
	workflowContext[prefix+"attempts"] = strconv.Itoa(meta.Attempts)
	workflowContext[prefix+"waited_ms"] = strconv.FormatInt(meta.WaitedMS, 10)
}

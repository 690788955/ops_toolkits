package runner

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"shell_ops/internal/config"
)

func TestWriteArchiveFileRejectsExistingSymlinks(t *testing.T) {
	for _, directory := range []bool{false, true} {
		name := "file"
		if directory {
			name = "directory"
		}
		t.Run(name, func(t *testing.T) {
			target, outside := t.TempDir(), t.TempDir()
			externalFile := filepath.Join(outside, "payload.txt")
			if err := os.WriteFile(externalFile, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			linkTarget, entry := externalFile, "linked"
			if directory {
				linkTarget, entry = outside, filepath.Join("linked", "payload.txt")
			}
			linkPath := filepath.Join(target, "linked")
			if err := os.Symlink(linkTarget, linkPath); err != nil {
				if !directory || runtime.GOOS != "windows" {
					t.Skipf("当前系统不能创建符号链接: %v", err)
				}
				// Junction 无需管理员权限，同样可能使包内父目录跳转到目标目录外。
				if output, junctionErr := exec.Command("cmd", "/c", "mklink", "/J", linkPath, linkTarget).CombinedOutput(); junctionErr != nil {
					t.Skipf("符号链接及 Junction 均不可用: symlink=%v junction=%v output=%s", err, junctionErr, output)
				}
				t.Log("使用 Windows Junction 验证已有父目录链接")
			}
			for _, overwrite := range []bool{false, true} {
				err := writeArchiveFile(context.Background(), target, entry, strings.NewReader("replacement"), 0o600, overwrite)
				if err == nil || !strings.Contains(err.Error(), "符号链接") {
					t.Fatalf("overwrite=%v: err=%v，已有符号链接应被拒绝", overwrite, err)
				}
			}
			if got := readFile(t, externalFile); got != "original" {
				t.Fatalf("目标目录外的文件被修改: %q", got)
			}
		})
	}
}

func TestArchiveExtractorsRejectLinkEntries(t *testing.T) {
	t.Run("zip_symlink", func(t *testing.T) {
		var data bytes.Buffer
		writer := zip.NewWriter(&data)
		header := &zip.FileHeader{Name: "linked"}
		header.SetMode(os.ModeSymlink | 0o777)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, "../outside"); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		archive := filepath.Join(t.TempDir(), "link.zip")
		if err := os.WriteFile(archive, data.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := extractZip(context.Background(), archive, t.TempDir(), config.WorkflowArchive{}); err == nil || !strings.Contains(err.Error(), "符号链接") {
			t.Fatalf("ZIP 符号链接未被拒绝: %v", err)
		}
	})
	for name, kind := range map[string]byte{"tar_symlink": tar.TypeSymlink, "tar_hardlink": tar.TypeLink} {
		t.Run(name, func(t *testing.T) {
			var data bytes.Buffer
			writer := tar.NewWriter(&data)
			if err := writer.WriteHeader(&tar.Header{Name: "linked", Typeflag: kind, Linkname: "../outside", Mode: 0o777}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(t.TempDir(), "link.tar")
			if err := os.WriteFile(archive, data.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := extractTar(context.Background(), archive, t.TempDir(), config.WorkflowArchive{}, false); err == nil || !strings.Contains(err.Error(), "链接") {
				t.Fatalf("TAR 链接未被拒绝: %v", err)
			}
		})
	}
}

func TestWriteArchiveFileHonorsOverwrite(t *testing.T) {
	target := t.TempDir()
	destination := filepath.Join(target, "payload.txt")
	if err := os.WriteFile(destination, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeArchiveFile(context.Background(), target, "payload.txt", strings.NewReader("replacement"), 0o600, false); err == nil {
		t.Fatal("overwrite=false 必须拒绝已有文件")
	}
	if got := readFile(t, destination); got != "original" {
		t.Fatalf("overwrite=false 修改了原文件: %q", got)
	}
	if err := writeArchiveFile(context.Background(), target, "payload.txt", strings.NewReader("replacement"), 0o600, true); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, destination); got != "replacement" {
		t.Fatalf("overwrite=true 未替换文件: %q", got)
	}
}

func TestWriteArchiveFileDoesNotTruncateExternalHardlink(t *testing.T) {
	target := t.TempDir()
	externalFile := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(externalFile, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(target, "payload.txt")
	if err := os.Link(externalFile, destination); err != nil {
		t.Skipf("当前文件系统不能创建硬链接: %v", err)
	}
	if err := writeArchiveFile(context.Background(), target, "payload.txt", strings.NewReader("replacement"), 0o600, true); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, externalFile); got != "original" {
		t.Fatalf("外部硬链接文件被截断: %q", got)
	}
	if got := readFile(t, destination); got != "replacement" {
		t.Fatalf("解压文件内容不正确: %q", got)
	}
}

type cancellingArchiveReader struct {
	cancel context.CancelFunc
	reads  int
}

func (r *cancellingArchiveReader) Read(data []byte) (int, error) {
	r.reads++
	if r.reads > 1 {
		return 0, errors.New("取消后不应继续读取归档内容")
	}
	r.cancel()
	return copy(data, "partial"), nil
}

func TestWriteArchiveFileCancellationPreservesExistingTarget(t *testing.T) {
	target := t.TempDir()
	destination := filepath.Join(target, "payload.txt")
	if err := os.WriteFile(destination, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &cancellingArchiveReader{cancel: cancel}
	err := writeArchiveFile(ctx, target, "payload.txt", source, 0o600, true)
	if !errors.Is(err, context.Canceled) || source.reads != 1 {
		t.Fatalf("复制未立即响应取消: err=%v reads=%d", err, source.reads)
	}
	if got := readFile(t, destination); got != "original" {
		t.Fatalf("取消后替换了原文件: %q", got)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "payload.txt" {
		t.Fatalf("取消后遗留临时文件: %v", entries)
	}
}

func TestExecuteWaitNodeMaxWaitInterruptsSlowHTTP(t *testing.T) {
	for _, slowBody := range []bool{false, true} {
		name := "headers"
		if slowBody {
			name = "body"
		}
		t.Run(name, func(t *testing.T) {
			var calls int32
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				atomic.AddInt32(&calls, 1)
				if slowBody {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				select {
				case <-req.Context().Done():
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)
			r, _ := newHTTPTestRunner(t)
			runDir := t.TempDir()
			node := config.WorkflowNode{ID: "poll", Wait: config.WorkflowWait{PollURL: server.URL, Interval: "10ms", MaxWait: "150ms"}}
			started := time.Now()
			err := r.executeWaitNode(context.Background(), node, nil, runDir, io.Discard)
			if err == nil || !strings.Contains(err.Error(), "轮询超时") {
				t.Fatalf("慢 HTTP 请求应受 max_wait 限制: %v", err)
			}
			if elapsed := time.Since(started); elapsed > 2*time.Second {
				t.Fatalf("慢响应突破 max_wait: %v", elapsed)
			}
			meta := readWaitMeta(filepath.Join(runDir, waitStepMetaFile))
			if meta.Matched || meta.Attempts != 1 || atomic.LoadInt32(&calls) != 1 {
				t.Fatalf("超时轮询记录不正确: %+v calls=%d", meta, calls)
			}
		})
	}
}

func TestExecuteWaitNodeStopsAtMaxAttempts(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	r, _ := newHTTPTestRunner(t)
	runDir := t.TempDir()
	node := config.WorkflowNode{ID: "poll", Wait: config.WorkflowWait{PollURL: server.URL, Interval: "1ms", MaxWait: "10s", MaxAttempts: 3}}
	if err := r.executeWaitNode(context.Background(), node, nil, runDir, io.Discard); err == nil {
		t.Fatal("达到最大尝试次数后应失败")
	}
	meta := readWaitMeta(filepath.Join(runDir, waitStepMetaFile))
	if got := atomic.LoadInt32(&calls); got != 3 || meta.Attempts != 3 || meta.Matched || !strings.Contains(meta.LastError, "503") {
		t.Fatalf("最大尝试次数未生效: calls=%d meta=%+v", got, meta)
	}
}

func TestExecuteWaitNodeCancellationInterruptsHTTP(t *testing.T) {
	started, release := make(chan struct{}, 1), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		started <- struct{}{}
		select {
		case <-req.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	r, _ := newHTTPTestRunner(t)
	runDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node := config.WorkflowNode{ID: "poll", Wait: config.WorkflowWait{PollURL: server.URL, MaxWait: "10s"}}
	result := make(chan error, 1)
	go func() { result <- r.executeWaitNode(ctx, node, nil, runDir, io.Discard) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("等待节点未发出 HTTP 请求")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("HTTP 取消错误未保留: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消未终止正在进行的 HTTP 请求")
	}
	if meta := readWaitMeta(filepath.Join(runDir, waitStepMetaFile)); meta.Matched || meta.Attempts != 1 {
		t.Fatalf("取消轮询记录不正确: %+v", meta)
	}
}

func TestExecuteShellNodeUsesRenderedEnvironmentAndWorkdir(t *testing.T) {
	r, dir := newHTTPTestRunner(t)
	workdir := filepath.Join(dir, "work dir's")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatal(err)
	}
	failOnExit := false
	node := config.WorkflowNode{ID: "script", Script: config.WorkflowScript{
		Script:  "printf '%s|%s' \"$MODE\" \"$OPS_PARAM_MODE\" > result.txt\npwd\nexit 7",
		Workdir: "{{ .workdir }}", Env: map[string]string{"MODE": "{{ .mode }}"}, FailOnExit: &failOnExit,
	}}
	values := map[string]string{"workdir": workdir, "mode": "value with 'quotes'"}
	runDir := filepath.Join(dir, "run")
	if err := r.executeShellNode(context.Background(), node, values, runDir, "", nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(workdir, "result.txt")); got != values["mode"]+"|"+values["mode"] {
		t.Fatalf("Shell 环境变量未正确传递: %q", got)
	}
	expectedWorkdir := filepath.ToSlash(workdir)
	if isWSLBash() {
		expectedWorkdir = windowsPathToWSLOrSlash(workdir)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(runDir, "stdout.log"))); got != expectedWorkdir {
		t.Fatalf("工作目录=%q，期望 %q", got, expectedWorkdir)
	}
	if stderr := readFile(t, filepath.Join(runDir, "stderr.log")); !strings.Contains(stderr, "7") {
		t.Fatalf("忽略退出码时未记录原始错误: %q", stderr)
	}
}

func TestExecuteShellNodeFailOnExitDoesNotSuppressTimeout(t *testing.T) {
	r, dir := newHTTPTestRunner(t)
	failOnExit := false
	node := config.WorkflowNode{ID: "script", Script: config.WorkflowScript{Script: "sleep 10", Timeout: "150ms", FailOnExit: &failOnExit}}
	if err := r.executeShellNode(context.Background(), node, nil, filepath.Join(dir, "run"), "", io.Discard, io.Discard); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("fail_on_exit=false 吞掉了执行超时: %v", err)
	}
}

func TestExecuteShellNodeUsesRemoteExecutorAndPreservesTransportFailure(t *testing.T) {
	r, dir := newHTTPTestRunner(t)
	transportErr := errors.New("SSH transport failed")
	remote := &fakeRemoteExecutor{err: transportErr}
	r.SetRemoteExecutor(remote)
	failOnExit := false
	node := config.WorkflowNode{ID: "script", Script: config.WorkflowScript{Script: "echo \"$MODE\"", Workdir: "/srv/app's dir", Env: map[string]string{"MODE": "{{ .mode }}"}, FailOnExit: &failOnExit}}
	err := r.executeShellNode(context.Background(), node, map[string]string{"mode": "remote"}, filepath.Join(dir, "run"), "host-1", io.Discard, io.Discard)
	if !errors.Is(err, transportErr) {
		t.Fatalf("fail_on_exit=false 吞掉了远程执行失败: %v", err)
	}
	if len(remote.requests) != 1 {
		t.Fatalf("内置 Shell 未使用远程执行器: %d", len(remote.requests))
	}
	req := remote.requests[0]
	if req.HostID != "host-1" || req.ToolID != "builtin.script" || filepath.Base(req.Entry) != "run.sh" {
		t.Fatalf("远程执行请求不正确: %+v", req)
	}
	if !strings.Contains(strings.Join(req.Env, "\n"), "MODE=remote") || !strings.Contains(strings.Join(req.Env, "\n"), "OPS_PARAM_MODE=remote") {
		t.Fatalf("远程执行环境不正确: %v", req.Env)
	}
	if script := readFile(t, req.Entry); !strings.Contains(script, "cd -- "+shellQuote(node.Script.Workdir)) {
		t.Fatalf("远程工作目录未保留并转义: %q", script)
	}
}

func TestExecuteChecksumNodeValidatesRenderedExpectedDigest(t *testing.T) {
	r, dir := newHTTPTestRunner(t)
	digest := strings.Repeat("a", 64)
	remote := &fakeRemoteExecutor{stdout: digest + "\n"}
	r.SetRemoteExecutor(remote)
	node := config.WorkflowNode{ID: "digest", Checksum: config.WorkflowChecksum{Path: "/srv/payload", Expected: "{{ .steps.Hash.stdout }}"}}
	if err := r.executeChecksumNode(context.Background(), node, map[string]string{"steps.Hash.stdout": "invalid"}, filepath.Join(dir, "invalid"), "host-1", io.Discard, io.Discard); err == nil {
		t.Fatal("渲染后的无效摘要必须在执行前失败")
	}
	if len(remote.requests) != 0 {
		t.Fatal("无效摘要仍触发了远程执行")
	}
	if err := r.executeChecksumNode(context.Background(), node, map[string]string{"steps.Hash.stdout": strings.ToUpper(digest) + "\n"}, filepath.Join(dir, "valid"), "host-1", io.Discard, io.Discard); err != nil {
		t.Fatalf("有效摘要模板未通过校验: %v", err)
	}
	if len(remote.requests) != 1 || remote.requests[0].HostID != "host-1" {
		t.Fatalf("校验节点未在目标主机执行: %+v", remote.requests)
	}
}

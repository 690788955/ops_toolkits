package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"shell_ops/internal/config"
)

const (
	httpResponseMetaFile   = "http-response.json"
	maxHTTPResponseBytes   = 4 << 20
	defaultHTTPNodeTimeout = 30 * time.Second
)

type httpResponseMeta struct {
	Method      string `json:"method"`
	URL         string `json:"url"`
	Status      int    `json:"status"`
	StatusText  string `json:"status_text,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	DurationMS  int64  `json:"duration_ms"`
	BodyBytes   int    `json:"body_bytes"`
	Truncated   bool   `json:"truncated,omitempty"`
	SavedPath   string `json:"saved_path,omitempty"`
}

// executeHTTPNode 执行内置 HTTP 调用节点：渲染模板 → 发起请求 → 落盘响应。
// 响应体写入 stdout.log，供下游用 {{ .steps.<id>.stdout }} 引用；
// 状态码、耗时等元数据写入 http-response.json，并同步到运行上下文。
func (r *Runner) executeHTTPNode(ctx context.Context, node config.WorkflowNode, workflowContext map[string]string, runDir string, out, errOut io.Writer) (httpResponseMeta, error) {
	meta := httpResponseMeta{}
	if mkErr := os.MkdirAll(runDir, 0o755); mkErr != nil {
		return meta, fmt.Errorf("创建 HTTP 节点运行目录失败: %w", mkErr)
	}
	method, methodErr := config.NormalizeHTTPMethod(renderTemplate(node.HTTP.Method, workflowContext))
	if methodErr != nil {
		return meta, methodErr
	}
	targetURL := strings.TrimSpace(renderTemplate(node.HTTP.URL, workflowContext))
	if targetURL == "" {
		return meta, fmt.Errorf("HTTP 节点 %s 的 http.url 渲染后为空", node.ID)
	}
	parsed, parseErr := url.Parse(targetURL)
	if parseErr != nil {
		return meta, fmt.Errorf("HTTP 节点 %s 的 http.url 无法解析: %w", node.ID, parseErr)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return meta, fmt.Errorf("HTTP 节点 %s 的 http.url 只支持 http:// 或 https://", node.ID)
	}
	meta.Method = method
	meta.URL = targetURL

	bodyType, bodyTypeErr := config.NormalizeHTTPBodyType(node.HTTP.BodyType, node.HTTP.Body)
	if bodyTypeErr != nil {
		return meta, bodyTypeErr
	}
	body := ""
	if bodyType != "none" {
		body = renderTemplate(node.HTTP.Body, workflowContext)
	}

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, reqErr := http.NewRequestWithContext(ctx, method, targetURL, reader)
	if reqErr != nil {
		return meta, fmt.Errorf("HTTP 节点 %s 构建请求失败: %w", node.ID, reqErr)
	}
	for name, value := range node.HTTP.Headers {
		headerName := strings.TrimSpace(renderTemplate(name, workflowContext))
		headerValue := renderTemplate(value, workflowContext)
		if headerName == "" {
			continue
		}
		if strings.ContainsAny(headerName, "\r\n") || strings.ContainsAny(headerValue, "\r\n") {
			return meta, fmt.Errorf("HTTP 节点 %s 的 header %s 不能包含换行", node.ID, headerName)
		}
		req.Header.Set(headerName, headerValue)
	}
	if body != "" && req.Header.Get("Content-Type") == "" {
		switch bodyType {
		case "json":
			req.Header.Set("Content-Type", "application/json")
		case "form":
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		default:
			req.Header.Set("Content-Type", "text/plain; charset=utf-8")
		}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json, text/plain, */*")
	}

	timeout := nodeHTTPTimeout(node)
	client := &http.Client{Timeout: timeout}
	started := time.Now()
	resp, respErr := client.Do(req)
	meta.DurationMS = time.Since(started).Milliseconds()
	if respErr != nil {
		_ = os.WriteFile(filepath.Join(runDir, "stderr.log"), []byte(respErr.Error()+"\n"), 0o600)
		return meta, fmt.Errorf("HTTP 节点 %s 请求失败: %w", node.ID, respErr)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxHTTPResponseBytes+1)
	payload, readErr := io.ReadAll(limited)
	if readErr != nil {
		return meta, fmt.Errorf("HTTP 节点 %s 读取响应失败: %w", node.ID, readErr)
	}
	if len(payload) > maxHTTPResponseBytes {
		payload = payload[:maxHTTPResponseBytes]
		meta.Truncated = true
	}
	meta.Status = resp.StatusCode
	meta.StatusText = strings.TrimSpace(resp.Status)
	meta.ContentType = resp.Header.Get("Content-Type")
	meta.BodyBytes = len(payload)

	if writeErr := os.WriteFile(filepath.Join(runDir, "stdout.log"), payload, 0o600); writeErr != nil {
		return meta, fmt.Errorf("HTTP 节点 %s 写入响应失败: %w", node.ID, writeErr)
	}
	savedPath, saveErr := saveHTTPResponse(runDir, node.HTTP.SaveResponse, payload)
	if saveErr != nil {
		return meta, saveErr
	}
	meta.SavedPath = savedPath
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(filepath.Join(runDir, httpResponseMetaFile), metaBytes, 0o600)

	if out != nil {
		fmt.Fprintf(out, "%s %s -> %d（%dms，%d 字节）\n", method, targetURL, meta.Status, meta.DurationMS, meta.BodyBytes)
	}
	if !node.HTTP.MatchesStatus(meta.Status) {
		snippet := strings.TrimSpace(string(payload))
		if len(snippet) > 400 {
			snippet = snippet[:400] + "..."
		}
		message := fmt.Sprintf("HTTP 节点 %s 返回状态码 %d，与期望不符", node.ID, meta.Status)
		if snippet != "" {
			message = message + "：" + snippet
		}
		_ = os.WriteFile(filepath.Join(runDir, "stderr.log"), []byte(message+"\n"), 0o600)
		if node.HTTP.ShouldFailOnError() {
			return meta, fmt.Errorf("%s", message)
		}
	}
	return meta, nil
}

func nodeHTTPTimeout(node config.WorkflowNode) time.Duration {
	if node.HTTP.Timeout != "" {
		if parsed, err := time.ParseDuration(strings.TrimSpace(node.HTTP.Timeout)); err == nil && parsed > 0 {
			return parsed
		}
	}
	return config.ParseTimeout(node.Timeout, defaultHTTPNodeTimeout)
}

func saveHTTPResponse(runDir, target string, payload []byte) (string, error) {
	clean, cleanErr := config.NormalizeHTTPSaveResponse(target)
	if cleanErr != nil || clean == "" {
		if cleanErr != nil {
			return "", cleanErr
		}
		return "", nil
	}
	path := filepath.Join(runDir, filepath.FromSlash(clean))
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return "", fmt.Errorf("创建响应目录失败: %w", mkErr)
	}
	if writeErr := os.WriteFile(path, payload, 0o600); writeErr != nil {
		return "", fmt.Errorf("保存响应文件失败: %w", writeErr)
	}
	return clean, nil
}

func readHTTPResponseMeta(path string) (httpResponseMeta, error) {
	var meta httpResponseMeta
	data, err := os.ReadFile(path)
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, err
	}
	return meta, nil
}

func addHTTPContext(workflowContext map[string]string, nodeID string, meta httpResponseMeta) {
	if workflowContext == nil {
		return
	}
	prefix := "steps." + nodeID + ".http."
	workflowContext[prefix+"method"] = meta.Method
	workflowContext[prefix+"url"] = meta.URL
	workflowContext[prefix+"status"] = strconv.Itoa(meta.Status)
	workflowContext[prefix+"duration_ms"] = strconv.FormatInt(meta.DurationMS, 10)
	if meta.ContentType != "" {
		workflowContext[prefix+"content_type"] = meta.ContentType
	}
	if meta.SavedPath != "" {
		workflowContext[prefix+"saved_path"] = meta.SavedPath
	}
}

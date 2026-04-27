package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

func TestToolDetailAPI(t *testing.T) {
	reg := testRegistry(t)
	req := httptest.NewRequest(http.MethodGet, "/api/tools/demo.hello", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "demo.hello") {
		t.Fatalf("响应缺少工具 ID: %s", res.Body.String())
	}
}

func TestCatalogAPIIncludesTags(t *testing.T) {
	reg := testRegistry(t)
	req := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, "工具标签") || !strings.Contains(body, "工作流标签") {
		t.Fatalf("响应缺少标签: %s", body)
	}
}

func TestWorkflowDetailAPI(t *testing.T) {
	reg := testRegistry(t)
	req := httptest.NewRequest(http.MethodGet, "/api/workflows/demo.flow", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "demo.flow") {
		t.Fatalf("响应缺少工作流 ID: %s", res.Body.String())
	}
}

func TestWorkflowValidateAPI(t *testing.T) {
	reg := testRegistry(t)
	body := `{"workflow":{"id":"demo.new","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.new/validate", strings.NewReader(body))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"valid":true`) {
		t.Fatalf("响应缺少 valid=true: %s", res.Body.String())
	}
}

func TestWorkflowValidateAPIRejectsMissingTool(t *testing.T) {
	reg := testRegistry(t)
	body := `{"workflow":{"id":"demo.new","nodes":[{"id":"first","tool":"missing.tool"}],"edges":[]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.new/validate", strings.NewReader(body))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"valid":false`) {
		t.Fatalf("响应缺少 valid=false: %s", res.Body.String())
	}
}

func TestWorkflowSaveAPI(t *testing.T) {
	reg := testRegistry(t)
	body := `{"workflow":{"id":"demo.saved","name":"已保存","category":"demo","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.saved/save", strings.NewReader(body))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	path := filepath.Join(reg.BaseDir, "workflows", "demo.saved.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("未找到已保存的工作流: %v", err)
	}
	if _, ok := reg.Workflows["demo.saved"]; !ok {
		t.Fatalf("已保存工作流未加入注册表")
	}
}

func TestWorkflowSaveAPIRejectsMismatchedID(t *testing.T) {
	reg := testRegistry(t)
	body := `{"workflow":{"id":"demo.other","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.saved/save", strings.NewReader(body))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, 期望 bad request; body = %s", res.Code, res.Body.String())
	}
}

func TestToolDevKitDownloadAPI(t *testing.T) {
	reg := testRegistry(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dev/toolkit.zip", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if contentType := res.Header().Get("Content-Type"); contentType != "application/zip" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	if disposition := res.Header().Get("Content-Disposition"); !strings.Contains(disposition, "ops-tool-devkit.zip") {
		t.Fatalf("Content-Disposition 缺少文件名: %s", disposition)
	}
	reader, err := zip.NewReader(bytes.NewReader(res.Body.Bytes()), int64(res.Body.Len()))
	if err != nil {
		t.Fatalf("无法读取 zip: %v", err)
	}
	entries := map[string]bool{}
	for _, file := range reader.File {
		entries[file.Name] = true
	}
	for _, name := range []string{"README.md", "SPEC.md", "tools/demo/sample-tool/tool.yaml", "tools/demo/sample-tool/bin/run.sh", "tools/demo/sample-tool/README.md"} {
		if !entries[name] {
			t.Fatalf("开发包缺少文件 %s", name)
		}
	}
}

func TestToolRunAPIRequiresConfirm(t *testing.T) {
	reg := testRegistry(t)
	reg.Tools["demo.hello"].Config.Confirm = config.Confirmation{Required: true, Message: "确认执行？"}
	req := httptest.NewRequest(http.MethodPost, "/api/tools/demo.hello/run", strings.NewReader(`{"params":{}}`))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad request; body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "需要确认") {
		t.Fatalf("响应缺少确认提示: %s", res.Body.String())
	}
}

func TestCatalogAPIIncludesSourceAndConfirm(t *testing.T) {
	reg := testRegistry(t)
	reg.Tools["demo.hello"].Source = registry.Source{Type: "plugin", PluginID: "vendor.demo", PluginName: "Demo", PluginVersion: "1.0.0"}
	reg.Tools["demo.hello"].Config.Confirm = config.Confirmation{Required: true, Message: "确认执行？"}
	req := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, "vendor.demo") || !strings.Contains(body, "确认执行") {
		t.Fatalf("catalog 缺少插件来源或确认信息: %s", body)
	}
}

func TestWorkflowRunAPIRequiresToolConfirm(t *testing.T) {
	reg := testRegistry(t)
	reg.Tools["demo.hello"].Config.Confirm = config.Confirmation{Required: true, Message: "确认工具？"}
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.flow/run", strings.NewReader(`{"params":{}}`))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad request; body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "需要确认") {
		t.Fatalf("响应缺少工具确认提示: %s", res.Body.String())
	}
}

func TestRunDetailAPIIncludesLogs(t *testing.T) {
	reg := testRegistry(t)
	runDir := filepath.Join(reg.BaseDir, "runs", "logs", "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "result.json"), []byte(`{"id":"run-1","kind":"tool","target":"demo.hello","status":"succeeded"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stdout.log"), []byte("标准输出\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stderr.log"), []byte("错误输出\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/runs/run-1", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "标准输出") || !strings.Contains(res.Body.String(), "错误输出") {
		t.Fatalf("响应缺少日志内容: %s", res.Body.String())
	}
}

func testRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	dir := t.TempDir()
	reg := &registry.Registry{
		BaseDir: dir,
		Root: &config.RootConfig{
			Paths: config.PathsConfig{Workflows: []string{"workflows"}, Logs: "runs/logs"},
			Menu:  config.MenuConfig{Categories: []config.Category{{ID: "demo", Name: "演示"}}},
		},
		Tools: map[string]*registry.Tool{
			"demo.hello": {
				Entry:  config.ToolEntry{ID: "demo.hello", Category: "demo", Name: "问候"},
				Config: &config.ToolConfig{ID: "demo.hello", Category: "demo", Name: "问候", Tags: []string{"工具标签"}, Execution: config.ExecutionConfig{Entry: "bin/run.sh"}},
				Dir:    filepath.Join(dir, "tools", "demo", "hello"),
			},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{ID: "demo.flow", Category: "demo", Tags: []string{"工作流标签"}, Nodes: []config.WorkflowNode{{ID: "first", Tool: "demo.hello"}}, Edges: []config.WorkflowEdge{}}
	reg.Workflows["demo.flow"] = &registry.Workflow{Entry: config.WorkflowRef{ID: "demo.flow", Category: "demo", Path: "workflows/demo.flow.yaml", Tags: wf.Tags}, Config: wf, Path: filepath.Join(dir, "workflows", "demo.flow.yaml")}
	return reg
}

func decodeResponse(t *testing.T, body *bytes.Buffer) response {
	t.Helper()
	var out response
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

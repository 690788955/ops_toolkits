package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func saveWorkflowViaAPI(t *testing.T, handler http.Handler, id, body string) {
	t.Helper()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/workflows/"+id+"/save", strings.NewReader(body)))
	if res.Code != http.StatusOK {
		t.Fatalf("save %s status = %d, body = %s", id, res.Code, res.Body.String())
	}
}

func workflowImportRequest(t *testing.T, filename string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func decodeImportResult(t *testing.T, res *httptest.ResponseRecorder) workflowImportResult {
	t.Helper()
	var envelope struct {
		Data workflowImportResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("解析导入结果失败: %v, body = %s", err, res.Body.String())
	}
	return envelope.Data
}

func TestWorkflowExportSingleYAML(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	saveWorkflowViaAPI(t, handler, "demo.exported", `{"workflow":{"id":"demo.exported","name":"导出流程","category":"demo","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`)

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/workflows/export?workflow=demo.exported", nil))

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if ct := res.Header().Get("Content-Type"); !strings.Contains(ct, "yaml") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cd := res.Header().Get("Content-Disposition"); !strings.Contains(cd, "demo.exported.yaml") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if !strings.Contains(res.Body.String(), "导出流程") {
		t.Fatalf("单工作流导出未包含内容: %s", res.Body.String())
	}
}

func TestWorkflowExportBatchZip(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	saveWorkflowViaAPI(t, handler, "demo.a", `{"workflow":{"id":"demo.a","name":"A","category":"demo","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`)
	saveWorkflowViaAPI(t, handler, "demo.b", `{"workflow":{"id":"demo.b","name":"B","category":"demo","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`)
	configDir := filepath.Join(reg.BaseDir, "plugins", "user.workflows", "config", "workflows", "demo.a")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "app.conf"), []byte("enabled=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/workflows/export?workflow=demo.a&workflow=demo.b", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if ct := res.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("Content-Type = %q", ct)
	}
	entries := readZipEntries(t, res.Body.Bytes())
	for _, name := range []string{"workflows/demo.a.yaml", "workflows/demo.b.yaml", "config/workflows/demo.a/app.conf", "export-manifest.json"} {
		if _, ok := entries[name]; !ok {
			t.Fatalf("批量导出 ZIP 缺少 %s，entries=%v", name, entries)
		}
	}
	if _, ok := entries["workflows/demo.missing.yaml"]; ok {
		t.Fatalf("批量导出不应包含未知工作流: %v", entries)
	}

	emptyRes := httptest.NewRecorder()
	handler.ServeHTTP(emptyRes, httptest.NewRequest(http.MethodGet, "/api/workflows/export", nil))
	if emptyRes.Code != http.StatusBadRequest {
		t.Fatalf("空选择应返回 400，status = %d, body = %s", emptyRes.Code, emptyRes.Body.String())
	}
}

func TestWorkflowImportYAMLRenamesOnConflict(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	content := "id: demo.imported\nname: 导入流程\ncategory: demo\nnodes:\n  - id: first\n    tool: demo.hello\nedges: []\n"

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, workflowImportRequest(t, "demo.imported.yaml", []byte(content)))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	result := decodeImportResult(t, res)
	if len(result.Imported) != 1 || result.Imported[0].Renamed {
		t.Fatalf("首次导入结果异常: %+v", result)
	}
	if reg.Workflows["demo.imported"] == nil {
		t.Fatal("导入后注册表未包含工作流")
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "user.workflows", "workflows", "demo.imported.yaml")); err != nil {
		t.Fatalf("导入未写出工作流文件: %v", err)
	}

	againRes := httptest.NewRecorder()
	handler.ServeHTTP(againRes, workflowImportRequest(t, "demo.imported.yaml", []byte(content)))
	if againRes.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", againRes.Code, againRes.Body.String())
	}
	again := decodeImportResult(t, againRes)
	if len(again.Imported) != 1 || !again.Imported[0].Renamed || again.Imported[0].NewID != "demo.imported-2" {
		t.Fatalf("ID 冲突未自动改名: %+v", again)
	}
	if reg.Workflows["demo.imported-2"] == nil {
		t.Fatal("改名后的工作流未落库")
	}
}

func TestWorkflowImportZipWithConfigFiles(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	writeZipFile(t, writer, "workflows/demo.zipped.yaml", "id: demo.zipped\nname: 压缩导入\ncategory: demo\nnodes:\n  - id: first\n    tool: demo.hello\n")
	writeZipFile(t, writer, "config/workflows/demo.zipped/app.conf", "enabled=true\n")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, workflowImportRequest(t, "workflows-export.zip", buf.Bytes()))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	result := decodeImportResult(t, res)
	if len(result.Imported) != 1 || result.Imported[0].ConfigFiles != 1 {
		t.Fatalf("ZIP 导入结果异常: %+v", result)
	}
	target := filepath.Join(reg.BaseDir, "plugins", "user.workflows", "config", "workflows", "demo.zipped", "app.conf")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("托管配置文件未落盘: %v", err)
	}
	if string(data) != "enabled=true\n" {
		t.Fatalf("托管配置文件内容不符: %q", string(data))
	}
}

func TestWorkflowImportSkipsMissingTool(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	content := "id: demo.missingtool\nname: 缺工具\nnodes:\n  - id: first\n    tool: not.installed\n"

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, workflowImportRequest(t, "demo.missingtool.yaml", []byte(content)))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	result := decodeImportResult(t, res)
	if len(result.Imported) != 0 || len(result.Failed) != 1 {
		t.Fatalf("缺工具的工作流应被跳过: %+v", result)
	}
	if !strings.Contains(result.Failed[0].Error, "不存在的工具") {
		t.Fatalf("失败原因未说明缺工具: %s", result.Failed[0].Error)
	}
	if reg.Workflows["demo.missingtool"] != nil {
		t.Fatal("缺工具的工作流不应写入注册表")
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "user.workflows", "workflows", "demo.missingtool.yaml")); !os.IsNotExist(err) {
		t.Fatalf("缺工具的工作流不应写出文件: %v", err)
	}
}

func TestWorkflowImportRejectsUnsafeZipPath(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	writeZipFile(t, writer, "workflows/../evil.yaml", "id: evil\nnodes:\n  - id: first\n    tool: demo.hello\n")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, workflowImportRequest(t, "evil.zip", buf.Bytes()))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("不安全路径应返回 400，status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestWorkflowExportImportRoundTrip(t *testing.T) {
	source := testRegistry(t)
	sourceHandler := NewHandler(source)
	saveWorkflowViaAPI(t, sourceHandler, "demo.roundtrip", `{"workflow":{"id":"demo.roundtrip","name":"往返流程","category":"demo","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`)
	configDir := filepath.Join(source.BaseDir, "plugins", "user.workflows", "config", "workflows", "demo.roundtrip")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "app.conf"), []byte("enabled=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exportRes := httptest.NewRecorder()
	sourceHandler.ServeHTTP(exportRes, httptest.NewRequest(http.MethodGet, "/api/workflows/export?workflow=demo.roundtrip", nil))
	if exportRes.Code != http.StatusOK {
		t.Fatalf("导出失败 status = %d, body = %s", exportRes.Code, exportRes.Body.String())
	}
	// 单工作流导出为 .yaml，这里直接导入 YAML 验证工作流本体可迁移。
	target := testRegistry(t)
	targetHandler := NewHandler(target)
	importRes := httptest.NewRecorder()
	targetHandler.ServeHTTP(importRes, workflowImportRequest(t, "demo.roundtrip.yaml", exportRes.Body.Bytes()))
	if importRes.Code != http.StatusOK {
		t.Fatalf("导入失败 status = %d, body = %s", importRes.Code, importRes.Body.String())
	}
	imported := target.Workflows["demo.roundtrip"]
	if imported == nil {
		t.Fatalf("往返后目标注册表缺少工作流: %+v", decodeImportResult(t, importRes))
	}
	if len(imported.Config.Nodes) != 1 || imported.Config.Nodes[0].Tool != "demo.hello" {
		t.Fatalf("往返后节点不一致: %+v", imported.Config.Nodes)
	}
}

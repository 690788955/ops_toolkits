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

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

func TestWebIndexDisablesBrowserCache(t *testing.T) {
	reg := testRegistry(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if contentType := res.Header().Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", contentType)
	}
}

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

func TestWorkflowRunAPIRunsUnsavedDraftWithoutPersisting(t *testing.T) {
	reg := testRegistry(t)
	toolDir := reg.Tools["demo.hello"].Dir
	if err := os.MkdirAll(filepath.Join(toolDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "bin", "run.sh"), []byte("#!/usr/bin/env bash\necho draft-run\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"params":{},"workflow":{"id":"demo.unsaved","name":"未保存草稿","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.unsaved/run", strings.NewReader(body))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"id":"workflow-`) || !strings.Contains(res.Body.String(), `"status":"succeeded"`) {
		t.Fatalf("响应缺少运行成功信息: %s", res.Body.String())
	}
	if _, ok := reg.Workflows["demo.unsaved"]; ok {
		t.Fatalf("未保存草稿不应写入注册表: %#v", reg.Workflows["demo.unsaved"])
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "user.workflows", "workflows", "demo.unsaved.yaml")); !os.IsNotExist(err) {
		t.Fatalf("未保存草稿不应写入 workflow 文件，stat err = %v", err)
	}
}

func TestWorkflowSaveAPI(t *testing.T) {
	reg := testRegistry(t)
	body := `{"workflow":{"id":"demo.saved","name":"已保存","category":"demo","tags":["自定义","迁移"],"nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.saved/save", strings.NewReader(body))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	path := filepath.Join(reg.BaseDir, "plugins", "user.workflows", "workflows", "demo.saved.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("未找到已保存的插件内工作流: %v", err)
	}
	manifestPath := filepath.Join(reg.BaseDir, "plugins", "user.workflows", "plugin.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("未找到用户工作流插件清单: %v", err)
	}
	manifestText := string(manifest)
	if !strings.Contains(manifestText, "id: user.workflows") || !strings.Contains(manifestText, "path: workflows/demo.saved.yaml") {
		t.Fatalf("用户工作流插件清单未维护 workflow 贡献: %s", manifestText)
	}
	if got := reg.Workflows["demo.saved"]; got == nil || got.Source.PluginID != "user.workflows" || len(got.Entry.Tags) != 2 {
		t.Fatalf("已保存工作流未以用户插件来源加入注册表: %#v", got)
	}

	installDemoToolPlugin(t, reg.BaseDir)
	reloaded, err := registry.Load(reg.BaseDir)
	if err != nil {
		t.Fatalf("重新加载注册表失败: %v", err)
	}
	got := reloaded.Workflows["demo.saved"]
	if got == nil || got.Source.PluginID != "user.workflows" || got.Entry.Category != "demo" || len(got.Config.Tags) != 2 {
		t.Fatalf("已保存工作流重启后未通过用户插件加载: %#v", got)
	}
}

func TestUserWorkflowPluginManifestRemovesDeletedWorkflowEntries(t *testing.T) {
	reg := testRegistry(t)
	workflowDir := filepath.Join(reg.BaseDir, "plugins", "user.workflows", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(reg.BaseDir, "plugins", "user.workflows", "plugin.yaml")
	staleManifest := `id: user.workflows
name: 用户工作流
version: 1.0.0
contributes:
  workflows:
    - path: workflows/deleted.yaml
`
	if err := os.WriteFile(manifestPath, []byte(staleManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"workflow":{"id":"demo.kept","name":"保留流程","category":"demo","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.kept/save", strings.NewReader(body))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifestText := string(manifest)
	if strings.Contains(manifestText, "workflows/deleted.yaml") || !strings.Contains(manifestText, "path: workflows/demo.kept.yaml") {
		t.Fatalf("用户工作流插件清单未按实际文件刷新: %s", manifestText)
	}
}

func TestUserWorkflowPluginExportAPI(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	saveReq := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.exported/save", strings.NewReader(`{"workflow":{"id":"demo.exported","name":"导出流程","category":"demo","tags":["导出"],"nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`))
	saveRes := httptest.NewRecorder()
	handler.ServeHTTP(saveRes, saveReq)
	if saveRes.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", saveRes.Code, saveRes.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/plugins/user-workflows.zip", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if contentType := res.Header().Get("Content-Type"); contentType != "application/zip" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	reader, err := zip.NewReader(bytes.NewReader(res.Body.Bytes()), int64(res.Body.Len()))
	if err != nil {
		t.Fatalf("无法读取 zip: %v", err)
	}
	entries := map[string]string{}
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatalf("无法打开 zip entry %s: %v", file.Name, err)
		}
		var content bytes.Buffer
		if _, err := content.ReadFrom(handle); err != nil {
			_ = handle.Close()
			t.Fatalf("无法读取 zip entry %s: %v", file.Name, err)
		}
		_ = handle.Close()
		entries[file.Name] = content.String()
	}
	for _, name := range []string{"user.workflows/plugin.yaml", "user.workflows/workflows/demo.exported.yaml"} {
		if _, ok := entries[name]; !ok {
			t.Fatalf("导出 ZIP 缺少文件 %s，entries=%v", name, entries)
		}
	}
	if !strings.Contains(entries["user.workflows/plugin.yaml"], "path: workflows/demo.exported.yaml") || !strings.Contains(entries["user.workflows/workflows/demo.exported.yaml"], "导出流程") {
		t.Fatalf("导出 ZIP 未保留插件清单或工作流内容: %#v", entries)
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
	if disposition := res.Header().Get("Content-Disposition"); !strings.Contains(disposition, "ops-plugin-template.zip") {
		t.Fatalf("Content-Disposition 缺少文件名: %s", disposition)
	}
	reader, err := zip.NewReader(bytes.NewReader(res.Body.Bytes()), int64(res.Body.Len()))
	if err != nil {
		t.Fatalf("无法读取 zip: %v", err)
	}
	entries := map[string]bool{}
	contents := map[string]string{}
	for _, file := range reader.File {
		entries[file.Name] = true
		handle, err := file.Open()
		if err != nil {
			t.Fatalf("无法打开 zip entry %s: %v", file.Name, err)
		}
		var content bytes.Buffer
		if _, err := content.ReadFrom(handle); err != nil {
			_ = handle.Close()
			t.Fatalf("无法读取 zip entry %s: %v", file.Name, err)
		}
		if err := handle.Close(); err != nil {
			t.Fatalf("无法关闭 zip entry %s: %v", file.Name, err)
		}
		contents[file.Name] = content.String()
	}
	for _, name := range []string{"README.md", "SPEC.md", "plugins/plugin.template/plugin.yaml", "plugins/plugin.template/scripts/run.sh", "plugins/plugin.template/workflows/maintenance-flow.yaml", "plugins/plugin.template/README.md", "plugins/plugin.template/examples/params.yaml", "plugins/plugin.template/examples/README.md"} {
		if !entries[name] {
			t.Fatalf("开发包缺少文件 %s", name)
		}
	}
	combined := strings.Join([]string{
		contents["README.md"],
		contents["SPEC.md"],
		contents["plugins/plugin.template/plugin.yaml"],
		contents["plugins/plugin.template/scripts/run.sh"],
		contents["plugins/plugin.template/workflows/maintenance-flow.yaml"],
		contents["plugins/plugin.template/README.md"],
		contents["plugins/plugin.template/examples/README.md"],
	}, "\n")
	for _, want := range []string{"插件开发包", "plugin.yaml", "规范插件模板", "可复制的规范模板", "id: plugin.template", "name: 规范插件模板", "version: 1.0.0", "description:", "author: your-team", "compatibility:", "contributes:", "categories:", "tools:", "workflows:", "plugin.template.inspect", "plugin.template.apply", "plugin.template.maintenance-flow", "confirm.required", "required: true", "default: demo", "type: bool", "timeout: 5m", "tags: [plugin, template, change, high-risk]", "command: scripts/run.sh", "workdir: .", "args:", "depends_on: [inspect]", "from: inspect", "to: apply", "usage()", "error()", "normalize_bool()", "未知参数", "缺少必填参数 target", "action 只支持 inspect 或 apply", "dry-run 只接受 true/false、yes/no、1/0、on/off", "dry-run", "不要在 stdout/stderr 输出密码", "./bin/opsctl.exe validate", "./bin/opsctl.exe run tool plugin.template.inspect", "./bin/opsctl.exe run workflow plugin.template.maintenance-flow", "./bin/opsctl.exe package build", "插件开发者交付清单", "更新已存在插件时提升 version", "不要假设交付或接入时会执行脚本", "宿主运行环境", "打包交付", "command、workdir、workflow path 都应留在插件目录内部"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("开发包文案缺少关键内容 %q", want)
		}
	}
	for _, forbidden := range []string{"Web UI", "页面", "catalog", "上传端点", "API", "后端", "前端", "Go/React", "平台源码", "页面插件管理", "运维平台", "上传过程只安装并校验插件文件"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("开发包文案不应包含面向平台内部或产品页面的词 %q", forbidden)
		}
	}
	for _, legacy := range []string{"tools/demo/sample-tool", "tools/demo/hello", "workflows/demo-hello", "./opsctl.exe", "opsctl validate", "opsctl run tool"} {
		if strings.Contains(combined, legacy) {
			t.Fatalf("开发包不应包含旧路径或旧命令 %q", legacy)
		}
	}
}

func TestCatalogAPIIncludesExportablePlugins(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPluginWithWorkflow(t, baseReg.BaseDir, "vendor.catalog", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, `"plugins"`) || !strings.Contains(body, "vendor.catalog") {
		t.Fatalf("catalog 缺少可导出插件列表: %s", body)
	}
}

func TestPluginUploadInstallsNewPluginAndRefreshesCatalog(t *testing.T) {
	reg := testRegistry(t)
	req := pluginUploadRequest(t, pluginZip(t, "vendor.upload", "1.0.0", false), false)
	res := httptest.NewRecorder()
	handler := NewHandler(reg)

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "vendor.upload", "plugin.yaml")); err != nil {
		t.Fatalf("插件未安装: %v", err)
	}
	catalogReq := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	catalogRes := httptest.NewRecorder()
	handler.ServeHTTP(catalogRes, catalogReq)
	if !strings.Contains(catalogRes.Body.String(), "vendor.upload.tool") {
		t.Fatalf("catalog 缺少上传插件贡献: %s", catalogRes.Body.String())
	}
}

func TestPluginUploadAcceptsSinglePluginDirectoryEntry(t *testing.T) {
	reg := testRegistry(t)
	req := pluginUploadRequest(t, pluginZipWithDirs(t, "vendor.dir", "1.0.0"), false)
	res := httptest.NewRecorder()
	handler := NewHandler(reg)

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "vendor.dir", "plugin.yaml")); err != nil {
		t.Fatalf("插件目录项 ZIP 未安装: %v", err)
	}
}

func TestPluginExportDownloadsInstalledPluginWithWorkflow(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPluginWithWorkflow(t, baseReg.BaseDir, "vendor.export", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.export.zip", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if contentType := res.Header().Get("Content-Type"); contentType != "application/zip" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	if disposition := res.Header().Get("Content-Disposition"); !strings.Contains(disposition, "vendor.export.zip") {
		t.Fatalf("Content-Disposition 缺少插件文件名: %s", disposition)
	}
	reader, err := zip.NewReader(bytes.NewReader(res.Body.Bytes()), int64(res.Body.Len()))
	if err != nil {
		t.Fatalf("无法读取导出 zip: %v", err)
	}
	entries := map[string]bool{}
	for _, file := range reader.File {
		entries[file.Name] = true
	}
	for _, name := range []string{"vendor.export/plugin.yaml", "vendor.export/scripts/run.sh", "vendor.export/workflows/flow.yaml", "vendor.export/README.md"} {
		if !entries[name] {
			t.Fatalf("导出 ZIP 缺少文件 %s，entries=%v", name, entries)
		}
	}
}

func TestPluginExportZipCanBeDiscoveredByUploadRoot(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPluginWithWorkflow(t, baseReg.BaseDir, "vendor.roundtrip", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	data, err := buildPluginExportZip(reg, "vendor.roundtrip")
	if err != nil {
		t.Fatalf("导出插件失败: %v", err)
	}
	staging := t.TempDir()
	if err := extractPluginZip(data, staging); err != nil {
		t.Fatalf("导出的 ZIP 不能被现有上传逻辑解压: %v", err)
	}
	root, err := findUploadedPluginRoot(staging)
	if err != nil {
		t.Fatalf("导出的 ZIP 不能被现有上传 root 发现: %v", err)
	}
	if filepath.Base(root) != "vendor.roundtrip" {
		t.Fatalf("root = %s, want vendor.roundtrip", root)
	}
}

func TestPluginExportRejectsUnknownPlugin(t *testing.T) {
	reg := testRegistry(t)
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.missing.zip", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want not found; body = %s", res.Code, res.Body.String())
	}
}

func TestPluginExportRejectsUnsafePluginID(t *testing.T) {
	reg := testRegistry(t)
	for _, pluginID := range []string{"../evil", `vendor.bad"name`, "vendor.bad;name", "vendor.bad name", "vendor.bad\nname"} {
		_, err := buildPluginExportZip(reg, pluginID)
		if err == nil || !strings.Contains(err.Error(), "不安全路径字符") {
			t.Fatalf("pluginID = %q, err = %v, want 不安全路径字符", pluginID, err)
		}
	}
}

func TestPluginExportRejectsUnsafePluginIDRequest(t *testing.T) {
	reg := testRegistry(t)
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/vendor%2Fevil.zip", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad request; body = %s", res.Code, res.Body.String())
	}
}

func TestPluginExportRejectsSymlink(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPlugin(t, baseReg.BaseDir, "vendor.symlink", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	pluginDir := filepath.Join(reg.BaseDir, "plugins", "vendor.symlink")
	if err := os.Symlink(filepath.Join(pluginDir, "plugin.yaml"), filepath.Join(pluginDir, "linked.yaml")); err != nil {
		t.Skipf("当前环境不能创建 symlink: %v", err)
	}

	err = buildPluginExportZipMustFail(reg, "vendor.symlink")
	if err == nil || !strings.Contains(err.Error(), "特殊文件") {
		t.Fatalf("err = %v, want 特殊文件", err)
	}
}

func TestPluginUploadRejectsMultiplePluginPackages(t *testing.T) {
	reg := testRegistry(t)
	req := pluginUploadRequest(t, multiPluginZip(t), false)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad request; body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "只包含一个插件包") {
		t.Fatalf("响应缺少单插件包提示: %s", res.Body.String())
	}
}

func TestPluginUploadRejectsPathTraversal(t *testing.T) {
	reg := testRegistry(t)
	for _, zipData := range [][]byte{
		unsafeZip(t, "../escape/plugin.yaml"),
		unsafeZip(t, "/abs/plugin.yaml"),
		unsafeZip(t, "safe/../plugin.yaml"),
	} {
		req := pluginUploadRequest(t, zipData, false)
		res := httptest.NewRecorder()

		NewHandler(reg).ServeHTTP(res, req)

		if res.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want bad request; body = %s", res.Code, res.Body.String())
		}
		if !strings.Contains(res.Body.String(), "不安全路径") {
			t.Fatalf("响应缺少不安全路径提示: %s", res.Body.String())
		}
	}
}

func TestPluginUploadRejectsUnsafePluginID(t *testing.T) {
	reg := testRegistry(t)
	req := pluginUploadRequest(t, pluginRootZip(t, "../evil", "1.0.0"), false)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad request; body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "不安全路径字符") {
		t.Fatalf("响应缺少插件 ID 安全提示: %s", res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "evil")); !os.IsNotExist(err) {
		t.Fatalf("不安全插件 ID 不应写出到插件根目录外: %v", err)
	}
}

func TestPluginUploadRejectsInvalidZip(t *testing.T) {
	reg := testRegistry(t)
	req := pluginUploadRequest(t, []byte("not a zip"), false)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad request; body = %s", res.Code, res.Body.String())
	}
}

func TestPluginUploadDuplicateWithoutReplaceReturnsPrompt(t *testing.T) {
	reg := testRegistry(t)
	installTestPlugin(t, reg.BaseDir, "vendor.dup", "1.0.0")
	req := pluginUploadRequest(t, pluginZip(t, "vendor.dup", "1.1.0", false), false)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusConflict {
		t.Fatalf("status = %d, want conflict; body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "是否更新") {
		t.Fatalf("响应缺少更新提示: %s", res.Body.String())
	}
}

func TestPluginUploadRejectsSameOrLowerVersionReplace(t *testing.T) {
	reg := testRegistry(t)
	installTestPlugin(t, reg.BaseDir, "vendor.version", "1.0.0")
	req := pluginUploadRequest(t, pluginZip(t, "vendor.version", "1.0.0", false), true)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad request; body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "不高于") {
		t.Fatalf("响应缺少版本拒绝提示: %s", res.Body.String())
	}
}

func TestPluginUploadReplacesHigherVersionAndRefreshesCatalog(t *testing.T) {
	reg := testRegistry(t)
	installTestPlugin(t, reg.BaseDir, "vendor.replace", "1.0.0")
	req := pluginUploadRequest(t, pluginZip(t, "vendor.replace", "1.1.0", true), true)
	res := httptest.NewRecorder()
	handler := NewHandler(reg)

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	catalogReq := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	catalogRes := httptest.NewRecorder()
	handler.ServeHTTP(catalogRes, catalogReq)
	body := catalogRes.Body.String()
	if !strings.Contains(body, "vendor.replace.newtool") || !strings.Contains(body, "1.1.0") {
		t.Fatalf("catalog 未刷新为高版本贡献: %s", body)
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

func TestWorkflowSaveAPIPreservesConditionRoundTrip(t *testing.T) {
	reg := testRegistry(t)
	payload := `{"workflow":{"id":"demo.condition","name":"条件流程","category":"demo","nodes":[{"id":"inspect","tool":"demo.hello"},{"id":"route","type":"condition","condition":{"input":"{{ .steps.inspect.stdout }}","cases":[{"id":"ok","name":"正常","operator":"contains","values":["OK"]}],"default_case":"default"}},{"id":"notify","tool":"demo.hello"}],"edges":[{"from":"inspect","to":"route"},{"from":"route","to":"notify","case":"ok"}]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.condition/save", strings.NewReader(payload))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	saved := reg.Workflows["demo.condition"].Config
	if saved.Nodes[1].Type != config.WorkflowNodeTypeCondition || saved.Nodes[1].Condition.Cases[0].ID != "ok" || saved.Edges[1].Case != "ok" {
		t.Fatalf("condition round-trip lost fields: %#v", saved)
	}
}

func TestWorkflowSaveAPIPreservesParallelJoinRoundTrip(t *testing.T) {
	reg := testRegistry(t)
	payload := `{"workflow":{"id":"demo.parallel","name":"并行合流流程","category":"demo","nodes":[{"id":"split","type":"parallel","name":"并行分支"},{"id":"left","tool":"demo.hello"},{"id":"right","tool":"demo.hello"},{"id":"join","type":"join","name":"合流"},{"id":"done","tool":"demo.hello"}],"edges":[{"from":"split","to":"left"},{"from":"split","to":"right"},{"from":"left","to":"join"},{"from":"right","to":"join"},{"from":"join","to":"done"}]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.parallel/save", strings.NewReader(payload))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	saved := reg.Workflows["demo.parallel"].Config
	if saved.Nodes[0].Type != config.WorkflowNodeTypeParallel || saved.Nodes[3].Type != config.WorkflowNodeTypeJoin {
		t.Fatalf("parallel/join round-trip lost node types: %#v", saved.Nodes)
	}
	workflowFile := filepath.Join(reg.BaseDir, "plugins", "user.workflows", "workflows", "demo.parallel.yaml")
	content, err := os.ReadFile(workflowFile)
	if err != nil {
		t.Fatalf("read saved workflow: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "type: parallel") || !strings.Contains(text, "type: join") {
		t.Fatalf("saved workflow missing parallel/join types: %s", text)
	}
}

func TestWorkflowSaveAPIPreservesLoopRoundTrip(t *testing.T) {
	reg := testRegistry(t)
	payload := `{"workflow":{"id":"demo.loop","name":"循环流程","category":"demo","nodes":[{"id":"repeat","type":"loop","name":"固定循环","loop":{"tool":"demo.hello","params":{"name":"{{ .name }}"},"max_iterations":3}},{"id":"done","tool":"demo.hello"}],"edges":[{"from":"repeat","to":"done"}]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.loop/save", strings.NewReader(payload))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	saved := reg.Workflows["demo.loop"].Config
	if saved.Nodes[0].Type != config.WorkflowNodeTypeLoop || saved.Nodes[0].Loop.Tool != "demo.hello" || saved.Nodes[0].Loop.Params["name"] != "{{ .name }}" || saved.Nodes[0].Loop.MaxIterations != 3 {
		t.Fatalf("loop round-trip lost fields: %#v", saved.Nodes)
	}
	workflowFile := filepath.Join(reg.BaseDir, "plugins", "user.workflows", "workflows", "demo.loop.yaml")
	content, err := os.ReadFile(workflowFile)
	if err != nil {
		t.Fatalf("read saved workflow: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "type: loop") || !strings.Contains(text, "tool: demo.hello") || !strings.Contains(text, "max_iterations: 3") {
		t.Fatalf("saved workflow missing loop fields: %s", text)
	}
}
func testRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	dir := t.TempDir()
	writeTestRootConfig(t, dir)
	reg := &registry.Registry{
		BaseDir: dir,
		Root: &config.RootConfig{
			Paths:   config.PathsConfig{Workflows: []string{"workflows"}, Logs: "runs/logs"},
			Menu:    config.MenuConfig{Categories: []config.Category{{ID: "demo", Name: "演示"}}},
			Plugins: config.PluginsConfig{Paths: []string{"plugins"}},
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

func writeTestRootConfig(t *testing.T, dir string) {
	t.Helper()
	configDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	root := `app:
  name: Test Ops
paths:
  tools: []
  workflows: []
  runs: runs
  logs: runs/logs
plugins:
  paths:
    - plugins
  strict: true
  disabled: []
menu:
  categories:
    - id: demo
      name: 演示
`
	if err := os.WriteFile(filepath.Join(configDir, "ops.yaml"), []byte(root), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pluginUploadRequest(t *testing.T, zipData []byte, replace bool) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "plugin.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(zipData); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := "/api/plugins/upload"
	if replace {
		path += "?replace=true"
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func pluginZip(t *testing.T, id, version string, renamedTool bool) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	manifest, script := pluginFiles(id, version, renamedTool)
	writeZipFile(t, writer, id+"/plugin.yaml", manifest)
	writeZipFile(t, writer, id+"/scripts/run.sh", script)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func pluginZipWithDirs(t *testing.T, id, version string) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	if _, err := writer.Create(id + "/"); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Create(id + "/scripts/"); err != nil {
		t.Fatal(err)
	}
	manifest, script := pluginFiles(id, version, false)
	writeZipFile(t, writer, id+"/plugin.yaml", manifest)
	writeZipFile(t, writer, id+"/scripts/run.sh", script)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func multiPluginZip(t *testing.T) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	firstManifest, firstScript := pluginFiles("vendor.first", "1.0.0", false)
	secondManifest, secondScript := pluginFiles("vendor.second", "1.0.0", false)
	writeZipFile(t, writer, "vendor.first/plugin.yaml", firstManifest)
	writeZipFile(t, writer, "vendor.first/scripts/run.sh", firstScript)
	writeZipFile(t, writer, "vendor.second/plugin.yaml", secondManifest)
	writeZipFile(t, writer, "vendor.second/scripts/run.sh", secondScript)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func pluginRootZip(t *testing.T, id, version string) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	manifest, script := pluginFiles(id, version, false)
	writeZipFile(t, writer, "plugin.yaml", manifest)
	writeZipFile(t, writer, "scripts/run.sh", script)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func pluginFiles(id, version string, renamedTool bool) (string, string) {
	toolID := id + ".tool"
	if renamedTool {
		toolID = id + ".newtool"
	}
	manifest := `id: ` + id + `
name: Upload Test
version: ` + version + `
contributes:
  categories:
    - id: upload
      name: 上传插件
  tools:
    - id: ` + toolID + `
      name: 上传工具
      category: upload
      command: scripts/run.sh
      workdir: .
      timeout: 30m
      parameters:
        - name: target
          type: string
          required: false
      confirm:
        required: false
`
	return manifest, "#!/usr/bin/env bash\necho uploaded\n"
}

func unsafeZip(t *testing.T, name string) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	writeZipFile(t, writer, name, "id: bad\n")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func writeZipFile(t *testing.T, writer *zip.Writer, name, content string) {
	t.Helper()
	file, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
}

func installDemoToolPlugin(t *testing.T, baseDir string) {
	t.Helper()
	pluginDir := filepath.Join(baseDir, "plugins", "demo")
	if err := os.MkdirAll(filepath.Join(pluginDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `id: demo
name: Demo Tools
version: 1.0.0
contributes:
  categories:
    - id: demo
      name: 演示
  tools:
    - id: demo.hello
      name: 问候
      category: demo
      command: scripts/run.sh
      workdir: .
      timeout: 30m
      tags: [工具标签]
      confirm:
        required: false
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "scripts", "run.sh"), []byte("#!/usr/bin/env bash\necho demo\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func installTestPlugin(t *testing.T, baseDir, id, version string) {
	t.Helper()
	pluginDir := filepath.Join(baseDir, "plugins", id)
	if err := os.MkdirAll(filepath.Join(pluginDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `id: ` + id + `
name: Existing Test
version: ` + version + `
contributes:
  categories:
    - id: upload
      name: 上传插件
  tools:
    - id: ` + id + `.tool
      name: 已有工具
      category: upload
      command: scripts/run.sh
      workdir: .
      timeout: 30m
      confirm:
        required: false
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "scripts", "run.sh"), []byte("#!/usr/bin/env bash\necho existing\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func buildPluginExportZipMustFail(reg *registry.Registry, pluginID string) error {
	_, err := buildPluginExportZip(reg, pluginID)
	return err
}

func installTestPluginWithWorkflow(t *testing.T, baseDir, id, version string) {
	t.Helper()
	pluginDir := filepath.Join(baseDir, "plugins", id)
	if err := os.MkdirAll(filepath.Join(pluginDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `id: ` + id + `
name: Export Test
version: ` + version + `
description: Plugin export fixture
contributes:
  categories:
    - id: upload
      name: 上传插件
  tools:
    - id: ` + id + `.tool
      name: 导出工具
      category: upload
      command: scripts/run.sh
      workdir: .
      timeout: 30m
      confirm:
        required: false
  workflows:
    - path: workflows/flow.yaml
`
	workflow := `id: ` + id + `.flow
name: 导出工作流
category: upload
nodes:
  - id: first
    tool: ` + id + `.tool
edges: []
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "scripts", "run.sh"), []byte("#!/usr/bin/env bash\necho export\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "workflows", "flow.yaml"), []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "README.md"), []byte("# Export Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func decodeResponse(t *testing.T, body *bytes.Buffer) response {
	t.Helper()
	var out response
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

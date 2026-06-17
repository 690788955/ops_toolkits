package server

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
	"shell_ops/internal/runner"
)

func TestWebIndexDisablesBrowserCache(t *testing.T) {
	if os.Getenv("OPS_TEST_HELPER_OUTPUT") != "" {
		io.WriteString(os.Stdout, os.Getenv("OPS_TEST_HELPER_OUTPUT")+"\n")
		os.Exit(0)
	}
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

func TestUIPreferencesAPIReadsLogFontSizeFromUIConfig(t *testing.T) {
	cases := []struct {
		name        string
		logFontSize int
		want        string
	}{
		{name: "default", logFontSize: 0, want: `"log_font_size":14`},
		{name: "configured", logFontSize: 16, want: `"log_font_size":16`},
		{name: "too small", logFontSize: 11, want: `"log_font_size":14`},
		{name: "too large", logFontSize: 30, want: `"log_font_size":14`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := testRegistry(t)
			reg.Root.UI.LogFontSize = tc.logFontSize
			req := httptest.NewRequest(http.MethodGet, "/api/ui/preferences", nil)
			res := httptest.NewRecorder()

			NewHandler(reg).ServeHTTP(res, req)

			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
			}
			if !strings.Contains(res.Body.String(), tc.want) {
				t.Fatalf("响应缺少 %s: %s", tc.want, res.Body.String())
			}
		})
	}
}

func TestCatalogAPIKeepsPluginToolDeclarationOrder(t *testing.T) {
	dir := t.TempDir()
	writeTestRootConfig(t, dir)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.order", "scripts", "run.sh"), "#!/usr/bin/env bash\necho order\n")
	writeFile(t, filepath.Join(dir, "plugins", "vendor.order", "plugin.yaml"), `id: vendor.order
name: Ordered Tools
version: 1.0.0
contributes:
  categories:
    - id: ordered
      name: 有序工具
  tools:
    - id: vendor.order.third
      name: 第三个字母工具
      category: ordered
      command: scripts/run.sh
    - id: vendor.order.first
      name: 第一个字母工具
      category: ordered
      command: scripts/run.sh
    - id: vendor.order.second
      name: 第二个字母工具
      category: ordered
      command: scripts/run.sh
`)
	reg, err := registry.Load(dir)
	if err != nil {
		t.Fatalf("加载注册表失败: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data struct {
			Tools []struct {
				ID string `json:"id"`
			} `json:"tools"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 catalog 响应失败: %v", err)
	}
	got := []string{}
	for _, tool := range body.Data.Tools {
		got = append(got, tool.ID)
	}
	want := []string{"vendor.order.third", "vendor.order.first", "vendor.order.second"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("catalog 工具顺序 = %v, want %v", got, want)
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

func TestWorkflowRunAPIAsyncReturnsRunningRecordAndPersistsLogs(t *testing.T) {
	reg := testRegistry(t)
	configureHelperTool(t, reg, "demo.hello", "async-run")
	body := `{"params":{},"workflow":{"id":"demo.async","name":"异步草稿","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.async/run?async=true", strings.NewReader(body))
	res := httptest.NewRecorder()

	handler := NewHandler(reg)
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var started response
	if err := json.Unmarshal(res.Body.Bytes(), &started); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if started.ID == "" || started.Status != "running" {
		t.Fatalf("async response = %#v, want running id", started)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		runReq := httptest.NewRequest(http.MethodGet, "/api/runs/"+started.ID, nil)
		runRes := httptest.NewRecorder()
		handler.ServeHTTP(runRes, runReq)
		if runRes.Code == http.StatusOK && strings.Contains(runRes.Body.String(), `"status":"succeeded"`) && strings.Contains(runRes.Body.String(), "async-run") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("异步运行未在超时内完成，last status=%d body=%s", runRes.Code, runRes.Body.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRunNodeRerunAPIOverwritesSameRun(t *testing.T) {
	reg := testRegistry(t)
	configureHelperTool(t, reg, "demo.hello", "rerun-api")
	body := `{"params":{},"workflow":{"id":"demo.rerun.api","name":"重跑流程","nodes":[{"id":"first","tool":"demo.hello"},{"id":"second","tool":"demo.hello","params":{"input":"{{ .steps.first.stdout }}"}}],"edges":[{"from":"first","to":"second"}]}}`
	handler := NewHandler(reg)
	runReq := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.rerun.api/run", strings.NewReader(body))
	runRes := httptest.NewRecorder()
	handler.ServeHTTP(runRes, runReq)
	if runRes.Code != http.StatusOK {
		t.Fatalf("run status = %d, body = %s", runRes.Code, runRes.Body.String())
	}
	var started response
	if err := json.Unmarshal(runRes.Body.Bytes(), &started); err != nil {
		t.Fatalf("解析运行响应失败: %v", err)
	}
	if started.ID == "" || started.Status != "succeeded" {
		t.Fatalf("run response = %#v", started)
	}

	rerunReq := httptest.NewRequest(http.MethodPost, "/api/runs/"+started.ID+"/nodes/second/rerun", strings.NewReader(body))
	rerunRes := httptest.NewRecorder()
	handler.ServeHTTP(rerunRes, rerunReq)
	if rerunRes.Code != http.StatusOK {
		t.Fatalf("rerun status = %d, body = %s", rerunRes.Code, rerunRes.Body.String())
	}
	if !strings.Contains(rerunRes.Body.String(), `"id":"`+started.ID+`"`) || !strings.Contains(rerunRes.Body.String(), `"status":"succeeded"`) || !strings.Contains(rerunRes.Body.String(), "rerun-api") {
		t.Fatalf("rerun response missing same run success detail: %s", rerunRes.Body.String())
	}
	detailRes := httptest.NewRecorder()
	handler.ServeHTTP(detailRes, httptest.NewRequest(http.MethodGet, "/api/runs/"+started.ID, nil))
	if detailRes.Code != http.StatusOK || !strings.Contains(detailRes.Body.String(), `"target":"demo.rerun.api"`) {
		t.Fatalf("detail status = %d, body = %s", detailRes.Code, detailRes.Body.String())
	}
}

func TestRunNodeRerunAPIRejectsNonPost(t *testing.T) {
	reg := testRegistry(t)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/api/runs/run-1/nodes/step-1/rerun", nil))

	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("rerun PUT status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestRunNodeRerunAPIUsesOverrideParams(t *testing.T) {
	reg := testRegistry(t)
	toolDir := reg.Tools["demo.hello"].Dir
	if err := os.MkdirAll(filepath.Join(toolDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "bin", "run.sh"), []byte("#!/usr/bin/env bash\nset -euo pipefail\necho \"name=${OPS_PARAM_NAME}\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"params":{"name":"old"},"workflow":{"id":"demo.rerun.params","name":"重跑参数流程","parameters":[{"name":"name","type":"string","default":"old"}],"nodes":[{"id":"first","tool":"demo.hello","params":{"name":"{{ .name }}"}},{"id":"second","tool":"demo.hello","params":{"name":"{{ .name }}"}}],"edges":[{"from":"first","to":"second"}]}}`
	handler := NewHandler(reg)
	runReq := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.rerun.params/run", strings.NewReader(body))
	runRes := httptest.NewRecorder()
	handler.ServeHTTP(runRes, runReq)
	if runRes.Code != http.StatusOK {
		t.Fatalf("run status = %d, body = %s", runRes.Code, runRes.Body.String())
	}
	var started response
	if err := json.Unmarshal(runRes.Body.Bytes(), &started); err != nil {
		t.Fatalf("解析运行响应失败: %v", err)
	}

	rerunBody := strings.Replace(body, `"name":"old"`, `"name":"new"`, 1)
	rerunReq := httptest.NewRequest(http.MethodPost, "/api/runs/"+started.ID+"/nodes/second/rerun", strings.NewReader(rerunBody))
	rerunRes := httptest.NewRecorder()
	handler.ServeHTTP(rerunRes, rerunReq)
	if rerunRes.Code != http.StatusOK {
		t.Fatalf("rerun status = %d, body = %s", rerunRes.Code, rerunRes.Body.String())
	}
	detailRes := httptest.NewRecorder()
	handler.ServeHTTP(detailRes, httptest.NewRequest(http.MethodGet, "/api/runs/"+started.ID, nil))
	bodyText := detailRes.Body.String()
	if detailRes.Code != http.StatusOK || !strings.Contains(bodyText, `"name":"new"`) || !strings.Contains(bodyText, "name=new") {
		t.Fatalf("detail status = %d, body = %s", detailRes.Code, bodyText)
	}
}

func TestToolRunAPIAsyncReturnsRunningRecordAndPersistsLogs(t *testing.T) {
	reg := testRegistry(t)
	configureHelperTool(t, reg, "demo.hello", "async-tool")
	req := httptest.NewRequest(http.MethodPost, "/api/tools/demo.hello/run?async=true", strings.NewReader(`{"params":{}}`))
	res := httptest.NewRecorder()

	handler := NewHandler(reg)
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var started response
	if err := json.Unmarshal(res.Body.Bytes(), &started); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if started.ID == "" || started.Status != "running" {
		t.Fatalf("async tool response = %#v, want running id", started)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		runReq := httptest.NewRequest(http.MethodGet, "/api/runs/"+started.ID, nil)
		runRes := httptest.NewRecorder()
		handler.ServeHTTP(runRes, runReq)
		if runRes.Code == http.StatusOK && strings.Contains(runRes.Body.String(), `"status":"succeeded"`) && strings.Contains(runRes.Body.String(), "async-tool") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("异步工具运行未在超时内完成，last status=%d body=%s", runRes.Code, runRes.Body.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func configureHelperTool(t *testing.T, reg *registry.Registry, id, output string) {
	t.Helper()
	tool := reg.Tools[id]
	if tool == nil {
		t.Fatalf("missing test tool %s", id)
	}
	helper, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test helper executable: %v", err)
	}
	tool.Config.Execution.Entry = helper
	tool.Config.Execution.Args = []string{"-test.run=TestWebIndexDisablesBrowserCache"}
	tool.Config.Env = map[string]string{"OPS_TEST_HELPER_OUTPUT": output}
	tool.Config.PassMode = config.PassMode{}
	if strings.Contains(helper, string(filepath.Separator)) {
		tool.Dir = filepath.Dir(helper)
	}
}

func TestRunCancelAPIStopsAsyncWorkflow(t *testing.T) {
	reg := testRegistry(t)
	toolDir := reg.Tools["demo.hello"].Dir
	if err := os.MkdirAll(filepath.Join(toolDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "bin", "run.sh"), []byte("#!/usr/bin/env bash\nsleep 10 &\nwait\necho should-not-finish\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"params":{},"workflow":{"id":"demo.cancel","name":"取消草稿","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`
	handler := NewHandler(reg)
	startReq := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.cancel/run?async=true", strings.NewReader(body))
	startRes := httptest.NewRecorder()

	handler.ServeHTTP(startRes, startReq)

	if startRes.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", startRes.Code, startRes.Body.String())
	}
	var started response
	if err := json.Unmarshal(startRes.Body.Bytes(), &started); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	cancelReq := httptest.NewRequest(http.MethodPost, "/api/runs/"+started.ID+"/cancel", strings.NewReader(`{}`))
	cancelRes := httptest.NewRecorder()
	handler.ServeHTTP(cancelRes, cancelReq)
	if cancelRes.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, body = %s", cancelRes.Code, cancelRes.Body.String())
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		runReq := httptest.NewRequest(http.MethodGet, "/api/runs/"+started.ID, nil)
		runRes := httptest.NewRecorder()
		handler.ServeHTTP(runRes, runReq)
		if runRes.Code == http.StatusOK && strings.Contains(runRes.Body.String(), `"status":"cancelled"`) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("异步运行未在超时内取消，last status=%d body=%s", runRes.Code, runRes.Body.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestWatchRunCompletionIgnoresTransientReadErrors(t *testing.T) {
	dir := t.TempDir()
	reg := testRegistry(t)
	reg.BaseDir = dir
	runDir := filepath.Join(dir, "runs", "logs", "workflow-transient")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	record := runner.RunRecord{ID: "workflow-transient", Status: "running"}
	if err := os.WriteFile(filepath.Join(runDir, "result.json"), []byte(`{"id":"workflow-transient","kind":"workflow","status":"running","steps":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	state := newServerState(reg)
	cancelCount := 0
	cancel := func() { cancelCount++ }

	done := make(chan struct{})
	go func() {
		watchRunCompletion(state, reg, record.ID, cancel)
		close(done)
	}()

	_ = os.Remove(filepath.Join(runDir, "result.json"))
	time.Sleep(150 * time.Millisecond)
	if cancelCount != 0 {
		t.Fatalf("cancelCount = %d, want 0 after transient missing record", cancelCount)
	}

	if err := os.WriteFile(filepath.Join(runDir, "result.json"), []byte(`{"id":"workflow-transient","kind":"workflow","status":"succeeded","steps":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchRunCompletion did not exit after record finished")
	}
	if cancelCount != 1 {
		t.Fatalf("cancelCount = %d, want 1 after completion", cancelCount)
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

func TestWorkflowDeleteAPIRemovesUserWorkflowAsset(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	saveReq := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.deleted/save", strings.NewReader(`{"workflow":{"id":"demo.deleted","name":"待删除","category":"demo","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`))
	saveRes := httptest.NewRecorder()
	handler.ServeHTTP(saveRes, saveReq)
	if saveRes.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", saveRes.Code, saveRes.Body.String())
	}
	workflowPath := filepath.Join(reg.BaseDir, "plugins", "user.workflows", "workflows", "demo.deleted.yaml")
	if _, err := os.Stat(workflowPath); err != nil {
		t.Fatalf("未找到待删除工作流: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/workflows/demo.deleted", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(workflowPath); !os.IsNotExist(err) {
		t.Fatalf("工作流文件未被删除，err=%v", err)
	}
	if got := reg.Workflows["demo.deleted"]; got != nil {
		t.Fatalf("注册表仍保留已删除工作流: %#v", got)
	}
	manifest, err := os.ReadFile(filepath.Join(reg.BaseDir, "plugins", "user.workflows", "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), "workflows/demo.deleted.yaml") {
		t.Fatalf("用户工作流插件清单仍包含已删除工作流: %s", string(manifest))
	}
}

func TestWorkflowDeleteAPIRejectsNonUserWorkflow(t *testing.T) {
	reg := testRegistry(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/workflows/demo.flow", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if got := reg.Workflows["demo.flow"]; got == nil {
		t.Fatalf("非用户工作流不应被删除")
	}
}

func TestWorkflowDeleteAPIUnknownWorkflow(t *testing.T) {
	reg := testRegistry(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/workflows/demo.missing", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
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
	for _, name := range []string{"README.md", "SPEC.md", "plugins/plugin.template/plugin.yaml", "plugins/plugin.template/config/example.conf", "plugins/plugin.template/scripts/run.sh", "plugins/plugin.template/workflows/maintenance-flow.yaml", "plugins/plugin.template/README.md", "plugins/plugin.template/examples/params.yaml", "plugins/plugin.template/examples/README.md"} {
		if !entries[name] {
			t.Fatalf("开发包缺少文件 %s", name)
		}
	}
	combined := strings.Join([]string{
		contents["README.md"],
		contents["SPEC.md"],
		contents["plugins/plugin.template/plugin.yaml"],
		contents["plugins/plugin.template/config/example.conf"],
		contents["plugins/plugin.template/scripts/run.sh"],
		contents["plugins/plugin.template/workflows/maintenance-flow.yaml"],
		contents["plugins/plugin.template/README.md"],
		contents["plugins/plugin.template/examples/README.md"],
	}, "\n")
	if !strings.Contains(contents["SPEC.md"], "config_dir: config") || !strings.Contains(contents["SPEC.md"], "config_files:") || !strings.Contains(contents["SPEC.md"], "- example.conf") {
		t.Fatalf("SPEC.md 缺少 config_dir 推荐写法: %s", contents["SPEC.md"])
	}
	if !strings.Contains(contents["plugins/plugin.template/plugin.yaml"], "config_dir: config") || !strings.Contains(contents["plugins/plugin.template/plugin.yaml"], "- example.conf") {
		t.Fatalf("模板 plugin.yaml 缺少 config_dir 推荐写法: %s", contents["plugins/plugin.template/plugin.yaml"])
	}
	if !strings.Contains(contents["plugins/plugin.template/README.md"], "config_dir: config") {
		t.Fatalf("模板 README 缺少 config_dir 说明: %s", contents["plugins/plugin.template/README.md"])
	}
	for _, want := range []string{"插件开发包", "plugin.yaml", "规范插件模板", "可复制的规范模板", "id: plugin.template", "name: 规范插件模板", "version: 1.0.0", "description:", "author: your-team", "compatibility:", "contributes:", "categories:", "tools:", "workflows:", "plugin.template.inspect", "plugin.template.apply", "plugin.template.maintenance-flow", "confirm.required", "required: true", "default: demo", "type: bool", "timeout: 5m", "tags: [plugin, template, change, high-risk]", "command: scripts/run.sh", "workdir: .", "args:", "depends_on: [inspect]", "from: inspect", "to: apply", "usage()", "error()", "normalize_bool()", "未知参数", "缺少必填参数 target", "action 只支持 inspect 或 apply", "dry-run 只接受 true/false、yes/no、1/0、on/off", "dry-run", "config_dir: config", "config_files:", "config/example.conf", "配置文件", "不要在 stdout/stderr 输出密码", "./bin/opsctl.exe validate", "./bin/opsctl.exe run tool plugin.template.inspect", "./bin/opsctl.exe run workflow plugin.template.maintenance-flow", "./bin/opsctl.exe package build", "插件开发者交付清单", "更新已存在插件时提升 version", "不要假设交付或接入时会执行脚本", "宿主运行环境", "打包交付", "command、workdir、workflow path 都应留在插件目录内部"} {
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

func TestToolDevKitTemplateCanRunWithoutModificationViaInstall(t *testing.T) {
	baseReg := testRegistry(t)
	handler := NewHandler(baseReg)

	devkitReq := httptest.NewRequest(http.MethodGet, "/api/dev/toolkit.zip", nil)
	devkitRes := httptest.NewRecorder()
	handler.ServeHTTP(devkitRes, devkitReq)
	if devkitRes.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", devkitRes.Code, devkitRes.Body.String())
	}

	staging := t.TempDir()
	if err := extractPluginZip(devkitRes.Body.Bytes(), staging); err != nil {
		t.Fatalf("解压模板开发包失败: %v", err)
	}
	srcPluginDir := filepath.Join(staging, "plugins", "plugin.template")
	if _, err := os.Stat(filepath.Join(srcPluginDir, "plugin.yaml")); err != nil {
		t.Fatalf("模板插件目录缺失: %v", err)
	}
	dstPluginDir := filepath.Join(baseReg.BaseDir, "plugins", "plugin.template")
	if err := copyDir(srcPluginDir, dstPluginDir); err != nil {
		t.Fatalf("安装模板插件失败: %v", err)
	}

	reloaded, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("模板插件安装后 validate/load 失败: %v", err)
	}
	if _, ok := reloaded.Tools["plugin.template.inspect"]; !ok {
		t.Fatalf("模板工具未注册")
	}
	if _, ok := reloaded.Workflows["plugin.template.maintenance-flow"]; !ok {
		t.Fatalf("模板工作流未注册")
	}

	r := runner.New(reloaded)
	toolRecord, err := r.RunTool(context.Background(), "plugin.template.inspect", map[string]string{
		"target":  "demo",
		"action":  "inspect",
		"dry_run": "true",
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("模板工具试跑失败: %v, record=%#v", err, toolRecord)
	}
	if toolRecord == nil || toolRecord.Status != "succeeded" {
		t.Fatalf("模板工具试跑状态异常: %#v", toolRecord)
	}

	workflowRecord, err := r.RunWorkflowWithConfirmation(context.Background(), "plugin.template.maintenance-flow", map[string]string{
		"target":  "demo",
		"action":  "apply",
		"dry_run": "true",
	}, true, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("模板工作流试跑失败: %v, record=%#v", err, workflowRecord)
	}
	if workflowRecord == nil || workflowRecord.Status != "succeeded" {
		t.Fatalf("模板工作流试跑状态异常: %#v", workflowRecord)
	}
}

func TestToolDevKitTemplateCanRunWithoutModificationViaUpload(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)

	devkitReq := httptest.NewRequest(http.MethodGet, "/api/dev/toolkit.zip", nil)
	devkitRes := httptest.NewRecorder()
	handler.ServeHTTP(devkitRes, devkitReq)
	if devkitRes.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", devkitRes.Code, devkitRes.Body.String())
	}

	uploadReq := pluginUploadRequest(t, devkitRes.Body.Bytes(), false)
	uploadRes := httptest.NewRecorder()
	handler.ServeHTTP(uploadRes, uploadReq)
	if uploadRes.Code != http.StatusOK {
		t.Fatalf("上传模板插件失败 status = %d, body = %s", uploadRes.Code, uploadRes.Body.String())
	}
	if !strings.Contains(uploadRes.Body.String(), `"plugin_id":"plugin.template"`) {
		t.Fatalf("上传返回缺少模板插件 ID: %s", uploadRes.Body.String())
	}

	toolReq := httptest.NewRequest(http.MethodPost, "/api/tools/plugin.template.inspect/run", strings.NewReader(`{"params":{"target":"demo","action":"inspect","dry_run":"true"}}`))
	toolRes := httptest.NewRecorder()
	handler.ServeHTTP(toolRes, toolReq)
	if toolRes.Code != http.StatusOK {
		t.Fatalf("模板工具 API 试跑失败 status = %d, body = %s", toolRes.Code, toolRes.Body.String())
	}
	if !strings.Contains(toolRes.Body.String(), `"status":"succeeded"`) {
		t.Fatalf("模板工具 API 试跑未成功: %s", toolRes.Body.String())
	}

	workflowReq := httptest.NewRequest(http.MethodPost, "/api/workflows/plugin.template.maintenance-flow/run", strings.NewReader(`{"confirm":true,"params":{"target":"demo","action":"apply","dry_run":"true"}}`))
	workflowRes := httptest.NewRecorder()
	handler.ServeHTTP(workflowRes, workflowReq)
	if workflowRes.Code != http.StatusOK {
		t.Fatalf("模板工作流 API 试跑失败 status = %d, body = %s", workflowRes.Code, workflowRes.Body.String())
	}
	if !strings.Contains(workflowRes.Body.String(), `"status":"succeeded"`) {
		t.Fatalf("模板工作流 API 试跑未成功: %s", workflowRes.Body.String())
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

func TestPluginUploadReloadKeepsPluginToolDeclarationOrder(t *testing.T) {
	reg := testRegistry(t)
	req := pluginUploadRequest(t, pluginZipWithToolIDs(t, "vendor.uploadorder", "1.0.0", []string{"third", "first", "second"}), false)
	res := httptest.NewRecorder()
	handler := NewHandler(reg)

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", res.Code, res.Body.String())
	}
	catalogRes := httptest.NewRecorder()
	handler.ServeHTTP(catalogRes, httptest.NewRequest(http.MethodGet, "/api/catalog", nil))
	if catalogRes.Code != http.StatusOK {
		t.Fatalf("catalog status = %d, body = %s", catalogRes.Code, catalogRes.Body.String())
	}
	var body struct {
		Data struct {
			Tools []struct {
				ID string `json:"id"`
			} `json:"tools"`
		} `json:"data"`
	}
	if err := json.Unmarshal(catalogRes.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 catalog 响应失败: %v", err)
	}
	got := []string{}
	for _, tool := range body.Data.Tools {
		got = append(got, tool.ID)
	}
	want := []string{"vendor.uploadorder.third", "vendor.uploadorder.first", "vendor.uploadorder.second"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("上传 reload 后 catalog 工具顺序 = %v, want %v", got, want)
	}
}

func TestPluginUploadAcceptsAllowedBareCommand(t *testing.T) {
	reg := testRegistry(t)
	reg.Root.Plugins.AllowedCommands = []string{"ansible-playbook"}
	rootConfig := filepath.Join(reg.BaseDir, "configs", "ops.yaml")
	content, err := os.ReadFile(rootConfig)
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), "  disabled: []\n", "  disabled: []\n  allowed_commands:\n    - ansible-playbook\n", 1))
	if err := os.WriteFile(rootConfig, content, 0o644); err != nil {
		t.Fatal(err)
	}
	req := pluginUploadRequest(t, pluginZipWithCommand(t, "vendor.pathupload", "1.0.0", "ansible-playbook"), false)
	res := httptest.NewRecorder()
	handler := NewHandler(reg)

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	catalogRes := httptest.NewRecorder()
	handler.ServeHTTP(catalogRes, httptest.NewRequest(http.MethodGet, "/api/catalog", nil))
	if !strings.Contains(catalogRes.Body.String(), "vendor.pathupload.tool") {
		t.Fatalf("catalog 缺少 PATH command 上传插件贡献: %s", catalogRes.Body.String())
	}
}

func TestPluginUploadReturnsWarningsAndCatalogIncludesWarnings(t *testing.T) {
	reg := testRegistry(t)
	req := pluginUploadRequest(t, pluginZipWithMissingConfig(t, "vendor.warnupload", "1.0.0"), false)
	res := httptest.NewRecorder()
	handler := NewHandler(reg)

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, `"warnings"`) || !strings.Contains(body, "CONFIG_FILE_MISSING") {
		t.Fatalf("上传响应缺少 warnings: %s", body)
	}

	catalogRes := httptest.NewRecorder()
	handler.ServeHTTP(catalogRes, httptest.NewRequest(http.MethodGet, "/api/catalog", nil))
	catalogBody := catalogRes.Body.String()
	if !strings.Contains(catalogBody, `"warnings"`) || !strings.Contains(catalogBody, "vendor.warnupload") || !strings.Contains(catalogBody, "CONFIG_FILE_MISSING") {
		t.Fatalf("catalog 缺少插件 warnings: %s", catalogBody)
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

func TestPluginRuntimePackageZipContainsSinglePluginRuntime(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPluginWithWorkflow(t, baseReg.BaseDir, "vendor.runtime", "1.0.0")
	installTestPluginWithWorkflow(t, baseReg.BaseDir, "vendor.other", "1.0.0")
	writeRuntimeBinary(t, baseReg.BaseDir, "linux", "amd64")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}

	data, err := buildPluginRuntimePackage(reg, "vendor.runtime", "linux", "amd64", "zip")
	if err != nil {
		t.Fatalf("构建运行包失败: %v", err)
	}
	entries := zipEntries(t, data)
	for _, name := range []string{
		"opsctl/bin/opsctl",
		"opsctl/configs/ops.yaml",
		"opsctl/README.md",
		"opsctl/plugins/vendor.runtime/plugin.yaml",
		"opsctl/plugins/vendor.runtime/scripts/run.sh",
		"opsctl/plugins/vendor.runtime/workflows/flow.yaml",
	} {
		if !entries[name] {
			t.Fatalf("运行包缺少文件 %s，entries=%v", name, entries)
		}
	}
	if entries["opsctl/plugins/vendor.other/plugin.yaml"] {
		t.Fatalf("运行包不应包含未选择插件: entries=%v", entries)
	}
	configData := zipEntryData(t, data, "opsctl/configs/ops.yaml")
	for _, want := range []string{"plugins:", "paths:", "- plugins", "host_config_files:", "allowed_dirs: []"} {
		if !strings.Contains(string(configData), want) {
			t.Fatalf("最小配置缺少 %q:\n%s", want, string(configData))
		}
	}
	if strings.Contains(string(configData), "vendor.other") || strings.Contains(string(configData), "vendor.runtime") {
		t.Fatalf("最小配置不应写入插件 ID: %s", string(configData))
	}
	readme := string(zipEntryData(t, data, "opsctl/README.md"))
	if !strings.Contains(readme, "vendor.runtime") || !strings.Contains(readme, "外部系统命令") {
		t.Fatalf("README 内容不完整: %s", readme)
	}
}

func TestPluginRuntimePackageTarGzContainsRuntime(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPluginWithWorkflow(t, baseReg.BaseDir, "vendor.runtime", "1.0.0")
	writeRuntimeBinary(t, baseReg.BaseDir, "windows", "amd64")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}

	data, err := buildPluginRuntimePackage(reg, "vendor.runtime", "windows", "amd64", "tar.gz")
	if err != nil {
		t.Fatalf("构建运行包失败: %v", err)
	}
	entries := tarGzEntries(t, data)
	for _, name := range []string{"opsctl/bin/opsctl.exe", "opsctl/configs/ops.yaml", "opsctl/plugins/vendor.runtime/plugin.yaml"} {
		if !entries[name] {
			t.Fatalf("运行包 tar.gz 缺少文件 %s，entries=%v", name, entries)
		}
	}
}

func TestPluginRuntimePackageRejectsMissingBinary(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPluginWithWorkflow(t, baseReg.BaseDir, "vendor.runtime", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}

	_, err = buildPluginRuntimePackage(reg, "vendor.runtime", "linux", "amd64", "zip")
	if err == nil || !strings.Contains(err.Error(), "二进制不存在") {
		t.Fatalf("err = %v, want 二进制不存在", err)
	}
}

func TestPluginRuntimeDownloadRoute(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPluginWithWorkflow(t, baseReg.BaseDir, "vendor.runtime", "1.0.0")
	writeRuntimeBinary(t, baseReg.BaseDir, "linux", "amd64")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.runtime/runtime.zip?goos=linux&goarch=amd64", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if contentType := res.Header().Get("Content-Type"); contentType != "application/zip" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	if disposition := res.Header().Get("Content-Disposition"); !strings.Contains(disposition, "vendor.runtime-opsctl-linux-amd64.zip") {
		t.Fatalf("Content-Disposition = %s", disposition)
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

func TestPluginDisableAddsConfigAndRefreshesCatalog(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPlugin(t, baseReg.BaseDir, "vendor.disable", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	handler := NewHandler(reg)
	req := httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.disable/disable", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	rootConfig, err := os.ReadFile(filepath.Join(reg.BaseDir, "configs", "ops.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rootConfig), "id: upload") {
		t.Fatalf("禁用写配置不应把运行期插件分类写入磁盘: %s", rootConfig)
	}
	if !strings.Contains(string(rootConfig), "vendor.disable") {
		t.Fatalf("禁用配置未写入插件 ID: %s", rootConfig)
	}
	catalogReq := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	catalogRes := httptest.NewRecorder()
	handler.ServeHTTP(catalogRes, catalogReq)
	body := catalogRes.Body.String()
	if strings.Contains(body, "vendor.disable.tool") {
		t.Fatalf("禁用插件贡献不应继续出现在 catalog: %s", body)
	}
	if !strings.Contains(body, "vendor.disable") || !strings.Contains(body, `"disabled":true`) {
		t.Fatalf("catalog 应包含已禁用插件状态: %s", body)
	}
	var catalogBody struct {
		Data struct {
			Categories []categoryCatalogEntry `json:"categories"`
			Plugins    []pluginCatalogEntry   `json:"plugins"`
		} `json:"data"`
	}
	if err := json.Unmarshal(catalogRes.Body.Bytes(), &catalogBody); err != nil {
		t.Fatalf("catalog JSON 解析失败: %v", err)
	}
	var disabledPlugin *pluginCatalogEntry
	for i := range catalogBody.Data.Plugins {
		if catalogBody.Data.Plugins[i].ID == "vendor.disable" {
			disabledPlugin = &catalogBody.Data.Plugins[i]
			break
		}
	}
	if disabledPlugin == nil || !disabledPlugin.Disabled {
		t.Fatalf("catalog 应包含已禁用插件状态: %#v", catalogBody.Data.Plugins)
	}
	var disabledCategory *categoryCatalogEntry
	for i := range catalogBody.Data.Categories {
		if catalogBody.Data.Categories[i].ID == "upload" {
			disabledCategory = &catalogBody.Data.Categories[i]
			break
		}
	}
	if disabledCategory == nil || !disabledCategory.Disabled || disabledCategory.Source == nil {
		t.Fatalf("catalog 应保留禁用插件分类来源，供前端置灰: %#v", catalogBody.Data.Categories)
	}
	if disabledCategory.Source.PluginID != "vendor.disable" || disabledCategory.Source.PluginName != "Existing Test" || disabledCategory.Source.PluginVersion != "1.0.0" {
		t.Fatalf("禁用分类 source 缺少前端提示所需插件信息: %#v", disabledCategory.Source)
	}
}

func TestPluginCatalogKeepsSharedDisabledCategoryEnabled(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPlugin(t, baseReg.BaseDir, "vendor.disabledshared", "1.0.0")
	installTestPlugin(t, baseReg.BaseDir, "vendor.enabledshared", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	handler := NewHandler(reg)
	disableRes := httptest.NewRecorder()
	handler.ServeHTTP(disableRes, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.disabledshared/disable", nil))
	if disableRes.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disableRes.Code, disableRes.Body.String())
	}
	catalogRes := httptest.NewRecorder()
	handler.ServeHTTP(catalogRes, httptest.NewRequest(http.MethodGet, "/api/catalog", nil))
	if catalogRes.Code != http.StatusOK {
		t.Fatalf("catalog status = %d, body = %s", catalogRes.Code, catalogRes.Body.String())
	}

	var body struct {
		Data struct {
			Categories []categoryCatalogEntry `json:"categories"`
		} `json:"data"`
	}
	if err := json.Unmarshal(catalogRes.Body.Bytes(), &body); err != nil {
		t.Fatalf("catalog JSON 解析失败: %v", err)
	}
	var uploadCategory *categoryCatalogEntry
	for i := range body.Data.Categories {
		if body.Data.Categories[i].ID == "upload" {
			uploadCategory = &body.Data.Categories[i]
			break
		}
	}
	if uploadCategory == nil {
		t.Fatalf("catalog 缺少共享分类 upload: %s", catalogRes.Body.String())
	}
	if uploadCategory.Disabled || uploadCategory.Source != nil {
		t.Fatalf("仍有启用插件引用共享分类时不应置灰或设置禁用来源: %#v", uploadCategory)
	}
	bodyText := catalogRes.Body.String()
	if strings.Contains(bodyText, "vendor.disabledshared.tool") {
		t.Fatalf("禁用插件工具不应出现在 catalog: %s", bodyText)
	}
	if !strings.Contains(bodyText, "vendor.enabledshared.tool") {
		t.Fatalf("启用插件工具应继续出现在 catalog: %s", bodyText)
	}
}

func TestPluginEnableRemovesDisabledConfigAndRefreshesCatalog(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPlugin(t, baseReg.BaseDir, "vendor.enable", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	handler := NewHandler(reg)
	disableRes := httptest.NewRecorder()
	handler.ServeHTTP(disableRes, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.enable/disable", nil))
	if disableRes.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disableRes.Code, disableRes.Body.String())
	}
	enableRes := httptest.NewRecorder()

	handler.ServeHTTP(enableRes, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.enable/enable", nil))

	if enableRes.Code != http.StatusOK {
		t.Fatalf("enable status = %d, body = %s", enableRes.Code, enableRes.Body.String())
	}
	rootConfig, err := os.ReadFile(filepath.Join(reg.BaseDir, "configs", "ops.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rootConfig), "vendor.enable") {
		t.Fatalf("启用成功后应清理 disabled 配置: %s", rootConfig)
	}
	catalogRes := httptest.NewRecorder()
	handler.ServeHTTP(catalogRes, httptest.NewRequest(http.MethodGet, "/api/catalog", nil))
	body := catalogRes.Body.String()
	if !strings.Contains(body, "vendor.enable.tool") {
		t.Fatalf("启用后插件贡献应重新出现在 catalog: %s", body)
	}
	var catalogBody struct {
		Data struct {
			Plugins []pluginCatalogEntry `json:"plugins"`
		} `json:"data"`
	}
	if err := json.Unmarshal(catalogRes.Body.Bytes(), &catalogBody); err != nil {
		t.Fatalf("catalog JSON 解析失败: %v", err)
	}
	for _, plugin := range catalogBody.Data.Plugins {
		if plugin.ID == "vendor.enable" && plugin.Disabled {
			t.Fatalf("启用后插件不应继续标记 disabled: %#v", plugin)
		}
	}
}

func TestPlatformFileUploadStoresFileUnderRunsUploads(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	req := fileUploadRequest(t, "artifact.txt", []byte("payload"))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data config.WorkflowUploadResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.FileName != "artifact.txt" || body.Data.Path == "" || body.Data.Size != int64(len("payload")) {
		t.Fatalf("upload data = %#v", body.Data)
	}
	if !strings.Contains(filepath.ToSlash(body.Data.RelativePath), "runs/uploads/upload-") {
		t.Fatalf("relative path = %q, want runs/uploads", body.Data.RelativePath)
	}
	data, err := os.ReadFile(body.Data.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "payload" {
		t.Fatalf("uploaded content = %q", data)
	}
}

func TestPlatformFileUploadStoresFileUnderTargetDir(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	req := fileUploadRequestWithTargetDir(t, "asset.zip", []byte("zip"), "assets/release")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data config.WorkflowUploadResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	rel := filepath.ToSlash(body.Data.RelativePath)
	if !strings.Contains(rel, "runs/uploads/assets/release/upload-") || !strings.HasSuffix(rel, "/asset.zip") {
		t.Fatalf("relative path = %q, want target dir under uploads", rel)
	}
	if _, err := os.Stat(body.Data.Path); err != nil {
		t.Fatalf("uploaded file missing: %v", err)
	}
}

func TestPlatformFileUploadStoresMultipleFiles(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	req := multiFileUploadRequest(t, "batch", []uploadRequestFile{
		{Name: "a.txt", Data: []byte("a")},
		{Name: "b.txt", Data: []byte("bb")},
	})
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data config.WorkflowUploadResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Count != 2 || body.Data.TotalSize != 3 || len(body.Data.Files) != 2 {
		t.Fatalf("upload data = %#v", body.Data)
	}
	if body.Data.FileName != "a.txt" || body.Data.Size != 1 {
		t.Fatalf("single-file compatibility fields = %#v", body.Data)
	}
	for _, item := range body.Data.Files {
		if _, err := os.Stat(item.Path); err != nil {
			t.Fatalf("uploaded file missing: %s err=%v", item.Path, err)
		}
	}
}

func TestPlatformFileUploadStoresDirectoryRelativePaths(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	req := multiFileUploadRequest(t, "", []uploadRequestFile{
		{Name: "a.txt", RelativePath: "dir/a.txt", Data: []byte("a")},
		{Name: "b.txt", RelativePath: "dir/sub/b.txt", Data: []byte("bb")},
	})
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data config.WorkflowUploadResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Files) != 2 {
		t.Fatalf("files = %#v", body.Data.Files)
	}
	relPaths := []string{filepath.ToSlash(body.Data.Files[0].RelativePath), filepath.ToSlash(body.Data.Files[1].RelativePath)}
	if !strings.Contains(relPaths[0], "/dir/a.txt") || !strings.Contains(relPaths[1], "/dir/sub/b.txt") {
		t.Fatalf("relative paths = %#v", relPaths)
	}
}

func TestPlatformFileUploadRejectsInvalidRelativePath(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	req := multiFileUploadRequest(t, "", []uploadRequestFile{{Name: "bad.txt", RelativePath: "../bad.txt", Data: []byte("bad")}})
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("upload status = %d, want 400; body = %s", res.Code, res.Body.String())
	}
}

func TestPlatformFileUploadRejectsInvalidTargetDir(t *testing.T) {
	cases := []string{"..", "a/../b", "/tmp/uploads", `C:\tmp\uploads`, "http://example.com/a"}
	for _, targetDir := range cases {
		t.Run(targetDir, func(t *testing.T) {
			reg := testRegistry(t)
			handler := NewHandler(reg)
			req := fileUploadRequestWithTargetDir(t, "asset.zip", []byte("zip"), targetDir)
			res := httptest.NewRecorder()

			handler.ServeHTTP(res, req)

			if res.Code != http.StatusBadRequest {
				t.Fatalf("upload status = %d, want 400; body = %s", res.Code, res.Body.String())
			}
		})
	}
}

func TestPlatformFileUploadSanitizesFilename(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	req := fileUploadRequest(t, `..\secret.txt`, []byte("payload"))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data config.WorkflowUploadResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.FileName != "secret.txt" {
		t.Fatalf("filename = %q, want basename", body.Data.FileName)
	}
	uploadRoot := filepath.Join(reg.BaseDir, "runs", "uploads")
	absPath, err := filepath.Abs(body.Data.Path)
	if err != nil {
		t.Fatal(err)
	}
	absRoot, err := filepath.Abs(uploadRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(absPath, absRoot+string(os.PathSeparator)) {
		t.Fatalf("uploaded path escaped upload root: %s", body.Data.Path)
	}
}

func TestPlatformFileUploadRejectsNonPost(t *testing.T) {
	res := httptest.NewRecorder()
	NewHandler(testRegistry(t)).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/files/upload", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", res.Code)
	}
}

func TestRunUploadNodeAPIUploadsOnlyWhenStepWaiting(t *testing.T) {
	reg := testRegistry(t)
	configureHelperTool(t, reg, "demo.hello", "path=${OPS_PARAM_PATH}")
	handler := NewHandler(reg)
	body := `{"params":{},"workflow":{"id":"demo.upload.async","name":"上传流程","nodes":[{"id":"upload","type":"upload","upload":{"target_dir":"assets"}},{"id":"consume","tool":"demo.hello","params":{"path":"{{ .steps.upload.file.path }}"}}],"edges":[{"from":"upload","to":"consume"}]}}`
	startReq := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.upload.async/run?async=true", strings.NewReader(body))
	startRes := httptest.NewRecorder()

	handler.ServeHTTP(startRes, startReq)

	if startRes.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", startRes.Code, startRes.Body.String())
	}
	var started response
	if err := json.Unmarshal(startRes.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	waitForServerRunStepStatus(t, handler, started.ID, "upload", "waiting")

	uploadReq := fileUploadRequestWithTargetDir(t, "artifact.txt", []byte("payload"), "assets")
	uploadReq.URL.Path = "/api/runs/" + started.ID + "/uploads/upload"
	uploadRes := httptest.NewRecorder()
	handler.ServeHTTP(uploadRes, uploadReq)

	if uploadRes.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", uploadRes.Code, uploadRes.Body.String())
	}
	finalDetail := waitForServerRunStatus(t, handler, started.ID, "succeeded")
	if !strings.Contains(finalDetail.Logs.Steps["upload"].Stdout, `"filename":"artifact.txt"`) {
		t.Fatalf("upload stdout = %s", finalDetail.Logs.Steps["upload"].Stdout)
	}
	if consumeStep, ok := findRunStep(finalDetail.Record, "consume"); !ok || consumeStep.Status != "succeeded" {
		t.Fatalf("consume step = %#v ok=%v", consumeStep, ok)
	}
}

func TestRunUploadNodeAPIChunkedUploadCompletesWorkflow(t *testing.T) {
	reg := testRegistry(t)
	configureHelperTool(t, reg, "demo.hello", "chunked")
	handler := NewHandler(reg)
	body := `{"params":{},"workflow":{"id":"demo.upload.chunked","name":"分片上传流程","nodes":[{"id":"upload","type":"upload","upload":{"target_dir":"assets"}},{"id":"consume","tool":"demo.hello","params":{"path":"{{ .steps.upload.files.relative_paths }}"}}],"edges":[{"from":"upload","to":"consume"}]}}`
	startReq := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.upload.chunked/run?async=true", strings.NewReader(body))
	startRes := httptest.NewRecorder()
	handler.ServeHTTP(startRes, startReq)
	if startRes.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", startRes.Code, startRes.Body.String())
	}
	var started response
	if err := json.Unmarshal(startRes.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	waitForServerRunStepStatus(t, handler, started.ID, "upload", "waiting")

	startUploadBody := `{"target_dir":"assets","files":[{"name":"a.txt","relative_path":"dir/a.txt","size":5}]}`
	chunkStartReq := httptest.NewRequest(http.MethodPost, "/api/runs/"+started.ID+"/uploads/upload/start", strings.NewReader(startUploadBody))
	chunkStartRes := httptest.NewRecorder()
	handler.ServeHTTP(chunkStartRes, chunkStartReq)
	if chunkStartRes.Code != http.StatusOK {
		t.Fatalf("chunk start status = %d, body = %s", chunkStartRes.Code, chunkStartRes.Body.String())
	}
	var chunkStart struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(chunkStartRes.Body.Bytes(), &chunkStart); err != nil {
		t.Fatal(err)
	}
	chunkReq := httptest.NewRequest(http.MethodPost, "/api/runs/"+started.ID+"/uploads/upload/chunk?session_id="+chunkStart.Data.ID+"&file_index=0&offset=0", strings.NewReader("hello"))
	chunkRes := httptest.NewRecorder()
	handler.ServeHTTP(chunkRes, chunkReq)
	if chunkRes.Code != http.StatusOK {
		t.Fatalf("chunk status = %d, body = %s", chunkRes.Code, chunkRes.Body.String())
	}
	finishReq := httptest.NewRequest(http.MethodPost, "/api/runs/"+started.ID+"/uploads/upload/finish", strings.NewReader(`{"session_id":"`+chunkStart.Data.ID+`"}`))
	finishRes := httptest.NewRecorder()
	handler.ServeHTTP(finishRes, finishReq)
	if finishRes.Code != http.StatusOK {
		t.Fatalf("finish status = %d, body = %s", finishRes.Code, finishRes.Body.String())
	}

	finalDetail := waitForServerRunStatus(t, handler, started.ID, "succeeded")
	if !strings.Contains(finalDetail.Logs.Steps["upload"].Stdout, "dir/a.txt") {
		t.Fatalf("upload stdout = %s", finalDetail.Logs.Steps["upload"].Stdout)
	}
}

func TestPluginEnableRollsBackConfigOnReloadFailure(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPlugin(t, baseReg.BaseDir, "vendor.enablerollback", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	handler := NewHandler(reg)
	disableRes := httptest.NewRecorder()
	handler.ServeHTTP(disableRes, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.enablerollback/disable", nil))
	if disableRes.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disableRes.Code, disableRes.Body.String())
	}
	installTestPlugin(t, reg.BaseDir, "vendor.enablebad", "1.0.0")
	if err := os.WriteFile(filepath.Join(reg.BaseDir, "plugins", "vendor.enablebad", "plugin.yaml"), []byte("id: ../bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	enableRes := httptest.NewRecorder()

	handler.ServeHTTP(enableRes, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.enablerollback/enable", nil))

	if enableRes.Code != http.StatusBadRequest {
		t.Fatalf("enable status = %d, want bad request; body = %s", enableRes.Code, enableRes.Body.String())
	}
	rootConfig, err := os.ReadFile(filepath.Join(reg.BaseDir, "configs", "ops.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rootConfig), "vendor.enablerollback") {
		t.Fatalf("刷新失败时应回滚并保留 disabled 配置: %s", rootConfig)
	}
	catalogRes := httptest.NewRecorder()
	handler.ServeHTTP(catalogRes, httptest.NewRequest(http.MethodGet, "/api/catalog", nil))
	if strings.Contains(catalogRes.Body.String(), "vendor.enablerollback.tool") {
		t.Fatalf("启用刷新失败后旧注册表应保持禁用状态: %s", catalogRes.Body.String())
	}
}

func TestPluginEnableRejectUnknownUnsafeAndNonPost(t *testing.T) {
	reg := testRegistry(t)
	cases := []struct {
		method string
		path   string
		want   int
		text   string
	}{
		{http.MethodPost, "/api/plugins/vendor.missing/enable", http.StatusNotFound, "vendor.missing"},
		{http.MethodPost, "/api/plugins/vendor.enable.zip/enable", http.StatusBadRequest, "不要使用 .zip"},
		{http.MethodPost, "/api/plugins/vendor%2Fevil/enable", http.StatusBadRequest, "不安全路径字符"},
		{http.MethodGet, "/api/plugins/vendor.method/enable", http.StatusMethodNotAllowed, "method not allowed"},
	}
	for _, tc := range cases {
		res := httptest.NewRecorder()
		NewHandler(reg).ServeHTTP(res, httptest.NewRequest(tc.method, tc.path, nil))
		if res.Code != tc.want {
			t.Fatalf("%s %s status = %d, want %d; body = %s", tc.method, tc.path, res.Code, tc.want, res.Body.String())
		}
		if tc.text != "" && !strings.Contains(res.Body.String(), tc.text) {
			t.Fatalf("%s %s 响应缺少 %q: %s", tc.method, tc.path, tc.text, res.Body.String())
		}
	}
}
func TestPluginDeleteRemovesDisabledPluginAndCleansConfig(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPlugin(t, baseReg.BaseDir, "vendor.delete", "1.0.0")
	writeTestRootConfigWithCategories(t, baseReg.BaseDir, []config.Category{{ID: "demo", Name: "演示"}, {ID: "upload", Name: "上传插件"}})
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	handler := NewHandler(reg)
	disableReq := httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.delete/disable", nil)
	disableRes := httptest.NewRecorder()
	handler.ServeHTTP(disableRes, disableReq)
	if disableRes.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disableRes.Code, disableRes.Body.String())
	}
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/plugins/vendor.delete", nil)
	deleteRes := httptest.NewRecorder()

	handler.ServeHTTP(deleteRes, deleteReq)

	if deleteRes.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRes.Code, deleteRes.Body.String())
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "vendor.delete")); !os.IsNotExist(err) {
		t.Fatalf("插件目录应被删除，stat err = %v", err)
	}
	rootConfig, err := os.ReadFile(filepath.Join(reg.BaseDir, "configs", "ops.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rootConfig), "vendor.delete") || strings.Contains(string(rootConfig), "id: upload") {
		t.Fatalf("删除成功后应清理 disabled 配置和插件独有分类: %s", rootConfig)
	}
	catalogReq := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	catalogRes := httptest.NewRecorder()
	handler.ServeHTTP(catalogRes, catalogReq)
	if strings.Contains(catalogRes.Body.String(), "vendor.delete") || strings.Contains(catalogRes.Body.String(), `"id":"upload"`) {
		t.Fatalf("删除插件不应继续出现在 catalog 或残留独有分类: %s", catalogRes.Body.String())
	}
}

func TestPluginDeletePreservesUserCategoryThatOnlySharesID(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPlugin(t, baseReg.BaseDir, "vendor.keepcategory", "1.0.0")
	writeTestRootConfigWithCategories(t, baseReg.BaseDir, []config.Category{{ID: "demo", Name: "演示"}, {ID: "upload", Name: "用户自定义上传分类", Description: "手写分类不应因 ID 相同被删除"}})
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	handler := NewHandler(reg)
	disableRes := httptest.NewRecorder()
	handler.ServeHTTP(disableRes, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.keepcategory/disable", nil))
	if disableRes.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disableRes.Code, disableRes.Body.String())
	}
	deleteRes := httptest.NewRecorder()

	handler.ServeHTTP(deleteRes, httptest.NewRequest(http.MethodDelete, "/api/plugins/vendor.keepcategory", nil))

	if deleteRes.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRes.Code, deleteRes.Body.String())
	}
	rootConfig, err := os.ReadFile(filepath.Join(reg.BaseDir, "configs", "ops.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	rootText := string(rootConfig)
	if !strings.Contains(rootText, "id: upload") || !strings.Contains(rootText, "用户自定义上传分类") {
		t.Fatalf("删除插件不应误删用户手写的非插件分类: %s", rootText)
	}
	catalogRes := httptest.NewRecorder()
	handler.ServeHTTP(catalogRes, httptest.NewRequest(http.MethodGet, "/api/catalog", nil))
	if !strings.Contains(catalogRes.Body.String(), `"id":"upload"`) || strings.Contains(catalogRes.Body.String(), "vendor.keepcategory") {
		t.Fatalf("catalog 应保留用户分类并移除插件: %s", catalogRes.Body.String())
	}
}

func TestPluginDeleteKeepsSharedCategoryWhenEnabledPluginStillUsesIt(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPlugin(t, baseReg.BaseDir, "vendor.deleteshared", "1.0.0")
	installTestPlugin(t, baseReg.BaseDir, "vendor.keepshared", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	handler := NewHandler(reg)
	disableRes := httptest.NewRecorder()
	handler.ServeHTTP(disableRes, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.deleteshared/disable", nil))
	if disableRes.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disableRes.Code, disableRes.Body.String())
	}
	deleteRes := httptest.NewRecorder()

	handler.ServeHTTP(deleteRes, httptest.NewRequest(http.MethodDelete, "/api/plugins/vendor.deleteshared", nil))

	if deleteRes.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRes.Code, deleteRes.Body.String())
	}
	catalogRes := httptest.NewRecorder()
	handler.ServeHTTP(catalogRes, httptest.NewRequest(http.MethodGet, "/api/catalog", nil))
	body := catalogRes.Body.String()
	if strings.Contains(body, "vendor.deleteshared") {
		t.Fatalf("已删除插件不应继续出现在 catalog: %s", body)
	}
	if !strings.Contains(body, "vendor.keepshared.tool") || !strings.Contains(body, `"id":"upload"`) {
		t.Fatalf("共享分类仍有启用插件使用时应保留: %s", body)
	}
}

func TestPluginDeleteRejectsEnabledPlugin(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPlugin(t, baseReg.BaseDir, "vendor.enabled", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/plugins/vendor.enabled", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad request; body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "请先禁用") {
		t.Fatalf("响应缺少先禁用提示: %s", res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "vendor.enabled")); err != nil {
		t.Fatalf("未禁用插件不应被删除: %v", err)
	}
	rootConfig, err := os.ReadFile(filepath.Join(reg.BaseDir, "configs", "ops.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rootConfig), "vendor.enabled") {
		t.Fatalf("删除失败不应写入或清理 disabled 配置: %s", rootConfig)
	}
}

func TestPluginDisableDeleteRejectUnknownPlugin(t *testing.T) {
	reg := testRegistry(t)
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.missing/disable", nil),
		httptest.NewRequest(http.MethodDelete, "/api/plugins/vendor.missing", nil),
	} {
		res := httptest.NewRecorder()
		NewHandler(reg).ServeHTTP(res, req)
		if res.Code != http.StatusNotFound {
			t.Fatalf("%s %s status = %d, want not found; body = %s", req.Method, req.URL.Path, res.Code, res.Body.String())
		}
	}
}

func TestPluginDisableDeleteRejectUnsafePluginID(t *testing.T) {
	reg := testRegistry(t)
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/plugins/vendor%2Fevil/disable", nil),
		httptest.NewRequest(http.MethodDelete, "/api/plugins/vendor%2Fevil", nil),
	} {
		res := httptest.NewRecorder()
		NewHandler(reg).ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status = %d, want bad request; body = %s", req.Method, req.URL.Path, res.Code, res.Body.String())
		}
		if !strings.Contains(res.Body.String(), "不安全路径字符") {
			t.Fatalf("响应缺少插件 ID 安全提示: %s", res.Body.String())
		}
	}
}

func TestPluginDisableRollsBackConfigOnReloadFailure(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPlugin(t, baseReg.BaseDir, "vendor.rollback", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	installTestPlugin(t, baseReg.BaseDir, "vendor.reloadbad", "1.0.0")
	if err := os.WriteFile(filepath.Join(reg.BaseDir, "plugins", "vendor.reloadbad", "plugin.yaml"), []byte("id: ../bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.rollback/disable", nil))

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad request; body = %s", res.Code, res.Body.String())
	}
	rootConfig, err := os.ReadFile(filepath.Join(reg.BaseDir, "configs", "ops.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rootConfig), "vendor.rollback") {
		t.Fatalf("刷新失败时应回滚 disabled 配置: %s", rootConfig)
	}
}

func TestPluginDeleteKeepsDisabledConfigOnDeleteFailure(t *testing.T) {
	baseReg := testRegistry(t)
	installTestPlugin(t, baseReg.BaseDir, "vendor.deletefail", "1.0.0")
	reg, err := registry.Load(baseReg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	handler := NewHandler(reg)
	disableRes := httptest.NewRecorder()
	handler.ServeHTTP(disableRes, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.deletefail/disable", nil))
	if disableRes.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disableRes.Code, disableRes.Body.String())
	}
	if err := os.Remove(filepath.Join(reg.BaseDir, "plugins", "vendor.deletefail", "plugin.yaml")); err != nil {
		t.Fatal(err)
	}
	deleteRes := httptest.NewRecorder()

	handler.ServeHTTP(deleteRes, httptest.NewRequest(http.MethodDelete, "/api/plugins/vendor.deletefail", nil))

	if deleteRes.Code != http.StatusBadRequest {
		t.Fatalf("delete status = %d, want bad request; body = %s", deleteRes.Code, deleteRes.Body.String())
	}
	rootConfig, err := os.ReadFile(filepath.Join(reg.BaseDir, "configs", "ops.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rootConfig), "vendor.deletefail") {
		t.Fatalf("删除失败时应保留 disabled 配置: %s", rootConfig)
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "vendor.deletefail")); err != nil {
		t.Fatalf("删除失败时不应移除插件目录: %v", err)
	}
}

func TestPluginRoutesPreserveUploadUserWorkflowActionsDeleteAndExport(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	uploadRes := httptest.NewRecorder()

	handler.ServeHTTP(uploadRes, pluginUploadRequest(t, pluginZip(t, "vendor.route", "1.0.0", false), false))

	if uploadRes.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", uploadRes.Code, uploadRes.Body.String())
	}
	exportRes := httptest.NewRecorder()
	handler.ServeHTTP(exportRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.route.zip", nil))
	if exportRes.Code != http.StatusOK {
		t.Fatalf("plugin export status = %d, body = %s", exportRes.Code, exportRes.Body.String())
	}
	if contentType := exportRes.Header().Get("Content-Type"); contentType != "application/zip" {
		t.Fatalf("export Content-Type = %q, want application/zip", contentType)
	}
	// 配置路由已移除，业务配置已废弃
	disableRes := httptest.NewRecorder()
	handler.ServeHTTP(disableRes, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.route/disable", nil))
	if disableRes.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disableRes.Code, disableRes.Body.String())
	}
	enableRes := httptest.NewRecorder()
	handler.ServeHTTP(enableRes, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.route/enable", nil))
	if enableRes.Code != http.StatusOK {
		t.Fatalf("enable status = %d, body = %s", enableRes.Code, enableRes.Body.String())
	}
	disableAgainRes := httptest.NewRecorder()
	handler.ServeHTTP(disableAgainRes, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.route/disable", nil))
	if disableAgainRes.Code != http.StatusOK {
		t.Fatalf("disable-again status = %d, body = %s", disableAgainRes.Code, disableAgainRes.Body.String())
	}
	deleteRes := httptest.NewRecorder()
	handler.ServeHTTP(deleteRes, httptest.NewRequest(http.MethodDelete, "/api/plugins/vendor.route", nil))
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRes.Code, deleteRes.Body.String())
	}
	workflowRes := httptest.NewRecorder()
	handler.ServeHTTP(workflowRes, httptest.NewRequest(http.MethodGet, "/api/plugins/user-workflows.zip", nil))
	if workflowRes.Code != http.StatusOK {
		t.Fatalf("user workflow export status = %d, body = %s", workflowRes.Code, workflowRes.Body.String())
	}
	if contentType := workflowRes.Header().Get("Content-Type"); contentType != "application/zip" {
		t.Fatalf("Content-Type = %q, want application/zip", contentType)
	}
}

func TestPluginDisableRejectsNonPostMethod(t *testing.T) {
	reg := testRegistry(t)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.method/disable", nil))

	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want method not allowed; body = %s", res.Code, res.Body.String())
	}
}

func TestPluginDeleteRejectsZipPath(t *testing.T) {
	reg := testRegistry(t)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, httptest.NewRequest(http.MethodDelete, "/api/plugins/vendor.delete.zip", nil))

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad request; body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "请使用插件 ID") {
		t.Fatalf("响应缺少插件 ID 提示: %s", res.Body.String())
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
	if !strings.Contains(res.Body.String(), `"items"`) || !strings.Contains(res.Body.String(), `"kind":"tool_run"`) {
		t.Fatalf("响应缺少结构化日志项: %s", res.Body.String())
	}
}

func TestRunDetailAPIIncludesLoopAggregateAndIterationItems(t *testing.T) {
	reg := testRegistry(t)
	runDir := filepath.Join(reg.BaseDir, "runs", "logs", "run-loop")
	if err := os.MkdirAll(filepath.Join(runDir, "repeat", "1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(runDir, "repeat", "2"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := `{"id":"run-loop","kind":"workflow","target":"demo.loop","status":"succeeded","steps":[{"id":"repeat","type":"loop","tool":"demo.hello","status":"succeeded","loop_iterations":2}]}`
	if err := os.WriteFile(filepath.Join(runDir, "result.json"), []byte(result), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "repeat", "stdout.log"), []byte("聚合输出\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "repeat", "1", "stdout.log"), []byte("第1次\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "repeat", "2", "stderr.log"), []byte("第2次错误\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/runs/run-loop", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, want := range []string{`"kind":"workflow_step"`, `"children"`, `"kind":"loop_iteration"`, "聚合输出", "第1次", "第2次错误"} {
		if !strings.Contains(body, want) {
			t.Fatalf("响应缺少 %q: %s", want, body)
		}
	}
}

func TestRunEventsAPIStreamsExistingLogsAndCompletes(t *testing.T) {
	reg := testRegistry(t)
	runDir := filepath.Join(reg.BaseDir, "runs", "logs", "run-events")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "result.json"), []byte(`{"id":"run-events","kind":"tool","target":"demo.hello","status":"succeeded"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stdout.log"), []byte("line-1\nline-2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/runs/run-events/events", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if contentType := res.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", contentType)
	}
	body := res.Body.String()
	for _, want := range []string{"event: log", `"text":"line-1"`, `"text":"line-2"`, "event: complete", `"status":"succeeded"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("事件流缺少 %q: %s", want, body)
		}
	}
}

func TestRunEventsAPIStreamsAppendedLogsAndCompletes(t *testing.T) {
	reg := testRegistry(t)
	runDir := filepath.Join(reg.BaseDir, "runs", "logs", "run-follow")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(runDir, "result.json")
	if err := os.WriteFile(resultPath, []byte(`{"id":"run-follow","kind":"tool","target":"demo.hello","status":"running"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stdoutPath := filepath.Join(runDir, "stdout.log")
	if err := os.WriteFile(stdoutPath, []byte("old-line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHandler(reg))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/runs/run-follow/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	bodyCh := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(resp.Body)
		bodyCh <- string(data)
	}()
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(stdoutPath, []byte("old-line\nnew-line\nnew-line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, []byte(`{"id":"run-follow","kind":"tool","target":"demo.hello","status":"succeeded"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case body := <-bodyCh:
		if strings.Count(body, `"text":"old-line"`) != 1 {
			t.Fatalf("历史日志发送次数不正确: %s", body)
		}
		if strings.Count(body, `"text":"new-line"`) != 1 {
			t.Fatalf("增量重复日志未被抑制: %s", body)
		}
		if !strings.Contains(body, "event: complete") || !strings.Contains(body, `"status":"succeeded"`) {
			t.Fatalf("事件流缺少 complete: %s", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("事件流未在运行结束后关闭")
	}
}

func TestRunsListAPI(t *testing.T) {
	reg := testRegistry(t)
	writeRunFixture(t, reg.BaseDir, "run-1", `{"id":"run-1","kind":"tool","target":"demo.hello","status":"succeeded","started_at":"2026-06-14T10:00:00Z"}`)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"runs"`) || !strings.Contains(res.Body.String(), "run-1") {
		t.Fatalf("响应缺少运行列表: %s", res.Body.String())
	}
}

func TestRunsCleanupAPIKeepsNewestRuns(t *testing.T) {
	reg := testRegistry(t)
	writeRunFixture(t, reg.BaseDir, "run-old", `{"id":"run-old","kind":"tool","target":"demo.old","status":"succeeded","started_at":"2026-06-14T09:00:00Z"}`)
	writeRunFixture(t, reg.BaseDir, "run-new", `{"id":"run-new","kind":"workflow","target":"demo.new","status":"succeeded","started_at":"2026-06-14T11:00:00Z"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/runs/cleanup", strings.NewReader(`{"keep":1}`))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "run-old") || !strings.Contains(res.Body.String(), `"deleted":1`) {
		t.Fatalf("响应缺少清理结果: %s", res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "runs", "logs", "run-old")); !os.IsNotExist(err) {
		t.Fatalf("旧运行记录未清理: %v", err)
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "runs", "logs", "run-new")); err != nil {
		t.Fatalf("最新运行记录不应被清理: %v", err)
	}
}

func TestRunSupportZipAPI(t *testing.T) {
	reg := testRegistry(t)
	writeRunFixture(t, reg.BaseDir, "run-1", `{"id":"run-1","kind":"tool","target":"demo.hello","status":"failed","params":{"api.token":"secret"},"error":"boom"}`)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/run-1/support.zip", nil)
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("Content-Type = %q", got)
	}
	zr, err := zip.NewReader(bytes.NewReader(res.Body.Bytes()), int64(res.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{}
	for _, file := range zr.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = string(data)
	}
	for _, want := range []string{"support-summary.json", "support-report.md", "doctor-report.json", "run/result.json", "run/stdout.log", "run/stderr.log"} {
		if _, ok := entries[want]; !ok {
			t.Fatalf("支持包缺少 %s，实际: %#v", want, entries)
		}
	}
	if !strings.Contains(entries["support-report.md"], "运行支持报告") || !strings.Contains(entries["support-report.md"], "诊断摘要") {
		t.Fatalf("支持报告内容不完整: %s", entries["support-report.md"])
	}
	if strings.Contains(entries["support-summary.json"], "secret") || !strings.Contains(entries["support-summary.json"], "******") {
		t.Fatalf("支持包摘要未脱敏: %s", entries["support-summary.json"])
	}
	if strings.Contains(entries["run/result.json"], "secret") || !strings.Contains(entries["run/result.json"], "******") {
		t.Fatalf("支持包运行记录未脱敏: %s", entries["run/result.json"])
	}
}

func TestTokenMiddlewareRequiresLocalToken(t *testing.T) {
	reg := testRegistry(t)
	handler := tokenMiddleware(NewHandler(reg), "abc")

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/catalog", nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("未带 token status = %d", res.Code)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/catalog?token=abc", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("带 token status = %d, body = %s", res.Code, res.Body.String())
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

func TestWorkflowConfigFilesAPIReadsAndWritesLegacyUserWorkflowFile(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	saveReq := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.config/save", strings.NewReader(`{"workflow":{"id":"demo.config","name":"配置流程","category":"demo","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`))
	saveRes := httptest.NewRecorder()
	handler.ServeHTTP(saveRes, saveReq)
	if saveRes.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", saveRes.Code, saveRes.Body.String())
	}
	configPath := filepath.Join(reg.BaseDir, "plugins", "user.workflows", "config", "workflows", "demo.config", "app.conf")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("initial=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	putRes := httptest.NewRecorder()
	handler.ServeHTTP(putRes, httptest.NewRequest(http.MethodPut, "/api/workflows/demo.config/files/app.conf", strings.NewReader(`{"content":"enabled=true\n"}`)))
	if putRes.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", putRes.Code, putRes.Body.String())
	}

	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, httptest.NewRequest(http.MethodGet, "/api/workflows/demo.config/files", nil))
	if listRes.Code != http.StatusOK || !strings.Contains(listRes.Body.String(), `"id":"app.conf"`) || !strings.Contains(listRes.Body.String(), `"config_dir":"config/workflows/demo.config"`) || !strings.Contains(listRes.Body.String(), `"display_path":"config/workflows/demo.config/app.conf"`) || !strings.Contains(listRes.Body.String(), `"readable":true`) {
		t.Fatalf("list status = %d, body = %s", listRes.Code, listRes.Body.String())
	}

	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, httptest.NewRequest(http.MethodGet, "/api/workflows/demo.config/files/app.conf", nil))
	if getRes.Code != http.StatusOK || !strings.Contains(getRes.Body.String(), "enabled=true") {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}
}

func TestWorkflowConfigFilesUseRealMountedUploadPath(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	uploadPath := filepath.Join(reg.BaseDir, "runs", "uploads", "upload-1", "pkg.tar.gz")
	if err := os.MkdirAll(filepath.Dir(uploadPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(uploadPath, []byte("initial=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	saveReq := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.uploadconfig/save", strings.NewReader(`{"workflow":{"id":"demo.uploadconfig","name":"上传配置","category":"demo","config_files":[{"id":"pkg","label":"部署包","config_dir":".","path":"runs/uploads/upload-1/pkg.tar.gz","access":"read_write","create":true}],"nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`))
	saveRes := httptest.NewRecorder()
	handler.ServeHTTP(saveRes, saveReq)
	if saveRes.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", saveRes.Code, saveRes.Body.String())
	}

	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, httptest.NewRequest(http.MethodGet, "/api/workflows/demo.uploadconfig/files", nil))
	if listRes.Code != http.StatusOK || !strings.Contains(listRes.Body.String(), `"id":"pkg"`) || !strings.Contains(listRes.Body.String(), `"path":"runs/uploads/upload-1/pkg.tar.gz"`) || !strings.Contains(listRes.Body.String(), `"display_path":"runs/uploads/upload-1/pkg.tar.gz"`) || strings.Contains(listRes.Body.String(), "plugins/user.workflows") {
		t.Fatalf("list status = %d, body = %s", listRes.Code, listRes.Body.String())
	}

	putRes := httptest.NewRecorder()
	handler.ServeHTTP(putRes, httptest.NewRequest(http.MethodPut, "/api/workflows/demo.uploadconfig/files/pkg", strings.NewReader(`{"content":"updated=true\n"}`)))
	if putRes.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", putRes.Code, putRes.Body.String())
	}
	data, err := os.ReadFile(uploadPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "updated=true\n" {
		t.Fatalf("upload config content = %q", got)
	}
}

func TestWorkflowConfigFilesDeleteMissingDeclaredFile(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	saveReq := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.missingconfig/save", strings.NewReader(`{"workflow":{"id":"demo.missingconfig","name":"缺失配置","category":"demo","config_files":[{"id":"missing","label":"缺失文件","config_dir":".","path":"runs/uploads/missing/app.conf","access":"read_write","create":true}],"nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`))
	saveRes := httptest.NewRecorder()
	handler.ServeHTTP(saveRes, saveReq)
	if saveRes.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", saveRes.Code, saveRes.Body.String())
	}

	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, httptest.NewRequest(http.MethodGet, "/api/workflows/demo.missingconfig/files", nil))
	if listRes.Code != http.StatusOK || !strings.Contains(listRes.Body.String(), `"id":"missing"`) || !strings.Contains(listRes.Body.String(), `"exists":false`) {
		t.Fatalf("list status = %d, body = %s", listRes.Code, listRes.Body.String())
	}

	deleteRes := httptest.NewRecorder()
	handler.ServeHTTP(deleteRes, httptest.NewRequest(http.MethodDelete, "/api/workflows/demo.missingconfig/files/missing", nil))
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRes.Code, deleteRes.Body.String())
	}

	listAfter := httptest.NewRecorder()
	handler.ServeHTTP(listAfter, httptest.NewRequest(http.MethodGet, "/api/workflows/demo.missingconfig/files", nil))
	if listAfter.Code != http.StatusOK || strings.Contains(listAfter.Body.String(), `"id":"missing"`) {
		t.Fatalf("list after delete status = %d, body = %s", listAfter.Code, listAfter.Body.String())
	}
	workflowPath := filepath.Join(reg.BaseDir, "plugins", "user.workflows", "workflows", "demo.missingconfig.yaml")
	workflowData, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(workflowData), "runs/uploads/missing/app.conf") {
		t.Fatalf("workflow declaration still contains deleted file:\n%s", string(workflowData))
	}
}

func TestWorkflowConfigFilesRejectUnsafeRealPath(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	saveReq := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.unsafeconfig/save", strings.NewReader(`{"workflow":{"id":"demo.unsafeconfig","name":"不安全配置","category":"demo","config_files":[{"id":"secret","config_dir":"..","path":"secret.conf","access":"read_write","create":true}],"nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`))
	saveRes := httptest.NewRecorder()
	handler.ServeHTTP(saveRes, saveReq)
	if saveRes.Code == http.StatusOK {
		t.Fatalf("save status = %d, body = %s", saveRes.Code, saveRes.Body.String())
	}
}
func TestPluginConfigFilesEditDeclaredPluginFile(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins", "vendor.configfiles")
	if err := os.MkdirAll(filepath.Join(pluginDir, "configs"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(pluginDir, "configs", "app.conf")
	if err := os.WriteFile(configPath, []byte("old=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{},
		Tools: map[string]*registry.Tool{
			"vendor.configfiles.run": {
				Entry: config.ToolEntry{ID: "vendor.configfiles.run"},
				Config: &config.ToolConfig{
					ID:          "vendor.configfiles.run",
					ConfigFiles: []string{"configs/app.conf"},
					PluginConfig: config.PluginToolConfig{
						ID:  "vendor.configfiles",
						Dir: pluginDir,
					},
				},
				Dir:    pluginDir,
				Source: registry.Source{Type: "plugin", PluginID: "vendor.configfiles"},
			},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	handler := NewHandler(reg)

	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.configfiles/files", nil))
	if listRes.Code != http.StatusOK || !strings.Contains(listRes.Body.String(), `"id":"configs/app.conf"`) || !strings.Contains(listRes.Body.String(), `"scope":"plugin"`) {
		t.Fatalf("list status = %d, body = %s", listRes.Code, listRes.Body.String())
	}

	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.configfiles/files/configs%2Fapp.conf", nil))
	if getRes.Code != http.StatusOK || !strings.Contains(getRes.Body.String(), "old=true") {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}

	putRes := httptest.NewRecorder()
	handler.ServeHTTP(putRes, httptest.NewRequest(http.MethodPut, "/api/plugins/vendor.configfiles/files/configs%2Fapp.conf", strings.NewReader(`{"content":"new=true\n"}`)))
	if putRes.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", putRes.Code, putRes.Body.String())
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new=true\n" {
		t.Fatalf("plugin config file = %q", content)
	}

	badRes := httptest.NewRecorder()
	handler.ServeHTTP(badRes, httptest.NewRequest(http.MethodPut, "/api/plugins/vendor.configfiles/files/plugin.yaml", strings.NewReader(`{"content":"bad"}`)))
	if badRes.Code != http.StatusBadRequest {
		t.Fatalf("undeclared put status = %d, body = %s", badRes.Code, badRes.Body.String())
	}
}

func TestPluginConfigFilesEditPluginFileOutsidePluginDir(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins", "vendor.externalconfig")
	sharedDir := filepath.Join(dir, "shared-config")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(sharedDir, "app.conf")
	if err := os.WriteFile(configPath, []byte("old=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{},
		Tools: map[string]*registry.Tool{
			"vendor.externalconfig.run": {
				Entry: config.ToolEntry{ID: "vendor.externalconfig.run"},
				Config: &config.ToolConfig{
					ID: "vendor.externalconfig.run",
					ConfigFileRefs: []config.ConfigFileRef{
						{ID: "app.conf", ConfigDir: "../../shared-config", Path: "app.conf", Scope: config.ConfigFileScopePlugin, Access: config.ConfigFileAccessReadWrite, Create: true},
					},
					PluginConfig: config.PluginToolConfig{
						ID:  "vendor.externalconfig",
						Dir: pluginDir,
					},
				},
				Dir:    pluginDir,
				Source: registry.Source{Type: "plugin", PluginID: "vendor.externalconfig"},
			},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	handler := NewHandler(reg)

	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.externalconfig/files/app.conf", nil))
	if getRes.Code != http.StatusOK || !strings.Contains(getRes.Body.String(), "old=true") {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}

	putRes := httptest.NewRecorder()
	handler.ServeHTTP(putRes, httptest.NewRequest(http.MethodPut, "/api/plugins/vendor.externalconfig/files/app.conf", strings.NewReader(`{"content":"new=true\n"}`)))
	if putRes.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", putRes.Code, putRes.Body.String())
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new=true\n" {
		t.Fatalf("external plugin config file = %q", content)
	}
}

func TestPluginConfigFilesEditPluginFileWithAbsoluteConfigDir(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins", "vendor.absoluteconfig")
	absoluteConfigDir := filepath.Join(dir, "absolute-config")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(absoluteConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(absoluteConfigDir, "app.conf")
	if err := os.WriteFile(configPath, []byte("old=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{},
		Tools: map[string]*registry.Tool{
			"vendor.absoluteconfig.run": {
				Entry: config.ToolEntry{ID: "vendor.absoluteconfig.run"},
				Config: &config.ToolConfig{
					ID: "vendor.absoluteconfig.run",
					ConfigFileRefs: []config.ConfigFileRef{
						{ID: "app.conf", ConfigDir: absoluteConfigDir, Path: "app.conf", Scope: config.ConfigFileScopePlugin, Access: config.ConfigFileAccessReadWrite, Create: true},
					},
					PluginConfig: config.PluginToolConfig{
						ID:  "vendor.absoluteconfig",
						Dir: pluginDir,
					},
				},
				Dir:    pluginDir,
				Source: registry.Source{Type: "plugin", PluginID: "vendor.absoluteconfig"},
			},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	handler := NewHandler(reg)

	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.absoluteconfig/files", nil))
	if listRes.Code != http.StatusOK || !strings.Contains(listRes.Body.String(), `"config_dir":"`+strings.ReplaceAll(absoluteConfigDir, `\`, `\\`)+`"`) {
		t.Fatalf("list status = %d, body = %s", listRes.Code, listRes.Body.String())
	}

	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.absoluteconfig/files/app.conf", nil))
	if getRes.Code != http.StatusOK || !strings.Contains(getRes.Body.String(), "old=true") {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}

	putRes := httptest.NewRecorder()
	handler.ServeHTTP(putRes, httptest.NewRequest(http.MethodPut, "/api/plugins/vendor.absoluteconfig/files/app.conf", strings.NewReader(`{"content":"new=true\n"}`)))
	if putRes.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", putRes.Code, putRes.Body.String())
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new=true\n" {
		t.Fatalf("absolute plugin config file = %q", content)
	}

	unsafeReg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{},
		Tools: map[string]*registry.Tool{
			"vendor.absoluteconfig.bad": {
				Entry: config.ToolEntry{ID: "vendor.absoluteconfig.bad"},
				Config: &config.ToolConfig{
					ID: "vendor.absoluteconfig.bad",
					ConfigFileRefs: []config.ConfigFileRef{
						{ID: "escape", ConfigDir: absoluteConfigDir, Path: "../secret.conf", Scope: config.ConfigFileScopePlugin, Access: config.ConfigFileAccessReadWrite, Create: true},
					},
					PluginConfig: config.PluginToolConfig{ID: "vendor.absoluteconfig", Dir: pluginDir},
				},
				Dir:    pluginDir,
				Source: registry.Source{Type: "plugin", PluginID: "vendor.absoluteconfig"},
			},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	unsafeHandler := NewHandler(unsafeReg)
	unsafeGet := httptest.NewRecorder()
	unsafeHandler.ServeHTTP(unsafeGet, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.absoluteconfig/files/escape", nil))
	if unsafeGet.Code != http.StatusBadRequest || !strings.Contains(unsafeGet.Body.String(), "不能逃逸") {
		t.Fatalf("unsafe get status = %d, body = %s", unsafeGet.Code, unsafeGet.Body.String())
	}
}

func TestPluginConfigFilesHostAbsoluteAccessControls(t *testing.T) {
	dir := t.TempDir()
	hostDir := filepath.Join(dir, "host")
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		t.Fatal(err)
	}
	readOnlyPath := filepath.Join(hostDir, "readonly.conf")
	writePath := filepath.Join(hostDir, "write.conf")
	createPath := filepath.Join(hostDir, "created.conf")
	if err := os.WriteFile(readOnlyPath, []byte("readonly=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(writePath, []byte("old=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := hostConfigFileTestRegistry(t, dir, []config.ConfigFileRef{
		{ID: "readonly", Label: "只读", Scope: config.ConfigFileScopeHostAbsolute, ConfigDir: hostDir, Path: "readonly.conf", Access: config.ConfigFileAccessRead},
		{ID: "write", Label: "可写", Scope: config.ConfigFileScopeHostAbsolute, ConfigDir: hostDir, Path: "write.conf", Access: config.ConfigFileAccessReadWrite},
		{ID: "missing", Label: "缺失", Scope: config.ConfigFileScopeHostAbsolute, ConfigDir: hostDir, Path: "missing.conf", Access: config.ConfigFileAccessReadWrite, Create: false},
		{ID: "create", Label: "创建", Scope: config.ConfigFileScopeHostAbsolute, ConfigDir: hostDir, Path: "created.conf", Access: config.ConfigFileAccessReadWrite, Create: true},
	})
	handler := NewHandler(reg)

	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.configfiles/files", nil))
	body := listRes.Body.String()
	if listRes.Code != http.StatusOK || !strings.Contains(body, `"scope":"host_absolute"`) || !strings.Contains(body, `"id":"missing"`) || !strings.Contains(body, `"writable":false`) {
		t.Fatalf("list status = %d, body = %s", listRes.Code, body)
	}

	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.configfiles/files/readonly", nil))
	if getRes.Code != http.StatusOK || !strings.Contains(getRes.Body.String(), "readonly=true") {
		t.Fatalf("readonly get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}

	readOnlyPut := httptest.NewRecorder()
	handler.ServeHTTP(readOnlyPut, httptest.NewRequest(http.MethodPut, "/api/plugins/vendor.configfiles/files/readonly", strings.NewReader(`{"content":"bad"}`)))
	if readOnlyPut.Code != http.StatusBadRequest || !strings.Contains(readOnlyPut.Body.String(), "只读") {
		t.Fatalf("readonly put status = %d, body = %s", readOnlyPut.Code, readOnlyPut.Body.String())
	}

	writePut := httptest.NewRecorder()
	handler.ServeHTTP(writePut, httptest.NewRequest(http.MethodPut, "/api/plugins/vendor.configfiles/files/write", strings.NewReader(`{"content":"new=true\n"}`)))
	if writePut.Code != http.StatusOK {
		t.Fatalf("write put status = %d, body = %s", writePut.Code, writePut.Body.String())
	}
	if content, _ := os.ReadFile(writePath); string(content) != "new=true\n" {
		t.Fatalf("write content = %q", content)
	}

	missingPut := httptest.NewRecorder()
	handler.ServeHTTP(missingPut, httptest.NewRequest(http.MethodPut, "/api/plugins/vendor.configfiles/files/missing", strings.NewReader(`{"content":"bad"}`)))
	if missingPut.Code != http.StatusBadRequest || !strings.Contains(missingPut.Body.String(), "不允许创建") {
		t.Fatalf("missing put status = %d, body = %s", missingPut.Code, missingPut.Body.String())
	}

	createPut := httptest.NewRecorder()
	handler.ServeHTTP(createPut, httptest.NewRequest(http.MethodPut, "/api/plugins/vendor.configfiles/files/create", strings.NewReader(`{"content":"created=true\n"}`)))
	if createPut.Code != http.StatusOK {
		t.Fatalf("create put status = %d, body = %s", createPut.Code, createPut.Body.String())
	}
	if content, _ := os.ReadFile(createPath); string(content) != "created=true\n" {
		t.Fatalf("created content = %q", content)
	}
}

func TestPluginConfigFilesRejectsUnsafeHostOperations(t *testing.T) {
	dir := t.TempDir()
	hostDir := filepath.Join(dir, "host")
	if err := os.MkdirAll(filepath.Join(hostDir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	hostPath := filepath.Join(hostDir, "app.conf")
	if err := os.WriteFile(hostPath, []byte("ok=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := hostConfigFileTestRegistry(t, dir, []config.ConfigFileRef{
		{ID: "host", Scope: config.ConfigFileScopeHostAbsolute, ConfigDir: hostDir, Path: "app.conf", Access: config.ConfigFileAccessReadWrite},
		{ID: "dir", Scope: config.ConfigFileScopeHostAbsolute, ConfigDir: hostDir, Path: "subdir", Access: config.ConfigFileAccessReadWrite},
	})
	handler := NewHandler(reg)

	encodedTraversal := httptest.NewRecorder()
	handler.ServeHTTP(encodedTraversal, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.configfiles/files/%252e%252e%252Fpasswd", nil))
	if encodedTraversal.Code != http.StatusBadRequest {
		t.Fatalf("encoded traversal status = %d, body = %s", encodedTraversal.Code, encodedTraversal.Body.String())
	}

	deleteRes := httptest.NewRecorder()
	handler.ServeHTTP(deleteRes, httptest.NewRequest(http.MethodDelete, "/api/plugins/vendor.configfiles/files/host", nil))
	if deleteRes.Code != http.StatusBadRequest || !strings.Contains(deleteRes.Body.String(), "不支持删除") {
		t.Fatalf("delete status = %d, body = %s", deleteRes.Code, deleteRes.Body.String())
	}

	dirGet := httptest.NewRecorder()
	handler.ServeHTTP(dirGet, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.configfiles/files/dir", nil))
	if dirGet.Code != http.StatusBadRequest || !strings.Contains(dirGet.Body.String(), "普通文件") {
		t.Fatalf("dir get status = %d, body = %s", dirGet.Code, dirGet.Body.String())
	}
}

func TestPluginConfigFilesRejectsHostFileSymlinkEscapeAtRuntime(t *testing.T) {
	dir := t.TempDir()
	hostDir := filepath.Join(dir, "host")
	outsideDir := filepath.Join(dir, "outside")
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(outsideDir, "secret.conf")
	if err := os.WriteFile(outsidePath, []byte("secret=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(hostDir, "linked.conf")
	if err := os.Symlink(outsidePath, symlinkPath); err != nil {
		t.Skipf("当前环境不支持创建文件符号链接: %v", err)
	}
	reg := hostConfigFileTestRegistry(t, dir, []config.ConfigFileRef{
		{ID: "host", Scope: config.ConfigFileScopeHostAbsolute, ConfigDir: hostDir, Path: "linked.conf", Access: config.ConfigFileAccessReadWrite},
	})
	handler := NewHandler(reg)

	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.configfiles/files/host", nil))
	if getRes.Code != http.StatusBadRequest || !strings.Contains(getRes.Body.String(), "白名单") {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}
}

func TestPluginConfigFilesHostAbsoluteRequiresRuntimeWhitelist(t *testing.T) {
	dir := t.TempDir()
	hostDir := filepath.Join(dir, "host")
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hostPath := filepath.Join(hostDir, "app.conf")
	if err := os.WriteFile(hostPath, []byte("ok=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := hostConfigFileTestRegistry(t, dir, []config.ConfigFileRef{
		{ID: "host", Scope: config.ConfigFileScopeHostAbsolute, ConfigDir: hostDir, Path: "app.conf", Access: config.ConfigFileAccessReadWrite},
	})
	reg.Root.HostConfigFiles.AllowedDirs = nil
	handler := NewHandler(reg)

	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.configfiles/files", nil))
	if listRes.Code != http.StatusOK || !strings.Contains(listRes.Body.String(), "host_config_files.allowed_dirs") || !strings.Contains(listRes.Body.String(), `"readable":false`) {
		t.Fatalf("list status = %d, body = %s", listRes.Code, listRes.Body.String())
	}

	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.configfiles/files/host", nil))
	if getRes.Code != http.StatusBadRequest || !strings.Contains(getRes.Body.String(), "host_config_files.allowed_dirs") {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}
}

func TestPluginConfigFilesExpandsDirectoryEntries(t *testing.T) {
	dir := t.TempDir()
	hostDir := filepath.Join(dir, "host")
	confDir := filepath.Join(hostDir, "conf.d")
	if err := os.MkdirAll(filepath.Join(confDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(confDir, "first.conf")
	secondPath := filepath.Join(confDir, "second.conf")
	if err := os.WriteFile(firstPath, []byte("first=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("second=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "nested", "ignored.conf"), []byte("ignored=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := hostConfigFileTestRegistry(t, dir, []config.ConfigFileRef{
		{ID: "conf-dir", Scope: config.ConfigFileScopeHostAbsolute, ConfigDir: hostDir, Path: "conf.d", Access: config.ConfigFileAccessReadWrite},
	})
	handler := NewHandler(reg)

	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.configfiles/files", nil))
	body := listRes.Body.String()
	if listRes.Code != http.StatusOK || !strings.Contains(body, `"id":"conf-dir:first.conf"`) || !strings.Contains(body, `"id":"conf-dir:second.conf"`) || strings.Contains(body, "ignored.conf") {
		t.Fatalf("list status = %d, body = %s", listRes.Code, body)
	}

	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, httptest.NewRequest(http.MethodGet, "/api/plugins/vendor.configfiles/files/conf-dir:first.conf", nil))
	if getRes.Code != http.StatusOK || !strings.Contains(getRes.Body.String(), "first=true") {
		t.Fatalf("get status = %d, body = %s", getRes.Code, getRes.Body.String())
	}
}

func hostConfigFileTestRegistry(t *testing.T, dir string, entries []config.ConfigFileRef) *registry.Registry {
	t.Helper()
	pluginDir := filepath.Join(dir, "plugins", "vendor.configfiles")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &registry.Registry{
		BaseDir: dir,
		Root: &config.RootConfig{
			HostConfigFiles: config.HostConfigFilesConfig{AllowedDirs: []string{filepath.Join(dir, "host")}},
		},
		Tools: map[string]*registry.Tool{
			"vendor.configfiles.run": {
				Entry: config.ToolEntry{ID: "vendor.configfiles.run"},
				Config: &config.ToolConfig{
					ID:             "vendor.configfiles.run",
					ConfigFiles:    []string{},
					ConfigFileRefs: entries,
					PluginConfig: config.PluginToolConfig{
						ID:  "vendor.configfiles",
						Dir: pluginDir,
					},
				},
				Dir:    pluginDir,
				Source: registry.Source{Type: "plugin", PluginID: "vendor.configfiles"},
			},
		},
		Workflows: map[string]*registry.Workflow{},
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

func writeTestRootConfigWithCategories(t *testing.T, dir string, categories []config.Category) {
	t.Helper()
	configDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var builder strings.Builder
	builder.WriteString("app:\n  name: Test Ops\npaths:\n  tools: []\n  workflows: []\n  runs: runs\n  logs: runs/logs\nplugins:\n  paths:\n    - plugins\n  strict: true\n  disabled: []\nmenu:\n  categories:\n")
	for _, category := range categories {
		builder.WriteString("    - id: ")
		builder.WriteString(category.ID)
		builder.WriteString("\n      name: ")
		builder.WriteString(category.Name)
		builder.WriteString("\n")
		if category.Description != "" {
			builder.WriteString("      description: ")
			builder.WriteString(category.Description)
			builder.WriteString("\n")
		}
	}
	if err := os.WriteFile(filepath.Join(configDir, "ops.yaml"), []byte(builder.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRunFixture(t *testing.T, baseDir, id, result string) {
	t.Helper()
	runDir := filepath.Join(baseDir, "runs", "logs", id)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "result.json"), []byte(result), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stdout.log"), []byte("标准输出\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stderr.log"), []byte("错误输出\n"), 0o644); err != nil {
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

func fileUploadRequest(t *testing.T, name string, data []byte) *http.Request {
	return fileUploadRequestWithTargetDir(t, name, data, "")
}

func fileUploadRequestWithTargetDir(t *testing.T, name string, data []byte, targetDir string) *http.Request {
	t.Helper()
	return multiFileUploadRequest(t, targetDir, []uploadRequestFile{{Name: name, Data: data}})
}

type uploadRequestFile struct {
	Name         string
	RelativePath string
	Data         []byte
}

func multiFileUploadRequest(t *testing.T, targetDir string, files []uploadRequestFile) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if targetDir != "" {
		if err := writer.WriteField("target_dir", targetDir); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range files {
		if err := writer.WriteField("relative_path", file.RelativePath); err != nil {
			t.Fatal(err)
		}
		part, err := writer.CreateFormFile("file", file.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(file.Data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/files/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func waitForServerRunStepStatus(t *testing.T, handler http.Handler, runID, stepID, status string) runDetail {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		detail, ok := fetchServerRunDetail(t, handler, runID)
		if ok {
			for _, step := range detail.Record.Steps {
				if step.ID == stepID && step.Status == status {
					return detail
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %s step %s did not reach %s", runID, stepID, status)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func waitForServerRunStatus(t *testing.T, handler http.Handler, runID, status string) runDetail {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		detail, ok := fetchServerRunDetail(t, handler, runID)
		if ok && detail.Record.Status == status {
			return detail
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %s did not reach %s", runID, status)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func fetchServerRunDetail(t *testing.T, handler http.Handler, runID string) (runDetail, bool) {
	t.Helper()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/runs/"+runID, nil))
	if res.Code != http.StatusOK {
		return runDetail{}, false
	}
	var body struct {
		Data runDetail `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Data, true
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

func pluginZipWithToolIDs(t *testing.T, id, version string, toolSuffixes []string) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	var manifest strings.Builder
	manifest.WriteString(`id: ` + id + `
name: Upload Order Test
version: ` + version + `
contributes:
  categories:
    - id: upload
      name: 上传插件
  tools:
`)
	for _, suffix := range toolSuffixes {
		manifest.WriteString(`    - id: ` + id + `.` + suffix + `
      name: ` + suffix + `
      category: upload
      command: scripts/run.sh
      workdir: .
`)
	}
	writeZipFile(t, writer, id+"/plugin.yaml", manifest.String())
	writeZipFile(t, writer, id+"/scripts/run.sh", "#!/usr/bin/env bash\necho uploaded\n")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func pluginZipWithMissingConfig(t *testing.T, id, version string) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	manifest, script := pluginFiles(id, version, false)
	manifest = strings.Replace(manifest, "      workdir: .\n", "      workdir: .\n      config_files:\n        - config/missing.conf\n", 1)
	writeZipFile(t, writer, id+"/plugin.yaml", manifest)
	writeZipFile(t, writer, id+"/scripts/run.sh", script)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func pluginZipWithCommand(t *testing.T, id, version, command string) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	manifest, script := pluginFiles(id, version, false)
	manifest = strings.Replace(manifest, "      command: scripts/run.sh\n", "      command: "+command+"\n", 1)
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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

func writeRuntimeBinary(t *testing.T, baseDir, goos, goarch string) {
	t.Helper()
	name := runtimeBinaryName(goos, goarch)
	path := filepath.Join(baseDir, "bin", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("opsctl binary"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func zipEntries(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("无法读取 zip: %v", err)
	}
	entries := map[string]bool{}
	for _, file := range reader.File {
		entries[file.Name] = true
	}
	return entries
}

func zipEntryData(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("无法读取 zip: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		content, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return content
	}
	t.Fatalf("zip entry %s not found", name)
	return nil
}

func tarGzEntries(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("无法读取 gzip: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entries := map[string]bool{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries[header.Name] = true
	}
	return entries
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

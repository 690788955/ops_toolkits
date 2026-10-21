package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell_ops/internal/registry"
)

func TestWorkflowRunTargetOverridesNodeHosts(t *testing.T) {
	reg := testRegistry(t)
	configureHelperTool(t, reg, "demo.hello", "target-run")
	body := `{"params":{},"target":"local","override_nodes":true,"workflow":{"id":"demo.target","name":"统一目标","nodes":[{"id":"first","tool":"demo.hello","host":"missing-operation-host"}],"edges":[]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.target/run", strings.NewReader(body))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"status":"succeeded"`) {
		t.Fatalf("响应缺少运行成功信息: %s", res.Body.String())
	}
	if strings.Contains(res.Body.String(), "missing-operation-host") {
		t.Fatalf("覆盖模式下不应再使用节点原有执行目标: %s", res.Body.String())
	}
}

func TestWorkflowRunTargetKeepsNodeHostWithoutOverride(t *testing.T) {
	reg := testRegistry(t)
	configureHelperTool(t, reg, "demo.hello", "target-keep")
	body := `{"params":{},"target":"local","workflow":{"id":"demo.keep","name":"保留节点目标","nodes":[{"id":"first","tool":"demo.hello","host":"missing-operation-host"}],"edges":[]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.keep/run", strings.NewReader(body))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "执行目标无效") {
		t.Fatalf("错误信息应指出节点执行目标无效: %s", res.Body.String())
	}
}

func TestWorkflowRunTargetRejectsUnknownMachine(t *testing.T) {
	reg := testRegistry(t)
	body := `{"params":{},"target":"no-such-host","workflow":{"id":"demo.badtarget","nodes":[{"id":"first","tool":"demo.hello"}],"edges":[]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.badtarget/run", strings.NewReader(body))
	res := httptest.NewRecorder()

	NewHandler(reg).ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "工作流运行目标无效") {
		t.Fatalf("错误信息应指出运行目标无效: %s", res.Body.String())
	}
}

func TestWorkflowRunTargetFollowNodesKeepsWorkflowConfig(t *testing.T) {
	reg := testRegistry(t)
	wf, err := reg.Workflow("demo.flow")
	if err != nil {
		t.Fatal(err)
	}
	before := wf.Config.Nodes[0].Host
	if _, err := applyWorkflowRunTarget(nil, wf.Config, workflowRunTargetFollowNodes, true); err != nil {
		t.Fatalf("跟随节点配置不应报错: %v", err)
	}
	if wf.Config.Nodes[0].Host != before {
		t.Fatalf("跟随节点配置时不应修改注册表中的工作流配置")
	}
}

func TestCatalogWorkflowExposesReferencedPluginConfigFiles(t *testing.T) {
	reg := testRegistry(t)
	writeConfigFilePlugin(t, reg.BaseDir)
	workflow := `id: cfgref.flow
name: 引用插件配置
category: cfgref
nodes:
  - id: first
    tool: vendor.configref.tool
edges: []
`
	workflowPath := filepath.Join(reg.BaseDir, "plugins", "user.workflows", "workflows", "cfgref.flow.yaml")
	if err := os.MkdirAll(filepath.Dir(workflowPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workflowPath, []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := maintainUserWorkflowPluginManifest(reg); err != nil {
		t.Fatalf("生成用户工作流插件清单失败: %v", err)
	}
	reloaded, err := registry.Load(reg.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}

	res := httptest.NewRecorder()
	NewHandler(reloaded).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/catalog", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data struct {
			Workflows []workflowCatalogEntry `json:"workflows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("catalog JSON 解析失败: %v", err)
	}
	for _, entry := range body.Data.Workflows {
		if entry.ID != "cfgref.flow" {
			continue
		}
		if len(entry.ReferencedPlugins) != 1 {
			t.Fatalf("应下发工作流引用的插件: %#v", entry.ReferencedPlugins)
		}
		plugin := entry.ReferencedPlugins[0]
		if plugin.ID != "vendor.configref" || plugin.Name == "" {
			t.Fatalf("引用插件信息不完整: %#v", plugin)
		}
		if len(plugin.Tools) != 1 || plugin.Tools[0] != "vendor.configref.tool" {
			t.Fatalf("引用工具应被记录: %#v", plugin.Tools)
		}
		if len(plugin.ConfigFiles) != 1 || plugin.ConfigFiles[0].Path != "app.yaml" || plugin.ConfigFiles[0].ConfigDir != "config" {
			t.Fatalf("插件配置文件应随引用插件下发: %#v", plugin.ConfigFiles)
		}
		return
	}
	t.Fatalf("catalog 缺少用户工作流条目: %#v", body.Data.Workflows)
}

func writeConfigFilePlugin(t *testing.T, baseDir string) {
	t.Helper()
	pluginDir := filepath.Join(baseDir, "plugins", "vendor.configref")
	if err := os.MkdirAll(filepath.Join(pluginDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `id: vendor.configref
name: 配置引用插件
version: 1.0.0
contributes:
  categories:
    - id: cfgref
      name: 配置引用
  tools:
    - id: vendor.configref.tool
      name: 配置工具
      category: cfgref
      command: scripts/run.sh
      workdir: .
      config_dir: config
      config_files:
        - app.yaml
      confirm:
        required: false
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "scripts", "run.sh"), []byte("#!/usr/bin/env bash\necho cfgref\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "config", "app.yaml"), []byte("key: value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

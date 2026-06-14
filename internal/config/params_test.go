package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeParamsPrecedence(t *testing.T) {
	defs := []Parameter{{Name: "name", Default: "default"}, {Name: "env", Default: "dev"}}
	fileParams := map[string]string{"name": "file"}
	overrides := map[string]string{"name": "cli"}

	got := MergeParams(defs, fileParams, overrides)
	if got["name"] != "cli" {
		t.Fatalf("name = %q, want cli", got["name"])
	}
	if got["env"] != "dev" {
		t.Fatalf("env = %q, want dev", got["env"])
	}
}

func TestParseSetValues(t *testing.T) {
	got, err := ParseSetValues([]string{"a=b", "c=d=e"})
	if err != nil {
		t.Fatalf("ParseSetValues 返回错误: %v", err)
	}
	if got["a"] != "b" || got["c"] != "d=e" {
		t.Fatalf("解析结果不符合预期: %#v", got)
	}
}

func TestNestedValuesDeepMergeSetPathAndRedaction(t *testing.T) {
	base := Values{
		"es": Values{
			"host":  "dev",
			"ports": []interface{}{9200},
			"auth":  Values{"user": "elastic", "password": "secret:env:ES_PASSWORD"},
		},
	}
	override := Values{
		"es": Values{
			"host":  "prod",
			"ports": []interface{}{9243},
		},
	}
	merged := MergeValues(base, override)
	if merged["es"].(Values)["host"] != "prod" {
		t.Fatalf("host 未按高优先级覆盖: %#v", merged)
	}
	ports := merged["es"].(Values)["ports"].([]interface{})
	if len(ports) != 1 || ports[0] != 9243 {
		t.Fatalf("数组应整体覆盖: %#v", merged)
	}
	if merged["es"].(Values)["auth"].(Values)["user"] != "elastic" {
		t.Fatalf("map 应递归保留低优先级字段: %#v", merged)
	}
	sets, err := ParseSetValuesNested([]string{"es.host=cli", "flat=value"})
	if err != nil {
		t.Fatalf("ParseSetValuesNested 返回错误: %v", err)
	}
	merged = MergeValues(merged, sets)
	if merged["es"].(Values)["host"] != "cli" || merged["flat"] != "value" {
		t.Fatalf("点路径覆盖失败: %#v", merged)
	}
	redacted := RedactSensitive(merged, []string{"es.auth.password"})
	if redacted["es"].(Values)["auth"].(Values)["password"] != "******" {
		t.Fatalf("敏感字段未脱敏: %#v", redacted)
	}
}

func TestPromptMissing(t *testing.T) {
	params := map[string]string{}
	var out bytes.Buffer
	err := PromptMissing([]Parameter{{Name: "name", Type: "string", Description: "用户名称", Required: true}}, params, bytes.NewBufferString("ops\n"), &out)
	if err != nil {
		t.Fatalf("PromptMissing 返回错误: %v", err)
	}
	if params["name"] != "ops" {
		t.Fatalf("name = %q, want ops", params["name"])
	}
	for _, want := range []string{"name", "用户名称", "类型=string", "必填"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("PromptMissing 提示缺少 %q: %s", want, out.String())
		}
	}
}

func TestPromptMissingSkipsExistingValues(t *testing.T) {
	params := map[string]string{"name": "cli"}
	var out bytes.Buffer
	err := PromptMissing([]Parameter{{Name: "name", Type: "string", Description: "用户名称", Required: true}}, params, bytes.NewBufferString("ignored\n"), &out)
	if err != nil {
		t.Fatalf("PromptMissing 返回错误: %v", err)
	}
	if params["name"] != "cli" {
		t.Fatalf("name = %q, want cli", params["name"])
	}
	if out.Len() != 0 {
		t.Fatalf("PromptMissing 不应提示已有值参数: %q", out.String())
	}
}

func TestPromptAllKeepsDefaultOnEnter(t *testing.T) {
	defs := []Parameter{{Name: "name", Type: "string", Description: "用户名称", Required: true, Default: "World"}}
	params := MergeParams(defs, nil, nil)
	var out bytes.Buffer

	if err := PromptAll(defs, params, bytes.NewBufferString("\n"), &out); err != nil {
		t.Fatalf("PromptAll 返回错误: %v", err)
	}
	if params["name"] != "World" {
		t.Fatalf("name = %q, want World", params["name"])
	}
	for _, want := range []string{"name", "用户名称", "类型=string", "必填", "默认值=World", "当前值=World"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("PromptAll 提示缺少 %q: %s", want, out.String())
		}
	}
}

func TestPromptAllKeepsCurrentSetValueOnEnter(t *testing.T) {
	defs := []Parameter{{Name: "name", Type: "string", Required: true, Default: "World"}}
	params := MergeParams(defs, nil, map[string]string{"name": "CLI"})
	var out bytes.Buffer

	if err := PromptAll(defs, params, bytes.NewBufferString("\n"), &out); err != nil {
		t.Fatalf("PromptAll 返回错误: %v", err)
	}
	if params["name"] != "CLI" {
		t.Fatalf("name = %q, want CLI", params["name"])
	}
	if !strings.Contains(out.String(), "当前值=CLI") || !strings.Contains(out.String(), "请输入 [当前: CLI]") {
		t.Fatalf("PromptAll 未展示当前值: %s", out.String())
	}
}

func TestPromptAllOverridesCurrentValue(t *testing.T) {
	defs := []Parameter{{Name: "name", Type: "string", Required: true, Default: "World"}}
	params := MergeParams(defs, nil, map[string]string{"name": "CLI"})

	if err := PromptAll(defs, params, bytes.NewBufferString("Alice\n"), &bytes.Buffer{}); err != nil {
		t.Fatalf("PromptAll 返回错误: %v", err)
	}
	if params["name"] != "Alice" {
		t.Fatalf("name = %q, want Alice", params["name"])
	}
}

func TestPromptAllPromptsAllParametersInOrder(t *testing.T) {
	defs := []Parameter{
		{Name: "first", Type: "string", Required: true, Default: "one"},
		{Name: "second", Type: "string", Required: false, Default: "two"},
	}
	params := MergeParams(defs, nil, nil)
	var out bytes.Buffer

	if err := PromptAll(defs, params, bytes.NewBufferString("\nchanged\n"), &out); err != nil {
		t.Fatalf("PromptAll 返回错误: %v", err)
	}
	if params["first"] != "one" || params["second"] != "changed" {
		t.Fatalf("params = %#v, want first default and second override", params)
	}
	text := out.String()
	firstIdx := strings.Index(text, "first")
	secondIdx := strings.Index(text, "second")
	if firstIdx < 0 || secondIdx < 0 || firstIdx >= secondIdx {
		t.Fatalf("PromptAll 未按定义顺序提示: %s", text)
	}
}

func TestPromptAllRequiredMissing(t *testing.T) {
	err := PromptAll([]Parameter{{Name: "name", Type: "string", Required: true}}, map[string]string{}, bytes.NewBufferString("\n"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "缺少必填参数 name") {
		t.Fatalf("PromptAll error = %v, want missing required name", err)
	}
}

func TestValidateRequired(t *testing.T) {
	err := ValidateRequired([]Parameter{{Name: "name", Required: true}}, map[string]string{"name": ""})
	if err == nil {
		t.Fatal("ValidateRequired 返回 nil，期望错误")
	}
}

func TestPromptConfirmation(t *testing.T) {
	err := PromptConfirmation(Confirmation{Required: true, Message: "确认？"}, bytes.NewBufferString("确认\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("PromptConfirmation 返回错误: %v", err)
	}
}

func TestPromptConfirmationRejectsMissingApproval(t *testing.T) {
	err := PromptConfirmation(Confirmation{Required: true, Message: "确认？"}, bytes.NewBufferString("no\n"), &bytes.Buffer{})
	if err == nil {
		t.Fatal("PromptConfirmation 返回 nil，期望错误")
	}
}

func TestLoadRootNewSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ops.yaml")
	content := `app:
  name: opsctl
  description: 测试应用
paths:
  tools: [tools]
  workflows: [workflows]
menu:
  categories:
    - id: demo
      name: Demo
server:
  host: 127.0.0.1
  port: 9090
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadRoot(path)
	if err != nil {
		t.Fatalf("LoadRoot 返回错误: %v", err)
	}
	if cfg.DisplayName() != "opsctl" || cfg.ListenAddr() != "127.0.0.1:9090" {
		t.Fatalf("根配置不符合预期: %#v", cfg)
	}
	if len(cfg.DisplayCategories()) != 1 || cfg.DisplayCategories()[0].ID != "demo" {
		t.Fatalf("分类不符合预期: %#v", cfg.DisplayCategories())
	}
	if len(cfg.Plugins.Paths) != 1 || cfg.Plugins.Paths[0] != "plugins" {
		t.Fatalf("插件路径默认值不符合预期: %#v", cfg.Plugins)
	}
}

func TestLoadRootDefaultsServerHostToLocalhost(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ops.yaml")
	if err := os.WriteFile(path, []byte("plugins:\n  paths: [plugins]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadRoot(path)
	if err != nil {
		t.Fatalf("LoadRoot 返回错误: %v", err)
	}
	if cfg.ListenAddr() != "127.0.0.1:8080" {
		t.Fatalf("默认监听地址 = %q, want 127.0.0.1:8080", cfg.ListenAddr())
	}
}

func TestLoadRootDoesNotDefaultLegacyPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ops.yaml")
	content := `plugins:
  paths: [plugins]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadRoot(path)
	if err != nil {
		t.Fatalf("LoadRoot 返回错误: %v", err)
	}
	if len(cfg.Paths.Tools) != 0 || len(cfg.Paths.Workflows) != 0 {
		t.Fatalf("legacy paths 不应被默认值覆盖: %#v", cfg.Paths)
	}
}

func TestSaveRootPreservesDiskFieldsAndOmitsRuntimeCategories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ops.yaml")
	cfg := &RootConfig{
		Name:        "legacy-name",
		Description: "legacy-desc",
		Categories:  []Category{{ID: "legacy", Name: "Legacy"}},
		Plugins:     PluginsConfig{Paths: []string{"plugins"}, Disabled: []string{"vendor.disabled"}},
		RuntimeCategories: []Category{
			{ID: "runtime", Name: "Runtime"},
		},
	}
	if err := SaveRoot(path, cfg); err != nil {
		t.Fatalf("SaveRoot 返回错误: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"name: legacy-name", "description: legacy-desc", "id: legacy", "vendor.disabled"} {
		if !strings.Contains(text, want) {
			t.Fatalf("SaveRoot 未保留 %q: %s", want, text)
		}
	}
	if strings.Contains(text, "runtime") {
		t.Fatalf("SaveRoot 不应写入运行期分类: %s", text)
	}
}

func TestLoadToolNewSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool.yaml")
	content := `id: demo.hello
name: Hello
category: demo
parameters:
  - name: name
    type: string
    required: true
execution:
  type: shell
  entry: bin/run.sh
  timeout: 1m
pass_mode:
  args: true
confirm:
  required: true
  message: continue?
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadTool(path)
	if err != nil {
		t.Fatalf("LoadTool 返回错误: %v", err)
	}
	if cfg.Execution.Entry != "bin/run.sh" || cfg.Timeout != "1m" {
		t.Fatalf("工具配置不符合预期: %#v", cfg)
	}
	if !cfg.Confirm.Required {
		t.Fatalf("确认配置未规范化: %#v", cfg.Confirm)
	}
}

func TestLoadWorkflowDAGSchemaWithEmbeddedLoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow-loop.yaml")
	content := `id: demo.loop
nodes:
  - id: repeat
    type: loop
    loop:
      tool: demo.greet
      params:
        name: "{{ .name }}"
      max_iterations: 2
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("LoadWorkflow 返回错误: %v", err)
	}
	if len(cfg.Nodes) != 1 || cfg.Nodes[0].Type != WorkflowNodeTypeLoop || cfg.Nodes[0].Loop.Tool != "demo.greet" || cfg.Nodes[0].Loop.MaxIterations != 2 {
		t.Fatalf("循环工作流配置不符合预期: %#v", cfg)
	}
}

package config

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestPromptMissing(t *testing.T) {
	params := map[string]string{}
	err := PromptMissing([]Parameter{{Name: "name", Required: true}}, params, bytes.NewBufferString("ops\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("PromptMissing 返回错误: %v", err)
	}
	if params["name"] != "ops" {
		t.Fatalf("name = %q, want ops", params["name"])
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

func TestLoadWorkflowDAGSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	content := `id: demo.flow
nodes:
  - id: first
    tool: demo.first
  - id: second
    tool: demo.second
edges:
  - from: first
    to: second
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("LoadWorkflow 返回错误: %v", err)
	}
	if len(cfg.Nodes) != 2 || len(cfg.Edges) != 1 {
		t.Fatalf("工作流配置不符合预期: %#v", cfg)
	}
}

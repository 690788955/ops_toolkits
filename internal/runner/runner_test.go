package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

func TestRunWorkflowRejectsUnconfirmedToolNode(t *testing.T) {
	dir := t.TempDir()
	toolDir := writeTool(t, dir, "danger", `#!/usr/bin/env bash
set -euo pipefail
echo danger
`)
	cfg := toolConfig("demo.danger")
	cfg.Confirm = config.Confirmation{Required: true, Message: "确认危险操作？"}
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools: map[string]*registry.Tool{
			"demo.danger": {Entry: config.ToolEntry{ID: "demo.danger", Category: "demo"}, Config: cfg, Dir: toolDir},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{ID: "demo.flow", Nodes: []config.WorkflowNode{{ID: "danger", Tool: "demo.danger"}}}
	reg.Workflows["demo.flow"] = &registry.Workflow{Entry: config.WorkflowRef{ID: "demo.flow"}, Config: wf}

	_, err := New(reg).RunWorkflow(context.Background(), "demo.flow", nil, nilWriter{}, nilWriter{})
	if err == nil || !strings.Contains(err.Error(), "需要确认") {
		t.Fatalf("RunWorkflow error = %v, want 需要确认", err)
	}
	_, err = New(reg).RunWorkflowWithConfirmation(context.Background(), "demo.flow", nil, true, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflowWithConfirmation error = %v", err)
	}
}

func TestRunWorkflowPassesUpstreamParamsAndOutput(t *testing.T) {
	dir := t.TempDir()
	producerDir := writeTool(t, dir, "producer", `#!/usr/bin/env bash
set -euo pipefail
echo "generated-${OPS_PARAM_NAME}"
`)
	consumerDir := writeTool(t, dir, "consumer", `#!/usr/bin/env bash
set -euo pipefail
echo "input=${OPS_PARAM_INPUT}"
echo "source=${OPS_PARAM_SOURCE}"
`)
	reg := &registry.Registry{
		BaseDir: dir,
		Root: &config.RootConfig{
			Paths: config.PathsConfig{Logs: "runs/logs"},
		},
		Tools: map[string]*registry.Tool{
			"demo.producer": {
				Entry:  config.ToolEntry{ID: "demo.producer", Category: "demo"},
				Config: toolConfig("demo.producer"),
				Dir:    producerDir,
			},
			"demo.consumer": {
				Entry:  config.ToolEntry{ID: "demo.consumer", Category: "demo"},
				Config: toolConfig("demo.consumer"),
				Dir:    consumerDir,
			},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{
		ID:       "demo.flow",
		Category: "demo",
		Parameters: []config.Parameter{
			{Name: "name", Required: true},
		},
		Nodes: []config.WorkflowNode{
			{ID: "first", Tool: "demo.producer", Params: map[string]interface{}{"name": "{{ .name }}"}},
			{ID: "second", Tool: "demo.consumer", Params: map[string]interface{}{"input": "{{ .steps.first.stdout }}", "source": "{{ .steps.first.params.name }}"}},
		},
		Edges: []config.WorkflowEdge{{From: "first", To: "second"}},
	}
	reg.Workflows["demo.flow"] = &registry.Workflow{Entry: config.WorkflowRef{ID: "demo.flow", Category: "demo"}, Config: wf, Path: filepath.Join(dir, "workflows", "demo.flow.yaml")}

	r := New(reg)
	record, err := r.RunWorkflow(context.Background(), "demo.flow", map[string]string{"name": "demo"}, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	if record.Status != "succeeded" {
		t.Fatalf("status = %s", record.Status)
	}
	consumerLog := readFile(t, filepath.Join(r.RunsDir, record.ID, "second", "stdout.log"))
	if !strings.Contains(consumerLog, "input=generated-demo") || !strings.Contains(consumerLog, "source=demo") {
		t.Fatalf("下游节点没有收到上游参数或输出: %s", consumerLog)
	}
}

func writeTool(t *testing.T, baseDir, name, script string) string {
	t.Helper()
	dir := filepath.Join(baseDir, "tools", "demo", name)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "run.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func toolConfig(id string) *config.ToolConfig {
	return &config.ToolConfig{
		ID:       id,
		Category: "demo",
		Execution: config.ExecutionConfig{
			Type:  "shell",
			Entry: "bin/run.sh",
		},
		PassMode: config.PassMode{Env: true},
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

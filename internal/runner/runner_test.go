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

func TestRunWorkflowRoutesConditionBranchAndSkipsInactive(t *testing.T) {
	dir := t.TempDir()
	inspectDir := writeTool(t, dir, "inspect", `#!/usr/bin/env bash
set -euo pipefail
echo "STATUS=OK"
`)
	okDir := writeTool(t, dir, "ok", `#!/usr/bin/env bash
set -euo pipefail
echo ok-branch
`)
	warnDir := writeTool(t, dir, "warn", `#!/usr/bin/env bash
set -euo pipefail
echo warn-branch
`)
	sharedDir := writeTool(t, dir, "shared", `#!/usr/bin/env bash
set -euo pipefail
echo shared
`)
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools: map[string]*registry.Tool{
			"demo.inspect": {Entry: config.ToolEntry{ID: "demo.inspect", Category: "demo"}, Config: toolConfig("demo.inspect"), Dir: inspectDir},
			"demo.ok":      {Entry: config.ToolEntry{ID: "demo.ok", Category: "demo"}, Config: toolConfig("demo.ok"), Dir: okDir},
			"demo.warn":    {Entry: config.ToolEntry{ID: "demo.warn", Category: "demo"}, Config: toolConfig("demo.warn"), Dir: warnDir},
			"demo.shared":  {Entry: config.ToolEntry{ID: "demo.shared", Category: "demo"}, Config: toolConfig("demo.shared"), Dir: sharedDir},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{
		ID: "demo.flow",
		Nodes: []config.WorkflowNode{
			{ID: "inspect", Tool: "demo.inspect"},
			{ID: "route", Type: config.WorkflowNodeTypeCondition, Condition: config.WorkflowCondition{Input: "{{ .steps.inspect.stdout }}", Cases: []config.ConditionCase{{ID: "ok", Name: "正常", Operator: "contains", Values: []string{"OK"}}, {ID: "warn", Name: "告警", Operator: "contains", Values: []string{"WARN"}}}, DefaultCase: "default"}},
			{ID: "ok", Tool: "demo.ok"},
			{ID: "warn", Tool: "demo.warn"},
			{ID: "shared", Tool: "demo.shared"},
		},
		Edges: []config.WorkflowEdge{{From: "inspect", To: "route"}, {From: "route", To: "ok", Case: "ok"}, {From: "route", To: "warn", Case: "warn"}, {From: "ok", To: "shared"}, {From: "warn", To: "shared"}, {From: "inspect", To: "shared"}},
	}
	reg.Workflows["demo.flow"] = &registry.Workflow{Entry: config.WorkflowRef{ID: "demo.flow", Category: "demo"}, Config: wf, Path: filepath.Join(dir, "workflows", "demo.flow.yaml")}

	r := New(reg)
	record, err := r.RunWorkflow(context.Background(), "demo.flow", nil, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	if record.Status != "succeeded" {
		t.Fatalf("status = %s", record.Status)
	}
	steps := map[string]StepRecord{}
	for _, step := range record.Steps {
		steps[step.ID] = step
	}
	if steps["route"].MatchedCase != "ok" || steps["route"].ConditionInput != "STATUS=OK" {
		t.Fatalf("condition step = %#v, want matched ok with input", steps["route"])
	}
	if steps["ok"].Status != "succeeded" || steps["warn"].Status != "skipped" || steps["shared"].Status != "succeeded" {
		t.Fatalf("steps = %#v, want ok succeeded, warn skipped, shared succeeded", steps)
	}
	if _, err := os.Stat(filepath.Join(r.RunsDir, record.ID, "warn", "stdout.log")); !os.IsNotExist(err) {
		t.Fatalf("inactive branch should not run, stat err = %v", err)
	}
}

func TestRunWorkflowUsesDefaultConditionBranch(t *testing.T) {
	dir := t.TempDir()
	inspectDir := writeTool(t, dir, "inspect", `#!/usr/bin/env bash
set -euo pipefail
echo "STATUS=UNKNOWN"
`)
	defaultDir := writeTool(t, dir, "default", `#!/usr/bin/env bash
set -euo pipefail
echo default-branch
`)
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools: map[string]*registry.Tool{
			"demo.inspect": {Entry: config.ToolEntry{ID: "demo.inspect", Category: "demo"}, Config: toolConfig("demo.inspect"), Dir: inspectDir},
			"demo.default": {Entry: config.ToolEntry{ID: "demo.default", Category: "demo"}, Config: toolConfig("demo.default"), Dir: defaultDir},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{
		ID: "demo.flow",
		Nodes: []config.WorkflowNode{
			{ID: "inspect", Tool: "demo.inspect"},
			{ID: "route", Type: config.WorkflowNodeTypeCondition, Condition: config.WorkflowCondition{Input: "{{ .steps.inspect.stdout }}", Cases: []config.ConditionCase{{ID: "ok", Name: "正常", Operator: "contains", Values: []string{"OK"}}}, DefaultCase: "default"}},
			{ID: "fallback", Tool: "demo.default"},
		},
		Edges: []config.WorkflowEdge{{From: "inspect", To: "route"}, {From: "route", To: "fallback", Case: "default"}},
	}
	reg.Workflows["demo.flow"] = &registry.Workflow{Entry: config.WorkflowRef{ID: "demo.flow", Category: "demo"}, Config: wf}

	r := New(reg)
	record, err := r.RunWorkflow(context.Background(), "demo.flow", nil, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	steps := map[string]StepRecord{}
	for _, step := range record.Steps {
		steps[step.ID] = step
	}
	if steps["route"].MatchedCase != "default" || steps["fallback"].Status != "succeeded" {
		t.Fatalf("steps = %#v, want default branch succeeded", steps)
	}
}

func TestRunWorkflowConditionWithoutDefaultSkipsAllBranches(t *testing.T) {
	dir := t.TempDir()
	inspectDir := writeTool(t, dir, "inspect-none", `#!/usr/bin/env bash
set -euo pipefail
echo "STATUS=UNKNOWN"
`)
	okDir := writeTool(t, dir, "ok-none", `#!/usr/bin/env bash
set -euo pipefail
echo should-not-run
`)
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools: map[string]*registry.Tool{
			"demo.inspect": {Entry: config.ToolEntry{ID: "demo.inspect", Category: "demo"}, Config: toolConfig("demo.inspect"), Dir: inspectDir},
			"demo.ok":      {Entry: config.ToolEntry{ID: "demo.ok", Category: "demo"}, Config: toolConfig("demo.ok"), Dir: okDir},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{
		ID: "demo.flow",
		Nodes: []config.WorkflowNode{
			{ID: "inspect", Tool: "demo.inspect"},
			{ID: "route", Type: config.WorkflowNodeTypeCondition, Condition: config.WorkflowCondition{Input: "{{ .steps.inspect.stdout }}", Cases: []config.ConditionCase{{ID: "ok", Name: "正常", Operator: "contains", Values: []string{"OK"}}}}},
			{ID: "ok", Tool: "demo.ok"},
		},
		Edges: []config.WorkflowEdge{{From: "inspect", To: "route"}, {From: "route", To: "ok", Case: "ok"}},
	}
	reg.Workflows["demo.flow"] = &registry.Workflow{Entry: config.WorkflowRef{ID: "demo.flow", Category: "demo"}, Config: wf}

	r := New(reg)
	record, err := r.RunWorkflow(context.Background(), "demo.flow", nil, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	steps := map[string]StepRecord{}
	for _, step := range record.Steps {
		steps[step.ID] = step
	}
	if steps["route"].MatchedCase != "" || steps["ok"].Status != "skipped" {
		t.Fatalf("steps = %#v, want no matched case and skipped ok branch", steps)
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

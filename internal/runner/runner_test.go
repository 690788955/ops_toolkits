package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestRunToolUsesBarePathCommand(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "plugins", "vendor.pathcmd")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := toolConfig("vendor.pathcmd.run")
	cfg.Execution.Entry = "go"
	cfg.Execution.Args = []string{"version"}
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools: map[string]*registry.Tool{
			"vendor.pathcmd.run": {Entry: config.ToolEntry{ID: "vendor.pathcmd.run", Category: "demo"}, Config: cfg, Dir: toolDir},
		},
		Workflows: map[string]*registry.Workflow{},
	}

	record, err := New(reg).RunTool(context.Background(), "vendor.pathcmd.run", nil, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunTool error: %v", err)
	}
	stdout := readFile(t, filepath.Join(dir, "runs", "logs", record.ID, "stdout.log"))
	if !strings.Contains(stdout, "go version") {
		t.Fatalf("stdout = %q, want go version", stdout)
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

func TestRunWorkflowExecutesEmbeddedLoopTool(t *testing.T) {
	dir := t.TempDir()
	loopDir := writeTool(t, dir, "loop", `#!/usr/bin/env bash
set -euo pipefail
echo "loop-${OPS_PARAM_NAME}"
`)
	afterDir := writeTool(t, dir, "after-loop", `#!/usr/bin/env bash
set -euo pipefail
echo "after=${OPS_PARAM_INPUT}"
`)
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools: map[string]*registry.Tool{
			"demo.loop":  {Entry: config.ToolEntry{ID: "demo.loop", Category: "demo"}, Config: toolConfig("demo.loop"), Dir: loopDir},
			"demo.after": {Entry: config.ToolEntry{ID: "demo.after", Category: "demo"}, Config: toolConfig("demo.after"), Dir: afterDir},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{
		ID:         "demo.loop-flow",
		Parameters: []config.Parameter{{Name: "name", Required: true}},
		Nodes: []config.WorkflowNode{
			{ID: "repeat", Type: config.WorkflowNodeTypeLoop, Loop: config.WorkflowLoop{Tool: "demo.loop", Params: map[string]interface{}{"name": "{{ .name }}"}, MaxIterations: 3}},
			{ID: "after", Tool: "demo.after", Params: map[string]interface{}{"input": "{{ .steps.repeat.stdout }}"}},
		},
		Edges: []config.WorkflowEdge{{From: "repeat", To: "after"}},
	}
	reg.Workflows["demo.loop-flow"] = &registry.Workflow{Entry: config.WorkflowRef{ID: "demo.loop-flow"}, Config: wf}

	r := New(reg)
	record, err := r.RunWorkflow(context.Background(), "demo.loop-flow", map[string]string{"name": "demo"}, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	steps := map[string]StepRecord{}
	for _, step := range record.Steps {
		steps[step.ID] = step
	}
	if record.Status != "succeeded" || steps["repeat"].Status != "succeeded" || steps["repeat"].Tool != "demo.loop" {
		t.Fatalf("record = %#v, steps = %#v", record, steps)
	}
	loopLog := readFile(t, filepath.Join(r.RunsDir, record.ID, "repeat", "stdout.log"))
	if strings.Count(loopLog, "loop-demo") != 3 {
		t.Fatalf("loop aggregate log = %q, want 3 iterations", loopLog)
	}
	afterLog := readFile(t, filepath.Join(r.RunsDir, record.ID, "after", "stdout.log"))
	if !strings.Contains(afterLog, "loop-demo") {
		t.Fatalf("after node did not receive loop aggregate stdout: %q", afterLog)
	}
}

func TestRunWorkflowStopsOnEmbeddedLoopToolFailure(t *testing.T) {
	dir := t.TempDir()
	loopDir := writeTool(t, dir, "loop-fail", `#!/usr/bin/env bash
set -euo pipefail
exit 7
`)
	afterDir := writeTool(t, dir, "after-fail", `#!/usr/bin/env bash
set -euo pipefail
echo should-not-run
`)
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools: map[string]*registry.Tool{
			"demo.loop":  {Entry: config.ToolEntry{ID: "demo.loop", Category: "demo"}, Config: toolConfig("demo.loop"), Dir: loopDir},
			"demo.after": {Entry: config.ToolEntry{ID: "demo.after", Category: "demo"}, Config: toolConfig("demo.after"), Dir: afterDir},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{
		ID: "demo.loop-fail",
		Nodes: []config.WorkflowNode{
			{ID: "repeat", Type: config.WorkflowNodeTypeLoop, Loop: config.WorkflowLoop{Tool: "demo.loop", MaxIterations: 2}},
			{ID: "after", Tool: "demo.after"},
		},
		Edges: []config.WorkflowEdge{{From: "repeat", To: "after"}},
	}
	reg.Workflows["demo.loop-fail"] = &registry.Workflow{Entry: config.WorkflowRef{ID: "demo.loop-fail"}, Config: wf}

	record, err := New(reg).RunWorkflow(context.Background(), "demo.loop-fail", nil, nilWriter{}, nilWriter{})
	if err == nil {
		t.Fatalf("RunWorkflow expected failure")
	}
	if record.Status != "failed" || len(record.Steps) != 1 || record.Steps[0].ID != "repeat" || record.Steps[0].Status != "failed" {
		t.Fatalf("record = %#v", record)
	}
}

func TestRunWorkflowPassesParallelAndJoinControlNodes(t *testing.T) {
	dir := t.TempDir()
	toolDir := writeTool(t, dir, "done", `#!/usr/bin/env bash
set -euo pipefail
echo done
`)
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools: map[string]*registry.Tool{
			"demo.done": {Entry: config.ToolEntry{ID: "demo.done", Category: "demo"}, Config: toolConfig("demo.done"), Dir: toolDir},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{
		ID: "demo.control-flow",
		Nodes: []config.WorkflowNode{
			{ID: "split", Type: config.WorkflowNodeTypeParallel},
			{ID: "join", Type: config.WorkflowNodeTypeJoin},
			{ID: "done", Tool: "demo.done"},
		},
		Edges: []config.WorkflowEdge{{From: "split", To: "join"}, {From: "join", To: "done"}},
	}
	reg.Workflows["demo.control-flow"] = &registry.Workflow{Entry: config.WorkflowRef{ID: "demo.control-flow"}, Config: wf}

	record, err := New(reg).RunWorkflow(context.Background(), "demo.control-flow", nil, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	steps := map[string]StepRecord{}
	for _, step := range record.Steps {
		steps[step.ID] = step
	}
	if record.Status != "succeeded" || steps["split"].Status != "succeeded" || steps["join"].Status != "succeeded" || steps["done"].Status != "succeeded" {
		t.Fatalf("record = %#v, steps = %#v", record, steps)
	}
}

func TestRunWorkflowUploadNodeWritesContextForDownstreamTool(t *testing.T) {
	dir := t.TempDir()
	consumerDir := writeTool(t, dir, "upload-consumer", `#!/usr/bin/env bash
set -euo pipefail
echo "path=${OPS_PARAM_PATH}"
echo "name=${OPS_PARAM_NAME}"
echo "count=${OPS_PARAM_COUNT}"
echo "paths=${OPS_PARAM_PATHS}"
`)
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools: map[string]*registry.Tool{
			"demo.consumer": {Entry: config.ToolEntry{ID: "demo.consumer", Category: "demo"}, Config: toolConfig("demo.consumer"), Dir: consumerDir},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{
		ID: "demo.upload-flow",
		Nodes: []config.WorkflowNode{
			{ID: "upload", Type: config.WorkflowNodeTypeUpload, Upload: config.WorkflowUpload{TargetDir: "assets"}},
			{ID: "consume", Tool: "demo.consumer", Params: map[string]interface{}{"path": "{{ .steps.upload.file.path }}", "name": "{{ .steps.upload.file.filename }}", "count": "{{ .steps.upload.files.count }}", "paths": "{{ .steps.upload.files.paths }}"}},
		},
		Edges: []config.WorkflowEdge{{From: "upload", To: "consume"}},
	}
	firstPath := filepath.Join(dir, "runs", "uploads", "assets", "upload-1", "a.zip")
	secondPath := filepath.Join(dir, "runs", "uploads", "assets", "upload-1", "dir", "b.zip")
	upload := config.WorkflowUploadResult{
		ID:           "upload-1",
		FileName:     "a.zip",
		Path:         firstPath,
		RelativePath: "runs/uploads/assets/upload-1/a.zip",
		Size:         123,
		Files: []config.WorkflowUploadFile{
			{FileName: "a.zip", Path: firstPath, RelativePath: "runs/uploads/assets/upload-1/a.zip", Size: 123},
			{FileName: "b.zip", Path: secondPath, RelativePath: "runs/uploads/assets/upload-1/dir/b.zip", Size: 456},
		},
		Count:     2,
		TotalSize: 579,
	}

	r := New(reg)
	record, err := r.RunWorkflowConfigWithUploads(context.Background(), wf, nil, false, map[string]config.WorkflowUploadResult{"upload": upload}, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflowConfigWithUploads error: %v", err)
	}
	if record.Status != "succeeded" {
		t.Fatalf("status = %s", record.Status)
	}
	uploadStdout := readFile(t, filepath.Join(r.RunsDir, record.ID, "upload", "stdout.log"))
	if !strings.Contains(uploadStdout, "SUCCESS 上传节点 upload 上传完成") || !strings.Contains(uploadStdout, `"filename":"a.zip"`) || !strings.Contains(uploadStdout, `"relative_path":"runs/uploads/assets/upload-1/a.zip"`) {
		t.Fatalf("upload stdout = %s", uploadStdout)
	}
	consumerStdout := readFile(t, filepath.Join(r.RunsDir, record.ID, "consume", "stdout.log"))
	if !strings.Contains(filepath.ToSlash(consumerStdout), "/runs/uploads/assets/upload-1/a.zip") || !strings.Contains(consumerStdout, "name=a.zip") || !strings.Contains(consumerStdout, "count=2") || !strings.Contains(filepath.ToSlash(consumerStdout), "/runs/uploads/assets/upload-1/dir/b.zip") {
		t.Fatalf("consumer stdout = %s", consumerStdout)
	}
}

func TestRunWorkflowUploadNodeFailsWithoutUploadResult(t *testing.T) {
	dir := t.TempDir()
	reg := &registry.Registry{
		BaseDir:   dir,
		Root:      &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools:     map[string]*registry.Tool{},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{
		ID:    "demo.upload-flow",
		Nodes: []config.WorkflowNode{{ID: "upload", Type: config.WorkflowNodeTypeUpload}},
	}

	r := New(reg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	record, err := r.RunWorkflowConfigWithUploads(ctx, wf, nil, false, nil, nilWriter{}, nilWriter{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunWorkflowConfigWithUploads error = %v, want context canceled", err)
	}
	if record.Status != "cancelled" || len(record.Steps) != 1 || record.Steps[0].Status != "cancelled" {
		t.Fatalf("record = %#v", record)
	}
}

func TestRunWorkflowUploadNodeWaitsForUploadResultFile(t *testing.T) {
	dir := t.TempDir()
	reg := &registry.Registry{
		BaseDir:   dir,
		Root:      &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools:     map[string]*registry.Tool{},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{
		ID:    "demo.upload-flow",
		Nodes: []config.WorkflowNode{{ID: "upload", Type: config.WorkflowNodeTypeUpload}},
	}

	r := New(reg)
	record, err := r.StartWorkflowConfigWithUploads(context.Background(), wf, nil, false, nil, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("StartWorkflowConfigWithUploads error: %v", err)
	}
	runDir := filepath.Join(r.RunsDir, record.ID)
	waitForRunnerStepStatus(t, runDir, "upload", "waiting")
	waitStdout := readFile(t, filepath.Join(runDir, "upload", "stdout.log"))
	if !strings.Contains(waitStdout, "WAIT 上传节点 upload 等待选择的文件上传到平台") {
		t.Fatalf("wait stdout = %s", waitStdout)
	}
	upload := config.WorkflowUploadResult{
		ID:           "upload-1",
		FileName:     "a.zip",
		Path:         filepath.Join(dir, "runs", "uploads", "upload-1", "a.zip"),
		RelativePath: "runs/uploads/upload-1/a.zip",
		Size:         123,
		Count:        1,
		TotalSize:    123,
	}
	if err := WriteWorkflowUploadResult(runDir, "upload", upload); err != nil {
		t.Fatalf("WriteWorkflowUploadResult error: %v", err)
	}
	finalRecord := waitForRunnerRecordStatus(t, runDir, "succeeded")
	if len(finalRecord.Steps) != 1 || finalRecord.Steps[0].Status != "succeeded" {
		t.Fatalf("record = %#v", finalRecord)
	}
	stdout := readFile(t, filepath.Join(runDir, "upload", "stdout.log"))
	if !strings.Contains(stdout, "WAIT 上传节点 upload") || !strings.Contains(stdout, "LOG 上传节点 upload 已接收上传文件") || !strings.Contains(stdout, "SUCCESS 上传节点 upload 上传完成") || !strings.Contains(stdout, `"filename":"a.zip"`) {
		t.Fatalf("stdout = %s", stdout)
	}
}

func TestRunToolMergesLayeredConfigWritesNestedParamFileTemplatesAndRedacts(t *testing.T) {
	dir := t.TempDir()
	toolDir := writeTool(t, dir, "layered", `#!/usr/bin/env bash
set -euo pipefail
echo "param_file=${OPS_PARAM_FILE}"
test -f "${OPS_PARAM_FILE}"
`)
	if err := os.MkdirAll(filepath.Join(toolDir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "templates", "native.tmpl"), []byte("host={{ .es.host }}\nport={{ .es.port }}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := toolConfig("plugin.layered.run")
	cfg.Parameters = []config.Parameter{{Name: "runtime", Default: "param-default"}, {Name: "es.host", Default: "parameter"}, {Name: "es.password", Sensitive: true}}
	cfg.PassMode = config.PassMode{Env: true, ParamFile: true, FileName: "params.yaml"}
	cfg.ConfigDefaults = map[string]interface{}{"es": map[string]interface{}{"port": "tool", "items": []interface{}{"tool"}}}
	cfg.PluginConfig = config.PluginToolConfig{
		ID:                   "plugin.layered",
		Dir:                  toolDir,
		SharedConfig:         map[string]interface{}{"es": map[string]interface{}{"host": "shared", "port": "shared"}},
		PackageDefaultConfig: map[string]interface{}{"es": map[string]interface{}{"host": "package", "nested": map[string]interface{}{"keep": "package"}}},
		HostConfig:           map[string]interface{}{"es": map[string]interface{}{"host": "host", "port": "host", "password": "secret:env:ES_PASSWORD"}},
		SensitivePaths:       []string{"es.password"},
	}
	reg := &registry.Registry{
		BaseDir: dir,
		Root: &config.RootConfig{
			Paths:          config.PathsConfig{Logs: "runs/logs"},
			ConfigDefaults: map[string]interface{}{"es": map[string]interface{}{"host": "global", "port": "global"}},
		},
		GlobalEnv: config.Values{"es": config.Values{"host": "env", "port": "env", "from_env": "global-env"}},
		Tools: map[string]*registry.Tool{
			"plugin.layered.run": {Entry: config.ToolEntry{ID: "plugin.layered.run", Category: "demo"}, Config: cfg, Dir: toolDir},
		},
		Workflows: map[string]*registry.Workflow{},
	}

	r := New(reg)
	record, err := r.RunTool(context.Background(), "plugin.layered.run", map[string]string{"es.host": "runtime", "runtime": "override"}, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunTool error: %v", err)
	}
	paramsYAML := readFile(t, filepath.Join(r.RunsDir, record.ID, "params.yaml"))
	if !strings.Contains(paramsYAML, "host: runtime") || !strings.Contains(paramsYAML, "port: host") || !strings.Contains(paramsYAML, "keep: package") || !strings.Contains(paramsYAML, "from_env: global-env") {
		t.Fatalf("params.yaml 未包含期望合并结果:\n%s", paramsYAML)
	}
	if record.Params["es.password"] != "******" || record.Config["es"].(config.Values)["password"] != "******" {
		t.Fatalf("运行记录未脱敏: %#v", record)
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

func waitForRunnerStepStatus(t *testing.T, runDir, stepID, status string) RunRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		record, ok := tryReadRunnerRecord(runDir)
		if !ok {
			if time.Now().After(deadline) {
				t.Fatalf("step %s did not reach status %s; result.json was not readable", stepID, status)
			}
			time.Sleep(25 * time.Millisecond)
			continue
		}
		for _, step := range record.Steps {
			if step.ID == stepID && step.Status == status {
				return record
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("step %s did not reach status %s, record=%#v", stepID, status, record)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func waitForRunnerRecordStatus(t *testing.T, runDir, status string) RunRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		record, ok := tryReadRunnerRecord(runDir)
		if !ok {
			if time.Now().After(deadline) {
				t.Fatalf("record did not reach status %s; result.json was not readable", status)
			}
			time.Sleep(25 * time.Millisecond)
			continue
		}
		if record.Status == status {
			return record
		}
		if time.Now().After(deadline) {
			t.Fatalf("record did not reach status %s, record=%#v", status, record)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func readRunnerRecord(t *testing.T, runDir string) RunRecord {
	t.Helper()
	record, ok := tryReadRunnerRecord(runDir)
	if !ok {
		t.Fatalf("read runner record from %s", runDir)
	}
	return record
}

func tryReadRunnerRecord(runDir string) (RunRecord, bool) {
	data, err := os.ReadFile(filepath.Join(runDir, "result.json"))
	if err != nil {
		return RunRecord{}, false
	}
	var record RunRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return RunRecord{}, false
	}
	return record, true
}

func TestWindowsPathToWSLConvertsDrivePath(t *testing.T) {
	got := windowsPathToWSL(`F:\ccb\ops_toolkits\plugins\plugin.template\scripts\sleep-30s.sh`)
	want := `/mnt/f/ccb/ops_toolkits/plugins/plugin.template/scripts/sleep-30s.sh`
	if got != want {
		t.Fatalf("windowsPathToWSL() = %q, want %q", got, want)
	}
}

func TestWSLBashScriptConvertsPathsAndExportsEnv(t *testing.T) {
	got := wslBashScript(
		`F:\ccb\ops_toolkits\plugins\plugin.template\scripts\run.sh`,
		`F:\ccb\ops_toolkits\plugins\plugin.template`,
		[]string{`OPS_PARAM_FILE=F:\ccb\ops_toolkits\runs\logs\tool-1\params.yaml`, `OPS_PARAM_NAME=demo`},
		[]string{`--params-file`, `F:\ccb\ops_toolkits\runs\logs\tool-1\params.yaml`},
	)
	for _, want := range []string{
		`cd '/mnt/f/ccb/ops_toolkits/plugins/plugin.template'`,
		`export OPS_PARAM_FILE='/mnt/f/ccb/ops_toolkits/runs/logs/tool-1/params.yaml'`,
		`export OPS_PARAM_NAME='demo'`,
		`exec '/mnt/f/ccb/ops_toolkits/plugins/plugin.template/scripts/run.sh' '--params-file' '/mnt/f/ccb/ops_toolkits/runs/logs/tool-1/params.yaml'`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("wslBashScript() = %q, missing %q", got, want)
		}
	}
}

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

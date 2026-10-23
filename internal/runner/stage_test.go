package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

func TestRerunStageWorkflowContinuesSerialSuccessors(t *testing.T) {
	for _, initiallyFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed run", true: "failed run"}[initiallyFails], func(t *testing.T) {
			var mu sync.Mutex
			counts := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				counts[req.URL.Path]++
				if initiallyFails && req.URL.Path == "/second" && counts["/second"] == 1 {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer server.Close()
			r, _ := newHTTPTestRunner(t)
			wf := &config.WorkflowConfig{
				ID: "demo.stage-rerun",
				Stages: []config.WorkflowStage{{ID: "prepare"}, {ID: "verify"}},
				Nodes: []config.WorkflowNode{
					{ID: "first", Type: config.WorkflowNodeTypeHTTP, StageID: "prepare", Order: 10, HTTP: config.WorkflowHTTP{URL: server.URL + "/first"}},
					{ID: "second", Type: config.WorkflowNodeTypeHTTP, StageID: "prepare", Order: 20, HTTP: config.WorkflowHTTP{URL: server.URL + "/second"}},
					{ID: "third", Type: config.WorkflowNodeTypeHTTP, StageID: "verify", Order: 10, HTTP: config.WorkflowHTTP{URL: server.URL + "/third"}},
				},
			}
			record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
			if (err != nil) != initiallyFails {
				t.Fatalf("initial error = %v, want failure %v", err, initiallyFails)
			}
			updated, err := r.RerunWorkflowNode(context.Background(), wf, record, filepath.Join(r.RunsDir, record.ID), "second", false, nilWriter{}, nilWriter{})
			if err != nil || updated.Status != "succeeded" {
				t.Fatalf("rerun error = %v, record = %+v", err, updated)
			}
			wantThird := 2
			if initiallyFails {
				wantThird = 1
			}
			mu.Lock()
			defer mu.Unlock()
			if counts["/first"] != 1 || counts["/second"] != 2 || counts["/third"] != wantThird {
				t.Fatalf("execution counts = %v, want first=1, second=2, third=%d", counts, wantThird)
			}
			if len(updated.Steps) != 3 || len(wf.Edges) != 0 {
				t.Fatalf("steps = %+v, edges = %+v; want 3 steps and no persisted edges", updated.Steps, wf.Edges)
			}
		})
	}
}

func TestRunWorkflowExecutesSerialStagesInCanvasOrder(t *testing.T) {
	dir := t.TempDir()
	firstDir := writeTool(t, dir, "first", `#!/usr/bin/env bash
set -euo pipefail
echo first
`)
	secondDir := writeTool(t, dir, "second", `#!/usr/bin/env bash
set -euo pipefail
echo second
`)
	reg := &registry.Registry{
		BaseDir: dir,
		Root:    &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools: map[string]*registry.Tool{
			"demo.first":  {Entry: config.ToolEntry{ID: "demo.first", Category: "demo"}, Config: toolConfig("demo.first"), Dir: firstDir},
			"demo.second": {Entry: config.ToolEntry{ID: "demo.second", Category: "demo"}, Config: toolConfig("demo.second"), Dir: secondDir},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	wf := &config.WorkflowConfig{
		ID: "demo.stages",
		Stages: []config.WorkflowStage{
			{ID: "prepare", Name: "准备"},
			{ID: "change", Name: "变更"},
		},
		Nodes: []config.WorkflowNode{
			{ID: "z-change", Type: config.WorkflowNodeTypeTool, Tool: "demo.second", StageID: "change", Order: 10},
			{ID: "a-prepare", Type: config.WorkflowNodeTypeTool, Tool: "demo.first", StageID: "prepare", Order: 10},
		},
	}
	reg.Workflows["demo.stages"] = &registry.Workflow{Entry: config.WorkflowRef{ID: "demo.stages"}, Config: wf}

	record, err := New(reg).RunWorkflow(context.Background(), "demo.stages", nil, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	if len(record.Steps) != 2 {
		t.Fatalf("steps = %#v, want 2", record.Steps)
	}
	if record.Steps[0].ID != "a-prepare" || record.Steps[1].ID != "z-change" {
		t.Fatalf("step order = %s, %s, want a-prepare, z-change", record.Steps[0].ID, record.Steps[1].ID)
	}
}

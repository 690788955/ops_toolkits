package registry

import (
	"strings"
	"testing"

	"shell_ops/internal/config"
)

func TestValidateWorkflowRejectsMissingTool(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{}}
	wf := &config.WorkflowConfig{ID: "demo.flow", Nodes: []config.WorkflowNode{{ID: "first", Tool: "missing.tool"}}}

	err := reg.ValidateWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "不存在的工具") {
		t.Fatalf("ValidateWorkflow error = %v, want 不存在的工具", err)
	}
}

func TestValidateWorkflowRejectsDuplicateNode(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{"demo.tool": {}}}
	wf := &config.WorkflowConfig{ID: "demo.flow", Nodes: []config.WorkflowNode{{ID: "first", Tool: "demo.tool"}, {ID: "first", Tool: "demo.tool"}}}

	err := reg.ValidateWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "节点 ID 重复") {
		t.Fatalf("ValidateWorkflow error = %v, want 节点 ID 重复", err)
	}
}

func TestValidateWorkflowRejectsMissingEdgeNode(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{"demo.tool": {}}}
	wf := &config.WorkflowConfig{
		ID:    "demo.flow",
		Nodes: []config.WorkflowNode{{ID: "first", Tool: "demo.tool"}},
		Edges: []config.WorkflowEdge{{From: "first", To: "missing"}},
	}

	err := reg.ValidateWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "不存在的节点") {
		t.Fatalf("ValidateWorkflow error = %v, want 不存在的节点", err)
	}
}

func TestValidateWorkflowRejectsCycle(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{"demo.tool": {}}}
	wf := &config.WorkflowConfig{
		ID: "demo.flow",
		Nodes: []config.WorkflowNode{
			{ID: "first", Tool: "demo.tool"},
			{ID: "second", Tool: "demo.tool"},
		},
		Edges: []config.WorkflowEdge{{From: "first", To: "second"}, {From: "second", To: "first"}},
	}

	err := reg.ValidateWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "环形依赖") {
		t.Fatalf("ValidateWorkflow error = %v, want 环形依赖", err)
	}
}

func TestOrderWorkflowReturnsDependencyOrder(t *testing.T) {
	wf := &config.WorkflowConfig{
		ID: "demo.flow",
		Nodes: []config.WorkflowNode{
			{ID: "second", Tool: "demo.tool"},
			{ID: "first", Tool: "demo.tool"},
		},
		Edges: []config.WorkflowEdge{{From: "first", To: "second"}},
	}

	ordered, err := OrderWorkflow(wf)
	if err != nil {
		t.Fatalf("OrderWorkflow returned error: %v", err)
	}
	if ordered[0].ID != "first" || ordered[1].ID != "second" {
		t.Fatalf("order = %#v, 期望 first 在 second 之前", ordered)
	}
}

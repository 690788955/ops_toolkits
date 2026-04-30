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

func TestValidateWorkflowAcceptsConditionNode(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{"demo.tool": {}}}
	wf := &config.WorkflowConfig{
		ID: "demo.flow",
		Nodes: []config.WorkflowNode{
			{ID: "inspect", Tool: "demo.tool"},
			{ID: "route", Type: config.WorkflowNodeTypeCondition, Condition: config.WorkflowCondition{Input: "{{ .steps.inspect.stdout }}", Cases: []config.ConditionCase{{ID: "ok", Name: "正常", Operator: "contains", Values: []string{"OK"}}, {ID: "warn", Name: "告警", Operator: "contains", Values: []string{"WARN"}}}, DefaultCase: "default"}},
			{ID: "apply", Tool: "demo.tool"},
			{ID: "notify", Tool: "demo.tool"},
		},
		Edges: []config.WorkflowEdge{{From: "inspect", To: "route"}, {From: "route", To: "apply", Case: "ok"}, {From: "route", To: "notify", Case: "default"}},
	}

	if err := reg.ValidateWorkflow(wf); err != nil {
		t.Fatalf("ValidateWorkflow error = %v", err)
	}
}

func TestValidateWorkflowRejectsInvalidConditionNode(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{"demo.tool": {}}}
	cases := []struct {
		name string
		wf   *config.WorkflowConfig
		want string
	}{
		{
			name: "tool on condition",
			wf:   &config.WorkflowConfig{ID: "demo.flow", Nodes: []config.WorkflowNode{{ID: "route", Type: config.WorkflowNodeTypeCondition, Tool: "demo.tool", Condition: config.WorkflowCondition{Input: "x", Cases: []config.ConditionCase{{ID: "ok", Name: "正常", Operator: "eq", Values: []string{"x"}}}}}}, Edges: []config.WorkflowEdge{{From: "route", To: "route", Case: "ok"}}},
			want: "不能同时配置 tool",
		},
		{
			name: "tool node with default case only",
			wf:   &config.WorkflowConfig{ID: "demo.flow", Nodes: []config.WorkflowNode{{ID: "first", Type: config.WorkflowNodeTypeTool, Tool: "demo.tool", Condition: config.WorkflowCondition{DefaultCase: "default"}}}},
			want: "不能配置 condition",
		},
		{
			name: "missing input",
			wf:   &config.WorkflowConfig{ID: "demo.flow", Nodes: []config.WorkflowNode{{ID: "route", Type: config.WorkflowNodeTypeCondition, Condition: config.WorkflowCondition{Cases: []config.ConditionCase{{ID: "ok", Name: "正常", Operator: "eq", Values: []string{"x"}}}}}}, Edges: []config.WorkflowEdge{{From: "route", To: "route", Case: "ok"}}},
			want: "condition.input 必填",
		},
		{
			name: "invalid operator",
			wf:   &config.WorkflowConfig{ID: "demo.flow", Nodes: []config.WorkflowNode{{ID: "route", Type: config.WorkflowNodeTypeCondition, Condition: config.WorkflowCondition{Input: "x", Cases: []config.ConditionCase{{ID: "ok", Name: "正常", Operator: "bad"}}}}}, Edges: []config.WorkflowEdge{{From: "route", To: "route", Case: "ok"}}},
			want: "非法 operator",
		},
		{
			name: "reserved default case id",
			wf:   &config.WorkflowConfig{ID: "demo.flow", Nodes: []config.WorkflowNode{{ID: "route", Type: config.WorkflowNodeTypeCondition, Condition: config.WorkflowCondition{Input: "x", Cases: []config.ConditionCase{{ID: "default", Name: "默认", Operator: "eq"}}}}}, Edges: []config.WorkflowEdge{{From: "route", To: "route", Case: "default"}}},
			want: "保留值 default",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := reg.ValidateWorkflow(tc.wf)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateWorkflow error = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestValidateWorkflowRejectsInvalidConditionEdges(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{"demo.tool": {}}}
	condition := config.WorkflowNode{ID: "route", Type: config.WorkflowNodeTypeCondition, Condition: config.WorkflowCondition{Input: "x", Cases: []config.ConditionCase{{ID: "ok", Name: "正常", Operator: "eq", Values: []string{"x"}}}}}
	cases := []struct {
		name  string
		nodes []config.WorkflowNode
		edges []config.WorkflowEdge
		want  string
	}{
		{name: "condition edge missing case", nodes: []config.WorkflowNode{condition, {ID: "next", Tool: "demo.tool"}}, edges: []config.WorkflowEdge{{From: "route", To: "next"}}, want: "出边必须配置 case"},
		{name: "condition edge missing case id", nodes: []config.WorkflowNode{condition, {ID: "next", Tool: "demo.tool"}}, edges: []config.WorkflowEdge{{From: "route", To: "next", Case: "bad"}}, want: "不存在的 case"},
		{name: "condition default edge without default_case", nodes: []config.WorkflowNode{condition, {ID: "next", Tool: "demo.tool"}}, edges: []config.WorkflowEdge{{From: "route", To: "next", Case: "default"}}, want: "未启用 default_case"},
		{name: "non condition edge has case", nodes: []config.WorkflowNode{{ID: "first", Tool: "demo.tool"}, {ID: "next", Tool: "demo.tool"}}, edges: []config.WorkflowEdge{{From: "first", To: "next", Case: "ok"}}, want: "非条件节点"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := reg.ValidateWorkflow(&config.WorkflowConfig{ID: "demo.flow", Nodes: tc.nodes, Edges: tc.edges})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateWorkflow error = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestValidateWorkflowAllowsConditionWithoutOutgoingEdges(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{"demo.tool": {}}}
	wf := &config.WorkflowConfig{
		ID: "demo.flow",
		Nodes: []config.WorkflowNode{
			{ID: "inspect", Tool: "demo.tool"},
			{ID: "route", Type: config.WorkflowNodeTypeCondition, Condition: config.WorkflowCondition{Input: "{{ .steps.inspect.stdout }}", Cases: []config.ConditionCase{{ID: "ok", Name: "正常", Operator: "contains", Values: []string{"OK"}}}}},
		},
		Edges: []config.WorkflowEdge{{From: "inspect", To: "route"}},
	}

	if err := reg.ValidateWorkflow(wf); err != nil {
		t.Fatalf("ValidateWorkflow error = %v", err)
	}
}

func TestValidateWorkflowAcceptsEmbeddedLoopNode(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{"demo.tool": {}}}
	wf := &config.WorkflowConfig{
		ID: "demo.loop",
		Nodes: []config.WorkflowNode{
			{ID: "loop", Type: config.WorkflowNodeTypeLoop, Loop: config.WorkflowLoop{Tool: "demo.tool", MaxIterations: 3, Params: map[string]interface{}{"name": "{{ .name }}"}}},
		},
	}

	if err := reg.ValidateWorkflow(wf); err != nil {
		t.Fatalf("ValidateWorkflow error = %v", err)
	}
}

func TestValidateWorkflowRejectsInvalidLoopNode(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{"demo.tool": {}}}
	cases := []struct {
		name string
		node config.WorkflowNode
		want string
	}{
		{name: "missing tool", node: config.WorkflowNode{ID: "loop", Type: config.WorkflowNodeTypeLoop, Loop: config.WorkflowLoop{MaxIterations: 1}}, want: "loop.tool 必填"},
		{name: "missing referenced tool", node: config.WorkflowNode{ID: "loop", Type: config.WorkflowNodeTypeLoop, Loop: config.WorkflowLoop{Tool: "missing.tool", MaxIterations: 1}}, want: "不存在的工具"},
		{name: "bad iterations low", node: config.WorkflowNode{ID: "loop", Type: config.WorkflowNodeTypeLoop, Loop: config.WorkflowLoop{Tool: "demo.tool", MaxIterations: 0}}, want: "1..20"},
		{name: "bad iterations high", node: config.WorkflowNode{ID: "loop", Type: config.WorkflowNodeTypeLoop, Loop: config.WorkflowLoop{Tool: "demo.tool", MaxIterations: 21}}, want: "1..20"},
		{name: "node tool forbidden", node: config.WorkflowNode{ID: "loop", Type: config.WorkflowNodeTypeLoop, Tool: "demo.tool", Loop: config.WorkflowLoop{Tool: "demo.tool", MaxIterations: 1}}, want: "不能同时配置 tool"},
		{name: "condition forbidden", node: config.WorkflowNode{ID: "loop", Type: config.WorkflowNodeTypeLoop, Loop: config.WorkflowLoop{Tool: "demo.tool", MaxIterations: 1}, Condition: config.WorkflowCondition{Input: "x"}}, want: "不能配置 condition"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := reg.ValidateWorkflow(&config.WorkflowConfig{ID: "demo.loop", Nodes: []config.WorkflowNode{tc.node}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateWorkflow error = %v, want %s", err, tc.want)
			}
		})
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

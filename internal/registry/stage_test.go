package registry

import (
	"strings"
	"testing"

	"shell_ops/internal/config"
)

func stageTestRegistry() *Registry {
	return &Registry{Tools: map[string]*Tool{"demo.tool": {}}}
}

func TestOrderWorkflowFollowsStageAndOrder(t *testing.T) {
	wf := &config.WorkflowConfig{
		ID: "demo.stages",
		Stages: []config.WorkflowStage{
			{ID: "prepare", Name: "准备"},
			{ID: "change", Name: "变更"},
		},
		Nodes: []config.WorkflowNode{
			{ID: "z-first", Type: config.WorkflowNodeTypeTool, Tool: "demo.tool", StageID: "prepare", Order: 10},
			{ID: "a-second", Type: config.WorkflowNodeTypeTool, Tool: "demo.tool", StageID: "prepare", Order: 20},
			{ID: "b-change", Type: config.WorkflowNodeTypeTool, Tool: "demo.tool", StageID: "change", Order: 10},
		},
	}

	ordered, err := OrderWorkflow(wf)
	if err != nil {
		t.Fatalf("OrderWorkflow error: %v", err)
	}
	got := []string{ordered[0].ID, ordered[1].ID, ordered[2].ID}
	want := []string{"z-first", "a-second", "b-change"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("ordered = %v, want %v", got, want)
		}
	}
}

func TestOrderWorkflowRejectsNodeStageWithoutStages(t *testing.T) {
	wf := &config.WorkflowConfig{Nodes: []config.WorkflowNode{{ID: "first", StageID: "prepare", Order: 10}}}
	if _, err := OrderWorkflow(wf); err == nil || !strings.Contains(err.Error(), "未定义 stages") {
		t.Fatalf("OrderWorkflow error = %v, want 未定义 stages", err)
	}
}

func TestOrderWorkflowRejectsDependencyConflictingWithNodeOrder(t *testing.T) {
	wf := &config.WorkflowConfig{
		ID:     "demo.stages",
		Stages: []config.WorkflowStage{{ID: "prepare", Name: "准备"}},
		Nodes: []config.WorkflowNode{
			{ID: "first", Type: config.WorkflowNodeTypeTool, Tool: "demo.tool", StageID: "prepare", Order: 20},
			{ID: "second", Type: config.WorkflowNodeTypeTool, Tool: "demo.tool", StageID: "prepare", Order: 10},
		},
		Edges: []config.WorkflowEdge{{From: "first", To: "second"}},
	}

	_, err := OrderWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "节点顺序冲突") {
		t.Fatalf("OrderWorkflow error = %v, want 节点顺序冲突", err)
	}
}

func TestValidateWorkflowAcceptsStageWorkflowWithoutEdges(t *testing.T) {
	wf := &config.WorkflowConfig{
		ID: "demo.stages",
		Stages: []config.WorkflowStage{
			{ID: "prepare", Name: "准备", ExecutionMode: config.WorkflowStageExecutionSerial},
			{ID: "verify", Name: "验证", ExecutionMode: config.WorkflowStageExecutionSerial},
		},
		Nodes: []config.WorkflowNode{
			{ID: "upload", Type: config.WorkflowNodeTypeTool, Tool: "demo.tool", StageID: "prepare", Order: 10},
			{ID: "health", Type: config.WorkflowNodeTypeTool, Tool: "demo.tool", StageID: "verify", Order: 10},
		},
	}

	if err := stageTestRegistry().ValidateWorkflow(wf); err != nil {
		t.Fatalf("ValidateWorkflow error: %v", err)
	}
}

func TestValidateWorkflowRejectsInvalidStageLayout(t *testing.T) {
	toolNode := func(id, stage string, order int) config.WorkflowNode {
		return config.WorkflowNode{ID: id, Type: config.WorkflowNodeTypeTool, Tool: "demo.tool", StageID: stage, Order: order}
	}
	cases := []struct {
		name   string
		stages []config.WorkflowStage
		nodes  []config.WorkflowNode
		edges  []config.WorkflowEdge
		want   string
	}{
		{
			name:  "missing stages but node has stage id",
			nodes: []config.WorkflowNode{toolNode("first", "prepare", 10)},
			want:  "未定义 stages",
		},
		{
			name:   "missing stage id",
			stages: []config.WorkflowStage{{ID: "prepare"}},
			nodes:  []config.WorkflowNode{{ID: "first", Type: config.WorkflowNodeTypeTool, Tool: "demo.tool", Order: 10}},
			want:   "缺少 stage_id",
		},
		{
			name:   "unknown stage",
			stages: []config.WorkflowStage{{ID: "prepare"}},
			nodes:  []config.WorkflowNode{toolNode("first", "missing", 10)},
			want:   "不存在的阶段",
		},
		{
			name:   "duplicate order",
			stages: []config.WorkflowStage{{ID: "prepare"}},
			nodes:  []config.WorkflowNode{toolNode("first", "prepare", 10), toolNode("second", "prepare", 10)},
			want:   "order 重复",
		},
		{
			name:   "unknown execution mode",
			stages: []config.WorkflowStage{{ID: "prepare", ExecutionMode: "race"}},
			nodes:  []config.WorkflowNode{toolNode("first", "prepare", 10)},
			want:   "只支持 serial 或 parallel",
		},
		{
			name:   "cyclic dependency within parallel stage",
			stages: []config.WorkflowStage{{ID: "prepare", ExecutionMode: config.WorkflowStageExecutionParallel}},
			nodes:  []config.WorkflowNode{toolNode("first", "prepare", 10), toolNode("second", "prepare", 20)},
			edges:  []config.WorkflowEdge{{From: "first", To: "second"}, {From: "second", To: "first"}},
			want:   "显式依赖存在环",
		},
		{
			name:   "backward dependency within serial stage",
			stages: []config.WorkflowStage{{ID: "prepare"}},
			nodes:  []config.WorkflowNode{toolNode("first", "prepare", 10), toolNode("second", "prepare", 20)},
			edges:  []config.WorkflowEdge{{From: "second", To: "first"}},
			want:   "节点顺序冲突",
		},
		{
			name:   "backward cross stage dependency",
			stages: []config.WorkflowStage{{ID: "prepare"}, {ID: "change"}},
			nodes:  []config.WorkflowNode{toolNode("first", "prepare", 10), toolNode("second", "change", 10)},
			edges:  []config.WorkflowEdge{{From: "second", To: "first"}},
			want:   "与阶段顺序冲突",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wf := &config.WorkflowConfig{ID: "demo.stages", Stages: tc.stages, Nodes: tc.nodes, Edges: tc.edges}
			err := stageTestRegistry().ValidateWorkflow(wf)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateWorkflow error = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestValidateWorkflowAllowsCrossStageConditionBranch(t *testing.T) {
	wf := &config.WorkflowConfig{
		ID:     "demo.stages",
		Stages: []config.WorkflowStage{{ID: "prepare"}, {ID: "change"}},
		Nodes: []config.WorkflowNode{
			{
				ID:      "route",
				Type:    config.WorkflowNodeTypeCondition,
				StageID: "prepare",
				Order:   10,
				Condition: config.WorkflowCondition{
					Input: "x",
					Cases: []config.ConditionCase{{ID: "ok", Name: "正常", Operator: "eq", Values: []string{"x"}}},
				},
			},
			{ID: "apply", Type: config.WorkflowNodeTypeTool, Tool: "demo.tool", StageID: "change", Order: 10},
		},
		Edges: []config.WorkflowEdge{{From: "route", To: "apply", Case: "ok"}},
	}

	if err := stageTestRegistry().ValidateWorkflow(wf); err != nil {
		t.Fatalf("ValidateWorkflow error = %v, want nil", err)
	}
}

func TestValidateWorkflowNotifyNode(t *testing.T) {
	reg := stageTestRegistry()
	ok := &config.WorkflowConfig{ID: "demo.notify", Nodes: []config.WorkflowNode{{
		ID: "alert", Type: config.WorkflowNodeTypeNotify,
		Notify: config.WorkflowNotify{Channel: config.NotifyChannelWebhook, URL: "https://example.com/hook", Message: "done"},
	}}}
	if err := reg.ValidateWorkflow(ok); err != nil {
		t.Fatalf("合法通知节点被拒绝: %v", err)
	}
	if got := effectiveNodeType(ok.Nodes[0]); got != config.WorkflowNodeTypeNotify {
		t.Fatalf("effectiveNodeType = %q", got)
	}
	missingURL := &config.WorkflowConfig{ID: "demo.notify", Nodes: []config.WorkflowNode{{
		ID: "alert", Type: config.WorkflowNodeTypeNotify, Notify: config.WorkflowNotify{Message: "done"},
	}}}
	if err := reg.ValidateWorkflow(missingURL); err == nil || !strings.Contains(err.Error(), "Webhook 地址必填") {
		t.Fatalf("缺少地址应报错: %v", err)
	}
	withLoop := &config.WorkflowConfig{ID: "demo.notify", Nodes: []config.WorkflowNode{{
		ID: "alert", Type: config.WorkflowNodeTypeNotify, Loop: config.WorkflowLoop{Tool: "demo.tool", MaxIterations: 2},
		Notify: config.WorkflowNotify{URL: "https://example.com/hook"},
	}}}
	if err := reg.ValidateWorkflow(withLoop); err == nil {
		t.Fatal("通知节点不应允许配置 loop")
	}
}

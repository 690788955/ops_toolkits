package config

import "testing"

func TestNormalizeWorkflowStageWorkflowKeepsEdgesEmpty(t *testing.T) {
	wf := &WorkflowConfig{
		ID: "demo.stages",
		Stages: []WorkflowStage{
			{ID: " prepare ", Name: " 准备 "},
			{ID: "verify"},
		},
		Nodes: []WorkflowNode{
			{ID: "upload", Type: WorkflowNodeTypeUpload, StageID: " prepare ", Order: 10, Upload: WorkflowUpload{TargetDir: "assets"}},
			{ID: "health", Type: WorkflowNodeTypeTool, Tool: "demo.health", StageID: "verify", Order: 10},
		},
	}

	NormalizeWorkflow(wf)

	if len(wf.Edges) != 0 {
		t.Fatalf("edges = %#v, want empty", wf.Edges)
	}
	if wf.Stages[0].ID != "prepare" || wf.Stages[0].Name != "准备" {
		t.Fatalf("stage = %#v, want trimmed id and name", wf.Stages[0])
	}
	if wf.Stages[0].ExecutionMode != WorkflowStageExecutionSerial {
		t.Fatalf("execution_mode = %q, want serial", wf.Stages[0].ExecutionMode)
	}
	if wf.Stages[1].Name != "verify" {
		t.Fatalf("stage name = %q, want verify", wf.Stages[1].Name)
	}
	if wf.Nodes[0].StageID != "prepare" {
		t.Fatalf("node stage_id = %q, want prepare", wf.Nodes[0].StageID)
	}
}

func TestNormalizeWorkflowStageWorkflowKeepsDeclaredDependencies(t *testing.T) {
	wf := &WorkflowConfig{
		ID:     "demo.stages",
		Stages: []WorkflowStage{{ID: "prepare"}, {ID: "verify"}},
		Nodes: []WorkflowNode{
			{ID: "upload", Type: WorkflowNodeTypeTool, Tool: "demo.upload", StageID: "prepare", Order: 10},
			{ID: "health", Type: WorkflowNodeTypeTool, Tool: "demo.health", StageID: "verify", Order: 10, DependsOn: []string{"upload"}},
		},
	}

	NormalizeWorkflow(wf)

	if len(wf.Edges) != 1 || wf.Edges[0].From != "upload" || wf.Edges[0].To != "health" {
		t.Fatalf("edges = %#v, want upload -> health", wf.Edges)
	}
}

func TestNormalizeWorkflowLegacyStillBuildsSequentialEdges(t *testing.T) {
	wf := &WorkflowConfig{
		ID: "demo.legacy",
		Nodes: []WorkflowNode{
			{ID: "first", Type: WorkflowNodeTypeTool, Tool: "demo.first"},
			{ID: "second", Type: WorkflowNodeTypeTool, Tool: "demo.second"},
		},
	}

	NormalizeWorkflow(wf)

	if len(wf.Edges) != 1 || wf.Edges[0].From != "first" || wf.Edges[0].To != "second" {
		t.Fatalf("edges = %#v, want first -> second", wf.Edges)
	}
}

func TestNormalizeWorkflowLegacyFoldsIntoSingleSerialStage(t *testing.T) {
	wf := &WorkflowConfig{
		ID: "demo.legacy",
		Nodes: []WorkflowNode{
			{ID: "first", Type: WorkflowNodeTypeTool, Tool: "demo.first"},
			{ID: "second", Type: WorkflowNodeTypeTool, Tool: "demo.second"},
		},
	}

	NormalizeWorkflow(wf)

	if len(wf.Stages) != 1 {
		t.Fatalf("stages = %#v, want one synthesized stage", wf.Stages)
	}
	if wf.Stages[0].ID != LegacyWorkflowStageID || wf.Stages[0].Name != LegacyWorkflowStageName {
		t.Fatalf("stage = %#v, want legacy stage", wf.Stages[0])
	}
	if wf.Stages[0].ExecutionMode != WorkflowStageExecutionSerial {
		t.Fatalf("execution_mode = %q, want serial", wf.Stages[0].ExecutionMode)
	}
	for index, node := range wf.Nodes {
		if node.StageID != LegacyWorkflowStageID {
			t.Fatalf("node %s stage_id = %q, want %q", node.ID, node.StageID, LegacyWorkflowStageID)
		}
		if node.Order != index+1 {
			t.Fatalf("node %s order = %d, want %d", node.ID, node.Order, index+1)
		}
	}
}

func TestNormalizeWorkflowLegacyOrdersNodesByDependency(t *testing.T) {
	wf := &WorkflowConfig{
		ID: "demo.legacy",
		Nodes: []WorkflowNode{
			{ID: "target", Type: WorkflowNodeTypeTool, Tool: "demo.target"},
			{ID: "left", Type: WorkflowNodeTypeTool, Tool: "demo.left"},
			{ID: "right", Type: WorkflowNodeTypeTool, Tool: "demo.right"},
		},
		Edges: []WorkflowEdge{
			{From: "left", To: "target"},
			{From: "right", To: "target"},
		},
	}

	NormalizeWorkflow(wf)

	order := map[string]int{}
	for _, node := range wf.Nodes {
		order[node.ID] = node.Order
	}
	if order["left"] >= order["target"] || order["right"] >= order["target"] {
		t.Fatalf("orders = %#v, want both branches before target", order)
	}
}

func TestNormalizeWorkflowKeepsDeclaredStages(t *testing.T) {
	wf := &WorkflowConfig{
		ID:     "demo.stages",
		Stages: []WorkflowStage{{ID: "prepare", Name: "准备"}},
		Nodes:  []WorkflowNode{{ID: "upload", Type: WorkflowNodeTypeUpload, StageID: "prepare", Order: 3, Upload: WorkflowUpload{TargetDir: "assets"}}},
	}

	NormalizeWorkflow(wf)

	if len(wf.Stages) != 1 || wf.Stages[0].ID != "prepare" {
		t.Fatalf("stages = %#v, want declared stage kept", wf.Stages)
	}
	if wf.Nodes[0].StageID != "prepare" || wf.Nodes[0].Order != 3 {
		t.Fatalf("node = %#v, want declared stage_id/order kept", wf.Nodes[0])
	}
}

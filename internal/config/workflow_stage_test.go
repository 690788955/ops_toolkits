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

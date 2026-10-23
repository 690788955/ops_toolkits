package runner

import (
	"context"
	"sort"
	"testing"

	"shell_ops/internal/config"
)

func shellNode(id, stage string, order int, script string) config.WorkflowNode {
	return config.WorkflowNode{
		ID:      id,
		Type:    config.WorkflowNodeTypeShell,
		StageID: stage,
		Order:   order,
		Script:  config.WorkflowScript{Script: script},
	}
}

func conditionNode(id, stage string, order int) config.WorkflowNode {
	return config.WorkflowNode{
		ID:      id,
		Type:    config.WorkflowNodeTypeCondition,
		StageID: stage,
		Order:   order,
		Condition: config.WorkflowCondition{
			Input: "{{ .mode }}",
			Cases: []config.ConditionCase{{ID: "fast", Operator: "eq", Values: []string{"fast"}}},
		},
	}
}

func stepStatusByID(record *RunRecord) map[string]string {
	out := map[string]string{}
	for _, step := range record.Steps {
		out[step.ID] = step.Status
	}
	return out
}

func TestRunWorkflowCrossStageConditionSkipsUnmatchedBranch(t *testing.T) {
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID:     "demo.cross-condition",
		Stages: []config.WorkflowStage{{ID: "prepare"}, {ID: "change"}},
		Nodes: []config.WorkflowNode{
			shellNode("prepare", "prepare", 10, "echo prepare"),
			conditionNode("route", "prepare", 20),
			shellNode("fast", "change", 10, "echo fast"),
			shellNode("slow", "change", 20, "echo slow"),
			shellNode("verify", "change", 30, "echo verify"),
		},
		Edges: []config.WorkflowEdge{
			{From: "prepare", To: "route"},
			{From: "route", To: "fast", Case: "fast"},
			{From: "route", To: "slow", Case: "default"},
		},
	}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, map[string]string{"mode": "fast"}, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	statuses := stepStatusByID(record)
	if statuses["fast"] != "succeeded" || statuses["prepare"] != "succeeded" || statuses["route"] != "succeeded" {
		t.Fatalf("activated branch statuses = %v", statuses)
	}
	// verify 只依赖 prepare，不依赖被跳过的分支，串行阶段里仍会执行。
	if statuses["slow"] != "skipped" || statuses["verify"] != "succeeded" {
		t.Fatalf("skipped branch statuses = %v, want slow/verify skipped", statuses)
	}
	for _, step := range record.Steps {
		if step.Status == "skipped" && step.SkippedReason != "条件分支未激活" {
			t.Fatalf("step %s skipped reason = %q", step.ID, step.SkippedReason)
		}
	}
}

func TestRunWorkflowParallelConditionReleasesSkippedBranch(t *testing.T) {
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID:     "demo.parallel-condition",
		Stages: []config.WorkflowStage{{ID: "change", ExecutionMode: config.WorkflowStageExecutionParallel, MaxConcurrency: 1}},
		Nodes: []config.WorkflowNode{
			conditionNode("route", "change", 10),
			shellNode("fast", "change", 20, "echo fast"),
			shellNode("slow", "change", 30, "echo slow"),
			shellNode("fast-follow", "change", 40, "echo fast-follow"),
			shellNode("slow-follow", "change", 50, "echo slow-follow"),
			shellNode("after", "change", 60, "echo after"),
		},
		Edges: []config.WorkflowEdge{
			{From: "route", To: "fast", Case: "fast"},
			{From: "route", To: "slow", Case: "default"},
			{From: "fast", To: "fast-follow"},
			{From: "slow", To: "slow-follow"},
			{From: "fast-follow", To: "after"},
			{From: "slow-follow", To: "after"},
		},
	}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, map[string]string{"mode": "slow"}, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	statuses := stepStatusByID(record)
	if statuses["after"] != "skipped" {
		t.Fatalf("after status = %q, want skipped (one branch never ran)", statuses["after"])
	}
	if statuses["fast"] != "skipped" || statuses["fast-follow"] != "skipped" {
		t.Fatalf("unmatched branch statuses = %v", statuses)
	}
	if len(record.Steps) != 6 {
		ids := make([]string, 0, len(record.Steps))
		for _, step := range record.Steps {
			ids = append(ids, step.ID)
		}
		sort.Strings(ids)
		t.Fatalf("steps = %v, want every node recorded exactly once", ids)
	}
}

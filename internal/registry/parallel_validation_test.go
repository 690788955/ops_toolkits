package registry

import (
	"strings"
	"testing"

	"shell_ops/internal/config"
)

func TestNormalizeDoesNotHideInvalidStageConcurrency(t *testing.T) {
	wf := &config.WorkflowConfig{
		ID:     "invalid.concurrency",
		Stages: []config.WorkflowStage{{ID: "parallel", ExecutionMode: "parallel", MaxConcurrency: -1}},
		Nodes:  []config.WorkflowNode{{ID: "task", Type: "tool", Tool: "demo.tool", StageID: "parallel", Order: 10}},
	}
	config.NormalizeWorkflow(wf)
	if err := stageTestRegistry().ValidateWorkflow(wf); err == nil || !strings.Contains(err.Error(), "max_concurrency") {
		t.Fatalf("归一化掩盖了非法并发上限: %v", err)
	}
}

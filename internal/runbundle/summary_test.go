package runbundle

import (
	"testing"
	"time"

	"shell_ops/internal/runner"
)

func TestSummarizeMarksAwaitingApproval(t *testing.T) {
	since := time.Now().Add(-time.Minute)
	record := runner.RunRecord{
		ID:     "workflow-approval",
		Kind:   "workflow",
		Target: "demo.approval",
		Status: "running",
		Approvals: []runner.ApprovalRecord{
			{NodeID: "earlier", Status: runner.ApprovalStatusApproved},
			{NodeID: "gate", Message: "改好配置后继续", Status: runner.ApprovalStatusWaiting, Since: since},
		},
	}
	summary := Summarize(record)
	if !summary.AwaitingApproval {
		t.Fatalf("summary = %#v, want awaiting_approval", summary)
	}
	if summary.Approval == nil || summary.Approval.NodeID != "gate" || summary.Approval.Message != "改好配置后继续" || !summary.Approval.Since.Equal(since) {
		t.Fatalf("approval = %#v", summary.Approval)
	}

	// 审批已通过或运行结束后，列表不应继续提示“卡在人工确认”。
	record.Approvals[1].Status = runner.ApprovalStatusApproved
	record.Approvals[1].DecidedAt = time.Now()
	record.Status = "succeeded"
	done := Summarize(record)
	if done.AwaitingApproval || done.Approval != nil {
		t.Fatalf("finished summary = %#v, want no awaiting approval", done)
	}
}
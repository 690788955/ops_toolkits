package runner

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shell_ops/internal/config"
)

func TestRequestApprovalBlocksUntilDecision(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type result struct {
		decision approvalDecision
		err      error
	}
	done := make(chan result, 1)
	go func() {
		decision, err := RequestApproval(ctx, "run-approve", "gate", 0)
		done <- result{decision: decision, err: err}
	}()
	waitForGate(t, "run-approve", "gate")
	if err := ApproveWorkflowNode("run-approve", "gate", " 配置已改好 ", "alice"); err != nil {
		t.Fatalf("ApproveWorkflowNode error: %v", err)
	}
	got := <-done
	if got.err != nil || !got.decision.Approved {
		t.Fatalf("decision = %#v err = %v, want approved", got.decision, got.err)
	}
	if got.decision.Note != " 配置已改好 " {
		t.Fatalf("note = %q, want raw note preserved", got.decision.Note)
	}
	if _, ok := PendingApproval("run-approve"); ok {
		t.Fatalf("gate should be released after decision")
	}
}

func TestRejectWorkflowNodeOnlyOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := RequestApproval(ctx, "run-reject", "gate", 0)
		done <- err
	}()
	waitForGate(t, "run-reject", "gate")
	if err := RejectWorkflowNode("run-reject", "gate", "配置写错了", ""); err != nil {
		t.Fatalf("RejectWorkflowNode error: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("RequestApproval error = %v, want nil (驳回交给调用方判定)", err)
	}
	if err := RejectWorkflowNode("run-reject", "gate", "", ""); err == nil {
		t.Fatalf("second reject should fail after gate closed")
	}
}

func TestRequestApprovalTimesOut(t *testing.T) {
	_, err := RequestApproval(context.Background(), "run-timeout", "gate", 30*time.Millisecond)
	if !errors.Is(err, errApprovalTimeout) {
		t.Fatalf("err = %v, want errApprovalTimeout", err)
	}
	if _, ok := PendingApproval("run-timeout"); ok {
		t.Fatalf("gate should be released after timeout")
	}
}

func TestRequestApprovalHonorsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := RequestApproval(ctx, "run-cancel", "gate", 0)
		done <- err
	}()
	waitForGate(t, "run-cancel", "gate")
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestApproveWorkflowNodeWithoutGateFails(t *testing.T) {
	err := ApproveWorkflowNode("run-missing", "gate", "", "")
	if err == nil || !strings.Contains(err.Error(), "不在等待人工审批") {
		t.Fatalf("err = %v, want 不在等待人工审批", err)
	}
}

func TestRunWorkflowApprovalNodeBlocksDownstreamUntilApproved(t *testing.T) {
	r, _ := newHTTPTestRunner(t)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	wf := &config.WorkflowConfig{
		ID:     "demo.approval",
		Stages: []config.WorkflowStage{{ID: "change"}},
		Nodes: []config.WorkflowNode{
			{ID: "gate", Type: config.WorkflowNodeTypeApproval, StageID: "change", Order: 10, Approval: config.WorkflowApproval{Message: "改好配置后继续 {{ .env }}"}},
			{ID: "deploy", Type: config.WorkflowNodeTypeHTTP, StageID: "change", Order: 20, HTTP: config.WorkflowHTTP{URL: server.URL}},
		},
	}
	record, err := r.StartWorkflowConfigWithConfirmation(context.Background(), wf, map[string]string{"env": "prod"}, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("StartWorkflowConfigWithConfirmation error: %v", err)
	}
	runDir := filepath.Join(r.RunsDir, record.ID)
	waiting := waitForRunnerStepStatus(t, runDir, "gate", "waiting")
	if len(waiting.Approvals) != 1 || waiting.Approvals[0].Status != ApprovalStatusWaiting {
		t.Fatalf("approvals = %#v, want one waiting entry", waiting.Approvals)
	}
	if got := waiting.Approvals[0].Message; got != "改好配置后继续 prod" {
		t.Fatalf("approval message = %q, want rendered template", got)
	}
	if waiting.Status != "running" {
		t.Fatalf("run status = %s, want running while waiting", waiting.Status)
	}
	// 审批未通过前下游不能执行，否则停机点就没有意义。
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("downstream node ran before approval")
	}
	if err := ApproveWorkflowNode(record.ID, "gate", "配置已修改", "tester"); err != nil {
		t.Fatalf("ApproveWorkflowNode error: %v", err)
	}
	final := waitForRunnerRecordStatus(t, runDir, "succeeded")
	if len(final.Steps) != 2 || final.Steps[0].Status != "succeeded" || final.Steps[1].Status != "succeeded" {
		t.Fatalf("steps = %#v, want both succeeded", final.Steps)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("downstream calls = %d, want 1 after approval", calls)
	}
	approval := final.Approvals[0]
	if approval.Status != ApprovalStatusApproved || approval.Note != "配置已修改" || approval.Reviewer != "tester" {
		t.Fatalf("approval = %#v, want approved with note and reviewer", approval)
	}
	meta := readApprovalMeta(filepath.Join(runDir, "gate"))
	if meta.Status != ApprovalStatusApproved {
		t.Fatalf("approval.json = %#v", meta)
	}
	stdout := readFile(t, filepath.Join(runDir, "gate", "stdout.log"))
	if !strings.Contains(stdout, "等待人工确认") || !strings.Contains(stdout, "审批通过") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunWorkflowApprovalRejectedStopsDownstream(t *testing.T) {
	r, _ := newHTTPTestRunner(t)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	wf := &config.WorkflowConfig{
		ID:     "demo.approval-reject",
		Stages: []config.WorkflowStage{{ID: "change"}},
		Nodes: []config.WorkflowNode{
			{ID: "gate", Type: config.WorkflowNodeTypeApproval, StageID: "change", Order: 10, Approval: config.WorkflowApproval{Message: "确认配置"}},
			{ID: "deploy", Type: config.WorkflowNodeTypeHTTP, StageID: "change", Order: 20, HTTP: config.WorkflowHTTP{URL: server.URL}},
		},
	}
	record, err := r.StartWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("StartWorkflowConfigWithConfirmation error: %v", err)
	}
	runDir := filepath.Join(r.RunsDir, record.ID)
	waitForRunnerStepStatus(t, runDir, "gate", "waiting")
	if err := RejectWorkflowNode(record.ID, "gate", "配置还没改完", "tester"); err != nil {
		t.Fatalf("RejectWorkflowNode error: %v", err)
	}
	final := waitForRunnerRecordStatus(t, runDir, "failed")
	if len(final.Steps) != 1 || final.Steps[0].Status != "failed" {
		t.Fatalf("steps = %#v, want only the approval node failed", final.Steps)
	}
	if !strings.Contains(final.Error, "驳回") || !strings.Contains(final.Error, "配置还没改完") {
		t.Fatalf("error = %q, want 驳回 with note", final.Error)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("downstream ran despite rejection")
	}
	if final.Approvals[0].Status != ApprovalStatusRejected {
		t.Fatalf("approval = %#v, want rejected", final.Approvals)
	}
}

func TestRunWorkflowApprovalTimeoutFails(t *testing.T) {
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID:     "demo.approval-timeout",
		Stages: []config.WorkflowStage{{ID: "change"}},
		Nodes: []config.WorkflowNode{{
			ID: "gate", Type: config.WorkflowNodeTypeApproval, StageID: "change", Order: 10,
			Approval: config.WorkflowApproval{Message: "确认", Timeout: "40ms"},
		}},
	}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err == nil || record.Status != "failed" {
		t.Fatalf("err = %v status = %s, want failure", err, record.Status)
	}
	if !strings.Contains(err.Error(), "等待超时") {
		t.Fatalf("err = %v, want 等待超时", err)
	}
	if record.Approvals[0].Status != ApprovalStatusTimedOut {
		t.Fatalf("approval = %#v, want timed_out", record.Approvals)
	}
}

func TestRunWorkflowApprovalCancelKeepsRunCancelled(t *testing.T) {
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID:     "demo.approval-cancel",
		Stages: []config.WorkflowStage{{ID: "change"}},
		Nodes: []config.WorkflowNode{{
			ID: "gate", Type: config.WorkflowNodeTypeApproval, StageID: "change", Order: 10,
			Approval: config.WorkflowApproval{Message: "确认"},
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	record, err := r.StartWorkflowConfigWithConfirmation(ctx, wf, nil, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("StartWorkflowConfigWithConfirmation error: %v", err)
	}
	runDir := filepath.Join(r.RunsDir, record.ID)
	waitForRunnerStepStatus(t, runDir, "gate", "waiting")
	cancel()
	final := waitForRunnerRecordStatus(t, runDir, "cancelled")
	if len(final.Approvals) != 1 || final.Approvals[0].Status != ApprovalStatusCancelled {
		t.Fatalf("approvals = %#v, want cancelled", final.Approvals)
	}
}

// waitForGate 等节点登记闸门后再投递决定，避免测试依赖调度时序。
func waitForGate(t *testing.T, runID, nodeID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if node, ok := PendingApproval(runID); ok && node == nodeID {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("approval gate %s/%s was not registered", runID, nodeID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestApprovalGateRejectsDuplicateWaiters(t *testing.T) {
	first, err := openApprovalGate(context.Background(), "run-dup", "gate", 0)
	if err != nil {
		t.Fatalf("openApprovalGate error: %v", err)
	}
	defer closeApprovalGate("run-dup", "gate", first)
	if _, err := openApprovalGate(context.Background(), "run-dup", "gate", 0); err == nil {
		t.Fatalf("duplicate gate should be rejected")
	}
}

func TestApprovalDecisionCanBeSubmittedOnlyOnceBeforeWaiterConsumesIt(t *testing.T) {
	gate, err := openApprovalGate(context.Background(), "run-race", "gate", 0)
	if err != nil {
		t.Fatalf("openApprovalGate error: %v", err)
	}
	defer closeApprovalGate("run-race", "gate", gate)

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- ApproveWorkflowNode("run-race", "gate", "", "")
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	failures := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("decision results = %d successes, %d failures; want exactly one each", successes, failures)
	}
}

func TestApprovalDecisionRejectsCancelledGateBeforeWaiterCleansUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gate, err := openApprovalGate(ctx, "run-late-cancel", "gate", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer closeApprovalGate("run-late-cancel", "gate", gate)
	cancel()
	if err := ApproveWorkflowNode("run-late-cancel", "gate", "", ""); err == nil {
		t.Fatal("approval after cancellation should be rejected before cleanup")
	}
	if _, err := waitApprovalDecision(gate); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v, want context.Canceled", err)
	}
}

func TestApprovalDecisionRejectsExpiredGateBeforeWaiterCleansUp(t *testing.T) {
	gate, err := openApprovalGate(context.Background(), "run-late-timeout", "gate", time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	defer closeApprovalGate("run-late-timeout", "gate", gate)
	gate.deadline = time.Now().Add(-time.Second)
	if err := ApproveWorkflowNode("run-late-timeout", "gate", "", ""); err == nil {
		t.Fatal("approval after timeout should be rejected before cleanup")
	}
	if _, err := waitApprovalDecision(gate); !errors.Is(err, errApprovalTimeout) {
		t.Fatalf("wait error = %v, want errApprovalTimeout", err)
	}
}

type approvalReadyWriter struct {
	runID string
	err   chan error
}

func (w *approvalReadyWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "审批节点 gate 等待人工确认") {
		w.err <- ApproveWorkflowNode(w.runID, "gate", "ready", "tester")
	}
	return len(p), nil
}

func TestApprovalIsActionableWhenWaitingIsPublished(t *testing.T) {
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID: "demo.approval-ready",
		Nodes: []config.WorkflowNode{{
			ID: "gate", Type: config.WorkflowNodeTypeApproval,
			Approval: config.WorkflowApproval{Message: "确认", Timeout: "1s"},
		}},
	}
	record, params, runDir, err := r.prepareWorkflowRun(wf, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := &approvalReadyWriter{runID: record.ID, err: make(chan error, 1)}
	final, err := r.runWorkflowPrepared(context.Background(), wf, params, false, nil, out, nilWriter{}, record, runDir)
	if err != nil {
		t.Fatalf("run error = %v", err)
	}
	select {
	case approvalErr := <-out.err:
		if approvalErr != nil {
			t.Fatalf("first approval after waiting was published failed: %v", approvalErr)
		}
	default:
		t.Fatal("approval waiting state was not published")
	}
	if final.Status != "succeeded" || len(final.Approvals) != 1 || final.Approvals[0].Status != ApprovalStatusApproved {
		t.Fatalf("final = %#v, want successful approved run", final)
	}
}

func TestApprovalMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	item := ApprovalRecord{NodeID: "gate", Message: "确认", Status: ApprovalStatusApproved, Since: time.Now(), DecidedAt: time.Now(), Note: "ok"}
	writeApprovalMeta(dir, item)
	got := readApprovalMeta(dir)
	if got.NodeID != item.NodeID || got.Status != item.Status || got.Note != "ok" {
		t.Fatalf("approval meta = %#v", got)
	}
	workflowContext := map[string]string{}
	addApprovalContext(workflowContext, "gate", item)
	if workflowContext["steps.gate.approval.status"] != ApprovalStatusApproved || workflowContext["steps.gate.approval.note"] != "ok" {
		t.Fatalf("context = %#v", workflowContext)
	}
	if _, err := os.Stat(filepath.Join(dir, approvalMetaFile)); err != nil {
		t.Fatalf("approval.json missing: %v", err)
	}
}

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"shell_ops/internal/config"
)

// 人工审批是运行期闸门：节点执行到这里不结束运行，而是让整条流水线停在 running，
// 等平台上的「确认继续」或「驳回」决定后才放开下游。等待期间用户可以改配置文件，
// 因此这里只记证据、不自动放行，也不把等待状态混进失败恢复链路（pending 专用于失败）。
const (
	ApprovalStatusWaiting   = "waiting"
	ApprovalStatusApproved  = "approved"
	ApprovalStatusRejected  = "rejected"
	ApprovalStatusTimedOut  = "timed_out"
	ApprovalStatusCancelled = "cancelled"

	approvalMetaFile = "approval.json"
)

// errApprovalTimeout 区分“没人来点确认”和“运行被取消”，两者在运行记录里语义不同。
var errApprovalTimeout = errors.New("审批等待超时")

// ApprovalRecord 随运行记录持久化一次审批的完整生命周期，重启后仍能解释这条运行为什么停过。
type ApprovalRecord struct {
	NodeID    string    `json:"node_id"`
	Message   string    `json:"message,omitempty"`
	Status    string    `json:"status"`
	Since     time.Time `json:"since"`
	DecidedAt time.Time `json:"decided_at,omitempty"`
	Note      string    `json:"note,omitempty"`
	Reviewer  string    `json:"reviewer,omitempty"`
}

// approvalDecision 是闸门的唯一输出：是否放行，以及人工填写的备注。
type approvalDecision struct {
	Approved bool
	Note     string
	Reviewer string
}

type approvalGate struct {
	ch       chan approvalDecision
	ctx      context.Context
	deadline time.Time
}

// approvalGates 是进程内的等待表。键由运行 ID 与节点 ID 组成：同一工作流的多次运行互不干扰，
// 重跑同一节点时会因为旧闸门已释放而重新登记，不会串给上一次运行。
var approvalGates = struct {
	mu    sync.Mutex
	gates map[string]*approvalGate
}{gates: map[string]*approvalGate{}}

func approvalKey(runID, nodeID string) string {
	return runID + "|" + nodeID
}

func openApprovalGate(ctx context.Context, runID, nodeID string, timeout time.Duration) (*approvalGate, error) {
	key := approvalKey(runID, nodeID)
	approvalGates.mu.Lock()
	defer approvalGates.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if approvalGates.gates == nil {
		approvalGates.gates = map[string]*approvalGate{}
	}
	if _, ok := approvalGates.gates[key]; ok {
		return nil, fmt.Errorf("运行 %s 的节点 %s 已有待确认的审批", runID, nodeID)
	}
	gate := &approvalGate{ch: make(chan approvalDecision, 1), ctx: ctx}
	if timeout > 0 {
		gate.deadline = time.Now().Add(timeout)
	}
	approvalGates.gates[key] = gate
	return gate, nil
}

func closeApprovalGate(runID, nodeID string, gate *approvalGate) {
	key := approvalKey(runID, nodeID)
	approvalGates.mu.Lock()
	defer approvalGates.mu.Unlock()
	if current, ok := approvalGates.gates[key]; ok && current == gate {
		delete(approvalGates.gates, key)
	}
}

// RequestApproval 登记闸门并阻塞，直到人工决定、运行取消或等待超时，三者都只结算一次。
func RequestApproval(ctx context.Context, runID, nodeID string, timeout time.Duration) (approvalDecision, error) {
	gate, err := openApprovalGate(ctx, runID, nodeID, timeout)
	if err != nil {
		return approvalDecision{}, err
	}
	defer closeApprovalGate(runID, nodeID, gate)
	return waitApprovalDecision(gate)
}

func waitApprovalDecision(gate *approvalGate) (approvalDecision, error) {
	// 取消优先于超时：运行被取消时不应报“等待超时”，否则运行记录会误导排查方向。
	select {
	case <-gate.ctx.Done():
		return approvalDecision{}, gate.ctx.Err()
	default:
	}
	var timeoutC <-chan time.Time
	if !gate.deadline.IsZero() {
		timer := time.NewTimer(time.Until(gate.deadline))
		defer timer.Stop()
		timeoutC = timer.C
	}
	select {
	case decision := <-gate.ch:
		return decision, nil
	case <-gate.ctx.Done():
		return approvalDecision{}, gate.ctx.Err()
	case <-timeoutC:
		return approvalDecision{}, errApprovalTimeout
	}
}

// ApproveWorkflowNode 由平台把「确认继续」投递给正在等待的节点。
func ApproveWorkflowNode(runID, nodeID, note, reviewer string) error {
	return deliverApprovalDecision(runID, nodeID, approvalDecision{Approved: true, Note: note, Reviewer: reviewer})
}

// RejectWorkflowNode 由平台把「驳回」投递给正在等待的节点，节点随即失败并停止下游。
func RejectWorkflowNode(runID, nodeID, note, reviewer string) error {
	return deliverApprovalDecision(runID, nodeID, approvalDecision{Approved: false, Note: note, Reviewer: reviewer})
}

func deliverApprovalDecision(runID, nodeID string, decision approvalDecision) error {
	runID = strings.TrimSpace(runID)
	nodeID = strings.TrimSpace(nodeID)
	if runID == "" || nodeID == "" {
		return fmt.Errorf("运行 ID 与节点 ID 不能为空")
	}
	key := approvalKey(runID, nodeID)
	approvalGates.mu.Lock()
	gate := approvalGates.gates[key]
	if gate != nil && (gate.ctx.Err() != nil || !gate.deadline.IsZero() && !time.Now().Before(gate.deadline)) {
		delete(approvalGates.gates, key)
		gate = nil
	}
	if gate != nil {
		// Claim the gate before releasing the mutex. Otherwise the waiting
		// goroutine can consume the first decision and a second HTTP request
		// can still enqueue another decision before its deferred cleanup runs.
		delete(approvalGates.gates, key)
	}
	approvalGates.mu.Unlock()
	if gate == nil {
		return fmt.Errorf("运行 %s 的节点 %s 当前不在等待人工审批", runID, nodeID)
	}
	// 缓冲为 1，重复点击只会被闸门取出一次；这里仍用非阻塞写避免连点卡住 HTTP 请求。
	select {
	case gate.ch <- decision:
		return nil
	default:
		return fmt.Errorf("运行 %s 的节点 %s 的审批决定已提交，请刷新查看结果", runID, nodeID)
	}
}

// PendingApproval 返回指定运行的等待节点，用于让平台侧给出更准确的错误提示。
func PendingApproval(runID string) (nodeID string, ok bool) {
	prefix := strings.TrimSpace(runID) + "|"
	approvalGates.mu.Lock()
	defer approvalGates.mu.Unlock()
	for key := range approvalGates.gates {
		if strings.HasPrefix(key, prefix) {
			return strings.TrimPrefix(key, prefix), true
		}
	}
	return "", false
}

// WaitingApproval 返回运行记录里最后一条仍在等待的审批。
// 平台列表只据此判断“这条运行卡在人身上”，历史通过/驳回的条目不算。
func WaitingApproval(record RunRecord) (ApprovalRecord, bool) {
	for index := len(record.Approvals) - 1; index >= 0; index-- {
		if record.Approvals[index].Status == ApprovalStatusWaiting {
			return record.Approvals[index], true
		}
	}
	return ApprovalRecord{}, false
}

// beginApproval 先把“等待中”写进运行记录：前端轮询和事件流据此立刻展示审批条，
// 即使进程随后崩溃，也能从运行记录看出这条运行停在哪一步等人。
func (e *workflowExec) beginApproval(node config.WorkflowNode, message string) (int, ApprovalRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	record := ApprovalRecord{NodeID: node.ID, Message: message, Status: ApprovalStatusWaiting, Since: time.Now()}
	e.record.Approvals = append(e.record.Approvals, record)
	_ = e.saveLocked()
	return len(e.record.Approvals) - 1, record
}

// finishApproval 只改写状态与决定信息，保留开始时间以便回溯到底等了多久。
func (e *workflowExec) finishApproval(index int, status, note, reviewer string) ApprovalRecord {
	e.mu.Lock()
	defer e.mu.Unlock()
	if index < 0 || index >= len(e.record.Approvals) {
		return ApprovalRecord{}
	}
	item := e.record.Approvals[index]
	item.Status = status
	item.DecidedAt = time.Now()
	item.Note = strings.TrimSpace(note)
	item.Reviewer = strings.TrimSpace(reviewer)
	e.record.Approvals[index] = item
	_ = e.saveLocked()
	return item
}

// runApprovalNode 是审批节点执行器：登记等待、写证据、阻塞，再把结果落回运行记录。
func (e *workflowExec) runApprovalNode(ctx context.Context, node config.WorkflowNode, message string, timeout time.Duration, stepRunDir string, out io.Writer) (ApprovalRecord, error) {
	// Register the gate before publishing waiting state so the first click can succeed.
	gate, err := openApprovalGate(ctx, e.record.ID, node.ID, timeout)
	if err != nil {
		return ApprovalRecord{}, err
	}
	defer closeApprovalGate(e.record.ID, node.ID, gate)
	index, pending := e.beginApproval(node, message)
	header := fmt.Sprintf("审批节点 %s 等待人工确认", node.ID)
	if pending.Message != "" {
		header += "：" + pending.Message
	}
	appendApprovalLog(stepRunDir, header)
	if out != nil {
		fmt.Fprintln(out, header)
		fmt.Fprintln(out, "该节点需要在平台「运行日志」面板点击确认或驳回；只跑 CLI 时可用 Ctrl+C 取消运行")
	}
	if timeout > 0 {
		if out != nil {
			fmt.Fprintf(out, "等待上限 %s；运行取消会立即结束等待\n", timeout)
		}
		appendApprovalLog(stepRunDir, fmt.Sprintf("等待上限 %s", timeout))
	}

	decision, waitErr := waitApprovalDecision(gate)
	var item ApprovalRecord
	switch {
	case waitErr == nil && decision.Approved:
		item = e.finishApproval(index, ApprovalStatusApproved, decision.Note, decision.Reviewer)
		appendApprovalLog(stepRunDir, "审批通过"+approvalNoteSuffix(item.Note))
		if out != nil {
			fmt.Fprintln(out, "审批通过，继续执行下游节点")
		}
		writeApprovalMeta(stepRunDir, item)
		return item, nil
	case waitErr == nil:
		item = e.finishApproval(index, ApprovalStatusRejected, decision.Note, decision.Reviewer)
		appendApprovalLog(stepRunDir, "审批被驳回"+approvalNoteSuffix(item.Note))
		writeApprovalMeta(stepRunDir, item)
		return item, fmt.Errorf("审批节点 %s 被驳回%s", node.ID, approvalNoteSuffix(item.Note))
	case errors.Is(waitErr, errApprovalTimeout):
		item = e.finishApproval(index, ApprovalStatusTimedOut, "", "")
		appendApprovalLog(stepRunDir, fmt.Sprintf("等待超时（%s），未收到人工确认", timeout))
		writeApprovalMeta(stepRunDir, item)
		return item, fmt.Errorf("审批节点 %s 等待超时（%s）：没有收到人工确认", node.ID, timeout)
	default:
		// 取消与其它上下文错误都按取消记录，交给 workflowNodeError 统一收敛成“运行已取消”。
		item = e.finishApproval(index, ApprovalStatusCancelled, "", "")
		appendApprovalLog(stepRunDir, "运行被取消，审批未确认")
		writeApprovalMeta(stepRunDir, item)
		return item, waitErr
	}
}

func approvalNoteSuffix(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	return "：" + note
}

// appendApprovalLog 复用节点的 stdout.log：SSE 事件流、节点日志面板和下游模板都看同一份证据。
func appendApprovalLog(stepRunDir, line string) {
	if strings.TrimSpace(stepRunDir) == "" {
		return
	}
	if err := os.MkdirAll(stepRunDir, 0o700); err != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(stepRunDir, "stdout.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(line + "\n")
}

func writeApprovalMeta(stepRunDir string, item ApprovalRecord) {
	payload, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(stepRunDir, 0o700); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(stepRunDir, approvalMetaFile), payload, 0o600)
}

func readApprovalMeta(stepRunDir string) ApprovalRecord {
	var item ApprovalRecord
	data, err := os.ReadFile(filepath.Join(stepRunDir, approvalMetaFile))
	if err != nil {
		return item
	}
	_ = json.Unmarshal(data, &item)
	return item
}

// addApprovalContext 把审批结论暴露给下游模板，例如 {{ .steps.gate.approval.status }}。
func addApprovalContext(workflowContext map[string]string, nodeID string, item ApprovalRecord) {
	prefix := "steps." + nodeID + ".approval."
	workflowContext[prefix+"status"] = item.Status
	workflowContext[prefix+"note"] = item.Note
	workflowContext[prefix+"reviewer"] = item.Reviewer
	waited := int64(0)
	if !item.DecidedAt.IsZero() {
		waited = item.DecidedAt.Sub(item.Since).Milliseconds()
	}
	workflowContext[prefix+"waited_ms"] = strconv.FormatInt(waited, 10)
}

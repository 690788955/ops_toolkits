package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

const (
	RunStatusPending    = "pending"
	RunStatusRolledBack = "rolled_back"

	PendingStatusPending    = "pending"
	PendingStatusResolved   = "resolved"
	PendingStatusRolledBack = "rolled_back"
	PendingStatusClosed     = "closed"

	AttemptKindCheck    = "check"
	AttemptKindRollback = "rollback"
	AttemptKindResolve  = "resolve"

	attemptsDirName = "attempts"
)

// RunSnapshot 固定一次运行使用的定义、参数和目标，恢复动作不再读取当前画布。
type RunSnapshot struct {
	Workflow       *config.WorkflowConfig         `json:"workflow,omitempty"`
	Params         map[string]string              `json:"params,omitempty"`
	Targets        map[string]string              `json:"targets,omitempty"`
	Tools          map[string]ToolSnapshotBinding `json:"tools,omitempty"`
	HostIdentities map[string]string              `json:"host_identities,omitempty"`
	CreatedAt      time.Time                      `json:"created_at"`
}

// BackupRecord 记录配置下发工具实际写入的文件和备份，只用于同一运行的恢复。
type BackupRecord struct {
	NodeID                string    `json:"node_id"`
	Tool                  string    `json:"tool,omitempty"`
	HostID                string    `json:"host_id,omitempty"`
	HostName              string    `json:"host_name,omitempty"`
	TargetPath            string    `json:"target_path"`
	BackupPath            string    `json:"backup_path,omitempty"`
	SHA256                string    `json:"sha256,omitempty"`
	Status                string    `json:"status,omitempty"`
	RecordedAt            time.Time `json:"recorded_at"`
	RestoredSHA256        string    `json:"restored_sha256,omitempty"`
	RestoredAt            time.Time `json:"restored_at,omitempty"`
	RestoreExpectedSHA256 string    `json:"restore_expected_sha256,omitempty"`
	RestoreStartedAt      time.Time `json:"restore_started_at,omitempty"`
}

type PendingState struct {
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	StepID     string    `json:"step_id,omitempty"`
	Since      time.Time `json:"since"`
	Resolution string    `json:"resolution,omitempty"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

// AttemptRecord 保存恢复动作的独立证据；原始步骤和日志永远不覆盖。
type AttemptRecord struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	Error      string    `json:"error,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	LogDir     string    `json:"log_dir,omitempty"`
	NodeIDs    []string  `json:"node_ids,omitempty"`
	KeepSource bool      `json:"keep_source,omitempty"`
}

type NotificationRecord struct {
	URL       string    `json:"url"`
	Event     string    `json:"event"`
	Status    string    `json:"status"`
	Attempts  int       `json:"attempts"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	SentAt    time.Time `json:"sent_at,omitempty"`
}

type pendingRunError struct {
	err    error
	stepID string
	reason string
}

func (e *pendingRunError) Error() string { return e.err.Error() }
func (e *pendingRunError) Unwrap() error { return e.err }

func isPendingRunError(err error) bool {
	var target *pendingRunError
	return errors.As(err, &target)
}

// recoveryFailure 把“已发生配置变更后”的失败转换为待处理，写操作本身失败仍按普通失败返回。
func recoveryFailure(record *RunRecord, node config.WorkflowNode, err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return err
	}
	if !shouldPauseForRecovery(record, node) {
		return err
	}
	reason := fmt.Sprintf("节点 %s 执行失败，配置已变更，需要人工确认或回滚", node.ID)
	return &pendingRunError{err: err, stepID: node.ID, reason: reason}
}

func applyPendingState(record *RunRecord, err error) {
	var pendingErr *pendingRunError
	if !errors.As(err, &pendingErr) {
		return
	}
	record.Pending = &PendingState{
		Status: PendingStatusPending,
		Reason: pendingErr.reason,
		StepID: pendingErr.stepID,
		Since:  time.Now(),
	}
}

func newRunSnapshot(wf *config.WorkflowConfig, params map[string]string) *RunSnapshot {
	snapshot := &RunSnapshot{Params: copyParams(params), CreatedAt: time.Now()}
	if wf == nil {
		return snapshot
	}
	data, err := json.Marshal(wf)
	if err == nil {
		var clone config.WorkflowConfig
		if json.Unmarshal(data, &clone) == nil {
			clone.Steps = nil
			snapshot.Workflow = &clone
		}
	}
	if snapshot.Workflow == nil {
		snapshot.Workflow = wf
	}
	targets := map[string]string{}
	for _, node := range wf.Nodes {
		if strings.TrimSpace(node.Host) != "" {
			targets[node.ID] = node.Host
		}
	}
	if len(targets) > 0 {
		snapshot.Targets = targets
	}
	return snapshot
}

func writeRunSnapshot(runDir string, snapshot *RunSnapshot) error {
	if snapshot == nil {
		return nil
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, "snapshot.json"), data, 0o644)
}

func snapshotWorkflow(record *RunRecord) *config.WorkflowConfig {
	if record == nil || record.Snapshot == nil || record.Snapshot.Workflow == nil {
		return nil
	}
	return record.Snapshot.Workflow
}

func snapshotParams(record *RunRecord) map[string]string {
	if record == nil || record.Snapshot == nil || len(record.Snapshot.Params) == 0 {
		if record == nil {
			return nil
		}
		return recordConfigParams(record)
	}
	return copyParams(record.Snapshot.Params)
}

// recordBackupMetadata 从配置下发工具的最后一行 JSON 提取备份信息，其他工具被忽略。
func recordBackupMetadata(record *RunRecord, node config.WorkflowNode, host RemoteHost, stepRunDir string) {
	if record == nil || strings.TrimSpace(node.Tool) == "" {
		return
	}
	payload, ok := lastStdoutJSON(readTextFile(filepath.Join(stepRunDir, "stdout.log")))
	if !ok {
		return
	}
	targetPath := strings.TrimSpace(stringField(payload, "target_path"))
	if targetPath == "" {
		return
	}
	backupPath := strings.TrimSpace(stringField(payload, "backup_path"))
	sha := strings.TrimSpace(stringField(payload, "sha256"))
	status := strings.TrimSpace(stringField(payload, "status"))
	if status == "" {
		status = "updated"
	}
	if backupPath == "" && status != "created" && status != "skipped" {
		return
	}
	record.Backups = append(record.Backups, BackupRecord{
		NodeID:     node.ID,
		Tool:       node.Tool,
		HostID:     host.ID,
		HostName:   host.Name,
		TargetPath: targetPath,
		BackupPath: backupPath,
		SHA256:     sha,
		Status:     status,
		RecordedAt: time.Now(),
	})
}

func stringField(payload map[string]interface{}, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func filterBackupsOutside(backups []BackupRecord, excluded map[string]bool) []BackupRecord {
	out := make([]BackupRecord, 0, len(backups))
	for _, backup := range backups {
		if excluded[backup.NodeID] {
			continue
		}
		out = append(out, backup)
	}
	return out
}

func recoveryPolicy(record *RunRecord) *config.WorkflowRecovery {
	wf := snapshotWorkflow(record)
	if wf == nil {
		return nil
	}
	return wf.Recovery
}

// shouldPauseForRecovery 只对已发生配置变更后的失败生效，写操作本身失败仍按普通失败处理。
func shouldPauseForRecovery(record *RunRecord, node config.WorkflowNode) bool {
	if record == nil || len(record.Backups) == 0 {
		return false
	}
	hasChange := false
	for _, backup := range record.Backups {
		if !strings.EqualFold(strings.TrimSpace(backup.Status), "skipped") {
			hasChange = true
			break
		}
	}
	if !hasChange {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(node.OnFailure), "pending") {
		return true
	}
	policy := recoveryPolicy(record)
	return policy != nil && policy.PendingOnFailure
}

func (r *Runner) changeNodeIDs(wf *config.WorkflowConfig) map[string]bool {
	out := map[string]bool{}
	if wf == nil {
		return out
	}
	if wf.Recovery != nil {
		for _, id := range wf.Recovery.ChangeNodeIDs {
			out[id] = true
		}
		if len(wf.Recovery.ChangeNodeIDs) > 0 {
			return out
		}
	}
	for _, node := range wf.Nodes {
		if out[node.ID] || node.Tool == "" {
			continue
		}
		tool, err := r.Registry.Tool(node.Tool)
		if err != nil {
			continue
		}
		if toolHasParameter(tool.Config, "backup") && toolHasOutput(tool.Config, "backup_path") {
			out[node.ID] = true
		}
	}
	return out
}

func (r *Runner) serviceNodeIDs(wf *config.WorkflowConfig) map[string]bool {
	out := map[string]bool{}
	if wf == nil {
		return out
	}
	if wf.Recovery != nil {
		for _, id := range wf.Recovery.ServiceNodeIDs {
			out[id] = true
		}
		if len(wf.Recovery.ServiceNodeIDs) > 0 {
			return out
		}
	}
	for _, node := range wf.Nodes {
		if out[node.ID] || node.Tool == "" {
			continue
		}
		tool, err := r.Registry.Tool(node.Tool)
		if err != nil {
			continue
		}
		if toolHasParameter(tool.Config, "service") && toolHasParameter(tool.Config, "action") {
			out[node.ID] = true
		}
	}
	return out
}

func toolHasParameter(tool *config.ToolConfig, name string) bool {
	if tool == nil {
		return false
	}
	for _, parameter := range tool.Parameters {
		if parameter.Name == name {
			return true
		}
	}
	return false
}

func toolHasOutput(tool *config.ToolConfig, name string) bool {
	if tool == nil {
		return false
	}
	for _, output := range tool.Outputs {
		if output.Name == name {
			return true
		}
	}
	return false
}

func (r *Runner) diffNodeIDs(wf *config.WorkflowConfig) map[string]bool {
	out := map[string]bool{}
	if wf == nil {
		return out
	}
	if wf.Recovery != nil && wf.Recovery.DiffNodeID != "" {
		out[wf.Recovery.DiffNodeID] = true
		return out
	}
	for _, node := range wf.Nodes {
		if out[node.ID] || node.Tool == "" {
			continue
		}
		tool, err := r.Registry.Tool(node.Tool)
		if err != nil {
			continue
		}
		if toolHasOutput(tool.Config, "changed") && toolHasOutput(tool.Config, "target_path") {
			out[node.ID] = true
		}
	}
	return out
}

// noDiffFromStep 判断差异检查节点是否明确报告“无变化”。
func noDiffFromStep(workflowContext map[string]string, nodeID string) bool {
	prefix := "steps." + nodeID + ".outputs."
	changed := strings.ToLower(strings.TrimSpace(workflowContext[prefix+"changed"]))
	if changed == "false" || changed == "0" {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(workflowContext[prefix+"status"]))
	return status == "identical"
}

func skippedChangeStep(node config.WorkflowNode, nodeType, reason string) StepRecord {
	step := skippedStepRecord(node, nodeType, reason)
	step.SkippedReason = reason
	return step
}

func (r *Runner) startRecoveryAttempt(record *RunRecord, runDir, kind string, nodeIDs []string) (*AttemptRecord, string, error) {
	id := fmt.Sprintf("attempt-%d-%s-%d", len(record.Attempts)+1, kind, time.Now().UnixNano())
	attempt := AttemptRecord{
		ID:        id,
		Kind:      kind,
		Status:    "running",
		StartedAt: time.Now(),
		NodeIDs:   append([]string{}, nodeIDs...),
		LogDir:    filepath.ToSlash(filepath.Join(attemptsDirName, id)),
	}
	dir := filepath.Join(runDir, filepath.FromSlash(attempt.LogDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	record.Attempts = append(record.Attempts, attempt)
	if err := r.saveRecord(runDir, record); err != nil {
		return nil, "", fmt.Errorf("保存恢复尝试失败，未执行恢复动作：%w", err)
	}
	return &record.Attempts[len(record.Attempts)-1], dir, nil
}

func finishAttempt(attempt *AttemptRecord, status, detail string, runErr error) {
	attempt.EndedAt = time.Now()
	attempt.Status = status
	attempt.Detail = detail
	if runErr != nil {
		attempt.Error = runErr.Error()
	}
}

func appendAttemptLine(dir, format string, args ...interface{}) {
	if dir == "" {
		return
	}
	line := fmt.Sprintf(format, args...)
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	_ = os.MkdirAll(dir, 0o755)
	file, err := os.OpenFile(filepath.Join(dir, "summary.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(line)
}

func recoveryHealthNodeIDs(record *RunRecord) []string {
	policy := recoveryPolicy(record)
	if policy != nil && len(policy.HealthNodeIDs) > 0 {
		return append([]string{}, policy.HealthNodeIDs...)
	}
	if record.Pending != nil && record.Pending.StepID != "" {
		return []string{record.Pending.StepID}
	}
	return nil
}

func recoveryServiceNodeIDs(r *Runner, record *RunRecord) []string {
	policy := recoveryPolicy(record)
	if policy != nil && len(policy.ServiceNodeIDs) > 0 {
		return append([]string{}, policy.ServiceNodeIDs...)
	}
	wf := snapshotWorkflow(record)
	ids := make([]string, 0)
	serviceNodes := r.serviceNodeIDs(wf)
	for _, node := range wf.Nodes {
		if serviceNodes[node.ID] {
			ids = append(ids, node.ID)
		}
	}
	return ids
}

func findWorkflowNode(wf *config.WorkflowConfig, nodeID string) (config.WorkflowNode, bool) {
	if wf == nil {
		return config.WorkflowNode{}, false
	}
	for _, node := range wf.Nodes {
		if node.ID == nodeID {
			return node, true
		}
	}
	return config.WorkflowNode{}, false
}

func (r *Runner) recoveryWorkflowContext(wf *config.WorkflowConfig, record *RunRecord, runDir string) (map[string]string, error) {
	workflowContext, active, nodeByID, _, edgesByFrom, err := prepareWorkflowExecutionState(wf, snapshotParams(record))
	if err != nil {
		return nil, err
	}
	if err := r.rebuildWorkflowContextFromSteps(workflowContext, record.Steps, nodeByID, runDir, edgesByFrom, active); err != nil {
		return nil, err
	}
	return workflowContext, nil
}

func (r *Runner) executeRecoveryNode(ctx context.Context, node config.WorkflowNode, workflowContext map[string]string, attemptDir string, out, errOut io.Writer) (string, error) {
	nodeType := workflowNodeType(node)
	nodeDir := filepath.Join(attemptDir, node.ID)
	switch nodeType {
	case config.WorkflowNodeTypeHTTP:
		meta, err := r.executeHTTPNode(ctx, node, workflowContext, nodeDir, out, errOut)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("HTTP %d", meta.Status), nil
	case config.WorkflowNodeTypeTool:
		tool, err := r.Registry.Tool(node.Tool)
		if err != nil {
			return "", err
		}
		stepParams := resolveStepParams(snapshotParamsForContext(workflowContext), workflowContext, node.Params)
		values, flat, _, err := r.resolveToolValues(tool, config.StringMapToValues(stepParams))
		if err != nil {
			return "", err
		}
		if err := r.executeTool(ctx, tool, values, flat, nodeDir, node.Host, out, errOut); err != nil {
			return "", err
		}
		return fmt.Sprintf("工具 %s 执行成功", node.Tool), nil
	default:
		return "", fmt.Errorf("节点 %s 类型 %s 暂不支持自动恢复检查", node.ID, nodeType)
	}
}

func snapshotParamsForContext(workflowContext map[string]string) map[string]string {
	if workflowContext == nil {
		return nil
	}
	params := make(map[string]string, len(workflowContext))
	for key, value := range workflowContext {
		if strings.HasPrefix(key, "steps.") {
			continue
		}
		params[key] = value
	}
	return params
}

// RetryPendingCheck 只重跑健康检查类节点，不重新执行任何配置写入。
func (r *Runner) RetryPendingCheck(ctx context.Context, record *RunRecord, runDir string) (actionErr error) {
	if record == nil || record.Pending == nil || record.Pending.Status != PendingStatusPending {
		return fmt.Errorf("运行不在待处理状态")
	}
	wf := snapshotWorkflow(record)
	if wf == nil {
		return fmt.Errorf("运行快照缺失，无法重试检查")
	}
	if err := r.validateRunSnapshotBindings(record.Snapshot); err != nil {
		return err
	}
	for _, backup := range record.Backups {
		if backup.RestoreExpectedSHA256 != "" && backup.RestoredSHA256 == "" {
			return fmt.Errorf("配置回滚动作曾被中断，请继续回滚确认文件恢复状态后再复检")
		}
	}
	anyRestored, allRestored := rollbackRestoreState(record.Backups)
	if anyRestored && !allRestored {
		return fmt.Errorf("配置只完成部分回滚，请继续回滚以恢复全部配置后再复检")
	}
	nodeIDs := recoveryHealthNodeIDs(record)
	if len(nodeIDs) == 0 {
		return fmt.Errorf("待处理记录没有可重试的检查节点")
	}
	workflowContext, err := r.recoveryWorkflowContext(wf, record, runDir)
	if err != nil {
		return err
	}
	attempt, attemptDir, err := r.startRecoveryAttempt(record, runDir, AttemptKindCheck, nodeIDs)
	if err != nil {
		return err
	}
	defer func() { actionErr = errors.Join(actionErr, r.saveRecord(runDir, record)) }()
	// 先检查全部目标，不能先执行部分检查后才发现后续目标会写配置或重启服务。
	for _, nodeID := range nodeIDs {
		node, ok := findWorkflowNode(wf, nodeID)
		if !ok {
			err = fmt.Errorf("运行快照中找不到检查节点 %s", nodeID)
		} else {
			err = r.validateRecoveryCheck(wf, node, workflowContext)
		}
		if err != nil {
			appendAttemptLine(attemptDir, "PRECHECK FAIL %s: %v", nodeID, err)
			finishAttempt(attempt, "failed", "重试检查目标不安全", err)
			return err
		}
	}
	var runErr error
	details := make([]string, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node, ok := findWorkflowNode(wf, nodeID)
		if !ok {
			runErr = fmt.Errorf("运行快照中找不到检查节点 %s", nodeID)
			break
		}
		appendAttemptLine(attemptDir, "== 重试检查 %s ==", nodeID)
		detail, err := r.executeRecoveryNode(ctx, node, workflowContext, attemptDir, io.Discard, io.Discard)
		if err != nil {
			appendAttemptLine(attemptDir, "FAIL %s: %v", nodeID, err)
			runErr = err
			break
		}
		appendAttemptLine(attemptDir, "OK %s: %s", nodeID, detail)
		details = append(details, detail)
	}
	if runErr != nil {
		finishAttempt(attempt, "failed", "重试检查未通过", runErr)
		return runErr
	}
	finishAttempt(attempt, "succeeded", strings.Join(details, "；"), nil)
	record.Pending.Status = PendingStatusResolved
	record.Pending.Resolution = "重试检查通过，服务已恢复"
	record.Pending.ResolvedAt = time.Now()
	record.Status = "succeeded"
	record.EndedAt = time.Now()
	if allRestored {
		record.Status = RunStatusRolledBack
		record.Pending.Status = PendingStatusRolledBack
		record.Pending.Resolution = "配置已回滚，服务已恢复"
		r.appendRecoveryNotification(ctx, record, config.WorkflowNotifyRolledBack)
	} else {
		record.Error = ""
		r.appendRecoveryNotification(ctx, record, config.WorkflowNotifySucceeded)
	}
	return nil
}

func rollbackRestoreState(backups []BackupRecord) (anyRestored, allRestored bool) {
	allRestored = len(backups) > 0
	for _, backup := range backups {
		if backup.Status == "skipped" {
			continue
		}
		if backup.RestoredSHA256 == "" {
			allRestored = false
		} else {
			anyRestored = true
		}
	}
	return anyRestored, anyRestored && allRestored
}

func (r *Runner) validateRecoveryCheck(wf *config.WorkflowConfig, node config.WorkflowNode, workflowContext map[string]string) error {
	if r.changeNodeIDs(wf)[node.ID] || node.Tool == "config.deploy.rollback" {
		return fmt.Errorf("节点 %s 是配置变更节点，不能作为只读检查重试", node.ID)
	}
	switch workflowNodeType(node) {
	case config.WorkflowNodeTypeHTTP:
		method, err := config.NormalizeHTTPMethod(renderTemplate(node.HTTP.Method, workflowContext))
		if err != nil {
			return err
		}
		if method != http.MethodGet && method != http.MethodHead {
			return fmt.Errorf("节点 %s 使用 HTTP %s，只读检查仅支持 GET 或 HEAD", node.ID, method)
		}
	case config.WorkflowNodeTypeTool:
		tool, err := r.Registry.Tool(node.Tool)
		if err != nil {
			return err
		}
		if toolHasParameter(tool.Config, "backup") && toolHasOutput(tool.Config, "backup_path") {
			return fmt.Errorf("节点 %s 使用配置写入工具，不能作为只读检查重试", node.ID)
		}
		if r.serviceNodeIDs(wf)[node.ID] || toolHasParameter(tool.Config, "service") && toolHasParameter(tool.Config, "action") {
			params := resolveStepParams(snapshotParamsForContext(workflowContext), workflowContext, node.Params)
			_, flat, _, err := r.resolveToolValues(tool, config.StringMapToValues(params))
			if err != nil {
				return err
			}
			if strings.ToLower(strings.TrimSpace(flat["action"])) != "status" {
				return fmt.Errorf("节点 %s 是服务变更节点，不能作为只读检查重试", node.ID)
			}
		}
	default:
		return fmt.Errorf("节点 %s 类型 %s 暂不支持只读恢复检查", node.ID, workflowNodeType(node))
	}
	return nil
}

func (r *Runner) validateRecoveryService(wf *config.WorkflowConfig, node config.WorkflowNode, workflowContext map[string]string) error {
	if workflowNodeType(node) != config.WorkflowNodeTypeTool || node.Tool == "config.deploy.rollback" || r.changeNodeIDs(wf)[node.ID] {
		return fmt.Errorf("服务节点 %s 必须使用服务操作工具，不能再次下发或回滚配置", node.ID)
	}
	tool, err := r.Registry.Tool(node.Tool)
	if err != nil {
		return err
	}
	if !toolHasParameter(tool.Config, "service") || !toolHasParameter(tool.Config, "action") || toolHasParameter(tool.Config, "backup") && toolHasOutput(tool.Config, "backup_path") {
		return fmt.Errorf("服务节点 %s 必须使用包含 service 和 action 参数的服务操作工具", node.ID)
	}
	params := resolveStepParams(snapshotParamsForContext(workflowContext), workflowContext, node.Params)
	_, flat, _, err := r.resolveToolValues(tool, config.StringMapToValues(params))
	if err != nil {
		return err
	}
	if strings.TrimSpace(flat["service"]) == "" {
		return fmt.Errorf("服务节点 %s 的服务名不能为空", node.ID)
	}
	switch strings.ToLower(strings.TrimSpace(flat["action"])) {
	case "start", "stop", "restart", "reload", "status", "try-restart", "reload-or-restart":
		return nil
	default:
		return fmt.Errorf("服务节点 %s 的服务操作 %s 不支持自动恢复", node.ID, flat["action"])
	}
}

type rollbackPlanItem struct {
	BackupIndex    int
	Backup         BackupRecord
	RestoredSHA256 string
}

// RollbackPendingRun 先整体检查全部备份，再按相反顺序恢复文件、重启关联服务并复检。
func (r *Runner) RollbackPendingRun(ctx context.Context, record *RunRecord, runDir string) (actionErr error) {
	if record == nil || record.Pending == nil || record.Pending.Status != PendingStatusPending {
		return fmt.Errorf("运行不在待处理状态")
	}
	wf := snapshotWorkflow(record)
	if wf == nil {
		return fmt.Errorf("运行快照缺失，无法回滚")
	}
	if err := r.validateRunSnapshotBindings(record.Snapshot); err != nil {
		return err
	}
	if len(record.Backups) == 0 {
		return fmt.Errorf("该运行没有可回滚的配置备份")
	}
	rollbackTool, err := r.Registry.Tool("config.deploy.rollback")
	if err != nil {
		return fmt.Errorf("未安装配置回滚工具 config.deploy.rollback，无法执行一键回滚")
	}
	if _, ok := record.Snapshot.Tools["config.deploy.rollback"]; !ok {
		return fmt.Errorf("运行快照未固定配置回滚工具，无法安全回滚")
	}
	healthNodeIDs := recoveryHealthNodeIDs(record)
	serviceNodeIDs := recoveryServiceNodeIDs(r, record)
	plan := make([]rollbackPlanItem, 0, len(record.Backups))
	blocked := make([]string, 0)
	for index, backup := range record.Backups {
		switch strings.ToLower(strings.TrimSpace(backup.Status)) {
		case "skipped":
			continue
		case "created":
			blocked = append(blocked, fmt.Sprintf("%s 为本次新建文件且没有历史备份，默认不自动删除", backup.TargetPath))
			continue
		}
		if strings.TrimSpace(backup.TargetPath) == "" || strings.TrimSpace(backup.BackupPath) == "" {
			blocked = append(blocked, fmt.Sprintf("节点 %s 的备份元数据不完整", backup.NodeID))
			continue
		}
		if strings.TrimSpace(backup.SHA256) == "" {
			blocked = append(blocked, fmt.Sprintf("节点 %s 缺少下发后的 SHA256，无法确认目标文件是否被外部修改", backup.NodeID))
			continue
		}
		_, ok := findWorkflowNode(wf, backup.NodeID)
		if !ok {
			blocked = append(blocked, fmt.Sprintf("运行快照中找不到变更节点 %s", backup.NodeID))
			continue
		}
		plan = append(plan, rollbackPlanItem{BackupIndex: index, Backup: backup})
	}
	attempt, attemptDir, err := r.startRecoveryAttempt(record, runDir, AttemptKindRollback, nil)
	if err != nil {
		return err
	}
	defer func() { actionErr = errors.Join(actionErr, r.saveRecord(runDir, record)) }()
	if len(plan) == 0 && len(blocked) == 0 {
		finishAttempt(attempt, "failed", "没有需要回滚的配置变更", fmt.Errorf("该运行没有可回滚的配置变更"))
		return fmt.Errorf("该运行没有可回滚的配置变更")
	}
	if len(blocked) > 0 {
		runErr := fmt.Errorf("回滚前检查未通过：%s", strings.Join(blocked, "；"))
		appendAttemptLine(attemptDir, "PRECHECK FAIL %s", runErr.Error())
		finishAttempt(attempt, "failed", "回滚前检查未通过", runErr)
		return runErr
	}
	workflowContext, err := r.recoveryWorkflowContext(wf, record, runDir)
	if err == nil {
		for _, nodeID := range healthNodeIDs {
			node, ok := findWorkflowNode(wf, nodeID)
			if !ok {
				err = fmt.Errorf("运行快照中找不到健康检查节点 %s", nodeID)
			} else {
				err = r.validateRecoveryCheck(wf, node, workflowContext)
			}
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		for _, nodeID := range serviceNodeIDs {
			node, ok := findWorkflowNode(wf, nodeID)
			if !ok {
				err = fmt.Errorf("运行快照中找不到服务节点 %s", nodeID)
			} else {
				err = r.validateRecoveryService(wf, node, workflowContext)
			}
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		finishAttempt(attempt, "failed", "回滚前检查未通过", err)
		return err
	}
	appendAttemptLine(attemptDir, "回滚计划：%d 个备份，%d 个服务节点，%d 个健康检查节点", len(plan), len(serviceNodeIDs), len(healthNodeIDs))
	for index, item := range plan {
		precheckDir := filepath.Join(attemptDir, "precheck", item.Backup.NodeID)
		expectedSHA := item.Backup.SHA256
		if item.Backup.RestoredSHA256 != "" {
			expectedSHA = item.Backup.RestoredSHA256
		}
		checkParams := map[string]string{
			"action":          "check",
			"target_path":     item.Backup.TargetPath,
			"backup_path":     item.Backup.BackupPath,
			"expected_sha256": expectedSHA,
		}
		detail, err := r.executeRecoveryTool(ctx, rollbackTool, checkParams, item.Backup.HostID, precheckDir)
		alreadyRestored := item.Backup.RestoredSHA256 != ""
		if err != nil && !alreadyRestored && item.Backup.RestoreExpectedSHA256 != "" {
			// 中断可能发生在文件替换之后、记录完成之前，只接受本次已持久化的备份摘要。
			checkParams["expected_sha256"] = item.Backup.RestoreExpectedSHA256
			detail, err = r.executeRecoveryTool(ctx, rollbackTool, checkParams, item.Backup.HostID, filepath.Join(precheckDir, "resume"))
			alreadyRestored = err == nil
		}
		if err == nil {
			payload, _ := lastStdoutJSON(detail)
			plan[index].RestoredSHA256 = strings.TrimSpace(stringField(payload, "sha256"))
			if plan[index].RestoredSHA256 == "" {
				err = fmt.Errorf("回滚检查没有返回备份 SHA256，无法记录恢复进度")
			} else if item.Backup.RestoredSHA256 != "" && item.Backup.RestoredSHA256 != plan[index].RestoredSHA256 {
				err = fmt.Errorf("备份文件在恢复后被修改，无法继续自动恢复")
			} else if item.Backup.RestoreExpectedSHA256 != "" && item.Backup.RestoreExpectedSHA256 != plan[index].RestoredSHA256 {
				err = fmt.Errorf("备份文件在回滚开始后被修改，无法继续自动恢复")
			}
		}
		if err == nil && alreadyRestored && item.Backup.RestoredSHA256 == "" {
			record.Backups[item.BackupIndex].RestoredSHA256 = plan[index].RestoredSHA256
			record.Backups[item.BackupIndex].RestoredAt = time.Now()
			if err := r.saveRecord(runDir, record); err != nil {
				finishAttempt(attempt, "failed", "保存中断恢复状态失败", err)
				return err
			}
			plan[index].Backup = record.Backups[item.BackupIndex]
		}
		if err != nil {
			runErr := fmt.Errorf("备份 %s 检查未通过：%w", item.Backup.TargetPath, err)
			appendAttemptLine(attemptDir, "PRECHECK FAIL %s: %v", item.Backup.TargetPath, err)
			finishAttempt(attempt, "failed", "回滚前检查未通过", runErr)
			return runErr
		}
		appendAttemptLine(attemptDir, "PRECHECK OK %s: %s", item.Backup.TargetPath, detail)
	}
	for index := len(plan) - 1; index >= 0; index-- {
		item := plan[index]
		if item.Backup.RestoredSHA256 != "" {
			appendAttemptLine(attemptDir, "SKIP %s 已恢复，继续服务与健康检查", item.Backup.TargetPath)
			continue
		}
		rollbackDir := filepath.Join(attemptDir, "rollback", item.Backup.NodeID)
		appendAttemptLine(attemptDir, "== 恢复 %s ==", item.Backup.TargetPath)
		record.Backups[item.BackupIndex].RestoreExpectedSHA256 = item.RestoredSHA256
		record.Backups[item.BackupIndex].RestoreStartedAt = time.Now()
		if err := r.saveRecord(runDir, record); err != nil {
			finishAttempt(attempt, "failed", "保存回滚计划失败，未恢复文件", err)
			return err
		}
		if _, err := r.executeRecoveryTool(ctx, rollbackTool, map[string]string{
			"action":          "rollback",
			"target_path":     item.Backup.TargetPath,
			"backup_path":     item.Backup.BackupPath,
			"expected_sha256": item.Backup.SHA256,
		}, item.Backup.HostID, rollbackDir); err != nil {
			runErr := fmt.Errorf("恢复 %s 失败：%w", item.Backup.TargetPath, err)
			appendAttemptLine(attemptDir, "FAIL %s: %v", item.Backup.TargetPath, err)
			finishAttempt(attempt, "failed", "配置回滚失败", runErr)
			return runErr
		}
		record.Backups[item.BackupIndex].RestoredSHA256 = item.RestoredSHA256
		record.Backups[item.BackupIndex].RestoredAt = time.Now()
		if err := r.saveRecord(runDir, record); err != nil {
			finishAttempt(attempt, "failed", "文件已恢复但恢复进度保存失败", err)
			return err
		}
		appendAttemptLine(attemptDir, "OK %s", item.Backup.TargetPath)
	}
	for _, nodeID := range serviceNodeIDs {
		node, ok := findWorkflowNode(wf, nodeID)
		if !ok {
			runErr := fmt.Errorf("运行快照中找不到服务节点 %s", nodeID)
			finishAttempt(attempt, "failed", "关联服务重启失败", runErr)
			return runErr
		}
		appendAttemptLine(attemptDir, "== 重启关联服务 %s ==", nodeID)
		if _, err := r.executeRecoveryNode(ctx, node, workflowContext, filepath.Join(attemptDir, "service"), io.Discard, io.Discard); err != nil {
			runErr := fmt.Errorf("配置已回滚，但关联服务节点 %s 重启失败：%w", nodeID, err)
			appendAttemptLine(attemptDir, "FAIL %s: %v", nodeID, err)
			finishAttempt(attempt, "failed", "配置已回滚但服务重启失败", runErr)
			return runErr
		}
		appendAttemptLine(attemptDir, "OK %s", nodeID)
	}
	for _, nodeID := range healthNodeIDs {
		node, ok := findWorkflowNode(wf, nodeID)
		if !ok {
			runErr := fmt.Errorf("运行快照中找不到健康检查节点 %s", nodeID)
			finishAttempt(attempt, "failed", "回滚后健康检查失败", runErr)
			return runErr
		}
		appendAttemptLine(attemptDir, "== 回滚后健康检查 %s ==", nodeID)
		if _, err := r.executeRecoveryNode(ctx, node, workflowContext, filepath.Join(attemptDir, "health"), io.Discard, io.Discard); err != nil {
			runErr := fmt.Errorf("配置已回滚，但健康检查节点 %s 未通过：%w", nodeID, err)
			appendAttemptLine(attemptDir, "FAIL %s: %v", nodeID, err)
			finishAttempt(attempt, "failed", "配置已回滚但健康检查失败", runErr)
			return runErr
		}
		appendAttemptLine(attemptDir, "OK %s", nodeID)
	}
	finishAttempt(attempt, "succeeded", "配置已回滚，服务已恢复", nil)
	record.Pending.Status = PendingStatusRolledBack
	record.Pending.Resolution = "配置已回滚，服务已恢复"
	record.Pending.ResolvedAt = time.Now()
	record.Status = RunStatusRolledBack
	record.EndedAt = time.Now()
	r.appendRecoveryNotification(ctx, record, config.WorkflowNotifyRolledBack)
	return nil
}

func (r *Runner) executeRecoveryTool(ctx context.Context, tool *registry.Tool, params map[string]string, hostID, runDir string) (string, error) {
	values, flat, _, err := r.resolveToolValues(tool, config.StringMapToValues(params))
	if err != nil {
		return "", err
	}
	if err := r.executeTool(ctx, tool, values, flat, runDir, hostID, io.Discard, io.Discard); err != nil {
		if stderr := strings.TrimSpace(readTextFile(filepath.Join(runDir, "stderr.log"))); stderr != "" {
			return "", fmt.Errorf("%w: %s", err, stderr)
		}
		return "", err
	}
	stdout := readTextFile(filepath.Join(runDir, "stdout.log"))
	return strings.TrimSpace(stdout), nil
}

func (r *Runner) ResolvePendingRun(record *RunRecord, note string) error {
	if record == nil || record.Pending == nil || record.Pending.Status != PendingStatusPending {
		return fmt.Errorf("运行不在待处理状态")
	}
	note = strings.TrimSpace(note)
	if note == "" {
		note = "已人工结束处理"
	}
	record.Pending.Status = PendingStatusClosed
	record.Pending.Resolution = note
	record.Pending.ResolvedAt = time.Now()
	record.Status = "failed"
	record.EndedAt = time.Now()
	record.Attempts = append(record.Attempts, AttemptRecord{
		ID:        fmt.Sprintf("attempt-%d-%s", len(record.Attempts)+1, AttemptKindResolve),
		Kind:      AttemptKindResolve,
		Status:    "succeeded",
		StartedAt: time.Now(),
		EndedAt:   time.Now(),
		Detail:    note,
	})
	r.appendRecoveryNotification(context.Background(), record, config.WorkflowNotifyFailed)
	return nil
}

func (r *Runner) appendRecoveryNotification(ctx context.Context, record *RunRecord, event string) {
	policy := recoveryPolicy(record)
	if policy == nil || policy.Notification == nil {
		return
	}
	cfg := policy.Notification
	if strings.TrimSpace(cfg.URL) == "" || !notificationEventEnabled(cfg, event) {
		return
	}
	notification := NotificationRecord{URL: cfg.URL, Event: event, UpdatedAt: time.Now()}
	sendCtx, cancel := notificationContext(ctx, cfg.Timeout)
	defer cancel()
	err := deliverWebhook(sendCtx, cfg, record, event)
	notification.Attempts = 1
	notification.Status = "sent"
	if err != nil {
		notification.Status = "failed"
		notification.Error = err.Error()
	} else {
		notification.SentAt = time.Now()
	}
	record.Notifications = append(record.Notifications, notification)
}

// RetryNotification 只重发指定通知，不重新执行任何配置变更。
func (r *Runner) RetryNotification(ctx context.Context, record *RunRecord, index int) error {
	if record == nil || index < 0 || index >= len(record.Notifications) {
		return fmt.Errorf("通知记录不存在")
	}
	policy := recoveryPolicy(record)
	if policy == nil || policy.Notification == nil || strings.TrimSpace(policy.Notification.URL) == "" {
		return fmt.Errorf("运行快照中没有通知配置")
	}
	notification := &record.Notifications[index]
	sendCtx, cancel := notificationContext(ctx, policy.Notification.Timeout)
	defer cancel()
	err := deliverWebhook(sendCtx, policy.Notification, record, notification.Event)
	notification.Attempts++
	notification.UpdatedAt = time.Now()
	if err != nil {
		notification.Status = "failed"
		notification.Error = err.Error()
		return err
	}
	notification.Status = "sent"
	notification.Error = ""
	notification.SentAt = time.Now()
	return nil
}

func notificationEventEnabled(cfg *config.WorkflowNotification, event string) bool {
	if cfg == nil || len(cfg.Events) == 0 {
		return true
	}
	for _, item := range cfg.Events {
		if item == event {
			return true
		}
	}
	return false
}

func notificationContext(ctx context.Context, timeoutValue string) (context.Context, context.CancelFunc) {
	timeout := config.ParseTimeout(timeoutValue, 10*time.Second)
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, timeout)
}

func deliverWebhook(ctx context.Context, cfg *config.WorkflowNotification, record *RunRecord, event string) error {
	if cfg == nil || strings.TrimSpace(cfg.URL) == "" {
		return nil
	}
	payload := map[string]interface{}{
		"event":       event,
		"run_id":      record.ID,
		"workflow_id": record.Target,
		"status":      record.Status,
		"target":      record.Target,
		"error":       record.Error,
		"started_at":  record.StartedAt,
		"ended_at":    record.EndedAt,
	}
	if record.Pending != nil {
		payload["pending"] = record.Pending
	}
	if len(record.Backups) > 0 {
		payload["backups"] = record.Backups
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range cfg.Headers {
		request.Header.Set(key, value)
	}
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Webhook 返回状态码 %d", response.StatusCode)
	}
	return nil
}

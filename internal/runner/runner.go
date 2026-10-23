package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

type Runner struct {
	Registry *registry.Registry
	RunsDir  string
	remote   RemoteExecutor
}

const workflowUploadResultFile = "upload-result.json"

// LocalTargetID 让批量执行可以把本机作为与操作机并列的目标。
const LocalTargetID = "__local__"

type RemoteHost struct {
	ID   string
	Name string
}

type RemoteToolRequest struct {
	HostID        string
	ToolID        string
	ToolDir       string
	Entry         string
	Workdir       string
	Args          []string
	Env           []string
	ParamFile     string
	ParamName     string
	GlobalEnvFile string
	GlobalEnv     []string
}

type RemoteExecutor interface {
	DescribeHost(hostID string) (RemoteHost, error)
	RunTool(ctx context.Context, req RemoteToolRequest, stdout, stderr io.Writer) error
}

func (r *Runner) SetRemoteExecutor(exec RemoteExecutor) {
	r.remote = exec
}

type RunRecord struct {
	ID            string                 `json:"id"`
	Kind          string                 `json:"kind"`
	Target        string                 `json:"target"`
	BatchID       string                 `json:"batch_id,omitempty"`
	HostID        string                 `json:"host_id,omitempty"`
	HostName      string                 `json:"host_name,omitempty"`
	Status        string                 `json:"status"`
	StartedAt     time.Time              `json:"started_at"`
	EndedAt       time.Time              `json:"ended_at"`
	Params        map[string]string      `json:"params"`
	Config        map[string]interface{} `json:"config,omitempty"`
	Steps         []StepRecord           `json:"steps,omitempty"`
	Error         string                 `json:"error,omitempty"`
	Snapshot      *RunSnapshot           `json:"snapshot,omitempty"`
	Pending       *PendingState          `json:"pending,omitempty"`
	Backups       []BackupRecord         `json:"backups,omitempty"`
	Attempts      []AttemptRecord        `json:"attempts,omitempty"`
	Notifications []NotificationRecord   `json:"notifications,omitempty"`
}

type StepRecord struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	Tool           string    `json:"tool,omitempty"`
	HostID         string    `json:"host_id,omitempty"`
	HostName       string    `json:"host_name,omitempty"`
	Status         string    `json:"status"`
	StartedAt      time.Time `json:"started_at"`
	EndedAt        time.Time `json:"ended_at"`
	Error          string    `json:"error,omitempty"`
	ConditionInput string    `json:"condition_input,omitempty"`
	MatchedCase    string    `json:"matched_case,omitempty"`
	SkippedReason  string    `json:"skipped_reason,omitempty"`
	LoopTarget     string    `json:"loop_target,omitempty"`
	LoopIterations int       `json:"loop_iterations,omitempty"`
	HTTPStatus     int       `json:"http_status,omitempty"`
}

func New(reg *registry.Registry) *Runner {
	return &Runner{Registry: reg, RunsDir: filepath.Join(reg.BaseDir, filepath.FromSlash(reg.Root.Paths.Logs))}
}

func (r *Runner) RunTool(ctx context.Context, id string, params map[string]string, out, errOut io.Writer) (*RunRecord, error) {
	return r.RunToolValues(ctx, id, config.StringMapToValues(params), out, errOut)
}

func (r *Runner) RunToolValues(ctx context.Context, id string, params map[string]interface{}, out, errOut io.Writer) (*RunRecord, error) {
	return r.RunToolValuesOn(ctx, id, "", params, out, errOut)
}

type BatchRun struct {
	BatchID string                        `json:"batch_id"`
	Runs    []*RunRecord                  `json:"runs"`
	Cancels map[string]context.CancelFunc `json:"-"`
}

func (r *Runner) RunToolValuesOn(ctx context.Context, id, hostID string, params map[string]interface{}, out, errOut io.Writer) (*RunRecord, error) {
	return r.runToolValuesOn(ctx, id, hostID, "", params, out, errOut)
}

func (r *Runner) RunToolValuesOnTargets(ctx context.Context, id string, hostIDs []string, params map[string]interface{}, out, errOut io.Writer) (*BatchRun, error) {
	unique, batchID, err := r.prepareBatchTargets(id, hostIDs)
	if err != nil {
		return nil, err
	}
	runs := make([]*RunRecord, len(unique))
	errs := make([]error, len(unique))
	var wg sync.WaitGroup
	for index, hostID := range unique {
		wg.Add(1)
		go func(index int, hostID string) {
			defer wg.Done()
			runs[index], errs[index] = r.runToolValuesOn(ctx, id, hostID, batchID, params, out, errOut)
		}(index, hostID)
	}
	wg.Wait()
	return &BatchRun{BatchID: batchID, Runs: runs}, errors.Join(errs...)
}

func (r *Runner) runToolValuesOn(ctx context.Context, id, hostID, batchID string, params map[string]interface{}, out, errOut io.Writer) (*RunRecord, error) {
	tool, err := r.Registry.Tool(id)
	if err != nil {
		return nil, err
	}
	host, executionHostID, err := r.resolveTargetHost(hostID)
	if err != nil {
		return nil, err
	}
	finalValues, finalParams, sensitivePaths, err := r.resolveToolValues(tool, params)
	if err != nil {
		return nil, err
	}
	record := newRecord("tool", id, config.RedactSensitive(finalValues, sensitivePaths))
	record.BatchID = batchID
	record.HostID = host.ID
	record.HostName = host.Name
	runDir, err := r.prepareRun(record.ID)
	if err != nil {
		return record, err
	}
	err = r.executeTool(ctx, tool, finalValues, finalParams, runDir, executionHostID, out, errOut)
	finishRecord(record, err)
	if saveErr := r.saveRecord(runDir, record); saveErr != nil && err == nil {
		err = saveErr
	}
	return record, err
}

func (r *Runner) StartToolValues(ctx context.Context, id string, params map[string]interface{}, out, errOut io.Writer) (*RunRecord, error) {
	return r.StartToolValuesOn(ctx, id, "", params, out, errOut)
}

func (r *Runner) StartToolValuesOn(ctx context.Context, id, hostID string, params map[string]interface{}, out, errOut io.Writer) (*RunRecord, error) {
	return r.startToolValuesOn(ctx, id, hostID, "", params, out, errOut)
}

func (r *Runner) StartToolValuesOnTargets(ctx context.Context, id string, hostIDs []string, params map[string]interface{}, out, errOut io.Writer) (*BatchRun, error) {
	unique, batchID, err := r.prepareBatchTargets(id, hostIDs)
	if err != nil {
		return nil, err
	}
	batch := &BatchRun{BatchID: batchID, Runs: make([]*RunRecord, 0, len(unique)), Cancels: make(map[string]context.CancelFunc, len(unique))}
	for _, hostID := range unique {
		runCtx, cancel := context.WithCancel(ctx)
		record, err := r.startToolValuesOn(runCtx, id, hostID, batchID, params, out, errOut)
		if err != nil {
			cancel()
			for _, started := range batch.Cancels {
				started()
			}
			return batch, err
		}
		batch.Runs = append(batch.Runs, record)
		batch.Cancels[record.ID] = cancel
	}
	return batch, nil
}

// prepareBatchTargets 先校验工具和全部操作机，避免批量执行时部分主机已启动才发现目标无效。
func (r *Runner) prepareBatchTargets(id string, hostIDs []string) ([]string, string, error) {
	if _, err := r.Registry.Tool(id); err != nil {
		return nil, "", err
	}
	seen := make(map[string]struct{}, len(hostIDs))
	unique := make([]string, 0, len(hostIDs))
	for _, raw := range hostIDs {
		hostID := strings.TrimSpace(raw)
		if hostID == "" {
			return nil, "", fmt.Errorf("执行目标不能为空")
		}
		if _, ok := seen[hostID]; ok {
			continue
		}
		seen[hostID] = struct{}{}
		if hostID != LocalTargetID {
			if _, err := r.remoteHost(hostID); err != nil {
				return nil, "", err
			}
		}
		unique = append(unique, hostID)
	}
	if len(unique) == 0 {
		return nil, "", fmt.Errorf("未指定执行目标")
	}
	return unique, newBatchID(), nil
}

func (r *Runner) startToolValuesOn(ctx context.Context, id, hostID, batchID string, params map[string]interface{}, out, errOut io.Writer) (*RunRecord, error) {
	tool, err := r.Registry.Tool(id)
	if err != nil {
		return nil, err
	}
	host, executionHostID, err := r.resolveTargetHost(hostID)
	if err != nil {
		return nil, err
	}
	finalValues, finalParams, sensitivePaths, err := r.resolveToolValues(tool, params)
	if err != nil {
		return nil, err
	}
	record := newRecord("tool", id, config.RedactSensitive(finalValues, sensitivePaths))
	record.BatchID = batchID
	record.HostID = host.ID
	record.HostName = host.Name
	runDir, err := r.prepareRun(record.ID)
	if err != nil {
		return record, err
	}
	if err := r.saveRecord(runDir, record); err != nil {
		return record, err
	}
	response := *record
	go func() {
		runErr := r.executeTool(ctx, tool, finalValues, finalParams, runDir, executionHostID, out, errOut)
		finishRecord(record, runErr)
		_ = r.saveRecord(runDir, record)
	}()
	return &response, nil
}

func (r *Runner) resolveTargetHost(hostID string) (RemoteHost, string, error) {
	if strings.TrimSpace(hostID) == LocalTargetID {
		return RemoteHost{ID: LocalTargetID, Name: "本机"}, "", nil
	}
	host, err := r.remoteHost(hostID)
	return host, hostID, err
}

func (r *Runner) remoteHost(hostID string) (RemoteHost, error) {
	if strings.TrimSpace(hostID) == "" {
		return RemoteHost{}, nil
	}
	if r.remote == nil {
		return RemoteHost{}, fmt.Errorf("当前运行器未配置远程执行能力")
	}
	return r.remote.DescribeHost(hostID)
}

func (r *Runner) RunWorkflow(ctx context.Context, id string, params map[string]string, out, errOut io.Writer) (*RunRecord, error) {
	return r.RunWorkflowWithConfirmation(ctx, id, params, false, out, errOut)
}

func (r *Runner) RunWorkflowWithConfirmation(ctx context.Context, id string, params map[string]string, confirmed bool, out, errOut io.Writer) (*RunRecord, error) {
	wf, err := r.Registry.Workflow(id)
	if err != nil {
		return nil, err
	}
	return r.RunWorkflowConfigWithConfirmation(ctx, wf.Config, params, confirmed, out, errOut)
}

func (r *Runner) RunWorkflowConfigWithConfirmation(ctx context.Context, wf *config.WorkflowConfig, params map[string]string, confirmed bool, out, errOut io.Writer) (*RunRecord, error) {
	return r.RunWorkflowConfigWithUploads(ctx, wf, params, confirmed, nil, out, errOut)
}

func (r *Runner) RunWorkflowConfigWithUploads(ctx context.Context, wf *config.WorkflowConfig, params map[string]string, confirmed bool, uploads map[string]config.WorkflowUploadResult, out, errOut io.Writer) (*RunRecord, error) {
	record, finalParams, runDir, err := r.prepareWorkflowRun(wf, params)
	if err != nil {
		return record, err
	}
	return r.runWorkflowPrepared(ctx, wf, finalParams, confirmed, uploads, out, errOut, record, runDir)
}

func (r *Runner) StartWorkflowConfigWithConfirmation(ctx context.Context, wf *config.WorkflowConfig, params map[string]string, confirmed bool, out, errOut io.Writer) (*RunRecord, error) {
	return r.StartWorkflowConfigWithUploads(ctx, wf, params, confirmed, nil, out, errOut)
}

func (r *Runner) StartWorkflowConfigWithUploads(ctx context.Context, wf *config.WorkflowConfig, params map[string]string, confirmed bool, uploads map[string]config.WorkflowUploadResult, out, errOut io.Writer) (*RunRecord, error) {
	record, finalParams, runDir, err := r.prepareWorkflowRun(wf, params)
	if err != nil {
		return record, err
	}
	if err := r.saveRecord(runDir, record); err != nil {
		return record, err
	}
	response := *record
	go func() {
		_, _ = r.runWorkflowPrepared(ctx, wf, finalParams, confirmed, uploads, out, errOut, record, runDir)
	}()
	return &response, nil
}

func (r *Runner) prepareWorkflowRun(wf *config.WorkflowConfig, params map[string]string) (*RunRecord, map[string]string, string, error) {
	if wf == nil {
		return nil, nil, "", fmt.Errorf("工作流不能为空")
	}
	config.NormalizeWorkflow(wf)
	finalParams := config.MergeParams(wf.Parameters, nil, params)
	if err := config.ValidateRequired(wf.Parameters, finalParams); err != nil {
		return nil, nil, "", err
	}
	record := newRecord("workflow", wf.ID, config.StringMapToValues(finalParams))
	runDir, err := r.prepareRun(record.ID)
	if err != nil {
		return record, nil, "", err
	}
	record.Snapshot = newRunSnapshot(wf, finalParams)
	r.captureRunSnapshotBindings(record.Snapshot)
	if err := writeRunSnapshot(runDir, record.Snapshot); err != nil {
		return record, nil, "", err
	}
	return record, finalParams, runDir, nil
}

func (r *Runner) runWorkflowPrepared(ctx context.Context, wf *config.WorkflowConfig, finalParams map[string]string, confirmed bool, uploads map[string]config.WorkflowUploadResult, out, errOut io.Writer, record *RunRecord, runDir string) (*RunRecord, error) {
	_ = r.saveRecord(runDir, record)
	ordered, err := registry.OrderWorkflow(wf)
	if err != nil {
		return record, r.finalizeWorkflowRun(ctx, record, runDir, err)
	}
	if err := r.validateWorkflowConfirmations(ordered, confirmed); err != nil {
		return record, r.finalizeWorkflowRun(ctx, record, runDir, err)
	}
	workflowContext, active, nodeByID, legacyLoopTargets, edgesByFrom, err := prepareWorkflowExecutionState(wf, finalParams)
	if err != nil {
		return record, r.finalizeWorkflowRun(ctx, record, runDir, err)
	}
	err = r.executeWorkflowNodes(ctx, wf, ordered, finalParams, uploads, out, errOut, record, runDir, workflowContext, active, nodeByID, legacyLoopTargets, edgesByFrom, nil)
	return record, r.finalizeWorkflowRun(ctx, record, runDir, err)
}

// finalizeWorkflowRun 统一收敛终态、待处理标记和结束动作通知，保证恢复链路与普通运行一致。
func (r *Runner) finalizeWorkflowRun(ctx context.Context, record *RunRecord, runDir string, runErr error) error {
	applyPendingState(record, runErr)
	finishRecord(record, runErr)
	switch record.Status {
	case RunStatusPending:
		r.appendRecoveryNotification(ctx, record, config.WorkflowNotifyPending)
	case "succeeded":
		r.appendRecoveryNotification(ctx, record, config.WorkflowNotifySucceeded)
	case "failed":
		r.appendRecoveryNotification(ctx, record, config.WorkflowNotifyFailed)
	}
	if saveErr := r.saveRecord(runDir, record); saveErr != nil && runErr == nil {
		return saveErr
	}
	return runErr
}

func (r *Runner) RerunWorkflowNode(ctx context.Context, wf *config.WorkflowConfig, record *RunRecord, runDir, nodeID string, confirmed bool, out, errOut io.Writer) (*RunRecord, error) {
	return r.RerunWorkflowNodeWithParams(ctx, wf, record, runDir, nodeID, nil, confirmed, out, errOut)
}

func (r *Runner) RerunWorkflowNodeWithParams(ctx context.Context, wf *config.WorkflowConfig, record *RunRecord, runDir, nodeID string, params map[string]string, confirmed bool, out, errOut io.Writer) (*RunRecord, error) {
	if wf == nil {
		return record, fmt.Errorf("工作流不能为空")
	}
	if record == nil {
		return nil, fmt.Errorf("运行记录不能为空")
	}
	config.NormalizeWorkflow(wf)
	if record.Kind != "workflow" {
		return record, fmt.Errorf("运行记录不是工作流")
	}
	if record.Status == "running" {
		return record, fmt.Errorf("运行中的工作流不能重跑节点")
	}
	if record.Pending != nil || len(record.Attempts) > 0 {
		return record, fmt.Errorf("有恢复历史的运行不能直接重跑节点，请使用恢复入口或新建运行，保留原始失败证据")
	}
	if strings.TrimSpace(nodeID) == "" {
		return record, fmt.Errorf("节点 ID 必填")
	}
	finalParams := rerunWorkflowParams(wf, record, params)
	if err := config.ValidateRequired(wf.Parameters, finalParams); err != nil {
		return record, err
	}
	ordered, err := registry.OrderWorkflow(wf)
	if err != nil {
		return record, err
	}
	if err := r.validateWorkflowConfirmations(ordered, confirmed); err != nil {
		return record, err
	}
	nodeByID := workflowNodeMap(wf.Nodes)
	if _, ok := nodeByID[nodeID]; !ok {
		return record, fmt.Errorf("工作流中找不到节点 %s", nodeID)
	}
	affected := downstreamNodeSet(nodeID, wf.Edges)
	if len(wf.Stages) > 0 {
		// 后续阶段受屏障影响，当前并行阶段只重跑真实依赖与主机冲突的后继。
		includeLater := false
		for _, stage := range workflowStages(wf, ordered) {
			if includeLater {
				for _, node := range stage.nodes {
					affected[node.ID] = true
				}
				continue
			}
			if stage.id != nodeByID[nodeID].StageID {
				continue
			}
			if stage.mode == config.WorkflowStageExecutionParallel {
				inStage := map[string]bool{}
				for _, node := range stage.nodes {
					inStage[node.ID] = true
				}
				graph := buildStageDependencyGraph(stage.nodes, wf.Edges, inStage)
				if err := graph.addMutationConflicts(stage.nodes, r.changeNodeIDs(wf), r.serviceNodeIDs(wf)); err != nil {
					return record, err
				}
				queue := []string{nodeID}
				visited := map[string]bool{}
				for len(queue) > 0 {
					id := queue[0]
					queue = queue[1:]
					if visited[id] {
						continue
					}
					visited[id] = true
					affected[id] = true
					queue = append(queue, graph.children[id]...)
				}
			} else {
				for _, node := range stage.nodes {
					if node.Order >= nodeByID[nodeID].Order {
						affected[node.ID] = true
					}
				}
			}
			includeLater = true
		}
	}
	rerunUploads := reusableUploadsForNodes(runDir, wf.Nodes, affected)
	record.Steps = filterStepsOutside(record.Steps, affected)
	record.Backups = filterBackupsOutside(record.Backups, affected)
	for affectedNodeID := range affected {
		if err := os.RemoveAll(filepath.Join(runDir, affectedNodeID)); err != nil {
			return record, fmt.Errorf("清理节点 %s 旧日志失败: %w", affectedNodeID, err)
		}
	}
	workflowContext, active, nodeByID, legacyLoopTargets, edgesByFrom, err := prepareWorkflowExecutionState(wf, finalParams)
	if err != nil {
		return record, err
	}
	if err := r.rebuildWorkflowContextFromSteps(workflowContext, record.Steps, nodeByID, runDir, edgesByFrom, active); err != nil {
		return record, err
	}
	activateRerunEntry(nodeID, wf.Edges, active)
	record.Params = copyParams(finalParams)
	record.Config = config.StringMapToValues(finalParams)
	record.Status = "running"
	record.Error = ""
	record.EndedAt = time.Time{}
	record.Snapshot = newRunSnapshot(wf, finalParams)
	r.captureRunSnapshotBindings(record.Snapshot)
	_ = writeRunSnapshot(runDir, record.Snapshot)
	_ = r.saveRecord(runDir, record)
	err = r.executeWorkflowNodes(ctx, wf, ordered, finalParams, rerunUploads, out, errOut, record, runDir, workflowContext, active, nodeByID, legacyLoopTargets, edgesByFrom, affected)
	return record, r.finalizeWorkflowRun(ctx, record, runDir, err)
}

func rerunWorkflowParams(wf *config.WorkflowConfig, record *RunRecord, overrides map[string]string) map[string]string {
	base := recordConfigParams(record)
	if len(base) == 0 {
		base = copyParams(record.Params)
	}
	if overrides == nil {
		return base
	}
	return config.MergeParams(wf.Parameters, base, overrides)
}

func prepareWorkflowExecutionState(wf *config.WorkflowConfig, finalParams map[string]string) (map[string]string, map[string]bool, map[string]config.WorkflowNode, map[string]bool, workflowEdgeBuckets, error) {
	workflowContext := copyParams(finalParams)
	nodeByID := map[string]config.WorkflowNode{}
	legacyLoopTargets := map[string]bool{}
	for _, node := range wf.Nodes {
		nodeByID[node.ID] = node
		if workflowNodeType(node) == config.WorkflowNodeTypeLoop && node.Loop.Tool == "" && node.Loop.Target != "" {
			legacyLoopTargets[node.Loop.Target] = true
		}
	}
	edgesByFrom, incomingByTo := workflowEdges(wf.Edges)
	active := map[string]bool{}
	for _, node := range wf.Nodes {
		active[node.ID] = len(incomingByTo[node.ID]) == 0
	}
	return workflowContext, active, nodeByID, legacyLoopTargets, edgesByFrom, nil
}

func (r *Runner) executeWorkflowNodes(ctx context.Context, wf *config.WorkflowConfig, ordered []config.WorkflowNode, finalParams map[string]string, uploads map[string]config.WorkflowUploadResult, out, errOut io.Writer, record *RunRecord, runDir string, workflowContext map[string]string, active map[string]bool, nodeByID map[string]config.WorkflowNode, legacyLoopTargets map[string]bool, edgesByFrom workflowEdgeBuckets, executeOnly map[string]bool) error {
	exec := &workflowExec{
		workflowContext:    workflowContext,
		active:             active,
		nodeByID:           nodeByID,
		legacyLoopTargets:  legacyLoopTargets,
		edgesByFrom:        edgesByFrom,
		changeNodes:        r.changeNodeIDs(wf),
		serviceNodes:       r.serviceNodeIDs(wf),
		diffNodes:          r.diffNodeIDs(wf),
		skipChangeOnNoDiff: wf.Recovery != nil && wf.Recovery.SkipChangeOnNoDiff,
		record:             record,
		runDir:             runDir,
		saver:              r.saveRecord,
		rerun:              executeOnly != nil,
	}
	for _, stage := range workflowStages(wf, ordered) {
		stageNodes := make([]config.WorkflowNode, 0, len(stage.nodes))
		for _, node := range stage.nodes {
			if executeOnly == nil || executeOnly[node.ID] {
				stageNodes = append(stageNodes, node)
			}
		}
		var stageErr error
		if stage.mode == config.WorkflowStageExecutionParallel {
			stageErr = r.executeParallelStage(ctx, wf, stage, stageNodes, finalParams, uploads, out, errOut, exec)
		} else {
			stageErr = r.executeSerialNodes(ctx, wf, stageNodes, finalParams, uploads, out, errOut, exec)
		}
		if stageErr != nil {
			return stageErr
		}
	}
	return nil
}

// workflowStagePlan 是运行时展开的阶段视图：串行阶段按 order 形成隐式链，并行阶段只保留显式依赖。
type workflowStagePlan struct {
	id    string
	name  string
	mode  string
	limit int
	nodes []config.WorkflowNode
}

// workflowStages 把有序节点切成阶段。没有 stages 的旧工作流视为一个串行阶段，保持原有语义。
func workflowStages(wf *config.WorkflowConfig, ordered []config.WorkflowNode) []workflowStagePlan {
	if len(wf.Stages) == 0 {
		return []workflowStagePlan{{id: "", name: "原有流程", mode: config.WorkflowStageExecutionSerial, nodes: ordered}}
	}
	plans := make([]workflowStagePlan, 0, len(wf.Stages))
	index := map[string]int{}
	for _, stage := range wf.Stages {
		mode := stage.ExecutionMode
		if mode == "" {
			mode = config.WorkflowStageExecutionSerial
		}
		index[stage.ID] = len(plans)
		plans = append(plans, workflowStagePlan{id: stage.ID, name: stage.Name, mode: mode, limit: stage.MaxConcurrency})
	}
	for _, node := range ordered {
		position, ok := index[node.StageID]
		if !ok {
			continue
		}
		plans[position].nodes = append(plans[position].nodes, node)
	}
	return plans
}

// executeSerialNodes 保持串行阶段语义：按 order 依次执行，任一节点失败后不再派发后续节点。
// 阶段内跳过的条件分支会级联跳过它的下游；但同一节点仍可能由其他分支激活，因此跳过不阻断后续节点。
func (r *Runner) executeSerialNodes(ctx context.Context, wf *config.WorkflowConfig, nodes []config.WorkflowNode, finalParams map[string]string, uploads map[string]config.WorkflowUploadResult, out, errOut io.Writer, exec *workflowExec) error {
	for _, node := range nodes {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err := r.dispatchWorkflowNode(ctx, wf, node, finalParams, uploads, out, errOut, exec, false); err != nil {
			return err
		}
	}
	return nil
}

// dispatchWorkflowNode 执行单个节点并写入步骤记录。串行阶段由调用方保证顺序；
// 并行阶段可能有多个 goroutine 同时进入，记录追加、上下文和激活状态都要经过 exec 的锁。
func (r *Runner) dispatchWorkflowNode(ctx context.Context, wf *config.WorkflowConfig, node config.WorkflowNode, finalParams map[string]string, uploads map[string]config.WorkflowUploadResult, out, errOut io.Writer, exec *workflowExec, parallel bool) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		step := StepRecord{ID: node.ID, Type: workflowNodeType(node), Tool: node.Tool, StartedAt: time.Now(), EndedAt: time.Now()}
		markStepFailure(&step, ctxErr)
		exec.appendStep(step)
		return ctxErr
	}
	nodeType := workflowNodeType(node)
	if exec.legacyLoopTargets[node.ID] {
		exec.recordSkippedStep(node, nodeType, "循环目标工具由循环节点内嵌执行")
		return nil
	}
	exec.mu.Lock()
	activeNow := exec.active[node.ID]
	skipChange := exec.skipChanges && (exec.changeNodes[node.ID] || exec.serviceNodes[node.ID])
	contextSnapshot, _ := cloneWorkflowContext(exec.workflowContext)
	exec.mu.Unlock()
	if !activeNow {
		exec.recordSkippedStep(node, nodeType, "条件分支未激活")
		return nil
	}
	if skipChange {
		exec.recordSkippedStep(node, nodeType, "差异检查未发现变更，跳过配置下发与服务重启")
		exec.activate(node.ID, "", false)
		return nil
	}
	if nodeType == config.WorkflowNodeTypeCondition {
		stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "running", StartedAt: time.Now()}
		stepIndex := exec.appendStep(stepRecord)
		inputValue := renderTemplate(node.Condition.Input, contextSnapshot)
		matchedCase := matchConditionCase(inputValue, node.Condition)
		stepRecord.ConditionInput = inputValue
		stepRecord.MatchedCase = matchedCase
		stepRecord.EndedAt = time.Now()
		stepRecord.Status = "succeeded"
		exec.updateStep(stepIndex, stepRecord)
		exec.activate(node.ID, matchedCase, true)
		exec.writeContext(func(workflowContext map[string]string) {
			workflowContext["steps."+node.ID+".condition.input"] = inputValue
			workflowContext["steps."+node.ID+".condition.case"] = matchedCase
		})
		return nil
	}
	if nodeType == config.WorkflowNodeTypeParallel || nodeType == config.WorkflowNodeTypeJoin {
		// 旧 parallel/join 节点只做兼容标记：并行能力由阶段模式承担，节点本身不再分叉。
		stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "succeeded", StartedAt: time.Now(), EndedAt: time.Now()}
		exec.appendStep(stepRecord)
		exec.activate(node.ID, "", false)
		return nil
	}
	if nodeType == config.WorkflowNodeTypeUpload {
		if parallel && !hasUploadResult(uploads, node.ID) {
			message := fmt.Sprintf("上传节点 %s 不能放在并行阶段：上传结果需要在运行启动前确定", node.ID)
			stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "failed", StartedAt: time.Now(), EndedAt: time.Now(), Error: message}
			exec.appendStep(stepRecord)
			return errors.New(message)
		}
		if exec.rerun && !hasUploadResult(uploads, node.ID) {
			stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "failed", StartedAt: time.Now(), EndedAt: time.Now(), Error: fmt.Sprintf("上传节点 %s 缺少可复用的上传结果", node.ID)}
			exec.appendStep(stepRecord)
			return fmt.Errorf("上传节点 %s 缺少可复用的上传结果", node.ID)
		}
		status := "waiting"
		if hasUploadResult(uploads, node.ID) {
			status = "running"
		}
		stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: status, StartedAt: time.Now()}
		stepRunDir := filepath.Join(exec.runDir, node.ID)
		stepIndex := exec.appendStep(stepRecord)
		upload, uploadErr := waitForUploadNode(ctx, node, uploads, stepRunDir)
		stepRecord.Status = "running"
		exec.updateStep(stepIndex, stepRecord)
		if uploadErr == nil {
			if hasUploadResult(uploads, node.ID) {
				uploadErr = WriteWorkflowUploadResult(exec.runDir, node.ID, upload)
			}
		}
		if uploadErr == nil {
			uploadErr = executeUploadNode(node, upload, stepRunDir)
		}
		stepRecord.EndedAt = time.Now()
		if uploadErr != nil {
			uploadErr = workflowNodeError(ctx, node.ID, uploadErr)
			markStepFailure(&stepRecord, uploadErr)
			exec.updateStep(stepIndex, stepRecord)
			return exec.recoveryFailure(node, uploadErr)
		}
		exec.writeContext(func(workflowContext map[string]string) {
			addUploadContext(workflowContext, node.ID, upload, stepRunDir)
		})
		stepRecord.Status = "succeeded"
		exec.updateStep(stepIndex, stepRecord)
		exec.activate(node.ID, "", false)
		return nil
	}
	if nodeType == config.WorkflowNodeTypeExtractConfig {
		stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "running", StartedAt: time.Now()}
		stepRunDir := filepath.Join(exec.runDir, node.ID)
		stepIndex := exec.appendStep(stepRecord)
		extractErr := r.executeExtractConfigNode(wf, node, contextSnapshot, stepRunDir)
		stepRecord.EndedAt = time.Now()
		if extractErr != nil {
			extractErr = workflowNodeError(ctx, node.ID, extractErr)
			markStepFailure(&stepRecord, extractErr)
			exec.updateStep(stepIndex, stepRecord)
			return exec.recoveryFailure(node, extractErr)
		}
		exec.writeContext(func(workflowContext map[string]string) {
			addStepContext(workflowContext, node.ID, nil, stepRunDir, nil)
		})
		stepRecord.Status = "succeeded"
		exec.updateStep(stepIndex, stepRecord)
		exec.activate(node.ID, "", false)
		return nil
	}
	if nodeType == config.WorkflowNodeTypeHTTP {
		stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "running", StartedAt: time.Now()}
		stepRunDir := filepath.Join(exec.runDir, node.ID)
		stepIndex := exec.appendStep(stepRecord)
		meta, httpErr := r.executeHTTPNode(ctx, node, contextSnapshot, stepRunDir, out, errOut)
		stepRecord.HTTPStatus = meta.Status
		stepRecord.EndedAt = time.Now()
		if httpErr != nil {
			httpErr = workflowNodeError(ctx, node.ID, httpErr)
			markStepFailure(&stepRecord, httpErr)
			exec.updateStep(stepIndex, stepRecord)
			return exec.recoveryFailure(node, httpErr)
		}
		exec.writeContext(func(workflowContext map[string]string) {
			addStepContext(workflowContext, node.ID, nil, stepRunDir, nil)
			addHTTPContext(workflowContext, node.ID, meta)
		})
		stepRecord.Status = "succeeded"
		exec.updateStep(stepIndex, stepRecord)
		exec.activate(node.ID, "", false)
		return nil
	}
	if nodeType == config.WorkflowNodeTypeShell || nodeType == config.WorkflowNodeTypeChecksum || nodeType == config.WorkflowNodeTypeArchive {
		hostID := node.Host
		if nodeType == config.WorkflowNodeTypeArchive {
			hostID = "" // 解压器只处理平台本地文件，不能记录或连接远程主机。
		}
		host, hostErr := r.remoteHost(hostID)
		stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "running", StartedAt: time.Now(), HostID: host.ID, HostName: host.Name}
		stepRunDir := filepath.Join(exec.runDir, node.ID)
		stepIndex := exec.appendStep(stepRecord)
		builtinErr := hostErr
		if builtinErr == nil {
			switch nodeType {
			case config.WorkflowNodeTypeShell:
				builtinErr = r.executeShellNode(ctx, node, contextSnapshot, stepRunDir, hostID, out, errOut)
			case config.WorkflowNodeTypeChecksum:
				builtinErr = r.executeChecksumNode(ctx, node, contextSnapshot, stepRunDir, hostID, out, errOut)
			default:
				builtinErr = r.executeArchiveNode(ctx, node, contextSnapshot, stepRunDir)
			}
		}
		stepRecord.EndedAt = time.Now()
		if builtinErr != nil {
			builtinErr = workflowNodeError(ctx, node.ID, builtinErr)
			markStepFailure(&stepRecord, builtinErr)
			exec.updateStep(stepIndex, stepRecord)
			return exec.recoveryFailure(node, builtinErr)
		}
		exec.writeContext(func(workflowContext map[string]string) {
			addStepContext(workflowContext, node.ID, nil, stepRunDir, nil)
		})
		stepRecord.Status = "succeeded"
		exec.updateStep(stepIndex, stepRecord)
		exec.activate(node.ID, "", false)
		return nil
	}
	if nodeType == config.WorkflowNodeTypeWait {
		stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "running", StartedAt: time.Now()}
		stepRunDir := filepath.Join(exec.runDir, node.ID)
		stepIndex := exec.appendStep(stepRecord)
		waitErr := r.executeWaitNode(ctx, node, contextSnapshot, stepRunDir, out)
		stepRecord.EndedAt = time.Now()
		if waitErr != nil {
			waitErr = workflowNodeError(ctx, node.ID, waitErr)
			markStepFailure(&stepRecord, waitErr)
			exec.updateStep(stepIndex, stepRecord)
			return exec.recoveryFailure(node, waitErr)
		}
		exec.writeContext(func(workflowContext map[string]string) {
			addStepContext(workflowContext, node.ID, nil, stepRunDir, nil)
			addWaitContext(workflowContext, node.ID, stepRunDir)
		})
		stepRecord.Status = "succeeded"
		exec.updateStep(stepIndex, stepRecord)
		exec.activate(node.ID, "", false)
		return nil
	}
	if nodeType == config.WorkflowNodeTypeNotify {
		stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "running", StartedAt: time.Now()}
		stepRunDir := filepath.Join(exec.runDir, node.ID)
		stepIndex := exec.appendStep(stepRecord)
		_, notifyErr := r.executeNotifyNode(ctx, node, contextSnapshot, stepRunDir, out, errOut)
		stepRecord.EndedAt = time.Now()
		if notifyErr != nil {
			notifyErr = workflowNodeError(ctx, node.ID, notifyErr)
			markStepFailure(&stepRecord, notifyErr)
			exec.updateStep(stepIndex, stepRecord)
			return exec.recoveryFailure(node, notifyErr)
		}
		exec.writeContext(func(workflowContext map[string]string) {
			addStepContext(workflowContext, node.ID, nil, stepRunDir, nil)
		})
		stepRecord.Status = "succeeded"
		exec.updateStep(stepIndex, stepRecord)
		exec.activate(node.ID, "", false)
		return nil
	}
	if nodeType == config.WorkflowNodeTypeLoop {
		loopHostID := loopNodeHostID(node, exec.nodeByID)
		host, hostErr := r.remoteHost(loopHostID)
		stepRecord := StepRecord{ID: node.ID, Type: nodeType, Tool: loopToolID(node, exec.nodeByID), HostID: host.ID, HostName: host.Name, Status: "running", StartedAt: time.Now(), LoopTarget: node.Loop.Target, LoopIterations: node.Loop.MaxIterations}
		stepIndex := exec.appendStep(stepRecord)
		var loopErr error
		if hostErr != nil {
			loopErr = hostErr
		} else {
			exec.mu.Lock()
			loopContext, _ := cloneWorkflowContext(exec.workflowContext)
			exec.mu.Unlock()
			loopErr = r.executeLoop(ctx, node, exec.nodeByID, finalParams, loopContext, exec.runDir, loopHostID, out, errOut)
			if loopErr == nil {
				exec.writeContext(func(workflowContext map[string]string) {
					for key, value := range loopContext {
						if strings.HasPrefix(key, "steps."+node.ID+".") {
							workflowContext[key] = value
						}
					}
				})
			}
		}
		stepRecord.EndedAt = time.Now()
		if loopErr != nil {
			loopErr = workflowNodeError(ctx, node.ID, loopErr)
			markStepFailure(&stepRecord, loopErr)
			exec.updateStep(stepIndex, stepRecord)
			return exec.recoveryFailure(node, loopErr)
		}
		stepRecord.Status = "succeeded"
		exec.updateStep(stepIndex, stepRecord)
		exec.activate(node.ID, "", false)
		return nil
	}
	stepParams := resolveStepParams(finalParams, contextSnapshot, node.Params)
	host, hostErr := r.remoteHost(node.Host)
	tool, toolErr := r.Registry.Tool(node.Tool)
	if hostErr != nil {
		toolErr = hostErr
	}
	stepRecord := StepRecord{ID: node.ID, Type: nodeType, Tool: node.Tool, HostID: host.ID, HostName: host.Name, Status: "running", StartedAt: time.Now()}
	stepRunDir := filepath.Join(exec.runDir, node.ID)
	stepIndex := exec.appendStep(stepRecord)
	if toolErr == nil {
		stepValues, stepFlat, _, resolveErr := r.resolveToolValues(tool, config.StringMapToValues(stepParams))
		if resolveErr != nil {
			toolErr = resolveErr
		} else {
			stepParams = stepFlat
			toolErr = r.executeTool(ctx, tool, stepValues, stepFlat, stepRunDir, node.Host, out, errOut)
		}
	}
	stepRecord.EndedAt = time.Now()
	// 远端可能已经完成写入并输出备份证据，随后才报告取消或退出失败。
	// 先保存有效证据，避免并行失败取消兄弟节点时丢失恢复依据。
	if exec.changeNodes[node.ID] {
		exec.mu.Lock()
		recordBackupMetadata(exec.record, node, host, stepRunDir)
		exec.mu.Unlock()
	}
	if toolErr != nil {
		toolErr = workflowNodeError(ctx, node.ID, toolErr)
		markStepFailure(&stepRecord, toolErr)
		exec.updateStep(stepIndex, stepRecord)
		return exec.recoveryFailure(node, toolErr)
	}
	exec.writeContext(func(workflowContext map[string]string) {
		addStepContext(workflowContext, node.ID, stepParams, stepRunDir, tool.Config.Outputs)
	})
	exec.mu.Lock()
	if exec.skipChangeOnNoDiff && exec.diffNodes[node.ID] {
		exec.skipChanges = noDiffFromStep(exec.workflowContext, node.ID) || exec.skipChanges
	}
	exec.mu.Unlock()
	stepRecord.Status = "succeeded"
	exec.updateStep(stepIndex, stepRecord)
	exec.activate(node.ID, "", false)
	return nil
}

func workflowNodeError(ctx context.Context, nodeID string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("节点 %s 执行超时：%v", nodeID, err)
	}
	return err
}

func markStepFailure(step *StepRecord, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		step.Status = "cancelled"
		step.Error = "运行已取消"
		return
	}
	step.Status = "failed"
	step.Error = err.Error()
}

// workflowExec 汇集一次运行中所有节点共享的可变状态。并行阶段会并发调用 dispatchWorkflowNode，
// 因此工作流上下文、激活集合、步骤记录和跳过判定都必须在该锁内读写。
type workflowExec struct {
	mu                 sync.Mutex
	workflowContext    map[string]string
	active             map[string]bool
	nodeByID           map[string]config.WorkflowNode
	legacyLoopTargets  map[string]bool
	edgesByFrom        workflowEdgeBuckets
	changeNodes        map[string]bool
	serviceNodes       map[string]bool
	diffNodes          map[string]bool
	skipChangeOnNoDiff bool
	skipChanges        bool
	record             *RunRecord
	runDir             string
	saver              func(dir string, record *RunRecord) error
	rerun              bool
}

func (e *workflowExec) appendStep(step StepRecord) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.record.Steps = append(e.record.Steps, step)
	index := len(e.record.Steps) - 1
	_ = e.saveLocked()
	return index
}

func (e *workflowExec) updateStep(index int, step StepRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if index >= 0 && index < len(e.record.Steps) {
		e.record.Steps[index] = step
	}
	_ = e.saveLocked()
}

func (e *workflowExec) writeContext(write func(map[string]string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	write(e.workflowContext)
}

func (e *workflowExec) recordSkippedStep(node config.WorkflowNode, nodeType, reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.record.Steps = append(e.record.Steps, skippedStepRecord(node, nodeType, reason))
	_ = e.saveLocked()
}

func (e *workflowExec) save() {
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = e.saveLocked()
}

func (e *workflowExec) saveLocked() error {
	if e.saver == nil {
		return nil
	}
	return e.saver(e.runDir, e.record)
}

func (e *workflowExec) recoveryFailure(node config.WorkflowNode, err error) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return recoveryFailure(e.record, node, err)
}

// activate 只沿实际执行的边激活节点；跳过的分支不传播激活，也不撤销其他路径的合流。
func (e *workflowExec) activate(nodeID, matchedCase string, condition bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, edge := range e.edgesByFrom[nodeID] {
		if !condition || edge.Case == matchedCase {
			e.active[edge.To] = true
		}
	}
	_ = e.saveLocked()
}

func cloneWorkflowContext(source map[string]string) (map[string]string, bool) {
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out, true
}

func (e *workflowExec) nodeActive(nodeID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.active[nodeID]
}

// executeParallelStage 按显式依赖做有界并发调度。阶段内没有隐式顺序，因此：
//   - 只有全部上游显式依赖都到终态的节点才可派发；
//   - 条件节点执行后会把未命中的分支标记为未激活，这些节点直接记为跳过，不阻塞阶段屏障；
//   - 同主机的变更/服务节点沿显式依赖的稳定拓扑序串成链，避免并发写入且不引入环；
//   - 任一节点失败后停止派发新节点，等待已开始的节点收敛；未派发节点记为跳过，阶段结束即屏障。
func (r *Runner) executeParallelStage(ctx context.Context, wf *config.WorkflowConfig, stage workflowStagePlan, nodes []config.WorkflowNode, finalParams map[string]string, uploads map[string]config.WorkflowUploadResult, out, errOut io.Writer, exec *workflowExec) error {
	if len(nodes) == 0 {
		return nil
	}
	limit := stage.limit
	if limit <= 0 {
		limit = defaultStageConcurrency
	}
	inStage := map[string]bool{}
	for _, node := range nodes {
		inStage[node.ID] = true
	}
	graph := buildStageDependencyGraph(nodes, wf.Edges, inStage)
	if err := graph.addMutationConflicts(nodes, exec.changeNodes, exec.serviceNodes); err != nil {
		return fmt.Errorf("阶段 %s 的依赖无法调度：%w", stage.id, err)
	}
	remaining := map[string]int{}
	for _, node := range nodes {
		remaining[node.ID] = len(graph.blockers[node.ID])
	}
	// 每个子节点从派发前就继承阶段取消信号，不依赖工作 goroutine 注册句柄。
	stageCtx, cancelStage := context.WithCancel(ctx)
	defer cancelStage()
	outputMu := &sync.Mutex{}
	out = stageOutputWriter{mu: outputMu, writer: out}
	errOut = stageOutputWriter{mu: outputMu, writer: errOut}
	type stageResult struct {
		id  string
		err error
	}
	finished := make(chan stageResult, len(nodes))
	done := map[string]bool{}
	started := map[string]bool{}
	var firstErr error
	var failedNode config.WorkflowNode
	running := 0
	for {
		if ctxErr := ctx.Err(); ctxErr != nil && firstErr == nil {
			firstErr = ctxErr
		}
		progress := false
		// 先收敛所有已可判定跳过的节点，再受并发上限约束派发；否则被跳过的上游没释放，下游会一直等不到就绪。
		for _, node := range nodes {
			if started[node.ID] || done[node.ID] || remaining[node.ID] > 0 {
				continue
			}
			if firstErr != nil || !exec.nodeActive(node.ID) {
				reason := "条件分支未激活"
				if firstErr != nil {
					reason = "运行已取消或阶段内已有节点失败，未派发"
				}
				r.dispatchStageSkip(exec, node, reason)
				done[node.ID] = true
				r.releaseStageNode(node.ID, graph, remaining)
				progress = true
			}
		}
		for _, node := range nodes {
			if running >= limit {
				break
			}
			if started[node.ID] || done[node.ID] {
				continue
			}
			if firstErr != nil || remaining[node.ID] > 0 {
				continue
			}
			started[node.ID] = true
			progress = true
			running++
			go func(node config.WorkflowNode) {
				err := r.dispatchWorkflowNode(stageCtx, wf, node, finalParams, uploads, out, errOut, exec, true)
				finished <- stageResult{id: node.ID, err: err}
			}(node)
		}
		if running == 0 {
			if progress && len(done) < len(nodes) {
				continue
			}
			if len(done) != len(nodes) && firstErr == nil {
				firstErr = fmt.Errorf("阶段 %s 仍有节点未满足依赖，无法继续执行", stage.id)
			}
			break
		}
		result := <-finished
		running--
		done[result.id] = true
		r.releaseStageNode(result.id, graph, remaining)
		if result.err != nil && firstErr == nil {
			firstErr = result.err
			failedNode = exec.nodeByID[result.id]
			cancelStage()
		}
	}
	// 没有真正执行过的节点无论因为条件未命中、上游失败还是取消，都要沿依赖继续传播未激活，
	// 否则后续阶段的节点只会看到“上游没成功”，既无法解释也无法被条件分支正确跳过。
	for _, node := range nodes {
		if !done[node.ID] && !started[node.ID] {
			r.dispatchStageSkip(exec, node, "阶段内上游失败或取消，未执行")
			exec.activate(node.ID, "", false)
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if firstErr != nil && failedNode.ID != "" {
		// 并行兄弟节点可能在失败被报告之后才写入备份，收敛后重新判定恢复入口。
		return exec.recoveryFailure(failedNode, firstErr)
	}
	return firstErr
}

type stageOutputWriter struct {
	mu     *sync.Mutex
	writer io.Writer
}

func (w stageOutputWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writer == nil {
		return len(data), nil
	}
	return w.writer.Write(data)
}

const defaultStageConcurrency = 4

func (r *Runner) dispatchStageSkip(exec *workflowExec, node config.WorkflowNode, reason string) {
	exec.recordSkippedStep(node, workflowNodeType(node), reason)
}

func (r *Runner) releaseStageNode(nodeID string, graph *stageDependencyGraph, remaining map[string]int) {
	for _, child := range graph.children[nodeID] {
		if remaining[child] > 0 {
			remaining[child]--
		}
	}
}

// stageDependencyGraph 只为并行阶段构建：显式依赖 + 同主机变更冲突边。
type stageDependencyGraph struct {
	blockers map[string][]string
	children map[string][]string
}

func buildStageDependencyGraph(nodes []config.WorkflowNode, edges []config.WorkflowEdge, inStage map[string]bool) *stageDependencyGraph {
	graph := &stageDependencyGraph{blockers: map[string][]string{}, children: map[string][]string{}}
	for _, node := range nodes {
		graph.blockers[node.ID] = nil
	}
	for _, edge := range edges {
		if !inStage[edge.From] || !inStage[edge.To] {
			continue
		}
		graph.blockers[edge.To] = append(graph.blockers[edge.To], edge.From)
		graph.children[edge.From] = append(graph.children[edge.From], edge.To)
	}
	return graph
}

// addMutationConflicts 把同主机的变更/服务节点按稳定拓扑顺序串成链。
// 只约束显式标记为变更或服务的节点，不推断任意 Shell 的副作用。
func (graph *stageDependencyGraph) addMutationConflicts(nodes []config.WorkflowNode, changeNodes, serviceNodes map[string]bool) error {
	candidates := append([]config.WorkflowNode(nil), nodes...)
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Order < candidates[j].Order })
	remaining := map[string]int{}
	for _, node := range nodes {
		remaining[node.ID] = len(graph.blockers[node.ID])
	}
	ordered := make([]config.WorkflowNode, 0, len(nodes))
	visited := map[string]bool{}
	for len(ordered) < len(nodes) {
		found := false
		for _, node := range candidates {
			if visited[node.ID] || remaining[node.ID] > 0 {
				continue
			}
			visited[node.ID] = true
			ordered = append(ordered, node)
			for _, child := range graph.children[node.ID] {
				remaining[child]--
			}
			found = true
			break
		}
		if !found {
			return fmt.Errorf("阶段内的依赖存在环")
		}
	}
	lastByHost := map[string]string{}
	for _, node := range ordered {
		if !changeNodes[node.ID] && !serviceNodes[node.ID] {
			continue
		}
		host := strings.TrimSpace(node.Host)
		if host == LocalTargetID {
			host = ""
		}
		if previous := lastByHost[host]; previous != "" {
			graph.blockers[node.ID] = append(graph.blockers[node.ID], previous)
			graph.children[previous] = append(graph.children[previous], node.ID)
		}
		lastByHost[host] = node.ID
	}
	return nil
}

func appendStepRecord(record *RunRecord, step StepRecord) int {
	record.Steps = append(record.Steps, step)
	return len(record.Steps) - 1
}

func recordConfigParams(record *RunRecord) map[string]string {
	if record == nil || len(record.Config) == 0 {
		return nil
	}
	return config.ValuesToStringMap(config.InterfaceMapToValues(record.Config))
}

func workflowNodeMap(nodes []config.WorkflowNode) map[string]config.WorkflowNode {
	out := map[string]config.WorkflowNode{}
	for _, node := range nodes {
		out[node.ID] = node
	}
	return out
}

func downstreamNodeSet(nodeID string, edges []config.WorkflowEdge) map[string]bool {
	out := map[string]bool{nodeID: true}
	queue := []string{nodeID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range edges {
			if edge.From != current || out[edge.To] {
				continue
			}
			out[edge.To] = true
			queue = append(queue, edge.To)
		}
	}
	return out
}

func filterStepsOutside(steps []StepRecord, excluded map[string]bool) []StepRecord {
	out := make([]StepRecord, 0, len(steps))
	for _, step := range steps {
		if excluded[step.ID] {
			continue
		}
		out = append(out, step)
	}
	return out
}

func activateRerunEntry(nodeID string, edges []config.WorkflowEdge, active map[string]bool) {
	active[nodeID] = true
	for _, edge := range edges {
		if edge.To != nodeID {
			continue
		}
		active[edge.To] = true
	}
}

func (r *Runner) rebuildWorkflowContextFromSteps(context map[string]string, steps []StepRecord, nodes map[string]config.WorkflowNode, runDir string, edgesByFrom workflowEdgeBuckets, active map[string]bool) error {
	for _, step := range steps {
		if step.Status != "succeeded" {
			continue
		}
		node, ok := nodes[step.ID]
		if !ok {
			continue
		}
		switch step.Type {
		case config.WorkflowNodeTypeCondition:
			context["steps."+step.ID+".condition.input"] = step.ConditionInput
			context["steps."+step.ID+".condition.case"] = step.MatchedCase
			activateConditionBranches(step.ID, step.MatchedCase, edgesByFrom, active)
		case config.WorkflowNodeTypeUpload:
			upload, err := readWorkflowUploadResult(filepath.Join(runDir, step.ID, workflowUploadResultFile))
			if err != nil {
				return fmt.Errorf("重建上传节点 %s 上下文失败: %w", step.ID, err)
			}
			addUploadContext(context, step.ID, upload, filepath.Join(runDir, step.ID))
			activatePlainBranches(step.ID, edgesByFrom, active)
		case config.WorkflowNodeTypeLoop:
			addLoopContext(context, step.ID, node.Loop.MaxIterations, runDir)
			activatePlainBranches(step.ID, edgesByFrom, active)
		case config.WorkflowNodeTypeHTTP:
			meta, metaErr := readHTTPResponseMeta(filepath.Join(runDir, step.ID, httpResponseMetaFile))
			if metaErr != nil {
				meta.Status = step.HTTPStatus
			}
			addStepContext(context, step.ID, nil, filepath.Join(runDir, step.ID), nil)
			addHTTPContext(context, step.ID, meta)
			activatePlainBranches(step.ID, edgesByFrom, active)
		case config.WorkflowNodeTypeWait:
			addStepContext(context, step.ID, nil, filepath.Join(runDir, step.ID), nil)
			addWaitContext(context, step.ID, filepath.Join(runDir, step.ID))
			activatePlainBranches(step.ID, edgesByFrom, active)
		default:
			outputs := r.outputsForWorkflowStep(node)
			addStepContext(context, step.ID, nil, filepath.Join(runDir, step.ID), outputs)
			activatePlainBranches(step.ID, edgesByFrom, active)
		}
	}
	return nil
}

func (r *Runner) outputsForWorkflowStep(node config.WorkflowNode) []config.ToolOutput {
	if workflowNodeType(node) != config.WorkflowNodeTypeTool || node.Tool == "" || r == nil || r.Registry == nil {
		return nil
	}
	tool, err := r.Registry.Tool(node.Tool)
	if err != nil {
		return nil
	}
	return tool.Config.Outputs
}

func reusableUploadsForNodes(runDir string, nodes []config.WorkflowNode, affected map[string]bool) map[string]config.WorkflowUploadResult {
	uploads := map[string]config.WorkflowUploadResult{}
	for _, node := range nodes {
		if !affected[node.ID] || workflowNodeType(node) != config.WorkflowNodeTypeUpload {
			continue
		}
		upload, err := readWorkflowUploadResult(filepath.Join(runDir, node.ID, workflowUploadResultFile))
		if err != nil {
			continue
		}
		uploads[node.ID] = upload
	}
	return uploads
}

func (r *Runner) validateWorkflowConfirmations(nodes []config.WorkflowNode, confirmed bool) error {
	for _, node := range nodes {
		nodeType := workflowNodeType(node)
		toolID := node.Tool
		if nodeType == config.WorkflowNodeTypeLoop {
			toolID = node.Loop.Tool
		}
		if nodeType != config.WorkflowNodeTypeTool && nodeType != config.WorkflowNodeTypeLoop {
			continue
		}
		if toolID == "" {
			continue
		}
		tool, err := r.Registry.Tool(toolID)
		if err != nil {
			return err
		}
		if tool.Config.Confirm.Required && !node.Confirm && !confirmed {
			return fmt.Errorf("工作流节点 %s 引用的工具 %s 需要确认", node.ID, toolID)
		}
	}
	return nil
}

type workflowEdgeBuckets map[string][]config.WorkflowEdge

func workflowEdges(edges []config.WorkflowEdge) (workflowEdgeBuckets, map[string][]config.WorkflowEdge) {
	byFrom := workflowEdgeBuckets{}
	byTo := map[string][]config.WorkflowEdge{}
	for _, edge := range edges {
		byFrom[edge.From] = append(byFrom[edge.From], edge)
		byTo[edge.To] = append(byTo[edge.To], edge)
	}
	return byFrom, byTo
}

func workflowNodeType(node config.WorkflowNode) string {
	if node.Type != "" {
		return node.Type
	}
	if node.Tool != "" {
		return config.WorkflowNodeTypeTool
	}
	if node.Condition.Input != "" || len(node.Condition.Cases) > 0 || node.Condition.DefaultCase != "" {
		return config.WorkflowNodeTypeCondition
	}
	if node.Loop.Tool != "" || node.Loop.Target != "" || node.Loop.MaxIterations != 0 || len(node.Loop.Params) > 0 {
		return config.WorkflowNodeTypeLoop
	}
	if node.Upload.TargetDir != "" {
		return config.WorkflowNodeTypeUpload
	}
	if node.Extract.SourceType != "" || node.Extract.FileName != "" || node.Extract.SourceNode != "" || node.Extract.SourceDir != "" || node.Extract.SourcePath != "" || node.Extract.TargetPath != "" || node.Extract.Label != "" || node.Extract.Replace || len(node.Extract.Files) > 0 {
		return config.WorkflowNodeTypeExtractConfig
	}
	if strings.TrimSpace(node.HTTP.URL) != "" || strings.TrimSpace(node.HTTP.Method) != "" || len(node.HTTP.Headers) > 0 || strings.TrimSpace(node.HTTP.Body) != "" || strings.TrimSpace(node.HTTP.SaveResponse) != "" || len(node.HTTP.ExpectStatus) > 0 {
		return config.WorkflowNodeTypeHTTP
	}
	if strings.TrimSpace(node.Script.Script) != "" {
		return config.WorkflowNodeTypeShell
	}
	if strings.TrimSpace(node.Checksum.Path) != "" {
		return config.WorkflowNodeTypeChecksum
	}
	if strings.TrimSpace(node.Archive.Path) != "" || strings.TrimSpace(node.Archive.TargetDir) != "" {
		return config.WorkflowNodeTypeArchive
	}
	if node.Wait.Seconds > 0 || strings.TrimSpace(node.Wait.PollURL) != "" || strings.TrimSpace(node.Wait.PollFile) != "" {
		return config.WorkflowNodeTypeWait
	}
	return ""
}

func skippedStepRecord(node config.WorkflowNode, nodeType, reason string) StepRecord {
	now := time.Now()
	return StepRecord{ID: node.ID, Type: nodeType, Tool: node.Tool, Status: "skipped", StartedAt: now, EndedAt: now, SkippedReason: reason}
}

func activatePlainBranches(nodeID string, edges workflowEdgeBuckets, active map[string]bool) {
	for _, edge := range edges[nodeID] {
		active[edge.To] = true
	}
}

func activateConditionBranches(nodeID, matchedCase string, edges workflowEdgeBuckets, active map[string]bool) {
	for _, edge := range edges[nodeID] {
		if edge.Case == matchedCase {
			active[edge.To] = true
		}
	}
}

func matchConditionCase(input string, condition config.WorkflowCondition) string {
	for _, item := range condition.Cases {
		if evaluateConditionCase(input, item) {
			return item.ID
		}
	}
	if condition.DefaultCase != "" {
		return condition.DefaultCase
	}
	return ""
}

func evaluateConditionCase(input string, item config.ConditionCase) bool {
	switch item.Operator {
	case "eq":
		return len(item.Values) > 0 && input == item.Values[0]
	case "neq":
		return len(item.Values) == 0 || input != item.Values[0]
	case "contains":
		return anyValue(item.Values, func(value string) bool { return strings.Contains(input, value) })
	case "not_contains":
		return !anyValue(item.Values, func(value string) bool { return strings.Contains(input, value) })
	case "in":
		return anyValue(item.Values, func(value string) bool { return input == value })
	case "not_in":
		return !anyValue(item.Values, func(value string) bool { return input == value })
	case "exists":
		return input != ""
	case "empty":
		return input == ""
	default:
		return false
	}
}

func anyValue(values []string, match func(string) bool) bool {
	for _, value := range values {
		if match(value) {
			return true
		}
	}
	return false
}
func (r *Runner) executeLoop(ctx context.Context, node config.WorkflowNode, nodes map[string]config.WorkflowNode, finalParams, workflowContext map[string]string, runDir, hostID string, out, errOut io.Writer) error {
	toolID := loopToolID(node, nodes)
	tool, err := r.Registry.Tool(toolID)
	if err != nil {
		return err
	}
	loopParams := node.Loop.Params
	if len(loopParams) == 0 && node.Loop.Target != "" {
		if target, ok := nodes[node.Loop.Target]; ok {
			loopParams = target.Params
		}
	}
	for iteration := 1; iteration <= node.Loop.MaxIterations; iteration++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		stepParams := resolveStepParams(finalParams, workflowContext, loopParams)
		stepRunDir := filepath.Join(runDir, node.ID, fmt.Sprintf("%d", iteration))
		stepValues, stepFlat, _, resolveErr := r.resolveToolValues(tool, config.StringMapToValues(stepParams))
		if resolveErr != nil {
			return fmt.Errorf("循环节点 %s 第 %d 次解析工具 %s 配置失败: %w", node.ID, iteration, toolID, resolveErr)
		}
		stepParams = stepFlat
		if err := r.executeTool(ctx, tool, stepValues, stepFlat, stepRunDir, hostID, out, errOut); err != nil {
			return fmt.Errorf("循环节点 %s 第 %d 次执行工具 %s 失败: %w", node.ID, iteration, toolID, err)
		}
		addStepContext(workflowContext, fmt.Sprintf("%s.%d", node.ID, iteration), stepParams, stepRunDir, tool.Config.Outputs)
		addStepContext(workflowContext, node.ID, stepParams, stepRunDir, tool.Config.Outputs)
		writeLoopAggregateLogs(runDir, node.ID, node.Loop.MaxIterations)
	}
	return nil
}

func writeLoopAggregateLogs(runDir, nodeID string, maxIterations int) {
	nodeRunDir := filepath.Join(runDir, nodeID)
	if err := os.MkdirAll(nodeRunDir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(nodeRunDir, "stdout.log"), []byte(readLoopText(runDir, nodeID, "stdout.log", maxIterations)), 0o644)
	_ = os.WriteFile(filepath.Join(nodeRunDir, "stderr.log"), []byte(readLoopText(runDir, nodeID, "stderr.log", maxIterations)), 0o644)
}

func loopToolID(node config.WorkflowNode, nodes map[string]config.WorkflowNode) string {
	if node.Loop.Tool != "" {
		return node.Loop.Tool
	}
	if node.Loop.Target != "" {
		return nodes[node.Loop.Target].Tool
	}
	return ""
}

func loopNodeHostID(node config.WorkflowNode, nodes map[string]config.WorkflowNode) string {
	if node.Host != "" {
		return node.Host
	}
	if node.Loop.Tool == "" && node.Loop.Target != "" {
		return nodes[node.Loop.Target].Host
	}
	return ""
}

func hasUploadResult(uploads map[string]config.WorkflowUploadResult, nodeID string) bool {
	if uploads == nil {
		return false
	}
	upload, ok := uploads[nodeID]
	return ok && upload.ID != ""
}

func waitForUploadNode(ctx context.Context, node config.WorkflowNode, uploads map[string]config.WorkflowUploadResult, runDir string) (config.WorkflowUploadResult, error) {
	if uploads != nil {
		if upload, ok := uploads[node.ID]; ok {
			return upload, nil
		}
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return config.WorkflowUploadResult{}, err
	}
	appendUploadStdout(runDir, fmt.Sprintf("WAIT 上传节点 %s 等待选择的文件上传到平台", node.ID))
	resultPath := filepath.Join(runDir, workflowUploadResultFile)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		upload, err := readWorkflowUploadResult(resultPath)
		if err == nil {
			return upload, nil
		}
		if !os.IsNotExist(err) {
			return config.WorkflowUploadResult{}, fmt.Errorf("上传节点 %s 读取上传结果失败: %w", node.ID, err)
		}
		select {
		case <-ctx.Done():
			return config.WorkflowUploadResult{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func readWorkflowUploadResult(path string) (config.WorkflowUploadResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config.WorkflowUploadResult{}, err
	}
	var upload config.WorkflowUploadResult
	if err := json.Unmarshal(data, &upload); err != nil {
		return config.WorkflowUploadResult{}, err
	}
	return upload, nil
}

func WriteWorkflowUploadResult(runDir, nodeID string, upload config.WorkflowUploadResult) error {
	stepDir := filepath.Join(runDir, nodeID)
	if err := os.MkdirAll(stepDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(upload, "", "  ")
	if err != nil {
		return err
	}
	tempPath := filepath.Join(stepDir, workflowUploadResultFile+".tmp")
	resultPath := filepath.Join(stepDir, workflowUploadResultFile)
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}
	if err := appendUploadStdout(stepDir, fmt.Sprintf("LOG 上传节点 %s 已接收上传文件：%s，文件数：%d，总大小：%d bytes", nodeID, upload.FileName, upload.Count, upload.TotalSize)); err != nil {
		return err
	}
	for index, item := range normalizedUploadFiles(upload) {
		if err := appendUploadStdout(stepDir, fmt.Sprintf("LOG 上传文件 %d：%s -> %s (%d bytes)", index+1, item.FileName, item.RelativePath, item.Size)); err != nil {
			return err
		}
	}
	return os.Rename(tempPath, resultPath)
}

func addLoopContext(context map[string]string, nodeID string, maxIterations int, runDir string) {
	context["steps."+nodeID+".loop.iterations"] = fmt.Sprint(maxIterations)
	context["steps."+nodeID+".stdout"] = strings.TrimSpace(readLoopText(runDir, nodeID, "stdout.log", maxIterations))
	context["steps."+nodeID+".stderr"] = strings.TrimSpace(readLoopText(runDir, nodeID, "stderr.log", maxIterations))
}

func executeUploadNode(node config.WorkflowNode, upload config.WorkflowUploadResult, runDir string) error {
	if upload.ID == "" || upload.Path == "" || upload.FileName == "" || upload.Count == 0 && len(upload.Files) == 0 {
		runErr := fmt.Errorf("上传节点 %s 缺少上传文件", node.ID)
		_ = writeUploadStderr(runDir, runErr)
		return runErr
	}
	data, err := json.Marshal(upload)
	if err != nil {
		runErr := fmt.Errorf("上传节点 %s 生成上传结果失败: %w", node.ID, err)
		_ = writeUploadStderr(runDir, runErr)
		return runErr
	}
	if err := appendUploadStdout(runDir, fmt.Sprintf("SUCCESS 上传节点 %s 上传完成，平台路径：%s", node.ID, upload.RelativePath)); err != nil {
		return err
	}
	if upload.Path != "" {
		if err := appendUploadStdout(runDir, fmt.Sprintf("DIR 上传目录（绝对路径）：%s", filepath.Dir(upload.Path))); err != nil {
			return err
		}
	}
	if upload.RelativePath != "" {
		if err := appendUploadStdout(runDir, fmt.Sprintf("DIR 上传目录（相对路径）：%s", filepath.ToSlash(filepath.Dir(upload.RelativePath)))); err != nil {
			return err
		}
	}
	if err := appendUploadStdout(runDir, "JSON "+string(data)); err != nil {
		return err
	}
	return writeUploadStderr(runDir, nil)
}

func writeUploadLogs(runDir string, stdout []byte, runErr error) error {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	if stdout == nil {
		stdout = []byte{}
	}
	if len(stdout) > 0 && stdout[len(stdout)-1] != '\n' {
		stdout = append(stdout, '\n')
	}
	if err := os.WriteFile(filepath.Join(runDir, "stdout.log"), stdout, 0o644); err != nil {
		return err
	}
	stderr := []byte{}
	if runErr != nil {
		stderr = []byte(runErr.Error() + "\n")
	}
	return os.WriteFile(filepath.Join(runDir, "stderr.log"), stderr, 0o644)
}

func appendUploadStdout(runDir, line string) error {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	file, err := os.OpenFile(filepath.Join(runDir, "stdout.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(line)
	return err
}

func writeUploadStderr(runDir string, runErr error) error {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	stderr := []byte{}
	if runErr != nil {
		stderr = []byte(runErr.Error() + "\n")
	}
	return os.WriteFile(filepath.Join(runDir, "stderr.log"), stderr, 0o644)
}

func (r *Runner) executeExtractConfigNode(wf *config.WorkflowConfig, node config.WorkflowNode, workflowContext map[string]string, runDir string) error {
	uploads, err := workflowUploadResults(workflowContext)
	if err != nil {
		runErr := fmt.Errorf("提取配置节点 %s 读取上传结果失败: %w", node.ID, err)
		_ = writeUploadStderr(runDir, runErr)
		return runErr
	}
	if len(uploads) == 0 {
		err := fmt.Errorf("提取配置节点 %s 找不到可用的上传结果", node.ID)
		_ = writeUploadStderr(runDir, err)
		return err
	}
	items, err := extractConfigItems(node.Extract)
	if err != nil {
		runErr := fmt.Errorf("提取配置节点 %s %w", node.ID, err)
		_ = writeUploadStderr(runDir, runErr)
		return runErr
	}
	if len(items) == 0 {
		err := fmt.Errorf("提取配置节点 %s 缺少可提取的配置项", node.ID)
		_ = writeUploadStderr(runDir, err)
		return err
	}
	for _, item := range items {
		sourceName := renderTemplate(item.FileName, workflowContext)
		if sourceName == "" && item.SourcePath != "" {
			sourceName = renderTemplate(item.SourcePath, workflowContext)
		}
		if sourceName == "" {
			runErr := fmt.Errorf("提取配置节点 %s 缺少 source_path 或 file_name", node.ID)
			_ = writeUploadStderr(runDir, runErr)
			return runErr
		}
		source, err := extractConfigSourceFile(r.Registry, uploads, sourceName)
		if err != nil {
			runErr := fmt.Errorf("提取配置节点 %s %w", node.ID, err)
			_ = writeUploadStderr(runDir, runErr)
			return runErr
		}
		target := renderTemplate(item.TargetPath, workflowContext)
		if target == "" {
			target = source.DisplayPath
		}
		configID, err := workflowConfigFileID(target)
		if err != nil {
			runErr := fmt.Errorf("提取配置节点 %s 配置 ID 无效: %w", node.ID, err)
			_ = writeUploadStderr(runDir, runErr)
			return runErr
		}
		entry := config.ConfigFileRef{
			ID:        configID,
			Label:     strings.TrimSpace(item.Label),
			ConfigDir: ".",
			Path:      source.DisplayPath,
			Scope:     config.ConfigFileScopePlugin,
			Access:    config.ConfigFileAccessReadWrite,
			Create:    true,
		}
		if err := r.registerExtractedWorkflowConfigFile(wf, entry, item.Replace); err != nil {
			runErr := fmt.Errorf("提取配置节点 %s 登记配置失败: %w", node.ID, err)
			_ = writeUploadStderr(runDir, runErr)
			return runErr
		}
		if err := appendUploadStdout(runDir, fmt.Sprintf("SUCCESS 提取配置节点 %s 已挂载 %s 为配置 %s", node.ID, source.DisplayPath, configID)); err != nil {
			return err
		}
		if err := appendUploadStdout(runDir, fmt.Sprintf("CONFIG %s", source.Path)); err != nil {
			return err
		}
	}
	return writeUploadStderr(runDir, nil)
}

func extractConfigItems(item config.WorkflowExtractConfig) ([]config.WorkflowExtractConfigFile, error) {
	if strings.EqualFold(strings.TrimSpace(item.SourceType), "directory") || strings.TrimSpace(item.SourceDir) != "" || len(item.Files) > 0 {
		out := make([]config.WorkflowExtractConfigFile, 0, len(item.Files))
		for _, file := range item.Files {
			sourcePath := strings.TrimSpace(file.SourcePath)
			if sourcePath == "" {
				sourcePath = strings.TrimSpace(file.FileName)
			}
			targetPath := strings.TrimSpace(file.TargetPath)
			if targetPath == "" {
				targetPath = sourcePath
			}
			out = append(out, config.WorkflowExtractConfigFile{
				FileName:   joinWorkflowRelativePath(item.SourceDir, sourcePath),
				SourcePath: sourcePath,
				TargetPath: targetPath,
				Label:      file.Label,
				Replace:    file.Replace,
			})
		}
		return out, nil
	}
	if item.FileName != "" || item.TargetPath != "" || item.Label != "" || item.Replace {
		return []config.WorkflowExtractConfigFile{{FileName: item.FileName, SourcePath: item.SourcePath, TargetPath: item.TargetPath, Label: item.Label, Replace: item.Replace}}, nil
	}
	if item.SourcePath != "" {
		return []config.WorkflowExtractConfigFile{{FileName: item.SourcePath, SourcePath: item.SourcePath, TargetPath: item.TargetPath, Label: item.Label, Replace: item.Replace}}, nil
	}
	return nil, nil
}

func joinWorkflowRelativePath(base, rel string) string {
	base = strings.TrimSpace(base)
	rel = strings.TrimSpace(rel)
	if base == "" {
		return rel
	}
	if rel == "" {
		return base
	}
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(base), filepath.FromSlash(rel)))
}

func workflowUploadResults(workflowContext map[string]string) ([]config.WorkflowUploadResult, error) {
	keys := make([]string, 0)
	for key := range workflowContext {
		if strings.HasPrefix(key, "steps.") && strings.HasSuffix(key, ".upload_result_path") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	uploads := make([]config.WorkflowUploadResult, 0, len(keys))
	for _, key := range keys {
		upload, err := readWorkflowUploadResult(workflowContext[key])
		if err != nil {
			return nil, err
		}
		uploads = append(uploads, upload)
	}
	return uploads, nil
}

type extractConfigSource struct {
	DisplayPath string
	Path        string
}

func extractConfigSourceFile(reg *registry.Registry, uploads []config.WorkflowUploadResult, fileName string) (extractConfigSource, error) {
	files := make([]config.WorkflowUploadFile, 0)
	for _, upload := range uploads {
		files = append(files, normalizedUploadFiles(upload)...)
	}
	if len(files) == 0 {
		return extractConfigSource{}, fmt.Errorf("上传结果中没有文件")
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return extractConfigSource{}, fmt.Errorf("源文件名不能为空")
	}
	normalizedSource := normalizeUploadPathAlias(fileName)
	if normalizedSource == "" || normalizedSource == "." || normalizedSource == ".." || strings.HasPrefix(normalizedSource, "../") || strings.Contains(normalizedSource, "/../") || strings.Contains(normalizedSource, "://") {
		return extractConfigSource{}, fmt.Errorf("源文件路径不安全")
	}
	if source, ok, err := extractConfigLocalSourceFile(reg, normalizedSource); ok || err != nil {
		return source, err
	}
	var selected config.WorkflowUploadFile
	for _, file := range files {
		if uploadPathMatches(file, normalizedSource) {
			selected = file
			break
		}
	}
	if selected.Path == "" {
		return extractConfigSource{}, fmt.Errorf("上传结果中找不到文件: %s", fileName)
	}
	pathAbs, err := filepath.Abs(selected.Path)
	if err != nil {
		return extractConfigSource{}, fmt.Errorf("源文件路径不可解析: %w", err)
	}
	displayPath, err := workflowConfigSourceDisplayPath(reg, pathAbs)
	if err != nil {
		return extractConfigSource{}, err
	}
	info, err := os.Stat(pathAbs)
	if err != nil {
		return extractConfigSource{}, fmt.Errorf("源文件不可读: %w", err)
	}
	if !info.Mode().IsRegular() {
		return extractConfigSource{}, fmt.Errorf("源文件不是普通文件")
	}
	if info.Size() > maxWorkflowConfigFileBytes {
		return extractConfigSource{}, fmt.Errorf("源文件超过配置文件大小限制")
	}
	return extractConfigSource{DisplayPath: displayPath, Path: pathAbs}, nil
}

func extractConfigLocalSourceFile(reg *registry.Registry, fileName string) (extractConfigSource, bool, error) {
	if reg == nil || strings.TrimSpace(reg.BaseDir) == "" {
		return extractConfigSource{}, false, nil
	}
	candidate := filepath.Clean(filepath.FromSlash(normalizeUploadPathAlias(fileName)))
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(reg.BaseDir, candidate)
	}
	baseAbs, err := filepath.Abs(reg.BaseDir)
	if err != nil {
		return extractConfigSource{}, true, err
	}
	pathAbs, err := filepath.Abs(candidate)
	if err != nil {
		return extractConfigSource{}, true, err
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return extractConfigSource{}, false, nil
	}
	info, err := os.Stat(pathAbs)
	if os.IsNotExist(err) {
		return extractConfigSource{}, false, nil
	}
	if err != nil {
		return extractConfigSource{}, true, fmt.Errorf("源文件不可读: %w", err)
	}
	if !info.Mode().IsRegular() {
		return extractConfigSource{}, true, fmt.Errorf("源文件不是普通文件")
	}
	if info.Size() > maxWorkflowConfigFileBytes {
		return extractConfigSource{}, true, fmt.Errorf("源文件超过配置文件大小限制")
	}
	return extractConfigSource{DisplayPath: filepath.ToSlash(rel), Path: pathAbs}, true, nil
}

func uploadPathMatches(file config.WorkflowUploadFile, source string) bool {
	source = normalizeUploadPathAlias(source)
	if source == "" {
		return false
	}
	candidates := []string{
		normalizeUploadPathAlias(file.RelativePath),
		normalizeUploadPathAlias(file.FileName),
		normalizeUploadPathAlias(file.Path),
		normalizeUploadPathAlias(filepath.Base(file.RelativePath)),
		normalizeUploadPathAlias(filepath.Base(file.FileName)),
		normalizeUploadPathAlias(filepath.Base(file.Path)),
	}
	for _, candidate := range candidates {
		if candidate != "" && strings.EqualFold(candidate, source) {
			return true
		}
	}
	return false
}

func normalizeUploadPathAlias(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, `\`, `/`)
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "/mnt/") && len(value) >= len("/mnt/x") && value[5] != '/' && (len(value) == len("/mnt/x") || value[6] == '/') {
		drive := value[5]
		if drive >= 'a' && drive <= 'z' || drive >= 'A' && drive <= 'Z' {
			rest := ""
			if len(value) > len("/mnt/x") {
				rest = value[6:]
			}
			value = string(drive) + ":/" + strings.TrimLeft(rest, "/")
		}
	}
	if len(value) >= 3 && value[0] == '/' && value[2] == '/' && ((value[1] >= 'a' && value[1] <= 'z') || (value[1] >= 'A' && value[1] <= 'Z')) {
		value = string(value[1]) + ":/" + value[3:]
	}
	cleaned := filepath.Clean(filepath.FromSlash(value))
	return filepath.ToSlash(cleaned)
}

const maxWorkflowConfigFileBytes int64 = 1024 * 1024

func workflowConfigSourceDisplayPath(reg *registry.Registry, path string) (string, error) {
	if reg == nil || strings.TrimSpace(reg.BaseDir) == "" {
		return "", fmt.Errorf("运行根目录不可用")
	}
	baseAbs, err := filepath.Abs(reg.BaseDir)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("源文件逃逸运行根目录")
	}
	return filepath.ToSlash(rel), nil
}

func workflowConfigFileID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("配置 ID 不能为空")
	}
	if strings.Contains(value, "://") || filepath.IsAbs(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") || len(value) >= 2 && value[1] == ':' {
		return "", fmt.Errorf("配置 ID 不能是绝对路径")
	}
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("配置 ID 包含不安全路径片段")
		}
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("配置 ID 包含不安全路径片段")
	}
	return clean, nil
}

func firstPluginRoot(reg *registry.Registry) string {
	if reg != nil && reg.Root != nil && len(reg.Root.Plugins.Paths) > 0 && strings.TrimSpace(reg.Root.Plugins.Paths[0]) != "" {
		return strings.TrimSpace(reg.Root.Plugins.Paths[0])
	}
	return "plugins"
}

func (r *Runner) registerExtractedWorkflowConfigFile(wf *config.WorkflowConfig, entry config.ConfigFileRef, replace bool) error {
	if wf == nil || strings.TrimSpace(wf.ID) == "" {
		return fmt.Errorf("工作流配置不可用")
	}
	config.NormalizeConfigFileRef(&entry)
	if err := entry.ValidateBasic(); err != nil {
		return err
	}
	entry.ConfigDir = "."
	entry.Scope = config.ConfigFileScopePlugin
	entry.Access = config.ConfigFileAccessReadWrite
	entry.Create = true
	for i, existing := range wf.ConfigFiles {
		config.NormalizeConfigFileRef(&existing)
		if existing.ID != entry.ID {
			continue
		}
		if existing.Path == entry.Path && strings.TrimSpace(existing.ConfigDir) == "." {
			wf.ConfigFiles[i] = mergeWorkflowConfigFileRef(existing, entry)
			return r.saveWorkflowConfig(wf)
		}
		if !replace {
			return fmt.Errorf("配置 %s 已映射到 %s，设置 replace: true 才能改绑到 %s", entry.ID, existing.Path, entry.Path)
		}
		wf.ConfigFiles[i] = entry
		return r.saveWorkflowConfig(wf)
	}
	wf.ConfigFiles = append(wf.ConfigFiles, entry)
	return r.saveWorkflowConfig(wf)
}

func mergeWorkflowConfigFileRef(existing, next config.ConfigFileRef) config.ConfigFileRef {
	if next.Label == "" {
		next.Label = existing.Label
	}
	return next
}

func (r *Runner) saveWorkflowConfig(wf *config.WorkflowConfig) error {
	if r == nil || r.Registry == nil {
		return nil
	}
	path := runnerWorkflowPath(r.Registry, wf.ID)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(wf); err != nil {
		_ = enc.Close()
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return err
	}
	if existing, ok := r.Registry.Workflows[wf.ID]; ok {
		existing.Config = wf
		existing.Path = path
	} else {
		r.Registry.Workflows[wf.ID] = &registry.Workflow{Config: wf, Path: path}
	}
	return nil
}

func runnerWorkflowPath(reg *registry.Registry, id string) string {
	if reg == nil {
		return ""
	}
	if wf, ok := reg.Workflows[id]; ok && wf.Path != "" {
		return wf.Path
	}
	return filepath.Join(reg.BaseDir, filepath.FromSlash(firstPluginRoot(reg)), "user.workflows", "workflows", runnerWorkflowFilename(id))
}

func runnerWorkflowFilename(id string) string {
	clean := strings.TrimSpace(id)
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	clean = replacer.Replace(clean)
	clean = strings.Trim(clean, ". ")
	if clean == "" {
		clean = "workflow"
	}
	return clean + ".yaml"
}

func addUploadContext(context map[string]string, nodeID string, upload config.WorkflowUploadResult, runDir string) {
	prefix := "steps." + nodeID + "."
	context[prefix+"stdout"] = strings.TrimSpace(readTextFile(filepath.Join(runDir, "stdout.log")))
	context[prefix+"stderr"] = strings.TrimSpace(readTextFile(filepath.Join(runDir, "stderr.log")))
	context[prefix+"upload_result_path"] = filepath.Join(runDir, workflowUploadResultFile)
	context[prefix+"file.id"] = upload.ID
	context[prefix+"file.filename"] = upload.FileName
	context[prefix+"file.path"] = upload.Path
	context[prefix+"file.relative_path"] = upload.RelativePath
	context[prefix+"file.size"] = fmt.Sprint(upload.Size)
	if upload.Path != "" {
		context[prefix+"file.dir"] = filepath.Dir(upload.Path)
	}
	if upload.RelativePath != "" {
		context[prefix+"file.relative_dir"] = filepath.ToSlash(filepath.Dir(upload.RelativePath))
	}
	files := normalizedUploadFiles(upload)
	context[prefix+"files.count"] = fmt.Sprint(len(files))
	context[prefix+"files.total_size"] = fmt.Sprint(totalUploadSize(files))
	context[prefix+"files.paths"] = strings.Join(uploadFileValues(files, func(item config.WorkflowUploadFile) string { return item.Path }), ", ")
	context[prefix+"files.relative_paths"] = strings.Join(uploadFileValues(files, func(item config.WorkflowUploadFile) string { return item.RelativePath }), ", ")
	context[prefix+"files.filenames"] = strings.Join(uploadFileValues(files, func(item config.WorkflowUploadFile) string { return item.FileName }), ", ")
	if len(files) > 0 {
		context[prefix+"file.output"] = deriveUploadOutputName(files[0].FileName)
		context[prefix+"files.output"] = strings.Join(uploadFileValues(files, func(item config.WorkflowUploadFile) string { return deriveUploadOutputName(item.FileName) }), ", ")
	}
	for index, item := range files {
		itemPrefix := fmt.Sprintf("%sfiles.%d.", prefix, index)
		context[itemPrefix+"filename"] = item.FileName
		context[itemPrefix+"path"] = item.Path
		context[itemPrefix+"relative_path"] = item.RelativePath
		context[itemPrefix+"size"] = fmt.Sprint(item.Size)
	}
}

func deriveUploadOutputName(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return ""
	}
	base := filepath.Base(fileName)
	trimmed := strings.TrimSuffix(base, filepath.Ext(base))
	if trimmed == "" {
		return base
	}
	return trimmed
}

func normalizedUploadFiles(upload config.WorkflowUploadResult) []config.WorkflowUploadFile {
	if len(upload.Files) > 0 {
		return upload.Files
	}
	if upload.FileName == "" && upload.Path == "" {
		return nil
	}
	return []config.WorkflowUploadFile{{
		FileName:     upload.FileName,
		Path:         upload.Path,
		RelativePath: upload.RelativePath,
		Size:         upload.Size,
	}}
}

func totalUploadSize(files []config.WorkflowUploadFile) int64 {
	var total int64
	for _, item := range files {
		total += item.Size
	}
	return total
}

func uploadFileValues(files []config.WorkflowUploadFile, value func(config.WorkflowUploadFile) string) []string {
	out := make([]string, 0, len(files))
	for _, item := range files {
		out = append(out, value(item))
	}
	return out
}

func readLoopText(runDir, nodeID, fileName string, maxIterations int) string {
	parts := []string{}
	for iteration := 1; iteration <= maxIterations; iteration++ {
		text := strings.TrimSpace(readTextFile(filepath.Join(runDir, nodeID, fmt.Sprintf("%d", iteration), fileName)))
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func (r *Runner) executeTool(ctx context.Context, tool *registry.Tool, values map[string]interface{}, params map[string]string, runDir, hostID string, out, errOut io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	timeout := config.ParseTimeout(tool.Config.Execution.Timeout, 30*time.Minute)
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	paramFile, err := writeParamFile(runDir, tool.Config.PassMode, values)
	if err != nil {
		return err
	}
	entry := resolveEntry(tool.Dir, tool.Config.Execution.Entry)
	workdir := resolveWorkdir(tool.Dir, tool.Config.Execution.Workdir)
	extraEnv := encodeEnv(params)
	globalEnvPath := config.GlobalEnvPath(r.Registry.BaseDir)
	globalEnv := encodeGlobalEnv(r.Registry.GlobalEnv)
	for k, v := range tool.Config.Env {
		extraEnv = append(extraEnv, fmt.Sprintf("%s=%s", k, v))
	}

	stdoutFile, err := os.Create(filepath.Join(runDir, "stdout.log"))
	if err != nil {
		return err
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(filepath.Join(runDir, "stderr.log"))
	if err != nil {
		return err
	}
	defer stderrFile.Close()
	stdoutWriter := io.MultiWriter(stdoutFile, out)
	stderrWriter := io.MultiWriter(stderrFile, errOut)

	if strings.TrimSpace(hostID) != "" {
		if r.remote == nil {
			return fmt.Errorf("当前运行器未配置远程执行能力")
		}
		paramName := ""
		if paramFile != "" {
			paramName = filepath.Base(paramFile)
		}
		request := RemoteToolRequest{
			HostID:        hostID,
			ToolID:        tool.Config.ID,
			ToolDir:       tool.Dir,
			Entry:         entry,
			Workdir:       workdir,
			Args:          commandArgs(*tool.Config, params, ""),
			Env:           extraEnv,
			ParamFile:     paramFile,
			ParamName:     paramName,
			GlobalEnvFile: globalEnvPath,
			GlobalEnv:     globalEnv,
		}
		if err := r.remote.RunTool(execCtx, request, stdoutWriter, stderrWriter); err != nil {
			if execCtx.Err() != nil {
				return execCtx.Err()
			}
			return fmt.Errorf("执行工具 %s 失败: %w", tool.Config.ID, err)
		}
	} else {
		localEnv := append([]string{}, globalEnv...)
		localEnv = append(localEnv, extraEnv...)
		if paramFile != "" {
			localEnv = append(localEnv, "OPS_PARAM_FILE="+paramFile)
		}
		if absPath, err := filepath.Abs(globalEnvPath); err == nil {
			localEnv = append(localEnv, "OPS_GLOBAL_ENV_FILE="+absPath)
		}
		cmd := buildCommand(execCtx, entry, *tool.Config, params, paramFile, workdir, localEnv)
		configureCommandCancellation(cmd)
		if !usesWSLBash(entry) {
			cmd.Dir = workdir
			cmd.Env = append(os.Environ(), localEnv...)
		}
		cmd.Stdout = stdoutWriter
		cmd.Stderr = stderrWriter
		if err := cmd.Run(); err != nil {
			if execCtx.Err() != nil {
				return execCtx.Err()
			}
			return fmt.Errorf("执行工具 %s 失败: %w", tool.Config.ID, err)
		}
	}
	if err := validateToolOutputsFromStdout(tool.Config.ID, readTextFile(filepath.Join(runDir, "stdout.log")), tool.Config.Outputs); err != nil {
		return err
	}
	return nil
}

func resolveEntry(toolDir, entry string) string {
	if entry == "" || filepath.IsAbs(entry) || !strings.ContainsAny(entry, `/\\`) {
		return entry
	}
	return filepath.Join(toolDir, filepath.FromSlash(entry))
}

func buildCommand(ctx context.Context, entry string, tool config.ToolConfig, params map[string]string, paramFile, workdir string, extraEnv []string) *exec.Cmd {
	args := commandArgs(tool, params, paramFile)
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(entry), ".sh") {
		bash, wsl := bashRuntime()
		if wsl {
			return exec.CommandContext(ctx, bash, "-lc", wslBashScript(entry, workdir, extraEnv, args))
		}
		return exec.CommandContext(ctx, bash, append([]string{filepath.ToSlash(entry)}, args...)...)
	}
	return exec.CommandContext(ctx, entry, args...)
}

func configureCommandCancellation(cmd *exec.Cmd) {
	prepareCommandProcessGroup(cmd)
	cmd.Cancel = func() error {
		return killCommandTree(cmd)
	}
	cmd.WaitDelay = 2 * time.Second
}

func commandArgs(tool config.ToolConfig, params map[string]string, paramFile string) []string {
	args := renderArgs(tool.Execution.Args, params)
	if len(args) == 0 && tool.PassMode.Args {
		for _, k := range sortedKeys(params) {
			args = append(args, "--"+k, params[k])
		}
	}
	if tool.PassMode.ParamFile && paramFile != "" {
		args = append(args, "--params-file", paramFile)
	}
	return args
}

func usesWSLBash(entry string) bool {
	return runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(entry), ".sh") && isWSLBash()
}

func wslBashScript(entry, workdir string, env []string, args []string) string {
	parts := []string{}
	if converted := windowsPathToWSL(workdir); converted != "" {
		parts = append(parts, "cd "+shellQuote(converted))
	}
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if !ok || !isShellEnvName(key) {
			continue
		}
		if converted := windowsPathToWSL(value); converted != "" {
			value = converted
		}
		parts = append(parts, "export "+key+"="+shellQuote(value))
	}
	command := "exec " + shellQuote(windowsPathToWSLOrSlash(entry))
	for _, arg := range args {
		command += " " + shellQuote(windowsPathToWSLOrSlash(arg))
	}
	parts = append(parts, command)
	return strings.Join(parts, " && ")
}

func windowsPathToWSLOrSlash(path string) string {
	if converted := windowsPathToWSL(path); converted != "" {
		return converted
	}
	return filepath.ToSlash(path)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func isShellEnvName(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func isWSLBash() bool {
	_, wsl := bashRuntime()
	return wsl
}

// bashRuntime 返回执行 .sh 的 bash 路径及是否为 WSL。
// Windows PATH 中 System32 可能排在 Git 之前，直接 LookPath 会命中 WSL 启动器，
// 因此按 PATH 顺序优先选择非 WSL 的 bash。
func bashRuntime() (string, bool) {
	if runtime.GOOS != "windows" {
		return "bash", false
	}
	return findBash(os.Getenv("PATH"), isWSLBashPath)
}

func findBash(pathValue string, isWSL func(string) bool) (string, bool) {
	fallback := ""
	for _, dir := range filepath.SplitList(pathValue) {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		candidate := filepath.Join(dir, "bash.exe")
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if isWSL(candidate) {
			if fallback == "" {
				fallback = candidate
			}
			continue
		}
		return candidate, false
	}
	if fallback != "" {
		return fallback, true
	}
	return "bash", false
}

func isWSLBashPath(path string) bool {
	normalized := strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(normalized, "/windows/system32/bash.exe") || strings.Contains(normalized, "/windowsapps/bash.exe")
}

func windowsPathToWSL(path string) string {
	volume := filepath.VolumeName(path)
	if len(volume) != 2 || volume[1] != ':' {
		return ""
	}
	drive := strings.ToLower(volume[:1])
	rest := strings.TrimLeft(path[len(volume):], `\/`)
	if rest == "" {
		return "/mnt/" + drive
	}
	return "/mnt/" + drive + "/" + filepath.ToSlash(rest)
}

func renderArgs(templates []string, params map[string]string) []string {
	args := make([]string, 0, len(templates))
	for _, item := range templates {
		args = append(args, renderTemplate(item, params))
	}
	return args
}

func renderTemplate(value string, params map[string]string) string {
	out := value
	for k, v := range params {
		out = strings.ReplaceAll(out, "{{ ."+k+" }}", v)
		out = strings.ReplaceAll(out, "{{."+k+"}}", v)
		out = strings.ReplaceAll(out, "${"+k+"}", v)
	}
	return out
}

func resolveWorkdir(toolDir, workdir string) string {
	if workdir == "" || workdir == "." {
		return toolDir
	}
	if filepath.IsAbs(workdir) {
		return workdir
	}
	return filepath.Join(toolDir, filepath.FromSlash(workdir))
}

func writeParamFile(runDir string, mode config.PassMode, params map[string]interface{}) (string, error) {
	if !mode.ParamFile {
		return "", nil
	}
	path := filepath.Join(runDir, filepath.Base(mode.FileName))
	data, err := yaml.Marshal(params)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (r *Runner) resolveToolValues(tool *registry.Tool, runtimeParams map[string]interface{}) (config.Values, map[string]string, []string, error) {
	values := config.MergeValues(
		config.MergeParameterDefaults(tool.Config.Parameters),
		r.Registry.GlobalEnv,
		r.Registry.Root.ConfigDefaults,
		tool.Config.PluginConfig.SharedConfig,
		tool.Config.PluginConfig.PackageDefaultConfig,
		tool.Config.ConfigDefaults,
		tool.Config.PluginConfig.HostConfig,
		config.InterfaceMapToValues(runtimeParams),
	)
	if err := config.ValidateRequiredValues(tool.Config.Parameters, values); err != nil {
		return nil, nil, nil, err
	}
	sensitivePaths := append([]string{}, tool.Config.PluginConfig.SensitivePaths...)
	sensitivePaths = append(sensitivePaths, tool.Config.SensitivePaths...)
	sensitivePaths = append(sensitivePaths, config.SensitivePathsFromParams(tool.Config.Parameters)...)
	return values, config.FlattenValues(values), sensitivePaths, nil
}

func encodeEnv(params map[string]string) []string {
	out := make([]string, 0, len(params))
	for k, v := range params {
		name := "OPS_PARAM_" + strings.ToUpper(strings.ReplaceAll(k, "-", "_"))
		out = append(out, fmt.Sprintf("%s=%s", name, v))
	}
	return out
}

func encodeGlobalEnv(values config.Values) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, fmt.Sprintf("%s=%v", key, values[key]))
	}
	return out
}

func resolveStepParams(global, templateContext map[string]string, step map[string]interface{}) map[string]string {
	base := config.StringMapToValues(global)
	resolved := config.Values{}
	for k, v := range step {
		mergeStepParam(resolved, k, v, templateContext)
	}
	return config.FlattenValues(config.MergeValues(base, resolved))
}

func mergeStepParam(out map[string]interface{}, prefix string, value interface{}, templateContext map[string]string) {
	if nested, ok := value.(map[string]interface{}); ok {
		branch := config.Values{}
		for k, v := range nested {
			mergeStepParam(branch, k, v, templateContext)
		}
		if err := config.SetPathValue(out, prefix, branch); err != nil {
			out[prefix] = branch
		}
		return
	}
	if nested, ok := value.(map[interface{}]interface{}); ok {
		branch := config.Values{}
		for k, v := range nested {
			mergeStepParam(branch, fmt.Sprint(k), v, templateContext)
		}
		if err := config.SetPathValue(out, prefix, branch); err != nil {
			out[prefix] = branch
		}
		return
	}
	if err := config.SetPathValue(out, prefix, renderTemplate(fmt.Sprint(value), templateContext)); err != nil {
		out[prefix] = renderTemplate(fmt.Sprint(value), templateContext)
	}
}

func addStepContext(context map[string]string, nodeID string, params map[string]string, runDir string, outputs []config.ToolOutput) {
	prefix := "steps." + nodeID + "."
	for k, v := range params {
		context[prefix+"params."+k] = v
	}
	stdout := strings.TrimSpace(readTextFile(filepath.Join(runDir, "stdout.log")))
	context[prefix+"stdout"] = stdout
	context[prefix+"stderr"] = strings.TrimSpace(readTextFile(filepath.Join(runDir, "stderr.log")))
	for name, value := range extractToolOutputs(stdout, outputs) {
		context[prefix+"outputs."+name] = value
	}
	if outputPath := outputPathFromStdout(stdout); outputPath != "" {
		context[prefix+"file.path"] = outputPath
		context[prefix+"file.filename"] = filepath.Base(outputPath)
		context[prefix+"file.output"] = filepath.Base(outputPath)
		context[prefix+"file.dir"] = filepath.Dir(outputPath)
	}
}

func outputPathFromStdout(stdout string) string {
	var output string
	for _, line := range strings.Split(stdout, "\n") {
		text := strings.TrimSpace(line)
		for _, marker := range []string{"输出文件:", "合并文件:"} {
			if _, after, ok := strings.Cut(text, marker); ok {
				candidate := strings.TrimSpace(after)
				if candidate != "" {
					output = candidate
				}
			}
		}
	}
	return output
}

func validateToolOutputsFromStdout(toolID, stdout string, outputs []config.ToolOutput) error {
	values := extractToolOutputs(stdout, outputs)
	for _, output := range outputs {
		if !output.Required {
			continue
		}
		if strings.TrimSpace(values[output.Name]) == "" {
			return fmt.Errorf("工具 %s 必需输出参数 %s 提取失败", toolID, output.Name)
		}
	}
	return nil
}

func extractToolOutputs(stdout string, outputs []config.ToolOutput) map[string]string {
	values := make(map[string]string, len(outputs))
	if len(outputs) == 0 {
		return values
	}
	for _, output := range outputs {
		values[output.Name] = ""
	}
	payload, ok := lastStdoutJSON(stdout)
	if !ok {
		return values
	}
	for _, output := range outputs {
		value, ok := valueAtDotPath(payload, output.JSONPath)
		if !ok || value == nil {
			continue
		}
		values[output.Name] = stringifyOutputValue(value)
	}
	return values
}

func lastStdoutJSON(stdout string) (map[string]interface{}, bool) {
	lines := strings.Split(stdout, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(line), &payload); err == nil {
			return payload, true
		}
	}
	return nil, false
}

func valueAtDotPath(payload map[string]interface{}, path string) (interface{}, bool) {
	current := interface{}(payload)
	for _, part := range strings.Split(path, ".") {
		key := strings.TrimSpace(part)
		if key == "" {
			return nil, false
		}
		object, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func stringifyOutputValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, stringifyOutputValue(item))
		}
		return strings.Join(parts, ",")
	case bool:
		return fmt.Sprint(typed)
	case float64:
		return fmt.Sprint(typed)
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func readTextFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func copyParams(params map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range params {
		out[k] = v
	}
	return out
}

func (r *Runner) prepareRun(id string) (string, error) {
	dir := filepath.Join(r.RunsDir, id)
	return dir, os.MkdirAll(dir, 0o755)
}

func (r *Runner) saveRecord(dir string, record *RunRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	tempPath := filepath.Join(dir, fmt.Sprintf("result.json.%d.tmp", time.Now().UnixNano()))
	resultPath := filepath.Join(dir, "result.json")
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < 25; attempt++ {
		if err := os.Rename(tempPath, resultPath); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if err := os.Remove(resultPath); err != nil && !os.IsNotExist(err) {
			lastErr = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if err := os.Rename(tempPath, resultPath); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = os.Remove(tempPath)
	return lastErr
}

var idSequence atomic.Uint64

// 批量执行会在同一纳秒内创建多条记录，追加自增序号避免运行目录重名。
func newID(kind string) string {
	return fmt.Sprintf("%s-%d-%d", kind, time.Now().UnixNano(), idSequence.Add(1))
}

func newBatchID() string {
	return newID("batch")
}

func newRecord(kind, target string, params map[string]interface{}) *RunRecord {
	flat := config.FlattenValues(params)
	return &RunRecord{ID: newID(kind), Kind: kind, Target: target, Status: "running", StartedAt: time.Now(), Params: flat, Config: config.CopyValues(params)}
}

func finishRecord(record *RunRecord, err error) {
	record.EndedAt = time.Now()
	if err != nil {
		if isPendingRunError(err) {
			record.Status = RunStatusPending
			record.Error = err.Error()
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			record.Status = "cancelled"
			record.Error = "运行已取消"
			markRunningStepsCancelled(record)
			return
		}
		record.Status = "failed"
		record.Error = err.Error()
		return
	}
	record.Status = "succeeded"
}

func markRunningStepsCancelled(record *RunRecord) {
	for index := range record.Steps {
		if record.Steps[index].Status != "running" && record.Steps[index].Status != "waiting" {
			continue
		}
		record.Steps[index].Status = "cancelled"
		record.Steps[index].EndedAt = record.EndedAt
		record.Steps[index].Error = "运行已取消"
	}
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

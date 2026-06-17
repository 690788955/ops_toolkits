package runner

import (
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
	"time"

	"gopkg.in/yaml.v3"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

type Runner struct {
	Registry *registry.Registry
	RunsDir  string
}

const workflowUploadResultFile = "upload-result.json"

type RunRecord struct {
	ID        string                 `json:"id"`
	Kind      string                 `json:"kind"`
	Target    string                 `json:"target"`
	Status    string                 `json:"status"`
	StartedAt time.Time              `json:"started_at"`
	EndedAt   time.Time              `json:"ended_at"`
	Params    map[string]string      `json:"params"`
	Config    map[string]interface{} `json:"config,omitempty"`
	Steps     []StepRecord           `json:"steps,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

type StepRecord struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	Tool           string    `json:"tool,omitempty"`
	Status         string    `json:"status"`
	StartedAt      time.Time `json:"started_at"`
	EndedAt        time.Time `json:"ended_at"`
	Error          string    `json:"error,omitempty"`
	ConditionInput string    `json:"condition_input,omitempty"`
	MatchedCase    string    `json:"matched_case,omitempty"`
	SkippedReason  string    `json:"skipped_reason,omitempty"`
	LoopTarget     string    `json:"loop_target,omitempty"`
	LoopIterations int       `json:"loop_iterations,omitempty"`
}

func New(reg *registry.Registry) *Runner {
	return &Runner{Registry: reg, RunsDir: filepath.Join(reg.BaseDir, filepath.FromSlash(reg.Root.Paths.Logs))}
}

func (r *Runner) RunTool(ctx context.Context, id string, params map[string]string, out, errOut io.Writer) (*RunRecord, error) {
	return r.RunToolValues(ctx, id, config.StringMapToValues(params), out, errOut)
}

func (r *Runner) RunToolValues(ctx context.Context, id string, params map[string]interface{}, out, errOut io.Writer) (*RunRecord, error) {
	tool, err := r.Registry.Tool(id)
	if err != nil {
		return nil, err
	}
	finalValues, finalParams, sensitivePaths, err := r.resolveToolValues(tool, params)
	if err != nil {
		return nil, err
	}
	record := newRecord("tool", id, config.RedactSensitive(finalValues, sensitivePaths))
	runDir, err := r.prepareRun(record.ID)
	if err != nil {
		return record, err
	}
	err = r.executeTool(ctx, tool, finalValues, finalParams, runDir, out, errOut)
	finishRecord(record, err)
	if saveErr := r.saveRecord(runDir, record); saveErr != nil && err == nil {
		err = saveErr
	}
	return record, err
}

func (r *Runner) StartToolValues(ctx context.Context, id string, params map[string]interface{}, out, errOut io.Writer) (*RunRecord, error) {
	tool, err := r.Registry.Tool(id)
	if err != nil {
		return nil, err
	}
	finalValues, finalParams, sensitivePaths, err := r.resolveToolValues(tool, params)
	if err != nil {
		return nil, err
	}
	record := newRecord("tool", id, config.RedactSensitive(finalValues, sensitivePaths))
	runDir, err := r.prepareRun(record.ID)
	if err != nil {
		return record, err
	}
	if err := r.saveRecord(runDir, record); err != nil {
		return record, err
	}
	response := *record
	go func() {
		runErr := r.executeTool(ctx, tool, finalValues, finalParams, runDir, out, errOut)
		finishRecord(record, runErr)
		_ = r.saveRecord(runDir, record)
	}()
	return &response, nil
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
	return record, finalParams, runDir, nil
}

func (r *Runner) runWorkflowPrepared(ctx context.Context, wf *config.WorkflowConfig, finalParams map[string]string, confirmed bool, uploads map[string]config.WorkflowUploadResult, out, errOut io.Writer, record *RunRecord, runDir string) (*RunRecord, error) {
	_ = r.saveRecord(runDir, record)
	ordered, err := registry.OrderWorkflow(wf)
	if err != nil {
		finishRecord(record, err)
		_ = r.saveRecord(runDir, record)
		return record, err
	}
	if err := r.validateWorkflowConfirmations(ordered, confirmed); err != nil {
		finishRecord(record, err)
		_ = r.saveRecord(runDir, record)
		return record, err
	}
	workflowContext, active, nodeByID, legacyLoopTargets, edgesByFrom, err := prepareWorkflowExecutionState(wf, finalParams)
	if err != nil {
		finishRecord(record, err)
		_ = r.saveRecord(runDir, record)
		return record, err
	}
	err = r.executeWorkflowNodes(ctx, wf, ordered, finalParams, uploads, out, errOut, record, runDir, workflowContext, active, nodeByID, legacyLoopTargets, edgesByFrom, nil)
	finishRecord(record, err)
	if saveErr := r.saveRecord(runDir, record); saveErr != nil && err == nil {
		err = saveErr
	}
	return record, err
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
	rerunUploads := reusableUploadsForNodes(runDir, wf.Nodes, affected)
	record.Steps = filterStepsOutside(record.Steps, affected)
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
	_ = r.saveRecord(runDir, record)
	err = r.executeWorkflowNodes(ctx, wf, ordered, finalParams, rerunUploads, out, errOut, record, runDir, workflowContext, active, nodeByID, legacyLoopTargets, edgesByFrom, affected)
	finishRecord(record, err)
	if saveErr := r.saveRecord(runDir, record); saveErr != nil && err == nil {
		err = saveErr
	}
	return record, err
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
	var err error
	for _, node := range ordered {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if executeOnly != nil && !executeOnly[node.ID] {
			continue
		}
		nodeType := workflowNodeType(node)
		if legacyLoopTargets[node.ID] {
			reason := "循环目标工具由循环节点内嵌执行"
			record.Steps = append(record.Steps, skippedStepRecord(node, nodeType, reason))
			_ = r.saveRecord(runDir, record)
			continue
		}
		if !active[node.ID] {
			reason := "条件分支未激活"
			record.Steps = append(record.Steps, skippedStepRecord(node, nodeType, reason))
			_ = r.saveRecord(runDir, record)
			continue
		}
		if nodeType == config.WorkflowNodeTypeCondition {
			stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "running", StartedAt: time.Now()}
			stepIndex := appendStepRecord(record, stepRecord)
			_ = r.saveRecord(runDir, record)
			inputValue := renderTemplate(node.Condition.Input, workflowContext)
			matchedCase := matchConditionCase(inputValue, node.Condition)
			stepRecord.ConditionInput = inputValue
			stepRecord.MatchedCase = matchedCase
			stepRecord.EndedAt = time.Now()
			stepRecord.Status = "succeeded"
			record.Steps[stepIndex] = stepRecord
			_ = r.saveRecord(runDir, record)
			activateConditionBranches(node.ID, matchedCase, edgesByFrom, active)
			workflowContext["steps."+node.ID+".condition.input"] = inputValue
			workflowContext["steps."+node.ID+".condition.case"] = matchedCase
			continue
		}
		if nodeType == config.WorkflowNodeTypeParallel || nodeType == config.WorkflowNodeTypeJoin {
			stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "succeeded", StartedAt: time.Now(), EndedAt: time.Now()}
			record.Steps = append(record.Steps, stepRecord)
			_ = r.saveRecord(runDir, record)
			activatePlainBranches(node.ID, edgesByFrom, active)
			continue
		}
		if nodeType == config.WorkflowNodeTypeUpload {
			if executeOnly != nil && !hasUploadResult(uploads, node.ID) {
				stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "failed", StartedAt: time.Now(), EndedAt: time.Now(), Error: fmt.Sprintf("上传节点 %s 缺少可复用的上传结果", node.ID)}
				record.Steps = append(record.Steps, stepRecord)
				_ = r.saveRecord(runDir, record)
				return fmt.Errorf("上传节点 %s 缺少可复用的上传结果", node.ID)
			}
			status := "waiting"
			if hasUploadResult(uploads, node.ID) {
				status = "running"
			}
			stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: status, StartedAt: time.Now()}
			stepRunDir := filepath.Join(runDir, node.ID)
			stepIndex := appendStepRecord(record, stepRecord)
			_ = r.saveRecord(runDir, record)
			upload, uploadErr := waitForUploadNode(ctx, node, uploads, stepRunDir)
			stepRecord.Status = "running"
			record.Steps[stepIndex] = stepRecord
			_ = r.saveRecord(runDir, record)
			if uploadErr == nil {
				if hasUploadResult(uploads, node.ID) {
					uploadErr = WriteWorkflowUploadResult(runDir, node.ID, upload)
				}
			}
			if uploadErr == nil {
				uploadErr = executeUploadNode(node, upload, stepRunDir)
			}
			stepRecord.EndedAt = time.Now()
			if uploadErr != nil {
				if errors.Is(uploadErr, context.Canceled) {
					stepRecord.Status = "cancelled"
					stepRecord.Error = "运行已取消"
				} else {
					stepRecord.Status = "failed"
					stepRecord.Error = uploadErr.Error()
				}
				record.Steps[stepIndex] = stepRecord
				_ = r.saveRecord(runDir, record)
				err = uploadErr
				break
			}
			addUploadContext(workflowContext, node.ID, upload, stepRunDir)
			stepRecord.Status = "succeeded"
			record.Steps[stepIndex] = stepRecord
			_ = r.saveRecord(runDir, record)
			activatePlainBranches(node.ID, edgesByFrom, active)
			continue
		}
		if nodeType == config.WorkflowNodeTypeExtractConfig {
			stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "running", StartedAt: time.Now()}
			stepRunDir := filepath.Join(runDir, node.ID)
			stepIndex := appendStepRecord(record, stepRecord)
			_ = r.saveRecord(runDir, record)
			extractErr := r.executeExtractConfigNode(wf.ID, node, workflowContext, stepRunDir)
			stepRecord.EndedAt = time.Now()
			if extractErr != nil {
				if errors.Is(extractErr, context.Canceled) {
					stepRecord.Status = "cancelled"
					stepRecord.Error = "运行已取消"
				} else {
					stepRecord.Status = "failed"
					stepRecord.Error = extractErr.Error()
				}
				record.Steps[stepIndex] = stepRecord
				_ = r.saveRecord(runDir, record)
				err = extractErr
				break
			}
			addStepContext(workflowContext, node.ID, nil, stepRunDir, nil)
			stepRecord.Status = "succeeded"
			record.Steps[stepIndex] = stepRecord
			_ = r.saveRecord(runDir, record)
			activatePlainBranches(node.ID, edgesByFrom, active)
			continue
		}
		if nodeType == config.WorkflowNodeTypeLoop {
			stepRecord := StepRecord{ID: node.ID, Type: nodeType, Tool: loopToolID(node, nodeByID), Status: "running", StartedAt: time.Now(), LoopTarget: node.Loop.Target, LoopIterations: node.Loop.MaxIterations}
			stepIndex := appendStepRecord(record, stepRecord)
			_ = r.saveRecord(runDir, record)
			loopErr := r.executeLoop(ctx, node, nodeByID, finalParams, workflowContext, runDir, out, errOut)
			stepRecord.EndedAt = time.Now()
			if loopErr != nil {
				if errors.Is(loopErr, context.Canceled) {
					stepRecord.Status = "cancelled"
					stepRecord.Error = "运行已取消"
				} else {
					stepRecord.Status = "failed"
					stepRecord.Error = loopErr.Error()
				}
				record.Steps[stepIndex] = stepRecord
				_ = r.saveRecord(runDir, record)
				err = loopErr
				break
			}
			addLoopContext(workflowContext, node.ID, node.Loop.MaxIterations, runDir)
			stepRecord.Status = "succeeded"
			record.Steps[stepIndex] = stepRecord
			_ = r.saveRecord(runDir, record)
			activatePlainBranches(node.ID, edgesByFrom, active)
			continue
		}
		stepParams := resolveStepParams(finalParams, workflowContext, node.Params)
		tool, toolErr := r.Registry.Tool(node.Tool)
		stepRecord := StepRecord{ID: node.ID, Type: nodeType, Tool: node.Tool, Status: "running", StartedAt: time.Now()}
		stepRunDir := filepath.Join(runDir, node.ID)
		stepIndex := appendStepRecord(record, stepRecord)
		_ = r.saveRecord(runDir, record)
		if toolErr == nil {
			stepValues, stepFlat, _, resolveErr := r.resolveToolValues(tool, config.StringMapToValues(stepParams))
			if resolveErr != nil {
				toolErr = resolveErr
			} else {
				stepParams = stepFlat
				toolErr = r.executeTool(ctx, tool, stepValues, stepFlat, stepRunDir, out, errOut)
			}
		}
		stepRecord.EndedAt = time.Now()
		if toolErr != nil {
			if errors.Is(toolErr, context.Canceled) {
				stepRecord.Status = "cancelled"
				stepRecord.Error = "运行已取消"
			} else {
				stepRecord.Status = "failed"
				stepRecord.Error = toolErr.Error()
			}
			record.Steps[stepIndex] = stepRecord
			_ = r.saveRecord(runDir, record)
			err = toolErr
			break
		}
		addStepContext(workflowContext, node.ID, stepParams, stepRunDir, tool.Config.Outputs)
		stepRecord.Status = "succeeded"
		record.Steps[stepIndex] = stepRecord
		_ = r.saveRecord(runDir, record)
		activatePlainBranches(node.ID, edgesByFrom, active)
	}
	return err
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
func (r *Runner) executeLoop(ctx context.Context, node config.WorkflowNode, nodes map[string]config.WorkflowNode, finalParams, workflowContext map[string]string, runDir string, out, errOut io.Writer) error {
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
		if err := r.executeTool(ctx, tool, stepValues, stepFlat, stepRunDir, out, errOut); err != nil {
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

func (r *Runner) executeExtractConfigNode(workflowID string, node config.WorkflowNode, workflowContext map[string]string, runDir string) error {
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
		source, data, err := extractConfigSourceFile(uploads, sourceName)
		if err != nil {
			runErr := fmt.Errorf("提取配置节点 %s %w", node.ID, err)
			_ = writeUploadStderr(runDir, runErr)
			return runErr
		}
		target := item.TargetPath
		if target == "" {
			target = source
		}
		targetPath, err := workflowConfigTargetPath(r.Registry, target)
		if err != nil {
			runErr := fmt.Errorf("提取配置节点 %s 目标路径无效: %w", node.ID, err)
			_ = writeUploadStderr(runDir, runErr)
			return runErr
		}
		if err := writeExtractedConfigFile(targetPath, data, item.Replace); err != nil {
			runErr := fmt.Errorf("提取配置节点 %s 写入配置失败: %w", node.ID, err)
			_ = writeUploadStderr(runDir, runErr)
			return runErr
		}
		if err := appendUploadStdout(runDir, fmt.Sprintf("SUCCESS 提取配置节点 %s 已复制 %s 到 %s", node.ID, source, targetPath)); err != nil {
			return err
		}
		if err := appendUploadStdout(runDir, fmt.Sprintf("CONFIG %s", targetPath)); err != nil {
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
			out = append(out, config.WorkflowExtractConfigFile{
				FileName:   joinWorkflowRelativePath(item.SourceDir, sourcePath),
				SourcePath: sourcePath,
				TargetPath: sourcePath,
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

func extractConfigSourceFile(uploads []config.WorkflowUploadResult, fileName string) (string, []byte, error) {
	files := make([]config.WorkflowUploadFile, 0)
	for _, upload := range uploads {
		files = append(files, normalizedUploadFiles(upload)...)
	}
	if len(files) == 0 {
		return "", nil, fmt.Errorf("上传结果中没有文件")
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "", nil, fmt.Errorf("源文件名不能为空")
	}
	normalizedSource := filepath.ToSlash(filepath.Clean(filepath.FromSlash(fileName)))
	if normalizedSource == "." || normalizedSource == ".." || strings.HasPrefix(normalizedSource, "../") || strings.Contains(normalizedSource, "/../") || filepath.IsAbs(normalizedSource) || strings.Contains(normalizedSource, "://") {
		return "", nil, fmt.Errorf("源文件路径不安全")
	}
	var selected config.WorkflowUploadFile
	for _, file := range files {
		rel := filepath.ToSlash(file.RelativePath)
		name := filepath.ToSlash(file.FileName)
		if name == normalizedSource || rel == normalizedSource || filepath.Base(rel) == normalizedSource || filepath.Base(name) == normalizedSource {
			selected = file
			break
		}
	}
	if selected.Path == "" {
		return "", nil, fmt.Errorf("上传结果中找不到文件: %s", fileName)
	}
	info, err := os.Stat(selected.Path)
	if err != nil {
		return "", nil, fmt.Errorf("源文件不可读: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("源文件不是普通文件")
	}
	if info.Size() > maxWorkflowConfigFileBytes {
		return "", nil, fmt.Errorf("源文件超过配置文件大小限制")
	}
	data, err := os.ReadFile(selected.Path)
	if err != nil {
		return "", nil, fmt.Errorf("读取源文件失败: %w", err)
	}
	return selected.RelativePath, data, nil
}

const maxWorkflowConfigFileBytes int64 = 1024 * 1024

func workflowConfigTargetPath(reg *registry.Registry, targetPath string) (string, error) {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return "", fmt.Errorf("目标配置路径不能为空")
	}
	if strings.Contains(targetPath, "://") || filepath.IsAbs(targetPath) || strings.HasPrefix(targetPath, "/") || strings.HasPrefix(targetPath, "\\") || len(targetPath) >= 2 && targetPath[1] == ':' {
		return "", fmt.Errorf("目标配置路径不能是绝对路径")
	}
	for _, part := range strings.FieldsFunc(targetPath, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("目标配置路径包含不安全路径片段")
		}
	}
	cleanItem := filepath.Clean(filepath.FromSlash(targetPath))
	baseAbs, err := filepath.Abs(reg.BaseDir)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(baseAbs, cleanItem))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("目标配置路径逃逸工作流配置目录")
	}
	return pathAbs, nil
}

func writeExtractedConfigFile(path string, data []byte, replace bool) error {
	if int64(len(data)) > maxWorkflowConfigFileBytes {
		return fmt.Errorf("配置文件内容超过大小限制")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil && !replace {
		return fmt.Errorf("目标配置文件已存在")
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("检查目标配置文件失败: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
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

func (r *Runner) executeTool(ctx context.Context, tool *registry.Tool, values map[string]interface{}, params map[string]string, runDir string, out, errOut io.Writer) error {
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
	if paramFile != "" {
		extraEnv = append(extraEnv, "OPS_PARAM_FILE="+paramFile)
	}
	globalEnvPath := config.GlobalEnvPath(r.Registry.BaseDir)
	if absPath, err := filepath.Abs(globalEnvPath); err == nil {
		extraEnv = append(extraEnv, "OPS_GLOBAL_ENV_FILE="+absPath)
	}
	for k, v := range tool.Config.Env {
		extraEnv = append(extraEnv, fmt.Sprintf("%s=%s", k, v))
	}
	cmd := buildCommand(execCtx, entry, *tool.Config, params, paramFile, workdir, extraEnv)
	configureCommandCancellation(cmd)
	if !usesWSLBash(entry) {
		cmd.Dir = workdir
		cmd.Env = append(os.Environ(), extraEnv...)
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
	cmd.Stdout = io.MultiWriter(stdoutFile, out)
	cmd.Stderr = io.MultiWriter(stderrFile, errOut)
	if err := cmd.Run(); err != nil {
		if execCtx.Err() != nil {
			return execCtx.Err()
		}
		return fmt.Errorf("执行工具 %s 失败: %w", tool.Config.ID, err)
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
	if usesWSLBash(entry) {
		return exec.CommandContext(ctx, "bash", "-lc", wslBashScript(entry, workdir, extraEnv, args))
	}
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(entry), ".sh") {
		return exec.CommandContext(ctx, "bash", append([]string{windowsBashPath(entry)}, args...)...)
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

func windowsBashPath(path string) string {
	if isWSLBash() {
		if converted := windowsPathToWSL(path); converted != "" {
			return converted
		}
	}
	return filepath.ToSlash(path)
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
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		return false
	}
	normalized := strings.ToLower(filepath.ToSlash(bashPath))
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

func newRecord(kind, target string, params map[string]interface{}) *RunRecord {
	flat := config.FlattenValues(params)
	return &RunRecord{ID: fmt.Sprintf("%s-%d", kind, time.Now().UnixNano()), Kind: kind, Target: target, Status: "running", StartedAt: time.Now(), Params: flat, Config: config.CopyValues(params)}
}

func finishRecord(record *RunRecord, err error) {
	record.EndedAt = time.Now()
	if err != nil {
		if errors.Is(err, context.Canceled) {
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

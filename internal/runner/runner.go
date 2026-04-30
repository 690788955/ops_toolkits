package runner

import (
	"context"
	"encoding/json"
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

type RunRecord struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Target    string            `json:"target"`
	Status    string            `json:"status"`
	StartedAt time.Time         `json:"started_at"`
	EndedAt   time.Time         `json:"ended_at"`
	Params    map[string]string `json:"params"`
	Steps     []StepRecord      `json:"steps,omitempty"`
	Error     string            `json:"error,omitempty"`
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
	tool, err := r.Registry.Tool(id)
	if err != nil {
		return nil, err
	}
	finalParams := config.MergeParams(tool.Config.Parameters, nil, params)
	if err := config.ValidateRequired(tool.Config.Parameters, finalParams); err != nil {
		return nil, err
	}
	record := newRecord("tool", id, finalParams)
	runDir, err := r.prepareRun(record.ID)
	if err != nil {
		return record, err
	}
	err = r.executeTool(ctx, tool, finalParams, runDir, out, errOut)
	finishRecord(record, err)
	if saveErr := r.saveRecord(runDir, record); saveErr != nil && err == nil {
		err = saveErr
	}
	return record, err
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
	if wf == nil {
		return nil, fmt.Errorf("工作流不能为空")
	}
	config.NormalizeWorkflow(wf)
	finalParams := config.MergeParams(wf.Parameters, nil, params)
	if err := config.ValidateRequired(wf.Parameters, finalParams); err != nil {
		return nil, err
	}
	record := newRecord("workflow", wf.ID, finalParams)
	runDir, err := r.prepareRun(record.ID)
	if err != nil {
		return nil, err
	}
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
	incomingCaseByTo := map[string]map[string]bool{}
	for _, edges := range edgesByFrom {
		for _, edge := range edges {
			if edge.Case == "" {
				continue
			}
			if incomingCaseByTo[edge.To] == nil {
				incomingCaseByTo[edge.To] = map[string]bool{}
			}
			incomingCaseByTo[edge.To][edge.Case] = true
		}
	}
	active := map[string]bool{}
	for _, node := range wf.Nodes {
		active[node.ID] = len(incomingByTo[node.ID]) == 0
	}
	for _, node := range ordered {
		nodeType := workflowNodeType(node)
		if legacyLoopTargets[node.ID] {
			reason := "循环目标工具由循环节点内嵌执行"
			record.Steps = append(record.Steps, skippedStepRecord(node, nodeType, reason))
			continue
		}
		if !active[node.ID] {
			reason := "条件分支未激活"
			record.Steps = append(record.Steps, skippedStepRecord(node, nodeType, reason))
			continue
		}
		if nodeType == config.WorkflowNodeTypeCondition {
			stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "running", StartedAt: time.Now()}
			inputValue := renderTemplate(node.Condition.Input, workflowContext)
			matchedCase := matchConditionCase(inputValue, node.Condition)
			stepRecord.ConditionInput = inputValue
			stepRecord.MatchedCase = matchedCase
			stepRecord.EndedAt = time.Now()
			stepRecord.Status = "succeeded"
			record.Steps = append(record.Steps, stepRecord)
			activateConditionBranches(node.ID, matchedCase, edgesByFrom, active)
			workflowContext["steps."+node.ID+".condition.input"] = inputValue
			workflowContext["steps."+node.ID+".condition.case"] = matchedCase
			continue
		}
		if nodeType == config.WorkflowNodeTypeParallel || nodeType == config.WorkflowNodeTypeJoin {
			stepRecord := StepRecord{ID: node.ID, Type: nodeType, Status: "succeeded", StartedAt: time.Now(), EndedAt: time.Now()}
			record.Steps = append(record.Steps, stepRecord)
			activatePlainBranches(node.ID, edgesByFrom, active)
			continue
		}
		if nodeType == config.WorkflowNodeTypeLoop {
			stepRecord := StepRecord{ID: node.ID, Type: nodeType, Tool: loopToolID(node, nodeByID), Status: "running", StartedAt: time.Now(), LoopTarget: node.Loop.Target, LoopIterations: node.Loop.MaxIterations}
			loopErr := r.executeLoop(ctx, node, nodeByID, finalParams, workflowContext, runDir, out, errOut)
			stepRecord.EndedAt = time.Now()
			if loopErr != nil {
				stepRecord.Status = "failed"
				stepRecord.Error = loopErr.Error()
				record.Steps = append(record.Steps, stepRecord)
				err = loopErr
				break
			}
			addLoopContext(workflowContext, node.ID, node.Loop.MaxIterations, runDir)
			stepRecord.Status = "succeeded"
			record.Steps = append(record.Steps, stepRecord)
			activatePlainBranches(node.ID, edgesByFrom, active)
			continue
		}
		stepParams := resolveStepParams(finalParams, workflowContext, node.Params)
		tool, toolErr := r.Registry.Tool(node.Tool)
		stepRecord := StepRecord{ID: node.ID, Type: nodeType, Tool: node.Tool, Status: "running", StartedAt: time.Now()}
		stepRunDir := filepath.Join(runDir, node.ID)
		if toolErr == nil {
			toolErr = r.executeTool(ctx, tool, stepParams, stepRunDir, out, errOut)
		}
		stepRecord.EndedAt = time.Now()
		if toolErr != nil {
			stepRecord.Status = "failed"
			stepRecord.Error = toolErr.Error()
			record.Steps = append(record.Steps, stepRecord)
			err = toolErr
			break
		}
		addStepContext(workflowContext, node.ID, stepParams, stepRunDir)
		stepRecord.Status = "succeeded"
		record.Steps = append(record.Steps, stepRecord)
		activatePlainBranches(node.ID, edgesByFrom, active)
	}
	finishRecord(record, err)
	if saveErr := r.saveRecord(runDir, record); saveErr != nil && err == nil {
		err = saveErr
	}
	return record, err
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
		stepParams := resolveStepParams(finalParams, workflowContext, loopParams)
		stepRunDir := filepath.Join(runDir, node.ID, fmt.Sprintf("%d", iteration))
		if err := r.executeTool(ctx, tool, stepParams, stepRunDir, out, errOut); err != nil {
			return fmt.Errorf("循环节点 %s 第 %d 次执行工具 %s 失败: %w", node.ID, iteration, toolID, err)
		}
		addStepContext(workflowContext, fmt.Sprintf("%s.%d", node.ID, iteration), stepParams, stepRunDir)
		addStepContext(workflowContext, node.ID, stepParams, stepRunDir)
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

func addLoopContext(context map[string]string, nodeID string, maxIterations int, runDir string) {
	context["steps."+nodeID+".loop.iterations"] = fmt.Sprint(maxIterations)
	context["steps."+nodeID+".stdout"] = strings.TrimSpace(readLoopText(runDir, nodeID, "stdout.log", maxIterations))
	context["steps."+nodeID+".stderr"] = strings.TrimSpace(readLoopText(runDir, nodeID, "stderr.log", maxIterations))
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

func (r *Runner) executeTool(ctx context.Context, tool *registry.Tool, params map[string]string, runDir string, out, errOut io.Writer) error {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	timeout := config.ParseTimeout(tool.Config.Execution.Timeout, 30*time.Minute)
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	paramFile, err := writeParamFile(runDir, tool.Config.PassMode, params)
	if err != nil {
		return err
	}
	entry := filepath.Join(tool.Dir, filepath.FromSlash(tool.Config.Execution.Entry))
	cmd := buildCommand(execCtx, entry, *tool.Config, params, paramFile)
	cmd.Dir = resolveWorkdir(tool.Dir, tool.Config.Execution.Workdir)
	cmd.Env = append(os.Environ(), encodeEnv(params)...)
	if paramFile != "" {
		cmd.Env = append(cmd.Env, "OPS_PARAM_FILE="+paramFile)
	}
	for k, v := range tool.Config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
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
		return fmt.Errorf("执行工具 %s 失败: %w", tool.Config.ID, err)
	}
	return nil
}

func buildCommand(ctx context.Context, entry string, tool config.ToolConfig, params map[string]string, paramFile string) *exec.Cmd {
	args := renderArgs(tool.Execution.Args, params)
	if len(args) == 0 && tool.PassMode.Args {
		for _, k := range sortedKeys(params) {
			args = append(args, "--"+k, params[k])
		}
	}
	if tool.PassMode.ParamFile && paramFile != "" {
		args = append(args, "--params-file", paramFile)
	}
	if runtime.GOOS == "windows" && strings.HasSuffix(entry, ".sh") {
		return exec.CommandContext(ctx, "bash", append([]string{entry}, args...)...)
	}
	return exec.CommandContext(ctx, entry, args...)
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

func writeParamFile(runDir string, mode config.PassMode, params map[string]string) (string, error) {
	if !mode.ParamFile {
		return "", nil
	}
	path := filepath.Join(runDir, filepath.Base(mode.FileName))
	data, err := yaml.Marshal(params)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
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
	out := copyParams(global)
	for k, v := range step {
		out[k] = renderTemplate(fmt.Sprint(v), templateContext)
	}
	return out
}

func addStepContext(context map[string]string, nodeID string, params map[string]string, runDir string) {
	prefix := "steps." + nodeID + "."
	for k, v := range params {
		context[prefix+"params."+k] = v
	}
	context[prefix+"stdout"] = strings.TrimSpace(readTextFile(filepath.Join(runDir, "stdout.log")))
	context[prefix+"stderr"] = strings.TrimSpace(readTextFile(filepath.Join(runDir, "stderr.log")))
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
	return os.WriteFile(filepath.Join(dir, "result.json"), data, 0o644)
}

func newRecord(kind, target string, params map[string]string) *RunRecord {
	return &RunRecord{ID: fmt.Sprintf("%s-%d", kind, time.Now().UnixNano()), Kind: kind, Target: target, Status: "running", StartedAt: time.Now(), Params: copyParams(params)}
}

func finishRecord(record *RunRecord, err error) {
	record.EndedAt = time.Now()
	if err != nil {
		record.Status = "failed"
		record.Error = err.Error()
		return
	}
	record.Status = "succeeded"
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

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
	ID        string    `json:"id"`
	Tool      string    `json:"tool"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Error     string    `json:"error,omitempty"`
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
	finalParams := config.MergeParams(wf.Config.Parameters, nil, params)
	if err := config.ValidateRequired(wf.Config.Parameters, finalParams); err != nil {
		return nil, err
	}
	record := newRecord("workflow", id, finalParams)
	runDir, err := r.prepareRun(record.ID)
	if err != nil {
		return nil, err
	}
	ordered, err := registry.OrderWorkflow(wf.Config)
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
	for _, node := range ordered {
		stepParams := resolveStepParams(finalParams, workflowContext, node.Params)
		tool, toolErr := r.Registry.Tool(node.Tool)
		stepRecord := StepRecord{ID: node.ID, Tool: node.Tool, Status: "running", StartedAt: time.Now()}
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
	}
	finishRecord(record, err)
	if saveErr := r.saveRecord(runDir, record); saveErr != nil && err == nil {
		err = saveErr
	}
	return record, err
}

func (r *Runner) validateWorkflowConfirmations(nodes []config.WorkflowNode, confirmed bool) error {
	for _, node := range nodes {
		tool, err := r.Registry.Tool(node.Tool)
		if err != nil {
			return err
		}
		if tool.Config.Confirm.Required && !node.Confirm && !confirmed {
			return fmt.Errorf("工作流节点 %s 引用的工具 %s 需要确认", node.ID, node.Tool)
		}
	}
	return nil
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

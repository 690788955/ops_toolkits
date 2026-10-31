package server

// 本文件负责运行记录读取与日志增量 tail。
import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
	"shell_ops/internal/runbundle"
	"shell_ops/internal/runner"
)

func (s *serverState) listRunSummaries(reg *registry.Registry) ([]runbundle.Summary, error) {
	s.runFileMu.Lock()
	defer s.runFileMu.Unlock()
	return runbundle.List(reg)
}

func listRuns(state *serverState, reg *registry.Registry) ([]runbundle.Summary, error) {
	items, err := state.listRunSummaries(reg)
	if err != nil {
		return nil, err
	}
	out := make([]runbundle.Summary, 0, len(items))
	for _, item := range items {
		if item.Status != "running" || state == nil || state.hasActiveRun(item.ID) {
			out = append(out, item)
			continue
		}
		record, err := state.reconcileRunRecord(reg, item.ID)
		if err != nil {
			out = append(out, item)
			continue
		}
		out = append(out, runbundle.Summarize(record))
	}
	for index := range out {
		if !out[index].AwaitingApproval {
			continue
		}
		actionable := state != nil && state.hasActiveRun(out[index].ID)
		out[index].ApprovalActionable = &actionable
	}
	return out, nil
}

func runSummaryMatches(item runbundle.Summary, keyword string) bool {
	fields := []string{item.ID, item.Kind, item.Target, item.BatchID, item.HostID, item.HostName, item.Status}
	if item.Approval != nil {
		// 审批提示里常写着要改哪个配置，搜索这张列表时应该也能命中。
		fields = append(fields, item.Approval.NodeID, item.Approval.Message)
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), keyword) {
			return true
		}
	}
	for key, value := range item.Params {
		if strings.Contains(strings.ToLower(key), keyword) || strings.Contains(strings.ToLower(value), keyword) {
			return true
		}
	}
	return false
}

// queryFlag 统一解析 1/true/yes 这类布尔查询参数。
func queryFlag(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func parseQueryInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func loadRunRecord(state *serverState, reg *registry.Registry, id string) (runner.RunRecord, error) {
	if state != nil {
		return state.reconcileRunRecord(reg, id)
	}
	return runbundle.LoadRecord(reg, id)
}

func isSummaryRunRequest(req *http.Request) bool {
	value := strings.ToLower(strings.TrimSpace(req.URL.Query().Get("summary")))
	return value == "1" || value == "true" || value == "yes"
}

func loadRunDetail(state *serverState, reg *registry.Registry, id string) (runDetail, error) {
	record, err := loadRunRecord(state, reg, id)
	if err != nil {
		return runDetail{}, err
	}
	runDir, err := runbundle.RunDir(reg, id)
	if err != nil {
		return runDetail{}, err
	}
	detail := runDetail{Record: record, Logs: loadRunLogs(runDir, record)}
	if record.Status == "running" || record.Status == runner.RunStatusPending {
		actionable := state != nil && state.hasActiveRun(id)
		detail.Actionable = &actionable
	}
	return detail, nil
}

func loadRunLogs(runDir string, record runner.RunRecord) runLogs {
	logs := runLogs{
		Stdout:   readTextFile(filepath.Join(runDir, "stdout.log")),
		Stderr:   readTextFile(filepath.Join(runDir, "stderr.log")),
		Attempts: loadAttemptLogs(runDir, record),
	}
	if len(record.Steps) == 0 {
		if logs.Stdout != "" || logs.Stderr != "" || record.Kind == "tool" {
			logs.Items = []runLogItem{{
				ID:     record.ID,
				Kind:   "tool_run",
				Title:  nonEmpty(record.Target, record.ID),
				Status: record.Status,
				Type:   "tool",
				Tool:   record.Target,
				Stdout: logs.Stdout,
				Stderr: logs.Stderr,
			}}
		}
		return logs
	}
	logs.Steps = map[string]runLogs{}
	for _, step := range record.Steps {
		logs.Steps[step.ID] = runLogs{
			Stdout: readTextFile(filepath.Join(runDir, step.ID, "stdout.log")),
			Stderr: readTextFile(filepath.Join(runDir, step.ID, "stderr.log")),
		}
	}
	logs.Items = buildRunLogItems(runDir, record, logs.Steps)
	return logs
}

func readTextFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// loadAttemptLogs 读取 attempts/<attempt-id>/ 下的恢复证据，路径离开运行目录时忽略。
func loadAttemptLogs(runDir string, record runner.RunRecord) map[string]runAttemptLogs {
	if len(record.Attempts) == 0 {
		return nil
	}
	out := make(map[string]runAttemptLogs, len(record.Attempts))
	for _, attempt := range record.Attempts {
		if strings.TrimSpace(attempt.LogDir) == "" {
			continue
		}
		dir := filepath.Join(runDir, filepath.FromSlash(attempt.LogDir))
		rel, err := filepath.Rel(runDir, dir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		logs := runAttemptLogs{Summary: readTextFile(filepath.Join(dir, "summary.log"))}
		_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return nil
			}
			name := entry.Name()
			if name != "stdout.log" && name != "stderr.log" {
				return nil
			}
			key, relErr := filepath.Rel(dir, filepath.Dir(path))
			if relErr != nil {
				return nil
			}
			key = filepath.ToSlash(key)
			if key == "." {
				key = "attempt"
			}
			if name == "stdout.log" {
				if logs.Stdout == nil {
					logs.Stdout = map[string]string{}
				}
				logs.Stdout[key] = readTextFile(path)
			} else {
				if logs.Stderr == nil {
					logs.Stderr = map[string]string{}
				}
				logs.Stderr[key] = readTextFile(path)
			}
			return nil
		})
		out[attempt.ID] = logs
	}
	return out
}

func buildRunLogItems(runDir string, record runner.RunRecord, steps map[string]runLogs) []runLogItem {
	items := make([]runLogItem, 0, len(record.Steps))
	for _, step := range record.Steps {
		stepLogs := steps[step.ID]
		item := runLogItem{
			ID:       step.ID,
			Kind:     "workflow_step",
			Title:    stepLogTitle(step),
			Status:   step.Status,
			Type:     step.Type,
			Tool:     step.Tool,
			HostID:   step.HostID,
			HostName: step.HostName,
			Stdout:   stepLogs.Stdout,
			Stderr:   stepLogs.Stderr,
		}
		if step.Type == config.WorkflowNodeTypeLoop {
			item.Children = loopLogItems(runDir, step.ID, step.LoopIterations, step.Status)
		}
		items = append(items, item)
	}
	return items
}

func stepLogTitle(step runner.StepRecord) string {
	if step.Type == config.WorkflowNodeTypeLoop && step.Tool != "" {
		return fmt.Sprintf("%s / 循环 / %s", step.ID, step.Tool)
	}
	if step.Tool != "" {
		return fmt.Sprintf("%s / %s", step.ID, step.Tool)
	}
	if step.Type != "" {
		return fmt.Sprintf("%s / %s", step.ID, step.Type)
	}
	return step.ID
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func loopLogItems(runDir, stepID string, iterations int, status string) []runLogItem {
	if iterations <= 0 {
		iterations = countLoopIterationDirs(runDir, stepID)
	}
	items := []runLogItem{}
	for iteration := 1; iteration <= iterations; iteration++ {
		dir := filepath.Join(runDir, stepID, fmt.Sprintf("%d", iteration))
		stdout := readTextFile(filepath.Join(dir, "stdout.log"))
		stderr := readTextFile(filepath.Join(dir, "stderr.log"))
		if stdout == "" && stderr == "" {
			continue
		}
		itemID := fmt.Sprintf("%s#%d", stepID, iteration)
		items = append(items, runLogItem{
			ID:        itemID,
			Kind:      "loop_iteration",
			Title:     fmt.Sprintf("%s / 第 %d 次", stepID, iteration),
			Status:    status,
			Type:      config.WorkflowNodeTypeLoop,
			Stdout:    stdout,
			Stderr:    stderr,
			Iteration: iteration,
		})
	}
	return items
}

func countLoopIterationDirs(runDir, stepID string) int {
	entries, err := os.ReadDir(filepath.Join(runDir, stepID))
	if err != nil {
		return 0
	}
	maxIteration := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var iteration int
		if _, err := fmt.Sscanf(entry.Name(), "%d", &iteration); err == nil && iteration > maxIteration {
			maxIteration = iteration
		}
	}
	return maxIteration
}

type runLogTailer struct {
	runID  string
	runDir string
	record runner.RunRecord
	offset map[string]int64
	line   map[string]int
}

type runLogSource struct {
	path      string
	itemID    string
	kind      string
	stepID    string
	iteration int
	stream    string
}

func newRunLogTailer(runID, runDir string, record runner.RunRecord) *runLogTailer {
	return &runLogTailer{runID: runID, runDir: runDir, record: record, offset: map[string]int64{}, line: map[string]int{}}
}

func (t *runLogTailer) read(status string) []runLogEvent {
	events := []runLogEvent{}
	for _, source := range t.sources() {
		lines, nextOffset := readNewLogLines(source.path, t.offset[source.path], status != "running")
		t.offset[source.path] = nextOffset
		for _, line := range lines {
			t.line[source.path]++
			events = append(events, runLogEvent{
				RunID:     t.runID,
				ItemID:    source.itemID,
				Kind:      source.kind,
				StepID:    source.stepID,
				Iteration: source.iteration,
				Stream:    source.stream,
				Line:      t.line[source.path],
				Text:      line,
				Status:    status,
			})
		}
	}
	return events
}

func (t *runLogTailer) sources() []runLogSource {
	if len(t.record.Steps) == 0 {
		return []runLogSource{
			{path: filepath.Join(t.runDir, "stdout.log"), itemID: t.record.ID, kind: "tool_run", stream: "stdout"},
			{path: filepath.Join(t.runDir, "stderr.log"), itemID: t.record.ID, kind: "tool_run", stream: "stderr"},
		}
	}
	sources := []runLogSource{}
	for _, step := range t.record.Steps {
		stepDir := filepath.Join(t.runDir, step.ID)
		sources = append(sources,
			runLogSource{path: filepath.Join(stepDir, "stdout.log"), itemID: step.ID, kind: "workflow_step", stepID: step.ID, stream: "stdout"},
			runLogSource{path: filepath.Join(stepDir, "stderr.log"), itemID: step.ID, kind: "workflow_step", stepID: step.ID, stream: "stderr"},
		)
		if step.Type == config.WorkflowNodeTypeLoop {
			iterations := step.LoopIterations
			if iterations <= 0 {
				iterations = countLoopIterationDirs(t.runDir, step.ID)
			}
			for iteration := 1; iteration <= iterations; iteration++ {
				itemID := fmt.Sprintf("%s#%d", step.ID, iteration)
				iterationDir := filepath.Join(stepDir, fmt.Sprintf("%d", iteration))
				sources = append(sources,
					runLogSource{path: filepath.Join(iterationDir, "stdout.log"), itemID: itemID, kind: "loop_iteration", stepID: step.ID, iteration: iteration, stream: "stdout"},
					runLogSource{path: filepath.Join(iterationDir, "stderr.log"), itemID: itemID, kind: "loop_iteration", stepID: step.ID, iteration: iteration, stream: "stderr"},
				)
			}
		}
	}
	return sources
}

func readNewLogLines(path string, offset int64, complete bool) ([]string, int64) {
	file, err := os.Open(path)
	if err != nil {
		return nil, offset
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, offset
	}
	if info.Size() < offset {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, offset
	}
	if !complete {
		data = data[:bytes.LastIndexAny(data, "\r\n")+1]
	}
	nextOffset := offset + int64(len(data))
	if len(data) == 0 {
		return nil, nextOffset
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nextOffset
}

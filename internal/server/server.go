package server

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
	"shell_ops/internal/runner"
)

//go:embed web/*
var webFiles embed.FS

const toolDevKitReadme = `# 运维工具开发包

这个压缩包用于分发给工具维护者。维护者只需要复制 tools/demo/sample-tool 目录，按实际分类和工具名改名后补齐 tool.yaml 与 bin/run.sh。

## 使用步骤

1. 解压压缩包。
2. 复制 tools/demo/sample-tool 到项目的 tools/<分类>/<工具>/。
3. 修改 tool.yaml 的 id、name、description、category、tags、parameters 和 execution。
4. 修改 bin/run.sh，读取 OPS_PARAM_<参数名大写> 或 OPS_PARAM_FILE。
5. 在框架项目里执行 opsctl validate 校验配置。
6. 通过 Web UI 或 opsctl run tool <工具ID> 执行验证。
`

const toolDevKitSpec = `# 工具开发规范

## 目录结构

工具必须放在 tools/<分类>/<工具>/ 下，至少包含：

- tool.yaml：工具元数据、参数、执行入口。
- bin/run.sh：实际执行脚本。
- README.md：维护说明、输入输出、回滚方式。

可选目录：

- lib/：工具内部复用脚本。
- conf/：工具配置。
- templates/：渲染模板。
- examples/：示例参数。

## tool.yaml 要求

- id 使用 <分类>.<工具>，例如 demo.sample-tool。
- category 必须和目录分类一致。
- tags 用于页面搜索和筛选。
- parameters 描述所有输入参数，必填参数设置 required: true。
- execution.entry 指向可执行脚本，例如 bin/run.sh。
- confirm.required 用于标记高风险工具是否需要确认。

## 参数传递

框架会把参数传给工具：

- 环境变量：OPS_PARAM_<参数名大写>。
- 参数文件：OPS_PARAM_FILE 指向 YAML 参数文件。
- 命令参数：开启 args 时会附加 key=value。

脚本应当优先做输入校验，失败时返回非 0 退出码。

## 输出和日志

工具写到 stdout/stderr 的内容会被框架保存到运行日志，并可在 Web UI 查看。不要在日志中输出密码、令牌或敏感凭据。

## 交付约定

工具维护者交付完整 tools/<分类>/<工具>/ 目录即可。合入框架项目后执行 opsctl validate，确认工具能被发现并通过配置校验。
`

const sampleToolYAML = `id: demo.sample-tool
name: 示例工具
description: 演示如何编写一个可被框架调用的 Shell 工具
version: 1.0.0
category: demo
tags: [demo, template]

help:
  usage: opsctl run tool demo.sample-tool --set target=<target> --no-prompt
  examples:
    - opsctl run tool demo.sample-tool --set target=demo --no-prompt

parameters:
  - name: target
    type: string
    description: 目标标识
    required: true
    default: demo
  - name: message
    type: string
    description: 输出消息
    required: false
    default: Hello

execution:
  type: shell
  entry: bin/run.sh
  timeout: 30m
  workdir: .

pass_mode:
  env: true
  args: true
  param_file: true
  file_name: params.yaml

confirm:
  required: false
`

const sampleRunScript = `#!/usr/bin/env bash
set -euo pipefail

target="${OPS_PARAM_TARGET:-}"
message="${OPS_PARAM_MESSAGE:-Hello}"

if [[ -z "$target" ]]; then
  echo "缺少目标参数 target" >&2
  exit 1
fi

echo "工具开始执行"
echo "目标: ${target}"
echo "消息: ${message}"

if [[ -n "${OPS_PARAM_FILE:-}" ]]; then
  echo "参数文件: ${OPS_PARAM_FILE}"
fi

echo "工具执行完成"
`

const sampleToolReadme = `# 示例工具

## 功能

描述这个工具解决的问题、适用场景和风险边界。

## 输入

- target：目标标识，必填。
- message：输出消息，可选。

## 输出

说明 stdout/stderr 中会出现哪些关键内容。

## 回滚

如果工具会修改系统状态，在这里写清楚回滚方式。
`

const sampleParamsYAML = `target: demo
message: Hello
`

type catalogResponse struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Categories  []config.Category      `json:"categories"`
	Tools       []toolCatalogEntry     `json:"tools"`
	Workflows   []workflowCatalogEntry `json:"workflows"`
}

type toolCatalogEntry struct {
	config.ToolEntry
	Tags       []string           `json:"tags"`
	Parameters []config.Parameter `json:"parameters"`
}

type workflowCatalogEntry struct {
	config.WorkflowRef
	Tags       []string           `json:"tags"`
	Parameters []config.Parameter `json:"parameters"`
}

type runRequest struct {
	Params map[string]string `json:"params"`
}

type workflowSaveRequest struct {
	Workflow config.WorkflowConfig `json:"workflow"`
}

type workflowValidation struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

type runLogs struct {
	Stdout string             `json:"stdout,omitempty"`
	Stderr string             `json:"stderr,omitempty"`
	Steps  map[string]runLogs `json:"steps,omitempty"`
}

type runDetail struct {
	Record runner.RunRecord `json:"record"`
	Logs   runLogs          `json:"logs"`
}

type response struct {
	ID     string      `json:"id,omitempty"`
	Status string      `json:"status,omitempty"`
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
}

func ListenAndServe(addr string, reg *registry.Registry) error {
	return http.ListenAndServe(addr, NewHandler(reg))
}

func NewHandler(reg *registry.Registry) http.Handler {
	mux := http.NewServeMux()
	r := runner.New(reg)
	registerWeb(mux)
	mux.HandleFunc("/api/catalog", catalogHandler(reg))
	mux.HandleFunc("/api/dev/toolkit.zip", toolDevKitHandler())
	mux.HandleFunc("/api/tools/", toolsHandler(reg, r))
	mux.HandleFunc("/api/workflows/", workflowsHandler(reg, r))
	mux.HandleFunc("/api/runs/", runsHandler(reg))
	return mux
}

func catalogHandler(reg *registry.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, response{Data: buildCatalog(reg)})
	}
}

func toolsHandler(reg *registry.Registry, r *runner.Runner) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/api/tools/")
		if req.Method == http.MethodGet {
			id := strings.Trim(path, "/")
			if id == "" {
				writeJSON(w, http.StatusNotFound, response{Error: "not found"})
				return
			}
			tool, err := reg.Tool(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, response{Data: tool})
			return
		}
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		id, ok := strings.CutSuffix(path, "/run")
		if !ok || id == "" {
			writeJSON(w, http.StatusNotFound, response{Error: "not found"})
			return
		}
		reqBody, err := decodeRunRequest(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		tool, err := reg.Tool(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		params := config.MergeParams(tool.Config.Parameters, nil, reqBody.Params)
		if err := config.ValidateRequired(tool.Config.Parameters, params); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		record, err := r.RunTool(context.Background(), id, params, io.Discard, io.Discard)
		writeRunResponse(w, record, err)
	}
}

func workflowsHandler(reg *registry.Registry, r *runner.Runner) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/api/workflows/")
		if req.Method == http.MethodGet {
			handleWorkflowGet(w, reg, path)
			return
		}
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if strings.HasSuffix(path, "/run") {
			handleWorkflowRun(w, req, reg, r, strings.TrimSuffix(path, "/run"))
			return
		}
		if strings.HasSuffix(path, "/validate") {
			handleWorkflowValidate(w, req, reg)
			return
		}
		if strings.HasSuffix(path, "/save") {
			handleWorkflowSave(w, req, reg, strings.TrimSuffix(path, "/save"))
			return
		}
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
	}
}

func handleWorkflowGet(w http.ResponseWriter, reg *registry.Registry, path string) {
	id := strings.Trim(path, "/")
	if id == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	wf, err := reg.Workflow(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: wf})
}

func handleWorkflowRun(w http.ResponseWriter, req *http.Request, reg *registry.Registry, r *runner.Runner, id string) {
	if id == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	reqBody, err := decodeRunRequest(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	wf, err := reg.Workflow(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	params := config.MergeParams(wf.Config.Parameters, nil, reqBody.Params)
	if err := config.ValidateRequired(wf.Config.Parameters, params); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	record, err := r.RunWorkflow(context.Background(), id, params, io.Discard, io.Discard)
	writeRunResponse(w, record, err)
}

func handleWorkflowValidate(w http.ResponseWriter, req *http.Request, reg *registry.Registry) {
	wf, err := decodeWorkflow(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: validateWorkflow(reg, wf)})
}

func handleWorkflowSave(w http.ResponseWriter, req *http.Request, reg *registry.Registry, id string) {
	wf, err := decodeWorkflow(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if id != "" && wf.ID != id {
		writeJSON(w, http.StatusBadRequest, response{Error: "workflow id does not match path"})
		return
	}
	result := validateWorkflow(reg, wf)
	if !result.Valid {
		writeJSON(w, http.StatusBadRequest, response{Data: result, Error: result.Error})
		return
	}
	path := workflowPath(reg, wf.ID)
	if err := saveWorkflow(path, wf); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	reg.Workflows[wf.ID] = &registry.Workflow{Entry: config.WorkflowRef{ID: wf.ID, Category: wf.Category, Path: relativePath(reg.BaseDir, path), Name: wf.Name, Description: wf.Description}, Config: wf, Path: path}
	writeJSON(w, http.StatusOK, response{Status: "saved", Data: reg.Workflows[wf.ID]})
}

func runsHandler(reg *registry.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		id := strings.TrimPrefix(req.URL.Path, "/api/runs/")
		detail, err := loadRunDetail(reg, id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Data: detail})
	}
}

func toolDevKitHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		data, err := buildToolDevKitZip()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="ops-tool-devkit.zip"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func loadRunDetail(reg *registry.Registry, id string) (runDetail, error) {
	cleanID := filepath.Clean(id)
	if cleanID == "." || cleanID == ".." || strings.ContainsAny(cleanID, `/\\`) {
		return runDetail{}, os.ErrNotExist
	}
	runDir := filepath.Join(reg.BaseDir, filepath.FromSlash(reg.Root.Paths.Logs), cleanID)
	data, err := os.ReadFile(filepath.Join(runDir, "result.json"))
	if err != nil {
		return runDetail{}, err
	}
	var record runner.RunRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return runDetail{}, err
	}
	return runDetail{Record: record, Logs: loadRunLogs(runDir, record)}, nil
}

func loadRunLogs(runDir string, record runner.RunRecord) runLogs {
	logs := runLogs{
		Stdout: readTextFile(filepath.Join(runDir, "stdout.log")),
		Stderr: readTextFile(filepath.Join(runDir, "stderr.log")),
	}
	if len(record.Steps) == 0 {
		return logs
	}
	logs.Steps = map[string]runLogs{}
	for _, step := range record.Steps {
		logs.Steps[step.ID] = runLogs{
			Stdout: readTextFile(filepath.Join(runDir, step.ID, "stdout.log")),
			Stderr: readTextFile(filepath.Join(runDir, step.ID, "stderr.log")),
		}
	}
	return logs
}

func readTextFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func buildToolDevKitZip() ([]byte, error) {
	files := map[string]string{
		"README.md":                               toolDevKitReadme,
		"SPEC.md":                                 toolDevKitSpec,
		"tools/demo/sample-tool/tool.yaml":        sampleToolYAML,
		"tools/demo/sample-tool/bin/run.sh":       sampleRunScript,
		"tools/demo/sample-tool/README.md":        sampleToolReadme,
		"tools/demo/sample-tool/examples/in.yaml": sampleParamsYAML,
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		file, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := file.Write([]byte(content)); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func registerWeb(mux *http.ServeMux) {
	assets, err := fs.Sub(webFiles, "web")
	if err != nil {
		return
	}
	fileServer := http.FileServer(http.FS(assets))
	mux.Handle("/assets/", fileServer)
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		data, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}

func buildCatalog(reg *registry.Registry) catalogResponse {
	out := catalogResponse{Name: reg.Root.DisplayName(), Description: reg.Root.DisplayDescription(), Categories: reg.Root.DisplayCategories()}
	for _, tool := range reg.Tools {
		out.Tools = append(out.Tools, toolCatalogEntry{ToolEntry: tool.Entry, Tags: tool.Config.Tags, Parameters: tool.Config.Parameters})
	}
	for _, wf := range reg.Workflows {
		out.Workflows = append(out.Workflows, workflowCatalogEntry{WorkflowRef: wf.Entry, Tags: wf.Config.Tags, Parameters: wf.Config.Parameters})
	}
	return out
}

func decodeWorkflow(req *http.Request) (*config.WorkflowConfig, error) {
	defer req.Body.Close()
	var body workflowSaveRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, err
	}
	config.NormalizeWorkflow(&body.Workflow)
	return &body.Workflow, nil
}

func validateWorkflow(reg *registry.Registry, wf *config.WorkflowConfig) workflowValidation {
	if err := reg.ValidateWorkflow(wf); err != nil {
		return workflowValidation{Valid: false, Error: err.Error()}
	}
	return workflowValidation{Valid: true}
}

func saveWorkflow(path string, wf *config.WorkflowConfig) error {
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
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func workflowPath(reg *registry.Registry, id string) string {
	if wf, ok := reg.Workflows[id]; ok && wf.Path != "" {
		return wf.Path
	}
	root := "workflows"
	if len(reg.Root.Paths.Workflows) > 0 {
		root = reg.Root.Paths.Workflows[0]
	}
	return filepath.Join(reg.BaseDir, filepath.FromSlash(root), id+".yaml")
}

func relativePath(baseDir, path string) string {
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func writeRunResponse(w http.ResponseWriter, record *runner.RunRecord, err error) {
	if record == nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: errorText(err)})
		return
	}
	status := http.StatusOK
	if err != nil {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, response{ID: record.ID, Status: record.Status, Error: errorText(err)})
}

func decodeRunRequest(req *http.Request) (*runRequest, error) {
	defer req.Body.Close()
	var body runRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Params == nil {
		body.Params = map[string]string{}
	}
	return &body, nil
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, response{Error: "method not allowed"})
}

func writeJSON(w http.ResponseWriter, status int, body response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

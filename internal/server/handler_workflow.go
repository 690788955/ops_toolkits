package server

// 本文件是工作流增删改查与配置文件绑定 handler。
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
	"shell_ops/internal/runner"
)

func workflowsHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reg := state.registry()
		r := state.newRunner(reg)
		path := strings.TrimPrefix(req.URL.Path, "/api/workflows/")
		if req.Method == http.MethodGet {
			if strings.HasSuffix(path, "/files") || strings.Contains(path, "/files/") {
				handleWorkflowFilesRoute(w, req, state, path)
				return
			}
			handleWorkflowGet(w, reg, path)
			return
		}
		if (req.Method == http.MethodPut || req.Method == http.MethodDelete) && (strings.HasSuffix(path, "/files") || strings.Contains(path, "/files/")) {
			handleWorkflowFilesRoute(w, req, state, path)
			return
		}
		if req.Method == http.MethodDelete {
			handleWorkflowDelete(w, reg, path)
			return
		}
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if strings.HasSuffix(path, "/run") {
			handleWorkflowRun(w, req, state, reg, r, strings.TrimSuffix(path, "/run"))
			return
		}
		if strings.HasSuffix(path, "/files") || strings.Contains(path, "/files/") {
			handleWorkflowFilesRoute(w, req, state, path)
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

func handleWorkflowFilesRoute(w http.ResponseWriter, req *http.Request, state *serverState, path string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[1] != "files" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	workflowID := parts[0]
	if len(parts) == 2 {
		handleWorkflowConfigFiles(w, req, state, workflowID)
		return
	}
	if req.Method == http.MethodPost && len(parts) == 3 && parts[2] == "upload" {
		handleWorkflowConfigFileUpload(w, req, state, workflowID)
		return
	}
	fileID, err := url.PathUnescape(strings.Join(parts[2:], "/"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件 ID 格式错误"})
		return
	}
	handleWorkflowConfigFile(w, req, state, workflowID, fileID)
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

func handleWorkflowRun(w http.ResponseWriter, req *http.Request, state *serverState, reg *registry.Registry, r *runner.Runner, id string) {
	if id == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	reqBody, err := decodeRunRequest(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	workflowConfig := reqBody.Workflow
	if workflowConfig == nil {
		wf, err := reg.Workflow(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		workflowConfig = wf.Config
	}
	params := config.ValuesToStringMap(config.MergeParamsValues(workflowConfig.Parameters, nil, config.InterfaceMapToValues(reqBody.Params)))
	if err := config.ValidateRequired(workflowConfig.Parameters, params); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	workflowConfig, err = applyWorkflowRunTarget(state, workflowConfig, reqBody.Target, reqBody.OverrideNodes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if err := validateWorkflowTargets(state, workflowConfig); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if workflowConfig.Confirm.Required && !reqBody.Confirm {
		writeJSON(w, http.StatusBadRequest, response{Error: "该工作流需要确认后执行"})
		return
	}
	if err := confirmWorkflowTools(reg, workflowConfig, reqBody.Confirm); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if isAsyncRunRequest(req) {
		ctx, cancel := context.WithCancel(context.Background())
		record, err := r.StartWorkflowConfigWithUploads(ctx, workflowConfig, params, reqBody.Confirm, reqBody.Uploads, io.Discard, io.Discard)
		if err != nil {
			cancel()
			writeRunResponse(w, record, err)
			return
		}
		state.registerRun(record.ID, cancel)
		go watchRunCompletion(state, reg, record.ID, cancel)
		writeRunResponse(w, record, err)
		return
	}
	if reqBody.Workflow != nil {
		record, err := r.RunWorkflowConfigWithUploads(context.Background(), workflowConfig, params, reqBody.Confirm, reqBody.Uploads, io.Discard, io.Discard)
		writeRunResponse(w, record, err)
		return
	}
	record, err := r.RunWorkflowConfigWithUploads(context.Background(), workflowConfig, params, reqBody.Confirm, reqBody.Uploads, io.Discard, io.Discard)
	writeRunResponse(w, record, err)
}

func watchRunCompletion(state *serverState, reg *registry.Registry, runID string, cancel context.CancelFunc) {
	readFailures := 0
	for {
		time.Sleep(100 * time.Millisecond)
		record, err := loadRunRecord(nil, reg, runID)
		if err != nil {
			readFailures++
			if readFailures < 5 {
				continue
			}
			state.finishRun(runID)
			cancel()
			return
		}
		readFailures = 0
		if record.Status != "running" {
			state.finishRun(runID)
			cancel()
			return
		}
	}
}

func isAsyncRunRequest(req *http.Request) bool {
	value := strings.ToLower(strings.TrimSpace(req.URL.Query().Get("async")))
	return value == "1" || value == "true" || value == "yes"
}

// workflowRunTargetFollowNodes 表示运行时不指定统一目标，沿用各节点自己的 host 配置。
const workflowRunTargetFollowNodes = "follow_nodes"

// applyWorkflowRunTarget 把“执行这一步选中的机器”应用到一个工作流副本上。
// target 为空 / follow_nodes 时不做任何改动；override 为真时覆盖节点已有 host。
func applyWorkflowRunTarget(state *serverState, wf *config.WorkflowConfig, rawTarget string, override bool) (*config.WorkflowConfig, error) {
	if wf == nil {
		return nil, nil
	}
	target := strings.TrimSpace(rawTarget)
	if target == "" || target == workflowRunTargetFollowNodes {
		return wf, nil
	}
	if target == "local" || target == runner.LocalTargetID {
		// 本机在节点层面就是“host 留空”，与节点配置里的“本机执行”保持一致。
		target = ""
	}
	if target != "" {
		if err := validateRemoteTarget(state, target); err != nil {
			return nil, fmt.Errorf("工作流运行目标无效: %w", err)
		}
	}
	clone := cloneWorkflowConfig(wf)
	if clone == nil {
		return wf, nil
	}
	for i := range clone.Nodes {
		node := &clone.Nodes[i]
		if node.Host != "" && !override {
			continue
		}
		node.Host = target
	}
	return clone, nil
}

func cloneWorkflowConfig(wf *config.WorkflowConfig) *config.WorkflowConfig {
	if wf == nil {
		return nil
	}
	data, err := json.Marshal(wf)
	if err != nil {
		return nil
	}
	var out config.WorkflowConfig
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	out.Steps = nil
	return &out
}

func validateWorkflowTargets(state *serverState, wf *config.WorkflowConfig) error {
	if wf == nil {
		return nil
	}
	nodes := make(map[string]config.WorkflowNode, len(wf.Nodes))
	for _, node := range wf.Nodes {
		nodes[node.ID] = node
	}
	for _, node := range wf.Nodes {
		hostID := node.Host
		if hostID == "" && node.Loop.Tool == "" && node.Loop.Target != "" {
			hostID = nodes[node.Loop.Target].Host
		}
		if hostID == "" {
			continue
		}
		if err := validateRemoteTarget(state, hostID); err != nil {
			return fmt.Errorf("工作流节点 %s 的执行目标无效: %w", node.ID, err)
		}
	}
	return nil
}

func validateRemoteTarget(state *serverState, hostID string) error {
	hostID = strings.TrimSpace(hostID)
	if hostID == "" {
		return nil
	}
	if state == nil || state.ssh == nil {
		return fmt.Errorf("未配置 SSH 操作机管理器")
	}
	host, err := state.ssh.HostByID(hostID)
	if err != nil {
		return err
	}
	if !host.IsEnabled() {
		return fmt.Errorf("操作机 %s 已停用", host.Name)
	}
	return nil
}

func confirmWorkflowTools(reg *registry.Registry, wf *config.WorkflowConfig, confirmed bool) error {
	for _, node := range wf.Nodes {
		nodeType := node.Type
		if nodeType == "" && node.Tool != "" {
			nodeType = config.WorkflowNodeTypeTool
		}
		if nodeType == "" && (node.Loop.Tool != "" || node.Loop.Target != "" || node.Loop.MaxIterations != 0 || len(node.Loop.Params) > 0) {
			nodeType = config.WorkflowNodeTypeLoop
		}
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
		tool, err := reg.Tool(toolID)
		if err != nil {
			return err
		}
		if tool.Config.Confirm.Required && !node.Confirm && !confirmed {
			return fmt.Errorf("工作流节点 %s 引用的工具 %s 需要确认", node.ID, toolID)
		}
	}
	return nil
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
	wf, ownerPluginID, err := decodeWorkflowSaveRequest(req)
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
	if ownerPluginID != nil {
		if existing := reg.Workflows[wf.ID]; existing != nil && existing.Source.Type != "plugin" {
			writeJSON(w, http.StatusBadRequest, response{Error: "内置工作流不支持修改归属插件"})
			return
		}
		if _, err := workflowOwnerForPlugin(reg, strings.TrimSpace(*ownerPluginID)); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
	}
	if err := normalizeWorkflowSharedWith(reg, wf, ownerPluginID); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	path, source, err := saveWorkflowAsset(reg, wf, ownerPluginID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	reg.Workflows[wf.ID] = &registry.Workflow{Entry: workflowEntryForSavedWorkflow(reg, path, wf), Config: wf, Path: path, Source: source}
	writeJSON(w, http.StatusOK, response{Status: "saved", Data: reg.Workflows[wf.ID]})
}

func handleWorkflowDelete(w http.ResponseWriter, reg *registry.Registry, path string) {
	id := strings.Trim(path, "/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	wf, err := reg.Workflow(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	if wf.Source.Type != "plugin" || strings.TrimSpace(wf.Source.PluginID) == "" {
		writeJSON(w, http.StatusBadRequest, response{Error: "只能删除插件中的工作流"})
		return
	}
	owner, err := workflowOwnerForPlugin(reg, wf.Source.PluginID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	if !pathWithinDir(owner.Dir, wf.Path) {
		writeJSON(w, http.StatusBadRequest, response{Error: "工作流文件不在插件目录内，无法删除"})
		return
	}
	if err := os.Remove(wf.Path); err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	if owner.ID == userWorkflowPluginID {
		if err := os.RemoveAll(workflowConfigDir(reg, id)); err != nil && !os.IsNotExist(err) {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
		if err := maintainUserWorkflowPluginManifest(reg); err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
	} else {
		rel, err := filepath.Rel(owner.Dir, wf.Path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
		if err := updatePluginWorkflowContribution(owner.Dir, "", filepath.ToSlash(rel)); err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
	}
	delete(reg.Workflows, id)
	writeJSON(w, http.StatusOK, response{Status: "deleted", ID: id})
}

func handleWorkflowConfigFiles(w http.ResponseWriter, req *http.Request, state *serverState, workflowID string) {
	switch req.Method {
	case http.MethodGet:
		reg := state.registry()
		wf, err := userWorkflow(reg, workflowID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		files, err := scanWorkflowConfigFiles(reg, wf.Config.ID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"files": files}})
	case http.MethodPost:
		handleBindWorkflowConfigFile(w, req, state, workflowID)
	default:
		methodNotAllowed(w)
	}
}

func handleBindWorkflowConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, workflowID string) {
	defer req.Body.Close()
	var body struct {
		Path  string `json:"path"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "请求格式错误"})
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	reg := state.reg
	wf, err := userWorkflow(reg, workflowID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	entry, filePath, err := bindWorkflowConfigPath(reg, wf, body.Path, body.Label)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	managed := pathWithinDir(workflowConfigDir(reg, wf.Config.ID), filePath)
	writeJSON(w, http.StatusOK, response{Status: "bound", Data: map[string]interface{}{"file": workflowConfigFileStatus(entry, filePath, "", managed)}})
}

func handleWorkflowConfigFileUpload(w http.ResponseWriter, req *http.Request, state *serverState, workflowID string) {
	req.Body = http.MaxBytesReader(w, req.Body, maxPluginConfigFileBytes+(2<<20))
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "读取上传文件失败: " + err.Error()})
		return
	}
	if req.MultipartForm != nil {
		defer req.MultipartForm.RemoveAll()
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	reg := state.reg
	wf, err := userWorkflow(reg, workflowID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	file, header, err := req.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "请选择要上传的配置文件"})
		return
	}
	defer file.Close()
	name := sanitizeUploadedFileName(header.Filename)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, response{Error: "上传文件名无效"})
		return
	}
	targetDir, err := workflowConfigUploadDir(reg, wf.Config.ID, req.FormValue("subdir"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	targetPath := filepath.Join(targetDir, name)
	if _, err := os.Stat(targetPath); err == nil {
		writeJSON(w, http.StatusBadRequest, response{Error: fmt.Sprintf("配置文件 %s 已存在，请直接绑定该文件或修改文件名", filepath.ToSlash(filepath.Join(req.FormValue("subdir"), name)))})
		return
	} else if !os.IsNotExist(err) {
		writeJSON(w, http.StatusBadRequest, response{Error: fmt.Sprintf("检查配置文件失败: %v", err)})
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxPluginConfigFileBytes+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "读取上传文件失败: " + err.Error()})
		return
	}
	if int64(len(data)) > maxPluginConfigFileBytes {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件超过大小限制"})
		return
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("创建配置目录失败: %v", err)})
		return
	}
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("保存上传文件失败: %v", err)})
		return
	}
	entry, err := bindWorkflowConfigAbsPath(reg, wf, targetPath, req.FormValue("label"), targetPath)
	if err != nil {
		_ = os.Remove(targetPath)
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "bound", Data: map[string]interface{}{"file": workflowConfigFileStatus(entry, targetPath, "", true)}})
}

func handleWorkflowConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, workflowID, fileID string) {
	switch req.Method {
	case http.MethodGet:
		handleGetWorkflowConfigFile(w, state, workflowID, fileID)
	case http.MethodPut:
		handleSaveWorkflowConfigFile(w, req, state, workflowID, fileID)
	case http.MethodDelete:
		handleDeleteWorkflowConfigFile(w, state, workflowID, fileID)
	default:
		methodNotAllowed(w)
	}
}

func handleGetWorkflowConfigFile(w http.ResponseWriter, state *serverState, workflowID, fileID string) {
	reg := state.registry()
	entry, _, err := declaredWorkflowConfigFile(reg, workflowID, fileID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	filePath, err := resolvedWorkflowConfigFilePath(reg, workflowID, entry)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	content, err := readConfigFileContent(filePath)
	if err != nil {
		if os.IsNotExist(err) && entry.Create {
			writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"content": ""}})
			return
		}
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"content": content}})
}

func handleSaveWorkflowConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, workflowID, fileID string) {
	defer req.Body.Close()
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "请求格式错误"})
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	entry, _, err := declaredWorkflowConfigFile(state.reg, workflowID, fileID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if entry.Access != config.ConfigFileAccessReadWrite {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件声明为只读，不能保存"})
		return
	}
	if int64(len([]byte(body.Content))) > maxPluginConfigFileBytes {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件内容超过大小限制"})
		return
	}
	filePath, err := resolvedWorkflowConfigFilePath(state.reg, workflowID, entry)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if err := writeConfigFileContent(filePath, body.Content, entry.Create); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"message": "工作流配置文件已保存"}})
}

func handleDeleteWorkflowConfigFile(w http.ResponseWriter, state *serverState, workflowID, fileID string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	entry, wf, err := declaredWorkflowConfigFile(state.reg, workflowID, fileID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	managedPath := ""
	if entry.Scope != config.ConfigFileScopeHostAbsolute {
		if filePath, err := resolvedWorkflowConfigFilePath(state.reg, workflowID, entry); err == nil && pathWithinDir(workflowConfigDir(state.reg, workflowID), filePath) {
			managedPath = filePath
		}
	}
	if wf != nil && wf.Config != nil && len(wf.Config.ConfigFiles) > 0 {
		filtered := wf.Config.ConfigFiles[:0]
		for _, item := range wf.Config.ConfigFiles {
			normalized := normalizeWorkflowConfigFileRef(wf.Config.ID, item)
			if normalized.ID == fileID {
				continue
			}
			filtered = append(filtered, item)
		}
		wf.Config.ConfigFiles = filtered
		if err := saveWorkflowAssetForWorkflow(state.reg, wf.Config); err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("更新工作流配置声明失败: %v", err)})
			return
		}
		state.reg.Workflows[wf.Config.ID] = wf
	}
	managed := managedPath != ""
	if managed {
		if err := os.Remove(managedPath); err != nil && !os.IsNotExist(err) {
			writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("删除工作流配置文件失败: %v", err)})
			return
		}
	}
	message := "工作流配置文件已删除"
	if !managed {
		message = "已解除绑定，原文件保留"
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"message": message, "file_removed": managed}})
}

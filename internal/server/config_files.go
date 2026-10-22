package server

// 本文件负责插件与工作流配置文件的读写、绑定与状态校验。
import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"shell_ops/internal/config"
	"shell_ops/internal/plugin"
	"shell_ops/internal/registry"
)

type pluginConfigFileStatus struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	ConfigDir   string `json:"config_dir,omitempty"`
	DisplayRoot string `json:"display_root,omitempty"`
	Path        string `json:"path"`
	DisplayPath string `json:"display_path,omitempty"`
	Scope       string `json:"scope"`
	Access      string `json:"access"`
	Create      bool   `json:"create"`
	Managed     bool   `json:"managed"`
	Exists      bool   `json:"exists"`
	Readable    bool   `json:"readable"`
	Writable    bool   `json:"writable"`
	Reason      string `json:"reason,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

func handlePluginConfigFiles(w http.ResponseWriter, req *http.Request, state *serverState, pluginID string) {
	if req.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	reg := state.registry()
	seen := map[string]bool{}
	var files []pluginConfigFileStatus
	for _, tool := range reg.Tools {
		if tool.Source.PluginID != pluginID {
			continue
		}
		for _, declared := range declaredConfigFiles(tool.Config) {
			expanded, err := expandDeclaredConfigFiles(reg.Root, tool.Config, declared)
			if err != nil {
				entry := normalizeServerConfigFileRef(declared)
				id := entry.ID
				if id == "" {
					id = entry.Path
				}
				if id == "" || seen[id] {
					continue
				}
				seen[id] = true
				files = append(files, pluginConfigFileStatus{ID: id, Label: entry.Label, ConfigDir: entry.ConfigDir, Path: entry.Path, Scope: entry.Scope, Access: entry.Access, Create: entry.Create, Reason: err.Error()})
				continue
			}
			for _, entry := range expanded {
				if entry.ID == "" || seen[entry.ID] {
					continue
				}
				seen[entry.ID] = true
				filePath, err := resolvedConfigFilePath(reg.Root, tool.Config, entry)
				if err != nil {
					files = append(files, pluginConfigFileStatus{ID: entry.ID, Label: entry.Label, ConfigDir: entry.ConfigDir, Path: entry.Path, Scope: entry.Scope, Access: entry.Access, Create: entry.Create, Reason: err.Error()})
					continue
				}
				files = append(files, configFileStatus(tool.Config, entry, filePath))
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ID < files[j].ID })

	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"files": files}})
}

func handlePluginConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, pluginID, fileID string) {
	switch req.Method {
	case http.MethodGet:
		handleGetPluginConfigFile(w, req, state, pluginID, fileID)
	case http.MethodPut:
		handleSavePluginConfigFile(w, req, state, pluginID, fileID)
	case http.MethodDelete:
		handleDeletePluginConfigFile(w, req, state, pluginID, fileID)
	default:
		methodNotAllowed(w)
	}
}

func handleGetPluginConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, pluginID, fileID string) {
	entry, toolCfg, err := declaredPluginConfigFile(state.registry(), pluginID, fileID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	filePath, err := resolvedConfigFilePath(state.registry().Root, toolCfg, entry)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) && entry.Scope == config.ConfigFileScopePlugin {
			writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"content": ""}})
			return
		}
		writeJSON(w, http.StatusBadRequest, response{Error: fmt.Sprintf("配置文件不可读: %v", err)})
		return
	}
	if !info.Mode().IsRegular() {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件不是普通文件"})
		return
	}
	if info.Size() > maxPluginConfigFileBytes {
		writeJSON(w, http.StatusBadRequest, response{Error: "配置文件超过大小限制"})
		return
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("读取配置文件失败: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"content": string(data)}})
}

func handleSavePluginConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, pluginID, fileID string) {
	defer req.Body.Close()
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "请求格式错误"})
		return
	}

	entry, toolCfg, err := declaredPluginConfigFile(state.registry(), pluginID, fileID)
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
	filePath, err := resolvedConfigFilePath(state.registry().Root, toolCfg, entry)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	info, err := os.Stat(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("检查配置文件失败: %v", err)})
			return
		}
		if entry.Scope == config.ConfigFileScopeHostAbsolute {
			if !entry.Create {
				writeJSON(w, http.StatusBadRequest, response{Error: "配置文件不存在且声明不允许创建"})
				return
			}
			if !parentWritable(filepath.Dir(filePath)) {
				writeJSON(w, http.StatusBadRequest, response{Error: "配置文件父目录不可写或不存在"})
				return
			}
		} else if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("创建配置目录失败: %v", err)})
			return
		}
	} else {
		if !info.Mode().IsRegular() {
			writeJSON(w, http.StatusBadRequest, response{Error: "配置文件不是普通文件"})
			return
		}
		file, err := os.OpenFile(filePath, os.O_WRONLY, 0)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: fmt.Sprintf("配置文件不可写: %v", err)})
			return
		}
		_ = file.Close()
	}
	if err := os.WriteFile(filePath, []byte(body.Content), 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("保存配置文件失败: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"message": "配置文件已保存"}})
}

func handleDeletePluginConfigFile(w http.ResponseWriter, req *http.Request, state *serverState, pluginID, fileID string) {
	entry, toolCfg, err := declaredPluginConfigFile(state.registry(), pluginID, fileID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if entry.Scope == config.ConfigFileScopeHostAbsolute {
		writeJSON(w, http.StatusBadRequest, response{Error: "宿主绝对路径配置文件默认不支持删除"})
		return
	}
	filePath, err := resolvedConfigFilePath(state.registry().Root, toolCfg, entry)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("删除配置文件失败: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"message": "配置文件已删除"}})
}

func declaredPluginConfigFile(reg *registry.Registry, pluginID, fileID string) (config.ConfigFileRef, *config.ToolConfig, error) {
	if strings.TrimSpace(fileID) == "" || strings.Contains(fileID, "\x00") {
		return config.ConfigFileRef{}, nil, fmt.Errorf("配置文件 ID 不安全")
	}
	for _, tool := range reg.Tools {
		if tool.Source.PluginID != pluginID {
			continue
		}
		for _, declared := range declaredConfigFiles(tool.Config) {
			declared = normalizeServerConfigFileRef(declared)
			if declared.ID == fileID {
				return declared, tool.Config, nil
			}
			expanded, err := expandDeclaredConfigFiles(reg.Root, tool.Config, declared)
			if err != nil {
				continue
			}
			for _, entry := range expanded {
				if entry.ID != fileID {
					continue
				}
				return entry, tool.Config, nil
			}
		}
	}
	return config.ConfigFileRef{}, nil, fmt.Errorf("配置文件 %s 未在插件 %s 中声明", fileID, pluginID)
}

func workflowReferencedPlugins(reg *registry.Registry, wf *config.WorkflowConfig) []workflowReferencedPlugin {
	if reg == nil || wf == nil {
		return nil
	}
	toolIDs := make([]string, 0, len(wf.Nodes))
	seenTools := make(map[string]bool, len(wf.Nodes))
	for _, node := range wf.Nodes {
		for _, raw := range []string{node.Tool, node.Loop.Tool} {
			id := strings.TrimSpace(raw)
			if id == "" || seenTools[id] {
				continue
			}
			seenTools[id] = true
			toolIDs = append(toolIDs, id)
		}
	}
	if len(toolIDs) == 0 {
		return nil
	}
	packages := make(map[string]plugin.Package)
	for _, pkg := range installedAnyPluginPackages(reg) {
		packages[pkg.Manifest.ID] = pkg
	}
	byPlugin := make(map[string]*workflowReferencedPlugin)
	order := make([]string, 0, len(toolIDs))
	for _, toolID := range toolIDs {
		tool, err := reg.Tool(toolID)
		if err != nil || tool == nil {
			continue
		}
		pluginID := strings.TrimSpace(tool.Source.PluginID)
		if pluginID == "" || pluginID == userWorkflowPluginID {
			continue
		}
		entry, ok := byPlugin[pluginID]
		if !ok {
			entry = &workflowReferencedPlugin{ID: pluginID}
			if pkg, found := packages[pluginID]; found {
				entry.Name = pkg.Manifest.Name
				entry.Version = pkg.Manifest.Version
				entry.Disabled = pluginDisabled(reg.Root.Plugins.Disabled, pkg)
			}
			byPlugin[pluginID] = entry
			order = append(order, pluginID)
		}
		entry.Tools = append(entry.Tools, toolID)
		seenFiles := make(map[string]bool, len(entry.ConfigFiles))
		for _, file := range entry.ConfigFiles {
			seenFiles[file.ID] = true
		}
		for _, file := range declaredConfigFiles(tool.Config) {
			if file.ID == "" || seenFiles[file.ID] {
				continue
			}
			seenFiles[file.ID] = true
			entry.ConfigFiles = append(entry.ConfigFiles, file)
		}
	}
	out := make([]workflowReferencedPlugin, 0, len(order))
	for _, pluginID := range order {
		out = append(out, *byPlugin[pluginID])
	}
	return out
}

func declaredConfigFiles(toolCfg *config.ToolConfig) []config.ConfigFileRef {
	if toolCfg == nil {
		return nil
	}
	if len(toolCfg.ConfigFileRefs) > 0 {
		out := make([]config.ConfigFileRef, 0, len(toolCfg.ConfigFileRefs))
		for _, entry := range toolCfg.ConfigFileRefs {
			out = append(out, normalizeServerConfigFileRef(entry))
		}
		return out
	}
	out := make([]config.ConfigFileRef, 0, len(toolCfg.ConfigFiles))
	for _, path := range toolCfg.ConfigFiles {
		out = append(out, config.NewPluginConfigFileRef(path))
	}
	return out
}

func bindWorkflowConfigPath(reg *registry.Registry, wf *registry.Workflow, rawPath, label string) (config.ConfigFileRef, string, error) {
	filePath, _, err := resolveWorkflowConfigBindPath(reg, rawPath)
	if err != nil {
		return config.ConfigFileRef{}, "", err
	}
	entry, err := bindWorkflowConfigAbsPath(reg, wf, filePath, label, filePath)
	if err != nil {
		return config.ConfigFileRef{}, "", err
	}
	return entry, filePath, nil
}

func bindWorkflowConfigAbsPath(reg *registry.Registry, wf *registry.Workflow, filePath, label, skipScannedPath string) (config.ConfigFileRef, error) {
	external := !pathWithinDir(reg.BaseDir, filePath)
	if external {
		resolvedPath, err := resolveWorkflowHostConfigPath(reg, filePath)
		if err != nil {
			return config.ConfigFileRef{}, err
		}
		filePath = resolvedPath
	}
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return config.ConfigFileRef{}, fmt.Errorf("配置文件不存在: %s", filepath.ToSlash(filePath))
		}
		return config.ConfigFileRef{}, fmt.Errorf("检查配置文件失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return config.ConfigFileRef{}, fmt.Errorf("配置文件路径不是普通文件")
	}
	if info.Size() > maxPluginConfigFileBytes {
		return config.ConfigFileRef{}, fmt.Errorf("配置文件超过大小限制")
	}
	entries, err := canonicalWorkflowConfigEntries(reg, wf, skipScannedPath)
	if err != nil {
		return config.ConfigFileRef{}, err
	}
	for _, entry := range entries {
		resolved, err := resolvedWorkflowConfigFilePath(reg, wf.Config.ID, entry)
		if err == nil && sameWorkflowConfigPath(resolved, filePath) {
			return config.ConfigFileRef{}, fmt.Errorf("配置文件 %s 已绑定", filepath.ToSlash(entry.Path))
		}
	}
	id := workflowConfigBindID(reg, wf.Config.ID, filePath)
	if external {
		id = externalWorkflowConfigID(filePath)
	}
	for _, entry := range entries {
		if entry.ID == id {
			return config.ConfigFileRef{}, fmt.Errorf("配置文件 ID %s 已存在", id)
		}
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = filepath.Base(filePath)
	}
	entry := config.ConfigFileRef{
		ID:     id,
		Label:  label,
		Scope:  config.ConfigFileScopePlugin,
		Access: config.ConfigFileAccessReadWrite,
		Create: true,
	}
	if external {
		entry.ConfigDir = filepath.Dir(filePath)
		entry.Path = filepath.Base(filePath)
		entry.Scope = config.ConfigFileScopeHostAbsolute
		entry.Create = false
	} else {
		rel, err := filepath.Rel(reg.BaseDir, filePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
			return config.ConfigFileRef{}, fmt.Errorf("配置文件路径逃逸运行根目录")
		}
		entry.ConfigDir = "."
		entry.Path = filepath.ToSlash(rel)
	}
	original := wf.Config.ConfigFiles
	wf.Config.ConfigFiles = append(entries, entry)
	if err := saveWorkflowAssetForWorkflow(reg, wf.Config); err != nil {
		wf.Config.ConfigFiles = original
		return config.ConfigFileRef{}, fmt.Errorf("保存工作流配置声明失败: %w", err)
	}
	return entry, nil
}

func canonicalWorkflowConfigEntries(reg *registry.Registry, wf *registry.Workflow, skipScannedPath string) ([]config.ConfigFileRef, error) {
	if len(wf.Config.ConfigFiles) > 0 {
		out := make([]config.ConfigFileRef, 0, len(wf.Config.ConfigFiles))
		for _, item := range wf.Config.ConfigFiles {
			entry := normalizeWorkflowConfigFileRef(wf.Config.ID, item)
			entry.Legacy = false
			if entry.Scope == config.ConfigFileScopeHostAbsolute {
				out = append(out, entry)
				continue
			}
			filePath, err := resolvedWorkflowConfigFilePath(reg, wf.Config.ID, entry)
			if err != nil {
				return nil, fmt.Errorf("配置文件 %s 路径无效: %w", entry.ID, err)
			}
			rel, err := filepath.Rel(reg.BaseDir, filePath)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
				return nil, fmt.Errorf("配置文件 %s 路径逃逸运行根目录", entry.ID)
			}
			entry.ConfigDir = "."
			entry.Path = filepath.ToSlash(rel)
			out = append(out, entry)
		}
		return out, nil
	}
	scanned, err := scanWorkflowConfigDirFiles(reg, wf.Config.ID)
	if err != nil {
		return nil, err
	}
	root := workflowConfigDir(reg, wf.Config.ID)
	out := make([]config.ConfigFileRef, 0, len(scanned)+1)
	for _, item := range scanned {
		filePath := filepath.Join(root, filepath.FromSlash(item.Path))
		if skipScannedPath != "" && sameWorkflowConfigPath(filePath, skipScannedPath) {
			continue
		}
		rel, err := filepath.Rel(reg.BaseDir, filePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
			return nil, fmt.Errorf("配置文件 %s 路径逃逸运行根目录", item.ID)
		}
		out = append(out, config.ConfigFileRef{
			ID:        item.ID,
			Label:     item.Label,
			ConfigDir: ".",
			Path:      filepath.ToSlash(rel),
			Scope:     config.ConfigFileScopePlugin,
			Access:    config.ConfigFileAccessReadWrite,
			Create:    true,
		})
	}
	return out, nil
}

func resolveWorkflowConfigBindPath(reg *registry.Registry, rawPath string) (string, string, error) {
	rawPath = strings.TrimSpace(strings.ReplaceAll(rawPath, "\\", "/"))
	if rawPath == "" {
		return "", "", fmt.Errorf("配置文件路径必填")
	}
	if strings.Contains(rawPath, "\x00") || strings.Contains(rawPath, "://") {
		return "", "", fmt.Errorf("配置文件路径格式无效")
	}
	localPath := filepath.FromSlash(rawPath)
	if filepath.IsAbs(localPath) || strings.HasPrefix(rawPath, "/") {
		filePath, err := filepath.Abs(localPath)
		if err != nil {
			return "", "", err
		}
		return filePath, filepath.ToSlash(filePath), nil
	}
	if len(rawPath) >= 2 && rawPath[1] == ':' {
		return "", "", fmt.Errorf("配置文件路径格式无效")
	}
	for _, part := range strings.FieldsFunc(rawPath, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "" || part == "." || part == ".." {
			return "", "", fmt.Errorf("配置文件路径包含不安全路径片段")
		}
	}
	filePath, err := filepath.Abs(filepath.Join(reg.BaseDir, localPath))
	if err != nil {
		return "", "", err
	}
	if !pathWithin(reg.BaseDir, filePath) {
		return "", "", fmt.Errorf("配置文件路径逃逸运行根目录")
	}
	rel, err := filepath.Rel(reg.BaseDir, filePath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("配置文件路径逃逸运行根目录")
	}
	return filePath, filepath.ToSlash(rel), nil
}

func workflowConfigUploadDir(reg *registry.Registry, workflowID, subdir string) (string, error) {
	root := workflowConfigDir(reg, workflowID)
	subdir = strings.TrimSpace(strings.ReplaceAll(subdir, "\\", "/"))
	if subdir == "" || subdir == "." {
		return root, nil
	}
	if strings.Contains(subdir, "\x00") || strings.Contains(subdir, "://") || strings.HasPrefix(subdir, "/") || filepath.IsAbs(subdir) || len(subdir) >= 2 && subdir[1] == ':' {
		return "", fmt.Errorf("保存子目录必须是工作流配置目录内的相对路径")
	}
	for _, part := range strings.Split(subdir, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("保存子目录包含不安全路径片段")
		}
	}
	dir := filepath.Join(root, filepath.FromSlash(subdir))
	if !pathWithinDir(root, dir) {
		return "", fmt.Errorf("保存子目录逃逸工作流配置目录")
	}
	return dir, nil
}

func workflowConfigBindID(reg *registry.Registry, workflowID, filePath string) string {
	root := workflowConfigDir(reg, workflowID)
	if rel, err := filepath.Rel(root, filePath); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel) {
		return filepath.ToSlash(rel)
	}
	rel, err := filepath.Rel(reg.BaseDir, filePath)
	if err != nil {
		return filepath.ToSlash(filepath.Base(filePath))
	}
	return filepath.ToSlash(rel)
}

func externalWorkflowConfigID(filePath string) string {
	cleaned := filepath.ToSlash(filepath.Clean(filePath))
	return "host:" + strings.TrimPrefix(cleaned, "/")
}

func sameWorkflowConfigPath(left, right string) bool {
	leftAbs, err := filepath.Abs(left)
	if err != nil {
		return false
	}
	rightAbs, err := filepath.Abs(right)
	if err != nil {
		return false
	}
	leftAbs = filepath.Clean(leftAbs)
	rightAbs = filepath.Clean(rightAbs)
	if filepath.Separator == '\\' {
		return strings.EqualFold(leftAbs, rightAbs)
	}
	return leftAbs == rightAbs
}

func workflowConfigDirRef(workflowID string) string {
	return filepath.ToSlash(filepath.Join("config", "workflows", workflowFilenameBase(workflowID)))
}

func workflowFilenameBase(id string) string {
	return strings.TrimSuffix(workflowFilename(id), ".yaml")
}

func workflowConfigDir(reg *registry.Registry, workflowID string) string {
	return filepath.Join(userWorkflowPluginDir(reg), "config", "workflows", workflowFilenameBase(workflowID))
}

func resolvedWorkflowConfigFilePath(reg *registry.Registry, workflowID string, entry config.ConfigFileRef) (string, error) {
	entry = normalizeWorkflowConfigFileRef(workflowID, entry)
	if entry.Scope == config.ConfigFileScopeHostAbsolute {
		if !filepath.IsAbs(strings.TrimSpace(entry.ConfigDir)) {
			return "", fmt.Errorf("工作流配置文件 %s 的 host_absolute config_dir 必须是绝对目录", entry.ID)
		}
		filePath, err := joinConfigFilePath(entry.ConfigDir, entry.Path)
		if err != nil {
			return "", err
		}
		return resolveWorkflowHostConfigPath(reg, filePath)
	}
	if entry.Scope != config.ConfigFileScopePlugin {
		return "", fmt.Errorf("工作流配置文件 scope 只支持 plugin 或 host_absolute")
	}
	if entry.Legacy {
		return joinConfigFilePath(workflowConfigDir(reg, workflowID), entry.Path)
	}
	baseDir, err := resolvedWorkflowConfigDir(reg, entry.ConfigDir)
	if err != nil {
		return "", err
	}
	return joinConfigFilePath(baseDir, entry.Path)
}

func resolveWorkflowHostConfigPath(reg *registry.Registry, filePath string) (string, error) {
	allowedDirs, err := normalizeHostAllowedDirsForServer(reg.Root)
	if err != nil {
		return "", fmt.Errorf("%w；请在 configs/ops.yaml 的 host_config_files.allowed_dirs 中添加该文件所在目录", err)
	}
	abs, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return "", fmt.Errorf("配置文件路径无效: %w", err)
	}
	if err := ensureHostPathAllowedForServer(abs, allowedDirs); err != nil {
		return "", fmt.Errorf("%w；请在 configs/ops.yaml 的 host_config_files.allowed_dirs 中添加该文件所在目录", err)
	}
	resolved, err := resolveHostConfigFileForServer(abs)
	if err != nil {
		return "", err
	}
	if err := ensureHostPathAllowedForServer(resolved, allowedDirs); err != nil {
		return "", fmt.Errorf("宿主配置文件符号链接最终路径未命中白名单: %w", err)
	}
	return filepath.Clean(resolved), nil
}

func workflowConfigFilePath(reg *registry.Registry, workflowID, path string) (string, error) {
	return resolvedWorkflowConfigFilePath(reg, workflowID, config.ConfigFileRef{
		ID:        path,
		ConfigDir: workflowConfigDirRef(workflowID),
		Path:      path,
		Scope:     config.ConfigFileScopePlugin,
		Access:    config.ConfigFileAccessReadWrite,
		Create:    true,
	})
}

func declaredWorkflowConfigFile(reg *registry.Registry, workflowID, fileID string) (config.ConfigFileRef, *registry.Workflow, error) {
	if strings.TrimSpace(fileID) == "" || strings.Contains(fileID, "\x00") {
		return config.ConfigFileRef{}, nil, fmt.Errorf("配置文件 ID 不安全")
	}
	wf, err := userWorkflow(reg, workflowID)
	if err != nil {
		return config.ConfigFileRef{}, nil, err
	}
	if len(wf.Config.ConfigFiles) > 0 {
		for _, entry := range declaredWorkflowConfigFiles(reg, wf) {
			if entry.ID == fileID {
				return entry, wf, nil
			}
		}
		return config.ConfigFileRef{}, nil, fmt.Errorf("配置文件 %s 未在工作流 %s 中声明", fileID, workflowID)
	}
	files, err := scanWorkflowConfigFiles(reg, workflowID)
	if err != nil {
		return config.ConfigFileRef{}, nil, err
	}
	for _, entry := range files {
		if entry.ID == fileID {
			return workflowStatusToConfigRef(workflowID, entry), wf, nil
		}
	}
	return config.ConfigFileRef{}, nil, fmt.Errorf("配置文件 %s 未在工作流 %s 中声明", fileID, workflowID)
}

func scanWorkflowConfigFiles(reg *registry.Registry, workflowID string) ([]pluginConfigFileStatus, error) {
	wf, err := userWorkflow(reg, workflowID)
	if err != nil {
		return nil, err
	}
	if len(wf.Config.ConfigFiles) > 0 {
		return workflowDeclaredConfigFileStatuses(reg, wf)
	}
	return scanWorkflowConfigDirFiles(reg, workflowID)
}

func scanWorkflowConfigDirFiles(reg *registry.Registry, workflowID string) ([]pluginConfigFileStatus, error) {
	root := workflowConfigDir(reg, workflowID)
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return []pluginConfigFileStatus{}, nil
		}
		return nil, err
	}
	files := []pluginConfigFileStatus{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		entry := pluginConfigFileStatus{
			ID:          rel,
			Label:       filepath.Base(rel),
			ConfigDir:   workflowConfigDirRef(workflowID),
			Path:        rel,
			DisplayPath: workflowConfigDisplayPath(config.ConfigFileRef{ConfigDir: workflowConfigDirRef(workflowID), Path: rel}),
			Scope:       config.ConfigFileScopePlugin,
			Access:      config.ConfigFileAccessReadWrite,
			Create:      true,
		}
		status := configFileStatus(nil, workflowStatusToConfigRef(workflowID, entry), path)
		status.ConfigDir = entry.ConfigDir
		status.DisplayPath = entry.DisplayPath
		files = append(files, status)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ID < files[j].ID })
	return files, nil
}

func workflowStatusToConfigRef(workflowID string, status pluginConfigFileStatus) config.ConfigFileRef {
	configDir := status.ConfigDir
	if configDir == "" {
		configDir = workflowConfigDirRef(workflowID)
	}
	return config.ConfigFileRef{
		ID:        status.ID,
		Label:     status.Label,
		ConfigDir: configDir,
		Path:      status.Path,
		Scope:     status.Scope,
		Access:    status.Access,
		Create:    status.Create,
		Legacy:    true,
	}
}

func declaredWorkflowConfigFiles(reg *registry.Registry, wf *registry.Workflow) []config.ConfigFileRef {
	if wf == nil || wf.Config == nil {
		return nil
	}
	out := make([]config.ConfigFileRef, 0, len(wf.Config.ConfigFiles))
	for _, entry := range wf.Config.ConfigFiles {
		out = append(out, normalizeWorkflowConfigFileRef(wf.Config.ID, entry))
	}
	return out
}

func workflowDeclaredConfigFileStatuses(reg *registry.Registry, wf *registry.Workflow) ([]pluginConfigFileStatus, error) {
	files := []pluginConfigFileStatus{}
	configDir := workflowConfigDir(reg, wf.Config.ID)
	for _, entry := range declaredWorkflowConfigFiles(reg, wf) {
		filePath, err := resolvedWorkflowConfigFilePath(reg, wf.Config.ID, entry)
		if err != nil {
			files = append(files, workflowConfigFileStatus(entry, "", err.Error(), false))
			continue
		}
		files = append(files, workflowConfigFileStatus(entry, filePath, "", pathWithinDir(configDir, filePath)))
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ID < files[j].ID })
	return files, nil
}

func workflowConfigFileStatus(entry config.ConfigFileRef, filePath, reason string, managed bool) pluginConfigFileStatus {
	status := pluginConfigFileStatus{
		ID:          entry.ID,
		Label:       entry.Label,
		ConfigDir:   entry.ConfigDir,
		DisplayRoot: workflowConfigDisplayRoot(entry),
		Path:        entry.Path,
		DisplayPath: workflowConfigDisplayPath(entry),
		Scope:       entry.Scope,
		Access:      entry.Access,
		Create:      entry.Create,
		Managed:     managed,
		Reason:      reason,
	}
	if filePath == "" {
		return status
	}
	next := configFileStatus(nil, entry, filePath)
	next.DisplayPath = workflowConfigDisplayPath(entry)
	next.Managed = managed
	if reason != "" {
		next.Reason = reason
	}
	return next
}

func workflowConfigFilesForCatalog(reg *registry.Registry, wf *config.WorkflowConfig) []config.ConfigFileRef {
	if wf == nil {
		return nil
	}
	if len(wf.ConfigFiles) > 0 {
		out := make([]config.ConfigFileRef, 0, len(wf.ConfigFiles))
		for _, entry := range wf.ConfigFiles {
			out = append(out, normalizeWorkflowConfigFileRef(wf.ID, entry))
		}
		return out
	}
	files, err := scanWorkflowConfigFiles(reg, wf.ID)
	if err == nil {
		out := make([]config.ConfigFileRef, 0, len(files))
		for _, file := range files {
			out = append(out, workflowStatusToConfigRef(wf.ID, file))
		}
		return out
	}
	out := make([]config.ConfigFileRef, 0, len(wf.ConfigFiles))
	for _, entry := range wf.ConfigFiles {
		out = append(out, normalizeWorkflowConfigFileRef(wf.ID, entry))
	}
	return out
}

func normalizeWorkflowConfigFileRef(workflowID string, entry config.ConfigFileRef) config.ConfigFileRef {
	legacy := entry.Legacy
	config.NormalizeConfigFileRef(&entry)
	entry.Legacy = legacy
	if entry.Scope != config.ConfigFileScopeHostAbsolute {
		entry.Scope = config.ConfigFileScopePlugin
	}
	if entry.Scope == config.ConfigFileScopeHostAbsolute {
		if strings.TrimSpace(entry.ConfigDir) != "" {
			entry.ConfigDir = filepath.Clean(entry.ConfigDir)
		}
		if entry.ID == "" {
			entry.ID = externalWorkflowConfigID(filepath.Join(entry.ConfigDir, filepath.FromSlash(entry.Path)))
		}
		return entry
	}
	if !entry.Legacy && entry.ConfigDir == workflowConfigDirRef(workflowID) {
		entry.ConfigDir = "."
	}
	if entry.Access == "" {
		entry.Access = config.ConfigFileAccessReadWrite
	}
	if entry.Path == "" {
		entry.Path = entry.ID
	}
	if entry.ID == "" {
		entry.ID = entry.Path
	}
	return entry
}

func resolvedWorkflowConfigDir(reg *registry.Registry, configDir string) (string, error) {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" {
		configDir = "config"
	}
	return joinWorkflowConfigDir(reg.BaseDir, configDir)
}

func joinWorkflowConfigDir(baseDir, configDir string) (string, error) {
	if strings.TrimSpace(configDir) == "" {
		return "", fmt.Errorf("工作流 config_dir 不能为空")
	}
	if strings.Contains(configDir, "://") || filepath.IsAbs(configDir) || strings.HasPrefix(configDir, "/") || strings.HasPrefix(configDir, "\\") || len(configDir) >= 2 && configDir[1] == ':' {
		return "", fmt.Errorf("工作流 config_dir 不能是绝对路径")
	}
	cleanDir := filepath.Clean(filepath.FromSlash(configDir))
	if cleanDir == "." {
		cleanDir = ""
	} else if cleanDir == ".." || strings.HasPrefix(cleanDir, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("工作流 config_dir 不能逃逸运行根目录")
	}
	for _, part := range strings.FieldsFunc(configDir, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "" || part == ".." {
			return "", fmt.Errorf("工作流 config_dir 包含不安全路径片段")
		}
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(baseAbs, cleanDir))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("工作流 config_dir 逃逸运行根目录")
	}
	return pathAbs, nil
}

func workflowConfigDisplayPath(entry config.ConfigFileRef) string {
	configDir := strings.TrimSpace(entry.ConfigDir)
	if configDir == "" || configDir == "." {
		return filepath.ToSlash(entry.Path)
	}
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(configDir), filepath.FromSlash(entry.Path)))
}

func workflowConfigDisplayRoot(entry config.ConfigFileRef) string {
	configDir := strings.TrimSpace(entry.ConfigDir)
	if configDir == "" || configDir == "." {
		return "运行根目录"
	}
	return filepath.ToSlash(configDir)
}

func userWorkflow(reg *registry.Registry, workflowID string) (*registry.Workflow, error) {
	if strings.TrimSpace(workflowID) == "" || strings.ContainsAny(workflowID, `/\`) || strings.Contains(workflowID, "\x00") {
		return nil, fmt.Errorf("工作流 ID 不安全")
	}
	wf, err := reg.Workflow(workflowID)
	if err != nil {
		return nil, err
	}
	if wf.Source.PluginID != userWorkflowPluginID || !isUserWorkflowPluginPath(reg, wf.Path) {
		return nil, fmt.Errorf("只能维护 Web 页面保存的用户工作流配置")
	}
	return wf, nil
}

func readConfigFileContent(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("配置文件不可读: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("配置文件不是普通文件")
	}
	if info.Size() > maxPluginConfigFileBytes {
		return "", fmt.Errorf("配置文件超过大小限制")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取配置文件失败: %w", err)
	}
	return string(data), nil
}

func writeConfigFileContent(filePath, content string, create bool) error {
	info, err := os.Stat(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("检查配置文件失败: %w", err)
		}
		if !create {
			return fmt.Errorf("配置文件不存在且声明不允许创建")
		}
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return fmt.Errorf("创建配置目录失败: %w", err)
		}
	} else {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("配置文件不是普通文件")
		}
		file, err := os.OpenFile(filePath, os.O_WRONLY, 0)
		if err != nil {
			return fmt.Errorf("配置文件不可写: %w", err)
		}
		_ = file.Close()
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("保存配置文件失败: %w", err)
	}
	return nil
}

func readWorkflowUploadResult(path string) (config.WorkflowUploadResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config.WorkflowUploadResult{}, fmt.Errorf("读取上传结果失败: %w", err)
	}
	var upload config.WorkflowUploadResult
	if err := json.Unmarshal(data, &upload); err != nil {
		return config.WorkflowUploadResult{}, fmt.Errorf("解析上传结果失败: %w", err)
	}
	return upload, nil
}

func saveWorkflowAssetForWorkflow(reg *registry.Registry, wf *config.WorkflowConfig) error {
	path := workflowPath(reg, wf.ID)
	if err := saveWorkflow(path, wf); err != nil {
		return err
	}
	if err := maintainUserWorkflowPluginManifest(reg); err != nil {
		return err
	}
	reg.Workflows[wf.ID] = &registry.Workflow{Entry: workflowEntryForSavedWorkflow(reg, path, wf), Config: wf, Path: path, Source: workflowSource(reg, path)}
	return nil
}

func expandDeclaredConfigFiles(root *config.RootConfig, toolCfg *config.ToolConfig, entry config.ConfigFileRef) ([]config.ConfigFileRef, error) {
	entry = normalizeServerConfigFileRef(entry)
	baseDir, err := resolvedConfigDir(root, toolCfg, entry)
	if err != nil {
		return nil, err
	}
	filePath, err := joinConfigFilePath(baseDir, entry.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []config.ConfigFileRef{entry}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return []config.ConfigFileRef{entry}, nil
	}
	items, err := os.ReadDir(filePath)
	if err != nil {
		return nil, err
	}
	out := make([]config.ConfigFileRef, 0, len(items))
	for _, item := range items {
		if item.Type()&os.ModeType != 0 {
			continue
		}
		child := entry
		child.Path = filepath.ToSlash(filepath.Join(entry.Path, item.Name()))
		child.ID = child.Path
		if entry.Scope == config.ConfigFileScopeHostAbsolute {
			child.ID = entry.ID + ":" + filepath.ToSlash(item.Name())
		}
		child.Label = item.Name()
		out = append(out, normalizeServerConfigFileRef(child))
	}
	return out, nil
}

func normalizeServerConfigFileRef(entry config.ConfigFileRef) config.ConfigFileRef {
	legacy := entry.Legacy
	config.NormalizeConfigFileRef(&entry)
	if legacy {
		entry.ConfigDir = "."
	}
	return entry
}

func resolvedConfigFilePath(root *config.RootConfig, toolCfg *config.ToolConfig, entry config.ConfigFileRef) (string, error) {
	entry = normalizeServerConfigFileRef(entry)
	baseDir, err := resolvedConfigDir(root, toolCfg, entry)
	if err != nil {
		return "", err
	}
	filePath, err := joinConfigFilePath(baseDir, entry.Path)
	if err != nil {
		return "", err
	}
	if entry.Scope != config.ConfigFileScopeHostAbsolute {
		return filePath, nil
	}
	allowedDirs, err := normalizeHostAllowedDirsForServer(root)
	if err != nil {
		return "", err
	}
	if err := ensureHostPathAllowedForServer(filePath, allowedDirs); err != nil {
		return "", err
	}
	resolved, err := resolveHostConfigFileForServer(filePath)
	if err != nil {
		return "", err
	}
	if err := ensureHostPathAllowedForServer(resolved, allowedDirs); err != nil {
		return "", fmt.Errorf("宿主配置文件符号链接最终路径未命中白名单: %w", err)
	}
	return filepath.Clean(resolved), nil
}

func resolvedConfigDir(root *config.RootConfig, toolCfg *config.ToolConfig, entry config.ConfigFileRef) (string, error) {
	entry = normalizeServerConfigFileRef(entry)
	switch entry.Scope {
	case config.ConfigFileScopePlugin:
		baseDir, err := plugin.ResolveConfigDir(toolCfg.PluginConfig.Dir, entry.ConfigDir)
		if err != nil {
			return "", fmt.Errorf("config_dir 不安全: %w", err)
		}
		return baseDir, nil
	case config.ConfigFileScopeHostAbsolute:
		if !filepath.IsAbs(entry.ConfigDir) {
			return "", fmt.Errorf("宿主 config_dir 必须是绝对路径")
		}
		allowedDirs, err := normalizeHostAllowedDirsForServer(root)
		if err != nil {
			return "", err
		}
		cleanAbs, err := filepath.Abs(filepath.Clean(entry.ConfigDir))
		if err != nil {
			return "", err
		}
		if err := ensureHostPathAllowedForServer(cleanAbs, allowedDirs); err != nil {
			return "", err
		}
		resolved, err := resolveHostConfigFileForServer(cleanAbs)
		if err != nil {
			return "", err
		}
		if err := ensureHostPathAllowedForServer(resolved, allowedDirs); err != nil {
			return "", fmt.Errorf("宿主 config_dir 符号链接最终路径未命中白名单: %w", err)
		}
		return filepath.Clean(resolved), nil
	default:
		return "", fmt.Errorf("配置文件 scope 不支持: %s", entry.Scope)
	}
}

func joinConfigFilePath(baseDir, item string) (string, error) {
	if strings.TrimSpace(item) == "" {
		return "", fmt.Errorf("config_files 条目不能为空")
	}
	if filepath.IsAbs(item) {
		return "", fmt.Errorf("config_files 条目不能是绝对路径")
	}
	cleanItem := filepath.Clean(filepath.FromSlash(item))
	if cleanItem == "." || cleanItem == ".." || strings.HasPrefix(cleanItem, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("config_files 条目不能逃逸 config_dir")
	}
	for _, part := range strings.FieldsFunc(item, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("config_files 条目包含不安全路径片段")
		}
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(baseAbs, cleanItem))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("config_files 条目逃逸 config_dir")
	}
	return pathAbs, nil
}
func normalizeHostAllowedDirsForServer(root *config.RootConfig) ([]string, error) {
	if root == nil || len(root.HostConfigFiles.AllowedDirs) == 0 {
		return nil, fmt.Errorf("未配置 host_config_files.allowed_dirs")
	}
	allowedDirs := make([]string, 0, len(root.HostConfigFiles.AllowedDirs))
	for index, dir := range root.HostConfigFiles.AllowedDirs {
		if strings.TrimSpace(dir) == "" {
			return nil, fmt.Errorf("host_config_files.allowed_dirs 第 %d 项不能为空", index+1)
		}
		if !filepath.IsAbs(dir) {
			return nil, fmt.Errorf("host_config_files.allowed_dirs 第 %d 项必须是当前平台可识别的绝对目录", index+1)
		}
		cleanAbs, err := filepath.Abs(filepath.Clean(dir))
		if err != nil {
			return nil, fmt.Errorf("host_config_files.allowed_dirs 第 %d 项无效: %w", index+1, err)
		}
		info, err := os.Stat(cleanAbs)
		if err != nil {
			return nil, fmt.Errorf("host_config_files.allowed_dirs 第 %d 项必须是已存在目录: %w", index+1, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("host_config_files.allowed_dirs 第 %d 项只支持目录白名单，不支持单文件", index+1)
		}
		resolved, err := filepath.EvalSymlinks(cleanAbs)
		if err != nil {
			return nil, fmt.Errorf("host_config_files.allowed_dirs 第 %d 项符号链接解析失败: %w", index+1, err)
		}
		allowedDirs = append(allowedDirs, filepath.Clean(resolved))
	}
	return allowedDirs, nil
}

func ensureHostPathAllowedForServer(path string, allowedDirs []string) error {
	cleanPath := filepath.Clean(path)
	for _, dir := range allowedDirs {
		cleanDir := filepath.Clean(dir)
		rel, err := filepath.Rel(cleanDir, cleanPath)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel) {
			return nil
		}
	}
	return fmt.Errorf("路径 %s 不在 host_config_files.allowed_dirs 白名单内", path)
}

func resolveHostConfigFileForServer(cleanAbs string) (string, error) {
	if _, err := os.Lstat(cleanAbs); err == nil {
		return filepath.EvalSymlinks(cleanAbs)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(cleanAbs)
	for {
		if info, err := os.Stat(parent); err == nil {
			if !info.IsDir() {
				return "", fmt.Errorf("父路径不是目录: %s", parent)
			}
			resolvedParent, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return "", err
			}
			rel, err := filepath.Rel(parent, cleanAbs)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolvedParent, rel), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", fmt.Errorf("未找到已存在父目录")
		}
		parent = next
	}
}

func configFileStatus(toolCfg *config.ToolConfig, entry config.ConfigFileRef, filePath string) pluginConfigFileStatus {
	entry = normalizeServerConfigFileRef(entry)
	status := pluginConfigFileStatus{ID: entry.ID, Label: entry.Label, ConfigDir: entry.ConfigDir, Path: entry.Path, DisplayPath: displayConfigFilePath(entry), Scope: entry.Scope, Access: entry.Access, Create: entry.Create}
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			status.Reason = "文件不存在"
			if entry.Scope == config.ConfigFileScopePlugin {
				status.Writable = entry.Access == config.ConfigFileAccessReadWrite
				return status
			}
			if entry.Create {
				if parentWritable(filepath.Dir(filePath)) {
					status.Writable = entry.Access == config.ConfigFileAccessReadWrite
				} else {
					status.Reason = "父目录不可写或不存在"
				}
			}
			return status
		}
		status.Reason = err.Error()
		return status
	}
	status.Exists = true
	status.Size = info.Size()
	if !info.Mode().IsRegular() {
		status.Reason = "不是普通文件"
		return status
	}
	if info.Size() > maxPluginConfigFileBytes {
		status.Reason = "超过大小限制"
		return status
	}
	if file, err := os.Open(filePath); err == nil {
		status.Readable = true
		_ = file.Close()
	} else {
		status.Reason = "不可读: " + err.Error()
	}
	if entry.Access == config.ConfigFileAccessReadWrite {
		if file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0); err == nil {
			status.Writable = true
			_ = file.Close()
		} else if status.Reason == "" {
			status.Reason = "不可写: " + err.Error()
		}
	} else if status.Reason == "" {
		status.Reason = "只读"
	}
	return status
}

func displayConfigFilePath(entry config.ConfigFileRef) string {
	entry = normalizeServerConfigFileRef(entry)
	if entry.ConfigDir == "" || entry.ConfigDir == "." {
		return filepath.ToSlash(entry.Path)
	}
	return filepath.ToSlash(filepath.Join(entry.ConfigDir, entry.Path))
}

func parentWritable(parent string) bool {
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return false
	}
	tmp, err := os.CreateTemp(parent, ".opsctl-perm-*")
	if err != nil {
		return false
	}
	name := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(name)
	return true
}

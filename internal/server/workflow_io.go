package server

// 本文件负责单个/批量工作流的导出与导入：导出为 YAML 或 ZIP，导入统一落回 user.workflows。
import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

const (
	workflowExportZipFilename = "workflows-export.zip"
	maxWorkflowImportSize     = 20 << 20
)

// ---------- 导出 ----------

func workflowExportHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		reg := state.registry()
		ids := make([]string, 0, 4)
		seen := map[string]bool{}
		for _, id := range req.URL.Query()["workflow"] {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			writeJSON(w, http.StatusBadRequest, response{Error: "请至少选择一个工作流"})
			return
		}
		workflows := make([]*registry.Workflow, 0, len(ids))
		for _, id := range ids {
			wf, err := reg.Workflow(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
				return
			}
			workflows = append(workflows, wf)
		}
		if len(workflows) == 1 {
			wf := workflows[0]
			data, err := os.ReadFile(wf.Path)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("读取工作流文件失败: %v", err)})
				return
			}
			w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, workflowFilename(wf.Entry.ID)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		data, err := buildWorkflowBatchExportZip(reg, workflows)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("导出工作流失败: %v", err)})
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, workflowExportZipFilename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

type workflowExportManifestEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	File      string `json:"file"`
	ConfigDir string `json:"config_dir,omitempty"`
}

type workflowExportManifest struct {
	SchemaVersion int                           `json:"schema_version"`
	Type          string                        `json:"type"`
	GeneratedAt   string                        `json:"generated_at"`
	Workflows     []workflowExportManifestEntry `json:"workflows"`
}

func buildWorkflowBatchExportZip(reg *registry.Registry, workflows []*registry.Workflow) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	builder := workflowExportZipBuilder{writer: zw, names: map[string]bool{}}
	manifest := workflowExportManifest{SchemaVersion: 1, Type: "workflow-export", GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, wf := range workflows {
		data, err := os.ReadFile(wf.Path)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		file := "workflows/" + workflowFilename(wf.Entry.ID)
		if err := builder.addData(file, data, 0o644); err != nil {
			_ = zw.Close()
			return nil, err
		}
		entry := workflowExportManifestEntry{ID: wf.Entry.ID, Name: wf.Entry.Name, File: file}
		// 只有用户工作流的托管配置目录会随包带走，其他归属的配置文件留在原插件目录里。
		if isUserWorkflowPluginPath(reg, wf.Path) {
			base := workflowFilenameBase(wf.Entry.ID)
			configDir := workflowConfigDir(reg, wf.Entry.ID)
			if info, statErr := os.Stat(configDir); statErr == nil && info.IsDir() {
				if err := builder.addDir("config/workflows/"+base, configDir); err != nil {
					_ = zw.Close()
					return nil, err
				}
				entry.ConfigDir = filepath.ToSlash(filepath.Join("config", "workflows", base))
			}
		}
		manifest.Workflows = append(manifest.Workflows, entry)
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := builder.addData("export-manifest.json", append(manifestData, '\n'), 0o644); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------- 导入 ----------

type workflowImportPayload struct {
	id          string
	config      *config.WorkflowConfig
	configFiles map[string][]byte
}

type workflowImportItem struct {
	ID          string `json:"id"`
	NewID       string `json:"new_id,omitempty"`
	Renamed     bool   `json:"renamed,omitempty"`
	ConfigFiles int    `json:"config_files,omitempty"`
}

type workflowImportFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

type workflowImportResult struct {
	Imported []workflowImportItem    `json:"imported"`
	Failed   []workflowImportFailure `json:"failed"`
}

func workflowImportHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, maxWorkflowImportSize)
		if err := req.ParseMultipartForm(4 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "读取上传文件失败: " + err.Error()})
			return
		}
		if req.MultipartForm != nil {
			defer req.MultipartForm.RemoveAll()
		}
		file, header, err := req.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "请选择要导入的工作流文件（.yaml 或 .zip）"})
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxWorkflowImportSize+1))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "读取上传文件失败: " + err.Error()})
			return
		}
		if int64(len(data)) > maxWorkflowImportSize {
			writeJSON(w, http.StatusBadRequest, response{Error: "导入文件超过大小限制"})
			return
		}
		payloads, err := parseWorkflowImport(header.Filename, data)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		writeJSON(w, http.StatusOK, response{Status: "imported", Data: importWorkflowPayloads(state.reg, payloads)})
	}
}

func parseWorkflowImport(name string, data []byte) ([]workflowImportPayload, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("导入文件为空")
	}
	if isZipPayload(name, data) {
		return parseWorkflowImportZip(data)
	}
	cfg, err := parseWorkflowYAML(data, name)
	if err != nil {
		return nil, err
	}
	return []workflowImportPayload{{id: cfg.ID, config: cfg}}, nil
}

func isZipPayload(name string, data []byte) bool {
	if strings.EqualFold(filepath.Ext(name), ".zip") {
		return true
	}
	return len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && (data[2] == 3 || data[3] == 5)
}

func parseWorkflowYAML(data []byte, source string) (*config.WorkflowConfig, error) {
	var cfg config.WorkflowConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析工作流 %s 失败: %v", source, err)
	}
	config.NormalizeWorkflow(&cfg)
	if strings.TrimSpace(cfg.ID) == "" {
		return nil, fmt.Errorf("工作流文件 %s 缺少 id", source)
	}
	if len(cfg.Nodes) == 0 {
		return nil, fmt.Errorf("工作流 %s 缺少 nodes", cfg.ID)
	}
	return &cfg, nil
}

func parseWorkflowImportZip(data []byte) ([]workflowImportPayload, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("无效 ZIP 文件: %v", err)
	}
	if len(zr.File) == 0 {
		return nil, fmt.Errorf("ZIP 文件为空")
	}
	if len(zr.File) > maxPluginUploadFiles {
		return nil, fmt.Errorf("ZIP 文件数量超过限制")
	}
	configByBase := map[string]map[string][]byte{}
	payloads := []workflowImportPayload{}
	seenID := map[string]bool{}
	var total uint64
	for _, entry := range zr.File {
		info := entry.FileInfo()
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("ZIP 包含不支持的特殊文件: %s", entry.Name)
		}
		name := filepath.ToSlash(entry.Name)
		check := strings.TrimSuffix(name, "/")
		if check == "" || strings.HasPrefix(name, "/") || filepath.IsAbs(name) || hasUnsafeZipPathSegment(check) {
			return nil, fmt.Errorf("ZIP 包含不安全路径: %s", entry.Name)
		}
		if info.IsDir() {
			continue
		}
		total += entry.UncompressedSize64
		if total > maxPluginUncompressedSize {
			return nil, fmt.Errorf("ZIP 解压后大小超过限制")
		}
		if strings.HasPrefix(name, "workflows/") && strings.EqualFold(filepath.Ext(name), ".yaml") {
			content, err := readZipEntry(entry)
			if err != nil {
				return nil, err
			}
			cfg, err := parseWorkflowYAML(content, name)
			if err != nil {
				return nil, err
			}
			if seenID[cfg.ID] {
				return nil, fmt.Errorf("ZIP 内工作流 ID 重复: %s", cfg.ID)
			}
			seenID[cfg.ID] = true
			payloads = append(payloads, workflowImportPayload{id: cfg.ID, config: cfg})
			continue
		}
		if strings.HasPrefix(name, "config/workflows/") {
			parts := strings.SplitN(strings.TrimPrefix(name, "config/workflows/"), "/", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				continue
			}
			content, err := readZipEntry(entry)
			if err != nil {
				return nil, err
			}
			byFile := configByBase[parts[0]]
			if byFile == nil {
				byFile = map[string][]byte{}
				configByBase[parts[0]] = byFile
			}
			byFile[parts[1]] = content
		}
	}
	if len(payloads) == 0 {
		return nil, fmt.Errorf("ZIP 中没有找到 workflows/*.yaml 工作流文件")
	}
	for i := range payloads {
		if files := configByBase[workflowFilenameBase(payloads[i].id)]; len(files) > 0 {
			payloads[i].configFiles = files
		}
	}
	return payloads, nil
}

func readZipEntry(entry *zip.File) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, maxPluginUncompressedSize+1))
}

func importWorkflowPayloads(reg *registry.Registry, payloads []workflowImportPayload) workflowImportResult {
	result := workflowImportResult{Imported: []workflowImportItem{}, Failed: []workflowImportFailure{}}
	owner := userWorkflowPluginID
	for _, payload := range payloads {
		if payload.config == nil {
			continue
		}
		originalID := payload.config.ID
		newID := uniqueWorkflowID(reg, originalID)
		wf := payload.config
		wf.ID = newID
		normalizeImportedWorkflowConfigFiles(reg, wf)
		// 目标环境校验：引用未安装工具、上传白名单不匹配的条目会被拒绝，而不是写坏注册表。
		if err := reg.ValidateWorkflow(wf); err != nil {
			result.Failed = append(result.Failed, workflowImportFailure{ID: originalID, Error: err.Error()})
			continue
		}
		path, source, err := saveWorkflowAsset(reg, wf, &owner)
		if err != nil {
			result.Failed = append(result.Failed, workflowImportFailure{ID: originalID, Error: err.Error()})
			continue
		}
		written := 0
		if len(payload.configFiles) > 0 {
			written, err = writeImportedWorkflowConfigFiles(reg, newID, payload.configFiles)
			if err != nil {
				_ = os.Remove(path)
				_ = maintainUserWorkflowPluginManifest(reg)
				result.Failed = append(result.Failed, workflowImportFailure{ID: originalID, Error: err.Error()})
				continue
			}
		}
		reg.Workflows[newID] = &registry.Workflow{Entry: workflowEntryForSavedWorkflow(reg, path, wf), Config: wf, Path: path, Source: source}
		result.Imported = append(result.Imported, workflowImportItem{ID: originalID, NewID: newID, Renamed: newID != originalID, ConfigFiles: written})
	}
	return result
}

func uniqueWorkflowID(reg *registry.Registry, base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "workflow"
	}
	if _, exists := reg.Workflows[base]; !exists {
		return base
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s-%d", base, index)
		if _, exists := reg.Workflows[candidate]; !exists {
			return candidate
		}
	}
}

// normalizeImportedWorkflowConfigFiles 把导入工作流声明的托管配置文件重新指向它自己的配置目录。
// 落盘格式里 config_dir 用 "."（相对运行根）配合相对运行根的 path；改名后必须重算 path。
func normalizeImportedWorkflowConfigFiles(reg *registry.Registry, wf *config.WorkflowConfig) {
	if len(wf.ConfigFiles) == 0 {
		return
	}
	relDir, err := filepath.Rel(reg.BaseDir, workflowConfigDir(reg, wf.ID))
	if err != nil {
		return
	}
	relDir = filepath.ToSlash(relDir)
	out := wf.ConfigFiles[:0]
	for _, entry := range wf.ConfigFiles {
		config.NormalizeConfigFileRef(&entry)
		if entry.Scope == config.ConfigFileScopeHostAbsolute {
			// 宿主绝对路径依赖目标机器，不随包迁移，保留原声明。
			out = append(out, entry)
			continue
		}
		rel := strings.TrimSpace(entry.ID)
		if rel == "" {
			rel = strings.TrimSpace(entry.Path)
		}
		rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
		if rel == "" || rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
			out = append(out, entry)
			continue
		}
		entry.Legacy = false
		entry.ConfigDir = "."
		entry.Path = filepath.ToSlash(filepath.Join(relDir, rel))
		out = append(out, entry)
	}
	wf.ConfigFiles = out
}

func writeImportedWorkflowConfigFiles(reg *registry.Registry, workflowID string, files map[string][]byte) (int, error) {
	root := workflowConfigDir(reg, workflowID)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	written := 0
	for _, name := range names {
		content := files[name]
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
		if clean == "" || clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") || hasUnsafeZipPathSegment(clean) {
			return written, fmt.Errorf("配置文件路径不安全: %s", name)
		}
		if int64(len(content)) > maxPluginConfigFileBytes {
			return written, fmt.Errorf("配置文件 %s 超过大小限制", name)
		}
		target := filepath.Join(root, filepath.FromSlash(clean))
		if !pathWithinDir(root, target) {
			return written, fmt.Errorf("配置文件路径逃逸配置目录: %s", name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return written, fmt.Errorf("创建配置目录失败: %v", err)
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return written, fmt.Errorf("写入配置文件 %s 失败: %v", name, err)
		}
		written++
	}
	return written, nil
}

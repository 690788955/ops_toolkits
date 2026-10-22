package server

// 本文件负责开发包 zip、静态资源注册与 catalog 构建。
import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

func buildToolDevKitZip() ([]byte, error) {
	files := map[string]string{
		"README.md":                              toolDevKitReadme,
		"SPEC.md":                                toolDevKitSpec,
		"plugins/plugin.template/plugin.yaml":    samplePluginYAML,
		"plugins/plugin.template/scripts/run.sh": sampleRunScript,
		"plugins/plugin.template/workflows/maintenance-flow.yaml": samplePluginWorkflowYAML,
		"plugins/plugin.template/README.md":                       samplePluginReadme,
		"plugins/plugin.template/config/example.conf":             sampleConfigFile,
		"plugins/plugin.template/examples/params.yaml":            sampleParamsYAML,
		"plugins/plugin.template/examples/README.md":              sampleExamplesReadme,
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
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	})
}

func buildCatalog(reg *registry.Registry) catalogResponse {
	out := catalogResponse{Name: reg.Root.DisplayName(), Description: reg.Root.DisplayDescription(), Categories: buildCatalogCategories(reg), Warnings: reg.Warnings}
	for _, item := range catalogPluginEntries(reg) {
		out.Plugins = append(out.Plugins, item)
	}
	for _, tool := range reg.OrderedTools() {
		out.Tools = append(out.Tools, toolCatalogEntry{ToolEntry: tool.Entry, Tags: tool.Config.Tags, Execution: tool.Config.Execution, Parameters: tool.Config.Parameters, Outputs: tool.Config.Outputs, ConfigFiles: tool.Config.ConfigFiles, ConfigFileRefs: declaredConfigFiles(tool.Config), Confirm: tool.Config.Confirm, Source: tool.Source})
	}
	resolver := newWorkflowDependencyResolver(reg)
	for _, wf := range reg.Workflows {
		entry := workflowCatalogEntry{WorkflowRef: wf.Entry, Tags: wf.Config.Tags, SharedWith: wf.Config.SharedWith, Parameters: wf.Config.Parameters, ConfigFiles: workflowConfigFilesForCatalog(reg, wf.Config), ReferencedPlugins: workflowReferencedPlugins(reg, wf.Config), Confirm: effectiveWorkflowConfirm(reg, wf.Config), Source: wf.Source}
		if wf.Source.PluginID == userWorkflowPluginID {
			entry.Dependencies, entry.MissingTools = workflowPluginDependencies(reg, resolver, wf.Config)
		}
		out.Workflows = append(out.Workflows, entry)
	}
	return out
}

func buildCatalogCategories(reg *registry.Registry) []categoryCatalogEntry {
	active := activeCategoryIDs(reg)
	out := make([]categoryCatalogEntry, 0, len(reg.Root.DisplayCategories()))
	seen := map[string]bool{}
	for _, category := range reg.Root.DisplayCategories() {
		entry := categoryCatalogEntry{Category: category}
		if !active[category.ID] {
			if source, ok := disabledCategorySource(reg, category.ID); ok {
				entry.Disabled = true
				entry.Source = &source
			}
		}
		out = append(out, entry)
		if category.ID != "" {
			seen[category.ID] = true
		}
	}
	for _, pkg := range installedAnyPluginPackages(reg) {
		if !pluginDisabled(reg.Root.Plugins.Disabled, pkg) {
			continue
		}
		source := registry.Source{Type: "plugin", PluginID: pkg.Manifest.ID, PluginName: pkg.Manifest.Name, PluginVersion: pkg.Manifest.Version}
		for _, category := range pkg.Manifest.Contributes.Categories {
			if category.ID == "" || seen[category.ID] || active[category.ID] {
				continue
			}
			out = append(out, categoryCatalogEntry{Category: category, Disabled: true, Source: &source})
			seen[category.ID] = true
		}
	}
	return out
}

func disabledCategorySource(reg *registry.Registry, categoryID string) (registry.Source, bool) {
	for _, pkg := range installedAnyPluginPackages(reg) {
		if !pluginDisabled(reg.Root.Plugins.Disabled, pkg) {
			continue
		}
		for _, category := range pkg.Manifest.Contributes.Categories {
			if category.ID == categoryID {
				return registry.Source{Type: "plugin", PluginID: pkg.Manifest.ID, PluginName: pkg.Manifest.Name, PluginVersion: pkg.Manifest.Version}, true
			}
		}
	}
	return registry.Source{}, false
}

func effectiveWorkflowConfirm(reg *registry.Registry, wf *config.WorkflowConfig) config.Confirmation {
	if wf.Confirm.Required {
		return wf.Confirm
	}
	for _, node := range wf.Nodes {
		nodeType := node.Type
		if nodeType == "" && node.Tool != "" {
			nodeType = config.WorkflowNodeTypeTool
		}
		if nodeType != config.WorkflowNodeTypeTool {
			continue
		}
		tool, err := reg.Tool(node.Tool)
		if err != nil || !tool.Config.Confirm.Required || node.Confirm {
			continue
		}
		message := tool.Config.Confirm.Message
		if message == "" {
			message = "工作流包含需要确认的工具"
		}
		return config.Confirmation{Required: true, Message: message}
	}
	return wf.Confirm
}

func decodeWorkflow(req *http.Request) (*config.WorkflowConfig, error) {
	wf, _, err := decodeWorkflowSaveRequest(req)
	return wf, err
}

func decodeWorkflowSaveRequest(req *http.Request) (*config.WorkflowConfig, *string, error) {
	defer req.Body.Close()
	var body workflowSaveRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, nil, err
	}
	config.NormalizeWorkflow(&body.Workflow)
	return &body.Workflow, body.OwnerPluginID, nil
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

// normalizeWorkflowSharedWith 校验共享插件列表：剔除归属插件与全局工作流自身，并要求目标插件已启用。
func normalizeWorkflowSharedWith(reg *registry.Registry, wf *config.WorkflowConfig, ownerPluginID *string) error {
	ownerID := ""
	if ownerPluginID != nil {
		ownerID = strings.TrimSpace(*ownerPluginID)
		if ownerID == "" {
			ownerID = userWorkflowPluginID
		}
	} else if existing := reg.Workflows[wf.ID]; existing != nil {
		ownerID = existing.Source.PluginID
	}
	kept := make([]string, 0, len(wf.SharedWith))
	for _, pluginID := range wf.SharedWith {
		if pluginID == ownerID || pluginID == userWorkflowPluginID {
			continue
		}
		if _, ok := installedPlugin(reg, pluginID); !ok {
			return fmt.Errorf("共享插件 %s 未安装或未启用", pluginID)
		}
		kept = append(kept, pluginID)
	}
	if len(kept) == 0 {
		wf.SharedWith = nil
		return nil
	}
	wf.SharedWith = kept
	return nil
}

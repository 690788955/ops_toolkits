package server

// 本文件负责用户工作流导出 zip 的构建。
import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"shell_ops/internal/config"
	"shell_ops/internal/plugin"
	"shell_ops/internal/registry"
)

type userWorkflowExportOptions struct {
	WorkflowIDs         []string
	IncludeDependencies bool
}

type userWorkflowExportManifest struct {
	SchemaVersion int                            `json:"schema_version"`
	Type          string                         `json:"type"`
	GeneratedAt   string                         `json:"generated_at"`
	Workflows     []userWorkflowExportWorkflow   `json:"workflows"`
	Dependencies  []userWorkflowExportDependency `json:"dependencies"`
	MissingTools  []string                       `json:"missing_tools,omitempty"`
}

type userWorkflowExportWorkflow struct {
	ID           string   `json:"id"`
	Name         string   `json:"name,omitempty"`
	Path         string   `json:"path"`
	Dependencies []string `json:"dependencies,omitempty"`
	MissingTools []string `json:"missing_tools,omitempty"`
}

type userWorkflowExportDependency struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Version  string `json:"version,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	Package  string `json:"package"`
}

type workflowDependencyResolver struct {
	toolPlugins map[string]string
	packages    map[string]plugin.Package
}

func newWorkflowDependencyResolver(reg *registry.Registry) *workflowDependencyResolver {
	resolver := &workflowDependencyResolver{toolPlugins: map[string]string{}, packages: map[string]plugin.Package{}}
	for _, pkg := range installedAnyPluginPackages(reg) {
		resolver.packages[pkg.Manifest.ID] = pkg
		for _, tool := range pkg.Manifest.Contributes.Tools {
			if tool.ID != "" {
				resolver.toolPlugins[tool.ID] = pkg.Manifest.ID
			}
		}
	}
	return resolver
}

func (r *workflowDependencyResolver) pluginForTool(reg *registry.Registry, toolID string) (string, bool) {
	if tool, err := reg.Tool(toolID); err == nil {
		if tool.Source.Type == "plugin" && tool.Source.PluginID != "" {
			return tool.Source.PluginID, true
		}
		return "", true
	}
	pluginID, ok := r.toolPlugins[toolID]
	return pluginID, ok
}

func workflowPluginDependencies(reg *registry.Registry, resolver *workflowDependencyResolver, wf *config.WorkflowConfig) ([]workflowDependency, []string) {
	if wf == nil {
		return nil, nil
	}
	toolIDs := map[string]bool{}
	for _, node := range wf.Nodes {
		nodeType := node.Type
		if nodeType == "" && node.Tool != "" {
			nodeType = config.WorkflowNodeTypeTool
		}
		if nodeType == "" && (node.Loop.Tool != "" || node.Loop.Target != "" || node.Loop.MaxIterations != 0 || len(node.Loop.Params) > 0) {
			nodeType = config.WorkflowNodeTypeLoop
		}
		toolID := ""
		switch nodeType {
		case config.WorkflowNodeTypeTool:
			toolID = node.Tool
		case config.WorkflowNodeTypeLoop:
			toolID = node.Loop.Tool
		}
		if toolID = strings.TrimSpace(toolID); toolID != "" {
			toolIDs[toolID] = true
		}
	}
	pluginIDs := map[string]bool{}
	missing := []string{}
	for toolID := range toolIDs {
		pluginID, ok := resolver.pluginForTool(reg, toolID)
		if !ok {
			missing = append(missing, toolID)
			continue
		}
		if pluginID != "" && pluginID != userWorkflowPluginID {
			pluginIDs[pluginID] = true
		}
	}
	dependencies := make([]workflowDependency, 0, len(pluginIDs))
	for pluginID := range pluginIDs {
		dependency := workflowDependency{ID: pluginID}
		if pkg, ok := resolver.packages[pluginID]; ok {
			dependency.Name = pkg.Manifest.Name
			dependency.Version = pkg.Manifest.Version
			dependency.Disabled = pluginDisabled(reg.Root.Plugins.Disabled, pkg)
		}
		dependencies = append(dependencies, dependency)
	}
	sort.Slice(dependencies, func(i, j int) bool { return dependencies[i].ID < dependencies[j].ID })
	sort.Strings(missing)
	return dependencies, missing
}

func userWorkflowExportSelection(reg *registry.Registry, ids []string) ([]*registry.Workflow, error) {
	all := make([]*registry.Workflow, 0, len(reg.Workflows))
	for _, wf := range reg.Workflows {
		if wf.Source.PluginID == userWorkflowPluginID && isUserWorkflowPluginPath(reg, wf.Path) {
			all = append(all, wf)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Entry.ID < all[j].Entry.ID })
	if len(ids) == 0 {
		return all, nil
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			wanted[id] = true
		}
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("请至少选择一个工作流")
	}
	selected := make([]*registry.Workflow, 0, len(wanted))
	for _, wf := range all {
		if wanted[wf.Entry.ID] {
			selected = append(selected, wf)
			delete(wanted, wf.Entry.ID)
		}
	}
	if len(wanted) > 0 {
		missing := make([]string, 0, len(wanted))
		for id := range wanted {
			missing = append(missing, id)
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("未找到用户工作流: %s", strings.Join(missing, "、"))
	}
	return selected, nil
}

type workflowExportZipBuilder struct {
	writer *zip.Writer
	names  map[string]bool
}

func (b *workflowExportZipBuilder) addData(name string, data []byte, mode os.FileMode) error {
	name = filepath.ToSlash(name)
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) || hasUnsafeZipPathSegment(name) {
		return fmt.Errorf("导出包包含不安全路径: %s", name)
	}
	if b.names[name] {
		return fmt.Errorf("导出包包含重复文件: %s", name)
	}
	b.names[name] = true
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(mode)
	header.SetModTime(time.Now())
	writer, err := b.writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func (b *workflowExportZipBuilder) addFile(name, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("不支持导出特殊文件: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return b.addData(name, data, info.Mode().Perm())
}

func (b *workflowExportZipBuilder) addDir(prefix, dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("导出路径不是目录: %s", dir)
	}
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		return b.addFile(prefix+"/"+filepath.ToSlash(rel), path)
	})
}

const userWorkflowExportReadme = `# 用户工作流导出包

本压缩包包含所选用户工作流，以及（可选）它们引用的已安装插件。

## 目录

- user.workflows/：可直接安装的用户工作流插件，plugin.yaml 只声明本次导出的工作流。
- dependencies/：工作流引用的插件 ZIP，需要单独安装。
- export-manifest.json：本次导出的工作流、依赖插件和缺失工具清单。

## 安装

1. 在目标环境的“插件管理 → 已安装”中上传整个 ZIP，安装 user.workflows（只包含所选工作流）。
2. 打开 dependencies/，把其中的插件 ZIP 逐个上传安装。若目标环境已安装相同或更高版本，可跳过。
3. 如果 export-manifest.json 的 missing_tools 非空，说明当前环境没有找到这些工具对应的插件，请先补齐后再运行工作流。
`

func buildUserWorkflowExportZip(reg *registry.Registry, options userWorkflowExportOptions) ([]byte, error) {
	selected, err := userWorkflowExportSelection(reg, options.WorkflowIDs)
	if err != nil {
		return nil, err
	}
	pluginDir := userWorkflowPluginDir(reg)
	manifest, err := loadOrDefaultUserWorkflowManifest(filepath.Join(pluginDir, "plugin.yaml"))
	if err != nil {
		return nil, err
	}
	manifest.ID = userWorkflowPluginID
	manifest.Name = userWorkflowPluginName
	if strings.TrimSpace(manifest.Version) == "" {
		manifest.Version = userWorkflowPluginVersion
	}
	if strings.TrimSpace(manifest.Description) == "" {
		manifest.Description = "Web 页面创建和维护的用户工作流集合"
	}
	manifest.Contributes.Tools = nil
	manifest.Contributes.Workflows = nil

	type exportWorkflow struct {
		workflow     *registry.Workflow
		relPath      string
		dependencies []workflowDependency
		missingTools []string
	}
	resolver := newWorkflowDependencyResolver(reg)
	workflows := make([]exportWorkflow, 0, len(selected))
	dependencies := map[string]workflowDependency{}
	missingTools := map[string]bool{}
	for _, wf := range selected {
		rel, err := filepath.Rel(pluginDir, wf.Path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
			return nil, fmt.Errorf("工作流 %s 的路径不在用户工作流插件目录内", wf.Entry.ID)
		}
		rel = filepath.ToSlash(rel)
		deps, missing := workflowPluginDependencies(reg, resolver, wf.Config)
		for _, dependency := range deps {
			dependencies[dependency.ID] = dependency
		}
		for _, toolID := range missing {
			missingTools[toolID] = true
		}
		workflows = append(workflows, exportWorkflow{workflow: wf, relPath: rel, dependencies: deps, missingTools: missing})
		manifest.Contributes.Workflows = append(manifest.Contributes.Workflows, plugin.Workflow{Path: rel})
	}
	manifestData, err := marshalPluginManifest(manifest)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	builder := workflowExportZipBuilder{writer: zw, names: map[string]bool{}}
	if err := builder.addData(userWorkflowPluginID+"/plugin.yaml", manifestData, 0o644); err != nil {
		_ = zw.Close()
		return nil, err
	}
	for _, entry := range workflows {
		if err := builder.addFile(userWorkflowPluginID+"/"+entry.relPath, entry.workflow.Path); err != nil {
			_ = zw.Close()
			return nil, err
		}
		configRel := filepath.ToSlash(filepath.Join("config", "workflows", workflowFilenameBase(entry.workflow.Entry.ID)))
		if err := builder.addDir(userWorkflowPluginID+"/"+configRel, filepath.Join(pluginDir, filepath.FromSlash(configRel))); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}

	if options.IncludeDependencies {
		dependencyIDs := make([]string, 0, len(dependencies))
		for id := range dependencies {
			dependencyIDs = append(dependencyIDs, id)
		}
		sort.Strings(dependencyIDs)
		exportDependencies := make([]userWorkflowExportDependency, 0, len(dependencyIDs))
		for _, id := range dependencyIDs {
			dependency := dependencies[id]
			if !isSafePluginExportID(id) {
				return nil, fmt.Errorf("依赖插件 ID 包含不安全路径字符: %s", id)
			}
			pkg, ok := resolver.packages[id]
			if !ok {
				pkg, ok = registeredPlugin(reg, id)
			}
			if !ok {
				return nil, fmt.Errorf("未找到依赖插件 %s 的安装目录", id)
			}
			if err := ensurePluginDirInConfiguredRoot(reg, pkg.Dir); err != nil {
				return nil, err
			}
			data, err := zipPluginDir(pkg.Dir)
			if err != nil {
				return nil, err
			}
			packagePath := "dependencies/" + id + ".zip"
			if err := builder.addData(packagePath, data, 0o644); err != nil {
				_ = zw.Close()
				return nil, err
			}
			exportDependencies = append(exportDependencies, userWorkflowExportDependency{
				ID:       dependency.ID,
				Name:     dependency.Name,
				Version:  dependency.Version,
				Disabled: dependency.Disabled,
				Package:  packagePath,
			})
		}
		exportManifest := userWorkflowExportManifest{
			SchemaVersion: 1,
			Type:          "user-workflow-export",
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
			Workflows:     make([]userWorkflowExportWorkflow, 0, len(workflows)),
			Dependencies:  exportDependencies,
		}
		for _, entry := range workflows {
			workflowEntry := userWorkflowExportWorkflow{
				ID:           entry.workflow.Entry.ID,
				Name:         entry.workflow.Entry.Name,
				Path:         userWorkflowPluginID + "/" + entry.relPath,
				MissingTools: entry.missingTools,
			}
			for _, dependency := range entry.dependencies {
				workflowEntry.Dependencies = append(workflowEntry.Dependencies, dependency.ID)
			}
			exportManifest.Workflows = append(exportManifest.Workflows, workflowEntry)
		}
		for toolID := range missingTools {
			exportManifest.MissingTools = append(exportManifest.MissingTools, toolID)
		}
		sort.Strings(exportManifest.MissingTools)
		manifestJSON, err := json.MarshalIndent(exportManifest, "", "  ")
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if err := builder.addData("export-manifest.json", append(manifestJSON, '\n'), 0o644); err != nil {
			_ = zw.Close()
			return nil, err
		}
		if err := builder.addData("README.md", []byte(userWorkflowExportReadme), 0o644); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

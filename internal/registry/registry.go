package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"shell_ops/internal/config"
	"shell_ops/internal/plugin"
)

type Source struct {
	Type          string `json:"type"`
	PluginID      string `json:"plugin_id,omitempty"`
	PluginName    string `json:"plugin_name,omitempty"`
	PluginVersion string `json:"plugin_version,omitempty"`
}

type Tool struct {
	Entry  config.ToolEntry
	Config *config.ToolConfig
	Dir    string
	Source Source
}

type Workflow struct {
	Entry  config.WorkflowRef
	Config *config.WorkflowConfig
	Path   string
	Source Source
}

type Registry struct {
	Root      *config.RootConfig
	BaseDir   string
	Tools     map[string]*Tool
	Workflows map[string]*Workflow
}

func Load(baseDir string) (*Registry, error) {
	root, err := config.LoadRoot(config.RootPath(baseDir))
	if err != nil {
		return nil, err
	}
	reg := &Registry{
		Root:      root,
		BaseDir:   baseDir,
		Tools:     map[string]*Tool{},
		Workflows: map[string]*Workflow{},
	}
	if err := reg.loadTools(); err != nil {
		return nil, err
	}
	if err := reg.loadWorkflows(); err != nil {
		return nil, err
	}
	if err := reg.Validate(); err != nil {
		return nil, err
	}
	return reg, nil
}

func (r *Registry) Tool(id string) (*Tool, error) {
	tool, ok := r.Tools[id]
	if !ok {
		return nil, fmt.Errorf("未找到工具 %s", id)
	}
	return tool, nil
}

func (r *Registry) Workflow(id string) (*Workflow, error) {
	wf, ok := r.Workflows[id]
	if !ok {
		return nil, fmt.Errorf("未找到工作流 %s", id)
	}
	return wf, nil
}

func (r *Registry) loadTools() error {
	entries := r.Root.Tools
	if len(entries) == 0 {
		discovered, err := r.discoverToolEntries()
		if err != nil {
			return err
		}
		entries = discovered
	}
	for _, entry := range entries {
		toolDir := filepath.Join(r.BaseDir, filepath.FromSlash(entry.Path))
		toolCfg, err := config.LoadTool(filepath.Join(toolDir, "tool.yaml"))
		if err != nil {
			return err
		}
		entry = normalizeToolEntry(entry, toolCfg)
		if _, exists := r.Tools[entry.ID]; exists {
			return fmt.Errorf("工具 ID 重复: %s", entry.ID)
		}
		r.Tools[entry.ID] = &Tool{Entry: entry, Config: toolCfg, Dir: toolDir, Source: Source{Type: "builtin"}}
	}
	return r.loadPluginContributions()
}

func (r *Registry) loadPluginContributions() error {
	result, err := plugin.Load(r.BaseDir, r.Root.Plugins)
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(os.Stderr, "插件加载警告: %s\n", warning)
	}
	for _, pkg := range result.Packages {
		categories, tools, workflows, err := r.buildPluginPackage(pkg)
		if err != nil {
			if r.Root.Plugins.Strict {
				return err
			}
			fmt.Fprintf(os.Stderr, "插件加载警告: %s\n", err)
			continue
		}
		for _, category := range categories {
			r.addCategory(category)
		}
		for id, tool := range tools {
			r.Tools[id] = tool
		}
		for id, workflow := range workflows {
			r.Workflows[id] = workflow
		}
	}
	return nil
}

func (r *Registry) buildPluginPackage(pkg plugin.Package) ([]config.Category, map[string]*Tool, map[string]*Workflow, error) {
	tools := map[string]*Tool{}
	workflows := map[string]*Workflow{}
	for _, contributed := range pkg.Manifest.Contributes.Tools {
		toolCfg, toolDir, err := normalizePluginTool(pkg, contributed)
		if err != nil {
			return nil, nil, nil, err
		}
		entry := normalizeToolEntry(config.ToolEntry{}, toolCfg)
		if _, exists := r.Tools[entry.ID]; exists {
			return nil, nil, nil, fmt.Errorf("插件 %s 的工具 ID 冲突: %s", pkg.Manifest.ID, entry.ID)
		}
		if _, exists := tools[entry.ID]; exists {
			return nil, nil, nil, fmt.Errorf("插件 %s 的工具 ID 冲突: %s", pkg.Manifest.ID, entry.ID)
		}
		tools[entry.ID] = &Tool{Entry: entry, Config: toolCfg, Dir: toolDir, Source: pluginSource(pkg)}
	}
	for _, contributed := range pkg.Manifest.Contributes.Workflows {
		workflowPath, err := plugin.SafePath(pkg.Dir, contributed.Path)
		if err != nil {
			return nil, nil, nil, err
		}
		wfCfg, err := config.LoadWorkflow(workflowPath)
		if err != nil {
			return nil, nil, nil, err
		}
		entry := normalizeWorkflowEntry(config.WorkflowRef{Path: relativePath(r.BaseDir, workflowPath)}, wfCfg)
		if _, exists := r.Workflows[entry.ID]; exists {
			return nil, nil, nil, fmt.Errorf("插件 %s 的工作流 ID 冲突: %s", pkg.Manifest.ID, entry.ID)
		}
		if _, exists := workflows[entry.ID]; exists {
			return nil, nil, nil, fmt.Errorf("插件 %s 的工作流 ID 冲突: %s", pkg.Manifest.ID, entry.ID)
		}
		workflows[entry.ID] = &Workflow{Entry: entry, Config: wfCfg, Path: workflowPath, Source: pluginSource(pkg)}
	}
	return pkg.Manifest.Contributes.Categories, tools, workflows, nil
}

func (r *Registry) loadWorkflows() error {
	entries := r.Root.Workflows
	if len(entries) == 0 {
		discovered, err := r.discoverWorkflowEntries()
		if err != nil {
			return err
		}
		entries = discovered
	}
	for _, entry := range entries {
		wfPath := filepath.Join(r.BaseDir, filepath.FromSlash(entry.Path))
		wfCfg, err := config.LoadWorkflow(wfPath)
		if err != nil {
			return err
		}
		entry = normalizeWorkflowEntry(entry, wfCfg)
		if _, exists := r.Workflows[entry.ID]; exists {
			return fmt.Errorf("工作流 ID 重复: %s", entry.ID)
		}
		r.Workflows[entry.ID] = &Workflow{Entry: entry, Config: wfCfg, Path: wfPath, Source: Source{Type: "builtin"}}
	}
	return nil
}

func (r *Registry) discoverToolEntries() ([]config.ToolEntry, error) {
	entries := []config.ToolEntry{}
	for _, root := range r.Root.Paths.Tools {
		rootDir := filepath.Join(r.BaseDir, filepath.FromSlash(root))
		if _, err := os.Stat(rootDir); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Name() != "tool.yaml" {
				return err
			}
			rel, err := filepath.Rel(r.BaseDir, filepath.Dir(path))
			if err != nil {
				return err
			}
			entries = append(entries, config.ToolEntry{Path: filepath.ToSlash(rel)})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func (r *Registry) discoverWorkflowEntries() ([]config.WorkflowRef, error) {
	entries := []config.WorkflowRef{}
	for _, root := range r.Root.Paths.Workflows {
		rootDir := filepath.Join(r.BaseDir, filepath.FromSlash(root))
		if _, err := os.Stat(rootDir); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".yaml" {
				return err
			}
			rel, err := filepath.Rel(r.BaseDir, path)
			if err != nil {
				return err
			}
			entries = append(entries, config.WorkflowRef{Path: filepath.ToSlash(rel)})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func (r *Registry) addCategory(category config.Category) {
	if category.ID == "" {
		return
	}
	for _, existing := range r.Root.Menu.Categories {
		if existing.ID == category.ID {
			return
		}
	}
	r.Root.Menu.Categories = append(r.Root.Menu.Categories, category)
}

func normalizePluginTool(pkg plugin.Package, contributed plugin.Tool) (*config.ToolConfig, string, error) {
	if _, err := plugin.SafePath(pkg.Dir, contributed.Command); err != nil {
		return nil, "", err
	}
	workdir := contributed.Workdir
	if workdir == "" {
		workdir = "."
	}
	if _, err := plugin.SafePath(pkg.Dir, workdir); err != nil {
		return nil, "", err
	}
	version := contributed.Version
	if version == "" {
		version = pkg.Manifest.Version
	}
	cfg := &config.ToolConfig{
		ID:          contributed.ID,
		Name:        contributed.Name,
		Description: contributed.Description,
		Version:     version,
		Category:    contributed.Category,
		Tags:        contributed.Tags,
		Help:        contributed.Help,
		Execution: config.ExecutionConfig{
			Type:    "shell",
			Entry:   filepath.ToSlash(filepath.Clean(contributed.Command)),
			Args:    contributed.Args,
			Timeout: contributed.Timeout,
			Workdir: filepath.ToSlash(filepath.Clean(workdir)),
		},
		Parameters: contributed.Parameters,
		PassMode:   config.PassMode{Env: true, FileName: "params.yaml"},
		Confirm:    contributed.Confirm,
		Env:        contributed.Env,
	}
	return cfg, pkg.Dir, nil
}

func pluginSource(pkg plugin.Package) Source {
	return Source{Type: "plugin", PluginID: pkg.Manifest.ID, PluginName: pkg.Manifest.Name, PluginVersion: pkg.Manifest.Version}
}

func relativePath(baseDir, path string) string {
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func normalizeToolEntry(entry config.ToolEntry, tool *config.ToolConfig) config.ToolEntry {
	if entry.ID == "" {
		entry.ID = tool.ID
	}
	if entry.Category == "" {
		entry.Category = tool.Category
	}
	if entry.Category == "" {
		entry.Category = categoryFromID(entry.ID)
	}
	if entry.Name == "" {
		entry.Name = tool.Name
	}
	if entry.Description == "" {
		entry.Description = tool.Description
	}
	return entry
}

func normalizeWorkflowEntry(entry config.WorkflowRef, wf *config.WorkflowConfig) config.WorkflowRef {
	if entry.ID == "" {
		entry.ID = wf.ID
	}
	if entry.Category == "" {
		entry.Category = wf.Category
	}
	if entry.Category == "" {
		entry.Category = categoryFromID(entry.ID)
	}
	if entry.Name == "" {
		entry.Name = wf.Name
	}
	if entry.Description == "" {
		entry.Description = wf.Description
	}
	if len(entry.Tags) == 0 {
		entry.Tags = wf.Tags
	}
	return entry
}

func categoryFromID(id string) string {
	for _, sep := range []string{".", "-"} {
		if before, _, ok := strings.Cut(id, sep); ok && before != "" {
			return before
		}
	}
	return "default"
}

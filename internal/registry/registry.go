package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	Order  int `json:"-"`
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
	GlobalEnv config.Values
	Warnings  []plugin.Warning
}

func Load(baseDir string) (*Registry, error) {
	return LoadWithVersion(baseDir, "")
}

func LoadWithVersion(baseDir, version string) (*Registry, error) {
	root, err := loadRootWithVersion(baseDir, version)
	if err != nil {
		return nil, err
	}
	globalEnv, err := config.LoadGlobalEnv(baseDir)
	if err != nil {
		return nil, err
	}
	reg := &Registry{
		Root:      root,
		BaseDir:   baseDir,
		Tools:     map[string]*Tool{},
		Workflows: map[string]*Workflow{},
		GlobalEnv: globalEnv,
	}
	if err := reg.loadToolsWithVersion(version); err != nil {
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

func loadRootWithVersion(baseDir, version string) (*config.RootConfig, error) {
	path := config.RootPath(baseDir)
	if version != "" {
		versionPath := filepath.Join(baseDir, "configs", ".versions", "ops", version+".yaml")
		if _, err := os.Stat(versionPath); err == nil {
			path = versionPath
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return config.LoadRoot(path)
}

func (r *Registry) Tool(id string) (*Tool, error) {
	tool, ok := r.Tools[id]
	if !ok {
		return nil, fmt.Errorf("未找到工具 %s", id)
	}
	return tool, nil
}

func (r *Registry) OrderedTools() []*Tool {
	tools := make([]*Tool, 0, len(r.Tools))
	for _, tool := range r.Tools {
		tools = append(tools, tool)
	}
	sort.SliceStable(tools, func(i, j int) bool {
		if tools[i].Order != tools[j].Order {
			return tools[i].Order < tools[j].Order
		}
		return tools[i].Entry.ID < tools[j].Entry.ID
	})
	return tools
}

func (r *Registry) Workflow(id string) (*Workflow, error) {
	wf, ok := r.Workflows[id]
	if !ok {
		return nil, fmt.Errorf("未找到工作流 %s", id)
	}
	return wf, nil
}

func (r *Registry) PluginWarnings(pluginID string) []plugin.Warning {
	warnings := []plugin.Warning{}
	for _, warning := range r.Warnings {
		if warning.PluginID == pluginID {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

func (r *Registry) loadTools() error {
	return r.loadToolsWithVersion("")
}

func (r *Registry) loadToolsWithVersion(version string) error {
	entries := r.Root.Tools
	if len(entries) == 0 {
		discovered, err := r.discoverToolEntries()
		if err != nil {
			return err
		}
		entries = discovered
	}
	for index, entry := range entries {
		toolDir := filepath.Join(r.BaseDir, filepath.FromSlash(entry.Path))
		toolCfg, err := config.LoadTool(filepath.Join(toolDir, "tool.yaml"))
		if err != nil {
			return err
		}
		entry = normalizeToolEntry(entry, toolCfg)
		if _, exists := r.Tools[entry.ID]; exists {
			return fmt.Errorf("工具 ID 重复: %s", entry.ID)
		}
		r.Tools[entry.ID] = &Tool{Entry: entry, Config: toolCfg, Dir: toolDir, Source: Source{Type: "builtin"}, Order: index}
	}
	return r.loadPluginContributionsWithVersion(version)
}

func (r *Registry) loadPluginContributions() error {
	return r.loadPluginContributionsWithVersion("")
}

func (r *Registry) loadPluginContributionsWithVersion(version string) error {
	result, err := plugin.Load(r.BaseDir, r.Root.Plugins)
	if err != nil {
		return err
	}
	r.Warnings = append(r.Warnings, result.Warnings...)
	for _, pkg := range result.Packages {
		categories, tools, workflows, err := r.buildPluginPackageWithVersion(pkg, version)
		if err != nil {
			if r.Root.Plugins.Strict {
				return err
			}
			r.Warnings = append(r.Warnings, plugin.Warning{
				Code:       "PLUGIN_LOAD_SKIPPED",
				PluginID:   pkg.Manifest.ID,
				Message:    err.Error(),
				Suggestion: "请修正插件配置、workflow 或工具声明后重新 validate。",
			})
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
	return r.buildPluginPackageWithVersion(pkg, "")
}

func (r *Registry) buildPluginPackageWithVersion(pkg plugin.Package, version string) ([]config.Category, map[string]*Tool, map[string]*Workflow, error) {
	tools := map[string]*Tool{}
	workflows := map[string]*Workflow{}
	packageDefaultConfig, err := config.LoadOptionalValues(filepath.Join(pkg.Dir, "configs", "default.yaml"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("读取插件 %s 默认配置失败: %w", pkg.Manifest.ID, err)
	}
	hostConfigPath, err := config.PluginHostConfigPath(r.BaseDir, pkg.Manifest.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	if version != "" {
		versionPath := filepath.Join(r.BaseDir, "configs", ".versions", pkg.Manifest.ID, version+".yaml")
		if _, err := os.Stat(versionPath); err == nil {
			hostConfigPath = versionPath
		} else if !os.IsNotExist(err) {
			return nil, nil, nil, err
		}
	}
	hostConfig, err := config.LoadOptionalValues(hostConfigPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("读取插件 %s 宿主配置失败: %w", pkg.Manifest.ID, err)
	}
	mapping, _, err := loadPluginConfigMapping(r.BaseDir, pkg, r.Root)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, contributed := range pkg.Manifest.Contributes.Tools {
		toolCfg, toolDir, err := normalizePluginTool(r.Root, pkg, contributed, packageDefaultConfig, hostConfig)
		if err != nil {
			return nil, nil, nil, err
		}
		if rule, ok := mapping.Tools[contributed.ID]; ok {
			entries := make([]config.ConfigFileRef, 0, len(rule.ConfigFiles))
			legacy := make([]string, 0, len(rule.ConfigFiles))
			for _, entry := range rule.ConfigFiles {
				entry = normalizeConfigFileRef(entry)
				entries = append(entries, entry)
				legacy = append(legacy, entry.ID)
			}
			toolCfg.ConfigFiles = legacy
			toolCfg.ConfigFileRefs = entries
		}
		entry := normalizeToolEntry(config.ToolEntry{}, toolCfg)
		if _, exists := r.Tools[entry.ID]; exists {
			return nil, nil, nil, fmt.Errorf("插件 %s 的工具 ID 冲突: %s", pkg.Manifest.ID, entry.ID)
		}
		if _, exists := tools[entry.ID]; exists {
			return nil, nil, nil, fmt.Errorf("插件 %s 的工具 ID 冲突: %s", pkg.Manifest.ID, entry.ID)
		}
		tools[entry.ID] = &Tool{Entry: entry, Config: toolCfg, Dir: toolDir, Source: pluginSource(pkg), Order: len(r.Tools) + len(tools)}
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
	for _, existing := range r.Root.RuntimeCategories {
		if existing.ID == category.ID {
			return
		}
	}
	r.Root.RuntimeCategories = append(r.Root.RuntimeCategories, category)
}

func normalizePluginTool(root *config.RootConfig, pkg plugin.Package, contributed plugin.Tool, packageDefaultConfig, hostConfig map[string]interface{}) (*config.ToolConfig, string, error) {
	toolType := strings.TrimSpace(contributed.Type)
	if toolType == "" {
		toolType = "shell"
	}
	commandEntry := normalizePluginCommandEntry(contributed.Command)
	if commandHasPath(contributed.Command) {
		if _, err := plugin.SafePath(pkg.Dir, contributed.Command); err != nil {
			return nil, "", err
		}
	} else if !pluginCommandAllowed(root, contributed.Command) {
		localCommand := strings.TrimSpace(contributed.Command)
		commandPath, err := plugin.SafePath(pkg.Dir, localCommand)
		if err != nil {
			return nil, "", fmt.Errorf("插件工具 %s 的 PATH command %s 未在 plugins.allowed_commands 中允许", contributed.ID, contributed.Command)
		}
		if info, err := os.Stat(commandPath); err != nil || info.IsDir() {
			return nil, "", fmt.Errorf("插件工具 %s 的 PATH command %s 未在 plugins.allowed_commands 中允许", contributed.ID, contributed.Command)
		}
		commandEntry = filepath.ToSlash(filepath.Join(".", filepath.Clean(localCommand)))
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
	configDir := contributed.ConfigDir
	legacyConfigFiles := configDir == ""
	if configDir == "" {
		configDir = "config"
	}
	entries := make([]config.ConfigFileRef, 0, len(contributed.ConfigFiles))
	for _, path := range contributed.ConfigFiles {
		entry := config.ConfigFileRef{ID: path, ConfigDir: configDir, Path: path, Scope: config.ConfigFileScopePlugin, Access: config.ConfigFileAccessReadWrite, Create: true}
		if legacyConfigFiles && legacyConfigFilePath(path) {
			entry.ConfigDir = "."
			entry.Legacy = true
		}
		config.NormalizeConfigFileRef(&entry)
		entries = append(entries, entry)
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
			Type:    toolType,
			Entry:   commandEntry,
			Args:    contributed.Args,
			Timeout: contributed.Timeout,
			Workdir: filepath.ToSlash(filepath.Clean(workdir)),
		},
		Parameters:     contributed.Parameters,
		Outputs:        contributed.Outputs,
		PassMode:       pluginPassMode(contributed.PassMode),
		ConfigDefaults: config.CopyValues(contributed.ConfigDefaults),
		ConfigDir:      configDir,
		ConfigFiles:    contributed.ConfigFiles,
		ConfigFileRefs: entries,
		SensitivePaths: append([]string{}, contributed.SensitivePaths...),
		PluginConfig: config.PluginToolConfig{
			ID:                   pkg.Manifest.ID,
			Dir:                  pkg.Dir,
			SharedConfig:         config.CopyValues(pkg.Manifest.SharedConfig),
			PackageDefaultConfig: config.CopyValues(packageDefaultConfig),
			HostConfig:           config.CopyValues(hostConfig),
			SensitivePaths:       append([]string{}, pkg.Manifest.SensitivePaths...),
		},
		Confirm: contributed.Confirm,
		Env:     contributed.Env,
	}
	config.NormalizeToolConfig(cfg)
	return cfg, pkg.Dir, nil
}

func legacyConfigFilePath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	return clean == "config" || strings.HasPrefix(clean, "config/")
}

func commandHasPath(command string) bool {
	return strings.ContainsAny(command, `/\\`)
}

func normalizePluginCommandEntry(command string) string {
	if !commandHasPath(command) {
		return strings.TrimSpace(command)
	}
	return filepath.ToSlash(filepath.Clean(command))
}

func pluginCommandAllowed(root *config.RootConfig, command string) bool {
	command = strings.TrimSpace(command)
	if command == "" || command == "." || command == ".." || strings.ContainsAny(command, `/\\`) {
		return false
	}
	if root == nil {
		return false
	}
	for _, allowed := range root.Plugins.AllowedCommands {
		if strings.TrimSpace(allowed) == command {
			return true
		}
	}
	return false
}

func pluginPassMode(mode config.PassMode) config.PassMode {
	if !mode.Env && !mode.Args && !mode.ParamFile && mode.FileName == "" {
		return config.PassMode{Env: true, FileName: "params.yaml"}
	}
	if mode.FileName == "" {
		mode.FileName = "params.yaml"
	}
	return mode
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

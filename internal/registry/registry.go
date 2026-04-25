package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"shell_ops/internal/config"
)

type Tool struct {
	Entry  config.ToolEntry
	Config *config.ToolConfig
	Dir    string
}

type Workflow struct {
	Entry  config.WorkflowRef
	Config *config.WorkflowConfig
	Path   string
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
		r.Tools[entry.ID] = &Tool{Entry: entry, Config: toolCfg, Dir: toolDir}
	}
	return nil
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
		r.Workflows[entry.ID] = &Workflow{Entry: entry, Config: wfCfg, Path: wfPath}
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

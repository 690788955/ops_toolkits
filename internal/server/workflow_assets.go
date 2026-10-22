package server

// 本文件负责工作流落盘、归属判定与依赖解析。
import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"shell_ops/internal/config"
	"shell_ops/internal/plugin"
	"shell_ops/internal/registry"
)

func saveWorkflowAsset(reg *registry.Registry, wf *config.WorkflowConfig, ownerPluginID *string) (string, registry.Source, error) {
	existing := reg.Workflows[wf.ID]
	ownerID := ""
	preservePath := false
	if ownerPluginID != nil {
		ownerID = strings.TrimSpace(*ownerPluginID)
		if ownerID == "" {
			ownerID = userWorkflowPluginID
		}
		if existing != nil && existing.Source.Type != "plugin" {
			return "", registry.Source{}, fmt.Errorf("内置工作流不支持修改归属插件")
		}
		preservePath = existing != nil && existing.Source.PluginID == ownerID
	} else if existing != nil {
		ownerID = existing.Source.PluginID
		preservePath = true
		if ownerID == "" {
			if err := saveWorkflow(existing.Path, wf); err != nil {
				return "", registry.Source{}, err
			}
			return existing.Path, existing.Source, nil
		}
	} else {
		ownerID = userWorkflowPluginID
	}

	owner, err := workflowOwnerForPlugin(reg, ownerID)
	if err != nil {
		return "", registry.Source{}, err
	}
	var oldOwner workflowOwnerInfo
	moving := existing != nil && existing.Path != "" && !preservePath
	if moving {
		if existing.Source.PluginID == "" {
			return "", registry.Source{}, fmt.Errorf("内置工作流不支持修改归属插件")
		}
		oldOwner, err = workflowOwnerForPlugin(reg, existing.Source.PluginID)
		if err != nil {
			return "", registry.Source{}, err
		}
		if !pathWithinDir(oldOwner.Dir, existing.Path) {
			return "", registry.Source{}, fmt.Errorf("工作流文件不在插件 %s 目录内，无法迁移", oldOwner.ID)
		}
	}

	path := ""
	if preservePath && existing != nil && strings.TrimSpace(existing.Path) != "" {
		path = existing.Path
	} else {
		path, err = newWorkflowOwnerPath(owner, wf.ID)
		if err != nil {
			return "", registry.Source{}, err
		}
	}
	if err := saveWorkflow(path, wf); err != nil {
		return "", registry.Source{}, err
	}

	if moving {
		if err := os.Remove(existing.Path); err != nil && !os.IsNotExist(err) {
			return "", registry.Source{}, err
		}
		if oldOwner.ID == userWorkflowPluginID {
			if err := maintainUserWorkflowPluginManifest(reg); err != nil {
				return "", registry.Source{}, err
			}
		} else {
			rel, err := filepath.Rel(oldOwner.Dir, existing.Path)
			if err != nil {
				return "", registry.Source{}, err
			}
			if err := updatePluginWorkflowContribution(oldOwner.Dir, "", filepath.ToSlash(rel)); err != nil {
				return "", registry.Source{}, err
			}
		}
	}

	if owner.ID == userWorkflowPluginID {
		if err := maintainUserWorkflowPluginManifest(reg); err != nil {
			return "", registry.Source{}, err
		}
	} else {
		rel, err := filepath.Rel(owner.Dir, path)
		if err != nil {
			return "", registry.Source{}, err
		}
		if err := updatePluginWorkflowContribution(owner.Dir, filepath.ToSlash(rel), ""); err != nil {
			return "", registry.Source{}, err
		}
	}
	return path, owner.Source, nil
}

type workflowOwnerInfo struct {
	ID     string
	Dir    string
	Source registry.Source
}

func workflowOwnerForPlugin(reg *registry.Registry, ownerID string) (workflowOwnerInfo, error) {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" || ownerID == userWorkflowPluginID {
		return workflowOwnerInfo{
			ID:  userWorkflowPluginID,
			Dir: userWorkflowPluginDir(reg),
			Source: registry.Source{
				Type:          "plugin",
				PluginID:      userWorkflowPluginID,
				PluginName:    userWorkflowPluginName,
				PluginVersion: userWorkflowPluginVersion,
			},
		}, nil
	}
	pkg, ok := installedPlugin(reg, ownerID)
	if !ok {
		return workflowOwnerInfo{}, fmt.Errorf("未找到已启用的插件 %s", ownerID)
	}
	return workflowOwnerInfo{
		ID:  pkg.Manifest.ID,
		Dir: pkg.Dir,
		Source: registry.Source{
			Type:          "plugin",
			PluginID:      pkg.Manifest.ID,
			PluginName:    pkg.Manifest.Name,
			PluginVersion: pkg.Manifest.Version,
		},
	}, nil
}

func newWorkflowOwnerPath(owner workflowOwnerInfo, workflowID string) (string, error) {
	dir := filepath.Join(owner.Dir, "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	filename := workflowFilename(workflowID)
	path := filepath.Join(dir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path, nil
	}
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	ext := filepath.Ext(filename)
	for index := 2; ; index++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, index, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
}

func updatePluginWorkflowContribution(pluginDir, addPath, removePath string) error {
	pkg, err := plugin.LoadPackage(pluginDir)
	if err != nil {
		return err
	}
	manifest := pkg.Manifest
	workflows := make([]plugin.Workflow, 0, len(manifest.Contributes.Workflows)+1)
	seen := map[string]bool{}
	for _, entry := range manifest.Contributes.Workflows {
		entryPath := filepath.ToSlash(entry.Path)
		if entryPath == removePath || entryPath == addPath {
			continue
		}
		if seen[entryPath] {
			continue
		}
		seen[entryPath] = true
		workflows = append(workflows, entry)
	}
	if addPath != "" && !seen[addPath] {
		workflows = append(workflows, plugin.Workflow{Path: addPath})
	}
	manifest.Contributes.Workflows = workflows
	return savePluginManifest(filepath.Join(pluginDir, "plugin.yaml"), manifest)
}

func pathWithinDir(dir, path string) bool {
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return pathAbs == dirAbs || strings.HasPrefix(pathAbs, dirAbs+string(os.PathSeparator))
}

func workflowPath(reg *registry.Registry, id string) string {
	if wf, ok := reg.Workflows[id]; ok && wf.Path != "" {
		return wf.Path
	}
	filename := workflowFilename(id)
	return filepath.Join(userWorkflowPluginDir(reg), "workflows", filename)
}

func workflowFilename(id string) string {
	clean := strings.TrimSpace(id)
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	clean = replacer.Replace(clean)
	clean = strings.Trim(clean, ". ")
	if clean == "" {
		clean = "workflow"
	}
	return clean + ".yaml"
}

func userWorkflowPluginDir(reg *registry.Registry) string {
	return filepath.Join(reg.BaseDir, filepath.FromSlash(firstPluginRoot(reg)), userWorkflowPluginID)
}

func isUserWorkflowPluginPath(reg *registry.Registry, path string) bool {
	pluginDir, err := filepath.Abs(userWorkflowPluginDir(reg))
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return absPath == pluginDir || strings.HasPrefix(absPath, pluginDir+string(os.PathSeparator))
}

func workflowEntryForSavedWorkflow(reg *registry.Registry, path string, wf *config.WorkflowConfig) config.WorkflowRef {
	return config.WorkflowRef{ID: wf.ID, Category: wf.Category, Path: relativePath(reg.BaseDir, path), Name: wf.Name, Description: wf.Description, Tags: wf.Tags}
}

func workflowSource(reg *registry.Registry, path string) registry.Source {
	for _, existing := range reg.Workflows {
		if existing.Path == path && existing.Source.Type != "" {
			return existing.Source
		}
	}
	if isUserWorkflowPluginPath(reg, path) {
		return registry.Source{Type: "plugin", PluginID: userWorkflowPluginID, PluginName: userWorkflowPluginName, PluginVersion: userWorkflowPluginVersion}
	}
	return registry.Source{Type: "builtin"}
}

func maintainUserWorkflowPluginManifest(reg *registry.Registry) error {
	pluginDir := userWorkflowPluginDir(reg)
	workflowsDir := filepath.Join(pluginDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		return err
	}
	manifest, err := loadOrDefaultUserWorkflowManifest(filepath.Join(pluginDir, "plugin.yaml"))
	if err != nil {
		return err
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
	paths := map[string]bool{}
	if err := filepath.WalkDir(workflowsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		rel, err := filepath.Rel(pluginDir, path)
		if err != nil {
			return err
		}
		paths[filepath.ToSlash(rel)] = true
		return nil
	}); err != nil {
		return err
	}
	workflowPaths := make([]string, 0, len(paths))
	for path := range paths {
		workflowPaths = append(workflowPaths, path)
	}
	sort.Strings(workflowPaths)
	manifest.Contributes.Workflows = manifest.Contributes.Workflows[:0]
	for _, path := range workflowPaths {
		manifest.Contributes.Workflows = append(manifest.Contributes.Workflows, plugin.Workflow{Path: path})
	}
	return savePluginManifest(filepath.Join(pluginDir, "plugin.yaml"), manifest)
}

func loadOrDefaultUserWorkflowManifest(path string) (plugin.Manifest, error) {
	manifest := plugin.Manifest{
		ID:          userWorkflowPluginID,
		Name:        userWorkflowPluginName,
		Version:     userWorkflowPluginVersion,
		Description: "Web 页面创建和维护的用户工作流集合",
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return plugin.Manifest{}, fmt.Errorf("解析用户工作流插件清单失败: %w", err)
		}
		return manifest, nil
	}
	if os.IsNotExist(err) {
		return manifest, nil
	}
	return plugin.Manifest{}, err
}

func marshalPluginManifest(manifest plugin.Manifest) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(manifest); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func savePluginManifest(path string, manifest plugin.Manifest) error {
	data, err := marshalPluginManifest(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"shell_ops/internal/config"
)

func Load(baseDir string, cfg config.PluginsConfig) (LoadResult, error) {
	result := LoadResult{}
	disabled := disabledSet(cfg.Disabled)
	for _, root := range cfg.Paths {
		rootDir := filepath.Join(baseDir, filepath.FromSlash(root))
		if _, err := os.Stat(rootDir); os.IsNotExist(err) {
			continue
		}
		entries, err := os.ReadDir(rootDir)
		if err != nil {
			dirErr := fmt.Errorf("读取插件目录 %s 失败: %w", rootDir, err)
			if handleLoadError(&result, cfg.Strict, dirErr) != nil {
				return result, dirErr
			}
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if disabled[entry.Name()] {
				continue
			}
			pkgDir := filepath.Join(rootDir, entry.Name())
			pkg, err := loadPackage(pkgDir)
			if err == nil && disabled[pkg.Manifest.ID] {
				continue
			}
			if err == nil {
				err = ValidatePackage(pkg)
			}
			if err != nil {
				if loadErr := handleLoadError(&result, cfg.Strict, err); loadErr != nil {
					return result, loadErr
				}
				continue
			}
			result.Packages = append(result.Packages, pkg)
		}
	}
	return result, nil
}

func LoadPackage(dir string) (Package, error) {
	return loadPackage(dir)
}

func loadPackage(dir string) (Package, error) {
	manifestPath := filepath.Join(dir, "plugin.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return Package{}, fmt.Errorf("读取插件清单 %s 失败: %w", manifestPath, err)
	}
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return Package{}, fmt.Errorf("解析插件清单 %s 失败: %w", manifestPath, err)
	}
	return Package{Manifest: manifest, Dir: dir, Path: manifestPath}, nil
}

func ValidatePackage(pkg Package) error {
	if err := validatePluginID(pkg.Manifest.ID, pkg.Path); err != nil {
		return err
	}
	if strings.TrimSpace(pkg.Manifest.Name) == "" {
		return fmt.Errorf("插件 %s 名称必填", pkg.Manifest.ID)
	}
	if strings.TrimSpace(pkg.Manifest.Version) == "" {
		return fmt.Errorf("插件 %s 版本必填", pkg.Manifest.ID)
	}
	seenTools := map[string]bool{}
	for _, tool := range pkg.Manifest.Contributes.Tools {
		if err := validateTool(pkg, tool, seenTools); err != nil {
			return err
		}
	}
	seenWorkflows := map[string]bool{}
	for _, workflow := range pkg.Manifest.Contributes.Workflows {
		if err := validateWorkflow(pkg, workflow, seenWorkflows); err != nil {
			return err
		}
	}
	return nil
}

func validatePluginID(id, path string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("插件 ID 必填: %s", path)
	}
	if id == "." || id == ".." || strings.ContainsAny(id, `/\\`) {
		return fmt.Errorf("插件 ID %s 包含不安全路径字符", id)
	}
	return nil
}

func validateTool(pkg Package, tool Tool, seen map[string]bool) error {
	if strings.TrimSpace(tool.ID) == "" {
		return fmt.Errorf("插件 %s 的工具 ID 必填", pkg.Manifest.ID)
	}
	if seen[tool.ID] {
		return fmt.Errorf("插件 %s 的工具 ID 重复: %s", pkg.Manifest.ID, tool.ID)
	}
	seen[tool.ID] = true
	if !strings.HasPrefix(tool.ID, pkg.Manifest.ID+".") {
		return fmt.Errorf("插件工具 ID %s 必须以插件 ID %s. 开头", tool.ID, pkg.Manifest.ID)
	}
	if strings.TrimSpace(tool.Category) == "" {
		return fmt.Errorf("插件工具 %s 的分类必填", tool.ID)
	}
	if strings.TrimSpace(tool.Command) == "" {
		return fmt.Errorf("插件工具 %s 的 command 必填", tool.ID)
	}
	commandPath, err := SafePath(pkg.Dir, tool.Command)
	if err != nil {
		return fmt.Errorf("插件工具 %s 的 command 不安全: %w", tool.ID, err)
	}
	if info, err := os.Stat(commandPath); err != nil {
		return fmt.Errorf("插件工具 %s 的 command 不存在: %w", tool.ID, err)
	} else if info.IsDir() {
		return fmt.Errorf("插件工具 %s 的 command 不能是目录", tool.ID)
	}
	if tool.Workdir != "" {
		if _, err := SafePath(pkg.Dir, tool.Workdir); err != nil {
			return fmt.Errorf("插件工具 %s 的 workdir 不安全: %w", tool.ID, err)
		}
	}
	return nil
}

func validateWorkflow(pkg Package, workflow Workflow, seen map[string]bool) error {
	if strings.TrimSpace(workflow.Path) == "" {
		return fmt.Errorf("插件 %s 的 workflow path 必填", pkg.Manifest.ID)
	}
	if seen[workflow.Path] {
		return fmt.Errorf("插件 %s 的 workflow path 重复: %s", pkg.Manifest.ID, workflow.Path)
	}
	seen[workflow.Path] = true
	workflowPath, err := SafePath(pkg.Dir, workflow.Path)
	if err != nil {
		return fmt.Errorf("插件 %s 的 workflow path 不安全: %w", pkg.Manifest.ID, err)
	}
	if info, err := os.Stat(workflowPath); err != nil {
		return fmt.Errorf("插件 %s 的 workflow 不存在: %w", pkg.Manifest.ID, err)
	} else if info.IsDir() {
		return fmt.Errorf("插件 %s 的 workflow 不能是目录", pkg.Manifest.ID)
	}
	return nil
}

func SafePath(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("不允许绝对路径 %s", rel)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	if pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径逃逸插件目录 %s", rel)
	}
	return pathAbs, nil
}

func disabledSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}

func handleLoadError(result *LoadResult, strict bool, err error) error {
	if strict {
		return err
	}
	result.Warnings = append(result.Warnings, err.Error())
	return nil
}

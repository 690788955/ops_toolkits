package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

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
				err = ValidatePackageWithConfig(pkg, cfg)
			}
			if err != nil {
				if loadErr := handleLoadError(&result, cfg.Strict, err); loadErr != nil {
					return result, loadErr
				}
				continue
			}
			result.Packages = append(result.Packages, pkg)
			result.Warnings = append(result.Warnings, PackageWarnings(pkg)...)
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
	return ValidatePackageWithConfig(pkg, config.PluginsConfig{})
}

func ValidatePackageWithConfig(pkg Package, cfg config.PluginsConfig) error {
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
		if err := validateTool(pkg, tool, seenTools, cfg); err != nil {
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

func validateTool(pkg Package, tool Tool, seen map[string]bool, cfg config.PluginsConfig) error {
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
	toolType := strings.TrimSpace(tool.Type)
	if toolType == "" {
		toolType = "shell"
	}
	if toolType != "shell" {
		return fmt.Errorf("插件工具 %s 的 type 不支持: %s", tool.ID, tool.Type)
	}
	if strings.TrimSpace(tool.Command) == "" {
		return fmt.Errorf("插件工具 %s 的 command 必填", tool.ID)
	}
	if commandHasPath(tool.Command) {
		if err := validatePluginCommandFile(pkg, tool); err != nil {
			return err
		}
	} else if !commandAllowed(cfg, tool.Command) {
		localTool := tool
		localTool.Command = strings.TrimSpace(tool.Command)
		if err := validatePluginCommandFile(pkg, localTool); err != nil {
			if filepath.Ext(localTool.Command) != "" {
				return err
			}
			return fmt.Errorf("插件工具 %s 的 PATH command %s 未在 plugins.allowed_commands 中允许", tool.ID, tool.Command)
		}
	}
	if tool.Workdir != "" {
		if _, err := SafePath(pkg.Dir, tool.Workdir); err != nil {
			return fmt.Errorf("插件工具 %s 的 workdir 不安全: %w", tool.ID, err)
		}
	}
	configDirSpecified := strings.TrimSpace(tool.ConfigDir) != ""
	configDir := strings.TrimSpace(tool.ConfigDir)
	if configDir == "" {
		configDir = "config"
	}
	configDirPath, err := ResolveConfigDir(pkg.Dir, configDir)
	if err != nil {
		return fmt.Errorf("插件工具 %s 的 config_dir 不安全: %w", tool.ID, err)
	}
	for _, cf := range tool.ConfigFiles {
		if err := validateConfigFileRelativeItem(cf); err != nil {
			return fmt.Errorf("插件工具 %s 的 config_files 路径不安全: %w", tool.ID, err)
		}
		basePath := configDirPath
		if !configDirSpecified && legacyConfigFilePath(cf) {
			basePath = pkg.Dir
		}
		path, err := SafePath(basePath, cf)
		if err != nil {
			return fmt.Errorf("插件工具 %s 的 config_files 路径不安全: %w", tool.ID, err)
		}
		if _, err := os.Stat(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("插件工具 %s 的 config_files 无法访问: %w", tool.ID, err)
		}
	}
	if err := validateToolOutputs(tool.ID, tool.Outputs); err != nil {
		return err
	}
	return nil
}

func validateToolOutputs(toolID string, outputs []config.ToolOutput) error {
	seen := map[string]bool{}
	for _, output := range outputs {
		name := strings.TrimSpace(output.Name)
		if name == "" {
			return fmt.Errorf("插件工具 %s 的 outputs.name 必填", toolID)
		}
		if seen[name] {
			return fmt.Errorf("插件工具 %s 的 outputs.name 重复: %s", toolID, name)
		}
		seen[name] = true
		if strings.TrimSpace(output.JSONPath) == "" {
			return fmt.Errorf("插件工具 %s 的 outputs.%s json_path 必填", toolID, name)
		}
		outputType := strings.TrimSpace(output.Type)
		if outputType == "" {
			outputType = "string"
		}
		if outputType != "string" && outputType != "number" && outputType != "bool" {
			return fmt.Errorf("插件工具 %s 的 outputs.%s type 只支持 string、number 或 bool", toolID, name)
		}
	}
	return nil
}

func legacyConfigFilePath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	return clean == "config" || strings.HasPrefix(clean, "config/")
}

func validateConfigFileRelativeItem(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("不能为空")
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("不能是绝对路径")
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("不能逃逸 config_dir")
	}
	for _, part := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("包含不安全路径片段")
		}
	}
	return nil
}

func validatePluginCommandFile(pkg Package, tool Tool) error {
	commandPath, err := SafePath(pkg.Dir, tool.Command)
	if err != nil {
		return fmt.Errorf("插件工具 %s 的 command 不安全: %w", tool.ID, err)
	}
	if info, err := os.Stat(commandPath); err != nil {
		return fmt.Errorf("插件工具 %s 的 command 不存在: %w", tool.ID, err)
	} else if info.IsDir() {
		return fmt.Errorf("插件工具 %s 的 command 不能是目录", tool.ID)
	}
	return nil
}

func commandHasPath(command string) bool {
	return strings.ContainsAny(command, `/\\`)
}

func commandAllowed(cfg config.PluginsConfig, command string) bool {
	command = strings.TrimSpace(command)
	if command == "" || command == "." || command == ".." || commandHasPath(command) {
		return false
	}
	for _, allowed := range cfg.AllowedCommands {
		if strings.TrimSpace(allowed) == command {
			return true
		}
	}
	return false
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

func PackageWarnings(pkg Package) []Warning {
	warnings := []Warning{}
	pluginID := pkg.Manifest.ID
	if _, err := os.Stat(filepath.Join(pkg.Dir, "README.md")); os.IsNotExist(err) {
		warnings = append(warnings, Warning{
			Code:       "PLUGIN_README_MISSING",
			PluginID:   pluginID,
			Field:      "README.md",
			Message:    fmt.Sprintf("插件 %s 缺少 README.md", pluginID),
			Suggestion: "请在插件目录增加 README.md，说明功能、输入、输出、风险、回滚方式和联系人。",
		})
	}
	if strings.TrimSpace(pkg.Manifest.Description) == "" {
		warnings = append(warnings, Warning{
			Code:       "PLUGIN_DESCRIPTION_MISSING",
			PluginID:   pluginID,
			Field:      "description",
			Message:    fmt.Sprintf("插件 %s 缺少 description", pluginID),
			Suggestion: "请在 plugin.yaml 顶层 description 描述插件用途，便于接入方理解。",
		})
	}
	for _, tool := range pkg.Manifest.Contributes.Tools {
		warnings = append(warnings, toolWarnings(pkg, tool)...)
	}
	return warnings
}

func toolWarnings(pkg Package, tool Tool) []Warning {
	warnings := []Warning{}
	pluginID := pkg.Manifest.ID
	toolID := tool.ID
	if strings.TrimSpace(tool.Description) == "" {
		warnings = append(warnings, Warning{
			Code:       "TOOL_DESCRIPTION_MISSING",
			PluginID:   pluginID,
			ToolID:     toolID,
			Field:      "contributes.tools[].description",
			Message:    fmt.Sprintf("插件工具 %s 缺少 description", toolID),
			Suggestion: "请为工具补充 description，说明工具用途、影响范围或典型使用场景。",
		})
	}
	for _, parameter := range tool.Parameters {
		if strings.TrimSpace(parameter.Description) != "" {
			continue
		}
		warnings = append(warnings, Warning{
			Code:       "PARAM_DESCRIPTION_MISSING",
			PluginID:   pluginID,
			ToolID:     toolID,
			Field:      "parameters." + parameter.Name + ".description",
			Message:    fmt.Sprintf("插件工具 %s 的参数 %s 缺少 description", toolID, parameter.Name),
			Suggestion: "请说明参数含义、示例值和对脚本行为的影响，方便页面展示和自助填写。",
		})
	}
	if tool.Confirm.Required && utf8.RuneCountInString(strings.TrimSpace(tool.Confirm.Message)) < 6 {
		warnings = append(warnings, Warning{
			Code:       "CONFIRM_MESSAGE_TOO_SHORT",
			PluginID:   pluginID,
			ToolID:     toolID,
			Field:      "confirm.message",
			Message:    fmt.Sprintf("插件工具 %s 需要确认但 confirm.message 为空或过短", toolID),
			Suggestion: "请写清影响范围、目标环境和是否可回滚，例如：确认重启生产集群？请先核对变更窗口和回滚方案。",
		})
	}
	configDirSpecified := strings.TrimSpace(tool.ConfigDir) != ""
	configDir := strings.TrimSpace(tool.ConfigDir)
	if configDir == "" {
		configDir = "config"
	}
	configDirPath, err := ResolveConfigDir(pkg.Dir, configDir)
	if err != nil {
		return warnings
	}
	for _, cf := range tool.ConfigFiles {
		basePath := configDirPath
		if !configDirSpecified && legacyConfigFilePath(cf) {
			basePath = pkg.Dir
		}
		path, err := SafePath(basePath, cf)
		if err != nil {
			continue
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			warnings = append(warnings, Warning{
				Code:       "CONFIG_FILE_MISSING",
				PluginID:   pluginID,
				ToolID:     toolID,
				Field:      "config_files",
				Message:    fmt.Sprintf("插件工具 %s 声明的配置文件 %s 不存在", toolID, cf),
				Suggestion: "请把该配置文件放入 config_dir 解析出的目录，或从 config_files 中移除未交付的声明。",
			})
		}
	}
	return warnings
}

func validateTemplateOutput(output string) error {
	if filepath.IsAbs(output) {
		return fmt.Errorf("不允许绝对路径 %s", output)
	}
	clean := filepath.Clean(filepath.FromSlash(output))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("路径逃逸 generated 目录 %s", output)
	}
	for _, part := range strings.FieldsFunc(output, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("包含不安全路径片段 %s", output)
		}
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

func SafeRelativePath(root, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("相对路径不能为空")
	}
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
	return pathAbs, nil
}

func ResolveConfigDir(root, dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("config_dir 不能为空")
	}
	if filepath.IsAbs(dir) {
		return filepath.Abs(filepath.Clean(dir))
	}
	return SafeRelativePath(root, dir)
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
	result.Warnings = append(result.Warnings, Warning{
		Code:       "PLUGIN_LOAD_SKIPPED",
		Message:    err.Error(),
		Suggestion: "请修正插件清单、路径或脚本文件后重新 validate。",
	})
	return nil
}

package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"shell_ops/internal/config"
	"shell_ops/internal/plugin"
)

var configTemplateArgPattern = regexp.MustCompile(`^--[A-Za-z0-9][A-Za-z0-9_-]*$`)

func loadPluginConfigMapping(baseDir string, pkg plugin.Package, root *config.RootConfig) (config.PluginConfigMapping, bool, error) {
	path, err := config.PluginConfigMappingPath(baseDir, pkg.Manifest.ID)
	if err != nil {
		return config.PluginConfigMapping{}, false, err
	}
	mapping, exists, err := config.LoadOptionalPluginConfigMapping(path)
	if err != nil {
		return mapping, exists, fmt.Errorf("读取插件 %s 映射规则失败: %w", pkg.Manifest.ID, err)
	}
	if err := ValidatePluginConfigMapping(pkg, mapping, root); err != nil {
		return mapping, exists, err
	}
	return mapping, exists, nil
}

func ValidatePluginConfigMapping(pkg plugin.Package, mapping config.PluginConfigMapping, root *config.RootConfig) error {
	if len(mapping.Tools) == 0 {
		return nil
	}
	toolIDs := map[string]bool{}
	for _, tool := range pkg.Manifest.Contributes.Tools {
		toolIDs[tool.ID] = true
	}
	allowedDirs, err := normalizeHostAllowedDirs(root)
	if err != nil {
		return err
	}
	for toolID, rule := range mapping.Tools {
		if !toolIDs[toolID] {
			return fmt.Errorf("插件 %s 映射规则引用未知工具 %s", pkg.Manifest.ID, toolID)
		}
		for index, cf := range rule.ConfigFiles {
			if err := validateMappingConfigFile(pkg, toolID, index, cf, allowedDirs); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMappingConfigFile(pkg plugin.Package, toolID string, index int, cf config.ConfigFileRef, allowedDirs []string) error {
	prefix := fmt.Sprintf("插件 %s 工具 %s 映射项 %d", pkg.Manifest.ID, toolID, index+1)
	cf = normalizeConfigFileRef(cf)
	if strings.TrimSpace(cf.ID) == "" {
		return fmt.Errorf("%s 的 id 必填", prefix)
	}
	if strings.Contains(cf.ID, "\x00") {
		return fmt.Errorf("%s 的 id 不能包含空字节", prefix)
	}
	if cf.Scope == config.ConfigFileScopeHostAbsolute && (strings.ContainsAny(cf.ID, `/\\`) || cf.ID == "." || cf.ID == ".." || strings.Contains(cf.ID, "%")) {
		return fmt.Errorf("%s 的宿主配置文件 id 不能包含路径或编码字符", prefix)
	}
	if strings.TrimSpace(cf.Path) == "" {
		return fmt.Errorf("%s 的文件路径必填", prefix)
	}
	if err := validateConfigFileRelativeItem(cf.Path); err != nil {
		return fmt.Errorf("%s 的 config_files 条目不安全: %w", prefix, err)
	}
	if cf.Access != config.ConfigFileAccessRead && cf.Access != config.ConfigFileAccessReadWrite {
		return fmt.Errorf("%s 的 access 只支持 read 或 read_write", prefix)
	}
	configDir := strings.TrimSpace(cf.ConfigDir)
	if configDir == "" {
		return fmt.Errorf("%s 的 config_dir 必填", prefix)
	}
	switch cf.Scope {
	case config.ConfigFileScopePlugin:
		if _, err := plugin.ResolveConfigDir(pkg.Dir, configDir); err != nil {
			return fmt.Errorf("%s 的 config_dir 不安全: %w", prefix, err)
		}
	case config.ConfigFileScopeHostAbsolute:
		if !filepath.IsAbs(configDir) {
			return fmt.Errorf("%s 的宿主 config_dir 必须是当前平台可识别的绝对目录", prefix)
		}
		cleanAbs, err := filepath.Abs(filepath.Clean(configDir))
		if err != nil {
			return fmt.Errorf("%s 的宿主 config_dir 无效: %w", prefix, err)
		}
		if err := ensureHostPathAllowed(cleanAbs, allowedDirs); err != nil {
			return fmt.Errorf("%s 的宿主 config_dir 未命中 host_config_files.allowed_dirs 白名单: %w", prefix, err)
		}
		resolved, err := resolveExistingHostPath(cleanAbs)
		if err != nil {
			return fmt.Errorf("%s 的宿主 config_dir 解析失败: %w", prefix, err)
		}
		if err := ensureHostPathAllowed(resolved, allowedDirs); err != nil {
			return fmt.Errorf("%s 的宿主 config_dir 符号链接最终路径未命中 host_config_files.allowed_dirs 白名单: %w", prefix, err)
		}
		targetAbs, err := filepath.Abs(filepath.Join(cleanAbs, filepath.Clean(filepath.FromSlash(cf.Path))))
		if err != nil {
			return fmt.Errorf("%s 的宿主配置文件路径无效: %w", prefix, err)
		}
		if err := ensureHostPathAllowed(targetAbs, allowedDirs); err != nil {
			return fmt.Errorf("%s 的宿主配置文件路径未命中 host_config_files.allowed_dirs 白名单: %w", prefix, err)
		}
		resolvedTarget, err := resolveExistingHostPath(targetAbs)
		if err != nil {
			return fmt.Errorf("%s 的宿主配置文件路径解析失败: %w", prefix, err)
		}
		if err := ensureHostPathAllowed(resolvedTarget, allowedDirs); err != nil {
			return fmt.Errorf("%s 的宿主配置文件符号链接最终路径未命中 host_config_files.allowed_dirs 白名单: %w", prefix, err)
		}
	default:
		return fmt.Errorf("%s 的 scope 只支持 plugin 或 host_absolute", prefix)
	}
	return nil
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

func normalizeConfigFileRef(entry config.ConfigFileRef) config.ConfigFileRef {
	config.NormalizeConfigFileRef(&entry)
	return entry
}

func normalizeHostAllowedDirs(root *config.RootConfig) ([]string, error) {
	if root == nil {
		return nil, nil
	}
	out := make([]string, 0, len(root.HostConfigFiles.AllowedDirs))
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
		out = append(out, filepath.Clean(resolved))
	}
	return out, nil
}

func ensureHostPathAllowed(path string, allowedDirs []string) error {
	if len(allowedDirs) == 0 {
		return fmt.Errorf("未配置 host_config_files.allowed_dirs")
	}
	cleanPath := filepath.Clean(path)
	for _, dir := range allowedDirs {
		cleanDir := filepath.Clean(dir)
		rel, err := filepath.Rel(cleanDir, cleanPath)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel) {
			return nil
		}
	}
	return fmt.Errorf("路径 %s 不在任何允许目录内", path)
}

func resolveExistingHostPath(cleanAbs string) (string, error) {
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

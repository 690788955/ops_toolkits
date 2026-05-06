package registry

import (
	"fmt"
	"regexp"
	"strings"

	"shell_ops/internal/config"
	"shell_ops/internal/plugin"
)

var configTemplateArgPattern = regexp.MustCompile(`^--[A-Za-z0-9][A-Za-z0-9_-]*$`)

func loadPluginConfigMapping(baseDir string, pkg plugin.Package) (config.PluginConfigMapping, bool, error) {
	path, err := config.PluginConfigMappingPath(baseDir, pkg.Manifest.ID)
	if err != nil {
		return config.PluginConfigMapping{}, false, err
	}
	mapping, exists, err := config.LoadOptionalPluginConfigMapping(path)
	if err != nil {
		return mapping, exists, fmt.Errorf("读取插件 %s 映射规则失败: %w", pkg.Manifest.ID, err)
	}
	if err := ValidatePluginConfigMapping(pkg, mapping); err != nil {
		return mapping, exists, err
	}
	return mapping, exists, nil
}

func ValidatePluginConfigMapping(pkg plugin.Package, mapping config.PluginConfigMapping) error {
	if len(mapping.Tools) == 0 {
		return nil
	}
	toolIDs := map[string]bool{}
	for _, tool := range pkg.Manifest.Contributes.Tools {
		toolIDs[tool.ID] = true
	}
	for toolID, rule := range mapping.Tools {
		if !toolIDs[toolID] {
			return fmt.Errorf("插件 %s 映射规则引用未知工具 %s", pkg.Manifest.ID, toolID)
		}
		for index, cf := range rule.ConfigFiles {
			if err := validateMappingConfigFile(pkg, toolID, index, cf); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMappingConfigFile(pkg plugin.Package, toolID string, index int, cf config.ConfigFile) error {
	prefix := fmt.Sprintf("插件 %s 工具 %s 映射项 %d", pkg.Manifest.ID, toolID, index+1)
	if strings.TrimSpace(cf.Name) == "" {
		return fmt.Errorf("%s 的 name 必填", prefix)
	}
	if strings.TrimSpace(cf.Format) == "" {
		return fmt.Errorf("%s 的 format 必填", prefix)
	}
	validFormats := map[string]bool{"ini": true, "env": true, "yaml": true, "json": true, "toml": true, "text": true}
	if !validFormats[cf.Format] {
		return fmt.Errorf("%s 的 format 必须是 ini/env/yaml/json/toml/text 之一", prefix)
	}
	if strings.TrimSpace(cf.PassVia) == "" {
		return fmt.Errorf("%s 的 pass_via 必填", prefix)
	}
	validPassVia := map[string]bool{"arg": true, "env": true, "copy": true}
	if !validPassVia[cf.PassVia] {
		return fmt.Errorf("%s 的 pass_via 必须是 arg/env/copy 之一", prefix)
	}
	if cf.PassVia == "arg" && strings.TrimSpace(cf.Arg) == "" {
		return fmt.Errorf("%s 的 arg 必填（当 pass_via=arg 时）", prefix)
	}
	if cf.PassVia == "env" && strings.TrimSpace(cf.Env) == "" {
		return fmt.Errorf("%s 的 env 必填（当 pass_via=env 时）", prefix)
	}
	return nil
}

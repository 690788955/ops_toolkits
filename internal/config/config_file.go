package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	ConfigFileScopePlugin       = "plugin"
	ConfigFileScopeHostAbsolute = "host_absolute"

	ConfigFileAccessRead      = "read"
	ConfigFileAccessReadWrite = "read_write"
)

type HostConfigFilesConfig struct {
	AllowedDirs []string `yaml:"allowed_dirs" json:"allowed_dirs"`
}

type ConfigFileRef struct {
	ID        string `yaml:"id" json:"id"`
	Label     string `yaml:"label" json:"label,omitempty"`
	ConfigDir string `yaml:"config_dir" json:"config_dir,omitempty"`
	Path      string `yaml:"path" json:"path"`
	Scope     string `yaml:"scope" json:"scope"`
	Access    string `yaml:"access" json:"access"`
	Create    bool   `yaml:"create" json:"create"`
	Legacy    bool   `yaml:"-" json:"-"`
}

func (r *ConfigFileRef) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		var path string
		if err := value.Decode(&path); err != nil {
			return err
		}
		path = strings.TrimSpace(path)
		*r = NewPluginConfigFileRef(path)
		return nil
	}
	var raw struct {
		ID        string `yaml:"id"`
		Label     string `yaml:"label"`
		ConfigDir string `yaml:"config_dir"`
		Path      string `yaml:"path"`
		Scope     string `yaml:"scope"`
		Access    string `yaml:"access"`
		Create    bool   `yaml:"create"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*r = ConfigFileRef{
		ID:        strings.TrimSpace(raw.ID),
		Label:     strings.TrimSpace(raw.Label),
		ConfigDir: strings.TrimSpace(raw.ConfigDir),
		Path:      strings.TrimSpace(raw.Path),
		Scope:     strings.TrimSpace(raw.Scope),
		Access:    strings.TrimSpace(raw.Access),
		Create:    raw.Create,
	}
	NormalizeConfigFileRef(r)
	return nil
}

func NewPluginConfigFileRef(path string) ConfigFileRef {
	path = strings.TrimSpace(path)
	ref := ConfigFileRef{ID: path, ConfigDir: ".", Path: path, Scope: ConfigFileScopePlugin, Access: ConfigFileAccessReadWrite, Create: true, Legacy: true}
	NormalizeConfigFileRef(&ref)
	return ref
}

func NormalizePluginConfigFileRefs(paths []string) []ConfigFileRef {
	out := make([]ConfigFileRef, 0, len(paths))
	for _, path := range paths {
		out = append(out, NewPluginConfigFileRef(path))
	}
	return out
}

func NormalizeConfigFileRef(ref *ConfigFileRef) {
	if ref == nil {
		return
	}
	ref.ID = strings.TrimSpace(ref.ID)
	ref.Label = strings.TrimSpace(ref.Label)
	ref.ConfigDir = strings.TrimSpace(ref.ConfigDir)
	ref.Path = strings.TrimSpace(ref.Path)
	ref.Scope = strings.TrimSpace(ref.Scope)
	ref.Access = strings.TrimSpace(ref.Access)
	if ref.Scope == "" {
		ref.Scope = ConfigFileScopePlugin
	}
	if ref.Access == "" {
		if ref.Scope == ConfigFileScopePlugin {
			ref.Access = ConfigFileAccessReadWrite
		} else {
			ref.Access = ConfigFileAccessRead
		}
	}
	if ref.ConfigDir == "" {
		ref.ConfigDir = "config"
	}
	if ref.Path == "" && ref.Scope == ConfigFileScopePlugin {
		ref.Path = ref.ID
	}
	if ref.ID == "" && ref.Scope == ConfigFileScopePlugin {
		ref.ID = ref.Path
	}
}

func (r ConfigFileRef) ValidateBasic() error {
	if strings.TrimSpace(r.Path) == "" {
		return fmt.Errorf("文件路径必填")
	}
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("文件 ID 必填")
	}
	if strings.TrimSpace(r.ConfigDir) == "" {
		return fmt.Errorf("config_dir 必填")
	}
	if r.Scope != ConfigFileScopePlugin && r.Scope != ConfigFileScopeHostAbsolute {
		return fmt.Errorf("scope 只支持 %s 或 %s", ConfigFileScopePlugin, ConfigFileScopeHostAbsolute)
	}
	if r.Access != ConfigFileAccessRead && r.Access != ConfigFileAccessReadWrite {
		return fmt.Errorf("access 只支持 %s 或 %s", ConfigFileAccessRead, ConfigFileAccessReadWrite)
	}
	return nil
}

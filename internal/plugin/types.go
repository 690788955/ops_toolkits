package plugin

import "shell_ops/internal/config"

type Manifest struct {
	ID             string                 `yaml:"id" json:"id"`
	Name           string                 `yaml:"name" json:"name"`
	Version        string                 `yaml:"version" json:"version"`
	Description    string                 `yaml:"description" json:"description"`
	Author         string                 `yaml:"author" json:"author"`
	Compatibility  Compatibility          `yaml:"compatibility" json:"compatibility"`
	SharedConfig   map[string]interface{} `yaml:"shared_config" json:"shared_config"`
	SensitivePaths []string               `yaml:"sensitive_paths" json:"sensitive_paths"`
	Contributes    Contributes            `yaml:"contributes" json:"contributes"`
}

type Compatibility struct {
	Opsctl string `yaml:"opsctl" json:"opsctl"`
}

type Contributes struct {
	Categories []config.Category `yaml:"categories" json:"categories"`
	Tools      []Tool            `yaml:"tools" json:"tools"`
	Workflows  []Workflow        `yaml:"workflows" json:"workflows"`
}

type Tool struct {
	ID             string                 `yaml:"id" json:"id"`
	Name           string                 `yaml:"name" json:"name"`
	Description    string                 `yaml:"description" json:"description"`
	Version        string                 `yaml:"version" json:"version"`
	Category       string                 `yaml:"category" json:"category"`
	Tags           []string               `yaml:"tags" json:"tags"`
	Help           config.HelpConfig      `yaml:"help" json:"help"`
	Command        string                 `yaml:"command" json:"command"`
	Args           []string               `yaml:"args" json:"args"`
	Workdir        string                 `yaml:"workdir" json:"workdir"`
	Timeout        string                 `yaml:"timeout" json:"timeout"`
	Parameters     []config.Parameter     `yaml:"parameters" json:"parameters"`
	PassMode       config.PassMode        `yaml:"pass_mode" json:"pass_mode"`
	ConfigDefaults map[string]interface{} `yaml:"config_defaults" json:"config_defaults"`
	ConfigDir      string                 `yaml:"config_dir" json:"config_dir"`
	ConfigFiles    []string               `yaml:"config_files" json:"config_files"`
	SensitivePaths []string               `yaml:"sensitive_paths" json:"sensitive_paths"`
	Confirm        config.Confirmation    `yaml:"confirm" json:"confirm"`
	Env            map[string]string      `yaml:"env" json:"env"`
}

type Workflow struct {
	Path string `yaml:"path" json:"path"`
}

type Package struct {
	Manifest Manifest
	Dir      string
	Path     string
}

type Warning struct {
	Code       string `json:"code"`
	PluginID   string `json:"plugin_id"`
	ToolID     string `json:"tool_id,omitempty"`
	Field      string `json:"field,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type LoadResult struct {
	Packages []Package
	Warnings []Warning
}

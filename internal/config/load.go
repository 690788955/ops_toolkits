package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func RootPath(baseDir string) string {
	return filepath.Join(baseDir, "configs", "ops.yaml")
}

func LoadRoot(path string) (*RootConfig, error) {
	var cfg RootConfig
	if err := loadYAML(path, &cfg); err != nil {
		return nil, err
	}
	normalizeRoot(&cfg)
	return &cfg, nil
}

func SaveRoot(path string, cfg *RootConfig) error {
	if cfg == nil {
		return fmt.Errorf("配置不能为空")
	}
	disk := *cfg
	disk.RuntimeCategories = nil
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&disk); err != nil {
		_ = enc.Close()
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(path), buf.Bytes(), 0o644)
}

func LoadTool(path string) (*ToolConfig, error) {
	var cfg ToolConfig
	if err := loadYAML(path, &cfg); err != nil {
		return nil, err
	}
	normalizeTool(&cfg)
	if cfg.ID == "" {
		return nil, fmt.Errorf("工具 ID 必填: %s", path)
	}
	if cfg.Execution.Entry == "" {
		return nil, fmt.Errorf("工具执行入口必填: %s", path)
	}
	return &cfg, nil
}

func LoadWorkflow(path string) (*WorkflowConfig, error) {
	var cfg WorkflowConfig
	if err := loadYAML(path, &cfg); err != nil {
		return nil, err
	}
	NormalizeWorkflow(&cfg)
	if cfg.ID == "" {
		return nil, fmt.Errorf("工作流 ID 必填: %s", path)
	}
	if len(cfg.Nodes) == 0 {
		return nil, fmt.Errorf("工作流节点必填: %s", path)
	}
	return &cfg, nil
}

func LoadParamsFile(path string) (map[string]string, error) {
	values, err := LoadParamsFileValues(path)
	if err != nil {
		return nil, err
	}
	return ValuesToStringMap(values), nil
}

func LoadParamsFileValues(path string) (Values, error) {
	if path == "" {
		return Values{}, nil
	}
	var raw map[string]interface{}
	if err := loadYAML(path, &raw); err != nil {
		return nil, err
	}
	return InterfaceMapToValues(raw), nil
}

func LoadOptionalValues(path string) (Values, error) {
	if path == "" {
		return Values{}, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return Values{}, nil
	}
	var raw map[string]interface{}
	if err := loadYAML(path, &raw); err != nil {
		return nil, err
	}
	return InterfaceMapToValues(raw), nil
}

func LoadGlobalEnv(baseDir string) (Values, error) {
	path := filepath.Join(baseDir, "configs", "global-env.conf")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return Values{}, nil
	}
	return loadEnvFile(path)
}

func GlobalEnvPath(baseDir string) string {
	return filepath.Join(baseDir, "configs", "global-env.conf")
}

func loadEnvFile(path string) (Values, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", path, err)
	}

	result := make(Values)
	lines := bytes.Split(data, []byte("\n"))

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		// 跳过空行和注释
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		// 解析 KEY=value
		parts := bytes.SplitN(line, []byte("="), 2)
		if len(parts) != 2 {
			continue
		}

		key := string(bytes.TrimSpace(parts[0]))
		value := string(bytes.TrimSpace(parts[1]))

		// 移除引号
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}

		result[key] = value
	}

	return result, nil
}

func PluginHostConfigPath(baseDir, pluginID string) (string, error) {
	if !SafePluginConfigID(pluginID) {
		return "", fmt.Errorf("插件 ID %s 包含不安全路径字符", pluginID)
	}
	return filepath.Join(baseDir, "configs", "plugins", pluginID+".yaml"), nil
}

func PluginConfigMappingPath(baseDir, pluginID string) (string, error) {
	if !SafePluginConfigID(pluginID) {
		return "", fmt.Errorf("插件 ID %s 包含不安全路径字符", pluginID)
	}
	return filepath.Join(baseDir, "configs", "plugins", pluginID+".mapping.yaml"), nil
}

func LoadOptionalPluginConfigMapping(path string) (PluginConfigMapping, bool, error) {
	var mapping PluginConfigMapping
	if path == "" {
		return mapping, false, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return PluginConfigMapping{Tools: map[string]PluginToolConfigMapping{}}, false, nil
	}
	if err := loadYAML(path, &mapping); err != nil {
		return mapping, false, err
	}
	if mapping.Tools == nil {
		mapping.Tools = map[string]PluginToolConfigMapping{}
	}
	return mapping, true, nil
}

func SavePluginConfigMapping(path string, mapping PluginConfigMapping) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if mapping.Tools == nil {
		mapping.Tools = map[string]PluginToolConfigMapping{}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&mapping); err != nil {
		_ = enc.Close()
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(path), buf.Bytes(), 0o644)
}

func normalizeRoot(cfg *RootConfig) {
	if cfg.App.Name == "" {
		cfg.App.Name = cfg.Name
	}
	if cfg.App.Description == "" {
		cfg.App.Description = cfg.Description
	}
	if len(cfg.Menu.Categories) == 0 {
		cfg.Menu.Categories = cfg.Categories
	}
	if len(cfg.Plugins.Paths) == 0 {
		cfg.Plugins.Paths = []string{"plugins"}
	}
	if cfg.Paths.Runs == "" {
		cfg.Paths.Runs = "runs"
	}
	if cfg.Paths.Logs == "" {
		cfg.Paths.Logs = filepath.ToSlash(filepath.Join(cfg.Paths.Runs, "logs"))
	}
	if cfg.Server.Host == "" {
		cfg.Server.Host = "127.0.0.1"
	}
	if cfg.Server.Port == 0 && cfg.HTTP.Addr == "" {
		cfg.Server.Port = 8080
	}
}

func normalizeTool(cfg *ToolConfig) {
	if cfg.Execution.Type == "" {
		cfg.Execution.Type = "shell"
	}
	if cfg.Execution.Entry == "" {
		cfg.Execution.Entry = cfg.Entry
	}
	if cfg.Entry == "" {
		cfg.Entry = cfg.Execution.Entry
	}
	if cfg.Execution.Timeout == "" {
		cfg.Execution.Timeout = cfg.Timeout
	}
	if cfg.Timeout == "" {
		cfg.Timeout = cfg.Execution.Timeout
	}
	if cfg.Confirm.Message == "" && cfg.Confirmation.Message != "" {
		cfg.Confirm = cfg.Confirmation
	}
	if !cfg.Confirm.Required && cfg.Confirmation.Required {
		cfg.Confirm = cfg.Confirmation
	}
	if !cfg.PassMode.Env && !cfg.PassMode.Args && !cfg.PassMode.ParamFile && len(cfg.Execution.Args) == 0 {
		cfg.PassMode.Env = true
	}
	if cfg.PassMode.FileName == "" {
		cfg.PassMode.FileName = "params.yaml"
	}
	for i := range cfg.Outputs {
		normalizeToolOutput(&cfg.Outputs[i])
	}
}

func NormalizeToolConfig(cfg *ToolConfig) {
	normalizeTool(cfg)
}

func normalizeToolOutput(output *ToolOutput) {
	if output == nil {
		return
	}
	output.Name = strings.TrimSpace(output.Name)
	output.Description = strings.TrimSpace(output.Description)
	output.Type = strings.TrimSpace(output.Type)
	if output.Type == "" {
		output.Type = "string"
	}
	output.JSONPath = strings.TrimSpace(output.JSONPath)
}

func NormalizeWorkflow(cfg *WorkflowConfig) {
	if len(cfg.Nodes) == 0 {
		cfg.Nodes = cfg.Steps
	}
	for i := range cfg.ConfigFiles {
		NormalizeConfigFileRef(&cfg.ConfigFiles[i])
		if cfg.ConfigFiles[i].Access == "" {
			cfg.ConfigFiles[i].Access = ConfigFileAccessReadWrite
		}
		if cfg.ConfigFiles[i].Path == "" {
			cfg.ConfigFiles[i].Path = cfg.ConfigFiles[i].ID
		}
		if cfg.ConfigFiles[i].ID == "" {
			cfg.ConfigFiles[i].ID = cfg.ConfigFiles[i].Path
		}
	}
	if len(cfg.Edges) == 0 {
		cfg.Edges = edgesFromDependsOn(cfg.Nodes)
	}
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Type == "" {
			if cfg.Nodes[i].Tool != "" {
				cfg.Nodes[i].Type = WorkflowNodeTypeTool
			} else if cfg.Nodes[i].Condition.Input != "" || len(cfg.Nodes[i].Condition.Cases) > 0 {
				cfg.Nodes[i].Type = WorkflowNodeTypeCondition
			} else if cfg.Nodes[i].Loop.Tool != "" || cfg.Nodes[i].Loop.Target != "" || cfg.Nodes[i].Loop.MaxIterations != 0 {
				cfg.Nodes[i].Type = WorkflowNodeTypeLoop
			} else if cfg.Nodes[i].Upload.TargetDir != "" {
				cfg.Nodes[i].Type = WorkflowNodeTypeUpload
			}
		}
		if cfg.Nodes[i].OnFailure == "" {
			cfg.Nodes[i].OnFailure = "stop"
		}
	}
}

func edgesFromDependsOn(nodes []WorkflowNode) []WorkflowEdge {
	edges := []WorkflowEdge{}
	for _, node := range nodes {
		for _, dep := range node.DependsOn {
			edges = append(edges, WorkflowEdge{From: dep, To: node.ID})
		}
	}
	if len(edges) > 0 || len(nodes) < 2 {
		return edges
	}
	for i := 1; i < len(nodes); i++ {
		edges = append(edges, WorkflowEdge{From: nodes[i-1].ID, To: nodes[i].ID})
	}
	return edges
}

func loadYAML(path string, dst interface{}) error {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	return nil
}

func stringifyMap(in map[string]interface{}) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = fmt.Sprint(v)
	}
	return out
}

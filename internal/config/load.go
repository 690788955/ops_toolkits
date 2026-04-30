package config

import (
	"fmt"
	"os"
	"path/filepath"

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
	if path == "" {
		return map[string]string{}, nil
	}
	var raw map[string]interface{}
	if err := loadYAML(path, &raw); err != nil {
		return nil, err
	}
	return stringifyMap(raw), nil
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
		cfg.Server.Host = "0.0.0.0"
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
}

func NormalizeWorkflow(cfg *WorkflowConfig) {
	if len(cfg.Nodes) == 0 {
		cfg.Nodes = cfg.Steps
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

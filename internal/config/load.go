package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	normalizeMarket(&cfg.Market)
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
	// 上传白名单只保留非空项，避免空行被当成允许任意目录。
	if len(cfg.Uploads.AllowedDirs) > 0 {
		dirs := make([]string, 0, len(cfg.Uploads.AllowedDirs))
		for _, dir := range cfg.Uploads.AllowedDirs {
			if strings.TrimSpace(dir) != "" {
				dirs = append(dirs, strings.TrimSpace(dir))
			}
		}
		cfg.Uploads.AllowedDirs = dirs
	}
}

func normalizeMarket(cfg *MarketConfig) {
	if cfg == nil {
		return
	}
	seen := map[string]bool{}
	for i := range cfg.Sources {
		source := &cfg.Sources[i]
		source.ID = strings.TrimSpace(source.ID)
		source.Name = strings.TrimSpace(source.Name)
		source.URL = strings.TrimSpace(source.URL)
		if source.ID == "" {
			source.ID = fmt.Sprintf("source-%d", i+1)
		}
		base := source.ID
		for suffix := 2; seen[source.ID]; suffix++ {
			source.ID = fmt.Sprintf("%s-%d", base, suffix)
		}
		if source.Name == "" {
			source.Name = source.ID
		}
		seen[source.ID] = true
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
	cfg.SharedWith = normalizeSharedWith(cfg.SharedWith)
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
	for i := range cfg.Stages {
		stage := &cfg.Stages[i]
		stage.ID = strings.TrimSpace(stage.ID)
		stage.Name = strings.TrimSpace(stage.Name)
		if stage.Name == "" {
			stage.Name = stage.ID
		}
		stage.ExecutionMode = strings.ToLower(strings.TrimSpace(stage.ExecutionMode))
		if stage.ExecutionMode == "" {
			stage.ExecutionMode = WorkflowStageExecutionSerial
		}
	}
	if len(cfg.Edges) == 0 {
		cfg.Edges = edgesFromDependsOn(cfg.Nodes, len(cfg.Stages) == 0)
	}
	for i := range cfg.Nodes {
		cfg.Nodes[i].StageID = strings.TrimSpace(cfg.Nodes[i].StageID)
		if cfg.Nodes[i].Type == "" {
			if cfg.Nodes[i].Tool != "" {
				cfg.Nodes[i].Type = WorkflowNodeTypeTool
			} else if cfg.Nodes[i].Condition.Input != "" || len(cfg.Nodes[i].Condition.Cases) > 0 {
				cfg.Nodes[i].Type = WorkflowNodeTypeCondition
			} else if cfg.Nodes[i].Loop.Tool != "" || cfg.Nodes[i].Loop.Target != "" || cfg.Nodes[i].Loop.MaxIterations != 0 {
				cfg.Nodes[i].Type = WorkflowNodeTypeLoop
			} else if cfg.Nodes[i].Upload.TargetDir != "" {
				cfg.Nodes[i].Type = WorkflowNodeTypeUpload
			} else if cfg.Nodes[i].Extract.SourceType != "" || cfg.Nodes[i].Extract.FileName != "" || cfg.Nodes[i].Extract.SourceNode != "" || cfg.Nodes[i].Extract.SourceDir != "" || cfg.Nodes[i].Extract.SourcePath != "" || cfg.Nodes[i].Extract.TargetPath != "" || len(cfg.Nodes[i].Extract.Files) > 0 {
				cfg.Nodes[i].Type = WorkflowNodeTypeExtractConfig
			} else if strings.TrimSpace(cfg.Nodes[i].HTTP.URL) != "" {
				cfg.Nodes[i].Type = WorkflowNodeTypeHTTP
			} else if strings.TrimSpace(cfg.Nodes[i].Script.Script) != "" {
				cfg.Nodes[i].Type = WorkflowNodeTypeShell
			} else if strings.TrimSpace(cfg.Nodes[i].Checksum.Path) != "" {
				cfg.Nodes[i].Type = WorkflowNodeTypeChecksum
			} else if strings.TrimSpace(cfg.Nodes[i].Archive.Path) != "" {
				cfg.Nodes[i].Type = WorkflowNodeTypeArchive
			} else if cfg.Nodes[i].Wait.Seconds > 0 || strings.TrimSpace(cfg.Nodes[i].Wait.PollURL) != "" || strings.TrimSpace(cfg.Nodes[i].Wait.PollFile) != "" {
				cfg.Nodes[i].Type = WorkflowNodeTypeWait
			}
		}
		if cfg.Nodes[i].Type == WorkflowNodeTypeHTTP {
			if method, err := NormalizeHTTPMethod(cfg.Nodes[i].HTTP.Method); err == nil {
				cfg.Nodes[i].HTTP.Method = method
			}
			if bodyType, err := NormalizeHTTPBodyType(cfg.Nodes[i].HTTP.BodyType, cfg.Nodes[i].HTTP.Body); err == nil {
				cfg.Nodes[i].HTTP.BodyType = bodyType
			}
		}
		if cfg.Nodes[i].OnFailure == "" {
			cfg.Nodes[i].OnFailure = "stop"
		}
	}
	normalizeLegacyWorkflowStage(cfg)
	normalizeWorkflowRecovery(cfg)
}

// normalizeLegacyWorkflowStage 把旧格式（只写 nodes/edges、没有 stages）折成单个串行阶段，
// 让编辑器、校验和运行统一走阶段模型；节点 order 按依赖拓扑排序生成，保留原有 edges 语义。
func normalizeLegacyWorkflowStage(cfg *WorkflowConfig) {
	if len(cfg.Stages) > 0 || len(cfg.Nodes) == 0 {
		return
	}
	cfg.Stages = []WorkflowStage{{
		ID:            LegacyWorkflowStageID,
		Name:          LegacyWorkflowStageName,
		ExecutionMode: WorkflowStageExecutionSerial,
	}}
	orders := legacyStageOrders(cfg)
	for i := range cfg.Nodes {
		cfg.Nodes[i].StageID = LegacyWorkflowStageID
		cfg.Nodes[i].Order = orders[cfg.Nodes[i].ID]
	}
}

// legacyStageOrders 用 Kahn 拓扑排序排出串行阶段内的 order；成环时把剩余节点按原始顺序补在末尾。
func legacyStageOrders(cfg *WorkflowConfig) map[string]int {
	position := make(map[string]int, len(cfg.Nodes))
	ids := make([]string, 0, len(cfg.Nodes))
	for index, node := range cfg.Nodes {
		if _, exists := position[node.ID]; exists {
			continue
		}
		position[node.ID] = index
		ids = append(ids, node.ID)
	}
	indegree := make(map[string]int, len(ids))
	next := make(map[string][]string, len(ids))
	for _, id := range ids {
		indegree[id] = 0
	}
	for _, edge := range cfg.Edges {
		if _, ok := indegree[edge.From]; !ok {
			continue
		}
		if _, ok := indegree[edge.To]; !ok {
			continue
		}
		next[edge.From] = append(next[edge.From], edge.To)
		indegree[edge.To]++
	}
	queue := make([]string, 0, len(ids))
	for _, id := range ids {
		if indegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	orders := make(map[string]int, len(ids))
	step := 0
	for len(queue) > 0 {
		best := 0
		for i := 1; i < len(queue); i++ {
			if position[queue[i]] < position[queue[best]] {
				best = i
			}
		}
		id := queue[best]
		queue = append(queue[:best], queue[best+1:]...)
		step++
		orders[id] = step
		for _, follow := range next[id] {
			indegree[follow]--
			if indegree[follow] == 0 {
				queue = append(queue, follow)
			}
		}
	}
	for _, id := range ids {
		if _, done := orders[id]; done {
			continue
		}
		step++
		orders[id] = step
	}
	return orders
}

func normalizeWorkflowRecovery(cfg *WorkflowConfig) {
	if cfg == nil || cfg.Recovery == nil {
		return
	}
	recovery := cfg.Recovery
	recovery.DiffNodeID = strings.TrimSpace(recovery.DiffNodeID)
	recovery.ChangeNodeIDs = normalizeStringList(recovery.ChangeNodeIDs)
	recovery.ServiceNodeIDs = normalizeStringList(recovery.ServiceNodeIDs)
	recovery.HealthNodeIDs = normalizeStringList(recovery.HealthNodeIDs)
	if recovery.Notification != nil {
		notification := recovery.Notification
		notification.URL = strings.TrimSpace(notification.URL)
		notification.Timeout = strings.TrimSpace(notification.Timeout)
		notification.Events = normalizeNotificationEvents(notification.Events)
		if len(notification.Headers) == 0 {
			notification.Headers = nil
		}
		if notification.URL == "" && len(notification.Events) == 0 && len(notification.Headers) == 0 && notification.Timeout == "" {
			recovery.Notification = nil
		}
	}
	if !recovery.PendingOnFailure && !recovery.SkipChangeOnNoDiff && recovery.DiffNodeID == "" &&
		len(recovery.ChangeNodeIDs) == 0 && len(recovery.ServiceNodeIDs) == 0 && len(recovery.HealthNodeIDs) == 0 &&
		recovery.Notification == nil {
		cfg.Recovery = nil
	}
}

func normalizeNotificationEvents(events []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(events))
	for _, raw := range events {
		event := strings.ToLower(strings.TrimSpace(raw))
		switch event {
		case WorkflowNotifySucceeded, WorkflowNotifyFailed, WorkflowNotifyPending, WorkflowNotifyRolledBack:
		default:
			continue
		}
		if seen[event] {
			continue
		}
		seen[event] = true
		out = append(out, event)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeSharedWith(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

// edgesFromDependsOn 兼容旧工作流的 depends_on。没有显式依赖时，只有非阶段工作流
// 才补一条串行链；阶段工作流的顺序由 stages + order 表达，不生成边。
func edgesFromDependsOn(nodes []WorkflowNode, sequentialFallback bool) []WorkflowEdge {
	edges := []WorkflowEdge{}
	for _, node := range nodes {
		for _, dep := range node.DependsOn {
			edges = append(edges, WorkflowEdge{From: dep, To: node.ID})
		}
	}
	if len(edges) > 0 || len(nodes) < 2 || !sequentialFallback {
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

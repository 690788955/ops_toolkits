package registry

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"shell_ops/internal/config"
)

var allowedConditionOperators = map[string]bool{
	"eq":           true,
	"neq":          true,
	"contains":     true,
	"not_contains": true,
	"in":           true,
	"not_in":       true,
	"exists":       true,
	"empty":        true,
}

func (r *Registry) Validate() error {
	if _, err := normalizeHostAllowedDirs(r.Root); err != nil {
		return err
	}
	for id, wf := range r.Workflows {
		if err := r.ValidateWorkflow(wf.Config); err != nil {
			return fmt.Errorf("工作流 %s: %w", id, err)
		}
	}
	return nil
}

func (r *Registry) ValidateWorkflow(wf *config.WorkflowConfig) error {
	if wf.ID == "" {
		return fmt.Errorf("工作流 ID 必填")
	}
	if len(wf.Nodes) == 0 {
		return fmt.Errorf("节点必填")
	}
	nodes := map[string]config.WorkflowNode{}
	for _, node := range wf.Nodes {
		if node.ID == "" {
			return fmt.Errorf("节点 ID 必填")
		}
		if _, ok := nodes[node.ID]; ok {
			return fmt.Errorf("节点 ID 重复: %s", node.ID)
		}
		nodes[node.ID] = node
	}
	for _, node := range wf.Nodes {
		nodeType := effectiveNodeType(node)
		if err := r.validateWorkflowNode(node, nodeType, nodes); err != nil {
			return err
		}
	}
	if err := validateWorkflowConfigFiles(wf); err != nil {
		return err
	}
	if err := validateWorkflowEdges(wf, nodes); err != nil {
		return err
	}
	_, err := OrderWorkflow(wf)
	return err
}

func (r *Registry) validateWorkflowNode(node config.WorkflowNode, nodeType string, nodes map[string]config.WorkflowNode) error {
	switch nodeType {
	case config.WorkflowNodeTypeTool:
		if node.Tool == "" {
			return fmt.Errorf("工具节点 %s 的工具必填", node.ID)
		}
		if node.Condition.Input != "" || len(node.Condition.Cases) > 0 || node.Condition.DefaultCase != "" {
			return fmt.Errorf("工具节点 %s 不能配置 condition", node.ID)
		}
		if hasLoopConfig(node.Loop) {
			return fmt.Errorf("工具节点 %s 不能配置 loop", node.ID)
		}
		if hasUploadConfig(node.Upload) {
			return fmt.Errorf("工具节点 %s 不能配置 upload", node.ID)
		}
		if hasExtractConfig(node.Extract) {
			return fmt.Errorf("工具节点 %s 不能配置 extract_config", node.ID)
		}
		if _, ok := r.Tools[node.Tool]; !ok {
			return fmt.Errorf("节点 %s 引用了不存在的工具 %s", node.ID, node.Tool)
		}
	case config.WorkflowNodeTypeCondition:
		if node.Tool != "" {
			return fmt.Errorf("条件节点 %s 不能同时配置 tool", node.ID)
		}
		if hasLoopConfig(node.Loop) {
			return fmt.Errorf("条件节点 %s 不能配置 loop", node.ID)
		}
		if hasUploadConfig(node.Upload) {
			return fmt.Errorf("条件节点 %s 不能配置 upload", node.ID)
		}
		if hasExtractConfig(node.Extract) {
			return fmt.Errorf("条件节点 %s 不能配置 extract_config", node.ID)
		}
		if node.Condition.Input == "" {
			return fmt.Errorf("条件节点 %s 的 condition.input 必填", node.ID)
		}
		if len(node.Condition.Cases) == 0 {
			return fmt.Errorf("条件节点 %s 至少需要一个 case", node.ID)
		}
		seen := map[string]bool{}
		for _, item := range node.Condition.Cases {
			if item.ID == "" {
				return fmt.Errorf("条件节点 %s 的 case.id 必填", node.ID)
			}
			if item.ID == "default" {
				return fmt.Errorf("条件节点 %s 的 case ID 不能使用保留值 default", node.ID)
			}
			if item.Name == "" {
				return fmt.Errorf("条件节点 %s 的 case %s name 必填", node.ID, item.ID)
			}
			if seen[item.ID] {
				return fmt.Errorf("条件节点 %s 的 case ID 重复: %s", node.ID, item.ID)
			}
			seen[item.ID] = true
			if !allowedConditionOperators[item.Operator] {
				return fmt.Errorf("条件节点 %s 的 case %s 使用非法 operator: %s", node.ID, item.ID, item.Operator)
			}
		}
		if node.Condition.DefaultCase != "" && node.Condition.DefaultCase != "default" {
			return fmt.Errorf("条件节点 %s 的 default_case 只支持 default", node.ID)
		}
	case config.WorkflowNodeTypeParallel, config.WorkflowNodeTypeJoin:
		if node.Tool != "" {
			return fmt.Errorf("编排节点 %s 不能配置 tool", node.ID)
		}
		if node.Condition.Input != "" || len(node.Condition.Cases) > 0 || node.Condition.DefaultCase != "" {
			return fmt.Errorf("编排节点 %s 不能配置 condition", node.ID)
		}
		if hasLoopConfig(node.Loop) {
			return fmt.Errorf("编排节点 %s 不能配置 loop", node.ID)
		}
		if hasUploadConfig(node.Upload) {
			return fmt.Errorf("编排节点 %s 不能配置 upload", node.ID)
		}
		if hasExtractConfig(node.Extract) {
			return fmt.Errorf("编排节点 %s 不能配置 extract_config", node.ID)
		}
	case config.WorkflowNodeTypeUpload:
		if node.Tool != "" {
			return fmt.Errorf("上传节点 %s 不能配置 tool", node.ID)
		}
		if node.Condition.Input != "" || len(node.Condition.Cases) > 0 || node.Condition.DefaultCase != "" {
			return fmt.Errorf("上传节点 %s 不能配置 condition", node.ID)
		}
		if hasLoopConfig(node.Loop) {
			return fmt.Errorf("上传节点 %s 不能配置 loop", node.ID)
		}
		if hasExtractConfig(node.Extract) {
			return fmt.Errorf("上传节点 %s 不能配置 extract_config", node.ID)
		}
		if _, err := config.NormalizeUploadTargetDir(node.Upload.TargetDir); err != nil {
			return fmt.Errorf("上传节点 %s 的 upload.target_dir 无效: %w", node.ID, err)
		}
		seenExports := map[string]bool{}
		for _, item := range node.Upload.ConfigExports {
			if err := validateWorkflowUploadConfigExport(node.ID, item); err != nil {
				return err
			}
			if seenExports[item.ID] {
				return fmt.Errorf("上传节点 %s 的 config_exports id 重复: %s", node.ID, item.ID)
			}
			seenExports[item.ID] = true
		}
	case config.WorkflowNodeTypeExtractConfig:
		if node.Tool != "" {
			return fmt.Errorf("提取配置节点 %s 不能配置 tool", node.ID)
		}
		if node.Condition.Input != "" || len(node.Condition.Cases) > 0 || node.Condition.DefaultCase != "" {
			return fmt.Errorf("提取配置节点 %s 不能配置 condition", node.ID)
		}
		if hasLoopConfig(node.Loop) {
			return fmt.Errorf("提取配置节点 %s 不能配置 loop", node.ID)
		}
		if hasUploadConfig(node.Upload) {
			return fmt.Errorf("提取配置节点 %s 不能配置 upload", node.ID)
		}
		if err := validateWorkflowExtractConfig(node.ID, node.Extract); err != nil {
			return err
		}
	case config.WorkflowNodeTypeLoop:
		if node.Tool != "" {
			return fmt.Errorf("循环节点 %s 不能同时配置 tool", node.ID)
		}
		if node.Condition.Input != "" || len(node.Condition.Cases) > 0 || node.Condition.DefaultCase != "" {
			return fmt.Errorf("循环节点 %s 不能配置 condition", node.ID)
		}
		if hasUploadConfig(node.Upload) {
			return fmt.Errorf("循环节点 %s 不能配置 upload", node.ID)
		}
		if hasExtractConfig(node.Extract) {
			return fmt.Errorf("循环节点 %s 不能配置 extract_config", node.ID)
		}
		if node.Loop.Target != "" {
			if node.Loop.Tool != "" {
				return fmt.Errorf("循环节点 %s 不能同时配置 loop.tool 和 loop.target", node.ID)
			}
			if targetNode, ok := nodes[node.Loop.Target]; ok {
				if effectiveNodeType(targetNode) != config.WorkflowNodeTypeTool || targetNode.Tool == "" {
					return fmt.Errorf("循环节点 %s 的 loop.target 必须引用工具节点", node.ID)
				}
				node.Loop.Tool = targetNode.Tool
			} else {
				return fmt.Errorf("循环节点 %s 的 loop.target 引用了不存在的节点 %s", node.ID, node.Loop.Target)
			}
		}
		if node.Loop.Tool == "" {
			return fmt.Errorf("循环节点 %s 的 loop.tool 必填", node.ID)
		}
		if _, ok := r.Tools[node.Loop.Tool]; !ok {
			return fmt.Errorf("循环节点 %s 引用了不存在的工具 %s", node.ID, node.Loop.Tool)
		}
		if node.Loop.MaxIterations < 1 || node.Loop.MaxIterations > 20 {
			return fmt.Errorf("循环节点 %s 的 loop.max_iterations 必须在 1..20 之间", node.ID)
		}
	default:
		return fmt.Errorf("节点 %s 使用未知类型: %s", node.ID, nodeType)
	}
	return nil
}

func validateWorkflowConfigFiles(wf *config.WorkflowConfig) error {
	seen := map[string]bool{}
	for _, entry := range wf.ConfigFiles {
		if entry.Scope != "" && entry.Scope != config.ConfigFileScopePlugin {
			return fmt.Errorf("工作流配置文件 %s 只支持 plugin scope", entry.ID)
		}
		if entry.Access != "" && entry.Access != config.ConfigFileAccessRead && entry.Access != config.ConfigFileAccessReadWrite {
			return fmt.Errorf("工作流配置文件 %s access 只支持 read 或 read_write", entry.ID)
		}
		if strings.TrimSpace(entry.ID) == "" {
			return fmt.Errorf("工作流配置文件 ID 必填")
		}
		if seen[entry.ID] {
			return fmt.Errorf("工作流配置文件 ID 重复: %s", entry.ID)
		}
		seen[entry.ID] = true
		if err := validateWorkflowRelativePath(entry.Path); err != nil {
			return fmt.Errorf("工作流配置文件 %s 的 path 不安全: %w", entry.ID, err)
		}
		if strings.TrimSpace(entry.ConfigDir) != "" && strings.TrimSpace(entry.ConfigDir) != "." {
			if err := validateWorkflowRelativePath(entry.ConfigDir); err != nil {
				return fmt.Errorf("工作流配置文件 %s 的 config_dir 不安全: %w", entry.ID, err)
			}
		}
	}
	return nil
}

func validateWorkflowUploadConfigExport(nodeID string, item config.WorkflowUploadConfigExport) error {
	if strings.TrimSpace(item.ID) == "" {
		return fmt.Errorf("上传节点 %s 的 config_exports.id 必填", nodeID)
	}
	if strings.TrimSpace(item.TargetPath) == "" {
		return fmt.Errorf("上传节点 %s 的 config_exports.target_path 必填", nodeID)
	}
	if item.Access != "" && item.Access != config.ConfigFileAccessRead && item.Access != config.ConfigFileAccessReadWrite {
		return fmt.Errorf("上传节点 %s 的 config_exports access 只支持 read 或 read_write", nodeID)
	}
	if item.SourcePath != "" {
		if err := validateWorkflowRelativePath(item.SourcePath); err != nil {
			return fmt.Errorf("上传节点 %s 的 config_exports.source_path 不安全: %w", nodeID, err)
		}
	}
	if err := validateWorkflowRelativePath(item.TargetPath); err != nil {
		return fmt.Errorf("上传节点 %s 的 config_exports.target_path 不安全: %w", nodeID, err)
	}
	return nil
}

func validateWorkflowRelativePath(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("不能为空")
	}
	if strings.Contains(value, "://") || filepath.IsAbs(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") || len(value) >= 2 && value[1] == ':' {
		return fmt.Errorf("不能是绝对路径")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return fmt.Errorf("不能逃逸工作流配置目录")
	}
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("包含不安全路径片段")
		}
	}
	return nil
}

func validateWorkflowEdges(wf *config.WorkflowConfig, nodes map[string]config.WorkflowNode) error {
	for _, edge := range wf.Edges {
		from, ok := nodes[edge.From]
		if !ok {
			continue
		}
		fromType := effectiveNodeType(from)
		if edge.Case != "" && fromType != config.WorkflowNodeTypeCondition {
			return fmt.Errorf("非条件节点 %s 的出边不能配置 case", edge.From)
		}
		if fromType != config.WorkflowNodeTypeCondition {
			continue
		}
		if edge.Case == "" {
			return fmt.Errorf("条件节点 %s 的出边必须配置 case", edge.From)
		}
		if edge.Case == "default" {
			if from.Condition.DefaultCase != "default" {
				return fmt.Errorf("条件节点 %s 未启用 default_case，不能配置 default 出边", edge.From)
			}
			continue
		}
		if !conditionCaseExists(from, edge.Case) {
			return fmt.Errorf("条件节点 %s 的出边引用了不存在的 case: %s", edge.From, edge.Case)
		}
	}
	return nil
}

func effectiveNodeType(node config.WorkflowNode) string {
	if node.Type != "" {
		return node.Type
	}
	if node.Tool != "" {
		return config.WorkflowNodeTypeTool
	}
	if node.Condition.Input != "" || len(node.Condition.Cases) > 0 || node.Condition.DefaultCase != "" {
		return config.WorkflowNodeTypeCondition
	}
	if hasLoopConfig(node.Loop) {
		return config.WorkflowNodeTypeLoop
	}
	if hasUploadConfig(node.Upload) {
		return config.WorkflowNodeTypeUpload
	}
	if hasExtractConfig(node.Extract) {
		return config.WorkflowNodeTypeExtractConfig
	}
	return ""
}

func hasLoopConfig(loop config.WorkflowLoop) bool {
	return loop.Tool != "" || loop.Target != "" || loop.MaxIterations != 0 || len(loop.Params) > 0
}

func hasUploadConfig(upload config.WorkflowUpload) bool {
	return upload.TargetDir != "" || len(upload.ConfigExports) > 0
}

func hasExtractConfig(item config.WorkflowExtractConfig) bool {
	return item.SourceType != "" || item.FileName != "" || item.TargetPath != "" || item.Label != "" || item.Replace || len(item.Files) > 0 || item.SourceNode != "" || item.SourceDir != "" || item.SourcePath != ""
}

func validateWorkflowExtractConfig(nodeID string, item config.WorkflowExtractConfig) error {
	if workflowExtractSourceType(item) == "directory" {
		if strings.TrimSpace(item.SourceDir) == "" {
			return fmt.Errorf("提取配置节点 %s 的 extract_config.source_dir 必填", nodeID)
		}
		if !strings.Contains(item.SourceDir, "{{") {
			if err := validateWorkflowRelativePath(item.SourceDir); err != nil {
				return fmt.Errorf("提取配置节点 %s 的 extract_config.source_dir 不安全: %w", nodeID, err)
			}
		}
		if len(item.Files) == 0 {
			return fmt.Errorf("提取配置节点 %s 的 extract_config.files 必填", nodeID)
		}
		for index, file := range item.Files {
			sourcePath := strings.TrimSpace(file.SourcePath)
			if sourcePath == "" {
				sourcePath = strings.TrimSpace(file.FileName)
			}
			if sourcePath == "" {
				return fmt.Errorf("提取配置节点 %s 的 extract_config.files[%d].source_path 必填", nodeID, index)
			}
			if strings.Contains(sourcePath, "{{") {
				return fmt.Errorf("提取配置节点 %s 的 extract_config.files[%d].source_path 不能包含模板", nodeID, index)
			}
			if err := validateWorkflowRelativePath(sourcePath); err != nil {
				return fmt.Errorf("提取配置节点 %s 的 extract_config.files[%d].source_path 不安全: %w", nodeID, index, err)
			}
			targetPath := strings.TrimSpace(file.TargetPath)
			if targetPath == "" {
				targetPath = sourcePath
			}
			if err := validateWorkflowRelativePath(targetPath); err != nil {
				return fmt.Errorf("提取配置节点 %s 的 extract_config.files[%d].target_path 不安全: %w", nodeID, index, err)
			}
		}
		return nil
	}
	file := workflowExtractConfigItem(item)
	if strings.TrimSpace(file.FileName) == "" {
		return fmt.Errorf("提取配置节点 %s 的 extract_config.file_name 必填", nodeID)
	}
	if !strings.Contains(file.FileName, "{{") {
		if err := validateWorkflowRelativePath(file.FileName); err != nil {
			return fmt.Errorf("提取配置节点 %s 的 extract_config.file_name 不安全: %w", nodeID, err)
		}
	}
	if strings.TrimSpace(file.TargetPath) == "" {
		return fmt.Errorf("提取配置节点 %s 的 extract_config.target_path 必填", nodeID)
	}
	if err := validateWorkflowRelativePath(file.TargetPath); err != nil {
		return fmt.Errorf("提取配置节点 %s 的 extract_config.target_path 不安全: %w", nodeID, err)
	}
	return nil
}

func workflowExtractSourceType(item config.WorkflowExtractConfig) string {
	if strings.EqualFold(strings.TrimSpace(item.SourceType), "directory") || strings.TrimSpace(item.SourceDir) != "" || len(item.Files) > 0 {
		return "directory"
	}
	return "file"
}

func workflowExtractConfigItem(item config.WorkflowExtractConfig) config.WorkflowExtractConfigFile {
	if item.FileName != "" || item.TargetPath != "" || item.Label != "" || item.Replace {
		return config.WorkflowExtractConfigFile{FileName: item.FileName, SourcePath: item.SourcePath, TargetPath: item.TargetPath, Label: item.Label, Replace: item.Replace}
	}
	if len(item.Files) > 0 {
		return item.Files[0]
	}
	return config.WorkflowExtractConfigFile{FileName: item.SourcePath, SourcePath: item.SourcePath, TargetPath: item.TargetPath, Label: item.Label, Replace: item.Replace}
}

func conditionCaseExists(node config.WorkflowNode, caseID string) bool {
	for _, item := range node.Condition.Cases {
		if item.ID == caseID {
			return true
		}
	}
	return false
}

func OrderWorkflow(wf *config.WorkflowConfig) ([]config.WorkflowNode, error) {
	nodes := map[string]config.WorkflowNode{}
	incoming := map[string]int{}
	children := map[string][]string{}
	for _, node := range wf.Nodes {
		if node.ID == "" {
			return nil, fmt.Errorf("节点 ID 必填")
		}
		if _, ok := nodes[node.ID]; ok {
			return nil, fmt.Errorf("节点 ID 重复: %s", node.ID)
		}
		nodes[node.ID] = node
		incoming[node.ID] = 0
	}
	for _, edge := range wf.Edges {
		if edge.From == "" || edge.To == "" {
			return nil, fmt.Errorf("工作流依赖的 from/to 必填")
		}
		if _, ok := nodes[edge.From]; !ok {
			return nil, fmt.Errorf("工作流依赖引用了不存在的节点 %s", edge.From)
		}
		if _, ok := nodes[edge.To]; !ok {
			return nil, fmt.Errorf("工作流依赖引用了不存在的节点 %s", edge.To)
		}
		children[edge.From] = append(children[edge.From], edge.To)
		incoming[edge.To]++
	}
	ready := []string{}
	for id, count := range incoming {
		if count == 0 {
			ready = append(ready, id)
		}
	}
	ordered := []config.WorkflowNode{}
	for len(ready) > 0 {
		sort.Strings(ready)
		id := ready[0]
		ready = ready[1:]
		ordered = append(ordered, nodes[id])
		for _, child := range children[id] {
			incoming[child]--
			if incoming[child] == 0 {
				ready = append(ready, child)
			}
		}
	}
	if len(ordered) != len(nodes) {
		return nil, fmt.Errorf("工作流存在环形依赖")
	}
	return ordered, nil
}

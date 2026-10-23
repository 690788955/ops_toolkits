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
	if err := validateWorkflowStages(wf, nodes); err != nil {
		return err
	}
	if err := validateWorkflowEdges(wf, nodes); err != nil {
		return err
	}
	_, err := OrderWorkflow(wf)
	return err
}

func (r *Registry) validateWorkflowNode(node config.WorkflowNode, nodeType string, nodes map[string]config.WorkflowNode) error {
	if nodeType != config.WorkflowNodeTypeHTTP && hasHTTPConfig(node.HTTP) {
		return fmt.Errorf("节点 %s 的类型 %s 不能配置 http", node.ID, nodeType)
	}
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
	case config.WorkflowNodeTypeHTTP:
		if node.Tool != "" {
			return fmt.Errorf("HTTP 节点 %s 不能配置 tool", node.ID)
		}
		if node.Condition.Input != "" || len(node.Condition.Cases) > 0 || node.Condition.DefaultCase != "" {
			return fmt.Errorf("HTTP 节点 %s 不能配置 condition", node.ID)
		}
		if hasLoopConfig(node.Loop) {
			return fmt.Errorf("HTTP 节点 %s 不能配置 loop", node.ID)
		}
		if hasUploadConfig(node.Upload) {
			return fmt.Errorf("HTTP 节点 %s 不能配置 upload", node.ID)
		}
		if hasExtractConfig(node.Extract) {
			return fmt.Errorf("HTTP 节点 %s 不能配置 extract_config", node.ID)
		}
		if _, err := config.ValidateHTTPConfig(node.ID, node.HTTP); err != nil {
			return err
		}
	case config.WorkflowNodeTypeShell:
		if node.Tool != "" || hasLoopConfig(node.Loop) || hasUploadConfig(node.Upload) || hasExtractConfig(node.Extract) {
			return fmt.Errorf("Shell 节点 %s 不能同时配置 tool、loop、upload 或 extract_config", node.ID)
		}
		if _, err := config.ValidateScriptConfig(node.ID, node.Script); err != nil {
			return err
		}
	case config.WorkflowNodeTypeChecksum:
		if node.Tool != "" || hasLoopConfig(node.Loop) || hasUploadConfig(node.Upload) || hasExtractConfig(node.Extract) {
			return fmt.Errorf("文件校验节点 %s 不能同时配置 tool、loop、upload 或 extract_config", node.ID)
		}
		if _, err := config.ValidateChecksumConfig(node.ID, node.Checksum); err != nil {
			return err
		}
	case config.WorkflowNodeTypeArchive:
		if node.Tool != "" || hasLoopConfig(node.Loop) || hasUploadConfig(node.Upload) || hasExtractConfig(node.Extract) {
			return fmt.Errorf("解压节点 %s 不能同时配置 tool、loop、upload 或 extract_config", node.ID)
		}
		if _, err := config.ValidateArchiveConfig(node.ID, node.Archive); err != nil {
			return err
		}
	case config.WorkflowNodeTypeWait:
		if node.Tool != "" || hasLoopConfig(node.Loop) || hasUploadConfig(node.Upload) || hasExtractConfig(node.Extract) {
			return fmt.Errorf("等待节点 %s 不能同时配置 tool、loop、upload 或 extract_config", node.ID)
		}
		if _, err := config.ValidateWaitConfig(node.ID, node.Wait); err != nil {
			return err
		}
	case config.WorkflowNodeTypeNotify:
		if node.Tool != "" || hasLoopConfig(node.Loop) || hasUploadConfig(node.Upload) || hasExtractConfig(node.Extract) {
			return fmt.Errorf("通知节点 %s 不能同时配置 tool、loop、upload 或 extract_config", node.ID)
		}
		if _, err := config.ValidateNotifyConfig(node.ID, node.Notify); err != nil {
			return err
		}
	default:
		return fmt.Errorf("节点 %s 使用未知类型: %s", node.ID, nodeType)
	}
	return nil
}

func validateWorkflowConfigFiles(wf *config.WorkflowConfig) error {
	seen := map[string]bool{}
	for _, entry := range wf.ConfigFiles {
		scope := strings.TrimSpace(entry.Scope)
		if scope == "" {
			scope = config.ConfigFileScopePlugin
		}
		if scope != config.ConfigFileScopePlugin && scope != config.ConfigFileScopeHostAbsolute {
			return fmt.Errorf("工作流配置文件 %s 的 scope 只支持 plugin 或 host_absolute", entry.ID)
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
		configDir := strings.TrimSpace(entry.ConfigDir)
		if scope == config.ConfigFileScopeHostAbsolute {
			if !filepath.IsAbs(configDir) {
				return fmt.Errorf("工作流配置文件 %s 的 host_absolute config_dir 必须是绝对目录", entry.ID)
			}
			continue
		}
		if configDir != "" && configDir != "." {
			if err := validateWorkflowRelativePath(configDir); err != nil {
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

func validateWorkflowStages(wf *config.WorkflowConfig, nodes map[string]config.WorkflowNode) error {
	hasNodeStage := false
	for _, node := range wf.Nodes {
		if strings.TrimSpace(node.StageID) != "" {
			hasNodeStage = true
			break
		}
	}
	if len(wf.Stages) == 0 {
		if hasNodeStage {
			return fmt.Errorf("工作流包含 stage_id 但未定义 stages")
		}
		return nil
	}
	stageOrder := map[string]int{}
	for index, stage := range wf.Stages {
		id := strings.TrimSpace(stage.ID)
		if id == "" {
			return fmt.Errorf("阶段 ID 必填")
		}
		if _, ok := stageOrder[id]; ok {
			return fmt.Errorf("阶段 ID 重复: %s", id)
		}
		stageOrder[id] = index
		if stage.ExecutionMode != "" && stage.ExecutionMode != config.WorkflowStageExecutionSerial && stage.ExecutionMode != config.WorkflowStageExecutionParallel {
			return fmt.Errorf("阶段 %s 的 execution_mode 只支持 serial 或 parallel", id)
		}
		if stage.MaxConcurrency < 0 {
			return fmt.Errorf("阶段 %s 的 max_concurrency 不能为负数", id)
		}
	}
	modeByStage := map[string]string{}
	for _, stage := range wf.Stages {
		mode := stage.ExecutionMode
		if mode == "" {
			mode = config.WorkflowStageExecutionSerial
		}
		modeByStage[strings.TrimSpace(stage.ID)] = mode
	}
	orderSeen := map[string]map[int]string{}
	for _, node := range wf.Nodes {
		stageID := strings.TrimSpace(node.StageID)
		if stageID == "" {
			return fmt.Errorf("阶段工作流的节点 %s 缺少 stage_id", node.ID)
		}
		if _, ok := stageOrder[stageID]; !ok {
			return fmt.Errorf("节点 %s 引用了不存在的阶段 %s", node.ID, stageID)
		}
		if modeByStage[stageID] == config.WorkflowStageExecutionParallel {
			// 并行阶段不建立隐式串行链，order 只用于画布展示，不参与调度。
			continue
		}
		if node.Order <= 0 {
			return fmt.Errorf("节点 %s 的 order 必须大于 0", node.ID)
		}
		if orderSeen[stageID] == nil {
			orderSeen[stageID] = map[int]string{}
		}
		if existing, ok := orderSeen[stageID][node.Order]; ok {
			return fmt.Errorf("阶段 %s 内节点 %s 与 %s 的 order 重复: %d", stageID, existing, node.ID, node.Order)
		}
		orderSeen[stageID][node.Order] = node.ID
	}
	for _, edge := range wf.Edges {
		from, fromOK := nodes[edge.From]
		to, toOK := nodes[edge.To]
		if !fromOK || !toOK {
			continue
		}
		fromStage := stageOrder[strings.TrimSpace(from.StageID)]
		toStage := stageOrder[strings.TrimSpace(to.StageID)]
		if fromStage > toStage {
			return fmt.Errorf("依赖 %s -> %s 与阶段顺序冲突", edge.From, edge.To)
		}
		fromStageID := strings.TrimSpace(from.StageID)
		if fromStage == toStage && modeByStage[fromStageID] != config.WorkflowStageExecutionParallel && from.Order >= to.Order {
			return fmt.Errorf("依赖 %s -> %s 与阶段 %s 的节点顺序冲突", edge.From, edge.To, from.StageID)
		}
	}
	return validateParallelStageAcyclic(wf, modeByStage)
}

// validateParallelStageAcyclic 只检查并行阶段内部的显式依赖：串行阶段靠 order 保证无环，
// 并行阶段没有隐式顺序，成环会让调度器永远等不到就绪节点。
func validateParallelStageAcyclic(wf *config.WorkflowConfig, modeByStage map[string]string) error {
	stageIDs := map[string]string{}
	for _, node := range wf.Nodes {
		stageIDs[node.ID] = strings.TrimSpace(node.StageID)
	}
	edges := make([]config.WorkflowEdge, 0, len(wf.Edges))
	for _, edge := range wf.Edges {
		stageID, ok := stageIDs[edge.From]
		if !ok || modeByStage[stageID] != config.WorkflowStageExecutionParallel || stageIDs[edge.To] != stageID {
			continue
		}
		edges = append(edges, edge)
	}
	if len(edges) == 0 {
		return nil
	}
	return orderDependencyEdges(edges)
}

// orderDependencyEdges 只统计阶段内依赖入度；指向其他阶段的边不参与本阶段的环检测。
func orderDependencyEdges(edges []config.WorkflowEdge) error {
	children := map[string][]string{}
	incoming := map[string]int{}
	inStage := map[string]bool{}
	for _, edge := range edges {
		children[edge.From] = append(children[edge.From], edge.To)
		incoming[edge.To]++
		inStage[edge.From] = true
		inStage[edge.To] = true
	}
	ready := []string{}
	for id := range inStage {
		if incoming[id] == 0 {
			ready = append(ready, id)
		}
	}
	visited := 0
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		visited++
		for _, child := range children[id] {
			incoming[child]--
			if incoming[child] == 0 {
				ready = append(ready, child)
			}
		}
	}
	if visited != len(inStage) {
		return fmt.Errorf("并行阶段内的显式依赖存在环")
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
	if hasHTTPConfig(node.HTTP) {
		return config.WorkflowNodeTypeHTTP
	}
	if hasScriptConfig(node.Script) {
		return config.WorkflowNodeTypeShell
	}
	if hasChecksumConfig(node.Checksum) {
		return config.WorkflowNodeTypeChecksum
	}
	if hasArchiveConfig(node.Archive) {
		return config.WorkflowNodeTypeArchive
	}
	if hasWaitConfig(node.Wait) {
		return config.WorkflowNodeTypeWait
	}
	if hasNotifyConfig(node.Notify) {
		return config.WorkflowNodeTypeNotify
	}
	return ""
}

func hasScriptConfig(item config.WorkflowScript) bool {
	return strings.TrimSpace(item.Script) != ""
}

func hasChecksumConfig(item config.WorkflowChecksum) bool {
	return strings.TrimSpace(item.Path) != ""
}

func hasArchiveConfig(item config.WorkflowArchive) bool {
	return strings.TrimSpace(item.Path) != "" || strings.TrimSpace(item.TargetDir) != ""
}

func hasWaitConfig(item config.WorkflowWait) bool {
	return item.Seconds > 0 || strings.TrimSpace(item.PollURL) != "" || strings.TrimSpace(item.PollFile) != ""
}

func hasNotifyConfig(item config.WorkflowNotify) bool {
	return strings.TrimSpace(item.Channel) != "" || strings.TrimSpace(item.URL) != "" || strings.TrimSpace(item.Webhook) != "" || strings.TrimSpace(item.Message) != "" || strings.TrimSpace(item.SMTPHost) != "" || len(item.To) > 0
}

func hasHTTPConfig(item config.WorkflowHTTP) bool {
	return strings.TrimSpace(item.URL) != "" || strings.TrimSpace(item.Method) != "" || len(item.Headers) > 0 || strings.TrimSpace(item.Body) != "" || strings.TrimSpace(item.SaveResponse) != "" || len(item.ExpectStatus) > 0
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
	for _, node := range wf.Nodes {
		if node.ID == "" {
			return nil, fmt.Errorf("节点 ID 必填")
		}
		if _, ok := nodes[node.ID]; ok {
			return nil, fmt.Errorf("节点 ID 重复: %s", node.ID)
		}
		nodes[node.ID] = node
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
	}
	if err := validateWorkflowStages(wf, nodes); err != nil {
		return nil, err
	}
	if len(wf.Stages) == 0 {
		return orderLegacyWorkflow(wf, nodes)
	}
	stageOrder := map[string]int{}
	for index, stage := range wf.Stages {
		stageOrder[strings.TrimSpace(stage.ID)] = index
	}
	// 串行阶段的合法依赖只能指向后续节点，校验通过后无需再次拓扑排序；
	// 并行阶段没有隐式顺序，这里仍按 order 输出稳定顺序，调度时再按依赖与并发度派发。
	ordered := append([]config.WorkflowNode(nil), wf.Nodes...)
	sort.Slice(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		leftStage := stageOrder[strings.TrimSpace(left.StageID)]
		rightStage := stageOrder[strings.TrimSpace(right.StageID)]
		if leftStage != rightStage {
			return leftStage < rightStage
		}
		return left.Order < right.Order
	})
	return ordered, nil
}

func orderLegacyWorkflow(wf *config.WorkflowConfig, nodes map[string]config.WorkflowNode) ([]config.WorkflowNode, error) {
	incoming := map[string]int{}
	children := map[string][]string{}
	for id := range nodes {
		incoming[id] = 0
	}
	for _, edge := range wf.Edges {
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

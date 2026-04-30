package registry

import (
	"fmt"
	"sort"

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
	case config.WorkflowNodeTypeLoop:
		if node.Tool != "" {
			return fmt.Errorf("循环节点 %s 不能同时配置 tool", node.ID)
		}
		if node.Condition.Input != "" || len(node.Condition.Cases) > 0 || node.Condition.DefaultCase != "" {
			return fmt.Errorf("循环节点 %s 不能配置 condition", node.ID)
		}
		loopTool := node.Loop.Tool
		if loopTool == "" && node.Loop.Target != "" {
			targetNode, ok := nodes[node.Loop.Target]
			if !ok {
				return fmt.Errorf("循环节点 %s 的 loop.target 引用了不存在的节点 %s", node.ID, node.Loop.Target)
			}
			if effectiveNodeType(targetNode) != config.WorkflowNodeTypeTool || targetNode.Tool == "" {
				return fmt.Errorf("循环节点 %s 的 loop.target 必须引用工具节点", node.ID)
			}
			loopTool = targetNode.Tool
		}
		if loopTool == "" {
			return fmt.Errorf("循环节点 %s 的 loop.tool 必填", node.ID)
		}
		if _, ok := r.Tools[loopTool]; !ok {
			return fmt.Errorf("循环节点 %s 引用了不存在的工具 %s", node.ID, loopTool)
		}
		if node.Loop.MaxIterations < 1 || node.Loop.MaxIterations > 20 {
			return fmt.Errorf("循环节点 %s 的 loop.max_iterations 必须在 1..20 之间", node.ID)
		}
	default:
		return fmt.Errorf("节点 %s 使用未知类型: %s", node.ID, nodeType)
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
	return ""
}

func hasLoopConfig(loop config.WorkflowLoop) bool {
	return loop.Tool != "" || loop.Target != "" || loop.MaxIterations != 0 || len(loop.Params) > 0
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

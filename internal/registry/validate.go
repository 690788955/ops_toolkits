package registry

import (
	"fmt"
	"sort"

	"shell_ops/internal/config"
)

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
	seen := map[string]bool{}
	for _, node := range wf.Nodes {
		if node.ID == "" {
			return fmt.Errorf("节点 ID 必填")
		}
		if seen[node.ID] {
			return fmt.Errorf("节点 ID 重复: %s", node.ID)
		}
		seen[node.ID] = true
		if node.Tool == "" {
			return fmt.Errorf("节点 %s 的工具必填", node.ID)
		}
		if _, ok := r.Tools[node.Tool]; !ok {
			return fmt.Errorf("节点 %s 引用了不存在的工具 %s", node.ID, node.Tool)
		}
	}
	_, err := OrderWorkflow(wf)
	return err
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

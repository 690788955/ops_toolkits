package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// 审批节点既要能从显式 type 读入，也要能按 approval 段自动识别，
// 否则手写 YAML 省略 type 时会被当成未知类型拒绝。
func TestWorkflowApprovalNodeParsesFromYAML(t *testing.T) {
	raw := []byte(`id: demo.approval
stages:
  - id: change
nodes:
  - id: gate
    type: approval
    stage_id: change
    order: 10
    approval:
      message: "确认 {{ .env }} 的配置已改好"
      timeout: 30m
  - id: gate-short
    stage_id: change
    order: 20
    approval:
      message: 先改配置
edges:
  - from: gate
    to: gate-short
`)
	var wf WorkflowConfig
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("解析工作流 YAML 失败: %v", err)
	}
	NormalizeWorkflow(&wf)
	if got := wf.Nodes[0]; got.Type != WorkflowNodeTypeApproval || got.Approval.Timeout != "30m" || got.Approval.Message == "" {
		t.Fatalf("显式审批节点解析失败: %+v", got)
	}
	if got := wf.Nodes[1]; got.Type != WorkflowNodeTypeApproval || got.Approval.Message != "先改配置" {
		t.Fatalf("省略 type 的审批节点未被识别: %+v", got)
	}

	roundTrip, err := yaml.Marshal(wf)
	if err != nil {
		t.Fatalf("序列化工作流失败: %v", err)
	}
	var again WorkflowConfig
	if err := yaml.Unmarshal(roundTrip, &again); err != nil {
		t.Fatalf("回读工作流失败: %v", err)
	}
	if again.Nodes[0].Approval.Timeout != "30m" || again.Nodes[0].Approval.Message != wf.Nodes[0].Approval.Message {
		t.Fatalf("审批配置在 YAML 往返中丢失: %+v", again.Nodes[0].Approval)
	}
}

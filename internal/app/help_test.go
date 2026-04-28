package app

import (
	"bytes"
	"strings"
	"testing"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

func TestPrintToolHelpIncludesParametersAndExamples(t *testing.T) {
	tool := &registry.Tool{
		Dir: "plugins/plugin.demo",
		Config: &config.ToolConfig{
			ID:          "plugin.demo.greet",
			Name:        "问候演示",
			Description: "输出问候语",
			Category:    "demo",
			Help:        config.HelpConfig{Usage: "opsctl run tool plugin.demo.greet", Examples: []string{"opsctl run tool plugin.demo.greet --set name=Tester"}},
			Parameters:  []config.Parameter{{Name: "name", Type: "string", Required: true, Default: "World", Description: "要问候的名称"}},
			Execution:   config.ExecutionConfig{Type: "shell", Entry: "scripts/run.sh", Timeout: "1m"},
		},
	}

	var out bytes.Buffer
	printToolHelp(&out, tool)
	text := out.String()
	for _, want := range []string{"工具: plugin.demo.greet", "name (必填, 类型=string, 默认值=World)", "opsctl run tool plugin.demo.greet --set name=Tester", "配置: plugins/plugin.demo/tool.yaml"} {
		if !strings.Contains(text, want) {
			t.Fatalf("帮助输出缺少 %q:\n%s", want, text)
		}
	}
}

func TestPrintWorkflowHelpIncludesDAG(t *testing.T) {
	wf := &registry.Workflow{
		Path: "plugins/plugin.demo/workflows/demo-condition.yaml",
		Config: &config.WorkflowConfig{
			ID: "plugin.demo.greet",
			Nodes: []config.WorkflowNode{
				{ID: "first", Tool: "demo.first"},
				{ID: "second", Tool: "demo.second"},
			},
			Edges: []config.WorkflowEdge{{From: "first", To: "second"}},
		},
	}

	var out bytes.Buffer
	printWorkflowHelp(&out, wf)
	text := out.String()
	for _, want := range []string{"工作流: plugin.demo.greet", "first\t工具节点\t工具=demo.first", "first -> second"} {
		if !strings.Contains(text, want) {
			t.Fatalf("workflow 帮助输出缺少 %q:\n%s", want, text)
		}
	}
}

func TestPrintWorkflowHelpIncludesConditionSemantics(t *testing.T) {
	wf := &registry.Workflow{
		Path: "workflows/demo-condition.yaml",
		Config: &config.WorkflowConfig{
			ID: "demo.condition",
			Nodes: []config.WorkflowNode{
				{ID: "inspect", Type: config.WorkflowNodeTypeTool, Tool: "demo.inspect"},
				{ID: "route", Type: config.WorkflowNodeTypeCondition, Name: "按巡检结果分支", Condition: config.WorkflowCondition{Input: "{{ .steps.inspect.stdout }}", Cases: []config.ConditionCase{{ID: "ok", Name: "正常", Operator: "contains", Values: []string{"OK"}}}, DefaultCase: "default"}},
				{ID: "notify", Type: config.WorkflowNodeTypeTool, Tool: "demo.notify"},
			},
			Edges: []config.WorkflowEdge{{From: "inspect", To: "route"}, {From: "route", To: "notify", Case: "ok"}},
		},
	}

	var out bytes.Buffer
	printWorkflowHelp(&out, wf)
	text := out.String()
	for _, want := range []string{"route\t编排节点/条件分支", "输入: {{ .steps.inspect.stdout }}", "ok (正常): contains OK", "默认分支: default", "case=ok -> notify", "route -> notify\tcase=ok"} {
		if !strings.Contains(text, want) {
			t.Fatalf("condition workflow 帮助输出缺少 %q:\n%s", want, text)
		}
	}
}

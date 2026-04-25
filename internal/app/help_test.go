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
		Dir: "tools/demo/hello",
		Config: &config.ToolConfig{
			ID:          "demo.hello",
			Name:        "问候演示",
			Description: "输出问候语",
			Category:    "demo",
			Help:        config.HelpConfig{Usage: "opsctl run tool demo.hello", Examples: []string{"opsctl run tool demo.hello --set name=Tester"}},
			Parameters:  []config.Parameter{{Name: "name", Type: "string", Required: true, Default: "World", Description: "要问候的名称"}},
			Execution:   config.ExecutionConfig{Type: "shell", Entry: "bin/run.sh", Timeout: "1m"},
		},
	}

	var out bytes.Buffer
	printToolHelp(&out, tool)
	text := out.String()
	for _, want := range []string{"工具: demo.hello", "name (必填, 类型=string, 默认值=World)", "opsctl run tool demo.hello --set name=Tester", "配置: tools/demo/hello/tool.yaml"} {
		if !strings.Contains(text, want) {
			t.Fatalf("帮助输出缺少 %q:\n%s", want, text)
		}
	}
}

func TestPrintWorkflowHelpIncludesDAG(t *testing.T) {
	wf := &registry.Workflow{
		Path: "workflows/demo-hello.yaml",
		Config: &config.WorkflowConfig{
			ID: "demo.hello",
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
	for _, want := range []string{"工作流: demo.hello", "first\t工具=demo.first", "first -> second"} {
		if !strings.Contains(text, want) {
			t.Fatalf("workflow 帮助输出缺少 %q:\n%s", want, text)
		}
	}
}

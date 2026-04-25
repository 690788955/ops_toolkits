package app

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

func printCatalogHelp(out io.Writer, reg *registry.Registry) {
	fmt.Fprintf(out, "%s\n", fallbackText(reg.Root.DisplayName(), "opsctl"))
	if desc := reg.Root.DisplayDescription(); desc != "" {
		fmt.Fprintf(out, "%s\n", desc)
	}
	fmt.Fprintln(out, "\n分类:")
	for _, category := range reg.Root.DisplayCategories() {
		fmt.Fprintf(out, "  %s\t%s\n", category.ID, category.Description)
	}
	fmt.Fprintln(out, "\n工具:")
	for _, id := range sortedToolIDs(reg) {
		tool := reg.Tools[id]
		fmt.Fprintf(out, "  %s\t%s\n", id, tool.Entry.Description)
	}
	fmt.Fprintln(out, "\n工作流:")
	for _, id := range sortedWorkflowIDs(reg) {
		wf := reg.Workflows[id]
		fmt.Fprintf(out, "  %s\t%s\n", id, wf.Entry.Description)
	}
	fmt.Fprintln(out, "\n示例:")
	fmt.Fprintln(out, "  opsctl help-auto tool <tool-id>")
	fmt.Fprintln(out, "  opsctl help-auto workflow <workflow-id>")
	fmt.Fprintln(out, "  opsctl run tool <tool-id> --set key=value --no-prompt")
}

func printToolHelp(out io.Writer, tool *registry.Tool) {
	cfg := tool.Config
	fmt.Fprintf(out, "工具: %s\n", cfg.ID)
	fmt.Fprintf(out, "名称: %s\n", fallbackText(cfg.Name, cfg.ID))
	if cfg.Description != "" {
		fmt.Fprintf(out, "描述: %s\n", cfg.Description)
	}
	if cfg.Category != "" {
		fmt.Fprintf(out, "分类: %s\n", cfg.Category)
	}
	fmt.Fprintf(out, "配置: %s/tool.yaml\n", filepathSlash(tool.Dir))
	if cfg.Help.Usage != "" {
		fmt.Fprintf(out, "\n用法:\n  %s\n", cfg.Help.Usage)
	} else {
		fmt.Fprintf(out, "\n用法:\n  opsctl run tool %s --set key=value --no-prompt\n", cfg.ID)
	}
	printParameters(out, cfg.Parameters)
	if len(cfg.Help.Examples) > 0 {
		fmt.Fprintln(out, "\n示例:")
		for _, item := range cfg.Help.Examples {
			fmt.Fprintf(out, "  %s\n", item)
		}
	}
	fmt.Fprintln(out, "\n执行:")
	fmt.Fprintf(out, "  类型: %s\n", fallbackText(cfg.Execution.Type, "shell"))
	fmt.Fprintf(out, "  入口: %s\n", cfg.Execution.Entry)
	fmt.Fprintf(out, "  超时: %s\n", fallbackText(cfg.Execution.Timeout, "30m"))
	if cfg.Confirm.Required {
		fmt.Fprintf(out, "\n确认: %s\n", fallbackText(cfg.Confirm.Message, "必填"))
	}
}

func printWorkflowHelp(out io.Writer, wf *registry.Workflow) {
	cfg := wf.Config
	fmt.Fprintf(out, "工作流: %s\n", cfg.ID)
	fmt.Fprintf(out, "名称: %s\n", fallbackText(cfg.Name, cfg.ID))
	if cfg.Description != "" {
		fmt.Fprintf(out, "描述: %s\n", cfg.Description)
	}
	if cfg.Category != "" {
		fmt.Fprintf(out, "分类: %s\n", cfg.Category)
	}
	fmt.Fprintf(out, "配置: %s\n", filepathSlash(wf.Path))
	fmt.Fprintf(out, "\n用法:\n  opsctl run workflow %s --set key=value --no-prompt\n", cfg.ID)
	printParameters(out, cfg.Parameters)
	fmt.Fprintln(out, "\nDAG 节点:")
	for _, node := range cfg.Nodes {
		fmt.Fprintf(out, "  %s\t工具=%s", node.ID, node.Tool)
		if node.Name != "" {
			fmt.Fprintf(out, "\t%s", node.Name)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "\nDAG 依赖:")
	if len(cfg.Edges) == 0 {
		fmt.Fprintln(out, "  无")
	} else {
		for _, edge := range cfg.Edges {
			fmt.Fprintf(out, "  %s -> %s\n", edge.From, edge.To)
		}
	}
	if cfg.Confirm.Required {
		fmt.Fprintf(out, "\n确认: %s\n", fallbackText(cfg.Confirm.Message, "必填"))
	}
}

func printParameters(out io.Writer, params []config.Parameter) {
	fmt.Fprintln(out, "\n参数:")
	if len(params) == 0 {
		fmt.Fprintln(out, "  无")
		return
	}
	for _, param := range params {
		requiredText := "可选"
		if param.Required {
			requiredText = "必填"
		}
		parts := []string{requiredText}
		if param.Type != "" {
			parts = append(parts, "类型="+param.Type)
		}
		if param.Default != nil {
			parts = append(parts, fmt.Sprintf("默认值=%v", param.Default))
		}
		fmt.Fprintf(out, "  %s (%s)", param.Name, strings.Join(parts, ", "))
		if param.Description != "" {
			fmt.Fprintf(out, " - %s", param.Description)
		}
		fmt.Fprintln(out)
	}
}

func sortedToolIDs(reg *registry.Registry) []string {
	ids := make([]string, 0, len(reg.Tools))
	for id := range reg.Tools {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedWorkflowIDs(reg *registry.Registry) []string {
	ids := make([]string, 0, len(reg.Workflows))
	for id := range reg.Workflows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func fallbackText(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func filepathSlash(value string) string {
	return strings.ReplaceAll(value, "\\", "/")
}

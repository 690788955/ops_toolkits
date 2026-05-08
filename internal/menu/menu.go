package menu

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
	"shell_ops/internal/runner"
)

type item struct {
	kind        string
	id          string
	name        string
	description string
}

const allCategoryID = "__all__"

var allCategory = config.Category{
	ID:          allCategoryID,
	Name:        "全局/全部",
	Description: "显示所有工具和工作流",
}

type RunOptions struct {
	FileParams map[string]string
	Overrides  map[string]string
	NoPrompt   bool
}

func Run(ctx context.Context, reg *registry.Registry, in io.Reader, out, errOut io.Writer) error {
	return RunWithOptions(ctx, reg, in, out, errOut, RunOptions{})
}

func RunWithOptions(ctx context.Context, reg *registry.Registry, in io.Reader, out, errOut io.Writer, opts RunOptions) error {
	scanner := bufio.NewScanner(in)
	promptIn := &scannerLineReader{scanner: scanner}
	for {
		category, ok, err := selectCategory(reg, scanner, out)
		if err != nil || !ok {
			return err
		}
		selected, ok, err := selectItem(reg, category.ID, scanner, out)
		if err != nil || !ok {
			continue
		}
		if err := runSelected(ctx, reg, selected, promptIn, out, errOut, opts); err != nil {
			fmt.Fprintf(errOut, "执行失败: %v\n", err)
		}
		fmt.Fprint(out, "\n按回车返回主菜单，输入 q 退出: ")
		if !scanner.Scan() || strings.EqualFold(strings.TrimSpace(scanner.Text()), "q") {
			return scanner.Err()
		}
	}
}

type scannerLineReader struct {
	scanner *bufio.Scanner
	pending []byte
}

func (r *scannerLineReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.pending) == 0 {
		if !r.scanner.Scan() {
			if err := r.scanner.Err(); err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		r.pending = append([]byte(r.scanner.Text()), '\n')
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func selectCategory(reg *registry.Registry, scanner *bufio.Scanner, out io.Writer) (config.Category, bool, error) {
	categories := append([]config.Category{}, reg.Root.DisplayCategories()...)
	categories = append(categories, allCategory)
	for {
		fmt.Fprintf(out, "\n%s\n", title(reg.Root.DisplayName(), "运维框架"))
		fmt.Fprintln(out, "请选择运维分类:")
		for i, category := range categories {
			fmt.Fprintf(out, "%d) %s", i+1, title(category.Name, category.ID))
			if category.Description != "" {
				fmt.Fprintf(out, " - %s", category.Description)
			}
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, "q) 退出")
		fmt.Fprint(out, "选择: ")
		if !scanner.Scan() {
			return config.Category{}, false, scanner.Err()
		}
		text := strings.TrimSpace(scanner.Text())
		if strings.EqualFold(text, "q") {
			return config.Category{}, false, nil
		}
		idx, err := strconv.Atoi(text)
		if err == nil && idx >= 1 && idx <= len(categories) {
			return categories[idx-1], true, nil
		}
		fmt.Fprintln(out, "无效选择，请重新输入。")
	}
}

func selectItem(reg *registry.Registry, categoryID string, scanner *bufio.Scanner, out io.Writer) (item, bool, error) {
	items := itemsForCategory(reg, categoryID)
	if len(items) == 0 {
		fmt.Fprintln(out, "该分类下暂无工具或工作流。")
		return item{}, false, nil
	}
	visible := items
	for {
		fmt.Fprintln(out, "\n请选择要执行的工具或工作流:")
		for i, it := range visible {
			fmt.Fprintf(out, "%d) [%s] %s", i+1, labelKind(it.kind), title(it.name, it.id))
			if it.description != "" {
				fmt.Fprintf(out, " - %s", it.description)
			}
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, "s) 搜索")
		fmt.Fprintln(out, "b) 返回上级")
		fmt.Fprintln(out, "q) 退出")
		fmt.Fprint(out, "选择: ")
		if !scanner.Scan() {
			return item{}, false, scanner.Err()
		}
		text := strings.TrimSpace(scanner.Text())
		switch strings.ToLower(text) {
		case "s":
			filtered, err := searchItems(items, scanner, out)
			if err != nil {
				return item{}, false, err
			}
			visible = filtered
			continue
		case "b":
			return item{}, false, nil
		case "q":
			return item{}, false, io.EOF
		}
		idx, err := strconv.Atoi(text)
		if err == nil && idx >= 1 && idx <= len(visible) {
			return visible[idx-1], true, nil
		}
		fmt.Fprintln(out, "无效选择，请重新输入。")
	}
}

func searchItems(items []item, scanner *bufio.Scanner, out io.Writer) ([]item, error) {
	fmt.Fprint(out, "搜索关键词（空值显示全部）: ")
	if !scanner.Scan() {
		return nil, scanner.Err()
	}
	query := strings.TrimSpace(scanner.Text())
	if query == "" {
		return items, nil
	}
	filtered := filterItems(items, query)
	if len(filtered) == 0 {
		fmt.Fprintln(out, "未找到匹配项，已恢复显示全部。")
		return items, nil
	}
	fmt.Fprintf(out, "已匹配 %d 项。\n", len(filtered))
	return filtered, nil
}

func filterItems(items []item, query string) []item {
	query = strings.ToLower(query)
	filtered := []item{}
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.id), query) || strings.Contains(strings.ToLower(it.name), query) || strings.Contains(strings.ToLower(it.description), query) {
			filtered = append(filtered, it)
		}
	}
	return filtered
}

func itemsForCategory(reg *registry.Registry, categoryID string) []item {
	items := []item{}
	for _, tool := range reg.Tools {
		if categoryID == allCategoryID || tool.Entry.Category == categoryID {
			items = append(items, item{kind: "tool", id: tool.Entry.ID, name: tool.Entry.Name, description: tool.Entry.Description})
		}
	}
	for _, wf := range reg.Workflows {
		if categoryID == allCategoryID || wf.Entry.Category == categoryID {
			items = append(items, item{kind: "workflow", id: wf.Entry.ID, name: wf.Entry.Name, description: wf.Entry.Description})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].kind == items[j].kind {
			return items[i].id < items[j].id
		}
		return items[i].kind < items[j].kind
	})
	return items
}

func runSelected(ctx context.Context, reg *registry.Registry, selected item, in io.Reader, out, errOut io.Writer, opts RunOptions) error {
	if selected.kind == "workflow" {
		if err := printWorkflowDetails(reg, selected.id, out); err != nil {
			return err
		}
	}
	defs, err := definitionsFor(reg, selected)
	if err != nil {
		return err
	}
	r := runner.New(reg)
	if selected.kind == "tool" {
		tool, err := reg.Tool(selected.id)
		if err != nil {
			return err
		}
		provided := config.MergeValues(config.StringMapToValues(opts.FileParams), config.StringMapToValues(opts.Overrides))
		allValues := config.MergeValues(menuToolConfigLayers(reg, tool), provided)
		if opts.NoPrompt {
			if err := config.ValidateRequiredValues(defs, allValues); err != nil {
				return err
			}
		} else {
			before := config.FlattenValues(allValues)
			promptParams := config.FlattenValues(allValues)
			if err := config.PromptAll(defs, promptParams, in, out); err != nil {
				return err
			}
			for k, v := range promptParams {
				if strings.TrimSpace(v) == "" {
					continue
				}
				if before[k] == v {
					continue
				}
				if err := config.SetPathValue(provided, k, v); err != nil {
					return err
				}
			}
		}
		if err := config.PromptConfirmation(tool.Config.Confirm, in, out); err != nil {
			return err
		}
		record, err := r.RunToolValues(ctx, selected.id, provided, out, errOut)
		printRecord(out, record)
		return err
	}
	params := config.MergeParams(defs, opts.FileParams, opts.Overrides)
	if opts.NoPrompt {
		if err := config.ValidateRequired(defs, params); err != nil {
			return err
		}
	} else if err := config.PromptAll(defs, params, in, out); err != nil {
		return err
	}
	wf, err := reg.Workflow(selected.id)
	if err != nil {
		return err
	}
	if err := config.PromptConfirmation(wf.Config.Confirm, in, out); err != nil {
		return err
	}
	confirmed, err := confirmWorkflowTools(reg, wf.Config, in, out)
	if err != nil {
		return err
	}
	record, err := r.RunWorkflowWithConfirmation(ctx, selected.id, params, confirmed, out, errOut)
	printRecord(out, record)
	return err
}

func menuToolConfigLayers(reg *registry.Registry, tool *registry.Tool) config.Values {
	return config.MergeValues(
		config.MergeParameterDefaults(tool.Config.Parameters),
		reg.GlobalEnv,
		reg.Root.ConfigDefaults,
	)
}

func confirmWorkflowTools(reg *registry.Registry, wf *config.WorkflowConfig, in io.Reader, out io.Writer) (bool, error) {
	confirmed := false
	for _, node := range wf.Nodes {
		nodeType := workflowNodeType(node)
		toolID := node.Tool
		if nodeType == config.WorkflowNodeTypeLoop {
			toolID = node.Loop.Tool
		}
		if nodeType != config.WorkflowNodeTypeTool && nodeType != config.WorkflowNodeTypeLoop {
			continue
		}
		if toolID == "" {
			continue
		}
		tool, err := reg.Tool(toolID)
		if err != nil {
			return false, err
		}
		if !tool.Config.Confirm.Required || node.Confirm {
			continue
		}
		if err := config.PromptConfirmation(tool.Config.Confirm, in, out); err != nil {
			return false, err
		}
		confirmed = true
	}
	return confirmed, nil
}

func definitionsFor(reg *registry.Registry, selected item) ([]config.Parameter, error) {
	if selected.kind == "tool" {
		tool, err := reg.Tool(selected.id)
		if err != nil {
			return nil, err
		}
		return tool.Config.Parameters, nil
	}
	wf, err := reg.Workflow(selected.id)
	if err != nil {
		return nil, err
	}
	return wf.Config.Parameters, nil
}

func printWorkflowDetails(reg *registry.Registry, id string, out io.Writer) error {
	wf, err := reg.Workflow(id)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\n工作流详情: %s\n", title(wf.Config.Name, wf.Config.ID))
	for _, node := range wf.Config.Nodes {
		if workflowNodeType(node) == config.WorkflowNodeTypeCondition {
			fmt.Fprintf(out, "  %s [编排节点/条件分支] 输入=%s 默认=%s\n", node.ID, title(node.Condition.Input, "未配置"), title(node.Condition.DefaultCase, "未启用"))
			for _, item := range node.Condition.Cases {
				fmt.Fprintf(out, "    case %s (%s): %s", item.ID, title(item.Name, item.ID), item.Operator)
				if len(item.Values) > 0 {
					fmt.Fprintf(out, " %s", strings.Join(item.Values, ", "))
				}
				fmt.Fprintln(out)
			}
			continue
		}
		if workflowNodeType(node) == config.WorkflowNodeTypeLoop {
			fmt.Fprintf(out, "  %s [编排节点/循环] 工具=%s 次数=%d\n", node.ID, title(node.Loop.Tool, "未配置"), node.Loop.MaxIterations)
			continue
		}
		fmt.Fprintf(out, "  %s [工具节点] 工具=%s\n", node.ID, node.Tool)
	}
	if len(wf.Config.Edges) > 0 {
		fmt.Fprintln(out, "  分支/依赖:")
		for _, edge := range wf.Config.Edges {
			fmt.Fprintf(out, "    %s -> %s", edge.From, edge.To)
			if edge.Case != "" {
				fmt.Fprintf(out, " case=%s", edge.Case)
			}
			fmt.Fprintln(out)
		}
	}
	return nil
}

func workflowNodeType(node config.WorkflowNode) string {
	if node.Type != "" {
		return node.Type
	}
	if node.Tool != "" {
		return config.WorkflowNodeTypeTool
	}
	if node.Loop.Tool != "" || node.Loop.Target != "" || node.Loop.MaxIterations != 0 || len(node.Loop.Params) > 0 {
		return config.WorkflowNodeTypeLoop
	}
	return ""
}

func printRecord(out io.Writer, record *runner.RunRecord) {
	if record == nil {
		return
	}
	fmt.Fprintf(out, "\nrun_id=%s status=%s\n", record.ID, record.Status)
	if len(record.Steps) == 0 {
		return
	}
	fmt.Fprintln(out, "步骤:")
	for _, step := range record.Steps {
		fmt.Fprintf(out, "  %s [%s] %s", step.ID, displayStepType(step), step.Status)
		if step.Tool != "" {
			fmt.Fprintf(out, " tool=%s", step.Tool)
		}
		if step.ConditionInput != "" {
			fmt.Fprintf(out, " input=%q", step.ConditionInput)
		}
		if step.MatchedCase != "" {
			fmt.Fprintf(out, " matched_case=%s", step.MatchedCase)
		}
		if step.SkippedReason != "" {
			fmt.Fprintf(out, " skipped_reason=%s", step.SkippedReason)
		}
		if step.Error != "" {
			fmt.Fprintf(out, " error=%s", step.Error)
		}
		fmt.Fprintln(out)
	}
}

func displayStepType(step runner.StepRecord) string {
	if step.Type == config.WorkflowNodeTypeCondition {
		return "编排节点/条件分支"
	}
	if step.Type == config.WorkflowNodeTypeLoop {
		return "编排节点/循环"
	}
	return "工具节点"
}

func title(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func labelKind(kind string) string {
	if kind == "workflow" {
		return "工作流"
	}
	return "工具"
}

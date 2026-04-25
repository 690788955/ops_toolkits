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

func Run(ctx context.Context, reg *registry.Registry, in io.Reader, out, errOut io.Writer) error {
	scanner := bufio.NewScanner(in)
	for {
		category, ok, err := selectCategory(reg, scanner, out)
		if err != nil || !ok {
			return err
		}
		selected, ok, err := selectItem(reg, category.ID, scanner, out)
		if err != nil || !ok {
			continue
		}
		if err := runSelected(ctx, reg, selected, in, out, errOut); err != nil {
			fmt.Fprintf(errOut, "执行失败: %v\n", err)
		}
		fmt.Fprint(out, "\n按回车返回主菜单，输入 q 退出: ")
		if !scanner.Scan() || strings.EqualFold(strings.TrimSpace(scanner.Text()), "q") {
			return scanner.Err()
		}
	}
}

func selectCategory(reg *registry.Registry, scanner *bufio.Scanner, out io.Writer) (config.Category, bool, error) {
	categories := reg.Root.DisplayCategories()
	if len(categories) == 0 {
		return config.Category{}, false, fmt.Errorf("未配置分类")
	}
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
	for {
		fmt.Fprintln(out, "\n请选择要执行的工具或工作流:")
		for i, it := range items {
			fmt.Fprintf(out, "%d) [%s] %s", i+1, labelKind(it.kind), title(it.name, it.id))
			if it.description != "" {
				fmt.Fprintf(out, " - %s", it.description)
			}
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, "b) 返回上级")
		fmt.Fprintln(out, "q) 退出")
		fmt.Fprint(out, "选择: ")
		if !scanner.Scan() {
			return item{}, false, scanner.Err()
		}
		text := strings.TrimSpace(scanner.Text())
		switch strings.ToLower(text) {
		case "b":
			return item{}, false, nil
		case "q":
			return item{}, false, io.EOF
		}
		idx, err := strconv.Atoi(text)
		if err == nil && idx >= 1 && idx <= len(items) {
			return items[idx-1], true, nil
		}
		fmt.Fprintln(out, "无效选择，请重新输入。")
	}
}

func itemsForCategory(reg *registry.Registry, categoryID string) []item {
	items := []item{}
	for _, tool := range reg.Tools {
		if tool.Entry.Category == categoryID {
			items = append(items, item{kind: "tool", id: tool.Entry.ID, name: tool.Entry.Name, description: tool.Entry.Description})
		}
	}
	for _, wf := range reg.Workflows {
		if wf.Entry.Category == categoryID {
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

func runSelected(ctx context.Context, reg *registry.Registry, selected item, in io.Reader, out, errOut io.Writer) error {
	defs, err := definitionsFor(reg, selected)
	if err != nil {
		return err
	}
	params := config.MergeParams(defs, nil, nil)
	if err := config.PromptMissing(defs, params, in, out); err != nil {
		return err
	}
	r := runner.New(reg)
	if selected.kind == "tool" {
		record, err := r.RunTool(ctx, selected.id, params, out, errOut)
		printRecord(out, record)
		return err
	}
	record, err := r.RunWorkflow(ctx, selected.id, params, out, errOut)
	printRecord(out, record)
	return err
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

func printRecord(out io.Writer, record *runner.RunRecord) {
	if record != nil {
		fmt.Fprintf(out, "\nrun_id=%s status=%s\n", record.ID, record.Status)
	}
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

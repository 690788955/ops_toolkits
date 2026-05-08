package menu

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

func TestSelectCategoryListsAllCategory(t *testing.T) {
	reg := testRegistry()
	var out bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader("q\n"))

	_, ok, err := selectCategory(reg, scanner, &out)
	if err != nil {
		t.Fatalf("selectCategory returned error: %v", err)
	}
	if ok {
		t.Fatal("selectCategory ok = true, want false after q")
	}

	text := out.String()
	if !strings.Contains(text, "全局/全部") {
		t.Fatalf("menu output = %q, want 全局/全部", text)
	}
	if !strings.Contains(text, "显示所有工具和工作流") {
		t.Fatalf("menu output = %q, want all category description", text)
	}
}

func TestItemsForAllCategoryIncludesAllToolsAndWorkflows(t *testing.T) {
	items := itemsForCategory(testRegistry(), allCategoryID)

	want := map[string]string{
		"tool:demo.deploy":      "部署工具",
		"tool:ops.cleanup":      "清理工具",
		"workflow:demo.release": "发布流程",
		"workflow:ops.maintain": "维护流程",
	}
	if len(items) != len(want) {
		t.Fatalf("len(items) = %d, want %d: %#v", len(items), len(want), items)
	}
	for _, it := range items {
		key := it.kind + ":" + it.id
		name, ok := want[key]
		if !ok {
			t.Fatalf("unexpected item: %#v", it)
		}
		if it.name != name {
			t.Fatalf("item %s name = %q, want %q", key, it.name, name)
		}
	}
}

func TestItemsForRealCategoryOnlyIncludesCategoryItems(t *testing.T) {
	items := itemsForCategory(testRegistry(), "demo")

	want := map[string]bool{
		"tool:demo.deploy":      true,
		"workflow:demo.release": true,
	}
	if len(items) != len(want) {
		t.Fatalf("len(items) = %d, want %d: %#v", len(items), len(want), items)
	}
	for _, it := range items {
		key := it.kind + ":" + it.id
		if !want[key] {
			t.Fatalf("unexpected item for real category: %#v", it)
		}
	}
}

func TestSelectItemSearchFiltersVisibleItems(t *testing.T) {
	var out bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader("s\ncleanup\n1\n"))

	selected, ok, err := selectItem(testRegistry(), allCategoryID, scanner, &out)
	if err != nil {
		t.Fatalf("selectItem returned error: %v", err)
	}
	if !ok {
		t.Fatal("selectItem ok = false, want true")
	}
	if selected.id != "ops.cleanup" {
		t.Fatalf("selected.id = %q, want ops.cleanup", selected.id)
	}
	text := out.String()
	for _, want := range []string{"s) 搜索", "搜索关键词", "已匹配 1 项"} {
		if !strings.Contains(text, want) {
			t.Fatalf("搜索输出缺少 %q:\n%s", want, text)
		}
	}
}

func TestFilterItemsMatchesIDNameAndDescription(t *testing.T) {
	items := []item{
		{kind: "tool", id: "demo.deploy", name: "部署工具", description: "发布应用"},
		{kind: "tool", id: "ops.cleanup", name: "清理工具", description: "删除临时文件"},
	}
	for _, query := range []string{"deploy", "清理", "临时"} {
		if got := filterItems(items, query); len(got) != 1 {
			t.Fatalf("filterItems(%q) len = %d, want 1: %#v", query, len(got), got)
		}
	}
}

func testRegistry() *registry.Registry {
	return &registry.Registry{
		Root: &config.RootConfig{
			App: config.AppConfig{Name: "测试应用"},
			Menu: config.MenuConfig{Categories: []config.Category{
				{ID: "demo", Name: "演示", Description: "演示分类"},
				{ID: "ops", Name: "运维", Description: "运维分类"},
			}},
		},
		Tools: map[string]*registry.Tool{
			"demo.deploy": {Entry: config.ToolEntry{ID: "demo.deploy", Category: "demo", Name: "部署工具"}},
			"ops.cleanup": {Entry: config.ToolEntry{ID: "ops.cleanup", Category: "ops", Name: "清理工具"}},
		},
		Workflows: map[string]*registry.Workflow{
			"demo.release": {Entry: config.WorkflowRef{ID: "demo.release", Category: "demo", Name: "发布流程"}},
			"ops.maintain": {Entry: config.WorkflowRef{ID: "ops.maintain", Category: "ops", Name: "维护流程"}},
		},
	}
}

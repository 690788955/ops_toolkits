# CLI Interaction Patterns

> Patterns for CLI commands, interactive menus, and user-facing output in opsctl.

---

## Parameter Prompts

### Pattern: Sequential Confirmation for Interactive Parameters

Interactive CLI execution should confirm every declared parameter in definition order, not only missing parameters. This applies to `opsctl run`, `opsctl start`, and `opsctl menu` paths when `--no-prompt` is not set.

**Contract**:

- Merge defaults, parameter files, and `--set` overrides first.
- Prompt each parameter once, preserving the configured order.
- Display metadata plus the current value when present.
- Empty input keeps the current value; if no current value exists, it uses the default value.
- Non-empty input overrides the current value for this run.
- Required parameters that remain empty fail with `缺少必填参数 <name>`.
- `--no-prompt` skips all parameter prompts and only validates required values.
- Confirmation for high-risk tools/workflows still runs after parameter confirmation.

**Example Output**:

```text
message 要显示的消息 (类型=string, 必填, 默认值=Hello World, 当前值=Hello World)
请输入 [当前: Hello World]:
```

**Why**: Menu users expect CLI execution to behave like the Web execution form: all inputs are visible and can be changed before execution. Prompting only missing values silently accepts defaults and hides important runtime choices.

**Location**: `internal/config/params.go`, `internal/app/app.go`, `internal/menu/menu.go`

### Pattern: Enhanced Parameter Labels

When prompting for parameters, display complete metadata to help operators understand what they're entering.

**Implementation**:

```go
func parameterPromptLabel(param Parameter) string {
    parts := []string{param.Name}
    if param.Description != "" {
        parts = append(parts, param.Description)
    }
    meta := []string{}
    if param.Type != "" {
        meta = append(meta, "类型="+param.Type)
    }
    if param.Required {
        meta = append(meta, "必填")
    } else {
        meta = append(meta, "可选")
    }
    if param.Default != nil {
        meta = append(meta, fmt.Sprintf("默认值=%v", param.Default))
    }
    if len(meta) > 0 {
        parts = append(parts, "("+strings.Join(meta, ", ")+")")
    }
    return strings.Join(parts, " ")
}
```

**Example Output**:

```
name 用户名称 (类型=string, 必填): 
host 目标主机 (类型=string, 可选, 默认值=localhost): 
```

**Why**: Operators need to understand parameter constraints before entering values. Showing type, required/optional status, and defaults reduces input errors and support questions.

**Location**: `internal/config/params.go`

---

## Interactive Menu

### Pattern: Search Filtering

Interactive menus should support keyword search to help operators quickly locate tools/workflows when the catalog is large.

**Implementation**:

```go
func selectItem(reg *registry.Registry, categoryID string, scanner *bufio.Scanner, out io.Writer) (item, bool, error) {
    items := itemsForCategory(reg, categoryID)
    visible := items  // Track filtered subset
    
    for {
        // Display visible items
        for i, it := range visible {
            fmt.Fprintf(out, "%d) [%s] %s", i+1, labelKind(it.kind), title(it.name, it.id))
            // ...
        }
        fmt.Fprintln(out, "s) 搜索")
        fmt.Fprintln(out, "b) 返回上级")
        fmt.Fprintln(out, "q) 退出")
        
        // Handle search
        case "s":
            filtered, err := searchItems(items, scanner, out)
            if err != nil {
                return item{}, false, err
            }
            visible = filtered
            continue
        
        // Selection uses visible, not items
        idx, err := strconv.Atoi(text)
        if err == nil && idx >= 1 && idx <= len(visible) {
            return visible[idx-1], true, nil
        }
    }
}

func filterItems(items []item, query string) []item {
    query = strings.ToLower(query)
    filtered := []item{}
    for _, it := range items {
        if strings.Contains(strings.ToLower(it.id), query) || 
           strings.Contains(strings.ToLower(it.name), query) || 
           strings.Contains(strings.ToLower(it.description), query) {
            filtered = append(filtered, it)
        }
    }
    return filtered
}
```

**Why**: When the catalog contains dozens of tools, scrolling through numbered lists is slow. Case-insensitive substring search across ID/name/description lets operators jump directly to what they need.

**UX Flow**:
1. Operator enters `s` to search
2. Enters keyword (e.g., "backup")
3. Menu redisplays with only matching items
4. Empty query restores full list
5. Zero matches shows warning and restores full list

**Location**: `internal/menu/menu.go`

---

## CLI Output Formatting

### Pattern: Category-Grouped List Output

`opsctl list` should group tools and workflows by category, preserving configured category order and using aligned columns.

**Implementation**:

```go
func printCatalogList(out io.Writer, reg *registry.Registry) {
    w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
    for _, category := range listCategories(reg) {
        fmt.Fprintf(w, "%s\n", fallbackText(category.Name, category.ID))
        printCategoryTools(w, reg, category.ID)
        printCategoryWorkflows(w, reg, category.ID)
        fmt.Fprintln(w)
    }
    w.Flush()
}

func listCategories(reg *registry.Registry) []config.Category {
    categories := append([]config.Category{}, reg.Root.DisplayCategories()...)
    seen := map[string]bool{}
    for _, category := range categories {
        seen[category.ID] = true
    }
    
    // Collect uncategorized items
    extra := []config.Category{}
    for _, tool := range reg.Tools {
        categoryID := itemCategory(tool.Entry.Category)
        if !seen[categoryID] {
            extra = append(extra, config.Category{ID: categoryID, Name: categoryName(categoryID)})
            seen[categoryID] = true
        }
    }
    // ... same for workflows
    
    sort.SliceStable(extra, func(i, j int) bool { return extra[i].ID < extra[j].ID })
    return append(categories, extra...)  // Configured first, then sorted extras
}
```

**Example Output**:

```
ES 备份恢复
  工具
    lzyh.es_backup.create   ES Snapshot 备份    按日期目录创建 ES Snapshot
    lzyh.es_backup.list     ES Snapshot 备份查询  列出可恢复备份
  工作流
    lzyh.es_backup.restore  ES Snapshot 恢复    将指定日期的 ES Snapshot 恢复

系统管理
  工具
    plugin.system-check.info  系统资源检查  检查系统版本、CPU、内存
```

**Why**: 
- Grouping by category helps operators navigate large catalogs
- Preserving configured category order respects intentional prioritization
- `text/tabwriter` ensures columns align regardless of ID length
- Uncategorized items get a synthetic `__uncategorized__` category to avoid silent omission

**Location**: `internal/app/app.go`

---

## Command Evolution

### Pattern: Primary Command with Compatibility Alias

When consolidating duplicate commands, designate one as primary and keep the other as a documented alias.

**Implementation**:

```go
func startCommand(opts *options) *cobra.Command {
    cmd := interactiveCommand(opts, "start", "启动交互式运维控制台", false)
    cmd.AddCommand(startToolCommand(opts), startWorkflowCommand(opts))
    return cmd
}

func menuCommand(opts *options) *cobra.Command {
    return interactiveCommand(opts, "menu", "打开编号菜单（start 的兼容别名）", true)
}

func interactiveCommand(opts *options, use, short string, alias bool) *cobra.Command {
    cmd := &cobra.Command{
        Use: use + " [tool-or-workflow-id]", 
        Short: short, 
        Args: cobra.MaximumNArgs(1), 
        RunE: func(cmd *cobra.Command, args []string) error {
            // ... implementation
        },
    }
    if alias {
        cmd.Long = "打开编号菜单。该命令为兼容保留；新用法推荐 opsctl start。"
    } else {
        cmd.Long = "启动交互式运维控制台。无参数时打开编号菜单，也可以使用子命令快捷执行工具或工作流。"
    }
    return cmd
}
```

**Why**: 
- Avoids breaking existing scripts and muscle memory
- `Short` description clearly marks the alias status
- `Long` description guides new users to the primary command
- Both commands share implementation, so no maintenance burden

**Example**:

```bash
$ opsctl start --help
启动交互式运维控制台。无参数时打开编号菜单，也可以使用子命令快捷执行工具或工作流。

$ opsctl menu --help
打开编号菜单（start 的兼容别名）

Usage:
  opsctl menu [tool-or-workflow-id] [flags]
  
打开编号菜单。该命令为兼容保留；新用法推荐 opsctl start。
```

**Location**: `internal/app/app.go`

---

## Testing Requirements

- Parameter prompt tests must verify label format includes type, required/optional, and default value
- Menu search tests must cover: match by ID, match by name, match by description, empty query, zero matches
- List output tests must verify: category grouping, configured category order, uncategorized handling, column alignment
- Command alias tests must verify: both commands work, help text distinguishes primary from alias

---

## Related

- [Quality Guidelines](./quality-guidelines.md) - General CLI requirements
- [Cross-Platform Runtime Thinking Guide](../guides/cross-platform-runtime-thinking-guide.md) - Shell execution considerations

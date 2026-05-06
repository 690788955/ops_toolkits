package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"shell_ops/internal/config"
	"shell_ops/internal/menu"
	"shell_ops/internal/packagebuild"
	"shell_ops/internal/registry"
	"shell_ops/internal/runner"
	"shell_ops/internal/scaffold"
	"shell_ops/internal/server"
)

type options struct {
	baseDir       string
	paramsFile    string
	setValues     []string
	noPrompt      bool
	configVersion string
}

func NewRootCommand() *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:   "opsctl",
		Short: "YAML 驱动的运维自动化框架",
	}
	cmd.PersistentFlags().StringVar(&opts.baseDir, "base-dir", ".", "项目根目录")
	cmd.PersistentFlags().StringVar(&opts.paramsFile, "params", "", "YAML 参数文件")
	cmd.PersistentFlags().StringArrayVar(&opts.setValues, "set", nil, "参数覆盖项 key=value")
	cmd.PersistentFlags().BoolVar(&opts.noPrompt, "no-prompt", false, "缺少必填参数时禁用交互提示")
	cmd.PersistentFlags().StringVar(&opts.configVersion, "config-version", "", "配置版本名称")
	cmd.SetHelpCommand(generatedHelpCommand(opts, "help"))

	cmd.AddCommand(listCommand(opts), validateCommand(opts), runCommand(opts), helpAutoCommand(opts), startCommand(opts), menuCommand(opts), serveCommand(opts), newCommand(opts), packageCommand(opts))
	return cmd
}

func Execute() error {
	return NewRootCommand().Execute()
}

func listCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "list", Short: "列出已配置的工具和工作流", RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := loadRegistry(opts)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "工具:")
		for id, tool := range reg.Tools {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\t%s\n", id, tool.Entry.Description)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "工作流:")
		for id, wf := range reg.Workflows {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\t%s\n", id, wf.Entry.Description)
		}
		return nil
	}}
}

func validateCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "validate", Short: "校验已配置的工具和工作流", RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := loadRegistry(opts)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "校验通过\n")
		fmt.Fprintf(cmd.OutOrStdout(), "  工具: %d\n", len(reg.Tools))
		fmt.Fprintf(cmd.OutOrStdout(), "  工作流: %d\n", len(reg.Workflows))
		fmt.Fprintf(cmd.OutOrStdout(), "  分类: %d\n", len(reg.Root.DisplayCategories()))
		return nil
	}}
}

func runCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "run", Short: "执行工具或工作流"}
	cmd.AddCommand(&cobra.Command{Use: "tool <id>", Short: "执行已配置的工具", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := loadRegistry(opts)
		if err != nil {
			return err
		}
		tool, err := reg.Tool(args[0])
		if err != nil {
			return err
		}
		params, err := resolveToolParams(opts, reg, tool)
		if err != nil {
			return err
		}
		if err := config.PromptConfirmation(tool.Config.Confirm, os.Stdin, cmd.OutOrStdout()); err != nil {
			return err
		}
		record, err := runner.New(reg).RunToolValues(context.Background(), args[0], params, cmd.OutOrStdout(), cmd.ErrOrStderr())
		if record != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "\nrun_id=%s status=%s\n", record.ID, record.Status)
		}
		return err
	}})
	cmd.AddCommand(&cobra.Command{Use: "workflow <id>", Short: "执行已配置的工作流", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := loadRegistry(opts)
		if err != nil {
			return err
		}
		wf, err := reg.Workflow(args[0])
		if err != nil {
			return err
		}
		params, err := resolveParams(opts, wf.Config.Parameters)
		if err != nil {
			return err
		}
		if err := config.PromptConfirmation(wf.Config.Confirm, os.Stdin, cmd.OutOrStdout()); err != nil {
			return err
		}
		confirmed, err := confirmWorkflowTools(reg, wf.Config, os.Stdin, cmd.OutOrStdout())
		if err != nil {
			return err
		}
		record, err := runner.New(reg).RunWorkflowWithConfirmation(context.Background(), args[0], params, confirmed, cmd.OutOrStdout(), cmd.ErrOrStderr())
		printRunSummary(cmd.OutOrStdout(), record)
		return err
	}})
	return cmd
}

func helpAutoCommand(opts *options) *cobra.Command {
	return generatedHelpCommand(opts, "help-auto")
}

func generatedHelpCommand(opts *options, use string) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: "显示由 YAML 元数据生成的帮助", RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := loadRegistry(opts)
		if err != nil {
			return err
		}
		printCatalogHelp(cmd.OutOrStdout(), reg)
		return nil
	}}
	cmd.AddCommand(&cobra.Command{Use: "tool <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := loadRegistry(opts)
		if err != nil {
			return err
		}
		tool, err := reg.Tool(args[0])
		if err != nil {
			return err
		}
		printToolHelp(cmd.OutOrStdout(), tool)
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "workflow <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := loadRegistry(opts)
		if err != nil {
			return err
		}
		wf, err := reg.Workflow(args[0])
		if err != nil {
			return err
		}
		printWorkflowHelp(cmd.OutOrStdout(), wf)
		return nil
	}})
	return cmd
}

func startCommand(opts *options) *cobra.Command {
	return interactiveCommand(opts, "start", "启动交互式运维控制台")
}

func menuCommand(opts *options) *cobra.Command {
	return interactiveCommand(opts, "menu", "打开编号菜单")
}

func interactiveCommand(opts *options, use, short string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := loadRegistry(opts)
		if err != nil {
			return err
		}
		return menu.Run(context.Background(), reg, os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr())
	}}
}

func serveCommand(opts *options) *cobra.Command {
	addr := ""
	port := ""
	cmd := &cobra.Command{Use: "serve", Short: "启动 HTTP API 和 Web UI 服务", RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := loadRegistry(opts)
		if err != nil {
			return err
		}
		listenAddr := resolveListenAddr(addr, port, reg.Root.ListenAddr())
		fmt.Fprintf(cmd.OutOrStdout(), "监听地址: %s\n", listenAddr)
		fmt.Fprintf(cmd.OutOrStdout(), "Web UI: http://127.0.0.1%s/\n", displayPort(listenAddr))
		return server.ListenAndServe(listenAddr, reg)
	}}
	cmd.Flags().StringVar(&addr, "addr", "", "监听地址，例如 0.0.0.0:8080")
	cmd.Flags().StringVar(&port, "port", "", "监听端口，例如 8080")
	return cmd
}

func resolveListenAddr(addr, port, fallback string) string {
	if port != "" {
		return ":" + port
	}
	if addr != "" {
		return addr
	}
	if fallback != "" {
		return fallback
	}
	return ":8080"
}

func displayPort(addr string) string {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return ":8080"
	}
	return addr[idx:]
}

func newCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "new", Short: "创建工具或工作流模板"}
	cmd.AddCommand(&cobra.Command{Use: "tool <category>/<tool>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return scaffold.NewTool(absBase(opts), args[0])
	}})
	cmd.AddCommand(&cobra.Command{Use: "workflow <workflow-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return scaffold.NewWorkflow(absBase(opts), args[0])
	}})
	return cmd
}

func packageCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "package", Short: "打包命令"}
	cmd.AddCommand(&cobra.Command{Use: "build", RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := loadRegistry(opts); err != nil {
			return err
		}
		dir, err := packagebuild.Build(absBase(opts))
		if err == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "交付包已生成: %s\n", dir)
		}
		return err
	}})
	return cmd
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

func printRunSummary(out io.Writer, record *runner.RunRecord) {
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

func loadRegistry(opts *options) (*registry.Registry, error) {
	return loadRegistryWithVersion(opts)
}

func absBase(opts *options) string {
	base, err := filepath.Abs(opts.baseDir)
	if err != nil {
		return opts.baseDir
	}
	return base
}

func resolveParams(opts *options, defs []config.Parameter) (map[string]string, error) {
	fileParams, err := config.LoadParamsFile(opts.paramsFile)
	if err != nil {
		return nil, err
	}
	overrides, err := config.ParseSetValues(opts.setValues)
	if err != nil {
		return nil, err
	}
	provided := map[string]string{}
	for k, v := range fileParams {
		provided[k] = v
	}
	for k, v := range overrides {
		provided[k] = v
	}
	if !opts.noPrompt {
		promptParams := config.MergeParams(defs, fileParams, overrides)
		if err := config.PromptMissing(defs, promptParams, os.Stdin, os.Stdout); err != nil {
			return nil, err
		}
		for k, v := range promptParams {
			if _, exists := provided[k]; exists {
				continue
			}
			if isParameterDefault(defs, k, v) {
				continue
			}
			provided[k] = v
		}
		return provided, nil
	}
	validationParams := config.MergeParams(defs, fileParams, overrides)
	if err := config.ValidateRequired(defs, validationParams); err != nil {
		return nil, err
	}
	return provided, nil
}

func resolveToolParams(opts *options, reg *registry.Registry, tool *registry.Tool) (map[string]interface{}, error) {
	fileParams, err := config.LoadParamsFileValues(opts.paramsFile)
	if err != nil {
		return nil, err
	}
	overrides, err := config.ParseSetValuesNested(opts.setValues)
	if err != nil {
		return nil, err
	}
	provided := config.MergeValues(fileParams, overrides)
	allValues := config.MergeValues(toolConfigLayers(reg, tool), provided)
	if !opts.noPrompt {
		before := config.FlattenValues(allValues)
		promptParams := config.FlattenValues(allValues)
		if err := config.PromptMissing(tool.Config.Parameters, promptParams, os.Stdin, os.Stdout); err != nil {
			return nil, err
		}
		for k, v := range promptParams {
			if strings.TrimSpace(v) == "" {
				continue
			}
			if existing, ok := before[k]; ok && strings.TrimSpace(existing) != "" {
				continue
			}
			if isParameterDefault(tool.Config.Parameters, k, v) {
				config.DeletePathValue(provided, k)
				continue
			}
			if err := config.SetPathValue(provided, k, v); err != nil {
				return nil, err
			}
		}
		return provided, nil
	}
	if err := config.ValidateRequiredValues(tool.Config.Parameters, allValues); err != nil {
		return nil, err
	}
	return provided, nil
}

func toolConfigLayers(reg *registry.Registry, tool *registry.Tool) config.Values {
	return config.MergeValues(
		config.MergeParameterDefaults(tool.Config.Parameters),
		reg.GlobalEnv,
		reg.Root.ConfigDefaults,
	)
}

func loadRegistryWithVersion(opts *options) (*registry.Registry, error) {
	if opts.configVersion == "" {
		return registry.Load(absBase(opts))
	}
	return registry.LoadWithVersion(absBase(opts), opts.configVersion)
}

func isParameterDefault(defs []config.Parameter, name, value string) bool {
	for _, p := range defs {
		if p.Name == name && p.Default != nil && fmt.Sprint(p.Default) == value {
			return true
		}
	}
	return false
}

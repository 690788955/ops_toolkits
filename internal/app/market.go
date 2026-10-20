package app

import (
	"fmt"

	"github.com/spf13/cobra"

	"shell_ops/internal/market"
)

func marketCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "market", Short: "生成和发布插件市场索引"}
	var (
		output string
		name   string
	)
	indexCmd := &cobra.Command{
		Use:   "index",
		Short: "生成插件市场索引和插件包",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := loadRegistry(opts)
			if err != nil {
				return err
			}
			result, err := market.BuildIndex(market.BuildOptions{
				BaseDir:   absBase(opts),
				Plugins:   reg.Root.Plugins,
				OutputDir: output,
				Name:      name,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "市场索引已生成: %s\n", result.IndexPath)
			fmt.Fprintf(cmd.OutOrStdout(), "插件包目录: %s\n", result.PackagesDir)
			fmt.Fprintf(cmd.OutOrStdout(), "插件数量: %d\n", result.Plugins)
			printWarnings(cmd.OutOrStdout(), result.Warnings)
			return nil
		},
	}
	indexCmd.Flags().StringVar(&output, "output", "market", "市场输出目录，相对路径基于项目根目录")
	indexCmd.Flags().StringVar(&name, "name", "Shell 运维插件市场", "市场名称")
	cmd.AddCommand(indexCmd)
	return cmd
}

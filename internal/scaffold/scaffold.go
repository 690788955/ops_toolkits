package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func NewTool(baseDir, spec string) error {
	parts := strings.Split(spec, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("工具规格必须是 <分类>/<工具>")
	}
	category, tool := parts[0], parts[1]
	dir := filepath.Join(baseDir, "tools", category, tool)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		return err
	}
	for _, subdir := range []string{"lib", "conf", "templates", "examples"} {
		if err := os.MkdirAll(filepath.Join(dir, subdir), 0o755); err != nil {
			return err
		}
	}
	id := category + "." + tool
	toolYAML := fmt.Sprintf(`id: %s
name: %s %s
description: TODO 描述这个工具
version: 1.0.0
category: %s
tags: [%s]

help:
  usage: opsctl run tool %s --set target=<target> --no-prompt
  examples:
    - opsctl run tool %s --set target=demo --no-prompt

parameters:
  - name: target
    type: string
    description: 目标标识
    required: true

execution:
  type: shell
  entry: bin/run.sh
  timeout: 30m
  workdir: .

pass_mode:
  env: true
  args: true
  param_file: true
  file_name: params.yaml

confirm:
  required: false
`, id, title(category), title(tool), category, category, id, id)
	if err := os.WriteFile(filepath.Join(dir, "tool.yaml"), []byte(toolYAML), 0o644); err != nil {
		return err
	}
	run := `#!/usr/bin/env bash
set -euo pipefail
echo "TODO 执行工具，target=${OPS_PARAM_TARGET:-}"
if [[ -n "${OPS_PARAM_FILE:-}" ]]; then
  echo "参数文件: ${OPS_PARAM_FILE}"
fi
`
	if err := os.WriteFile(filepath.Join(dir, "bin", "run.sh"), []byte(run), 0o755); err != nil {
		return err
	}
	readme := fmt.Sprintf("# %s %s\n\nTODO: 描述维护说明、输入、输出和回滚指引。\n", title(category), title(tool))
	return os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644)
}

func NewWorkflow(baseDir, id string) error {
	if id == "" || strings.Contains(id, "/") {
		return fmt.Errorf("工作流 ID 必须是简单名称")
	}
	dir := filepath.Join(baseDir, "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf(`id: %s
name: %s
description: TODO 描述这个工作流
version: 1.0.0
category: demo
tags: [demo]

parameters:
  - name: target
    type: string
    description: 目标标识
    required: true

nodes:
  - id: first
    name: 第一步
    tool: demo.hello
    params:
      name: "{{ .target }}"
    on_failure: stop

edges: []

confirm:
  required: false
`, id, title(strings.ReplaceAll(id, "-", " ")))
	return os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(content), 0o644)
}

func title(value string) string {
	parts := strings.Fields(strings.ReplaceAll(value, "-", " "))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

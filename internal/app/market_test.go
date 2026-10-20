package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell_ops/internal/market"
)

func TestMarketIndexCommandGeneratesMarket(t *testing.T) {
	baseDir := t.TempDir()
	writeMarketTestFile(t, filepath.Join(baseDir, "configs", "ops.yaml"), `app:
  name: Test Ops
  version: 0.1.0
paths:
  tools: []
  workflows: []
  runs: runs
  logs: runs/logs
menu:
  categories: []
plugins:
  paths:
    - plugins
  strict: true
  disabled: []
`)
	writeMarketTestFile(t, filepath.Join(baseDir, "plugins", "demo.sample", "plugin.yaml"), `id: demo.sample
name: 示例插件
version: 1.0.0
description: 用于测试市场索引
contributes:
  categories:
    - id: demo
      name: 演示
  tools:
    - id: demo.sample.hello
      name: 问候
      category: demo
      command: scripts/run.sh
      workdir: .
      tags: [示例]
      confirm:
        required: false
`)
	writeMarketTestFile(t, filepath.Join(baseDir, "plugins", "demo.sample", "scripts", "run.sh"), "#!/usr/bin/env bash\necho hello\n")

	var output bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"--base-dir", baseDir, "market", "index"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("执行 market index 失败: %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), "市场索引已生成") {
		t.Fatalf("输出缺少生成结果: %s", output.String())
	}

	data, err := os.ReadFile(filepath.Join(baseDir, "market", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index market.Index
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("解析市场索引失败: %v", err)
	}
	if len(index.Plugins) != 1 {
		t.Fatalf("插件数量 = %d, want 1", len(index.Plugins))
	}
	entry := index.Plugins[0]
	if entry.ID != "demo.sample" || entry.Package.URL != "packages/demo.sample-1.0.0.zip" || len(entry.Package.SHA256) != 64 {
		t.Fatalf("市场索引条目不符合预期: %+v", entry)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "market", "packages", "demo.sample-1.0.0.zip")); err != nil {
		t.Fatalf("插件包未生成: %v", err)
	}
}

func writeMarketTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

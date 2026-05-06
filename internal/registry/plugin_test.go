package registry

import (
	"os"
	"path/filepath"
	"testing"

	"shell_ops/internal/config"
)

func TestLoadRegistersPluginToolAndCategory(t *testing.T) {
	dir := t.TempDir()
	writeRoot(t, dir, `app:
  name: test
plugins:
  paths: [plugins]
  strict: true
menu:
  categories:
    - id: demo
      name: Demo
`)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.backup", "scripts", "backup.sh"), "#!/usr/bin/env bash\necho backup\n", 0o755)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.backup", "plugin.yaml"), `id: vendor.backup
name: Backup
version: 1.0.0
contributes:
  categories:
    - id: backup
      name: 备份
  tools:
    - id: vendor.backup.full
      name: 全量备份
      category: backup
      command: scripts/backup.sh
      args:
        - --target
        - "{{ .target }}"
      parameters:
        - name: target
          required: true
      confirm:
        required: true
        message: 确认备份？
`, 0o644)

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	tool, err := reg.Tool("vendor.backup.full")
	if err != nil {
		t.Fatalf("plugin tool not found: %v", err)
	}
	if tool.Source.Type != "plugin" || tool.Source.PluginID != "vendor.backup" {
		t.Fatalf("source = %#v, want plugin vendor.backup", tool.Source)
	}
	if tool.Config.Execution.Entry != "scripts/backup.sh" || tool.Config.Execution.Args[0] != "--target" {
		t.Fatalf("tool execution = %#v", tool.Config.Execution)
	}
	if !tool.Config.Confirm.Required {
		t.Fatalf("confirm not preserved: %#v", tool.Config.Confirm)
	}
	categories := reg.Root.DisplayCategories()
	if categories[len(categories)-1].ID != "backup" {
		t.Fatalf("categories = %#v, want backup appended", categories)
	}
}

func TestLoadRejectsDuplicatePluginToolIDInStrictMode(t *testing.T) {
	dir := t.TempDir()
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
tools:
  - path: tools/demo/hello
`)
	writeFile(t, filepath.Join(dir, "tools", "demo", "hello", "tool.yaml"), `id: vendor.backup.full
name: Builtin
category: demo
execution:
  entry: bin/run.sh
`, 0o644)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.backup", "scripts", "backup.sh"), "#!/usr/bin/env bash\necho backup\n", 0o755)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.backup", "plugin.yaml"), `id: vendor.backup
name: Backup
version: 1.0.0
contributes:
  tools:
    - id: vendor.backup.full
      category: demo
      command: scripts/backup.sh
`, 0o644)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load returned nil, want duplicate error")
	}
}

func TestLoadPluginLayeredConfigMetadata(t *testing.T) {
	dir := t.TempDir()
	writeRoot(t, dir, `config_defaults:
  es:
    host: global
plugins:
  paths: [plugins]
  strict: true
`)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.config", "scripts", "run.sh"), "#!/usr/bin/env bash\necho run\n", 0o755)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.config", "configs", "default.yaml"), `es:
  host: package
  port: "9200"
`, 0o644)
	writeFile(t, filepath.Join(dir, "configs", "plugins", "vendor.config.yaml"), `es:
  host: host
`, 0o644)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.config", "plugin.yaml"), `id: vendor.config
name: Config
version: 1.0.0
shared_config:
  es:
    host: shared
sensitive_paths: [es.password]
contributes:
  categories:
    - id: config
      name: Config
  tools:
    - id: vendor.config.run
      name: Run
      category: config
      command: scripts/run.sh
      pass_mode:
        param_file: true
      config_defaults:
        es:
          port: "9243"
      sensitive_paths: [api.token]
`, 0o644)

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	tool, err := reg.Tool("vendor.config.run")
	if err != nil {
		t.Fatalf("tool not found: %v", err)
	}
	if !tool.Config.PassMode.ParamFile {
		t.Fatalf("pass_mode 未保留: %#v", tool.Config.PassMode)
	}
	if tool.Config.PluginConfig.SharedConfig["es"].(config.Values)["host"] != "shared" {
		t.Fatalf("shared_config 未保留: %#v", tool.Config.PluginConfig.SharedConfig)
	}
	if tool.Config.PluginConfig.PackageDefaultConfig["es"].(config.Values)["host"] != "package" {
		t.Fatalf("插件默认配置未加载: %#v", tool.Config.PluginConfig.PackageDefaultConfig)
	}
	if tool.Config.PluginConfig.HostConfig["es"].(config.Values)["host"] != "host" {
		t.Fatalf("宿主覆盖配置未加载: %#v", tool.Config.PluginConfig.HostConfig)
	}
}

// 以下测试针对已废弃的配置模板机制，已注释
/*
func TestLoadRejectsUnsafeConfigTemplatePath(t *testing.T) {
	dir := t.TempDir()
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
`)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.bad", "scripts", "run.sh"), "#!/usr/bin/env bash\necho run\n", 0o755)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.bad", "plugin.yaml"), `id: vendor.bad
name: Bad
version: 1.0.0
contributes:
  tools:
    - id: vendor.bad.run
      category: bad
      command: scripts/run.sh
      config_templates:
        - name: bad
          template: ../outside.tmpl
          output: bad.conf
`, 0o644)
	if _, err := Load(dir); err == nil {
		t.Fatal("Load returned nil, want unsafe template path error")
	}
}

func TestLoadRejectsMissingConfigTemplate(t *testing.T) {
	dir := t.TempDir()
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
`)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.missingtemplate", "scripts", "run.sh"), "#!/usr/bin/env bash\necho run\n", 0o755)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.missingtemplate", "plugin.yaml"), `id: vendor.missingtemplate
name: Missing Template
version: 1.0.0
contributes:
  tools:
    - id: vendor.missingtemplate.run
      category: bad
      command: scripts/run.sh
      config_templates:
        - name: missing
          template: templates/missing.tmpl
          output: missing.conf
`, 0o644)
	if _, err := Load(dir); err == nil {
		t.Fatal("Load returned nil, want missing template error")
	}
}

func TestLoadRejectsUnsafeConfigTemplateOutput(t *testing.T) {
	dir := t.TempDir()
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
`)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.badout", "scripts", "run.sh"), "#!/usr/bin/env bash\necho run\n", 0o755)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.badout", "templates", "native.tmpl"), "host={{ .host }}\n", 0o644)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.badout", "plugin.yaml"), `id: vendor.badout
name: Bad Output
version: 1.0.0
contributes:
  tools:
    - id: vendor.badout.run
      category: bad
      command: scripts/run.sh
      config_templates:
        - name: bad
          template: templates/native.tmpl
          output: ../escape.conf
`, 0o644)
	if _, err := Load(dir); err == nil {
		t.Fatal("Load returned nil, want unsafe template output error")
	}
}
*/

func TestLoadAppliesPluginConfigMappingOverride(t *testing.T) {
	dir := t.TempDir()
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
`)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.mapping", "scripts", "run.sh"), "#!/usr/bin/env bash\necho run\n", 0o755)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.mapping", "plugin.yaml"), `id: vendor.mapping
name: Mapping
version: 1.0.0
contributes:
  tools:
    - id: vendor.mapping.run
      category: mapping
      command: scripts/run.sh
      config_files:
        - default.conf
    - id: vendor.mapping.other
      category: mapping
      command: scripts/run.sh
      config_files:
        - other.conf
`, 0o644)
	writeFile(t, filepath.Join(dir, "configs", "plugins", "vendor.mapping.mapping.yaml"), `tools:
  vendor.mapping.run:
    config_files:
      - host.conf
`, 0o644)

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	runTool, err := reg.Tool("vendor.mapping.run")
	if err != nil {
		t.Fatal(err)
	}
	if got := runTool.Config.ConfigFiles; len(got) != 1 || got[0] != "host.conf" {
		t.Fatalf("mapping 未整体覆盖默认 config_files: %#v", got)
	}
	otherTool, err := reg.Tool("vendor.mapping.other")
	if err != nil {
		t.Fatal(err)
	}
	if got := otherTool.Config.ConfigFiles; len(got) != 1 || got[0] != "other.conf" {
		t.Fatalf("未配置工具不应受 mapping 影响: %#v", got)
	}
}

func TestLoadRejectsPluginConfigMappingUnknownToolAndUnsafeFields(t *testing.T) {
	cases := []struct {
		name    string
		mapping string
	}{
		{"unknown-tool", `tools:
  vendor.mapping.missing:
    config_files: []
`},
		{"empty-name", `tools:
  vendor.mapping.run:
    config_files:
      - ""
`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
`)
			writeFile(t, filepath.Join(dir, "plugins", "vendor.mapping", "scripts", "run.sh"), "#!/usr/bin/env bash\necho run\n", 0o755)
			writeFile(t, filepath.Join(dir, "plugins", "vendor.mapping", "plugin.yaml"), `id: vendor.mapping
name: Mapping
version: 1.0.0
contributes:
  tools:
    - id: vendor.mapping.run
      category: mapping
      command: scripts/run.sh
`, 0o644)
			writeFile(t, filepath.Join(dir, "configs", "plugins", "vendor.mapping.mapping.yaml"), tc.mapping, 0o644)
			if _, err := Load(dir); err == nil {
				t.Fatal("Load returned nil, want mapping validation error")
			}
		})
	}
}

func writeRoot(t *testing.T, dir, content string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "configs", "ops.yaml"), content, 0o644)
}

func writeFile(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
}

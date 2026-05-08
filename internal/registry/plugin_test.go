package registry

import (
	"os"
	"path/filepath"
	"strings"
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

func TestOrderedToolsKeepsBuiltinsThenPluginDeclarationOrder(t *testing.T) {
	dir := t.TempDir()
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
tools:
  - path: tools/builtin/zeta
  - path: tools/builtin/alpha
`)
	writeFile(t, filepath.Join(dir, "tools", "builtin", "zeta", "tool.yaml"), `id: builtin.zeta
name: Builtin Zeta
category: builtin
execution:
  entry: run.sh
`, 0o644)
	writeFile(t, filepath.Join(dir, "tools", "builtin", "alpha", "tool.yaml"), `id: builtin.alpha
name: Builtin Alpha
category: builtin
execution:
  entry: run.sh
`, 0o644)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.first", "scripts", "run.sh"), "#!/usr/bin/env bash\necho first\n", 0o755)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.first", "plugin.yaml"), `id: vendor.first
name: First
version: 1.0.0
contributes:
  tools:
    - id: vendor.first.zeta
      category: ordered
      command: scripts/run.sh
    - id: vendor.first.alpha
      category: ordered
      command: scripts/run.sh
`, 0o644)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.second", "scripts", "run.sh"), "#!/usr/bin/env bash\necho second\n", 0o755)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.second", "plugin.yaml"), `id: vendor.second
name: Second
version: 1.0.0
contributes:
  tools:
    - id: vendor.second.beta
      category: ordered
      command: scripts/run.sh
    - id: vendor.second.gamma
      category: ordered
      command: scripts/run.sh
`, 0o644)

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	got := []string{}
	for _, tool := range reg.OrderedTools() {
		got = append(got, tool.Entry.ID)
	}
	want := []string{"builtin.zeta", "builtin.alpha", "vendor.first.zeta", "vendor.first.alpha", "vendor.second.beta", "vendor.second.gamma"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("OrderedTools = %v, want %v", got, want)
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

func TestLoadNormalizesExternalPluginConfigDir(t *testing.T) {
	dir := t.TempDir()
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
`)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.shared", "scripts", "run.sh"), "#!/usr/bin/env bash\necho run\n", 0o755)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.shared", "plugin.yaml"), `id: vendor.shared
name: Shared
version: 1.0.0
contributes:
  tools:
    - id: vendor.shared.run
      category: shared
      command: scripts/run.sh
      config_dir: ../../shared-config
      config_files:
        - app.conf
`, 0o644)

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	tool, err := reg.Tool("vendor.shared.run")
	if err != nil {
		t.Fatal(err)
	}
	if tool.Config.ConfigDir != "../../shared-config" {
		t.Fatalf("ConfigDir = %q, want ../../shared-config", tool.Config.ConfigDir)
	}
	if len(tool.Config.ConfigFileRefs) != 1 || tool.Config.ConfigFileRefs[0].ConfigDir != "../../shared-config" || tool.Config.ConfigFileRefs[0].Path != "app.conf" {
		t.Fatalf("ConfigFileRefs = %#v", tool.Config.ConfigFileRefs)
	}
}

func TestLoadNormalizesAbsolutePluginConfigDir(t *testing.T) {
	dir := t.TempDir()
	absConfigDir := filepath.Join(dir, "absolute-config")
	writeFile(t, filepath.Join(absConfigDir, "app.conf"), "old=true\n", 0o644)
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
`)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.absolute", "scripts", "run.sh"), "#!/usr/bin/env bash\necho run\n", 0o755)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.absolute", "plugin.yaml"), `id: vendor.absolute
name: Absolute
version: 1.0.0
contributes:
  tools:
    - id: vendor.absolute.run
      category: absolute
      command: scripts/run.sh
      config_dir: `+absConfigDir+`
      config_files:
        - app.conf
`, 0o644)

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	tool, err := reg.Tool("vendor.absolute.run")
	if err != nil {
		t.Fatal(err)
	}
	if tool.Config.ConfigDir != absConfigDir {
		t.Fatalf("ConfigDir = %q, want %q", tool.Config.ConfigDir, absConfigDir)
	}
	if len(tool.Config.ConfigFileRefs) != 1 || tool.Config.ConfigFileRefs[0].ConfigDir != absConfigDir || tool.Config.ConfigFileRefs[0].Path != "app.conf" {
		t.Fatalf("ConfigFileRefs = %#v", tool.Config.ConfigFileRefs)
	}
}

func TestLoadAcceptsConfiguredBarePluginCommand(t *testing.T) {
	dir := t.TempDir()
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
  allowed_commands:
    - java
    - ansible-playbook
`)
	writeFile(t, filepath.Join(dir, "plugins", "vendor.pathcmd", "plugin.yaml"), `id: vendor.pathcmd
name: Path Command
version: 1.0.0
contributes:
  tools:
    - id: vendor.pathcmd.run
      category: pathcmd
      command: ansible-playbook
`, 0o644)

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	tool, err := reg.Tool("vendor.pathcmd.run")
	if err != nil {
		t.Fatal(err)
	}
	if tool.Config.Execution.Entry != "ansible-playbook" {
		t.Fatalf("entry = %q, want ansible-playbook", tool.Config.Execution.Entry)
	}
}

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

func TestLoadAcceptsPluginScopeMappingWithAbsoluteConfigDir(t *testing.T) {
	dir := t.TempDir()
	absConfigDir := filepath.Join(dir, "absolute-config")
	writeFile(t, filepath.Join(absConfigDir, "mapped.conf"), "old=true\n", 0o644)
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
	writeFile(t, filepath.Join(dir, "configs", "plugins", "vendor.mapping.mapping.yaml"), `tools:
  vendor.mapping.run:
    config_files:
      - id: mapped
        scope: plugin
        config_dir: `+absConfigDir+`
        path: mapped.conf
        access: read_write
        create: true
`, 0o644)

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	runTool, err := reg.Tool("vendor.mapping.run")
	if err != nil {
		t.Fatal(err)
	}
	if len(runTool.Config.ConfigFileRefs) != 1 || runTool.Config.ConfigFileRefs[0].ConfigDir != absConfigDir || runTool.Config.ConfigFileRefs[0].Path != "mapped.conf" {
		t.Fatalf("mapping config refs = %#v", runTool.Config.ConfigFileRefs)
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

func TestLoadAcceptsHostAbsoluteConfigMappingInsideWhitelist(t *testing.T) {
	dir := t.TempDir()
	hostDir := filepath.Join(dir, "host-config")
	hostFile := filepath.Join(hostDir, "app.conf")
	writeFile(t, hostFile, "old=true\n", 0o644)
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
host_config_files:
  allowed_dirs:
    - `+hostDir+`
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
	writeFile(t, filepath.Join(dir, "configs", "plugins", "vendor.mapping.mapping.yaml"), `tools:
  vendor.mapping.run:
    config_files:
      - id: app-main
        label: App 主配置
        scope: host_absolute
        config_dir: `+hostDir+`
        path: app.conf
        access: read_write
        create: false
`, 0o644)

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	tool, err := reg.Tool("vendor.mapping.run")
	if err != nil {
		t.Fatal(err)
	}
	if len(tool.Config.ConfigFileRefs) != 1 || tool.Config.ConfigFileRefs[0].ID != "app-main" || tool.Config.ConfigFileRefs[0].Scope != config.ConfigFileScopeHostAbsolute {
		t.Fatalf("host config entry not normalized: %#v", tool.Config.ConfigFileRefs)
	}
}

func TestLoadRejectsHostAbsoluteConfigMappingOutsideWhitelist(t *testing.T) {
	dir := t.TempDir()
	allowedDir := filepath.Join(dir, "allowed")
	outsideDir := filepath.Join(dir, "outside")
	writeFile(t, filepath.Join(allowedDir, "placeholder"), "x", 0o644)
	outsideFile := filepath.Join(outsideDir, "app.conf")
	writeFile(t, outsideFile, "old=true\n", 0o644)
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
host_config_files:
  allowed_dirs:
    - `+allowedDir+`
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
	writeFile(t, filepath.Join(dir, "configs", "plugins", "vendor.mapping.mapping.yaml"), `tools:
  vendor.mapping.run:
    config_files:
      - id: app-main
        scope: host_absolute
        config_dir: `+outsideDir+`
        path: app.conf
        access: read
`, 0o644)

	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "白名单") {
		t.Fatalf("Load error = %v, want whitelist error", err)
	}
}

func TestLoadRejectsHostConfigWhitelistFile(t *testing.T) {
	dir := t.TempDir()
	allowedFile := filepath.Join(dir, "allowed.conf")
	writeFile(t, allowedFile, "x", 0o644)
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
host_config_files:
  allowed_dirs:
    - `+allowedFile+`
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
	writeFile(t, filepath.Join(dir, "configs", "plugins", "vendor.mapping.mapping.yaml"), `tools:
  vendor.mapping.run:
    config_files:
      - id: app-main
        scope: host_absolute
        path: `+allowedFile+`
        access: read
`, 0o644)

	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "目录白名单") {
		t.Fatalf("Load error = %v, want directory whitelist error", err)
	}
}

func TestLoadRejectsHostAbsoluteConfigMappingSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	allowedDir := filepath.Join(dir, "allowed")
	outsideDir := filepath.Join(dir, "outside")
	if err := os.MkdirAll(allowedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(allowedDir, "link")
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Skipf("当前环境不支持创建目录符号链接: %v", err)
	}
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
host_config_files:
  allowed_dirs:
    - `+allowedDir+`
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
	writeFile(t, filepath.Join(dir, "configs", "plugins", "vendor.mapping.mapping.yaml"), `tools:
  vendor.mapping.run:
    config_files:
      - id: app-main
        scope: host_absolute
        config_dir: `+linkDir+`
        path: app.conf
        access: read_write
        create: true
`, 0o644)

	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "符号链接") || !strings.Contains(err.Error(), "白名单") {
		t.Fatalf("Load error = %v, want symlink whitelist escape error", err)
	}
}

func TestLoadRejectsHostAbsoluteConfigMappingFileSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	allowedDir := filepath.Join(dir, "allowed")
	outsideDir := filepath.Join(dir, "outside")
	if err := os.MkdirAll(allowedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outsideDir, "secret.conf")
	writeFile(t, outsideFile, "secret=true\n", 0o644)
	linkFile := filepath.Join(allowedDir, "linked.conf")
	if err := os.Symlink(outsideFile, linkFile); err != nil {
		t.Skipf("当前环境不支持创建文件符号链接: %v", err)
	}
	writeRoot(t, dir, `plugins:
  paths: [plugins]
  strict: true
host_config_files:
  allowed_dirs:
    - `+allowedDir+`
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
	writeFile(t, filepath.Join(dir, "configs", "plugins", "vendor.mapping.mapping.yaml"), `tools:
  vendor.mapping.run:
    config_files:
      - id: app-main
        scope: host_absolute
        config_dir: `+allowedDir+`
        path: linked.conf
        access: read
`, 0o644)

	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "符号链接") || !strings.Contains(err.Error(), "白名单") {
		t.Fatalf("Load error = %v, want file symlink whitelist escape error", err)
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

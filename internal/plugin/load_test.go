package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell_ops/internal/config"
)

func TestLoadSkipsBadPluginWhenNonStrict(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad", `id: bad.plugin
name: Bad
version: 1.0.0
contributes:
  tools:
    - id: bad.plugin.tool
      category: bad
      command: missing.sh
`)

	result, err := Load(dir, config.PluginsConfig{Paths: []string{"plugins"}})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(result.Packages) != 0 || len(result.Warnings) != 1 {
		t.Fatalf("result = %#v, want one warning and no packages", result)
	}
}

func TestLoadFailsBadPluginWhenStrict(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "bad", `id: bad.plugin
name: Bad
version: 1.0.0
contributes:
  tools:
    - id: bad.plugin.tool
      category: bad
      command: missing.sh
`)

	_, err := Load(dir, config.PluginsConfig{Paths: []string{"plugins"}, Strict: true})
	if err == nil || !strings.Contains(err.Error(), "command 不存在") {
		t.Fatalf("Load error = %v, want command 不存在", err)
	}
}

func TestPackageWarningsReportsIntegrationQuality(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins", "warn")
	if err := os.MkdirAll(filepath.Join(pluginDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "scripts", "run.sh"), []byte("#!/usr/bin/env bash\necho warn\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := writePlugin(t, dir, "warn", `id: vendor.warn
name: Warn
version: 1.0.0
contributes:
  tools:
    - id: vendor.warn.tool
      name: Warning Tool
      category: warn
      command: scripts/run.sh
      config_files:
        - config/missing.conf
      parameters:
        - name: target
          type: string
          required: false
      confirm:
        required: true
        message: 确认
`)

	if err := ValidatePackage(pkg); err != nil {
		t.Fatalf("ValidatePackage returned error: %v", err)
	}
	warnings := PackageWarnings(pkg)
	codes := map[string]bool{}
	for _, warning := range warnings {
		codes[warning.Code] = true
		if warning.PluginID != "vendor.warn" {
			t.Fatalf("warning plugin_id = %q, want vendor.warn", warning.PluginID)
		}
	}
	for _, code := range []string{"PLUGIN_README_MISSING", "PLUGIN_DESCRIPTION_MISSING", "TOOL_DESCRIPTION_MISSING", "PARAM_DESCRIPTION_MISSING", "CONFIRM_MESSAGE_TOO_SHORT", "CONFIG_FILE_MISSING"} {
		if !codes[code] {
			t.Fatalf("warnings missing code %s: %#v", code, warnings)
		}
	}
}

func TestValidatePackageRejectsPathEscape(t *testing.T) {
	dir := t.TempDir()
	pkg := writePlugin(t, dir, "escape", `id: vendor.escape
name: Escape
version: 1.0.0
contributes:
  tools:
    - id: vendor.escape.tool
      category: demo
      command: ../outside.sh
`)

	err := ValidatePackage(pkg)
	if err == nil || !strings.Contains(err.Error(), "路径逃逸") {
		t.Fatalf("ValidatePackage error = %v, want 路径逃逸", err)
	}
}

func TestValidatePackageAllowsConfigDirOutsidePlugin(t *testing.T) {
	dir := t.TempDir()
	pkg := writePlugin(t, dir, "shared", `id: vendor.shared
name: Shared
version: 1.0.0
contributes:
  tools:
    - id: vendor.shared.tool
      category: demo
      command: scripts/run.sh
      config_dir: ../../shared-config
      config_files:
        - app.conf
`)
	if err := os.MkdirAll(filepath.Join(dir, "plugins", "shared", "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugins", "shared", "scripts", "run.sh"), []byte("#!/usr/bin/env bash\necho run\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ValidatePackage(pkg); err != nil {
		t.Fatalf("ValidatePackage returned error: %v", err)
	}
}

func TestValidatePackageAllowsAbsoluteConfigDir(t *testing.T) {
	dir := t.TempDir()
	absConfigDir := filepath.Join(dir, "absolute-config")
	pkg := writePlugin(t, dir, "absolute", `id: vendor.absolute
name: Absolute
version: 1.0.0
contributes:
  tools:
    - id: vendor.absolute.tool
      category: demo
      command: scripts/run.sh
      config_dir: `+absConfigDir+`
      config_files:
        - app.conf
`)
	if err := os.MkdirAll(filepath.Join(dir, "plugins", "absolute", "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugins", "absolute", "scripts", "run.sh"), []byte("#!/usr/bin/env bash\necho run\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ValidatePackage(pkg); err != nil {
		t.Fatalf("ValidatePackage returned error: %v", err)
	}
}

func TestLoadAllowsConfiguredBarePathCommand(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "pathcmd", `id: vendor.pathcmd
name: Path Command
version: 1.0.0
contributes:
  tools:
    - id: vendor.pathcmd.tool
      category: demo
      command: ansible-playbook
`)

	result, err := Load(dir, config.PluginsConfig{Paths: []string{"plugins"}, Strict: true, AllowedCommands: []string{"ansible-playbook"}})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(result.Packages) != 1 {
		t.Fatalf("packages = %d, want 1", len(result.Packages))
	}
}

func TestLoadRejectsUnconfiguredBarePathCommand(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "pathcmd-bad", `id: vendor.pathcmdbad
name: Path Command Bad
version: 1.0.0
contributes:
  tools:
    - id: vendor.pathcmdbad.tool
      category: demo
      command: ansible-playbook
`)

	_, err := Load(dir, config.PluginsConfig{Paths: []string{"plugins"}, Strict: true})
	if err == nil || !strings.Contains(err.Error(), "allowed_commands") {
		t.Fatalf("Load error = %v, want allowed_commands", err)
	}
}

func TestValidatePackageRejectsConfigFileEscapeFromExternalConfigDir(t *testing.T) {
	dir := t.TempDir()
	pkg := writePlugin(t, dir, "shared-escape", `id: vendor.sharedescape
name: Shared Escape
version: 1.0.0
contributes:
  tools:
    - id: vendor.sharedescape.tool
      category: demo
      command: scripts/run.sh
      config_dir: ../../shared-config
      config_files:
        - ../secret
`)
	if err := os.MkdirAll(filepath.Join(dir, "plugins", "shared-escape", "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugins", "shared-escape", "scripts", "run.sh"), []byte("#!/usr/bin/env bash\necho run\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := ValidatePackage(pkg)
	if err == nil || !strings.Contains(err.Error(), "config_files 路径不安全") {
		t.Fatalf("ValidatePackage error = %v, want config_files 路径不安全", err)
	}
}

func TestValidatePackageRejectsConfigFileEscapeFromAbsoluteConfigDir(t *testing.T) {
	dir := t.TempDir()
	absConfigDir := filepath.Join(dir, "absolute-config")
	pkg := writePlugin(t, dir, "absolute-escape", `id: vendor.absoluteescape
name: Absolute Escape
version: 1.0.0
contributes:
  tools:
    - id: vendor.absoluteescape.tool
      category: demo
      command: scripts/run.sh
      config_dir: `+absConfigDir+`
      config_files:
        - ../secret
`)
	if err := os.MkdirAll(filepath.Join(dir, "plugins", "absolute-escape", "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugins", "absolute-escape", "scripts", "run.sh"), []byte("#!/usr/bin/env bash\necho run\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := ValidatePackage(pkg)
	if err == nil || !strings.Contains(err.Error(), "config_files 路径不安全") {
		t.Fatalf("ValidatePackage error = %v, want config_files 路径不安全", err)
	}
}

func TestValidatePackageRejectsBackslashConfigFileEscape(t *testing.T) {
	dir := t.TempDir()
	pkg := writePlugin(t, dir, "backslash-escape", `id: vendor.backslashescape
name: Backslash Escape
version: 1.0.0
contributes:
  tools:
    - id: vendor.backslashescape.tool
      category: demo
      command: scripts/run.sh
      config_dir: config
      config_files:
        - ..\\secret
`)
	if err := os.MkdirAll(filepath.Join(dir, "plugins", "backslash-escape", "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugins", "backslash-escape", "scripts", "run.sh"), []byte("#!/usr/bin/env bash\necho run\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := ValidatePackage(pkg)
	if err == nil || !strings.Contains(err.Error(), "config_files 路径不安全") {
		t.Fatalf("ValidatePackage error = %v, want config_files 路径不安全", err)
	}
}

func TestLoadSkipsDisabledPluginByDirectoryName(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins", "bad-dir")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(`id: vendor.bad
name: Bad
version: 1.0.0
contributes:
  tools:
    - id: vendor.bad.tool
      category: bad
      command: missing.sh
`), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Load(dir, config.PluginsConfig{Paths: []string{"plugins"}, Disabled: []string{"bad-dir"}})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(result.Packages) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("result = %#v, want disabled directory skipped silently", result)
	}
}

func TestLoadSkipsDisabledPlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "disabled", `id: vendor.disabled
name: Disabled
version: 1.0.0
`)

	result, err := Load(dir, config.PluginsConfig{Paths: []string{"plugins"}, Disabled: []string{"vendor.disabled"}})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(result.Packages) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("result = %#v, want disabled plugin skipped silently", result)
	}
}

func writePlugin(t *testing.T, baseDir, name, manifest string) Package {
	t.Helper()
	dir := filepath.Join(baseDir, "plugins", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plugin.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg, err := loadPackage(dir)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

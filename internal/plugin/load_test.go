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

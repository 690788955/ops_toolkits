package registry

import (
	"os"
	"path/filepath"
	"testing"
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

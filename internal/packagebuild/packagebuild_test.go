package packagebuild

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildCopiesPackageContents(t *testing.T) {
	baseDir := t.TempDir()
	writeFile(t, filepath.Join(baseDir, "configs", "ops.yaml"), "app:\n  name: 测试运维\n")
	writeFile(t, filepath.Join(baseDir, "plugins", "vendor.demo", "plugin.yaml"), "id: vendor.demo\n")

	outDir, err := Build(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	exePath := filepath.Join("bin", filepath.Base(os.Args[0]))
	for _, path := range []string{
		filepath.Join("configs", "ops.yaml"),
		filepath.Join("plugins", "vendor.demo", "plugin.yaml"),
		exePath,
	} {
		if _, err := os.Stat(filepath.Join(outDir, path)); err != nil {
			t.Fatalf("交付包文件 %s 缺失: %v", path, err)
		}
	}

	zipPath := outDir + ".zip"
	if _, err := os.Stat(zipPath); err != nil {
		t.Fatalf("交付包 zip 缺失: %v", err)
	}
	assertZipEntries(t, zipPath, []string{
		"configs/ops.yaml",
		"plugins/vendor.demo/plugin.yaml",
		filepath.ToSlash(exePath),
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertZipEntries(t *testing.T, zipPath string, paths []string) {
	t.Helper()
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	entries := map[string]bool{}
	for _, file := range reader.File {
		entries[file.Name] = true
	}
	packageDir := filepath.Base(zipPath[:len(zipPath)-len(filepath.Ext(zipPath))])
	for _, path := range paths {
		entry := packageDir + "/" + path
		if !entries[entry] {
			t.Fatalf("zip 条目 %s 缺失", entry)
		}
	}
}

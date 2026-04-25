package packagebuild

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildCopiesPackageContents(t *testing.T) {
	baseDir := t.TempDir()
	writeFile(t, filepath.Join(baseDir, "ops.yaml"), "app:\n  name: 测试运维\n")
	writeFile(t, filepath.Join(baseDir, "configs", "ops.yaml"), "app:\n  name: 测试运维\n")
	writeFile(t, filepath.Join(baseDir, "tools", "demo", "hello", "tool.yaml"), "id: demo.hello\n")
	writeFile(t, filepath.Join(baseDir, "tools", "demo", "hello", "bin", "run.sh"), "#!/usr/bin/env bash\n")
	writeFile(t, filepath.Join(baseDir, "workflows", "demo-hello.yaml"), "id: demo.hello\n")

	outDir, err := Build(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"ops.yaml",
		filepath.Join("configs", "ops.yaml"),
		filepath.Join("tools", "demo", "hello", "tool.yaml"),
		filepath.Join("tools", "demo", "hello", "bin", "run.sh"),
		filepath.Join("workflows", "demo-hello.yaml"),
		"opsctl",
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
		"ops.yaml",
		"configs/ops.yaml",
		"tools/demo/hello/tool.yaml",
		"tools/demo/hello/bin/run.sh",
		"workflows/demo-hello.yaml",
		"opsctl",
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

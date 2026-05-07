package packagebuild

import (
	"archive/tar"
	"compress/gzip"
	"io"
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

	tarPath := outDir + ".tar.gz"
	if _, err := os.Stat(tarPath); err != nil {
		t.Fatalf("交付包 tar.gz 缺失: %v", err)
	}
	assertTarGzEntries(t, tarPath, []string{
		"opsctl/configs/ops.yaml",
		"opsctl/plugins/vendor.demo/plugin.yaml",
		"opsctl/" + filepath.ToSlash(exePath),
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

func assertTarGzEntries(t *testing.T, tarPath string, paths []string) {
	t.Helper()
	f, err := os.Open(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	entries := map[string]bool{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries[header.Name] = true
		if header.Typeflag == tar.TypeDir && header.Name == "opsctl/plugins/vendor.demo" && header.Mode != 0o755 {
			t.Fatalf("插件目录权限 = %#o, want 0755", header.Mode)
		}
	}
	for _, path := range paths {
		if !entries[path] {
			t.Fatalf("tar.gz 条目 %s 缺失", path)
		}
	}
}


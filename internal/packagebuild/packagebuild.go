package packagebuild

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Build(baseDir string) (string, error) {
	distRoot := filepath.Join(baseDir, "dist")
	name := "opsctl"
	outDir := filepath.Join(distRoot, name)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	for _, item := range []string{"configs", "plugins"} {
		src := filepath.Join(baseDir, item)
		if _, err := os.Stat(src); err == nil {
			if err := copyPath(src, filepath.Join(outDir, item)); err != nil {
				return "", err
			}
		}
	}
	if exe, err := os.Executable(); err == nil {
		_ = copyFile(exe, filepath.Join(outDir, "bin", filepath.Base(exe)))
	}
	if err := writeRuntimeHelpers(outDir); err != nil {
		return "", err
	}
	if err := tarGzDir(outDir, outDir+".tar.gz"); err != nil {
		return "", err
	}
	return outDir, nil
}

func writeRuntimeHelpers(outDir string) error {
	files := map[string]string{
		"start-web.bat":         "@echo off\r\ncd /d \"%~dp0\"\r\n\"bin\\opsctl.exe\" serve\r\npause\r\n",
		"validate.bat":          "@echo off\r\ncd /d \"%~dp0\"\r\n\"bin\\opsctl.exe\" validate\r\npause\r\n",
		"doctor.bat":            "@echo off\r\ncd /d \"%~dp0\"\r\n\"bin\\opsctl.exe\" doctor\r\npause\r\n",
		"export-latest-run.bat": "@echo off\r\ncd /d \"%~dp0\"\r\nfor /f \"skip=1 tokens=1\" %%i in ('\"bin\\opsctl.exe\" runs list') do set LAST_RUN=%%i& goto export\r\n:export\r\nif \"%LAST_RUN%\"==\"\" echo 未找到运行记录。& pause& exit /b 1\r\n\"bin\\opsctl.exe\" runs export %LAST_RUN%\r\npause\r\n",
		"docs/使用说明.md":          runtimeUsageDoc(),
		"docs/故障反馈说明.md":        runtimeSupportDoc(),
	}
	for name, content := range files {
		path := filepath.Join(outDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("写入交付辅助文件失败 %s: %w", name, err)
		}
	}
	return nil
}

func runtimeUsageDoc() string {
	return "# 使用说明\r\n\r\n1. 解压交付包并进入目录。\r\n2. 双击 `start-web.bat` 启动本地控制台。\r\n3. 使用命令行窗口中打印的 `http://127.0.0.1:端口/?token=...` 地址访问。\r\n4. 在页面选择工具，确认参数后执行。\r\n5. 执行失败时点击页面中的“导出支持包”，或运行 `export-latest-run.bat`。\r\n\r\n默认服务只监听本机 `127.0.0.1`，不对局域网开放。\r\n"
}

func runtimeSupportDoc() string {
	return "# 故障反馈说明\r\n\r\n请优先提供以下文件：\r\n\r\n- 页面“导出支持包”生成的 ZIP。\r\n- 或 `runs/exports/` 下由 `export-latest-run.bat` 生成的 ZIP。\r\n- `runs/doctor-report.json`，可通过双击 `doctor.bat` 生成。\r\n\r\n支持包包含运行记录、stdout/stderr 日志和脱敏摘要，不需要手动复制 `runs/logs` 目录。\r\n"
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dst)
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	info, err := in.Stat()
	if err == nil {
		return os.Chmod(dst, info.Mode())
	}
	return nil
}

func tarGzDir(srcDir, tarPath string) error {
	out, err := os.Create(tarPath)
	if err != nil {
		return err
	}
	defer out.Close()
	gw := gzip.NewWriter(out)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Dir(srcDir), path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if header.Name == "opsctl/plugins" || strings.HasPrefix(header.Name, "opsctl/plugins/") {
			header.Mode = 0o755
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(tw, in)
		return err
	})
}

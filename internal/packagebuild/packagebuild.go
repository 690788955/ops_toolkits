package packagebuild

import (
	"archive/tar"
	"compress/gzip"
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
	if err := tarGzDir(outDir, outDir+".tar.gz"); err != nil {
		return "", err
	}
	return outDir, nil
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
		if strings.HasPrefix(header.Name, "opsctl/plugins/") {
			if d.IsDir() || strings.HasSuffix(header.Name, ".sh") {
				header.Mode = 0o755
			}
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

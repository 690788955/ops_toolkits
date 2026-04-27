package server

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"shell_ops/internal/plugin"
	"shell_ops/internal/registry"
)

const (
	maxPluginUploadSize       = 20 << 20
	maxPluginUploadFiles      = 200
	maxPluginUncompressedSize = 50 << 20
)

type pluginUploadResult struct {
	PluginID        string `json:"plugin_id"`
	Version         string `json:"version"`
	Status          string `json:"status"`
	Existing        bool   `json:"existing"`
	ExistingVersion string `json:"existing_version,omitempty"`
}

func pluginUploadHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		replace := req.URL.Query().Get("replace") == "true" || req.URL.Query().Get("replace") == "1"
		data, err := readPluginUpload(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		result, err := installUploadedPlugin(state, data, replace)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errPluginDuplicate) {
				status = http.StatusConflict
			}
			writeJSON(w, status, response{Error: err.Error(), Data: result})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: result.Status, Data: result})
	}
}

var errPluginDuplicate = errors.New("插件已存在")

func readPluginUpload(req *http.Request) ([]byte, error) {
	req.Body = http.MaxBytesReader(nil, req.Body, maxPluginUploadSize)
	contentType := req.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := req.ParseMultipartForm(maxPluginUploadSize); err != nil {
			return nil, fmt.Errorf("读取上传文件失败: %w", err)
		}
		file, header, err := multipartFile(req.MultipartForm)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		if header.Size > maxPluginUploadSize {
			return nil, fmt.Errorf("上传文件过大")
		}
		return io.ReadAll(io.LimitReader(file, maxPluginUploadSize+1))
	}
	if contentType != "" && !strings.HasPrefix(contentType, "application/zip") && !strings.HasPrefix(contentType, "application/octet-stream") {
		return nil, fmt.Errorf("仅支持 ZIP 插件包")
	}
	data, err := io.ReadAll(io.LimitReader(req.Body, maxPluginUploadSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxPluginUploadSize {
		return nil, fmt.Errorf("上传文件过大")
	}
	return data, nil
}

func multipartFile(form *multipart.Form) (multipart.File, *multipart.FileHeader, error) {
	if form == nil || form.File == nil {
		return nil, nil, fmt.Errorf("缺少上传文件")
	}
	keys := []string{"file", "plugin", "zip"}
	for _, key := range keys {
		files := form.File[key]
		if len(files) == 0 {
			continue
		}
		file, err := files[0].Open()
		if err != nil {
			return nil, nil, err
		}
		return file, files[0], nil
	}
	for _, files := range form.File {
		if len(files) == 0 {
			continue
		}
		file, err := files[0].Open()
		if err != nil {
			return nil, nil, err
		}
		return file, files[0], nil
	}
	return nil, nil, fmt.Errorf("缺少上传文件")
}

func installUploadedPlugin(state *serverState, data []byte, replace bool) (pluginUploadResult, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	reg := state.reg
	staging, err := os.MkdirTemp("", "ops-plugin-upload-*")
	if err != nil {
		return pluginUploadResult{}, err
	}
	defer os.RemoveAll(staging)
	if err := extractPluginZip(data, staging); err != nil {
		return pluginUploadResult{}, err
	}
	pkgDir, err := findUploadedPluginRoot(staging)
	if err != nil {
		return pluginUploadResult{}, err
	}
	pkg, err := plugin.LoadPackage(pkgDir)
	if err == nil {
		err = plugin.ValidatePackage(pkg)
	}
	if err != nil {
		return pluginUploadResult{}, err
	}
	result := pluginUploadResult{PluginID: pkg.Manifest.ID, Version: pkg.Manifest.Version}
	pluginsRoot := firstPluginRoot(reg)
	installDir := filepath.Join(reg.BaseDir, filepath.FromSlash(pluginsRoot), pkg.Manifest.ID)
	if existing, ok := existingPlugin(reg, pkg.Manifest.ID); ok {
		result.Existing = true
		result.ExistingVersion = existing.Manifest.Version
		if !replace {
			result.Status = "duplicate"
			return result, fmt.Errorf("%w，是否更新？", errPluginDuplicate)
		}
		if compareVersions(pkg.Manifest.Version, existing.Manifest.Version) <= 0 {
			return result, fmt.Errorf("插件 %s 已安装版本为 %s，上传版本 %s 不高于已安装版本", pkg.Manifest.ID, existing.Manifest.Version, pkg.Manifest.Version)
		}
		if err := replacePlugin(reg, state, pkgDir, installDir); err != nil {
			return result, err
		}
		result.Status = "updated"
		return result, nil
	}
	if replace {
		return result, fmt.Errorf("插件 %s 不存在，无法更新", pkg.Manifest.ID)
	}
	if err := copyDir(pkgDir, installDir); err != nil {
		return result, err
	}
	newReg, err := registry.Load(reg.BaseDir)
	if err != nil {
		_ = os.RemoveAll(installDir)
		return result, err
	}
	state.reg = newReg
	result.Status = "installed"
	return result, nil
}

func extractPluginZip(data []byte, dest string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("无效 ZIP 文件: %w", err)
	}
	if len(zr.File) == 0 {
		return fmt.Errorf("ZIP 文件为空")
	}
	if len(zr.File) > maxPluginUploadFiles {
		return fmt.Errorf("ZIP 文件数量超过限制")
	}
	var total uint64
	for _, file := range zr.File {
		if file.FileInfo().Mode()&os.ModeSymlink != 0 || !file.FileInfo().Mode().IsRegular() && !file.FileInfo().IsDir() {
			return fmt.Errorf("ZIP 包含不支持的特殊文件: %s", file.Name)
		}
		name := filepath.ToSlash(file.Name)
		checkName := strings.TrimSuffix(name, "/")
		if checkName == "" || strings.HasPrefix(name, "/") || filepath.IsAbs(name) || hasUnsafeZipPathSegment(checkName) {
			return fmt.Errorf("ZIP 包含不安全路径: %s", file.Name)
		}
		total += file.UncompressedSize64
		if total > maxPluginUncompressedSize {
			return fmt.Errorf("ZIP 解压后大小超过限制")
		}
		path := filepath.Join(dest, filepath.FromSlash(name))
		cleanDest, _ := filepath.Abs(dest)
		cleanPath, _ := filepath.Abs(path)
		if cleanPath != cleanDest && !strings.HasPrefix(cleanPath, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("ZIP 包含路径逃逸: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		in, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, file.FileInfo().Mode().Perm())
		if err != nil {
			_ = in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		_ = in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func hasUnsafeZipPathSegment(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return true
		}
	}
	return false
}

func findUploadedPluginRoot(staging string) (string, error) {
	pluginRoots := map[string]bool{}
	err := filepath.WalkDir(staging, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "plugin.yaml" {
			return nil
		}
		rel, err := filepath.Rel(staging, path)
		if err != nil {
			return err
		}
		dir := filepath.Dir(rel)
		if dir == "." {
			pluginRoots[staging] = true
			return nil
		}
		parts := strings.Split(filepath.ToSlash(dir), "/")
		if len(parts) == 1 && parts[0] != "" && parts[0] != "." {
			pluginRoots[filepath.Join(staging, parts[0])] = true
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(pluginRoots) == 0 {
		return "", fmt.Errorf("未找到 plugin.yaml")
	}
	if len(pluginRoots) > 1 {
		return "", fmt.Errorf("ZIP 必须只包含一个插件包")
	}
	for root := range pluginRoots {
		return root, nil
	}
	return "", fmt.Errorf("未找到 plugin.yaml")
}

func firstPluginRoot(reg *registry.Registry) string {
	if len(reg.Root.Plugins.Paths) > 0 && strings.TrimSpace(reg.Root.Plugins.Paths[0]) != "" {
		return reg.Root.Plugins.Paths[0]
	}
	return "plugins"
}

func existingPlugin(reg *registry.Registry, id string) (plugin.Package, bool) {
	result, err := plugin.Load(reg.BaseDir, reg.Root.Plugins)
	if err != nil {
		return plugin.Package{}, false
	}
	for _, pkg := range result.Packages {
		if pkg.Manifest.ID == id {
			return pkg, true
		}
	}
	return plugin.Package{}, false
}

func replacePlugin(reg *registry.Registry, state *serverState, srcDir, installDir string) error {
	backupDir := installDir + ".backup-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.Rename(installDir, backupDir); err != nil {
		return err
	}
	installed := false
	defer func() {
		if !installed {
			_ = os.RemoveAll(installDir)
			_ = os.Rename(backupDir, installDir)
		}
	}()
	if err := copyDir(srcDir, installDir); err != nil {
		return err
	}
	newReg, err := registry.Load(reg.BaseDir)
	if err != nil {
		return err
	}
	state.reg = newReg
	installed = true
	_ = os.RemoveAll(backupDir)
	return nil
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("目标目录已存在: %s", dst)
	} else if !os.IsNotExist(err) {
		return err
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		outPath := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(outPath, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("不支持特殊文件: %s", path)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func compareVersions(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	max := len(leftParts)
	if len(rightParts) > max {
		max = len(rightParts)
	}
	for i := 0; i < max; i++ {
		lp, rp := "0", "0"
		if i < len(leftParts) {
			lp = leftParts[i]
		}
		if i < len(rightParts) {
			rp = rightParts[i]
		}
		li, lerr := strconv.Atoi(lp)
		ri, rerr := strconv.Atoi(rp)
		if lerr == nil && rerr == nil {
			if li > ri {
				return 1
			}
			if li < ri {
				return -1
			}
			continue
		}
		if cmp := strings.Compare(lp, rp); cmp != 0 {
			if cmp > 0 {
				return 1
			}
			return -1
		}
	}
	return 0
}

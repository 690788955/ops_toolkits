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
	"sort"
	"strconv"
	"strings"
	"time"

	"shell_ops/internal/config"
	"shell_ops/internal/plugin"
	"shell_ops/internal/registry"
)

const (
	maxPluginUploadSize       = 20 << 20
	maxPluginUploadFiles      = 200
	maxPluginUncompressedSize = 50 << 20
	uploadPluginFileMode      = 0o755
	uploadPluginDirectoryMode = 0o755
)

type pluginUploadResult struct {
	PluginID        string           `json:"plugin_id"`
	Version         string           `json:"version"`
	Status          string           `json:"status"`
	Existing        bool             `json:"existing"`
	ExistingVersion string           `json:"existing_version,omitempty"`
	Warnings        []plugin.Warning `json:"warnings,omitempty"`
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
var errPluginNotFound = errors.New("插件不存在")
var errPluginNotDisabled = errors.New("插件未禁用")

type pluginActionResult struct {
	PluginID string `json:"plugin_id"`
	Status   string `json:"status"`
}

func handlePluginDisable(w http.ResponseWriter, req *http.Request, state *serverState, pluginID string) {
	if req.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	result, err := disableInstalledPlugin(state, strings.Trim(pluginID, "/"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errPluginNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, response{Error: err.Error(), Data: result})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: result.Status, Data: result})
}

func handlePluginEnable(w http.ResponseWriter, req *http.Request, state *serverState, pluginID string) {
	if req.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	result, err := enableInstalledPlugin(state, strings.Trim(pluginID, "/"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errPluginNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, response{Error: err.Error(), Data: result})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: result.Status, Data: result})
}

func handlePluginDelete(w http.ResponseWriter, req *http.Request, state *serverState, pluginID string) {
	if req.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	result, err := deleteDisabledPlugin(state, pluginID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errPluginNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, response{Error: err.Error(), Data: result})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: result.Status, Data: result})
}

func disableInstalledPlugin(state *serverState, pluginID string) (pluginActionResult, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	reg := state.reg
	result := pluginActionResult{PluginID: pluginID}
	if !isSafePluginExportID(pluginID) {
		return result, fmt.Errorf("插件 ID 包含不安全路径字符")
	}
	pkg, ok := registeredPlugin(reg, pluginID)
	if !ok {
		pkg, ok = installedPlugin(reg, pluginID)
	}
	if !ok || !registryKnowsPlugin(reg, pkg) {
		return result, fmt.Errorf("%w: %s", errPluginNotFound, pluginID)
	}
	if err := ensurePluginDirInConfiguredRoot(reg, pkg.Dir); err != nil {
		return result, err
	}
	root := *reg.Root
	originalDisabled := append([]string(nil), root.Plugins.Disabled...)
	root.Plugins.Disabled = appendDisabledPlugin(append([]string(nil), originalDisabled...), pluginID)
	configPath := config.RootPath(reg.BaseDir)
	if err := config.SaveRoot(configPath, &root); err != nil {
		return result, fmt.Errorf("写入禁用配置失败: %w", err)
	}
	newReg, err := registry.Load(reg.BaseDir)
	if err != nil {
		root.Plugins.Disabled = originalDisabled
		_ = config.SaveRoot(configPath, &root)
		return result, fmt.Errorf("刷新插件注册表失败: %w", err)
	}
	state.reg = newReg
	result.Status = "disabled"
	return result, nil
}

func enableInstalledPlugin(state *serverState, pluginID string) (pluginActionResult, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	reg := state.reg
	result := pluginActionResult{PluginID: pluginID}
	if strings.HasSuffix(pluginID, ".zip") {
		return result, fmt.Errorf("启用插件请使用插件 ID，不要使用 .zip 下载路径")
	}
	if !isSafePluginExportID(pluginID) {
		return result, fmt.Errorf("插件 ID 包含不安全路径字符")
	}
	pkg, ok := installedAnyPlugin(reg, pluginID)
	if !ok {
		pkg, ok = disabledPluginCandidate(reg, pluginID)
	}
	if !ok {
		return result, fmt.Errorf("%w: %s", errPluginNotFound, pluginID)
	}
	result.PluginID = pkg.Manifest.ID
	if !pluginDisabled(reg.Root.Plugins.Disabled, pkg) {
		result.Status = "enabled"
		return result, nil
	}
	if err := ensurePluginDirInConfiguredRoot(reg, pkg.Dir); err != nil {
		return result, err
	}
	root := *reg.Root
	originalDisabled := append([]string(nil), root.Plugins.Disabled...)
	root.Plugins.Disabled = removeDisabledPlugin(append([]string(nil), originalDisabled...), pkg)
	configPath := config.RootPath(reg.BaseDir)
	if err := config.SaveRoot(configPath, &root); err != nil {
		return result, fmt.Errorf("写入启用配置失败: %w", err)
	}
	newReg, err := registry.Load(reg.BaseDir)
	if err != nil {
		root.Plugins.Disabled = originalDisabled
		_ = config.SaveRoot(configPath, &root)
		return result, fmt.Errorf("刷新插件注册表失败: %w", err)
	}
	state.reg = newReg
	result.Status = "enabled"
	return result, nil
}

func deleteDisabledPlugin(state *serverState, pluginID string) (pluginActionResult, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	reg := state.reg
	result := pluginActionResult{PluginID: pluginID}
	if strings.HasSuffix(pluginID, ".zip") {
		return result, fmt.Errorf("删除插件请使用插件 ID，不要使用 .zip 下载路径")
	}
	if !isSafePluginExportID(pluginID) {
		return result, fmt.Errorf("插件 ID 包含不安全路径字符")
	}
	pkg, ok := installedAnyPlugin(reg, pluginID)
	if !ok {
		pkg, ok = disabledPluginCandidate(reg, pluginID)
	}
	if !ok {
		return result, fmt.Errorf("%w: %s", errPluginNotFound, pluginID)
	}
	result.PluginID = pkg.Manifest.ID
	if !pluginDisabled(reg.Root.Plugins.Disabled, pkg) {
		return result, fmt.Errorf("%w，请先禁用插件 %s", errPluginNotDisabled, pkg.Manifest.ID)
	}
	if err := ensurePluginDirInConfiguredRoot(reg, pkg.Dir); err != nil {
		return result, err
	}
	if err := ensureDeletablePluginDir(pkg.Dir); err != nil {
		return result, err
	}
	if err := os.RemoveAll(pkg.Dir); err != nil {
		return result, fmt.Errorf("删除插件目录失败: %w", err)
	}
	root := *reg.Root
	root.Menu.Categories = removeDeletedPluginCategories(root.Menu.Categories, pkg, activeCategoryIDs(reg))
	root.Categories = removeDeletedPluginCategories(root.Categories, pkg, activeCategoryIDs(reg))
	root.Plugins.Disabled = removeDisabledPlugin(append([]string(nil), root.Plugins.Disabled...), pkg)
	if err := config.SaveRoot(config.RootPath(reg.BaseDir), &root); err != nil {
		return result, fmt.Errorf("插件目录已删除，但清理禁用配置失败: %w", err)
	}
	newReg, err := registry.Load(reg.BaseDir)
	if err != nil {
		return result, fmt.Errorf("插件目录已删除，但刷新插件注册表失败: %w", err)
	}
	state.reg = newReg
	result.Status = "deleted"
	return result, nil
}

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
	result := pluginUploadResult{PluginID: pkg.Manifest.ID, Version: pkg.Manifest.Version, Warnings: plugin.PackageWarnings(pkg)}
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
			if err := os.MkdirAll(path, uploadPluginDirectoryMode); err != nil {
				return err
			}
			if err := os.Chmod(path, uploadPluginDirectoryMode); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), uploadPluginDirectoryMode); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Dir(path), uploadPluginDirectoryMode); err != nil {
			return err
		}
		in, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, uploadPluginFileMode)
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
		if err := os.Chmod(path, uploadPluginFileMode); err != nil {
			return err
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
		dir := filepath.ToSlash(filepath.Dir(rel))
		if dir == "." {
			pluginRoots[staging] = true
			return nil
		}
		parts := strings.Split(dir, "/")
		if len(parts) == 1 && parts[0] != "" && parts[0] != "." {
			pluginRoots[filepath.Join(staging, parts[0])] = true
			return nil
		}
		if len(parts) == 2 && parts[0] == "plugins" && parts[1] != "" && parts[1] != "." {
			pluginRoots[filepath.Join(staging, filepath.FromSlash("plugins/"+parts[1]))] = true
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
	if err := os.MkdirAll(filepath.Dir(dst), uploadPluginDirectoryMode); err != nil {
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
			if err := os.MkdirAll(outPath, uploadPluginDirectoryMode); err != nil {
				return err
			}
			return os.Chmod(outPath, uploadPluginDirectoryMode)
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
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, uploadPluginFileMode)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		return os.Chmod(outPath, uploadPluginFileMode)
	})
}

func buildPluginExportZip(reg *registry.Registry, pluginID string) ([]byte, error) {
	if !isSafePluginExportID(pluginID) {
		return nil, fmt.Errorf("插件 ID 包含不安全路径字符")
	}
	pkg, ok := installedPlugin(reg, pluginID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errPluginNotFound, pluginID)
	}
	if !registryKnowsPlugin(reg, pkg) {
		return nil, fmt.Errorf("%w: %s", errPluginNotFound, pluginID)
	}
	if err := ensurePluginDirInConfiguredRoot(reg, pkg.Dir); err != nil {
		return nil, err
	}
	return zipPluginDir(pkg.Dir)
}

func isSafePluginExportID(pluginID string) bool {
	if strings.TrimSpace(pluginID) == "" || pluginID != strings.TrimSpace(pluginID) || pluginID == "." || pluginID == ".." || strings.ContainsAny(pluginID, `/\\`) {
		return false
	}
	if strings.Contains(pluginID, "%") || filepath.Clean(pluginID) != pluginID {
		return false
	}
	for _, ch := range pluginID {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == '-' {
			continue
		}
		return false
	}
	return true
}

func installedPluginPackages(reg *registry.Registry) []plugin.Package {
	return installedPluginPackagesWithDisabled(reg, false)
}

func installedAnyPluginPackages(reg *registry.Registry) []plugin.Package {
	return installedPluginPackagesWithDisabled(reg, true)
}

func installedPluginPackagesWithDisabled(reg *registry.Registry, includeDisabled bool) []plugin.Package {
	pluginCfg := reg.Root.Plugins
	if includeDisabled {
		pluginCfg.Disabled = nil
	}
	result, err := plugin.Load(reg.BaseDir, pluginCfg)
	if err != nil {
		return nil
	}
	return result.Packages
}

func catalogPluginEntries(reg *registry.Registry) []pluginCatalogEntry {
	entries := []pluginCatalogEntry{}
	seen := map[string]bool{}
	for _, pkg := range installedPluginPackages(reg) {
		if !registryKnowsPlugin(reg, pkg) {
			continue
		}
		entries = append(entries, pluginCatalogEntry{ID: pkg.Manifest.ID, Name: pkg.Manifest.Name, Version: pkg.Manifest.Version, Description: pkg.Manifest.Description, Disabled: false, Warnings: reg.PluginWarnings(pkg.Manifest.ID)})
		seen[pkg.Manifest.ID] = true
	}
	for _, pkg := range installedAnyPluginPackages(reg) {
		if seen[pkg.Manifest.ID] || !pluginDisabled(reg.Root.Plugins.Disabled, pkg) {
			continue
		}
		entries = append(entries, pluginCatalogEntry{ID: pkg.Manifest.ID, Name: pkg.Manifest.Name, Version: pkg.Manifest.Version, Description: pkg.Manifest.Description, Disabled: true})
		seen[pkg.Manifest.ID] = true
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func installedPlugin(reg *registry.Registry, id string) (plugin.Package, bool) {
	for _, pkg := range installedPluginPackages(reg) {
		if pkg.Manifest.ID == id {
			return pkg, true
		}
	}
	return plugin.Package{}, false
}

func installedAnyPlugin(reg *registry.Registry, id string) (plugin.Package, bool) {
	for _, pkg := range installedAnyPluginPackages(reg) {
		if pkg.Manifest.ID == id {
			return pkg, true
		}
	}
	return plugin.Package{}, false
}

func registeredPlugin(reg *registry.Registry, id string) (plugin.Package, bool) {
	for _, tool := range reg.Tools {
		if tool.Source.Type != "plugin" || tool.Source.PluginID != id {
			continue
		}
		return pluginPackageFromSource(id, tool.Source, tool.Dir), true
	}
	for _, workflow := range reg.Workflows {
		if workflow.Source.Type != "plugin" || workflow.Source.PluginID != id {
			continue
		}
		pkgDir := pluginDirFromPluginAssetPath(reg, workflow.Path)
		if pkgDir == "" {
			continue
		}
		return pluginPackageFromSource(id, workflow.Source, pkgDir), true
	}
	return plugin.Package{}, false
}

func pluginPackageFromSource(id string, source registry.Source, dir string) plugin.Package {
	return plugin.Package{
		Manifest: plugin.Manifest{
			ID:      id,
			Name:    source.PluginName,
			Version: source.PluginVersion,
		},
		Dir:  dir,
		Path: filepath.Join(dir, "plugin.yaml"),
	}
}

func pluginDirFromPluginAssetPath(reg *registry.Registry, path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	assetAbs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	for _, root := range configuredPluginRoots(reg) {
		rootAbs, err := filepath.Abs(filepath.Join(reg.BaseDir, filepath.FromSlash(root)))
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootAbs, assetAbs)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
			continue
		}
		parts := strings.Split(rel, string(os.PathSeparator))
		if len(parts) == 0 || parts[0] == "" || parts[0] == "." || parts[0] == ".." {
			continue
		}
		return filepath.Join(rootAbs, parts[0])
	}
	return ""
}

func disabledPluginCandidate(reg *registry.Registry, id string) (plugin.Package, bool) {
	if !disabledValuePresent(reg.Root.Plugins.Disabled, id) {
		return plugin.Package{}, false
	}
	for _, root := range configuredPluginRoots(reg) {
		rootPath := filepath.Join(reg.BaseDir, filepath.FromSlash(root))
		entries, err := os.ReadDir(rootPath)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() != id || !disabledValuePresent(reg.Root.Plugins.Disabled, entry.Name()) {
				continue
			}
			dir := filepath.Join(rootPath, entry.Name())
			return plugin.Package{Manifest: plugin.Manifest{ID: id}, Dir: dir, Path: filepath.Join(dir, "plugin.yaml")}, true
		}
	}
	return plugin.Package{}, false
}

func disabledValuePresent(disabled []string, value string) bool {
	for _, item := range disabled {
		if item == value {
			return true
		}
	}
	return false
}

func pluginDisabled(disabled []string, pkg plugin.Package) bool {
	dirName := filepath.Base(pkg.Dir)
	for _, value := range disabled {
		if value == pkg.Manifest.ID || value == dirName {
			return true
		}
	}
	return false
}

func appendDisabledPlugin(disabled []string, pluginID string) []string {
	for _, value := range disabled {
		if value == pluginID {
			return disabled
		}
	}
	return append(disabled, pluginID)
}

func removeDisabledPlugin(disabled []string, pkg plugin.Package) []string {
	out := disabled[:0]
	dirName := filepath.Base(pkg.Dir)
	for _, value := range disabled {
		if value == pkg.Manifest.ID || value == dirName {
			continue
		}
		out = append(out, value)
	}
	return out
}

func activeCategoryIDs(reg *registry.Registry) map[string]bool {
	active := map[string]bool{}
	for _, tool := range reg.Tools {
		if tool.Entry.Category != "" {
			active[tool.Entry.Category] = true
		}
	}
	for _, workflow := range reg.Workflows {
		if workflow.Entry.Category != "" {
			active[workflow.Entry.Category] = true
		}
	}
	return active
}

func removeDeletedPluginCategories(categories []config.Category, pkg plugin.Package, active map[string]bool) []config.Category {
	if len(categories) == 0 || len(pkg.Manifest.Contributes.Categories) == 0 {
		return categories
	}
	pluginCategories := map[string]config.Category{}
	for _, category := range pkg.Manifest.Contributes.Categories {
		if category.ID != "" {
			pluginCategories[category.ID] = category
		}
	}
	out := categories[:0]
	for _, category := range categories {
		pluginCategory, ok := pluginCategories[category.ID]
		if ok && !active[category.ID] && categoryMatchesPluginContribution(category, pluginCategory) {
			continue
		}
		out = append(out, category)
	}
	return out
}

func categoryMatchesPluginContribution(category, pluginCategory config.Category) bool {
	if category.ID != pluginCategory.ID {
		return false
	}
	if pluginCategory.Name != "" && category.Name != pluginCategory.Name {
		return false
	}
	if pluginCategory.Description != "" && category.Description != pluginCategory.Description {
		return false
	}
	return true
}
func ensureDeletablePluginDir(pluginDir string) error {
	info, err := os.Lstat(pluginDir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("插件目录不是可删除的普通目录")
	}
	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return fmt.Errorf("插件目录缺少 plugin.yaml: %w", err)
	}
	if manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
		return fmt.Errorf("插件清单不是普通文件")
	}
	return nil
}
func registryKnowsPlugin(reg *registry.Registry, pkg plugin.Package) bool {
	for _, tool := range reg.Tools {
		if tool.Source.Type == "plugin" && tool.Source.PluginID == pkg.Manifest.ID {
			return true
		}
	}
	for _, workflow := range reg.Workflows {
		if workflow.Source.Type == "plugin" && workflow.Source.PluginID == pkg.Manifest.ID {
			return true
		}
	}
	return false
}

func ensurePluginDirInConfiguredRoot(reg *registry.Registry, pluginDir string) error {
	pluginReal, err := filepath.EvalSymlinks(pluginDir)
	if err != nil {
		return err
	}
	pluginAbs, err := filepath.Abs(pluginReal)
	if err != nil {
		return err
	}
	for _, root := range configuredPluginRoots(reg) {
		rootPath := filepath.Join(reg.BaseDir, filepath.FromSlash(root))
		rootReal, err := filepath.EvalSymlinks(rootPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		rootAbs, err := filepath.Abs(rootReal)
		if err != nil {
			return err
		}
		if pluginAbs != rootAbs && strings.HasPrefix(pluginAbs, rootAbs+string(os.PathSeparator)) {
			return nil
		}
	}
	return fmt.Errorf("插件目录不在配置的 plugins root 内")
}

func configuredPluginRoots(reg *registry.Registry) []string {
	if len(reg.Root.Plugins.Paths) > 0 {
		return reg.Root.Plugins.Paths
	}
	return []string{"plugins"}
}

func zipPluginDir(pluginDir string) ([]byte, error) {
	rootName := filepath.Base(pluginDir)
	if !isSafePluginExportID(rootName) {
		return nil, fmt.Errorf("插件目录名不安全")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.WalkDir(pluginDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(pluginDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		entryName := filepath.ToSlash(filepath.Join(rootName, rel))
		checkName := strings.TrimSuffix(entryName, "/")
		if checkName == "" || strings.HasPrefix(entryName, "/") || filepath.IsAbs(entryName) || hasUnsafeZipPathSegment(checkName) {
			return fmt.Errorf("插件包含不安全路径: %s", rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("插件包含不支持的特殊文件: %s", rel)
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = entryName
		header.Method = zip.Deflate
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, file)
		return err
	})
	if err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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

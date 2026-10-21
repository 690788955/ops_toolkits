package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"shell_ops/internal/market"
	"shell_ops/internal/marketsource"
	"shell_ops/internal/nexus"
	"shell_ops/internal/plugin"
	"shell_ops/internal/registry"
)

type nexusPluginView struct {
	market.Plugin
	Installed            bool   `json:"installed"`
	InstalledVersion     string `json:"installed_version,omitempty"`
	Disabled             bool   `json:"disabled"`
	InstallStatus        string `json:"install_status"`
	Compatible           bool   `json:"compatible"`
	CompatibilityMessage string `json:"compatibility_message,omitempty"`
}

type nexusPluginsResponse struct {
	SourceID    string            `json:"source_id"`
	SourceName  string            `json:"source_name"`
	Configured  bool              `json:"configured"`
	IndexURL    string            `json:"index_url,omitempty"`
	IndexExists bool              `json:"index_exists"`
	GeneratedAt string            `json:"generated_at,omitempty"`
	Plugins     []nexusPluginView `json:"plugins"`
}

type nexusUploadResult struct {
	PluginID        string           `json:"plugin_id"`
	Version         string           `json:"version"`
	PackagePath     string           `json:"package_path"`
	PackageURL      string           `json:"package_url"`
	Size            int64            `json:"size"`
	SHA256          string           `json:"sha256"`
	Existing        bool             `json:"existing"`
	Replaced        bool             `json:"replaced"`
	Warnings        []plugin.Warning `json:"warnings,omitempty"`
	IndexUpdated    bool             `json:"index_updated"`
	PreviousPackage string           `json:"previous_package,omitempty"`
}

type nexusDeleteResult struct {
	PluginID     string `json:"plugin_id"`
	Version      string `json:"version"`
	PackagePath  string `json:"package_path"`
	AssetDeleted bool   `json:"asset_deleted"`
	IndexEntries int    `json:"index_entries"`
}

type nexusRebuildWarning struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

type nexusRebuildResult struct {
	Scanned     int                   `json:"scanned"`
	Indexed     int                   `json:"indexed"`
	Skipped     int                   `json:"skipped"`
	IndexURL    string                `json:"index_url"`
	GeneratedAt string                `json:"generated_at"`
	Warnings    []nexusRebuildWarning `json:"warnings,omitempty"`
}

func nexusConfigForRequest(reg *registry.Registry, req *http.Request) (marketsource.Source, nexus.Config, error) {
	source, err := nexusSourceByID(reg, req.URL.Query().Get("source"))
	if err != nil {
		return marketsource.Source{}, nexus.Config{}, err
	}
	return source, nexusConfigForMarketSource(source), nil
}

func nexusPluginsHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		reg := state.registry()
		source, cfg, err := nexusConfigForRequest(reg, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		result := nexusPluginsResponse{
			SourceID:   source.ID,
			SourceName: source.Name,
			Configured: true,
			IndexURL:   cfg.IndexURL(),
			Plugins:    []nexusPluginView{},
		}
		ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
		defer cancel()
		index, exists, err := nexus.NewClient(cfg).FetchIndex(ctx)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, response{Error: err.Error()})
			return
		}
		result.IndexExists = exists
		result.GeneratedAt = index.GeneratedAt
		result.Plugins = nexusPluginViews(index, installedMarketPlugins(reg), reg.Root.App.Version)
		writeJSON(w, http.StatusOK, response{Data: result})
	}
}

func nexusPluginViews(index *market.Index, installed map[string]installedMarketPlugin, appVersion string) []nexusPluginView {
	views := make([]nexusPluginView, 0, len(index.Plugins))
	for _, item := range index.Plugins {
		view := nexusPluginView{Plugin: item, Compatible: true}
		if status, ok := installed[item.ID]; ok {
			view.Installed = true
			view.InstalledVersion = status.Package.Manifest.Version
			view.Disabled = status.Disabled
		}
		view.InstallStatus = marketInstallStatus(view.Installed, view.Disabled, item.Version, view.InstalledVersion)
		view.Compatible, view.CompatibilityMessage = opsctlCompatibility(appVersion, item.Compatibility.Opsctl)
		views = append(views, view)
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].ID != views[j].ID {
			return views[i].ID < views[j].ID
		}
		return compareVersions(views[i].Version, views[j].Version) > 0
	})
	return views
}

func nexusUploadHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		reg := state.registry()
		_, cfg, err := nexusConfigForRequest(reg, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		data, err := readPluginUpload(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		result, status, err := pushPluginZipToNexus(req.Context(), state, reg, cfg, data, nexusReplaceRequested(req))
		if err != nil {
			writeJSON(w, status, response{Error: err.Error(), Data: result})
			return
		}
		action := "uploaded"
		if result.Replaced {
			action = "replaced"
		}
		writeJSON(w, http.StatusOK, response{Status: action, Data: result})
	}
}

func nexusReplaceRequested(req *http.Request) bool {
	value := req.URL.Query().Get("replace")
	return value == "true" || value == "1"
}

// pushPluginZipToNexus 校验插件 ZIP 并写入 Nexus 仓库（插件包 + 索引），
// 返回建议的 HTTP 状态码，便于上传与分享两类入口复用同一套写入逻辑。
func pushPluginZipToNexus(ctx context.Context, state *serverState, reg *registry.Registry, cfg nexus.Config, data []byte, replace bool) (nexusUploadResult, int, error) {
	pkg, cleanup, err := loadPluginZipPackage(data)
	if err != nil {
		return nexusUploadResult{}, http.StatusBadRequest, err
	}
	defer cleanup()
	if err := plugin.ValidatePackageWithConfig(pkg, reg.Root.Plugins); err != nil {
		return nexusUploadResult{}, http.StatusBadRequest, err
	}
	assetPath, err := cfg.PackagePath(pkg.Manifest.ID, pkg.Manifest.Version)
	if err != nil {
		return nexusUploadResult{}, http.StatusBadRequest, err
	}
	entry := market.PluginEntry(pkg.Manifest)
	sum := sha256.Sum256(data)
	entry.Package = market.Package{
		URL:    nexus.RelativeAssetURL(cfg.IndexPath, assetPath),
		SHA256: hex.EncodeToString(sum[:]),
		Size:   int64(len(data)),
	}
	result := nexusUploadResult{
		PluginID:    pkg.Manifest.ID,
		Version:     pkg.Manifest.Version,
		PackagePath: assetPath,
		PackageURL:  entry.Package.URL,
		Size:        entry.Package.Size,
		SHA256:      entry.Package.SHA256,
		Warnings:    plugin.PackageWarnings(pkg),
	}

	state.nexusMu.Lock()
	defer state.nexusMu.Unlock()
	pushCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	client := nexus.NewClient(cfg)
	index, _, err := client.FetchIndex(pushCtx)
	if err != nil {
		return result, http.StatusBadGateway, err
	}
	position, indexed := nexus.FindPlugin(index, entry.ID, entry.Version)
	var previousPath string
	if indexed {
		result.Existing = true
		if previousPath, err = nexus.ResolveAssetPath(cfg.IndexPath, index.Plugins[position].Package.URL); err != nil {
			previousPath = ""
		}
	}
	exists, err := client.AssetExists(pushCtx, assetPath)
	if err != nil {
		return result, http.StatusBadGateway, err
	}
	if (indexed || exists) && !replace {
		result.Existing = true
		message := fmt.Sprintf("Nexus 仓库中已存在插件 %s@%s，是否覆盖？", entry.ID, entry.Version)
		if indexed && previousPath != "" && previousPath != assetPath {
			message = fmt.Sprintf("市场索引中已存在插件 %s@%s（包 %s），是否覆盖？", entry.ID, entry.Version, previousPath)
		}
		return result, http.StatusConflict, errors.New(message)
	}
	if err := client.UploadPackage(pushCtx, assetPath, data); err != nil {
		return result, http.StatusBadGateway, err
	}
	result.Replaced = nexus.UpsertPlugin(index, entry)
	if index.SchemaVersion == 0 {
		index.SchemaVersion = market.SchemaVersion
	}
	if strings.TrimSpace(index.Name) == "" {
		index.Name = cfg.SourceName
	}
	index.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	if err := client.PutIndex(pushCtx, index); err != nil {
		return result, http.StatusBadGateway, fmt.Errorf("插件包已上传，但更新索引失败：%v。可在市场来源配置的 Nexus 来源中点击“重建索引”修复。", err)
	}
	result.IndexUpdated = true
	if indexed && previousPath != "" && previousPath != assetPath {
		result.PreviousPackage = previousPath
		if _, err := client.DeleteAsset(pushCtx, previousPath); err != nil {
			result.Warnings = append(result.Warnings, plugin.Warning{
				Code:    "NEXUS_OLD_PACKAGE",
				Message: fmt.Sprintf("旧插件包 %s 删除失败: %v", previousPath, err),
			})
		}
	}
	state.market.ClearCache()
	return result, http.StatusOK, nil
}

func nexusDeleteHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodDelete {
			methodNotAllowed(w)
			return
		}
		pluginID := strings.TrimSpace(req.URL.Query().Get("id"))
		version := strings.TrimSpace(req.URL.Query().Get("version"))
		if !isSafePluginExportID(pluginID) {
			writeJSON(w, http.StatusBadRequest, response{Error: "插件 ID 不安全"})
			return
		}
		if version == "" {
			writeJSON(w, http.StatusBadRequest, response{Error: "请指定要删除的插件版本"})
			return
		}
		reg := state.registry()
		_, cfg, err := nexusConfigForRequest(reg, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		state.nexusMu.Lock()
		defer state.nexusMu.Unlock()
		ctx, cancel := context.WithTimeout(req.Context(), time.Minute)
		defer cancel()
		client := nexus.NewClient(cfg)
		index, _, err := client.FetchIndex(ctx)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, response{Error: err.Error()})
			return
		}
		position, ok := nexus.FindPlugin(index, pluginID, version)
		if !ok {
			writeJSON(w, http.StatusNotFound, response{Error: fmt.Sprintf("Nexus 索引中没有插件 %s@%s", pluginID, version)})
			return
		}
		assetPath, err := nexus.ResolveAssetPath(cfg.IndexPath, index.Plugins[position].Package.URL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		if assetPath == cfg.IndexPath || !strings.HasPrefix(assetPath, cfg.PackagePrefix()) {
			writeJSON(w, http.StatusBadRequest, response{Error: "索引中的插件包路径不在配置的插件包目录内，已拒绝删除"})
			return
		}
		deleted, err := client.DeleteAsset(ctx, assetPath)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, response{Error: err.Error()})
			return
		}
		nexus.RemovePlugin(index, pluginID, version)
		index.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
		if err := client.PutIndex(ctx, index); err != nil {
			writeJSON(w, http.StatusBadGateway, response{Error: fmt.Sprintf("插件包已删除，但更新索引失败：%v。请重建索引修复。", err)})
			return
		}
		state.market.ClearCache()
		writeJSON(w, http.StatusOK, response{Status: "deleted", Data: nexusDeleteResult{
			PluginID:     pluginID,
			Version:      version,
			PackagePath:  assetPath,
			AssetDeleted: deleted,
			IndexEntries: len(index.Plugins),
		}})
	}
}

func nexusDownloadHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		pluginID := strings.TrimSpace(req.URL.Query().Get("id"))
		version := strings.TrimSpace(req.URL.Query().Get("version"))
		if !isSafePluginExportID(pluginID) || version == "" {
			writeJSON(w, http.StatusBadRequest, response{Error: "插件 ID 或版本无效"})
			return
		}
		reg := state.registry()
		_, cfg, err := nexusConfigForRequest(reg, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Minute)
		defer cancel()
		client := nexus.NewClient(cfg)
		index, _, err := client.FetchIndex(ctx)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, response{Error: err.Error()})
			return
		}
		position, ok := nexus.FindPlugin(index, pluginID, version)
		if !ok {
			writeJSON(w, http.StatusNotFound, response{Error: fmt.Sprintf("Nexus 索引中没有插件 %s@%s", pluginID, version)})
			return
		}
		assetPath, err := nexus.ResolveAssetPath(cfg.IndexPath, index.Plugins[position].Package.URL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		data, err := client.DownloadPackage(ctx, assetPath)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, response{Error: err.Error()})
			return
		}
		fileName := path.Base(assetPath)
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	}
}

func nexusRebuildHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		reg := state.registry()
		_, cfg, err := nexusConfigForRequest(reg, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		state.nexusMu.Lock()
		defer state.nexusMu.Unlock()
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Minute)
		defer cancel()
		client := nexus.NewClient(cfg)
		assets, err := client.ListPackages(ctx)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, response{Error: err.Error()})
			return
		}
		result := nexusRebuildResult{
			Scanned:  len(assets),
			IndexURL: cfg.IndexURL(),
			Warnings: []nexusRebuildWarning{},
		}
		index := &market.Index{
			SchemaVersion: market.SchemaVersion,
			Name:          cfg.SourceName,
			Plugins:       []market.Plugin{},
		}
		seen := map[string]string{}
		for _, asset := range assets {
			entry, warning := rebuildNexusEntry(ctx, client, cfg, asset.Path, seen, reg)
			if warning != nil {
				result.Warnings = append(result.Warnings, *warning)
				continue
			}
			index.Plugins = append(index.Plugins, entry)
		}
		result.Indexed = len(index.Plugins)
		result.Skipped = result.Scanned - result.Indexed
		if result.Scanned > 0 && result.Indexed == 0 {
			writeJSON(w, http.StatusBadGateway, response{
				Error: fmt.Sprintf("扫描到 %d 个插件包，但全部无法解析，索引未更新", result.Scanned),
				Data:  result,
			})
			return
		}
		index.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
		if err := client.PutIndex(ctx, index); err != nil {
			writeJSON(w, http.StatusBadGateway, response{Error: err.Error(), Data: result})
			return
		}
		result.GeneratedAt = index.GeneratedAt
		state.market.ClearCache()
		writeJSON(w, http.StatusOK, response{Status: "rebuilt", Data: result})
	}
}

func rebuildNexusEntry(ctx context.Context, client *nexus.Client, cfg nexus.Config, assetPath string, seen map[string]string, reg *registry.Registry) (market.Plugin, *nexusRebuildWarning) {
	warn := func(err error) (market.Plugin, *nexusRebuildWarning) {
		return market.Plugin{}, &nexusRebuildWarning{Path: assetPath, Error: err.Error()}
	}
	data, err := client.DownloadPackage(ctx, assetPath)
	if err != nil {
		return warn(err)
	}
	pkg, cleanup, err := loadPluginZipPackage(data)
	if err != nil {
		return warn(err)
	}
	defer cleanup()
	if err := plugin.ValidatePackageWithConfig(pkg, reg.Root.Plugins); err != nil {
		return warn(err)
	}
	key := pkg.Manifest.ID + "@" + pkg.Manifest.Version
	if previous, ok := seen[key]; ok {
		return warn(fmt.Errorf("插件 %s 与 %s 重复", key, previous))
	}
	seen[key] = assetPath
	sum := sha256.Sum256(data)
	entry := market.PluginEntry(pkg.Manifest)
	entry.Package = market.Package{
		URL:    nexus.RelativeAssetURL(cfg.IndexPath, assetPath),
		SHA256: hex.EncodeToString(sum[:]),
		Size:   int64(len(data)),
	}
	return entry, nil
}

func loadPluginZipPackage(data []byte) (plugin.Package, func(), error) {
	staging, err := os.MkdirTemp("", "ops-nexus-plugin-*")
	if err != nil {
		return plugin.Package{}, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(staging) }
	if err := extractPluginZip(data, staging); err != nil {
		cleanup()
		return plugin.Package{}, func() {}, err
	}
	pkgDir, err := findUploadedPluginRoot(staging)
	if err != nil {
		cleanup()
		return plugin.Package{}, func() {}, err
	}
	pkg, err := plugin.LoadPackage(pkgDir)
	if err != nil {
		cleanup()
		return plugin.Package{}, func() {}, err
	}
	return pkg, cleanup, nil
}

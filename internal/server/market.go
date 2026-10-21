package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"shell_ops/internal/market"
	"shell_ops/internal/marketsource"
	"shell_ops/internal/nexus"
	"shell_ops/internal/plugin"
	"shell_ops/internal/registry"
)

type marketCatalogResponse struct {
	Sources []marketSourceView `json:"sources"`
}

type marketSourceView struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	URL       string             `json:"url"`
	Type      string             `json:"type,omitempty"`
	Location  string             `json:"location,omitempty"`
	Status    string             `json:"status"`
	Error     string             `json:"error,omitempty"`
	FetchedAt string             `json:"fetched_at,omitempty"`
	Plugins   []marketPluginView `json:"plugins"`
}

type marketPluginView struct {
	market.Plugin
	SourceID             string `json:"source_id"`
	SourceName           string `json:"source_name"`
	Installed            bool   `json:"installed"`
	InstalledVersion     string `json:"installed_version,omitempty"`
	Disabled             bool   `json:"disabled"`
	InstallStatus        string `json:"install_status"`
	Compatible           bool   `json:"compatible"`
	CompatibilityMessage string `json:"compatibility_message,omitempty"`
}

type installedMarketPlugin struct {
	Package  plugin.Package
	Disabled bool
}

type marketInstallRequest struct {
	SourceID string `json:"source_id"`
	PluginID string `json:"plugin_id"`
	Version  string `json:"version"`
	Replace  bool   `json:"replace"`
}

func marketCatalogHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		reg := state.registry()
		installed := installedMarketPlugins(reg)
		refresh := req.URL.Query().Get("refresh")
		force := refresh == "1" || strings.EqualFold(refresh, "true")
		sourceConfigs, err := loadMarketSources(reg)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
		sources := make([]marketSourceView, 0, len(sourceConfigs))
		for _, source := range sourceConfigs {
			if !source.IsEnabled() {
				continue
			}
			view := marketSourceView{
				ID:      source.ID,
				Name:    source.Name,
				URL:     source.URL,
				Type:    source.Type,
				Status:  "ok",
				Plugins: []marketPluginView{},
			}
			ctx, cancel := context.WithTimeout(req.Context(), 20*time.Second)
			var indexData *market.Index
			var fetchedAt time.Time
			if source.Type == marketsource.TypeNexus {
				cfg := nexusConfigForMarketSource(source)
				view.Location = cfg.IndexURL()
				if cfg.Ready() {
					indexData, _, err = nexus.NewClient(cfg).FetchIndex(ctx)
					if err == nil {
						fetchedAt = time.Now().UTC()
					}
				} else {
					err = fmt.Errorf("Nexus 来源未配置完整")
				}
			} else {
				var resolved market.Source
				resolved, err = market.ResolveSource(source.URL, reg.BaseDir)
				if err == nil {
					view.Location = resolved.Location
					err = ensureLocalMarketIndex(state, reg, source, resolved)
					if err == nil {
						indexData, fetchedAt, err = state.market.Fetch(ctx, source.URL, force, marketCredentials(source))
					}
				}
			}
			cancel()
			if err != nil {
				view.Status = "error"
				view.Error = err.Error()
				sources = append(sources, view)
				continue
			}
			if !fetchedAt.IsZero() {
				view.FetchedAt = fetchedAt.UTC().Format(time.RFC3339)
			}
			view.Plugins = marketPluginViews(indexData, installed, view.ID, view.Name, reg.Root.App.Version)
			sources = append(sources, view)
		}
		writeJSON(w, http.StatusOK, response{Data: marketCatalogResponse{Sources: sources}})
	}
}

func marketInstallHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var payload marketInstallRequest
		if err := json.NewDecoder(io.LimitReader(req.Body, 1<<16)).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "请求参数无效: " + err.Error()})
			return
		}
		payload.SourceID = strings.TrimSpace(payload.SourceID)
		payload.PluginID = strings.TrimSpace(payload.PluginID)
		payload.Version = strings.TrimSpace(payload.Version)
		if !isSafePluginExportID(payload.PluginID) {
			writeJSON(w, http.StatusBadRequest, response{Error: "插件 ID 不安全"})
			return
		}
		reg := state.registry()
		sourceConfigs, err := loadMarketSources(reg)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
		source, err := findMarketSource(sourceConfigs, payload.SourceID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		fetchCtx, cancelFetch := context.WithTimeout(req.Context(), 20*time.Second)
		var indexData *market.Index
		if source.Type == marketsource.TypeNexus {
			indexData, _, err = nexus.NewClient(nexusConfigForMarketSource(source)).FetchIndex(fetchCtx)
		} else {
			resolved, resolveErr := market.ResolveSource(source.URL, reg.BaseDir)
			if resolveErr != nil {
				err = resolveErr
			} else if ensureErr := ensureLocalMarketIndex(state, reg, source, resolved); ensureErr != nil {
				err = ensureErr
			} else {
				indexData, _, err = state.market.Fetch(fetchCtx, source.URL, true, marketCredentials(source))
			}
		}
		cancelFetch()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, response{Error: err.Error()})
			return
		}
		entry, err := findMarketPlugin(indexData, payload.PluginID, payload.Version)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		if compatible, message := opsctlCompatibility(reg.Root.App.Version, entry.Compatibility.Opsctl); !compatible {
			writeJSON(w, http.StatusBadRequest, response{Error: message})
			return
		}
		result := pluginUploadResult{PluginID: entry.ID, Version: entry.Version}
		installedPkg, installed := installedAnyPlugin(reg, entry.ID)
		if installed {
			result.Existing = true
			result.ExistingVersion = installedPkg.Manifest.Version
			if pluginDisabled(reg.Root.Plugins.Disabled, installedPkg) && payload.Replace {
				writeJSON(w, http.StatusBadRequest, response{Error: fmt.Sprintf("插件 %s 已安装但处于禁用状态，请先启用后再更新", entry.ID), Data: result})
				return
			}
			if !payload.Replace {
				result.Status = "duplicate"
				writeJSON(w, http.StatusConflict, response{
					Error: fmt.Sprintf("插件 %s 已安装（当前版本 %s），是否更新到 %s？", entry.ID, installedPkg.Manifest.Version, entry.Version),
					Data:  result,
				})
				return
			}
		} else if payload.Replace {
			writeJSON(w, http.StatusBadRequest, response{Error: fmt.Sprintf("插件 %s 不存在，无法更新", entry.ID), Data: result})
			return
		}
		downloadCtx, cancelDownload := context.WithTimeout(req.Context(), 2*time.Minute)
		data, err := downloadMarketPackage(downloadCtx, state, source, entry.Package)
		cancelDownload()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, response{Error: err.Error()})
			return
		}
		result, err = installPluginArchive(state, data, payload.Replace, &expectedPlugin{ID: entry.ID, Version: entry.Version})
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

func ensureLocalMarketIndex(state *serverState, reg *registry.Registry, source marketsource.Source, resolved market.Source) error {
	if resolved.Kind != market.SourceLocal || filepath.Base(resolved.Location) != "index.json" {
		return nil
	}
	state.marketBuildMu.Lock()
	defer state.marketBuildMu.Unlock()
	if _, err := os.Stat(resolved.Location); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取本地市场索引失败: %w", err)
	}
	if !pathWithinDir(reg.BaseDir, resolved.LocalDir) {
		return nil
	}
	name := strings.TrimSpace(source.Name)
	if name == "" {
		name = marketsource.DefaultLocalName
	}
	if _, err := market.BuildIndex(market.BuildOptions{
		BaseDir:    reg.BaseDir,
		Plugins:    reg.Root.Plugins,
		OutputDir:  resolved.LocalDir,
		Name:       name,
		AllowEmpty: true,
	}); err != nil {
		return fmt.Errorf("生成本地市场索引失败: %w", err)
	}
	return nil
}

func installedMarketPlugins(reg *registry.Registry) map[string]installedMarketPlugin {
	result := map[string]installedMarketPlugin{}
	for _, pkg := range installedAnyPluginPackages(reg) {
		result[pkg.Manifest.ID] = installedMarketPlugin{
			Package:  pkg,
			Disabled: pluginDisabled(reg.Root.Plugins.Disabled, pkg),
		}
	}
	return result
}

func marketPluginViews(index *market.Index, installed map[string]installedMarketPlugin, sourceID, sourceName, appVersion string) []marketPluginView {
	catalog := make([]market.Plugin, 0, len(index.Plugins))
	positions := map[string]int{}
	for _, item := range index.Plugins {
		position, ok := positions[item.ID]
		if !ok {
			positions[item.ID] = len(catalog)
			catalog = append(catalog, item)
			continue
		}
		if compareVersions(item.Version, catalog[position].Version) > 0 {
			catalog[position] = item
		}
	}
	views := make([]marketPluginView, 0, len(catalog))
	for _, item := range catalog {
		view := marketPluginView{
			Plugin:     item,
			SourceID:   sourceID,
			SourceName: sourceName,
			Compatible: true,
		}
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
		if views[i].Name == views[j].Name {
			return views[i].ID < views[j].ID
		}
		return views[i].Name < views[j].Name
	})
	return views
}

func marketInstallStatus(installed, disabled bool, marketVersion, installedVersion string) string {
	if !installed {
		return "not_installed"
	}
	if disabled {
		return "disabled"
	}
	switch compareVersions(marketVersion, installedVersion) {
	case 1:
		return "update_available"
	case -1:
		return "local_newer"
	default:
		return "installed"
	}
}

func findMarketSource(sources []marketsource.Source, id string) (marketsource.Source, error) {
	if len(sources) == 0 {
		return marketsource.Source{}, fmt.Errorf("未配置插件市场源")
	}
	if id == "" {
		if len(sources) == 1 {
			if !sources[0].IsEnabled() {
				return marketsource.Source{}, fmt.Errorf("插件市场源 %s 已禁用", sources[0].ID)
			}
			return sources[0], nil
		}
		return marketsource.Source{}, fmt.Errorf("配置了多个市场源，请指定 source_id")
	}
	for _, source := range sources {
		if source.ID == id {
			if !source.IsEnabled() {
				return marketsource.Source{}, fmt.Errorf("插件市场源 %s 已禁用", id)
			}
			return source, nil
		}
	}
	return marketsource.Source{}, fmt.Errorf("未找到市场源 %s", id)
}

func downloadMarketPackage(ctx context.Context, state *serverState, source marketsource.Source, pkg market.Package) ([]byte, error) {
	if source.Type != marketsource.TypeNexus {
		return state.market.Download(ctx, source.URL, pkg, marketCredentials(source))
	}
	cfg := nexusConfigForMarketSource(source)
	assetPath, err := nexus.ResolveAssetPath(cfg.IndexPath, pkg.URL)
	if err != nil {
		return nil, err
	}
	data, err := nexus.NewClient(cfg).DownloadPackage(ctx, assetPath)
	if err != nil {
		return nil, err
	}
	if err := market.VerifyPackage(data, pkg); err != nil {
		return nil, err
	}
	return data, nil
}

func findMarketPlugin(index *market.Index, id, version string) (market.Plugin, error) {
	var found *market.Plugin
	for i := range index.Plugins {
		item := &index.Plugins[i]
		if item.ID != id {
			continue
		}
		if version != "" && item.Version != version {
			continue
		}
		if found == nil || compareVersions(item.Version, found.Version) > 0 {
			found = item
		}
	}
	if found == nil {
		if version != "" {
			return market.Plugin{}, fmt.Errorf("市场中没有插件 %s@%s", id, version)
		}
		return market.Plugin{}, fmt.Errorf("市场中没有插件 %s", id)
	}
	return *found, nil
}

func opsctlCompatibility(current, constraint string) (bool, string) {
	current = strings.TrimPrefix(strings.TrimSpace(current), "v")
	constraint = strings.TrimSpace(constraint)
	if current == "" || constraint == "" {
		return true, ""
	}
	tokens := strings.FieldsFunc(constraint, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	for _, token := range tokens {
		if token == "*" || strings.EqualFold(token, "any") {
			continue
		}
		operator, version := splitVersionConstraint(token)
		version = strings.TrimPrefix(strings.TrimSpace(version), "v")
		if version == "" {
			return false, fmt.Sprintf("无法识别的 compatibility.opsctl 版本约束: %s", constraint)
		}
		comparison := compareVersions(current, version)
		matched := false
		switch operator {
		case ">=":
			matched = comparison >= 0
		case ">":
			matched = comparison > 0
		case "<=":
			matched = comparison <= 0
		case "<":
			matched = comparison < 0
		case "=", "==":
			matched = comparison == 0
		case "":
			matched = comparison == 0
		default:
			return false, fmt.Sprintf("无法识别的 compatibility.opsctl 版本约束: %s", token)
		}
		if !matched {
			return false, fmt.Sprintf("插件要求 opsctl %s，当前版本 %s", constraint, current)
		}
	}
	return true, ""
}

func splitVersionConstraint(token string) (string, string) {
	for _, operator := range []string{">=", "<=", "==", ">", "<", "="} {
		if strings.HasPrefix(token, operator) {
			return operator, strings.TrimSpace(strings.TrimPrefix(token, operator))
		}
	}
	return "", token
}

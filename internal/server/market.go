package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"shell_ops/internal/config"
	"shell_ops/internal/market"
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
		sourceConfigs := configuredMarketSources(reg)
		sources := make([]marketSourceView, 0, len(sourceConfigs))
		for index, source := range sourceConfigs {
			view := marketSourceView{
				ID:      source.ID,
				Name:    source.Name,
				URL:     source.URL,
				Status:  "ok",
				Plugins: []marketPluginView{},
			}
			if view.ID == "" {
				view.ID = fmt.Sprintf("source-%d", index+1)
			}
			if view.Name == "" {
				view.Name = view.ID
			}
			resolved, err := market.ResolveSource(source.URL, reg.BaseDir)
			if err != nil {
				view.Status = "error"
				view.Error = err.Error()
				sources = append(sources, view)
				continue
			}
			view.Type = string(resolved.Kind)
			view.Location = resolved.Location
			ctx, cancel := context.WithTimeout(req.Context(), 20*time.Second)
			indexData, fetchedAt, err := state.market.Fetch(ctx, source.URL, force)
			cancel()
			if err != nil {
				view.Status = "error"
				view.Error = err.Error()
				sources = append(sources, view)
				continue
			}
			view.FetchedAt = fetchedAt.UTC().Format(time.RFC3339)
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
		source, err := findMarketSource(configuredMarketSources(reg), payload.SourceID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		if _, err := market.ResolveSource(source.URL, reg.BaseDir); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		fetchCtx, cancelFetch := context.WithTimeout(req.Context(), 20*time.Second)
		indexData, _, err := state.market.Fetch(fetchCtx, source.URL, true)
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
		data, err := state.market.Download(downloadCtx, source.URL, entry.Package)
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

func configuredMarketSources(reg *registry.Registry) []config.MarketSource {
	if reg == nil || reg.Root == nil {
		return nil
	}
	if len(reg.Root.Market.Sources) > 0 {
		return reg.Root.Market.Sources
	}
	return []config.MarketSource{{ID: "local", Name: "本地插件市场", URL: "market"}}
}

func findMarketSource(sources []config.MarketSource, id string) (config.MarketSource, error) {
	if len(sources) == 0 {
		return config.MarketSource{}, fmt.Errorf("未配置插件市场源")
	}
	if id == "" {
		if len(sources) == 1 {
			return sources[0], nil
		}
		return config.MarketSource{}, fmt.Errorf("配置了多个市场源，请指定 source_id")
	}
	for _, source := range sources {
		if source.ID == id {
			return source, nil
		}
	}
	return config.MarketSource{}, fmt.Errorf("未找到市场源 %s", id)
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

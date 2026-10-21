package server

import (
	"errors"
	"net/http"
	"strings"

	"shell_ops/internal/marketsource"
	"shell_ops/internal/registry"
)

// nexusShareTarget 描述一个可用于接收插件分享的远程插件市场（Nexus）。
// 只有启用且地址/仓库名完整的 Nexus 来源才会出现在分享目标里。
type nexusShareTarget struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	Repository string `json:"repository"`
	IndexPath  string `json:"index_path"`
	IndexURL   string `json:"index_url,omitempty"`
}

type nexusShareTargetsResponse struct {
	Targets []nexusShareTarget `json:"targets"`
	Message string             `json:"message,omitempty"`
}

type pluginShareResult struct {
	SourceID   string `json:"source_id"`
	SourceName string `json:"source_name"`
	nexusUploadResult
}

func nexusShareTargetsHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		targets, err := nexusShareTargets(state.registry())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
		result := nexusShareTargetsResponse{Targets: targets}
		if len(targets) == 0 {
			result.Message = "未配置可用的远程插件市场（Nexus），只能使用本地插件市场。请在插件市场 → 来源配置中新增并启用 Nexus 来源后再分享插件。"
		}
		writeJSON(w, http.StatusOK, response{Data: result})
	}
}

func nexusShareTargets(reg *registry.Registry) ([]nexusShareTarget, error) {
	sources, err := loadMarketSources(reg)
	if err != nil {
		return nil, err
	}
	targets := make([]nexusShareTarget, 0, len(sources))
	for _, source := range sources {
		if source.Type != marketsource.TypeNexus || !source.Ready() {
			continue
		}
		cfg := nexusConfigForMarketSource(source)
		targets = append(targets, nexusShareTarget{
			ID:         source.ID,
			Name:       source.Name,
			URL:        source.URL,
			Repository: source.Repository,
			IndexPath:  cfg.IndexPath,
			IndexURL:   cfg.IndexURL(),
		})
	}
	return targets, nil
}

// handlePluginShare 把本机已安装插件打包成 ZIP 后直接上传到远程插件市场（Nexus）。
// 没有可用的远程市场时返回 400，由前端把分享入口置灰。
func handlePluginShare(w http.ResponseWriter, req *http.Request, state *serverState, pluginID string) {
	if req.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	pluginID = strings.Trim(strings.TrimSpace(pluginID), "/")
	if !isSafePluginExportID(pluginID) {
		writeJSON(w, http.StatusBadRequest, response{Error: "插件 ID 不安全"})
		return
	}
	reg := state.registry()
	source, err := nexusSourceByID(reg, req.URL.Query().Get("source"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	data, err := buildPluginExportZip(reg, pluginID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errPluginNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, response{Error: err.Error()})
		return
	}
	result, status, err := pushPluginZipToNexus(req.Context(), state, reg, nexusConfigForMarketSource(source), data, nexusReplaceRequested(req))
	share := pluginShareResult{SourceID: source.ID, SourceName: source.Name, nexusUploadResult: result}
	if err != nil {
		writeJSON(w, status, response{Error: err.Error(), Data: share})
		return
	}
	action := "shared"
	if result.Replaced {
		action = "updated"
	}
	writeJSON(w, http.StatusOK, response{Status: action, Data: share})
}

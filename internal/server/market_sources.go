package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"shell_ops/internal/market"
	"shell_ops/internal/marketsource"
	"shell_ops/internal/nexus"
	"shell_ops/internal/registry"
)

type marketSourceConfigView struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Type               string `json:"type"`
	URL                string `json:"url"`
	Enabled            bool   `json:"enabled"`
	AuthType           string `json:"auth_type"`
	Username           string `json:"username,omitempty"`
	PasswordSet        bool   `json:"password_set"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	Repository         string `json:"repository,omitempty"`
	IndexPath          string `json:"index_path,omitempty"`
	PackageDir         string `json:"package_dir,omitempty"`
	Ready              bool   `json:"ready"`
	IndexURL           string `json:"index_url,omitempty"`
}

type marketSourcesResponse struct {
	Sources    []marketSourceConfigView `json:"sources"`
	ConfigPath string                   `json:"config_path"`
}

type marketSourceTestResult struct {
	Type        string `json:"type"`
	Reachable   bool   `json:"reachable"`
	IndexExists bool   `json:"index_exists"`
	IndexValid  bool   `json:"index_valid"`
	PluginCount int    `json:"plugin_count"`
	IndexURL    string `json:"index_url,omitempty"`
	Location    string `json:"location,omitempty"`
	Message     string `json:"message"`
}

type marketSourcePayload struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Type               string `json:"type"`
	URL                string `json:"url"`
	Enabled            *bool  `json:"enabled"`
	AuthType           string `json:"auth_type"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	Repository         string `json:"repository"`
	IndexPath          string `json:"index_path"`
	PackageDir         string `json:"package_dir"`
}

func marketSourcesHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reg := state.registry()
		switch req.Method {
		case http.MethodGet:
			sources, err := loadMarketSources(reg)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, response{Data: marketSourcesResponse{
				Sources:    marketSourceConfigViews(sources),
				ConfigPath: marketsource.ConfigPath(reg.BaseDir),
			}})
		case http.MethodPost, http.MethodPut:
			state.marketSourceMu.Lock()
			defer state.marketSourceMu.Unlock()
			var payload marketSourcePayload
			if err := json.NewDecoder(io.LimitReader(req.Body, 1<<16)).Decode(&payload); err != nil {
				writeJSON(w, http.StatusBadRequest, response{Error: "请求参数无效: " + err.Error()})
				return
			}
			sources, err := loadMarketSources(reg)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
				return
			}
			id := strings.TrimSpace(payload.ID)
			position := marketSourcePosition(sources, id)
			if req.Method == http.MethodPost && position >= 0 {
				writeJSON(w, http.StatusConflict, response{Error: fmt.Sprintf("市场来源 ID %s 已存在", id)})
				return
			}
			if req.Method == http.MethodPut && position < 0 {
				writeJSON(w, http.StatusNotFound, response{Error: fmt.Sprintf("未找到市场来源 %s", id)})
				return
			}
			var existing *marketsource.Source
			if position >= 0 {
				existing = &sources[position]
				if strings.TrimSpace(payload.AuthType) == "" {
					payload.AuthType = existing.AuthType
				}
			}
			source := payload.toSource()
			if source.Password == "" && existing != nil && existing.AuthType == marketsource.AuthBasic && source.AuthType == marketsource.AuthBasic {
				source.Password = existing.Password
			}
			normalized, err := marketsource.NormalizeSource(source)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
				return
			}
			if position >= 0 {
				sources[position] = normalized
			} else {
				sources = append(sources, normalized)
			}
			if _, err := marketsource.Save(reg.BaseDir, marketsource.Config{Sources: sources}); err != nil {
				writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
				return
			}
			state.market.ClearCache()
			writeJSON(w, http.StatusOK, response{Status: "saved", Data: marketSourceConfigViewFor(normalized)})
		case http.MethodDelete:
			state.marketSourceMu.Lock()
			defer state.marketSourceMu.Unlock()
			id := strings.TrimSpace(req.URL.Query().Get("id"))
			sources, err := loadMarketSources(reg)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
				return
			}
			position := marketSourcePosition(sources, id)
			if position < 0 {
				writeJSON(w, http.StatusNotFound, response{Error: fmt.Sprintf("未找到市场来源 %s", id)})
				return
			}
			removed := sources[position]
			sources = append(sources[:position], sources[position+1:]...)
			if _, err := marketsource.Save(reg.BaseDir, marketsource.Config{Sources: sources}); err != nil {
				writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
				return
			}
			state.market.ClearCache()
			writeJSON(w, http.StatusOK, response{Status: "deleted", Data: map[string]interface{}{
				"id":        removed.ID,
				"name":      removed.Name,
				"remaining": len(sources),
			}})
		default:
			methodNotAllowed(w)
		}
	}
}

func marketSourceTestHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var payload marketSourcePayload
		if err := json.NewDecoder(io.LimitReader(req.Body, 1<<16)).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "请求参数无效: " + err.Error()})
			return
		}
		reg := state.registry()
		source := payload.toSource()
		if strings.TrimSpace(source.ID) == "" {
			source.ID = "test-source"
		}
		sources, err := loadMarketSources(reg)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
		if position := marketSourcePosition(sources, source.ID); position >= 0 {
			existing := sources[position]
			if source.Password == "" && source.AuthType == marketsource.AuthBasic && existing.AuthType == marketsource.AuthBasic {
				source.Password = existing.Password
			}
		}
		normalized, err := marketsource.NormalizeSource(source)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
		defer cancel()
		writeJSON(w, http.StatusOK, response{Data: testMarketSource(ctx, state, reg, normalized)})
	}
}

func testMarketSource(ctx context.Context, state *serverState, reg *registry.Registry, source marketsource.Source) marketSourceTestResult {
	result := marketSourceTestResult{Type: source.Type}
	if source.Type == marketsource.TypeNexus {
		test := nexus.NewClient(nexusConfigForMarketSource(source)).Test(ctx)
		result.Reachable = test.Reachable
		result.IndexExists = test.IndexExists
		result.IndexValid = test.IndexValid
		result.PluginCount = test.PluginCount
		result.IndexURL = test.IndexURL
		result.Message = test.Message
		return result
	}
	resolved, err := market.ResolveSource(source.URL, reg.BaseDir)
	if err != nil {
		result.Message = err.Error()
		return result
	}
	if err := ensureLocalMarketIndex(state, reg, source, resolved); err != nil {
		result.Message = err.Error()
		return result
	}
	result.Location = resolved.Location
	result.IndexURL = source.URL
	index, _, err := state.market.Fetch(ctx, source.URL, true, marketCredentials(source))
	if err != nil {
		result.Message = err.Error()
		return result
	}
	result.Reachable = true
	result.IndexExists = true
	result.IndexValid = true
	result.PluginCount = len(index.Plugins)
	result.Message = fmt.Sprintf("连接成功，索引包含 %d 个插件版本。", len(index.Plugins))
	return result
}

func loadMarketSources(reg *registry.Registry) ([]marketsource.Source, error) {
	if reg == nil || reg.Root == nil {
		return nil, nil
	}
	cfg, exists, err := marketsource.Load(reg.BaseDir)
	if err != nil {
		return nil, err
	}
	if exists {
		return cfg.Sources, nil
	}
	return migrateMarketSources(reg), nil
}

func migrateMarketSources(reg *registry.Registry) []marketsource.Source {
	sources := []marketsource.Source{}
	seenIDs := map[string]bool{}
	seenURLs := map[string]bool{}
	for index, legacy := range reg.Root.Market.Sources {
		id := strings.TrimSpace(legacy.ID)
		if id == "" {
			id = fmt.Sprintf("source-%d", index+1)
		}
		name := strings.TrimSpace(legacy.Name)
		if name == "" {
			name = "市场来源 " + id
		}
		source, err := marketsource.NormalizeSource(marketsource.Source{ID: id, Name: name, URL: legacy.URL})
		if err != nil || seenIDs[source.ID] || seenURLs[source.URL] {
			continue
		}
		seenIDs[source.ID] = true
		seenURLs[source.URL] = true
		sources = append(sources, source)
	}
	if legacy, err := nexus.LoadConfig(reg.BaseDir); err == nil && legacy.Ready() {
		source, err := marketsource.NormalizeSource(marketsource.Source{
			ID:                 legacy.SourceID,
			Name:               legacy.SourceName,
			Type:               marketsource.TypeNexus,
			URL:                legacy.BaseURL,
			Repository:         legacy.Repository,
			IndexPath:          legacy.IndexPath,
			PackageDir:         legacy.PackageDir,
			Username:           legacy.Username,
			Password:           legacy.Password,
			InsecureSkipVerify: legacy.InsecureSkipVerify,
		})
		if err == nil && !seenIDs[source.ID] && !seenURLs[source.URL] {
			seenIDs[source.ID] = true
			seenURLs[source.URL] = true
			sources = append(sources, source)
		}
	}
	if len(sources) == 0 {
		return []marketsource.Source{marketsource.DefaultLocalSource()}
	}
	return sources
}

func marketSourceConfigViews(sources []marketsource.Source) []marketSourceConfigView {
	views := make([]marketSourceConfigView, 0, len(sources))
	for _, source := range sources {
		views = append(views, marketSourceConfigViewFor(source))
	}
	return views
}

func marketSourceConfigViewFor(source marketsource.Source) marketSourceConfigView {
	view := marketSourceConfigView{
		ID:                 source.ID,
		Name:               source.Name,
		Type:               source.Type,
		URL:                source.URL,
		Enabled:            source.IsEnabled(),
		AuthType:           source.AuthType,
		Username:           source.Username,
		PasswordSet:        source.Password != "",
		InsecureSkipVerify: source.InsecureSkipVerify,
		Repository:         source.Repository,
		IndexPath:          source.IndexPath,
		PackageDir:         source.PackageDir,
		Ready:              source.Ready(),
	}
	if source.Type == marketsource.TypeNexus {
		view.IndexURL = nexusConfigForMarketSource(source).IndexURL()
	}
	return view
}

func marketSourcePosition(sources []marketsource.Source, id string) int {
	for index, source := range sources {
		if source.ID == id {
			return index
		}
	}
	return -1
}

func nexusConfigForMarketSource(source marketsource.Source) nexus.Config {
	enabled := source.IsEnabled()
	return nexus.Config{
		Enabled:            &enabled,
		BaseURL:            source.URL,
		Repository:         source.Repository,
		SourceID:           source.ID,
		SourceName:         source.Name,
		IndexPath:          source.IndexPath,
		PackageDir:         source.PackageDir,
		Username:           source.Username,
		Password:           source.Password,
		InsecureSkipVerify: source.InsecureSkipVerify,
	}
}

func nexusSourceByID(reg *registry.Registry, sourceID string) (marketsource.Source, error) {
	sources, err := loadMarketSources(reg)
	if err != nil {
		return marketsource.Source{}, err
	}
	sourceID = strings.TrimSpace(sourceID)
	if sourceID != "" {
		position := marketSourcePosition(sources, sourceID)
		if position < 0 {
			return marketsource.Source{}, fmt.Errorf("未找到市场来源 %s", sourceID)
		}
		source := sources[position]
		if source.Type != marketsource.TypeNexus {
			return marketsource.Source{}, fmt.Errorf("市场来源 %s 不是 Nexus 类型", sourceID)
		}
		if !source.Ready() {
			return source, fmt.Errorf("Nexus 市场来源 %s 未启用或缺少地址/仓库名", sourceID)
		}
		return source, nil
	}
	var candidates []marketsource.Source
	for _, source := range sources {
		if source.Type == marketsource.TypeNexus && source.Ready() {
			candidates = append(candidates, source)
		}
	}
	switch len(candidates) {
	case 0:
		return marketsource.Source{}, fmt.Errorf("未配置可用的 Nexus 市场来源，请先在插件市场的来源配置中新增")
	case 1:
		return candidates[0], nil
	default:
		return marketsource.Source{}, fmt.Errorf("存在多个 Nexus 市场来源，请指定 source 参数")
	}
}

func marketCredentials(source marketsource.Source) *market.Credentials {
	if !source.IsBasicAuth() {
		return nil
	}
	return &market.Credentials{Username: source.Username, Password: source.Password}
}

func (p marketSourcePayload) toSource() marketsource.Source {
	return marketsource.Source{
		ID:                 strings.TrimSpace(p.ID),
		Name:               strings.TrimSpace(p.Name),
		Type:               strings.TrimSpace(p.Type),
		URL:                strings.TrimSpace(p.URL),
		Enabled:            p.Enabled,
		AuthType:           strings.TrimSpace(p.AuthType),
		Username:           strings.TrimSpace(p.Username),
		Password:           p.Password,
		InsecureSkipVerify: p.InsecureSkipVerify,
		Repository:         strings.TrimSpace(p.Repository),
		IndexPath:          strings.TrimSpace(p.IndexPath),
		PackageDir:         strings.TrimSpace(p.PackageDir),
	}
}

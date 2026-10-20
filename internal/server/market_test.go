package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell_ops/internal/config"
	"shell_ops/internal/market"
	"shell_ops/internal/registry"
)

func TestMarketCatalogReportsInstallStatuses(t *testing.T) {
	newZip := pluginZip(t, "vendor.marketnew", "1.0.0", false)
	updateZip := pluginZip(t, "vendor.marketupdate", "1.2.0", false)
	sameZip := pluginZip(t, "vendor.marketsame", "1.0.0", false)
	disabledZip := pluginZip(t, "vendor.marketdisabled", "1.0.0", false)
	indexServer := marketTestServer(t, []market.Plugin{
		marketTestEntry("vendor.marketnew", "1.0.0", newZip),
		marketTestEntry("vendor.marketupdate", "1.2.0", updateZip),
		marketTestEntry("vendor.marketsame", "1.0.0", sameZip),
		marketTestEntry("vendor.marketdisabled", "1.0.0", disabledZip),
	}, newZip, updateZip, sameZip, disabledZip)

	reg := marketTestRegistry(t, indexServer.URL)
	installTestPlugin(t, reg.BaseDir, "vendor.marketupdate", "1.0.0")
	installTestPlugin(t, reg.BaseDir, "vendor.marketsame", "1.0.0")
	installTestPlugin(t, reg.BaseDir, "vendor.marketdisabled", "1.0.0")
	reg.Root.Plugins.Disabled = []string{"vendor.marketdisabled"}

	res := httptest.NewRecorder()
	marketCatalogHandler(newServerState(reg))(res, httptest.NewRequest(http.MethodGet, "/api/market", nil))

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data marketCatalogResponse `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析市场响应失败: %v", err)
	}
	if len(body.Data.Sources) != 1 {
		t.Fatalf("市场源数量 = %d, want 1", len(body.Data.Sources))
	}
	source := body.Data.Sources[0]
	if source.Status != "ok" {
		t.Fatalf("市场源状态 = %q, error = %q", source.Status, source.Error)
	}
	if source.FetchedAt == "" {
		t.Fatal("市场源缺少 fetched_at")
	}
	statuses := map[string]string{}
	for _, item := range source.Plugins {
		statuses[item.ID] = item.InstallStatus
	}
	want := map[string]string{
		"vendor.marketnew":      "not_installed",
		"vendor.marketupdate":   "update_available",
		"vendor.marketsame":     "installed",
		"vendor.marketdisabled": "disabled",
	}
	for id, wantStatus := range want {
		if statuses[id] != wantStatus {
			t.Fatalf("%s install_status = %q, want %q", id, statuses[id], wantStatus)
		}
	}
}

func TestMarketCatalogReportsSourceErrors(t *testing.T) {
	reg := marketTestRegistry(t, "http://127.0.0.1:1")
	res := httptest.NewRecorder()
	marketCatalogHandler(newServerState(reg))(res, httptest.NewRequest(http.MethodGet, "/api/market", nil))

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data marketCatalogResponse `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析市场响应失败: %v", err)
	}
	if len(body.Data.Sources) != 1 || body.Data.Sources[0].Status != "error" || body.Data.Sources[0].Error == "" {
		t.Fatalf("市场源错误未上报: %+v", body.Data.Sources)
	}
}

func TestMarketCatalogUsesDefaultLocalSource(t *testing.T) {
	zipData := pluginZip(t, "vendor.localmarket", "1.0.0", false)
	reg := testRegistry(t)
	writeLocalMarketSource(t, reg.BaseDir, []market.Plugin{marketTestEntry("vendor.localmarket", "1.0.0", zipData)}, zipData)

	res := httptest.NewRecorder()
	marketCatalogHandler(newServerState(reg))(res, httptest.NewRequest(http.MethodGet, "/api/market", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data marketCatalogResponse `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析市场响应失败: %v", err)
	}
	if len(body.Data.Sources) != 1 {
		t.Fatalf("市场源数量 = %d, want 1", len(body.Data.Sources))
	}
	source := body.Data.Sources[0]
	if source.ID != "local" || source.Name != "本地插件市场" || source.Type != "local" {
		t.Fatalf("默认市场源 = %+v", source)
	}
	if want := filepath.Join(reg.BaseDir, "market", "index.json"); source.Location != want {
		t.Fatalf("市场源位置 = %q, want %q", source.Location, want)
	}
	if source.Status != "ok" || source.Error != "" {
		t.Fatalf("市场源状态 = %q, error = %q", source.Status, source.Error)
	}
	if len(source.Plugins) != 1 || source.Plugins[0].ID != "vendor.localmarket" {
		t.Fatalf("市场插件 = %+v", source.Plugins)
	}
}

func TestMarketInstallUsesDefaultLocalSource(t *testing.T) {
	zipData := pluginZip(t, "vendor.localinstall", "1.0.0", false)
	reg := testRegistry(t)
	writeLocalMarketSource(t, reg.BaseDir, []market.Plugin{marketTestEntry("vendor.localinstall", "1.0.0", zipData)}, zipData)
	state := newServerState(reg)

	res := httptest.NewRecorder()
	marketInstallHandler(state)(res, marketTestInstallRequest(t, "local", "vendor.localinstall", "", false))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "vendor.localinstall", "plugin.yaml")); err != nil {
		t.Fatalf("插件未安装: %v", err)
	}
}

func TestMarketInstallInstallsPlugin(t *testing.T) {
	zipData := pluginZip(t, "vendor.marketinstall", "1.0.0", false)
	indexServer := marketTestServer(t, []market.Plugin{marketTestEntry("vendor.marketinstall", "1.0.0", zipData)}, zipData)
	reg := marketTestRegistry(t, indexServer.URL)
	state := newServerState(reg)

	res := httptest.NewRecorder()
	marketInstallHandler(state)(res, marketTestInstallRequest(t, "internal", "vendor.marketinstall", "", false))

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "vendor.marketinstall", "plugin.yaml")); err != nil {
		t.Fatalf("插件未安装: %v", err)
	}
	if state.registry().Tools["vendor.marketinstall.tool"] == nil {
		t.Fatal("安装后注册表未刷新")
	}
}

func TestMarketInstallRequiresReplaceForExistingPlugin(t *testing.T) {
	zipData := pluginZip(t, "vendor.marketdupe", "1.1.0", false)
	indexServer := marketTestServer(t, []market.Plugin{marketTestEntry("vendor.marketdupe", "1.1.0", zipData)}, zipData)
	reg := marketTestRegistry(t, indexServer.URL)
	installTestPlugin(t, reg.BaseDir, "vendor.marketdupe", "1.0.0")
	state := newServerState(reg)

	res := httptest.NewRecorder()
	marketInstallHandler(state)(res, marketTestInstallRequest(t, "internal", "vendor.marketdupe", "", false))
	if res.Code != http.StatusConflict {
		t.Fatalf("未确认更新时 status = %d, body = %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	marketInstallHandler(state)(res, marketTestInstallRequest(t, "internal", "vendor.marketdupe", "", true))
	if res.Code != http.StatusOK {
		t.Fatalf("确认更新时 status = %d, body = %s", res.Code, res.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(reg.BaseDir, "plugins", "vendor.marketdupe", "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: 1.1.0") {
		t.Fatalf("插件未更新到市场版本: %s", data)
	}
}

func TestMarketInstallRejectsChecksumMismatch(t *testing.T) {
	zipData := pluginZip(t, "vendor.marketbadsum", "1.0.0", false)
	entry := marketTestEntry("vendor.marketbadsum", "1.0.0", zipData)
	entry.Package.SHA256 = strings.Repeat("0", 64)
	indexServer := marketTestServer(t, []market.Plugin{entry}, zipData)
	reg := marketTestRegistry(t, indexServer.URL)

	res := httptest.NewRecorder()
	marketInstallHandler(newServerState(reg))(res, marketTestInstallRequest(t, "internal", "vendor.marketbadsum", "", false))

	if res.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "vendor.marketbadsum")); !os.IsNotExist(err) {
		t.Fatalf("校验失败后插件不应安装: %v", err)
	}
}

func TestMarketInstallRejectsCrossOriginPackage(t *testing.T) {
	zipData := pluginZip(t, "vendor.marketcross", "1.0.0", false)
	entry := marketTestEntry("vendor.marketcross", "1.0.0", zipData)
	entry.Package.URL = "http://market.example.invalid/packages/cross.zip"
	indexServer := marketTestServer(t, []market.Plugin{entry}, zipData)
	reg := marketTestRegistry(t, indexServer.URL)

	res := httptest.NewRecorder()
	marketInstallHandler(newServerState(reg))(res, marketTestInstallRequest(t, "internal", "vendor.marketcross", "", false))

	if res.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "同源") {
		t.Fatalf("错误信息未说明跨源限制: %s", res.Body.String())
	}
}

func TestMarketInstallRejectsIncompatibleOpsctlVersion(t *testing.T) {
	zipData := pluginZip(t, "vendor.marketincompatible", "1.0.0", false)
	entry := marketTestEntry("vendor.marketincompatible", "1.0.0", zipData)
	entry.Compatibility = market.Compatibility{Opsctl: ">= 99.0.0"}
	indexServer := marketTestServer(t, []market.Plugin{entry}, zipData)
	reg := marketTestRegistry(t, indexServer.URL)
	reg.Root.App.Version = "0.1.0"

	res := httptest.NewRecorder()
	marketInstallHandler(newServerState(reg))(res, marketTestInstallRequest(t, "internal", "vendor.marketincompatible", "", false))

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "vendor.marketincompatible")); !os.IsNotExist(err) {
		t.Fatalf("版本不兼容时插件不应安装: %v", err)
	}
}

func TestMarketInstallRejectsDisabledPluginUpdate(t *testing.T) {
	zipData := pluginZip(t, "vendor.marketdisabledupdate", "1.1.0", false)
	indexServer := marketTestServer(t, []market.Plugin{marketTestEntry("vendor.marketdisabledupdate", "1.1.0", zipData)}, zipData)
	reg := marketTestRegistry(t, indexServer.URL)
	installTestPlugin(t, reg.BaseDir, "vendor.marketdisabledupdate", "1.0.0")
	reg.Root.Plugins.Disabled = []string{"vendor.marketdisabledupdate"}

	res := httptest.NewRecorder()
	marketInstallHandler(newServerState(reg))(res, marketTestInstallRequest(t, "internal", "vendor.marketdisabledupdate", "", true))

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(reg.BaseDir, "plugins", "vendor.marketdisabledupdate", "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: 1.0.0") {
		t.Fatalf("禁用插件不应被更新: %s", data)
	}
}

func TestMarketInstallVerifiesManifestIdentity(t *testing.T) {
	zipData := pluginZip(t, "vendor.marketother", "1.0.0", false)
	indexServer := marketTestServer(t, []market.Plugin{marketTestEntry("vendor.marketexpected", "1.0.0", zipData)}, zipData)
	reg := marketTestRegistry(t, indexServer.URL)

	res := httptest.NewRecorder()
	marketInstallHandler(newServerState(reg))(res, marketTestInstallRequest(t, "internal", "vendor.marketexpected", "", false))

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(filepath.Join(reg.BaseDir, "plugins", "vendor.marketother")); !os.IsNotExist(err) {
		t.Fatalf("清单 ID 不一致时插件不应安装: %v", err)
	}
}

func TestFindMarketPluginSelectsHighestVersion(t *testing.T) {
	index := &market.Index{Plugins: []market.Plugin{
		{ID: "demo.plugin", Version: "1.0.0"},
		{ID: "demo.plugin", Version: "1.10.0"},
		{ID: "demo.plugin", Version: "1.9.0"},
	}}
	item, err := findMarketPlugin(index, "demo.plugin", "")
	if err != nil {
		t.Fatal(err)
	}
	if item.Version != "1.10.0" {
		t.Fatalf("version = %s, want 1.10.0", item.Version)
	}
	item, err = findMarketPlugin(index, "demo.plugin", "1.9.0")
	if err != nil {
		t.Fatal(err)
	}
	if item.Version != "1.9.0" {
		t.Fatalf("version = %s, want 1.9.0", item.Version)
	}
}

func TestMarketPluginViewsCollapseVersions(t *testing.T) {
	index := &market.Index{Plugins: []market.Plugin{
		{ID: "demo.plugin", Name: "Demo", Version: "1.0.0"},
		{ID: "demo.plugin", Name: "Demo", Version: "2.0.0"},
		{ID: "demo.plugin", Name: "Demo", Version: "1.5.0"},
	}}
	views := marketPluginViews(index, map[string]installedMarketPlugin{}, "internal", "内网市场", "0.1.0")
	if len(views) != 1 {
		t.Fatalf("views = %d, want 1", len(views))
	}
	if views[0].Version != "2.0.0" {
		t.Fatalf("version = %s, want 2.0.0", views[0].Version)
	}
}

func marketTestRegistry(t *testing.T, indexURL string) *registry.Registry {
	t.Helper()
	reg := testRegistry(t)
	reg.Root.Market = config.MarketConfig{Sources: []config.MarketSource{{ID: "internal", Name: "内网市场", URL: indexURL + "/index.json"}}}
	return reg
}

func marketTestServer(t *testing.T, entries []market.Plugin, packages ...[]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		data, err := json.Marshal(market.Index{SchemaVersion: market.SchemaVersion, Name: "测试市场", Plugins: entries})
		if err != nil {
			t.Errorf("序列化市场索引失败: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
	for index, data := range packages {
		path := entries[index].Package.URL
		packageData := data
		mux.HandleFunc("/"+strings.TrimPrefix(path, "/"), func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(packageData)
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func writeLocalMarketSource(t *testing.T, baseDir string, entries []market.Plugin, packages ...[]byte) {
	t.Helper()
	marketDir := filepath.Join(baseDir, "market")
	if err := os.MkdirAll(filepath.Join(marketDir, "packages"), 0o755); err != nil {
		t.Fatal(err)
	}
	for index, entry := range entries {
		if index >= len(packages) {
			t.Fatalf("市场插件 %s 缺少测试包", entry.ID)
		}
		path := filepath.Join(marketDir, filepath.FromSlash(entry.Package.URL))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, packages[index], 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(market.Index{SchemaVersion: market.SchemaVersion, Name: "本地测试市场", Plugins: entries})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(marketDir, "index.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func marketTestEntry(id, version string, zipData []byte) market.Plugin {
	sum := sha256.Sum256(zipData)
	return market.Plugin{
		ID:      id,
		Name:    id + " 名称",
		Version: version,
		Package: market.Package{
			URL:    "packages/" + id + "-" + version + ".zip",
			SHA256: hex.EncodeToString(sum[:]),
			Size:   int64(len(zipData)),
		},
	}
}

func marketTestInstallRequest(t *testing.T, sourceID, pluginID, version string, replace bool) *http.Request {
	t.Helper()
	body, err := json.Marshal(marketInstallRequest{SourceID: sourceID, PluginID: pluginID, Version: version, Replace: replace})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/market/install", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

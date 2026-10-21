package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shell_ops/internal/market"
	"shell_ops/internal/marketsource"
	"shell_ops/internal/registry"
)

func TestNexusShareTargetsOnlyExposeReadyNexusSources(t *testing.T) {
	repo := newNexusTestRepo("", "")
	server := httptest.NewServer(repo)
	t.Cleanup(server.Close)
	reg := nexusTestRegistry(t, server.URL, "", "")

	res := httptest.NewRecorder()
	nexusShareTargetsHandler(newServerState(reg))(res, httptest.NewRequest(http.MethodGet, "/api/nexus/share/targets", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("targets status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data nexusShareTargetsResponse `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Targets) != 1 || body.Data.Targets[0].ID != "nexus" {
		t.Fatalf("targets = %+v", body.Data.Targets)
	}
	if body.Data.Targets[0].IndexPath != "index.json" || body.Data.Targets[0].Repository != "plugins" {
		t.Fatalf("target detail = %+v", body.Data.Targets[0])
	}
	if body.Data.Message != "" {
		t.Fatalf("unexpected message = %q", body.Data.Message)
	}
}

func TestNexusShareTargetsEmptyForLocalOnlyMarket(t *testing.T) {
	reg := testRegistry(t)
	if _, err := marketsource.Save(reg.BaseDir, marketsource.Config{Sources: []marketsource.Source{marketsource.DefaultLocalSource()}}); err != nil {
		t.Fatal(err)
	}

	res := httptest.NewRecorder()
	nexusShareTargetsHandler(newServerState(reg))(res, httptest.NewRequest(http.MethodGet, "/api/nexus/share/targets", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("targets status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data nexusShareTargetsResponse `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Targets) != 0 {
		t.Fatalf("local-only market should expose no share target: %+v", body.Data.Targets)
	}
	if body.Data.Message == "" {
		t.Fatal("expected guidance message when no remote market is configured")
	}
}

func TestPluginShareUploadsInstalledPluginToNexus(t *testing.T) {
	repo := newNexusTestRepo("", "")
	server := httptest.NewServer(repo)
	t.Cleanup(server.Close)
	baseReg := nexusTestRegistry(t, server.URL, "", "")
	installTestPluginWithWorkflow(t, baseReg.BaseDir, "vendor.share", "1.2.0")
	reg := loadSharedTestRegistry(t, baseReg)

	handler := NewHandler(reg)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.share/share?source=nexus", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("share status = %d, body = %s", res.Code, res.Body.String())
	}
	var shared struct {
		Status string            `json:"status"`
		Data   pluginShareResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &shared); err != nil {
		t.Fatal(err)
	}
	if shared.Status != "shared" {
		t.Fatalf("share status field = %q", shared.Status)
	}
	if shared.Data.SourceID != "nexus" || shared.Data.PluginID != "vendor.share" || shared.Data.Version != "1.2.0" {
		t.Fatalf("share data = %+v", shared.Data)
	}
	if !shared.Data.IndexUpdated || shared.Data.PackagePath != "packages/vendor.share-1.2.0.zip" {
		t.Fatalf("share data = %+v", shared.Data)
	}
	asset := repo.asset("packages/vendor.share-1.2.0.zip")
	if len(asset) == 0 {
		t.Fatal("shared package should be stored in nexus repository")
	}
	var index market.Index
	if err := json.Unmarshal(repo.asset("index.json"), &index); err != nil {
		t.Fatalf("index json: %v", err)
	}
	if len(index.Plugins) != 1 || index.Plugins[0].ID != "vendor.share" {
		t.Fatalf("index plugins = %+v", index.Plugins)
	}
	if index.Plugins[0].Package.URL != "packages/vendor.share-1.2.0.zip" {
		t.Fatalf("index package url = %q", index.Plugins[0].Package.URL)
	}
}

func TestPluginShareRequiresReplaceForSameVersion(t *testing.T) {
	repo := newNexusTestRepo("", "")
	server := httptest.NewServer(repo)
	t.Cleanup(server.Close)
	baseReg := nexusTestRegistry(t, server.URL, "", "")
	installTestPluginWithWorkflow(t, baseReg.BaseDir, "vendor.shareddupe", "2.0.0")
	reg := loadSharedTestRegistry(t, baseReg)
	handler := NewHandler(reg)

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.shareddupe/share?source=nexus", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("first share status = %d, body = %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.shareddupe/share?source=nexus", nil))
	if res.Code != http.StatusConflict {
		t.Fatalf("second share status = %d, body = %s", res.Code, res.Body.String())
	}
	var conflict struct {
		Data pluginShareResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &conflict); err != nil {
		t.Fatal(err)
	}
	if !conflict.Data.Existing {
		t.Fatalf("conflict data = %+v", conflict.Data)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.shareddupe/share?source=nexus&replace=true", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("replace share status = %d, body = %s", res.Code, res.Body.String())
	}
	var replaced struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &replaced)
	if replaced.Status != "updated" {
		t.Fatalf("replace status = %q", replaced.Status)
	}
}

func TestPluginShareRejectsLocalOnlyMarket(t *testing.T) {
	baseReg := testRegistry(t)
	if _, err := marketsource.Save(baseReg.BaseDir, marketsource.Config{Sources: []marketsource.Source{marketsource.DefaultLocalSource()}}); err != nil {
		t.Fatal(err)
	}
	installTestPluginWithWorkflow(t, baseReg.BaseDir, "vendor.sharelocal", "1.0.0")
	reg := loadSharedTestRegistry(t, baseReg)

	res := httptest.NewRecorder()
	NewHandler(reg).ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.sharelocal/share", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if body := res.Body.String(); !strings.Contains(body, "Nexus") || !strings.Contains(body, "未配置") {
		t.Fatalf("error should explain missing remote market: %s", body)
	}
}

func TestPluginShareUnknownPlugin(t *testing.T) {
	repo := newNexusTestRepo("", "")
	server := httptest.NewServer(repo)
	t.Cleanup(server.Close)
	reg := nexusTestRegistry(t, server.URL, "", "")

	res := httptest.NewRecorder()
	NewHandler(reg).ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/plugins/vendor.missing/share?source=nexus", nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func loadSharedTestRegistry(t *testing.T, base *registry.Registry) *registry.Registry {
	t.Helper()
	reg, err := registry.Load(base.BaseDir)
	if err != nil {
		t.Fatalf("加载测试注册表失败: %v", err)
	}
	return reg
}

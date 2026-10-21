package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"shell_ops/internal/config"
	"shell_ops/internal/market"
	"shell_ops/internal/marketsource"
	"shell_ops/internal/nexus"
	"shell_ops/internal/registry"
)

func TestNexusUploadListDownloadDelete(t *testing.T) {
	repo := newNexusTestRepo("", "")
	server := httptest.NewServer(repo)
	t.Cleanup(server.Close)
	reg := nexusTestRegistry(t, server.URL, "", "")
	state := newServerState(reg)
	zipData := pluginZip(t, "vendor.nexus", "1.0.0", false)

	res := httptest.NewRecorder()
	nexusUploadHandler(state)(res, nexusUploadRequest(t, zipData, false, "nexus"))
	if res.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", res.Code, res.Body.String())
	}
	var uploaded struct {
		Status string            `json:"status"`
		Data   nexusUploadResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	if uploaded.Status != "uploaded" || !uploaded.Data.IndexUpdated {
		t.Fatalf("upload response = %s", res.Body.String())
	}
	if uploaded.Data.PackagePath != "packages/vendor.nexus-1.0.0.zip" {
		t.Fatalf("package path = %q", uploaded.Data.PackagePath)
	}
	if uploaded.Data.PackageURL != "packages/vendor.nexus-1.0.0.zip" {
		t.Fatalf("package url = %q", uploaded.Data.PackageURL)
	}
	sum := sha256.Sum256(zipData)
	if uploaded.Data.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha = %q", uploaded.Data.SHA256)
	}
	if got := repo.asset("packages/vendor.nexus-1.0.0.zip"); !bytes.Equal(got, zipData) {
		t.Fatalf("stored package size = %d", len(got))
	}

	res = httptest.NewRecorder()
	nexusPluginsHandler(state)(res, httptest.NewRequest(http.MethodGet, "/api/nexus/plugins?source=nexus", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", res.Code, res.Body.String())
	}
	var listed struct {
		Data nexusPluginsResponse `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if !listed.Data.Configured || !listed.Data.IndexExists || len(listed.Data.Plugins) != 1 {
		t.Fatalf("list data = %+v", listed.Data)
	}
	if listed.Data.SourceID != "nexus" || listed.Data.Plugins[0].ID != "vendor.nexus" || listed.Data.Plugins[0].InstallStatus != "not_installed" {
		t.Fatalf("plugin view = %+v", listed.Data)
	}

	res = httptest.NewRecorder()
	marketCatalogHandler(state)(res, httptest.NewRequest(http.MethodGet, "/api/market", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("market status = %d, body = %s", res.Code, res.Body.String())
	}
	var catalog struct {
		Data marketCatalogResponse `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Data.Sources) != 1 || catalog.Data.Sources[0].ID != "nexus" || catalog.Data.Sources[0].Status != "ok" {
		t.Fatalf("market sources = %+v", catalog.Data.Sources)
	}
	if catalog.Data.Sources[0].Type != marketsource.TypeNexus {
		t.Fatalf("market source type = %q", catalog.Data.Sources[0].Type)
	}
	if len(catalog.Data.Sources[0].Plugins) != 1 || catalog.Data.Sources[0].Plugins[0].ID != "vendor.nexus" {
		t.Fatalf("market plugins = %+v", catalog.Data.Sources[0].Plugins)
	}

	res = httptest.NewRecorder()
	nexusDownloadHandler(state)(res, httptest.NewRequest(http.MethodGet, "/api/nexus/download?source=nexus&id=vendor.nexus&version=1.0.0", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("download status = %d, body = %s", res.Code, res.Body.String())
	}
	if !bytes.Equal(res.Body.Bytes(), zipData) {
		t.Fatalf("download size = %d, want %d", res.Body.Len(), len(zipData))
	}
	if disposition := res.Header().Get("Content-Disposition"); !strings.Contains(disposition, "vendor.nexus-1.0.0.zip") {
		t.Fatalf("content disposition = %q", disposition)
	}

	res = httptest.NewRecorder()
	nexusDeleteHandler(state)(res, httptest.NewRequest(http.MethodDelete, "/api/nexus/plugin?source=nexus&id=vendor.nexus&version=1.0.0", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", res.Code, res.Body.String())
	}
	var deleted struct {
		Data nexusDeleteResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &deleted); err != nil {
		t.Fatal(err)
	}
	if !deleted.Data.AssetDeleted || deleted.Data.IndexEntries != 0 {
		t.Fatalf("delete data = %+v", deleted.Data)
	}
	if repo.asset("packages/vendor.nexus-1.0.0.zip") != nil {
		t.Fatal("package asset should be deleted")
	}
	var index market.Index
	if err := json.Unmarshal(repo.asset("index.json"), &index); err != nil {
		t.Fatalf("index json: %v", err)
	}
	if len(index.Plugins) != 0 {
		t.Fatalf("index plugins = %d", len(index.Plugins))
	}
}

func TestNexusUploadRequiresReplace(t *testing.T) {
	repo := newNexusTestRepo("", "")
	server := httptest.NewServer(repo)
	t.Cleanup(server.Close)
	state := newServerState(nexusTestRegistry(t, server.URL, "", ""))
	zipData := pluginZip(t, "vendor.nexusdupe", "1.0.0", false)

	res := httptest.NewRecorder()
	nexusUploadHandler(state)(res, nexusUploadRequest(t, zipData, false, "nexus"))
	if res.Code != http.StatusOK {
		t.Fatalf("first upload status = %d, body = %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	nexusUploadHandler(state)(res, nexusUploadRequest(t, zipData, false, "nexus"))
	if res.Code != http.StatusConflict {
		t.Fatalf("second upload status = %d, body = %s", res.Code, res.Body.String())
	}
	var conflict struct {
		Data nexusUploadResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &conflict); err != nil {
		t.Fatal(err)
	}
	if !conflict.Data.Existing {
		t.Fatalf("conflict data = %+v", conflict.Data)
	}

	res = httptest.NewRecorder()
	nexusUploadHandler(state)(res, nexusUploadRequest(t, zipData, true, "nexus"))
	if res.Code != http.StatusOK {
		t.Fatalf("replace upload status = %d, body = %s", res.Code, res.Body.String())
	}
	var replaced struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &replaced)
	if replaced.Status != "replaced" {
		t.Fatalf("status = %q", replaced.Status)
	}
}

func TestNexusRebuildIndexFromPackages(t *testing.T) {
	repo := newNexusTestRepo("", "")
	server := httptest.NewServer(repo)
	t.Cleanup(server.Close)
	state := newServerState(nexusTestRegistry(t, server.URL, "", ""))
	zipData := pluginZip(t, "vendor.nexusrebuild", "2.3.0", false)
	repo.setAsset("packages/vendor.nexusrebuild-2.3.0.zip", zipData)
	repo.setAsset("packages/broken.zip", []byte("not a zip"))

	res := httptest.NewRecorder()
	nexusRebuildHandler(state)(res, httptest.NewRequest(http.MethodPost, "/api/nexus/rebuild?source=nexus", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("rebuild status = %d, body = %s", res.Code, res.Body.String())
	}
	var rebuilt struct {
		Status string             `json:"status"`
		Data   nexusRebuildResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &rebuilt); err != nil {
		t.Fatal(err)
	}
	if rebuilt.Status != "rebuilt" || rebuilt.Data.Scanned != 2 || rebuilt.Data.Indexed != 1 || rebuilt.Data.Skipped != 1 {
		t.Fatalf("rebuild data = %+v", rebuilt.Data)
	}
	if len(rebuilt.Data.Warnings) != 1 || rebuilt.Data.Warnings[0].Path != "packages/broken.zip" {
		t.Fatalf("warnings = %+v", rebuilt.Data.Warnings)
	}
	var index market.Index
	if err := json.Unmarshal(repo.asset("index.json"), &index); err != nil {
		t.Fatalf("index json: %v", err)
	}
	if len(index.Plugins) != 1 || index.Plugins[0].ID != "vendor.nexusrebuild" {
		t.Fatalf("index = %+v", index.Plugins)
	}
	if index.Plugins[0].Package.URL != "packages/vendor.nexusrebuild-2.3.0.zip" {
		t.Fatalf("package url = %q", index.Plugins[0].Package.URL)
	}
}

func TestNexusBasicAuth(t *testing.T) {
	repo := newNexusTestRepo("ops", "secret")
	server := httptest.NewServer(repo)
	t.Cleanup(server.Close)
	reg := nexusTestRegistry(t, server.URL, "ops", "secret")
	state := newServerState(reg)
	zipData := pluginZip(t, "vendor.nexusauth", "1.0.0", false)

	ctx := context.Background()
	test := nexus.NewClient(nexusConfigForMarketSource(mustNexusSource(t, reg))).Test(ctx)
	if !test.Reachable {
		t.Fatalf("authenticated test failed: %+v", test)
	}

	res := httptest.NewRecorder()
	nexusUploadHandler(state)(res, nexusUploadRequest(t, zipData, false, "nexus"))
	if res.Code != http.StatusOK {
		t.Fatalf("authenticated upload status = %d, body = %s", res.Code, res.Body.String())
	}

	badSource := mustNexusSource(t, reg)
	badSource.Password = "wrong"
	bad := nexus.NewClient(nexusConfigForMarketSource(badSource)).Test(ctx)
	if bad.Reachable {
		t.Fatalf("wrong password should not pass: %+v", bad)
	}
}

func TestMarketSourcesMigrateLegacyConfiguration(t *testing.T) {
	reg := testRegistry(t)
	reg.Root.Market.Sources = []config.MarketSource{{ID: "internal", Name: "内网市场", URL: "http://market.example.com/index.json"}}
	if _, err := nexus.SaveConfig(reg.BaseDir, nexus.Config{BaseURL: "http://nexus.example.com", Repository: "plugins"}); err != nil {
		t.Fatal(err)
	}
	sources, err := loadMarketSources(reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0].ID != "internal" || sources[1].ID != "nexus" {
		t.Fatalf("migrated sources = %+v", sources)
	}
	if sources[1].Type != marketsource.TypeNexus || sources[1].URL != "http://nexus.example.com" {
		t.Fatalf("migrated nexus source = %+v", sources[1])
	}
	if _, err := marketsource.Save(reg.BaseDir, marketsource.Config{Sources: []marketsource.Source{marketsource.DefaultLocalSource()}}); err != nil {
		t.Fatal(err)
	}
	sources, err = loadMarketSources(reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].ID != "local" {
		t.Fatalf("unified config should replace legacy sources: %+v", sources)
	}
}

func nexusTestRegistry(t *testing.T, nexusBaseURL, username, password string) *registry.Registry {
	t.Helper()
	reg := testRegistry(t)
	source := marketsource.Source{
		ID:         "nexus",
		Name:       "Nexus 插件仓库",
		Type:       marketsource.TypeNexus,
		URL:        nexusBaseURL,
		Repository: "plugins",
	}
	if username != "" {
		source.AuthType = marketsource.AuthBasic
		source.Username = username
		source.Password = password
	}
	if _, err := marketsource.Save(reg.BaseDir, marketsource.Config{Sources: []marketsource.Source{source}}); err != nil {
		t.Fatalf("save market sources: %v", err)
	}
	return reg
}

func mustNexusSource(t *testing.T, reg *registry.Registry) marketsource.Source {
	t.Helper()
	source, err := nexusSourceByID(reg, "nexus")
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func nexusUploadRequest(t *testing.T, data []byte, replace bool, source string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "plugin.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	query := url.Values{}
	if source != "" {
		query.Set("source", source)
	}
	if replace {
		query.Set("replace", "true")
	}
	target := "/api/nexus/upload"
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

type nexusTestRepo struct {
	mu       sync.Mutex
	assets   map[string][]byte
	username string
	password string
}

func newNexusTestRepo(username, password string) *nexusTestRepo {
	return &nexusTestRepo{assets: map[string][]byte{}, username: username, password: password}
}

func (r *nexusTestRepo) asset(name string) []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.assets[name]
}

func (r *nexusTestRepo) setAsset(name string, data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assets[name] = data
}

func (r *nexusTestRepo) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if r.username != "" {
		username, password, ok := req.BasicAuth()
		if !ok || username != r.username || password != r.password {
			w.Header().Set("WWW-Authenticate", `Basic realm="nexus"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	if req.URL.Path == "/service/rest/v1/search/assets" {
		if req.URL.Query().Get("repository") != "plugins" {
			http.NotFound(w, req)
			return
		}
		r.mu.Lock()
		items := make([]map[string]interface{}, 0, len(r.assets))
		for name, data := range r.assets {
			items = append(items, map[string]interface{}{"path": name, "fileSize": len(data)})
		}
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
		return
	}
	const prefix = "/repository/plugins/"
	if !strings.HasPrefix(req.URL.Path, prefix) {
		http.NotFound(w, req)
		return
	}
	name := strings.TrimPrefix(req.URL.Path, prefix)
	r.mu.Lock()
	defer r.mu.Unlock()
	switch req.Method {
	case http.MethodGet, http.MethodHead:
		data, ok := r.assets[name]
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if req.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	case http.MethodPut:
		data, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.assets[name] = data
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		if _, ok := r.assets[name]; !ok {
			http.NotFound(w, req)
			return
		}
		delete(r.assets, name)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

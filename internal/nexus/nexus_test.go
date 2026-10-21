package nexus

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"shell_ops/internal/market"
)

func TestNormalizeConfigDefaultsAndValidation(t *testing.T) {
	cfg, err := NormalizeConfig(Config{BaseURL: "http://nexus.example.com:8081/", Repository: "ops-plugins"})
	if err != nil {
		t.Fatalf("NormalizeConfig error: %v", err)
	}
	if cfg.BaseURL != "http://nexus.example.com:8081" {
		t.Fatalf("base url = %q", cfg.BaseURL)
	}
	if cfg.SourceID != DefaultSourceID || cfg.SourceName != DefaultSourceName {
		t.Fatalf("source defaults = %q/%q", cfg.SourceID, cfg.SourceName)
	}
	if cfg.IndexPath != "index.json" || cfg.PackageDir != "packages" {
		t.Fatalf("path defaults = %q/%q", cfg.IndexPath, cfg.PackageDir)
	}
	if !cfg.Ready() {
		t.Fatal("config should be ready")
	}
	if got, want := cfg.IndexURL(), "http://nexus.example.com:8081/repository/ops-plugins/index.json"; got != want {
		t.Fatalf("index url = %q, want %q", got, want)
	}

	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{name: "scheme", cfg: Config{BaseURL: "ftp://nexus", Repository: "repo"}},
		{name: "userinfo", cfg: Config{BaseURL: "http://user:pass@nexus", Repository: "repo"}},
		{name: "query", cfg: Config{BaseURL: "http://nexus/?x=1", Repository: "repo"}},
		{name: "repository", cfg: Config{BaseURL: "http://nexus", Repository: "repo name"}},
		{name: "source id", cfg: Config{BaseURL: "http://nexus", Repository: "repo", SourceID: "../bad"}},
		{name: "index escape", cfg: Config{BaseURL: "http://nexus", Repository: "repo", IndexPath: "../index.json"}},
		{name: "package escape", cfg: Config{BaseURL: "http://nexus", Repository: "repo", PackageDir: "a/../b"}},
		{name: "same path", cfg: Config{BaseURL: "http://nexus", Repository: "repo", IndexPath: "packages", PackageDir: "packages"}},
	} {
		if _, err := NormalizeConfig(tc.cfg); err == nil {
			t.Fatalf("%s: expected validation error", tc.name)
		}
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()
	saved, err := SaveConfig(dir, Config{BaseURL: "https://nexus.example.com", Repository: "plugins", SourceName: "内网 Nexus"})
	if err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}
	if !saved.Ready() {
		t.Fatal("saved config should be ready")
	}
	info, err := os.Stat(ConfigPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 && runtime.GOOS != "windows" {
		t.Fatalf("config mode = %o, want private", info.Mode().Perm())
	}
	loaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if loaded.BaseURL != saved.BaseURL || loaded.Repository != saved.Repository || loaded.SourceName != saved.SourceName {
		t.Fatalf("loaded config = %+v, want %+v", loaded, saved)
	}
	if _, err := SaveConfig(dir, Config{}); err == nil {
		t.Fatal("SaveConfig should require base url and repository")
	}
}

func TestAssetPathHelpers(t *testing.T) {
	cases := []struct {
		name      string
		indexPath string
		assetPath string
		wantURL   string
	}{
		{name: "root index", indexPath: "index.json", assetPath: "packages/demo-1.0.0.zip", wantURL: "packages/demo-1.0.0.zip"},
		{name: "nested index same branch", indexPath: "market/index.json", assetPath: "market/packages/demo.zip", wantURL: "packages/demo.zip"},
		{name: "nested index sibling", indexPath: "market/index.json", assetPath: "packages/demo.zip", wantURL: "../packages/demo.zip"},
		{name: "deep index", indexPath: "a/b/index.json", assetPath: "a/c/packages/demo.zip", wantURL: "../c/packages/demo.zip"},
	}
	for _, tc := range cases {
		got := RelativeAssetURL(tc.indexPath, tc.assetPath)
		if got != tc.wantURL {
			t.Fatalf("%s: url = %q, want %q", tc.name, got, tc.wantURL)
		}
		resolved, err := ResolveAssetPath(tc.indexPath, got)
		if err != nil {
			t.Fatalf("%s: resolve error: %v", tc.name, err)
		}
		if resolved != tc.assetPath {
			t.Fatalf("%s: resolved = %q, want %q", tc.name, resolved, tc.assetPath)
		}
	}
	for _, invalid := range []string{"http://evil.example/plugin.zip", "/packages/a.zip", "../../etc/passwd", "packages/a.zip?x=1"} {
		if _, err := ResolveAssetPath("index.json", invalid); err == nil {
			t.Fatalf("ResolveAssetPath(%q) should fail", invalid)
		}
	}
}

func TestRawRepositoryClientRoundTrip(t *testing.T) {
	repo := newRawRepository(t)
	server := httptest.NewServer(repo)
	t.Cleanup(server.Close)
	cfg := Config{BaseURL: server.URL, Repository: "plugins", SourceID: "nexus", SourceName: "Nexus", IndexPath: "index.json", PackageDir: "packages"}
	client := NewClient(cfg)
	ctx := context.Background()

	result := client.Test(ctx)
	if !result.Reachable || result.IndexExists {
		t.Fatalf("test result = %+v", result)
	}

	index := &market.Index{SchemaVersion: market.SchemaVersion, Name: "Nexus", Plugins: []market.Plugin{}}
	if err := client.PutIndex(ctx, index); err != nil {
		t.Fatalf("PutIndex error: %v", err)
	}
	fetched, exists, err := client.FetchIndex(ctx)
	if err != nil || !exists {
		t.Fatalf("FetchIndex = %v, %v", exists, err)
	}
	if len(fetched.Plugins) != 0 {
		t.Fatalf("plugins = %d", len(fetched.Plugins))
	}

	packageData := []byte("fake zip")
	if err := client.UploadPackage(ctx, "packages/demo-1.0.0.zip", packageData); err != nil {
		t.Fatalf("UploadPackage error: %v", err)
	}
	exists, err = client.AssetExists(ctx, "packages/demo-1.0.0.zip")
	if err != nil || !exists {
		t.Fatalf("AssetExists = %v, %v", exists, err)
	}
	downloaded, err := client.DownloadPackage(ctx, "packages/demo-1.0.0.zip")
	if err != nil || !bytes.Equal(downloaded, packageData) {
		t.Fatalf("DownloadPackage = %q, %v", downloaded, err)
	}
	missing, err := client.DownloadPackage(ctx, "packages/missing.zip")
	if err == nil {
		t.Fatalf("DownloadPackage missing = %q", missing)
	}

	repo.addAsset("packages/demo-2.0.0.zip", []byte("zip"))
	repo.addAsset("other/ignored.zip", []byte("zip"))
	assets, err := client.ListPackages(ctx)
	if err != nil {
		t.Fatalf("ListPackages error: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("assets = %+v", assets)
	}
	if assets[0].Path != "packages/demo-1.0.0.zip" || assets[1].Path != "packages/demo-2.0.0.zip" {
		t.Fatalf("assets order = %+v", assets)
	}

	deleted, err := client.DeleteAsset(ctx, "packages/demo-1.0.0.zip")
	if err != nil || !deleted {
		t.Fatalf("DeleteAsset = %v, %v", deleted, err)
	}
	deleted, err = client.DeleteAsset(ctx, "packages/demo-1.0.0.zip")
	if err != nil || deleted {
		t.Fatalf("second DeleteAsset = %v, %v", deleted, err)
	}
	index.Plugins = append(index.Plugins, market.Plugin{
		ID: "demo", Name: "Demo", Version: "2.0.0",
		Package: market.Package{URL: "packages/demo-2.0.0.zip", SHA256: strings.Repeat("a", 64)},
	})
	if err := client.PutIndex(ctx, index); err != nil {
		t.Fatalf("PutIndex with plugin error: %v", err)
	}
	result = client.Test(ctx)
	if !result.Reachable || !result.IndexExists || !result.IndexValid || result.PluginCount != 1 {
		t.Fatalf("test result = %+v", result)
	}
}

func TestListPackagesSurfacesSearchPermissionErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	client := NewClient(Config{BaseURL: server.URL, Repository: "plugins", IndexPath: "index.json", PackageDir: "packages"})
	if _, err := client.ListPackages(context.Background()); err == nil || !strings.Contains(err.Error(), "浏览和搜索权限") {
		t.Fatalf("error = %v", err)
	}
}

func TestPackageFileName(t *testing.T) {
	name, err := PackageFileName("demo.plugin", "1.0.0-beta+1")
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo.plugin-1.0.0-beta_1.zip" {
		t.Fatalf("name = %q", name)
	}
	if _, err := PackageFileName("../bad", "1.0.0"); err == nil {
		t.Fatal("unsafe plugin id should fail")
	}
	if _, err := PackageFileName("demo", "///"); err == nil {
		t.Fatal("unsafe version should fail")
	}
}

type rawRepository struct {
	t      *testing.T
	mu     sync.Mutex
	assets map[string][]byte
}

func newRawRepository(t *testing.T) *rawRepository {
	t.Helper()
	return &rawRepository{t: t, assets: map[string][]byte{}}
}

func (r *rawRepository) addAsset(name string, data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assets[name] = data
}

func (r *rawRepository) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/service/rest/v1/search/assets" {
		r.search(w, req)
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
			r.t.Errorf("read PUT body: %v", err)
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

func (r *rawRepository) search(w http.ResponseWriter, req *http.Request) {
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
}

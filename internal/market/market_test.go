package market

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"shell_ops/internal/config"
)

func TestBuildIndexProducesDeterministicPackages(t *testing.T) {
	baseDir := t.TempDir()
	writeTestPlugin(t, baseDir, "test.plugin", "1.0.0")

	first, err := BuildIndex(BuildOptions{
		BaseDir:   baseDir,
		Plugins:   config.PluginsConfig{Paths: []string{"plugins"}, Strict: true},
		OutputDir: filepath.Join(baseDir, "market-a"),
		Name:      "测试市场",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Plugins != 1 {
		t.Fatalf("plugins = %d, want 1", first.Plugins)
	}
	indexData, err := os.ReadFile(first.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index Index
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatal(err)
	}
	if index.SchemaVersion != SchemaVersion || index.Name != "测试市场" {
		t.Fatalf("unexpected index metadata: %+v", index)
	}
	if len(index.Plugins) != 1 {
		t.Fatalf("plugins = %d, want 1", len(index.Plugins))
	}
	entry := index.Plugins[0]
	if entry.ID != "test.plugin" || entry.Version != "1.0.0" {
		t.Fatalf("unexpected plugin entry: %+v", entry)
	}
	if entry.ToolCount != 1 || len(entry.Tags) != 2 {
		t.Fatalf("unexpected plugin metadata: %+v", entry)
	}
	packagePath := filepath.Join(first.PackagesDir, "test.plugin-1.0.0.zip")
	packageData, err := os.ReadFile(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(packageData)
	if entry.Package.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha256 = %s, want hash of package", entry.Package.SHA256)
	}
	if entry.Package.Size != int64(len(packageData)) {
		t.Fatalf("size = %d, want %d", entry.Package.Size, len(packageData))
	}
	reader, err := zip.OpenReader(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	names := map[string]bool{}
	for _, file := range reader.File {
		names[file.Name] = true
	}
	for _, name := range []string{"test.plugin/plugin.yaml", "test.plugin/scripts/run.sh", "test.plugin/README.md"} {
		if !names[name] {
			t.Fatalf("package missing %s, entries = %v", name, names)
		}
	}

	second, err := BuildIndex(BuildOptions{
		BaseDir:   baseDir,
		Plugins:   config.PluginsConfig{Paths: []string{"plugins"}, Strict: true},
		OutputDir: filepath.Join(baseDir, "market-b"),
	})
	if err != nil {
		t.Fatal(err)
	}
	secondData, err := os.ReadFile(second.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	var secondIndex Index
	if err := json.Unmarshal(secondData, &secondIndex); err != nil {
		t.Fatal(err)
	}
	if secondIndex.Plugins[0].Package.SHA256 != entry.Package.SHA256 {
		t.Fatalf("package hash is not deterministic: %s != %s", secondIndex.Plugins[0].Package.SHA256, entry.Package.SHA256)
	}
}

func TestBuildIndexSkipsLocalUserWorkflows(t *testing.T) {
	baseDir := t.TempDir()
	writeTestPlugin(t, baseDir, "test.plugin", "1.0.0")
	writeLocalUserWorkflowPlugin(t, baseDir)

	result, err := BuildIndex(BuildOptions{
		BaseDir:   baseDir,
		Plugins:   config.PluginsConfig{Paths: []string{"plugins"}, Strict: true},
		OutputDir: filepath.Join(baseDir, "market"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Plugins != 1 {
		t.Fatalf("plugins = %d, want 1", result.Plugins)
	}
	found := false
	for _, warning := range result.Warnings {
		if warning.Code == "MARKET_PLUGIN_SKIPPED" && warning.PluginID == userWorkflowPluginID {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %+v, want user workflow skip warning", result.Warnings)
	}
	if _, err := os.Stat(filepath.Join(result.PackagesDir, userWorkflowPluginID+"-1.0.0.zip")); !os.IsNotExist(err) {
		t.Fatalf("user workflow package should not be published: %v", err)
	}
}

func TestBuildIndexFailsOnInvalidPlugin(t *testing.T) {
	baseDir := t.TempDir()
	pluginDir := filepath.Join(baseDir, "plugins", "bad.plugin")
	if err := os.MkdirAll(filepath.Join(pluginDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `id: bad.plugin
name: Bad Plugin
version: 1.0.0
contributes:
  categories:
    - id: bad
      name: Bad
  tools:
    - id: wrong.prefix.tool
      name: Bad Tool
      category: bad
      command: scripts/run.sh
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "scripts", "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := BuildIndex(BuildOptions{
		BaseDir:   baseDir,
		Plugins:   config.PluginsConfig{Paths: []string{"plugins"}},
		OutputDir: filepath.Join(baseDir, "market"),
	})
	if err == nil || !strings.Contains(err.Error(), "插件校验失败") {
		t.Fatalf("err = %v, want plugin validation failure", err)
	}
}

func TestClientFetchCachesAndDownloads(t *testing.T) {
	packageData := []byte("plugin zip bytes")
	sum := sha256.Sum256(packageData)
	index := Index{
		SchemaVersion: SchemaVersion,
		Name:          "Test Market",
		Plugins: []Plugin{{
			ID:      "test.plugin",
			Name:    "Test Plugin",
			Version: "1.0.0",
			Package: Package{
				URL:    "packages/test.plugin-1.0.0.zip",
				SHA256: hex.EncodeToString(sum[:]),
				Size:   int64(len(packageData)),
			},
		}},
	}
	var indexRequests atomic.Int32
	var packageRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/index.json":
			indexRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(index)
		case "/packages/test.plugin-1.0.0.zip":
			packageRequests.Add(1)
			_, _ = w.Write(packageData)
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	client := NewClient()
	ctx := context.Background()
	loaded, _, err := client.Fetch(ctx, server.URL+"/index.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Plugins[0].ID != "test.plugin" {
		t.Fatalf("unexpected index: %+v", loaded)
	}
	if _, _, err := client.Fetch(ctx, server.URL+"/index.json", false); err != nil {
		t.Fatal(err)
	}
	if indexRequests.Load() != 1 {
		t.Fatalf("index requests = %d, want 1 (cached)", indexRequests.Load())
	}
	if _, _, err := client.Fetch(ctx, server.URL+"/index.json", true); err != nil {
		t.Fatal(err)
	}
	if indexRequests.Load() != 2 {
		t.Fatalf("index requests = %d, want 2 (forced)", indexRequests.Load())
	}

	data, err := client.Download(ctx, server.URL+"/index.json", loaded.Plugins[0].Package)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(packageData) {
		t.Fatalf("downloaded data = %q, want %q", data, packageData)
	}
	if packageRequests.Load() != 1 {
		t.Fatalf("package requests = %d, want 1", packageRequests.Load())
	}
}

func TestClientDownloadRejectsInvalidPackage(t *testing.T) {
	packageData := []byte("plugin zip bytes")
	sum := sha256.Sum256(packageData)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write(packageData)
	}))
	defer server.Close()
	client := NewClient()
	ctx := context.Background()
	valid := Package{URL: "packages/test.zip", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(packageData))}

	badHash := valid
	badHash.SHA256 = strings.Repeat("0", 64)
	if _, err := client.Download(ctx, server.URL+"/index.json", badHash); err == nil || !strings.Contains(err.Error(), "SHA256") {
		t.Fatalf("err = %v, want sha256 failure", err)
	}

	badSize := valid
	badSize.Size = int64(len(packageData) + 1)
	if _, err := client.Download(ctx, server.URL+"/index.json", badSize); err == nil || !strings.Contains(err.Error(), "大小不匹配") {
		t.Fatalf("err = %v, want size failure", err)
	}

	crossOrigin := valid
	crossOrigin.URL = "http://example.invalid/packages/test.zip"
	if _, err := client.Download(ctx, server.URL+"/index.json", crossOrigin); err == nil || !strings.Contains(err.Error(), "同源") {
		t.Fatalf("err = %v, want same-origin failure", err)
	}
}

func TestClientDownloadRejectsCrossOriginRedirect(t *testing.T) {
	packageData := []byte("plugin zip bytes")
	sum := sha256.Sum256(packageData)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write(packageData)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, target.URL+"/package.zip", http.StatusFound)
	}))
	defer source.Close()
	client := NewClient()
	pkg := Package{URL: "packages/test.zip", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(packageData))}

	if _, err := client.Download(context.Background(), source.URL+"/index.json", pkg); err == nil || !strings.Contains(err.Error(), "非同源") {
		t.Fatalf("err = %v, want cross-origin redirect failure", err)
	}
}

func TestIndexValidate(t *testing.T) {
	valid := Index{
		SchemaVersion: SchemaVersion,
		Plugins: []Plugin{{
			ID:      "test.plugin",
			Name:    "Test",
			Version: "1.0.0",
			Package: Package{URL: "packages/test.zip", SHA256: strings.Repeat("a", 64)},
		}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid index rejected: %v", err)
	}
	multiVersion := Index{SchemaVersion: SchemaVersion, Plugins: []Plugin{
		{ID: "test.plugin", Name: "Test", Version: "1.0.0", Package: Package{URL: "packages/a.zip", SHA256: strings.Repeat("a", 64)}},
		{ID: "test.plugin", Name: "Test", Version: "1.1.0", Package: Package{URL: "packages/b.zip", SHA256: strings.Repeat("b", 64)}},
	}}
	if err := multiVersion.Validate(); err != nil {
		t.Fatalf("multi-version index rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*Index)
	}{
		{"schema", func(index *Index) { index.SchemaVersion = 2 }},
		{"unsafe id", func(index *Index) { index.Plugins[0].ID = "../escape" }},
		{"duplicate version", func(index *Index) { index.Plugins = append(index.Plugins, index.Plugins[0]) }},
		{"missing name", func(index *Index) { index.Plugins[0].Name = "" }},
		{"missing version", func(index *Index) { index.Plugins[0].Version = "" }},
		{"missing url", func(index *Index) { index.Plugins[0].Package.URL = "" }},
		{"bad hash", func(index *Index) { index.Plugins[0].Package.SHA256 = "abc" }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			index := Index{
				SchemaVersion: valid.SchemaVersion,
				Plugins:       append([]Plugin(nil), valid.Plugins...),
			}
			testCase.mutate(&index)
			if err := index.Validate(); err == nil {
				t.Fatal("invalid index accepted")
			}
		})
	}
}

func TestResolveSource(t *testing.T) {
	baseDir := t.TempDir()
	marketDir := filepath.Join(baseDir, "market")
	if err := os.MkdirAll(marketDir, 0o755); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(marketDir, "index.json")
	if err := os.WriteFile(indexPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	remote := []string{"http://plugins.internal/index.json", "https://plugins.internal/ops/index.json"}
	for _, value := range remote {
		source, err := ResolveSource(value, baseDir)
		if err != nil {
			t.Fatalf("valid remote source %q rejected: %v", value, err)
		}
		if source.Kind != SourceRemote || source.Location != value {
			t.Fatalf("remote source = %+v, want %q", source, value)
		}
	}

	fileURL := "file:///" + strings.TrimPrefix(filepath.ToSlash(indexPath), "/")
	local := []struct {
		raw  string
		want string
	}{
		{"market", indexPath},
		{marketDir, indexPath},
		{indexPath, indexPath},
		{fileURL, indexPath},
	}
	for _, testCase := range local {
		source, err := ResolveSource(testCase.raw, baseDir)
		if err != nil {
			t.Fatalf("valid local source %q rejected: %v", testCase.raw, err)
		}
		if source.Kind != SourceLocal || source.Location != testCase.want {
			t.Fatalf("local source = %+v, want %q", source, testCase.want)
		}
	}

	invalid := []string{"", "ftp://plugins.internal/index.json", "http:///index.json", "http://user:pass@plugins.internal/index.json", "file://otherhost/tmp/index.json"}
	for _, value := range invalid {
		if _, err := ResolveSource(value, baseDir); err == nil {
			t.Fatalf("invalid source %q accepted", value)
		}
	}
}

func TestClientReadsLocalSource(t *testing.T) {
	baseDir := t.TempDir()
	marketDir := filepath.Join(baseDir, "market")
	packagesDir := filepath.Join(marketDir, "packages")
	if err := os.MkdirAll(packagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	packageData := []byte("local plugin zip bytes")
	sum := sha256.Sum256(packageData)
	packageName := "test.plugin-1.0.0.zip"
	if err := os.WriteFile(filepath.Join(packagesDir, packageName), packageData, 0o644); err != nil {
		t.Fatal(err)
	}
	index := Index{
		SchemaVersion: SchemaVersion,
		Name:          "Local Market",
		Plugins: []Plugin{{
			ID:      "test.plugin",
			Name:    "Test Plugin",
			Version: "1.0.0",
			Package: Package{
				URL:    "packages/" + packageName,
				SHA256: hex.EncodeToString(sum[:]),
				Size:   int64(len(packageData)),
			},
		}},
	}
	writeLocalMarketIndex(t, marketDir, index)

	client := NewClient()
	client.BaseDir = baseDir
	ctx := context.Background()
	loaded, fetchedAt, err := client.Fetch(ctx, "market", false)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Local Market" || len(loaded.Plugins) != 1 {
		t.Fatalf("unexpected local index: %+v", loaded)
	}
	if fetchedAt.IsZero() {
		t.Fatal("本地市场源缺少 fetched_at")
	}
	data, err := client.Download(ctx, "market", loaded.Plugins[0].Package)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(packageData) {
		t.Fatalf("downloaded data = %q, want %q", data, packageData)
	}

	index.Name = "Updated Local Market"
	writeLocalMarketIndex(t, marketDir, index)
	loaded, _, err = client.Fetch(ctx, "market", false)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Updated Local Market" {
		t.Fatalf("local index was cached: name = %q", loaded.Name)
	}

	fileURL := "file:///" + strings.TrimPrefix(filepath.ToSlash(filepath.Join(marketDir, "index.json")), "/")
	if _, _, err := client.Fetch(ctx, fileURL, false); err != nil {
		t.Fatalf("file:// source rejected: %v", err)
	}

	traversal := loaded.Plugins[0].Package
	traversal.URL = "../outside.zip"
	if _, err := client.Download(ctx, "market", traversal); err == nil || !strings.Contains(err.Error(), "超出市场目录") {
		t.Fatalf("err = %v, want path traversal failure", err)
	}

	absolute := loaded.Plugins[0].Package
	absolute.URL = filepath.ToSlash(filepath.Join(packagesDir, packageName))
	if _, err := client.Download(ctx, "market", absolute); err == nil || !strings.Contains(err.Error(), "相对路径") {
		t.Fatalf("err = %v, want absolute path rejection", err)
	}

	badHash := loaded.Plugins[0].Package
	badHash.SHA256 = strings.Repeat("0", 64)
	if _, err := client.Download(ctx, "market", badHash); err == nil || !strings.Contains(err.Error(), "SHA256") {
		t.Fatalf("err = %v, want sha256 failure", err)
	}

	badSize := loaded.Plugins[0].Package
	badSize.Size = int64(len(packageData) + 1)
	if _, err := client.Download(ctx, "market", badSize); err == nil || !strings.Contains(err.Error(), "大小不匹配") {
		t.Fatalf("err = %v, want size failure", err)
	}
}

func TestClientReportsMissingLocalIndex(t *testing.T) {
	client := NewClient()
	client.BaseDir = t.TempDir()
	_, _, err := client.Fetch(context.Background(), "market", false)
	if err == nil || !strings.Contains(err.Error(), "本地市场索引不存在") || !strings.Contains(err.Error(), "opsctl market index") {
		t.Fatalf("err = %v, want missing local index hint", err)
	}
}

func writeLocalMarketIndex(t *testing.T, marketDir string, index Index) {
	t.Helper()
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(marketDir, "index.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeLocalUserWorkflowPlugin(t *testing.T, baseDir string) {
	t.Helper()
	pluginDir := filepath.Join(baseDir, "plugins", userWorkflowPluginID)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `id: ` + userWorkflowPluginID + `
name: 用户工作流
version: 1.0.0
description: Web 页面创建和维护的用户工作流集合
contributes: {}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestPlugin(t *testing.T, baseDir, id, version string) {
	t.Helper()
	pluginDir := filepath.Join(baseDir, "plugins", id)
	if err := os.MkdirAll(filepath.Join(pluginDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `id: ` + id + `
name: Test Plugin
version: ` + version + `
description: Test plugin
author: tester
compatibility:
  opsctl: ">=0.1.0"
contributes:
  categories:
    - id: test
      name: Test
  tools:
    - id: ` + id + `.run
      name: Run
      category: test
      description: Run test
      command: scripts/run.sh
      workdir: .
      tags: [alpha, beta]
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "scripts", "run.sh"), []byte("#!/bin/sh\necho test\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "README.md"), []byte("# Test Plugin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

package marketsource

import (
	"strings"
	"testing"
)

func TestNormalizeSourceTypes(t *testing.T) {
	remote, err := NormalizeSource(Source{ID: "internal", URL: "https://plugins.example.com/index.json"})
	if err != nil {
		t.Fatal(err)
	}
	if remote.Type != TypeRemote || remote.Name != "远程插件市场" || remote.AuthType != AuthAnonymous {
		t.Fatalf("remote source = %+v", remote)
	}

	local, err := NormalizeSource(Source{ID: "local", Type: TypeLocal, URL: "market", AuthType: AuthBasic, Username: "ops", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if local.Username != "" || local.Password != "" || local.AuthType != AuthAnonymous {
		t.Fatalf("local source should clear credentials: %+v", local)
	}

	nexusSource, err := NormalizeSource(Source{
		ID:         "nexus",
		Type:       TypeNexus,
		URL:        "http://nexus.example.com/",
		Repository: "plugins",
		AuthType:   AuthBasic,
		Username:   "ops",
		Password:   "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if nexusSource.URL != "http://nexus.example.com" || nexusSource.IndexPath != DefaultIndexPath || nexusSource.PackageDir != DefaultPackageDir {
		t.Fatalf("nexus source = %+v", nexusSource)
	}
	if !nexusSource.Ready() || !nexusSource.IsBasicAuth() {
		t.Fatalf("nexus source should be ready with auth: %+v", nexusSource)
	}
}

func TestNormalizeSourceRejectsInvalidConfig(t *testing.T) {
	cases := []struct {
		name   string
		source Source
		want   string
	}{
		{name: "unknown type", source: Source{ID: "demo", Type: "git", URL: "market"}, want: "不支持的市场来源类型"},
		{name: "basic without user", source: Source{ID: "demo", Type: TypeRemote, URL: "https://example.com/index.json", AuthType: AuthBasic}, want: "用户名"},
		{name: "local with http", source: Source{ID: "demo", Type: TypeLocal, URL: "https://example.com/index.json"}, want: "不能是 HTTP"},
		{name: "nexus without repo", source: Source{ID: "demo", Type: TypeNexus, URL: "https://example.com"}, want: "仓库名"},
		{name: "nexus path traversal", source: Source{ID: "demo", Type: TypeNexus, URL: "https://example.com", Repository: "plugins", IndexPath: "../index.json"}, want: "路径片段"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := NormalizeSource(test.source)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()
	saved, err := Save(dir, Config{Sources: []Source{
		DefaultLocalSource(),
		{ID: "nexus", Type: TypeNexus, URL: "http://nexus.example.com", Repository: "plugins", AuthType: AuthBasic, Username: "ops", Password: "secret"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Sources) != 2 {
		t.Fatalf("saved sources = %+v", saved.Sources)
	}
	loaded, exists, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || len(loaded.Sources) != 2 {
		t.Fatalf("exists = %v sources = %+v", exists, loaded.Sources)
	}
	if loaded.Sources[1].Password != "secret" || !loaded.Sources[1].IsBasicAuth() {
		t.Fatalf("password not persisted: %+v", loaded.Sources[1])
	}
	if loaded.Sources[0].AuthType != AuthAnonymous {
		t.Fatalf("local source auth = %q", loaded.Sources[0].AuthType)
	}

	missing, exists, err := Load(t.TempDir())
	if err != nil || exists || len(missing.Sources) != 0 {
		t.Fatalf("missing config = %+v exists=%v err=%v", missing, exists, err)
	}
}

func TestNormalizeConfigRejectsDuplicateIDs(t *testing.T) {
	_, err := NormalizeConfig(Config{Sources: []Source{
		DefaultLocalSource(),
		DefaultLocalSource(),
	}})
	if err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("error = %v", err)
	}
}

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"shell_ops/internal/market"
	"shell_ops/internal/marketsource"
)

func TestMarketSourceHandlersCRUD(t *testing.T) {
	reg := testRegistry(t)
	state := newServerState(reg)

	res := httptest.NewRecorder()
	marketSourcesHandler(state)(res, httptest.NewRequest(http.MethodGet, "/api/market/sources", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("initial list status = %d, body = %s", res.Code, res.Body.String())
	}
	var listed struct {
		Data marketSourcesResponse `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data.Sources) != 1 || listed.Data.Sources[0].ID != "local" || listed.Data.Sources[0].Type != marketsource.TypeLocal {
		t.Fatalf("initial sources = %+v", listed.Data.Sources)
	}

	res = httptest.NewRecorder()
	marketSourcesHandler(state)(res, marketSourceRequest(t, http.MethodPost, map[string]interface{}{
		"id":          "nexus",
		"name":        "Nexus 插件仓库",
		"type":        "nexus",
		"url":         "http://nexus.example.com",
		"enabled":     true,
		"auth_type":   "basic",
		"username":    "ops",
		"password":    "secret-token",
		"repository":  "plugins",
		"index_path":  "index.json",
		"package_dir": "packages",
	}))
	if res.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "secret-token") {
		t.Fatalf("response leaked password: %s", res.Body.String())
	}
	var created struct {
		Data marketSourceConfigView `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.ID != "nexus" || !created.Data.PasswordSet || !created.Data.Ready || created.Data.AuthType != marketsource.AuthBasic {
		t.Fatalf("created view = %+v", created.Data)
	}
	if created.Data.IndexURL != "http://nexus.example.com/repository/plugins/index.json" {
		t.Fatalf("index url = %q", created.Data.IndexURL)
	}

	res = httptest.NewRecorder()
	marketSourcesHandler(state)(res, marketSourceRequest(t, http.MethodPost, map[string]interface{}{"id": "nexus", "type": "local", "url": "market"}))
	if res.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, body = %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	marketSourcesHandler(state)(res, marketSourceRequest(t, http.MethodPut, map[string]interface{}{
		"id":          "nexus",
		"name":        "Nexus 主仓库",
		"type":        "nexus",
		"url":         "http://nexus.example.com",
		"enabled":     true,
		"auth_type":   "basic",
		"username":    "ops",
		"password":    "",
		"repository":  "plugins",
		"index_path":  "index.json",
		"package_dir": "packages",
	}))
	if res.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", res.Code, res.Body.String())
	}
	cfg, exists, err := marketsource.Load(reg.BaseDir)
	if err != nil || !exists {
		t.Fatalf("load saved config: exists=%v err=%v", exists, err)
	}
	if len(cfg.Sources) != 2 || cfg.Sources[1].Password != "secret-token" {
		t.Fatalf("password should be preserved on update: %+v", cfg.Sources)
	}
	var updated struct {
		Data marketSourceConfigView `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Data.Name != "Nexus 主仓库" || !updated.Data.PasswordSet {
		t.Fatalf("updated view = %+v", updated.Data)
	}

	res = httptest.NewRecorder()
	marketSourcesHandler(state)(res, marketSourceRequest(t, http.MethodPut, map[string]interface{}{
		"id":          "nexus",
		"name":        "Nexus 主仓库",
		"type":        "nexus",
		"url":         "http://nexus.example.com",
		"enabled":     true,
		"auth_type":   "anonymous",
		"repository":  "plugins",
		"index_path":  "index.json",
		"package_dir": "packages",
	}))
	if res.Code != http.StatusOK {
		t.Fatalf("switch auth status = %d, body = %s", res.Code, res.Body.String())
	}
	cfg, _, err = marketsource.Load(reg.BaseDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sources[1].Password != "" || cfg.Sources[1].Username != "" {
		t.Fatalf("anonymous source should clear credentials: %+v", cfg.Sources[1])
	}

	res = httptest.NewRecorder()
	marketSourcesHandler(state)(res, httptest.NewRequest(http.MethodDelete, "/api/market/sources?id=nexus", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(marketsource.ConfigPath(reg.BaseDir)); err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	res = httptest.NewRecorder()
	marketSourcesHandler(state)(res, httptest.NewRequest(http.MethodDelete, "/api/market/sources?id=nexus", nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("missing delete status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestMarketSourceTestHandlerRemoteBasicAuth(t *testing.T) {
	const username, password = "market", "secret"
	var indexBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		user, pass, ok := req.BasicAuth()
		if !ok || user != username || pass != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="market"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if req.URL.Path != "/index.json" {
			http.NotFound(w, req)
			return
		}
		_, _ = w.Write(indexBody)
	}))
	t.Cleanup(server.Close)
	data, err := json.Marshal(market.Index{SchemaVersion: market.SchemaVersion, Plugins: []market.Plugin{}})
	if err != nil {
		t.Fatal(err)
	}
	indexBody = data

	reg := testRegistry(t)
	state := newServerState(reg)
	payload := map[string]interface{}{
		"id":        "internal",
		"name":      "内网市场",
		"type":      "remote",
		"url":       server.URL + "/index.json",
		"enabled":   true,
		"auth_type": "basic",
		"username":  username,
		"password":  password,
	}

	res := httptest.NewRecorder()
	marketSourceTestHandler(state)(res, marketSourceRequest(t, http.MethodPost, payload))
	if res.Code != http.StatusOK {
		t.Fatalf("test status = %d, body = %s", res.Code, res.Body.String())
	}
	var result struct {
		Data marketSourceTestResult `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Data.Reachable || result.Data.PluginCount != 0 {
		t.Fatalf("test result = %+v", result.Data)
	}

	payload["password"] = "wrong"
	res = httptest.NewRecorder()
	marketSourceTestHandler(state)(res, marketSourceRequest(t, http.MethodPost, payload))
	if res.Code != http.StatusOK {
		t.Fatalf("wrong password test status = %d, body = %s", res.Code, res.Body.String())
	}
	result.Data = marketSourceTestResult{}
	if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Reachable {
		t.Fatalf("wrong password should not be reachable: %+v", result.Data)
	}
}

func TestMarketSourceHandlerValidatesType(t *testing.T) {
	state := newServerState(testRegistry(t))
	res := httptest.NewRecorder()
	marketSourcesHandler(state)(res, marketSourceRequest(t, http.MethodPost, map[string]interface{}{
		"id":   "bad",
		"type": "unknown",
		"url":  "market",
	}))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "不支持的市场来源类型") {
		t.Fatalf("unexpected error: %s", res.Body.String())
	}
}

func marketSourceRequest(t *testing.T, method string, payload interface{}) *http.Request {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, "/api/market/sources", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

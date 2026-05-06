package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"shell_ops/internal/registry"
)

func TestGlobalConfigGetAPI(t *testing.T) {
	baseDir := t.TempDir()
	configPath := filepath.Join(baseDir, "configs", "ops.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `name: 测试控制台
description: 测试描述
plugins:
  paths:
    - plugins
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := registry.Load(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(reg)
	req := httptest.NewRequest(http.MethodGet, "/api/config/global", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be a map")
	}

	gotContent, ok := data["content"].(string)
	if !ok {
		t.Fatal("expected content to be a string")
	}

	if gotContent != content {
		t.Errorf("expected content %q, got %q", content, gotContent)
	}

	gotPath, ok := data["path"].(string)
	if !ok {
		t.Fatal("expected path to be a string")
	}

	if gotPath != "configs/ops.yaml" {
		t.Errorf("expected path %q, got %q", "configs/ops.yaml", gotPath)
	}
}

func TestGlobalConfigPutAPI(t *testing.T) {
	baseDir := t.TempDir()
	configPath := filepath.Join(baseDir, "configs", "ops.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	initialContent := `name: 初始控制台
description: 初始描述
plugins:
  paths:
    - plugins
`
	if err := os.WriteFile(configPath, []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := registry.Load(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(reg)

	newContent := `name: 更新后的控制台
description: 更新后的描述
plugins:
  paths:
    - plugins
    - custom-plugins
`
	reqBody := map[string]string{"content": newContent}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/api/config/global", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if resp.Status != "saved" {
		t.Errorf("expected status 'saved', got %q", resp.Status)
	}

	savedContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(savedContent) != newContent {
		t.Errorf("expected saved content %q, got %q", newContent, string(savedContent))
	}
}

func TestGlobalConfigPutRejectsInvalidYAML(t *testing.T) {
	baseDir := t.TempDir()
	configPath := filepath.Join(baseDir, "configs", "ops.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	initialContent := `name: 初始控制台
description: 初始描述
plugins:
  paths:
    - plugins
`
	if err := os.WriteFile(configPath, []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := registry.Load(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(reg)

	invalidContent := `name: 无效配置
description: [无效的 YAML 语法
`
	reqBody := map[string]string{"content": invalidContent}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/api/config/global", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}

	savedContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(savedContent) != initialContent {
		t.Errorf("expected original content to be preserved, got %q", string(savedContent))
	}
}

func TestGlobalConfigRejectsNonGetPut(t *testing.T) {
	baseDir := t.TempDir()
	configPath := filepath.Join(baseDir, "configs", "ops.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `name: 测试控制台
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := registry.Load(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(reg)

	methods := []string{http.MethodPost, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/api/config/global", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected status 405, got %d", method, rec.Code)
		}
	}
}

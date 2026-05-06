package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

type pluginHostConfigResult struct {
	PluginID string `json:"plugin_id"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Exists   bool   `json:"exists"`
}

type pluginHostConfigSaveRequest struct {
	Content string `json:"content"`
}

func handlePluginConfig(w http.ResponseWriter, req *http.Request, state *serverState, pluginID string) {
	pluginID = strings.Trim(pluginID, "/")
	switch req.Method {
	case http.MethodGet:
		result, err := readPluginHostConfig(state, pluginID)
		writePluginHostConfigResponse(w, result, err, "读取插件配置失败", "loaded")
	case http.MethodPut:
		result, err := updatePluginHostConfig(state, req, pluginID)
		writePluginHostConfigResponse(w, result, err, "保存插件配置失败", "saved")
	default:
		methodNotAllowed(w)
	}
}

func writePluginHostConfigResponse(w http.ResponseWriter, result pluginHostConfigResult, err error, fallback, status string) {
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errPluginNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, response{Error: fmt.Sprintf("%s: %s", fallback, err.Error()), Data: result})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: status, Data: result})
}

func readPluginHostConfig(state *serverState, pluginID string) (pluginHostConfigResult, error) {
	reg := state.registry()
	result, path, err := pluginHostConfigTarget(reg, pluginID)
	if err != nil {
		return result, err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		result.Content = string(data)
		result.Exists = true
		return result, nil
	}
	if os.IsNotExist(err) {
		result.Content = ""
		result.Exists = false
		return result, nil
	}
	return result, err
}

func updatePluginHostConfig(state *serverState, req *http.Request, pluginID string) (pluginHostConfigResult, error) {
	defer req.Body.Close()
	state.mu.Lock()
	defer state.mu.Unlock()
	reg := state.reg
	result, path, err := pluginHostConfigTarget(reg, pluginID)
	if err != nil {
		return result, err
	}

	var body pluginHostConfigSaveRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return result, fmt.Errorf("解析请求失败: %w", err)
	}
	result.Content = body.Content
	if err := validatePluginHostConfigContent(body.Content); err != nil {
		return result, err
	}

	oldContent, readErr := os.ReadFile(path)
	existed := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return result, readErr
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return result, err
	}
	if err := os.WriteFile(path, []byte(body.Content), 0o644); err != nil {
		return result, err
	}
	newReg, err := registry.Load(reg.BaseDir)
	if err != nil {
		if existed {
			_ = os.WriteFile(path, oldContent, 0o644)
		} else {
			_ = os.Remove(path)
		}
		return result, fmt.Errorf("刷新插件注册表失败，已回滚配置文件: %w", err)
	}
	state.reg = newReg
	result.Exists = true
	return result, nil
}

func pluginHostConfigTarget(reg *registry.Registry, pluginID string) (pluginHostConfigResult, string, error) {
	result := pluginHostConfigResult{PluginID: pluginID}
	if !config.SafePluginConfigID(pluginID) {
		return result, "", fmt.Errorf("插件 ID 包含不安全路径字符")
	}
	pkg, ok := registeredPlugin(reg, pluginID)
	if !ok {
		pkg, ok = installedAnyPlugin(reg, pluginID)
	}
	if !ok {
		pkg, ok = disabledPluginCandidate(reg, pluginID)
	}
	if !ok {
		return result, "", fmt.Errorf("%w: %s", errPluginNotFound, pluginID)
	}
	if err := ensurePluginDirInConfiguredRoot(reg, pkg.Dir); err != nil {
		return result, "", err
	}
	path, err := config.PluginHostConfigPath(reg.BaseDir, pkg.Manifest.ID)
	if err != nil {
		return result, "", err
	}
	result.PluginID = pkg.Manifest.ID
	result.Path = relativePath(reg.BaseDir, path)
	return result, path, nil
}

func validatePluginHostConfigContent(content string) error {
	tmpDir, err := os.MkdirTemp("", "ops-plugin-config-validate-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	if _, err := config.LoadOptionalValues(path); err != nil {
		return fmt.Errorf("YAML 语法错误: %w", err)
	}
	return nil
}

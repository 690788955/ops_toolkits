package server

// 本文件是 catalog、UI 偏好与全局配置相关 handler。
import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

func catalogHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, response{Data: buildCatalog(state.registry())})
	}
}

func uiPreferencesHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, response{Data: uiPreferencesFromConfig(state.registry().Root.UI)})
	}
}

func uiPreferencesFromConfig(ui config.UIConfig) map[string]interface{} {
	return map[string]interface{}{
		"log_font_size": uiLogFontSize(ui.LogFontSize),
	}
}

func uiLogFontSize(value int) int {
	const (
		defaultLogFontSize = 14
		minLogFontSize     = 12
		maxLogFontSize     = 20
	)
	if value < minLogFontSize || value > maxLogFontSize {
		return defaultLogFontSize
	}
	return value
}

func globalConfigHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reg := state.registry()
		if req.Method == http.MethodGet {
			handleGlobalConfigGet(w, reg)
			return
		}
		if req.Method == http.MethodPut {
			handleGlobalConfigPut(w, req, state)
			return
		}
		methodNotAllowed(w)
	}
}

func handleGlobalConfigGet(w http.ResponseWriter, reg *registry.Registry) {
	path := config.RootPath(reg.BaseDir)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, response{Error: "框架设置文件不存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{
		"content": string(content),
		"path":    relativePath(reg.BaseDir, path),
	}})
}

func handleGlobalConfigPut(w http.ResponseWriter, req *http.Request, state *serverState) {
	defer req.Body.Close()
	state.mu.Lock()
	defer state.mu.Unlock()
	reg := state.reg
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	// 先验证 YAML 是否有效
	var testRoot config.RootConfig
	if err := yaml.Unmarshal([]byte(body.Content), &testRoot); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: fmt.Sprintf("YAML 格式无效: %v", err)})
		return
	}

	path := config.RootPath(reg.BaseDir)
	oldContent, readErr := os.ReadFile(path)
	existed := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		writeJSON(w, http.StatusInternalServerError, response{Error: readErr.Error()})
		return
	}
	if err := os.WriteFile(path, []byte(body.Content), 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	newReg, err := registry.Load(reg.BaseDir)
	if err != nil {
		if existed {
			_ = os.WriteFile(path, oldContent, 0o644)
		} else {
			_ = os.Remove(path)
		}
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("重新加载注册表失败，已回滚配置文件: %v", err)})
		return
	}
	state.reg = newReg
	writeJSON(w, http.StatusOK, response{Status: "saved", Data: map[string]string{"message": "框架设置已保存"}})
}

func globalEnvConfigHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reg := state.registry()
		if req.Method == http.MethodGet {
			handleGlobalEnvConfigGet(w, reg)
			return
		}
		if req.Method == http.MethodPut {
			handleGlobalEnvConfigPut(w, req, state)
			return
		}
		methodNotAllowed(w)
	}
}

func handleGlobalEnvConfigGet(w http.ResponseWriter, reg *registry.Registry) {
	path := config.GlobalEnvPath(reg.BaseDir)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{
				"content": "",
				"path":    relativePath(reg.BaseDir, path),
			}})
			return
		}
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{
		"content": string(content),
		"path":    relativePath(reg.BaseDir, path),
	}})
}

func handleGlobalEnvConfigPut(w http.ResponseWriter, req *http.Request, state *serverState) {
	defer req.Body.Close()
	state.mu.Lock()
	defer state.mu.Unlock()
	reg := state.reg
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	path := config.GlobalEnvPath(reg.BaseDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	oldContent, readErr := os.ReadFile(path)
	existed := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		writeJSON(w, http.StatusInternalServerError, response{Error: readErr.Error()})
		return
	}
	if err := os.WriteFile(path, []byte(body.Content), 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	newReg, err := registry.Load(reg.BaseDir)
	if err != nil {
		if existed {
			_ = os.WriteFile(path, oldContent, 0o644)
		} else {
			_ = os.Remove(path)
		}
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("重新加载注册表失败，已回滚配置文件: %v", err)})
		return
	}
	state.reg = newReg
	writeJSON(w, http.StatusOK, response{Status: "saved", Data: map[string]string{"message": "全局环境配置已保存"}})
}

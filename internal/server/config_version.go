package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

// 版本元数据结构
type configVersionMeta struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   string    `json:"created_by"`
	Default     bool      `json:"default"`
}

type configVersionMetaFile struct {
	Versions []configVersionMeta `json:"versions"`
}

type versionListResult struct {
	ConfigID string              `json:"config_id"`
	MetaPath string              `json:"meta_path"`
	Versions []configVersionMeta `json:"versions"`
}

type versionSaveRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type versionContentResult struct {
	ConfigID string `json:"config_id"`
	Version  string `json:"version"`
	Content  string `json:"content"`
	Exists   bool   `json:"exists"`
}

var versionNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// 处理版本列表请求
func handleConfigVersions(w http.ResponseWriter, req *http.Request, state *serverState, configType, configID string) {
	switch req.Method {
	case http.MethodGet:
		result, err := listConfigVersions(state, configType, configID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "loaded", Data: result})
	case http.MethodPost:
		result, err := saveConfigVersion(state, req, configType, configID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "saved", Data: result})
	default:
		methodNotAllowed(w)
	}
}

// 处理单个版本请求
func handleConfigVersion(w http.ResponseWriter, req *http.Request, state *serverState, configType, configID, version string) {
	switch req.Method {
	case http.MethodGet:
		result, err := getConfigVersionContent(state, configType, configID, version)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "loaded", Data: result})
	case http.MethodDelete:
		err := deleteConfigVersion(state, configType, configID, version)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "deleted"})
	default:
		methodNotAllowed(w)
	}
}

// 处理设置默认版本请求
func handleSetDefaultVersion(w http.ResponseWriter, req *http.Request, state *serverState, configType, configID, version string) {
	if req.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}
	err := setDefaultConfigVersion(state, configType, configID, version)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "updated"})
}

// 列出所有版本
func listConfigVersions(state *serverState, configType, configID string) (versionListResult, error) {
	reg := state.registry()
	metaPath, err := getConfigMetaPath(reg, configType, configID)
	if err != nil {
		return versionListResult{}, err
	}

	meta, err := loadVersionMeta(metaPath)
	if err != nil {
		return versionListResult{}, err
	}

	return versionListResult{
		ConfigID: configID,
		MetaPath: relativePath(reg.BaseDir, metaPath),
		Versions: meta.Versions,
	}, nil
}

// 保存当前配置为新版本
func saveConfigVersion(state *serverState, req *http.Request, configType, configID string) (configVersionMeta, error) {
	defer req.Body.Close()
	state.mu.Lock()
	defer state.mu.Unlock()

	var body versionSaveRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return configVersionMeta{}, fmt.Errorf("解析请求失败: %w", err)
	}

	if !versionNamePattern.MatchString(body.Name) {
		return configVersionMeta{}, errors.New("版本名称只能包含字母、数字、连字符和下划线")
	}

	reg := state.reg
	metaPath, err := getConfigMetaPath(reg, configType, configID)
	if err != nil {
		return configVersionMeta{}, err
	}

	// 读取当前配置内容
	currentContent, err := readCurrentConfig(reg, configType, configID)
	if err != nil {
		return configVersionMeta{}, fmt.Errorf("读取当前配置失败: %w", err)
	}

	// 加载元数据
	meta, err := loadVersionMeta(metaPath)
	if err != nil {
		return configVersionMeta{}, err
	}

	// 检查版本名称冲突
	for _, v := range meta.Versions {
		if v.Name == body.Name {
			return configVersionMeta{}, fmt.Errorf("版本名称 %q 已存在", body.Name)
		}
	}

	// 创建版本快照文件
	versionPath, err := getVersionSnapshotPath(reg, configType, configID, body.Name)
	if err != nil {
		return configVersionMeta{}, err
	}

	if err := os.MkdirAll(filepath.Dir(versionPath), 0o755); err != nil {
		return configVersionMeta{}, err
	}

	if err := os.WriteFile(versionPath, []byte(currentContent), 0o644); err != nil {
		return configVersionMeta{}, err
	}

	// 添加版本元数据
	newVersion := configVersionMeta{
		Name:        body.Name,
		Description: body.Description,
		CreatedAt:   time.Now(),
		CreatedBy:   "system", // TODO: 从请求上下文获取用户信息
		Default:     false,
	}
	meta.Versions = append(meta.Versions, newVersion)

	// 保存元数据
	if err := saveVersionMeta(metaPath, meta); err != nil {
		_ = os.Remove(versionPath) // 回滚快照文件
		return configVersionMeta{}, err
	}

	return newVersion, nil
}

// 获取指定版本的内容
func getConfigVersionContent(state *serverState, configType, configID, version string) (versionContentResult, error) {
	reg := state.registry()
	versionPath, err := getVersionSnapshotPath(reg, configType, configID, version)
	if err != nil {
		return versionContentResult{}, err
	}

	data, err := os.ReadFile(versionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return versionContentResult{
				ConfigID: configID,
				Version:  version,
				Content:  "",
				Exists:   false,
			}, nil
		}
		return versionContentResult{}, err
	}

	return versionContentResult{
		ConfigID: configID,
		Version:  version,
		Content:  string(data),
		Exists:   true,
	}, nil
}

// 删除指定版本
func deleteConfigVersion(state *serverState, configType, configID, version string) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	reg := state.reg
	metaPath, err := getConfigMetaPath(reg, configType, configID)
	if err != nil {
		return err
	}

	meta, err := loadVersionMeta(metaPath)
	if err != nil {
		return err
	}

	// 查找并删除版本
	found := false
	newVersions := make([]configVersionMeta, 0, len(meta.Versions))
	for _, v := range meta.Versions {
		if v.Name == version {
			found = true
			continue
		}
		newVersions = append(newVersions, v)
	}

	if !found {
		return fmt.Errorf("版本 %q 不存在", version)
	}

	// 删除快照文件
	versionPath, err := getVersionSnapshotPath(reg, configType, configID, version)
	if err != nil {
		return err
	}
	if err := os.Remove(versionPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	// 更新元数据
	meta.Versions = newVersions
	return saveVersionMeta(metaPath, meta)
}

// 设置默认版本
func setDefaultConfigVersion(state *serverState, configType, configID, version string) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	reg := state.reg
	metaPath, err := getConfigMetaPath(reg, configType, configID)
	if err != nil {
		return err
	}

	meta, err := loadVersionMeta(metaPath)
	if err != nil {
		return err
	}

	// 查找版本并设置默认
	found := false
	for i := range meta.Versions {
		if meta.Versions[i].Name == version {
			meta.Versions[i].Default = true
			found = true
		} else {
			meta.Versions[i].Default = false
		}
	}

	if !found {
		return fmt.Errorf("版本 %q 不存在", version)
	}

	return saveVersionMeta(metaPath, meta)
}

// 辅助函数：获取元数据文件路径
func getConfigMetaPath(reg *registry.Registry, configType, configID string) (string, error) {
	switch configType {
	case "global":
		return filepath.Join(reg.BaseDir, "configs", "ops.yaml.meta.json"), nil
	case "plugin":
		if !config.SafePluginConfigID(configID) {
			return "", errors.New("插件 ID 包含不安全路径字符")
		}
		return filepath.Join(reg.BaseDir, "configs", "plugins", configID+".yaml.meta.json"), nil
	default:
		return "", fmt.Errorf("不支持的配置类型: %s", configType)
	}
}

// 辅助函数：获取版本快照文件路径
func getVersionSnapshotPath(reg *registry.Registry, configType, configID, version string) (string, error) {
	if !versionNamePattern.MatchString(version) {
		return "", errors.New("版本名称只能包含字母、数字、连字符和下划线")
	}

	switch configType {
	case "global":
		return filepath.Join(reg.BaseDir, "configs", ".versions", "ops", version+".yaml"), nil
	case "plugin":
		if !config.SafePluginConfigID(configID) {
			return "", errors.New("插件 ID 包含不安全路径字符")
		}
		return filepath.Join(reg.BaseDir, "configs", ".versions", configID, version+".yaml"), nil
	default:
		return "", fmt.Errorf("不支持的配置类型: %s", configType)
	}
}

// 辅助函数：读取当前配置内容
func readCurrentConfig(reg *registry.Registry, configType, configID string) (string, error) {
	var path string
	var err error

	switch configType {
	case "global":
		path = filepath.Join(reg.BaseDir, "configs", "ops.yaml")
	case "plugin":
		path, err = config.PluginHostConfigPath(reg.BaseDir, configID)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("不支持的配置类型: %s", configType)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	return string(data), nil
}

// 辅助函数：加载版本元数据
func loadVersionMeta(path string) (configVersionMetaFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return configVersionMetaFile{Versions: []configVersionMeta{}}, nil
		}
		return configVersionMetaFile{}, err
	}

	var meta configVersionMetaFile
	if err := json.Unmarshal(data, &meta); err != nil {
		return configVersionMetaFile{}, fmt.Errorf("解析元数据文件失败: %w", err)
	}

	if meta.Versions == nil {
		meta.Versions = []configVersionMeta{}
	}

	return meta, nil
}

// 辅助函数：保存版本元数据
func saveVersionMeta(path string, meta configVersionMetaFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

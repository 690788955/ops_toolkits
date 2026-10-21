package nexus

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"shell_ops/internal/market"
)

const (
	configFileName = "nexus.yaml"

	DefaultSourceID   = "nexus"
	DefaultSourceName = "Nexus 插件仓库"
	DefaultIndexPath  = "index.json"
	DefaultPackageDir = "packages"
)

const configFileHeader = `# Nexus 3 Raw 插件仓库配置，由 Web 控制台维护。
# 支持匿名或账号密码访问；文件包含密码时请勿提交到版本库。
`

type Config struct {
	Enabled            *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	BaseURL            string `yaml:"base_url" json:"base_url"`
	Repository         string `yaml:"repository" json:"repository"`
	SourceID           string `yaml:"source_id" json:"source_id"`
	SourceName         string `yaml:"source_name" json:"source_name"`
	IndexPath          string `yaml:"index_path" json:"index_path"`
	PackageDir         string `yaml:"package_dir" json:"package_dir"`
	Username           string `yaml:"username,omitempty" json:"-"`
	Password           string `yaml:"password,omitempty" json:"-"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
}

func DefaultConfig() Config {
	enabled := true
	return Config{
		Enabled:    &enabled,
		SourceID:   DefaultSourceID,
		SourceName: DefaultSourceName,
		IndexPath:  DefaultIndexPath,
		PackageDir: DefaultPackageDir,
	}
}

func (c Config) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

func ConfigPath(baseDir string) string {
	return filepath.Join(baseDir, "configs", configFileName)
}

func LoadConfig(baseDir string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(filepath.Clean(ConfigPath(baseDir)))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("读取 Nexus 仓库配置失败: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("解析 Nexus 仓库配置失败: %w", err)
	}
	return NormalizeConfig(cfg)
}

func SaveConfig(baseDir string, cfg Config) (Config, error) {
	normalized, err := NormalizeConfig(cfg)
	if err != nil {
		return Config{}, err
	}
	if normalized.BaseURL == "" {
		return Config{}, fmt.Errorf("Nexus 地址不能为空")
	}
	if normalized.Repository == "" {
		return Config{}, fmt.Errorf("Nexus 仓库名不能为空")
	}
	data, err := yaml.Marshal(normalized)
	if err != nil {
		return Config{}, fmt.Errorf("序列化 Nexus 仓库配置失败: %w", err)
	}
	target := ConfigPath(baseDir)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return Config{}, fmt.Errorf("创建配置目录失败: %w", err)
	}
	content := append([]byte(configFileHeader), data...)
	temp := target + ".tmp"
	if err := os.WriteFile(temp, content, 0o600); err != nil {
		return Config{}, fmt.Errorf("写入 Nexus 仓库配置失败: %w", err)
	}
	if err := os.Rename(temp, target); err != nil {
		_ = os.Remove(temp)
		return Config{}, fmt.Errorf("写入 Nexus 仓库配置失败: %w", err)
	}
	return normalized, nil
}

func NormalizeConfig(cfg Config) (Config, error) {
	if cfg.Enabled == nil {
		enabled := true
		cfg.Enabled = &enabled
	}
	baseURL, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return Config{}, err
	}
	cfg.BaseURL = baseURL
	cfg.Repository = strings.TrimSpace(cfg.Repository)
	if cfg.Repository != "" && !isSafeName(cfg.Repository) {
		return Config{}, fmt.Errorf("Nexus 仓库名只能包含字母、数字、点、下划线和短横线")
	}
	cfg.SourceID = strings.TrimSpace(cfg.SourceID)
	if cfg.SourceID == "" {
		cfg.SourceID = DefaultSourceID
	}
	if !market.ValidPluginID(cfg.SourceID) {
		return Config{}, fmt.Errorf("市场源 ID 只能包含字母、数字、点、下划线和短横线")
	}
	cfg.SourceName = strings.TrimSpace(cfg.SourceName)
	if cfg.SourceName == "" {
		cfg.SourceName = DefaultSourceName
	}
	cfg.IndexPath, err = normalizeAssetPath(cfg.IndexPath, DefaultIndexPath, "索引路径")
	if err != nil {
		return Config{}, err
	}
	cfg.PackageDir, err = normalizeAssetPath(cfg.PackageDir, DefaultPackageDir, "插件包目录")
	if err != nil {
		return Config{}, err
	}
	if cfg.IndexPath == cfg.PackageDir {
		return Config{}, fmt.Errorf("索引路径不能与插件包目录相同")
	}
	cfg.Username = strings.TrimSpace(cfg.Username)
	if cfg.Username == "" {
		cfg.Password = ""
	}
	return cfg, nil
}

func (c Config) IsBasicAuth() bool {
	return strings.TrimSpace(c.Username) != ""
}

func (c Config) Ready() bool {
	return c.IsEnabled() && c.BaseURL != "" && c.Repository != ""
}

func (c Config) IndexURL() string {
	if c.BaseURL == "" || c.Repository == "" {
		return ""
	}
	return contentURL(c.BaseURL, c.Repository, c.IndexPath)
}

func (c Config) PackagePrefix() string {
	return c.PackageDir + "/"
}

func (c Config) PackagePath(id, version string) (string, error) {
	fileName, err := PackageFileName(id, version)
	if err != nil {
		return "", err
	}
	return path.Join(c.PackageDir, fileName), nil
}

func PackageFileName(id, version string) (string, error) {
	if !market.ValidPluginID(id) {
		return "", fmt.Errorf("插件 ID 不安全: %q", id)
	}
	safeVersion := sanitizeFileComponent(version)
	if safeVersion == "" || !hasAlphaNumeric(safeVersion) {
		return "", fmt.Errorf("插件 %s 的版本号不安全: %q", id, version)
	}
	return id + "-" + safeVersion + ".zip", nil
}

func hasAlphaNumeric(value string) bool {
	for _, ch := range value {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' {
			return true
		}
	}
	return false
}

func sanitizeFileComponent(value string) string {
	var builder strings.Builder
	for _, ch := range strings.TrimSpace(value) {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == '-' {
			builder.WriteRune(ch)
			continue
		}
		builder.WriteRune('_')
	}
	return strings.Trim(builder.String(), ".")
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.ContainsAny(raw, " \t\r\n") {
		return "", fmt.Errorf("Nexus 地址不能包含空白字符")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("Nexus 地址无效: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("Nexus 地址只支持 http/https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("Nexus 地址缺少主机名")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("Nexus 地址不支持内嵌用户名密码")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("Nexus 地址不能包含查询参数或锚点")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeAssetPath(raw, fallback, label string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = fallback
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if strings.HasPrefix(value, "/") || strings.Contains(value, "://") {
		return "", fmt.Errorf("%s必须是仓库内的相对路径", label)
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return "", fmt.Errorf("%s不能包含 . 或 .. 路径片段", label)
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == "" || segment == "." || segment == ".." || !isSafeName(segment) {
			return "", fmt.Errorf("%s只能包含字母、数字、点、下划线和短横线", label)
		}
	}
	return clean, nil
}

func isSafeName(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, ch := range value {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == '-' {
			continue
		}
		return false
	}
	return true
}

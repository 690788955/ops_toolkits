package marketsource

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
	configFileName = "market_sources.yaml"

	TypeLocal  = "local"
	TypeRemote = "remote"
	TypeNexus  = "nexus"

	AuthAnonymous = "anonymous"
	AuthBasic     = "basic"

	DefaultLocalID   = "local"
	DefaultLocalName = "本地插件市场"
	DefaultNexusID   = "nexus"
	DefaultNexusName = "Nexus 插件仓库"
	DefaultIndexPath  = "index.json"
	DefaultPackageDir = "packages"
)

const configFileHeader = `# 插件市场来源配置，由 Web 控制台维护。
# 文件可能包含 Nexus 账号密码，请勿提交到版本库。
`

type Source struct {
	ID                 string `yaml:"id" json:"id"`
	Name               string `yaml:"name" json:"name"`
	Type               string `yaml:"type" json:"type"`
	URL                string `yaml:"url" json:"url"`
	Enabled            *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	AuthType           string `yaml:"auth_type,omitempty" json:"auth_type,omitempty"`
	Username           string `yaml:"username,omitempty" json:"username,omitempty"`
	Password           string `yaml:"password,omitempty" json:"password,omitempty"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify,omitempty" json:"insecure_skip_verify,omitempty"`
	Repository         string `yaml:"repository,omitempty" json:"repository,omitempty"`
	IndexPath          string `yaml:"index_path,omitempty" json:"index_path,omitempty"`
	PackageDir         string `yaml:"package_dir,omitempty" json:"package_dir,omitempty"`
}

type Config struct {
	Sources []Source `yaml:"sources" json:"sources"`
}

func DefaultLocalSource() Source {
	enabled := true
	return Source{
		ID:        DefaultLocalID,
		Name:      DefaultLocalName,
		Type:      TypeLocal,
		URL:       "market",
		Enabled:   &enabled,
		AuthType:  AuthAnonymous,
		IndexPath: "",
	}
}

func (s Source) IsEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}

func (s Source) IsBasicAuth() bool {
	return s.AuthType == AuthBasic && strings.TrimSpace(s.Username) != ""
}

func (s Source) Ready() bool {
	if !s.IsEnabled() || strings.TrimSpace(s.URL) == "" {
		return false
	}
	if s.Type == TypeNexus {
		return strings.TrimSpace(s.Repository) != ""
	}
	return true
}

func ConfigPath(baseDir string) string {
	return filepath.Join(baseDir, "configs", configFileName)
}

func Load(baseDir string) (Config, bool, error) {
	target := filepath.Clean(ConfigPath(baseDir))
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, false, nil
		}
		return Config{}, false, fmt.Errorf("读取市场来源配置失败: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, true, fmt.Errorf("解析市场来源配置失败: %w", err)
	}
	normalized, err := NormalizeConfig(cfg)
	if err != nil {
		return Config{}, true, err
	}
	return normalized, true, nil
}

func Save(baseDir string, cfg Config) (Config, error) {
	normalized, err := NormalizeConfig(cfg)
	if err != nil {
		return Config{}, err
	}
	data, err := yaml.Marshal(normalized)
	if err != nil {
		return Config{}, fmt.Errorf("序列化市场来源配置失败: %w", err)
	}
	target := ConfigPath(baseDir)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return Config{}, fmt.Errorf("创建配置目录失败: %w", err)
	}
	content := append([]byte(configFileHeader), data...)
	temp := target + ".tmp"
	if err := os.WriteFile(temp, content, 0o600); err != nil {
		return Config{}, fmt.Errorf("写入市场来源配置失败: %w", err)
	}
	if err := os.Rename(temp, target); err != nil {
		_ = os.Remove(temp)
		return Config{}, fmt.Errorf("写入市场来源配置失败: %w", err)
	}
	return normalized, nil
}

func NormalizeConfig(cfg Config) (Config, error) {
	seen := map[string]bool{}
	sources := make([]Source, 0, len(cfg.Sources))
	for i, source := range cfg.Sources {
		normalized, err := NormalizeSource(source)
		if err != nil {
			return Config{}, fmt.Errorf("第 %d 个市场来源无效: %w", i+1, err)
		}
		if seen[normalized.ID] {
			return Config{}, fmt.Errorf("市场来源 ID 重复: %s", normalized.ID)
		}
		seen[normalized.ID] = true
		sources = append(sources, normalized)
	}
	cfg.Sources = sources
	return cfg, nil
}

func NormalizeSource(source Source) (Source, error) {
	if source.Enabled == nil {
		enabled := true
		source.Enabled = &enabled
	}
	source.ID = strings.TrimSpace(source.ID)
	if source.ID == "" {
		return Source{}, fmt.Errorf("市场来源 ID 不能为空")
	}
	if !market.ValidPluginID(source.ID) {
		return Source{}, fmt.Errorf("市场来源 ID 只能包含字母、数字、点、下划线和短横线")
	}
	source.Type = strings.ToLower(strings.TrimSpace(source.Type))
	if source.Type == "" {
		source.Type = InferType(source.URL)
	}
	switch source.Type {
	case TypeLocal, TypeRemote, TypeNexus:
	default:
		return Source{}, fmt.Errorf("不支持的市场来源类型: %s", source.Type)
	}
	source.Name = strings.TrimSpace(source.Name)
	if source.Name == "" {
		source.Name = defaultName(source.Type)
	}
	source.AuthType = strings.ToLower(strings.TrimSpace(source.AuthType))
	if source.AuthType == "" {
		if strings.TrimSpace(source.Username) != "" {
			source.AuthType = AuthBasic
		} else {
			source.AuthType = AuthAnonymous
		}
	}
	switch source.AuthType {
	case AuthAnonymous:
		source.Username = ""
		source.Password = ""
	case AuthBasic:
		source.Username = strings.TrimSpace(source.Username)
		if source.Username == "" {
			return Source{}, fmt.Errorf("使用账号密码访问时必须填写用户名")
		}
	default:
		return Source{}, fmt.Errorf("不支持的市场来源认证方式: %s", source.AuthType)
	}

	switch source.Type {
	case TypeLocal:
		source.URL = strings.TrimSpace(source.URL)
		if source.URL == "" {
			return Source{}, fmt.Errorf("本地市场目录不能为空")
		}
		if strings.HasPrefix(strings.ToLower(source.URL), "http://") || strings.HasPrefix(strings.ToLower(source.URL), "https://") {
			return Source{}, fmt.Errorf("本地来源的地址不能是 HTTP/HTTPS 地址")
		}
		source.AuthType = AuthAnonymous
		source.Username = ""
		source.Password = ""
		source.InsecureSkipVerify = false
		source.Repository = ""
		source.IndexPath = ""
		source.PackageDir = ""
	case TypeRemote:
		baseURL, err := normalizeHTTPURL(source.URL, "市场索引地址")
		if err != nil {
			return Source{}, err
		}
		source.URL = baseURL
		source.InsecureSkipVerify = false
		source.Repository = ""
		source.IndexPath = ""
		source.PackageDir = ""
	case TypeNexus:
		baseURL, err := normalizeHTTPURL(source.URL, "Nexus 地址")
		if err != nil {
			return Source{}, err
		}
		source.URL = baseURL
		source.Repository = strings.TrimSpace(source.Repository)
		if source.Repository == "" {
			return Source{}, fmt.Errorf("Nexus 仓库名不能为空")
		}
		if !isSafeName(source.Repository) {
			return Source{}, fmt.Errorf("Nexus 仓库名只能包含字母、数字、点、下划线和短横线")
		}
		source.IndexPath, err = normalizeAssetPath(source.IndexPath, DefaultIndexPath, "索引路径")
		if err != nil {
			return Source{}, err
		}
		source.PackageDir, err = normalizeAssetPath(source.PackageDir, DefaultPackageDir, "插件包目录")
		if err != nil {
			return Source{}, err
		}
		if source.IndexPath == source.PackageDir {
			return Source{}, fmt.Errorf("索引路径不能与插件包目录相同")
		}
	}
	return source, nil
}

func InferType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return TypeRemote
	}
	return TypeLocal
}

func defaultName(sourceType string) string {
	switch sourceType {
	case TypeNexus:
		return DefaultNexusName
	case TypeRemote:
		return "远程插件市场"
	default:
		return DefaultLocalName
	}
}

func normalizeHTTPURL(raw, label string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%s不能为空", label)
	}
	if strings.ContainsAny(raw, " \t\r\n") {
		return "", fmt.Errorf("%s不能包含空白字符", label)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s无效: %w", label, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%s只支持 http/https", label)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("%s缺少主机名", label)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%s不支持内嵌用户名密码，请使用账号密码配置", label)
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

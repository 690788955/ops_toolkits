package sshmgr

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	configFileName = "ssh_hosts.yaml"

	DefaultIntervalSeconds = 60
	MinIntervalSeconds     = 5
	MaxIntervalSeconds     = 3600

	DefaultTimeoutSeconds = 8
	MinTimeoutSeconds     = 2
	MaxTimeoutSeconds     = 60

	AuthKey      = "key"
	AuthPassword = "password"

	StateUnknown  = "unknown"
	StateChecking = "checking"
	StateOnline   = "online"
	StateOffline  = "offline"
)

const configFileHeader = `# SSH 操作机配置，由 Web 控制台维护。
# 密码和私钥口令以明文保存，请限制该文件访问权限。
# keep_bundle: true 会保留下发到操作机的临时文件，默认执行后自动清理。
`

type Host struct {
	ID             string `yaml:"id"`
	Name           string `yaml:"name"`
	Address        string `yaml:"address"`
	Port           int    `yaml:"port"`
	User           string `yaml:"user"`
	Auth           string `yaml:"auth"`
	KeyPath        string `yaml:"key_path,omitempty"`
	Password       string `yaml:"password,omitempty"`
	Passphrase     string `yaml:"passphrase,omitempty"`
	StrictHostKey  *bool  `yaml:"strict_host_key,omitempty"`
	KnownHostsPath string `yaml:"known_hosts_path,omitempty"`
	BundleBaseDir  string `yaml:"bundle_base_dir,omitempty"`
	KeepBundle     bool   `yaml:"keep_bundle,omitempty"`
	Enabled        *bool  `yaml:"enabled,omitempty"`
	Description    string `yaml:"description,omitempty"`
}

func (h Host) IsEnabled() bool {
	return h.Enabled == nil || *h.Enabled
}

func (h Host) IsStrictHostKey() bool {
	return h.StrictHostKey == nil || *h.StrictHostKey
}

type HostInput struct {
	Name            string `json:"name"`
	Address         string `json:"address"`
	Port            int    `json:"port"`
	User            string `json:"user"`
	Auth            string `json:"auth"`
	KeyPath         string `json:"key_path"`
	Password        string `json:"password"`
	Passphrase      string `json:"passphrase"`
	ClearPassword   bool   `json:"clear_password"`
	ClearPassphrase bool   `json:"clear_passphrase"`
	StrictHostKey   *bool  `json:"strict_host_key"`
	KnownHostsPath  string `json:"known_hosts_path"`
	BundleBaseDir   string `json:"bundle_base_dir"`
	KeepBundle      bool   `json:"keep_bundle"`
	Enabled         *bool  `json:"enabled"`
	Description     string `json:"description"`
}

type SettingsInput struct {
	IntervalSeconds int `json:"interval_seconds"`
	TimeoutSeconds  int `json:"timeout_seconds"`
}

type HostView struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Address        string `json:"address"`
	Port           int    `json:"port"`
	User           string `json:"user"`
	Auth           string `json:"auth"`
	KeyPath        string `json:"key_path,omitempty"`
	KnownHostsPath string `json:"known_hosts_path,omitempty"`
	BundleBaseDir  string `json:"bundle_base_dir,omitempty"`
	KeepBundle     bool   `json:"keep_bundle"`
	StrictHostKey  bool   `json:"strict_host_key"`
	Enabled        bool   `json:"enabled"`
	Description    string `json:"description,omitempty"`
	HasPassword    bool   `json:"has_password"`
	HasPassphrase  bool   `json:"has_passphrase"`
	Status         string `json:"status"`
	LatencyMS      int64  `json:"latency_ms,omitempty"`
	LastChecked    string `json:"last_checked,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	Fingerprint    string `json:"fingerprint,omitempty"`
}

type Overview struct {
	IntervalSeconds int        `json:"interval_seconds"`
	TimeoutSeconds  int        `json:"timeout_seconds"`
	ConfigPath      string     `json:"config_path"`
	Hosts           []HostView `json:"hosts"`
}

type ExecResult struct {
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	ExitCode    int    `json:"exit_code"`
	LatencyMS   int64  `json:"latency_ms"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type StreamResult struct {
	ExitCode    int    `json:"exit_code"`
	LatencyMS   int64  `json:"latency_ms"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type fileConfig struct {
	IntervalSeconds int    `yaml:"interval_seconds"`
	TimeoutSeconds  int    `yaml:"timeout_seconds"`
	Hosts           []Host `yaml:"hosts"`
}

func (c fileConfig) clone() fileConfig {
	out := c
	out.Hosts = append([]Host(nil), c.Hosts...)
	return out
}

func defaultFileConfig() fileConfig {
	return fileConfig{
		IntervalSeconds: DefaultIntervalSeconds,
		TimeoutSeconds:  DefaultTimeoutSeconds,
		Hosts:           []Host{},
	}
}

func configPath(baseDir string) string {
	return filepath.Join(baseDir, "configs", configFileName)
}

func loadConfig(path string) (fileConfig, error) {
	cfg := defaultFileConfig()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("读取 SSH 操作机配置失败: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("解析 SSH 操作机配置失败: %w", err)
	}
	if cfg.Hosts == nil {
		cfg.Hosts = []Host{}
	}
	if cfg.IntervalSeconds == 0 {
		cfg.IntervalSeconds = DefaultIntervalSeconds
	}
	if cfg.TimeoutSeconds == 0 {
		cfg.TimeoutSeconds = DefaultTimeoutSeconds
	}
	seen := map[string]bool{}
	for index := range cfg.Hosts {
		normalizeHost(&cfg.Hosts[index])
		host := cfg.Hosts[index]
		if host.ID == "" {
			return cfg, fmt.Errorf("SSH 操作机配置第 %d 项缺少 id", index+1)
		}
		if seen[host.ID] {
			return cfg, fmt.Errorf("SSH 操作机 id 重复: %s", host.ID)
		}
		seen[host.ID] = true
		if err := validateHost(host, false); err != nil {
			return cfg, fmt.Errorf("SSH 操作机 %s 配置无效: %w", host.ID, err)
		}
	}
	return cfg, nil
}

func saveConfig(path string, cfg fileConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteString(configFileHeader)
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(cfg); err != nil {
		_ = encoder.Close()
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	tempPath := fmt.Sprintf("%s.%d.tmp", path, time.Now().UnixNano())
	if err := os.WriteFile(tempPath, buf.Bytes(), 0o600); err != nil {
		return err
	}
	_ = os.Chmod(tempPath, 0o600)
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(path)
		if renameErr := os.Rename(tempPath, path); renameErr != nil {
			_ = os.Remove(tempPath)
			return renameErr
		}
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

func normalizeHost(host *Host) {
	host.ID = strings.TrimSpace(host.ID)
	host.Name = strings.TrimSpace(host.Name)
	host.Address = strings.TrimSpace(host.Address)
	host.User = strings.TrimSpace(host.User)
	host.Auth = strings.ToLower(strings.TrimSpace(host.Auth))
	host.KeyPath = strings.TrimSpace(host.KeyPath)
	host.KnownHostsPath = strings.TrimSpace(host.KnownHostsPath)
	host.BundleBaseDir = strings.TrimSpace(host.BundleBaseDir)
	if host.BundleBaseDir != "" {
		host.BundleBaseDir = path.Clean(host.BundleBaseDir)
	}
	host.Description = strings.TrimSpace(host.Description)
	if host.Port == 0 {
		host.Port = 22
	}
	if host.Auth == "" {
		if host.KeyPath != "" {
			host.Auth = AuthKey
		} else {
			host.Auth = AuthPassword
		}
	}
	if host.StrictHostKey == nil {
		host.StrictHostKey = boolValue(true)
	}
	if host.Enabled == nil {
		host.Enabled = boolValue(true)
	}
	if host.Name == "" {
		host.Name = host.Address
	}
}

func validateHost(host Host, requireSecret bool) error {
	if host.Address == "" {
		return fmt.Errorf("主机地址必填")
	}
	if strings.ContainsAny(host.Address, " \t\r\n") {
		return fmt.Errorf("主机地址不能包含空白字符")
	}
	if strings.ContainsAny(host.Address, "/@") {
		return fmt.Errorf("主机地址格式无效")
	}
	if _, _, err := net.SplitHostPort(host.Address); err == nil {
		return fmt.Errorf("主机地址不要包含端口，请填写到端口字段")
	}
	if host.Port < 1 || host.Port > 65535 {
		return fmt.Errorf("端口必须在 1-65535 之间")
	}
	if host.User == "" {
		return fmt.Errorf("用户名必填")
	}
	if strings.ContainsAny(host.User, " \t\r\n") {
		return fmt.Errorf("用户名不能包含空白字符")
	}
	switch host.Auth {
	case AuthKey:
	case AuthPassword:
		if requireSecret && host.Password == "" {
			return fmt.Errorf("密码认证需要填写密码")
		}
	default:
		return fmt.Errorf("认证方式必须是 %s 或 %s", AuthKey, AuthPassword)
	}
	if err := validateBundleBaseDir(host.BundleBaseDir); err != nil {
		return err
	}
	return nil
}

func validateBundleBaseDir(value string) error {
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, "\x00\r\n\t") {
		return fmt.Errorf("下发根目录不能包含控制字符")
	}
	if strings.Contains(value, "\\") {
		return fmt.Errorf("下发根目录必须使用 POSIX 路径分隔符 /")
	}
	if !strings.HasPrefix(value, "/") {
		return fmt.Errorf("下发根目录必须是绝对路径，例如 /tmp 或 /data/opsctl")
	}
	if path.Clean(value) == "/" {
		return fmt.Errorf("下发根目录不能是根目录 /")
	}
	return nil
}

func buildHost(input HostInput, existing *Host) (Host, error) {
	var host Host
	if existing != nil {
		host = *existing
	} else {
		host = Host{
			StrictHostKey: boolValue(true),
			Enabled:       boolValue(true),
		}
	}
	host.Name = strings.TrimSpace(input.Name)
	host.Address = strings.TrimSpace(input.Address)
	host.Port = input.Port
	if host.Port == 0 {
		host.Port = 22
	}
	host.User = strings.TrimSpace(input.User)
	host.Auth = strings.ToLower(strings.TrimSpace(input.Auth))
	if host.Auth == "" {
		if strings.TrimSpace(input.KeyPath) != "" {
			host.Auth = AuthKey
		} else if existing != nil {
			host.Auth = existing.Auth
		} else {
			host.Auth = AuthPassword
		}
	}
	host.KeyPath = strings.TrimSpace(input.KeyPath)
	host.KnownHostsPath = strings.TrimSpace(input.KnownHostsPath)
	host.BundleBaseDir = strings.TrimSpace(input.BundleBaseDir)
	host.KeepBundle = input.KeepBundle
	host.Description = strings.TrimSpace(input.Description)
	if input.StrictHostKey != nil {
		host.StrictHostKey = boolValue(*input.StrictHostKey)
	}
	if input.Enabled != nil {
		host.Enabled = boolValue(*input.Enabled)
	}
	if input.ClearPassword {
		host.Password = ""
	} else if input.Password != "" {
		host.Password = input.Password
	}
	if input.ClearPassphrase {
		host.Passphrase = ""
	} else if input.Passphrase != "" {
		host.Passphrase = input.Passphrase
	}
	switch host.Auth {
	case AuthPassword:
		host.KeyPath = ""
		host.Passphrase = ""
	case AuthKey:
		host.Password = ""
	}
	if host.Name == "" {
		host.Name = host.Address
	}
	normalizeHost(&host)
	if err := validateHost(host, true); err != nil {
		return Host{}, err
	}
	return host, nil
}

func validateSettings(input SettingsInput) error {
	if input.IntervalSeconds < MinIntervalSeconds || input.IntervalSeconds > MaxIntervalSeconds {
		return fmt.Errorf("巡检间隔必须在 %d-%d 秒之间", MinIntervalSeconds, MaxIntervalSeconds)
	}
	if input.TimeoutSeconds < MinTimeoutSeconds || input.TimeoutSeconds > MaxTimeoutSeconds {
		return fmt.Errorf("连接超时必须在 %d-%d 秒之间", MinTimeoutSeconds, MaxTimeoutSeconds)
	}
	return nil
}

func newHostID(existing []Host) string {
	used := make(map[string]bool, len(existing))
	for _, host := range existing {
		used[host.ID] = true
	}
	for attempt := 0; attempt < 20; attempt++ {
		buf := make([]byte, 4)
		if _, err := rand.Read(buf); err != nil {
			break
		}
		id := "host-" + hex.EncodeToString(buf)
		if !used[id] {
			return id
		}
	}
	return fmt.Sprintf("host-%d", time.Now().UnixNano())
}

func boolValue(value bool) *bool {
	return &value
}

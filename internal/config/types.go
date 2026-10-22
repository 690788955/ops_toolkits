package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type RootConfig struct {
	App               AppConfig              `yaml:"app" json:"app"`
	Paths             PathsConfig            `yaml:"paths" json:"paths"`
	Server            ServerConfig           `yaml:"server" json:"server"`
	Menu              MenuConfig             `yaml:"menu" json:"menu"`
	Registry          RegistryConfig         `yaml:"registry" json:"registry"`
	Plugins           PluginsConfig          `yaml:"plugins" json:"plugins"`
	Market            MarketConfig           `yaml:"market" json:"market"`
	HostConfigFiles   HostConfigFilesConfig  `yaml:"host_config_files" json:"host_config_files"`
	UI                UIConfig               `yaml:"ui" json:"ui"`
	ConfigDefaults    map[string]interface{} `yaml:"config_defaults" json:"config_defaults"`
	RuntimeCategories []Category             `yaml:"-" json:"-"`
	Name              string                 `yaml:"name" json:"-"`
	Description       string                 `yaml:"description" json:"-"`
	Categories        []Category             `yaml:"categories" json:"-"`
	Tools             []ToolEntry            `yaml:"tools" json:"-"`
	Workflows         []WorkflowRef          `yaml:"workflows" json:"-"`
	HTTP              HTTPConfig             `yaml:"http" json:"-"`
}

type AppConfig struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Version     string `yaml:"version" json:"version"`
}

type PathsConfig struct {
	Tools     []string `yaml:"tools" json:"tools"`
	Workflows []string `yaml:"workflows" json:"workflows"`
	Runs      string   `yaml:"runs" json:"runs"`
	Logs      string   `yaml:"logs" json:"logs"`
}

type ServerConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Host    string `yaml:"host" json:"host"`
	Port    int    `yaml:"port" json:"port"`
}

type MenuConfig struct {
	Categories []Category `yaml:"categories" json:"categories"`
}

type RegistryConfig struct {
	IncludeTools     []string `yaml:"include_tools" json:"include_tools"`
	IncludeWorkflows []string `yaml:"include_workflows" json:"include_workflows"`
}

type PluginsConfig struct {
	Paths           []string `yaml:"paths" json:"paths"`
	Strict          bool     `yaml:"strict" json:"strict"`
	Disabled        []string `yaml:"disabled" json:"disabled"`
	AllowedCommands []string `yaml:"allowed_commands" json:"allowed_commands"`
}

type MarketConfig struct {
	Sources []MarketSource `yaml:"sources" json:"sources"`
}

type MarketSource struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
}

type UIConfig struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	Title       string `yaml:"title" json:"title"`
	LogFontSize int    `yaml:"log_font_size" json:"log_font_size"`
}

type HTTPConfig struct {
	Addr string `yaml:"addr" json:"addr"`
}

type Category struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
}

type ToolEntry struct {
	ID          string `yaml:"id" json:"id"`
	Category    string `yaml:"category" json:"category"`
	Path        string `yaml:"path" json:"path"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
}

type WorkflowRef struct {
	ID          string   `yaml:"id" json:"id"`
	Category    string   `yaml:"category" json:"category"`
	Path        string   `yaml:"path" json:"path"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Tags        []string `yaml:"tags" json:"tags"`
}

type ToolConfig struct {
	ID             string                 `yaml:"id" json:"id"`
	Name           string                 `yaml:"name" json:"name"`
	Description    string                 `yaml:"description" json:"description"`
	Version        string                 `yaml:"version" json:"version"`
	Category       string                 `yaml:"category" json:"category"`
	Tags           []string               `yaml:"tags" json:"tags"`
	Help           HelpConfig             `yaml:"help" json:"help"`
	Entry          string                 `yaml:"entry" json:"entry"`
	Execution      ExecutionConfig        `yaml:"execution" json:"execution"`
	Parameters     []Parameter            `yaml:"parameters" json:"parameters"`
	Outputs        []ToolOutput           `yaml:"outputs" json:"outputs"`
	PassMode       PassMode               `yaml:"pass_mode" json:"pass_mode"`
	Timeout        string                 `yaml:"timeout" json:"timeout"`
	Confirm        Confirmation           `yaml:"confirm" json:"confirm"`
	Confirmation   Confirmation           `yaml:"confirmation" json:"-"`
	Env            map[string]string      `yaml:"env" json:"env"`
	ConfigDefaults map[string]interface{} `yaml:"config_defaults" json:"config_defaults"`
	PluginConfig   PluginToolConfig       `yaml:"-" json:"-"`
	ConfigDir      string                 `yaml:"config_dir" json:"config_dir,omitempty"`
	ConfigFiles    []string               `yaml:"config_files" json:"config_files"`
	ConfigFileRefs []ConfigFileRef        `yaml:"-" json:"config_file_entries,omitempty"`
	SensitivePaths []string               `yaml:"sensitive_paths" json:"sensitive_paths"`
}

type HelpConfig struct {
	Usage    string   `yaml:"usage" json:"usage"`
	Examples []string `yaml:"examples" json:"examples"`
}

type ExecutionConfig struct {
	Type    string   `yaml:"type" json:"type"`
	Entry   string   `yaml:"entry" json:"entry"`
	Args    []string `yaml:"args" json:"args"`
	Timeout string   `yaml:"timeout" json:"timeout"`
	Workdir string   `yaml:"workdir" json:"workdir"`
}

type PluginConfigMapping struct {
	Tools map[string]PluginToolConfigMapping `yaml:"tools" json:"tools"`
}

type PluginToolConfigMapping struct {
	ConfigFiles []ConfigFileRef `yaml:"config_files" json:"config_files"`
}

type PluginToolConfig struct {
	ID                   string                 `yaml:"-" json:"-"`
	Dir                  string                 `yaml:"-" json:"-"`
	SharedConfig         map[string]interface{} `yaml:"-" json:"-"`
	PackageDefaultConfig map[string]interface{} `yaml:"-" json:"-"`
	HostConfig           map[string]interface{} `yaml:"-" json:"-"`
	SensitivePaths       []string               `yaml:"-" json:"-"`
}

type PassMode struct {
	Env       bool   `yaml:"env" json:"env"`
	Args      bool   `yaml:"args" json:"args"`
	ParamFile bool   `yaml:"param_file" json:"param_file"`
	FileName  string `yaml:"file_name" json:"file_name"`
}

type Confirmation struct {
	Required bool   `yaml:"required" json:"required"`
	Message  string `yaml:"message" json:"message"`
}

type Parameter struct {
	Name        string      `yaml:"name" json:"name"`
	Type        string      `yaml:"type" json:"type"`
	Description string      `yaml:"description" json:"description"`
	Required    bool        `yaml:"required" json:"required"`
	Default     interface{} `yaml:"default" json:"default"`
	Options     []string    `yaml:"options" json:"options"`
	Sensitive   bool        `yaml:"sensitive" json:"sensitive"`
}

type ToolOutput struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Type        string `yaml:"type" json:"type"`
	Required    bool   `yaml:"required" json:"required"`
	JSONPath    string `yaml:"json_path" json:"json_path"`
}

type WorkflowConfig struct {
	ID          string          `yaml:"id" json:"id"`
	Name        string          `yaml:"name" json:"name"`
	Description string          `yaml:"description" json:"description"`
	Version     string          `yaml:"version" json:"version"`
	Category    string          `yaml:"category" json:"category"`
	Tags        []string        `yaml:"tags" json:"tags"`
	SharedWith  []string        `yaml:"shared_with,omitempty" json:"shared_with,omitempty"`
	Parameters  []Parameter     `yaml:"parameters" json:"parameters"`
	ConfigFiles []ConfigFileRef `yaml:"config_files" json:"config_files"`
	Nodes       []WorkflowNode  `yaml:"nodes" json:"nodes"`
	Edges       []WorkflowEdge  `yaml:"edges" json:"edges"`
	Steps       []WorkflowNode  `yaml:"steps" json:"-"`
	Confirm     Confirmation    `yaml:"confirm" json:"confirm"`
}

type WorkflowNode struct {
	ID        string                 `yaml:"id" json:"id"`
	Type      string                 `yaml:"type" json:"type"`
	Name      string                 `yaml:"name" json:"name"`
	Tool      string                 `yaml:"tool" json:"tool"`
	Host      string                 `yaml:"host,omitempty" json:"host,omitempty"`
	Condition WorkflowCondition      `yaml:"condition" json:"condition"`
	Loop      WorkflowLoop           `yaml:"loop" json:"loop"`
	Upload    WorkflowUpload         `yaml:"upload" json:"upload"`
	Extract   WorkflowExtractConfig  `yaml:"extract_config" json:"extract_config"`
	HTTP      WorkflowHTTP           `yaml:"http,omitempty" json:"http"`
	DependsOn []string               `yaml:"depends_on" json:"depends_on"`
	Params    map[string]interface{} `yaml:"params" json:"params"`
	Optional  bool                   `yaml:"optional" json:"optional"`
	Timeout   string                 `yaml:"timeout" json:"timeout"`
	Confirm   bool                   `yaml:"confirm" json:"confirm"`
	OnFailure string                 `yaml:"on_failure" json:"on_failure"`
}

type WorkflowLoop struct {
	Tool          string                 `yaml:"tool" json:"tool"`
	Params        map[string]interface{} `yaml:"params" json:"params"`
	MaxIterations int                    `yaml:"max_iterations" json:"max_iterations"`
	Target        string                 `yaml:"target" json:"target,omitempty"`
}

type WorkflowUpload struct {
	TargetDir     string                       `yaml:"target_dir" json:"target_dir"`
	ConfigExports []WorkflowUploadConfigExport `yaml:"config_exports" json:"config_exports"`
}

type WorkflowUploadConfigExport struct {
	ID         string `yaml:"id" json:"id"`
	Label      string `yaml:"label" json:"label,omitempty"`
	SourcePath string `yaml:"source_path" json:"source_path"`
	TargetPath string `yaml:"target_path" json:"target_path"`
	Access     string `yaml:"access" json:"access"`
	Create     bool   `yaml:"create" json:"create"`
}

type WorkflowExtractConfig struct {
	SourceType string `yaml:"source_type,omitempty" json:"source_type,omitempty"`
	FileName   string `yaml:"file_name" json:"file_name"`
	TargetPath string `yaml:"target_path" json:"target_path"`
	Label      string `yaml:"label" json:"label,omitempty"`
	Replace    bool   `yaml:"replace" json:"replace"`

	Files      []WorkflowExtractConfigFile `yaml:"files,omitempty" json:"files,omitempty"`
	SourceNode string                      `yaml:"source_node,omitempty" json:"source_node,omitempty"`
	SourceDir  string                      `yaml:"source_dir,omitempty" json:"source_dir,omitempty"`
	SourcePath string                      `yaml:"source_path,omitempty" json:"source_path,omitempty"`
}

type WorkflowExtractConfigFile struct {
	FileName   string `yaml:"file_name" json:"file_name"`
	SourcePath string `yaml:"source_path,omitempty" json:"source_path,omitempty"`
	TargetPath string `yaml:"target_path" json:"target_path"`
	Label      string `yaml:"label" json:"label,omitempty"`
	Replace    bool   `yaml:"replace" json:"replace"`
}

// WorkflowHTTP 描述内置 HTTP 调用节点，等价于一条可参数化的 curl 命令。
// URL、Header 与 Body 都支持 {{ .steps.<id>.stdout }} 这类运行期模板。
type WorkflowHTTP struct {
	Method       string            `yaml:"method,omitempty" json:"method,omitempty"`
	URL          string            `yaml:"url" json:"url"`
	Headers      map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	BodyType     string            `yaml:"body_type,omitempty" json:"body_type,omitempty"`
	Body         string            `yaml:"body,omitempty" json:"body,omitempty"`
	ExpectStatus []int             `yaml:"expect_status,omitempty" json:"expect_status,omitempty"`
	FailOnError  *bool             `yaml:"fail_on_error,omitempty" json:"fail_on_error,omitempty"`
	SaveResponse string            `yaml:"save_response,omitempty" json:"save_response,omitempty"`
	Timeout      string            `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

type WorkflowUploadResult struct {
	ID           string               `yaml:"id" json:"id"`
	FileName     string               `yaml:"filename" json:"filename"`
	Path         string               `yaml:"path" json:"path"`
	RelativePath string               `yaml:"relative_path" json:"relative_path"`
	Size         int64                `yaml:"size" json:"size"`
	Files        []WorkflowUploadFile `yaml:"files" json:"files,omitempty"`
	Count        int                  `yaml:"count" json:"count"`
	TotalSize    int64                `yaml:"total_size" json:"total_size"`
}

type WorkflowUploadFile struct {
	FileName     string `yaml:"filename" json:"filename"`
	Path         string `yaml:"path" json:"path"`
	RelativePath string `yaml:"relative_path" json:"relative_path"`
	Size         int64  `yaml:"size" json:"size"`
}

type WorkflowCondition struct {
	Input       string          `yaml:"input" json:"input"`
	Cases       []ConditionCase `yaml:"cases" json:"cases"`
	DefaultCase string          `yaml:"default_case" json:"default_case"`
}

type ConditionCase struct {
	ID       string   `yaml:"id" json:"id"`
	Name     string   `yaml:"name" json:"name"`
	Operator string   `yaml:"operator" json:"operator"`
	Values   []string `yaml:"values" json:"values"`
}

type WorkflowEdge struct {
	From string `yaml:"from" json:"from"`
	To   string `yaml:"to" json:"to"`
	Case string `yaml:"case" json:"case"`
}

const (
	WorkflowNodeTypeTool          = "tool"
	WorkflowNodeTypeCondition     = "condition"
	WorkflowNodeTypeParallel      = "parallel"
	WorkflowNodeTypeJoin          = "join"
	WorkflowNodeTypeLoop          = "loop"
	WorkflowNodeTypeUpload        = "upload"
	WorkflowNodeTypeExtractConfig = "extract_config"
	WorkflowNodeTypeHTTP          = "http_request"
)

var (
	httpMethods   = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	httpBodyTypes = []string{"none", "json", "text", "form"}
)

func allowedHTTPMethodSet() map[string]bool {
	out := make(map[string]bool, len(httpMethods))
	for _, method := range httpMethods {
		out[method] = true
	}
	return out
}

// NormalizeHTTPMethod 返回大写方法名；空值回落为 GET。
func NormalizeHTTPMethod(value string) (string, error) {
	method := strings.ToUpper(strings.TrimSpace(value))
	if method == "" {
		return "GET", nil
	}
	if !allowedHTTPMethodSet()[method] {
		return "", fmt.Errorf("HTTP 方法只支持 %s", strings.Join(httpMethods, " / "))
	}
	return method, nil
}

// NormalizeHTTPBodyType 归一化请求体类型；空值按是否有 body 推断。
func NormalizeHTTPBodyType(value, body string) (string, error) {
	bodyType := strings.ToLower(strings.TrimSpace(value))
	if bodyType == "" {
		if strings.TrimSpace(body) == "" {
			return "none", nil
		}
		return "json", nil
	}
	for _, item := range httpBodyTypes {
		if bodyType == item {
			return bodyType, nil
		}
	}
	return "", fmt.Errorf("请求体类型只支持 %s", strings.Join(httpBodyTypes, " / "))
}

func (h WorkflowHTTP) ShouldFailOnError() bool {
	return h.FailOnError == nil || *h.FailOnError
}

// IsZero 让 yaml omitempty 可以省略未配置 http 的节点段。
func (h WorkflowHTTP) IsZero() bool {
	return h.Method == "" && h.URL == "" && len(h.Headers) == 0 && h.BodyType == "" &&
		h.Body == "" && len(h.ExpectStatus) == 0 && h.FailOnError == nil &&
		h.SaveResponse == "" && h.Timeout == ""
}

// ExpectedStatusCodes 返回显式声明的期望状态码；为空表示按 2xx 处理。
func (h WorkflowHTTP) ExpectedStatusCodes() []int {
	return h.ExpectStatus
}

func (h WorkflowHTTP) MatchesStatus(code int) bool {
	if len(h.ExpectStatus) == 0 {
		return code >= 200 && code < 300
	}
	for _, item := range h.ExpectStatus {
		if item == code {
			return true
		}
	}
	return false
}

// NormalizeHTTPSaveResponse 校验响应落盘路径，只允许节点运行目录内的相对路径。
func NormalizeHTTPSaveResponse(value string) (string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", nil
	}
	if strings.Contains(raw, "://") {
		return "", fmt.Errorf("响应保存路径必须是节点运行目录下的相对路径")
	}
	raw = strings.ReplaceAll(raw, "\\", "/")
	if filepath.IsAbs(raw) || strings.HasPrefix(raw, "/") || len(raw) >= 2 && raw[1] == ':' {
		return "", fmt.Errorf("响应保存路径不能是绝对路径")
	}
	parts := strings.Split(raw, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("响应保存路径不能包含空路径、. 或 ..")
		}
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(raw)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("响应保存路径不能逃逸节点运行目录")
	}
	return clean, nil
}

// ValidateHTTPConfig 校验内置 HTTP 节点配置，返回归一化后的副本。
func ValidateHTTPConfig(nodeID string, item WorkflowHTTP) (WorkflowHTTP, error) {
	out := item
	url := strings.TrimSpace(out.URL)
	if url == "" {
		return out, fmt.Errorf("HTTP 节点 %s 的 http.url 必填", nodeID)
	}
	if !strings.Contains(url, "{{") {
		if !strings.HasPrefix(strings.ToLower(url), "http://") && !strings.HasPrefix(strings.ToLower(url), "https://") {
			return out, fmt.Errorf("HTTP 节点 %s 的 http.url 只支持 http:// 或 https://", nodeID)
		}
	}
	method, err := NormalizeHTTPMethod(out.Method)
	if err != nil {
		return out, fmt.Errorf("HTTP 节点 %s: %w", nodeID, err)
	}
	out.Method = method
	bodyType, err := NormalizeHTTPBodyType(out.BodyType, out.Body)
	if err != nil {
		return out, fmt.Errorf("HTTP 节点 %s: %w", nodeID, err)
	}
	out.BodyType = bodyType
	if bodyType != "none" && method == "GET" {
		return out, fmt.Errorf("HTTP 节点 %s 的 GET 请求不能配置请求体，请改用 POST/PUT/PATCH", nodeID)
	}
	for name, value := range out.Headers {
		if strings.TrimSpace(name) == "" {
			return out, fmt.Errorf("HTTP 节点 %s 的 http.headers 存在空 header 名", nodeID)
		}
		if strings.ContainsAny(name, "\r\n") || strings.ContainsAny(value, "\r\n") {
			return out, fmt.Errorf("HTTP 节点 %s 的 header %s 不能包含换行", nodeID, name)
		}
	}
	for _, code := range out.ExpectStatus {
		if code < 100 || code > 599 {
			return out, fmt.Errorf("HTTP 节点 %s 的 expect_status 必须在 100..599 之间", nodeID)
		}
	}
	savePath, err := NormalizeHTTPSaveResponse(out.SaveResponse)
	if err != nil {
		return out, fmt.Errorf("HTTP 节点 %s: %w", nodeID, err)
	}
	out.SaveResponse = savePath
	if strings.TrimSpace(out.Timeout) != "" {
		if _, err := time.ParseDuration(out.Timeout); err != nil {
			return out, fmt.Errorf("HTTP 节点 %s 的 http.timeout 格式无效，例如 30s、2m", nodeID)
		}
	}
	return out, nil
}

func NormalizeUploadTargetDir(value string) (string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", nil
	}
	if strings.Contains(raw, "://") {
		return "", fmt.Errorf("上传目标目录必须是 uploads 下的相对子目录")
	}
	if len(raw) >= 2 && raw[1] == ':' {
		return "", fmt.Errorf("上传目标目录不能是绝对路径")
	}
	raw = strings.ReplaceAll(raw, "\\", "/")
	if filepath.IsAbs(raw) || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("上传目标目录不能是绝对路径")
	}
	parts := strings.Split(raw, "/")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("上传目标目录不能包含空路径、. 或 ..")
		}
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(raw)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("上传目标目录不能逃逸 uploads 目录")
	}
	return clean, nil
}

func (c RootConfig) DisplayName() string {
	if c.App.Name != "" {
		return c.App.Name
	}
	return c.Name
}

func (c RootConfig) DisplayDescription() string {
	if c.App.Description != "" {
		return c.App.Description
	}
	return c.Description
}

func (c RootConfig) DisplayCategories() []Category {
	base := c.Menu.Categories
	if len(base) == 0 {
		base = c.Categories
	}
	if len(c.RuntimeCategories) == 0 {
		return base
	}
	out := make([]Category, 0, len(base)+len(c.RuntimeCategories))
	seen := map[string]bool{}
	for _, category := range base {
		out = append(out, category)
		if category.ID != "" {
			seen[category.ID] = true
		}
	}
	for _, category := range c.RuntimeCategories {
		if category.ID == "" || seen[category.ID] {
			continue
		}
		out = append(out, category)
		seen[category.ID] = true
	}
	return out
}

func (c RootConfig) ListenAddr() string {
	if c.Server.Port > 0 {
		host := c.Server.Host
		if host == "" {
			host = "127.0.0.1"
		}
		return host + ":" + fmtInt(c.Server.Port)
	}
	if c.HTTP.Addr != "" {
		return c.HTTP.Addr
	}
	return ":8080"
}

func ParseTimeout(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return d
}

func fmtInt(value int) string {
	return strconv.Itoa(value)
}

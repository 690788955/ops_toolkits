package config

import (
	"fmt"
	"math"
	"net/mail"
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
	Uploads           UploadsConfig          `yaml:"uploads" json:"uploads"`
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

// UploadsConfig 限制上传节点可写入的绝对目录；留空时只允许 runs/uploads 下的相对子目录。
type UploadsConfig struct {
	AllowedDirs []string `yaml:"allowed_dirs" json:"allowed_dirs"`
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
	ID          string                 `yaml:"id" json:"id"`
	Name        string                 `yaml:"name" json:"name"`
	Description string                 `yaml:"description" json:"description"`
	Version     string                 `yaml:"version" json:"version"`
	Category    string                 `yaml:"category" json:"category"`
	Tags        []string               `yaml:"tags" json:"tags"`
	SharedWith  []string               `yaml:"shared_with,omitempty" json:"shared_with,omitempty"`
	Parameters  []Parameter            `yaml:"parameters" json:"parameters"`
	ConfigFiles []ConfigFileRef        `yaml:"config_files" json:"config_files"`
	Stages      []WorkflowStage        `yaml:"stages,omitempty" json:"stages,omitempty"`
	Nodes       []WorkflowNode         `yaml:"nodes" json:"nodes"`
	Edges       []WorkflowEdge         `yaml:"edges" json:"edges"`
	Steps       []WorkflowNode         `yaml:"steps" json:"-"`
	Confirm     Confirmation           `yaml:"confirm" json:"confirm"`
	Recovery    *WorkflowRecovery      `yaml:"recovery,omitempty" json:"recovery,omitempty"`
	ExtraFields map[string]interface{} `yaml:",inline" json:"-"`
}

// WorkflowRecovery 是模板级失败恢复策略，不写死到执行器：
// 差异检查节点输出无变化时可跳过变更与服务重启；变更后的失败可暂停为待处理。
type WorkflowRecovery struct {
	PendingOnFailure   bool                  `yaml:"pending_on_failure,omitempty" json:"pending_on_failure,omitempty"`
	SkipChangeOnNoDiff bool                  `yaml:"skip_change_on_no_diff,omitempty" json:"skip_change_on_no_diff,omitempty"`
	DiffNodeID         string                `yaml:"diff_node_id,omitempty" json:"diff_node_id,omitempty"`
	ChangeNodeIDs      []string              `yaml:"change_node_ids,omitempty" json:"change_node_ids,omitempty"`
	ServiceNodeIDs     []string              `yaml:"service_node_ids,omitempty" json:"service_node_ids,omitempty"`
	HealthNodeIDs      []string              `yaml:"health_node_ids,omitempty" json:"health_node_ids,omitempty"`
	Notification       *WorkflowNotification `yaml:"notification,omitempty" json:"notification,omitempty"`
}

type WorkflowNotification struct {
	URL     string            `yaml:"url,omitempty" json:"url,omitempty"`
	Events  []string          `yaml:"events,omitempty" json:"events,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	Timeout string            `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

const (
	WorkflowNotifySucceeded  = "succeeded"
	WorkflowNotifyFailed     = "failed"
	WorkflowNotifyPending    = "pending"
	WorkflowNotifyRolledBack = "rolled_back"
)

type WorkflowStage struct {
	ID             string                 `yaml:"id" json:"id"`
	Name           string                 `yaml:"name" json:"name"`
	ExecutionMode  string                 `yaml:"execution_mode,omitempty" json:"execution_mode,omitempty"`
	MaxConcurrency int                    `yaml:"max_concurrency,omitempty" json:"max_concurrency,omitempty"`
	ExtraFields    map[string]interface{} `yaml:",inline" json:"-"`
}

type WorkflowNode struct {
	ID          string                 `yaml:"id" json:"id"`
	Type        string                 `yaml:"type" json:"type"`
	Name        string                 `yaml:"name" json:"name"`
	StageID     string                 `yaml:"stage_id,omitempty" json:"stage_id,omitempty"`
	Order       int                    `yaml:"order,omitempty" json:"order,omitempty"`
	Tool        string                 `yaml:"tool" json:"tool"`
	Host        string                 `yaml:"host,omitempty" json:"host,omitempty"`
	Condition   WorkflowCondition      `yaml:"condition" json:"condition"`
	Loop        WorkflowLoop           `yaml:"loop" json:"loop"`
	Upload      WorkflowUpload         `yaml:"upload" json:"upload"`
	Extract     WorkflowExtractConfig  `yaml:"extract_config" json:"extract_config"`
	HTTP        WorkflowHTTP           `yaml:"http,omitempty" json:"http"`
	Script      WorkflowScript         `yaml:"script,omitempty" json:"script,omitempty"`
	Checksum    WorkflowChecksum       `yaml:"checksum,omitempty" json:"checksum,omitempty"`
	Archive     WorkflowArchive        `yaml:"archive,omitempty" json:"archive,omitempty"`
	Wait        WorkflowWait           `yaml:"wait,omitempty" json:"wait,omitempty"`
	Notify      WorkflowNotify         `yaml:"notify,omitempty" json:"notify,omitempty"`
	DependsOn   []string               `yaml:"depends_on" json:"depends_on"`
	Params      map[string]interface{} `yaml:"params" json:"params"`
	Optional    bool                   `yaml:"optional" json:"optional"`
	Timeout     string                 `yaml:"timeout" json:"timeout"`
	Confirm     bool                   `yaml:"confirm" json:"confirm"`
	OnFailure   string                 `yaml:"on_failure" json:"on_failure"`
	ExtraFields map[string]interface{} `yaml:",inline" json:"-"`
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
	From        string                 `yaml:"from" json:"from"`
	To          string                 `yaml:"to" json:"to"`
	Case        string                 `yaml:"case" json:"case"`
	ExtraFields map[string]interface{} `yaml:",inline" json:"-"`
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
	WorkflowNodeTypeShell         = "shell"
	WorkflowNodeTypeChecksum      = "file_checksum"
	WorkflowNodeTypeArchive       = "archive"
	WorkflowNodeTypeWait          = "wait"
	WorkflowNodeTypeNotify        = "notify"

	WorkflowStageExecutionSerial   = "serial"
	WorkflowStageExecutionParallel = "parallel"

	// 旧格式工作流（只写 nodes/edges、没有 stages）归一化后使用的唯一阶段。
	LegacyWorkflowStageID   = "main"
	LegacyWorkflowStageName = "原有流程"
)

// WorkflowScript 是内置 Shell 节点：在目标机器上执行内联脚本，不要求先封装插件。
type WorkflowScript struct {
	Script     string            `yaml:"script,omitempty" json:"script,omitempty"`
	Workdir    string            `yaml:"workdir,omitempty" json:"workdir,omitempty"`
	Env        map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	FailOnExit *bool             `yaml:"fail_on_exit,omitempty" json:"fail_on_exit,omitempty"`
	Timeout    string            `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// WorkflowChecksum 对文件做 MD5/SHA256 校验，可要求与期望值一致。
type WorkflowChecksum struct {
	Path      string `yaml:"path" json:"path"`
	Algorithm string `yaml:"algorithm,omitempty" json:"algorithm,omitempty"`
	Expected  string `yaml:"expected,omitempty" json:"expected,omitempty"`
}

// WorkflowArchive 解开本地压缩包到目标目录。
type WorkflowArchive struct {
	Path      string `yaml:"path" json:"path"`
	TargetDir string `yaml:"target_dir" json:"target_dir"`
	Format    string `yaml:"format,omitempty" json:"format,omitempty"`
	Strip     int    `yaml:"strip_components,omitempty" json:"strip_components,omitempty"`
	Overwrite bool   `yaml:"overwrite,omitempty" json:"overwrite,omitempty"`
}

// WorkflowWait 固定等待或有限轮询：轮询支持 HTTP 与文件存在两种检查方式，取消可立即生效。
type WorkflowWait struct {
	Seconds      float64           `yaml:"seconds,omitempty" json:"seconds,omitempty"`
	PollURL      string            `yaml:"poll_url,omitempty" json:"poll_url,omitempty"`
	PollFile     string            `yaml:"poll_file,omitempty" json:"poll_file,omitempty"`
	ExpectStatus []int             `yaml:"expect_status,omitempty" json:"expect_status,omitempty"`
	Interval     string            `yaml:"interval,omitempty" json:"interval,omitempty"`
	MaxAttempts  int               `yaml:"max_attempts,omitempty" json:"max_attempts,omitempty"`
	MaxWait      string            `yaml:"max_wait,omitempty" json:"max_wait,omitempty"`
	Headers      map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
}

// WorkflowNotify 把上游结果推送到外部通道，作为流程收尾或失败提示。
// 标题与正文支持运行期模板；通道参数按 channel 分别校验。
type WorkflowNotify struct {
	Channel string            `yaml:"channel,omitempty" json:"channel,omitempty"`
	Title   string            `yaml:"title,omitempty" json:"title,omitempty"`
	Message string            `yaml:"message,omitempty" json:"message,omitempty"`
	URL     string            `yaml:"url,omitempty" json:"url,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	Webhook string            `yaml:"webhook,omitempty" json:"webhook,omitempty"`
	Timeout string            `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	SMTPHost string   `yaml:"smtp_host,omitempty" json:"smtp_host,omitempty"`
	SMTPPort int      `yaml:"smtp_port,omitempty" json:"smtp_port,omitempty"`
	SMTPUser string   `yaml:"smtp_user,omitempty" json:"smtp_user,omitempty"`
	SMTPPass string   `yaml:"smtp_pass,omitempty" json:"smtp_pass,omitempty"`
	SMTPTLS  string   `yaml:"smtp_tls,omitempty" json:"smtp_tls,omitempty"`
	From     string   `yaml:"from,omitempty" json:"from,omitempty"`
	To       []string `yaml:"to,omitempty" json:"to,omitempty"`
	Subject  string   `yaml:"subject,omitempty" json:"subject,omitempty"`
}

const (
	NotifyChannelWebhook = "webhook"
	NotifyChannelWeCom   = "wecom"
	NotifyChannelEmail   = "email"
	NotifyTLSStartTLS    = "starttls"
	NotifyTLSTLS         = "tls"
	NotifyTLSNone        = "none"
)

var notifyChannels = []string{NotifyChannelWebhook, NotifyChannelWeCom, NotifyChannelEmail}
var notifyTLSModes = []string{NotifyTLSStartTLS, NotifyTLSTLS, NotifyTLSNone}

// ValidateNotifyConfig 校验通知节点：通道、目标地址与通道专属参数。
func ValidateNotifyConfig(nodeID string, item WorkflowNotify) (WorkflowNotify, error) {
	out := item
	out.Channel = strings.ToLower(strings.TrimSpace(out.Channel))
	if out.Channel == "" {
		out.Channel = NotifyChannelWebhook
	}
	valid := false
	for _, channel := range notifyChannels {
		if out.Channel == channel {
			valid = true
			break
		}
	}
	if !valid {
		return out, fmt.Errorf("通知节点 %s 的 notify.channel 只支持 webhook、wecom 或 email", nodeID)
	}
	out.URL = strings.TrimSpace(out.URL)
	out.Webhook = strings.TrimSpace(out.Webhook)
	out.Timeout = strings.TrimSpace(out.Timeout)
	for name, value := range out.Headers {
		if strings.TrimSpace(name) == "" {
			return out, fmt.Errorf("通知节点 %s 的 notify.headers 存在空 header 名", nodeID)
		}
		if strings.ContainsAny(name, "\r\n") || strings.ContainsAny(value, "\r\n") {
			return out, fmt.Errorf("通知节点 %s 的 header %s 不能包含换行", nodeID, name)
		}
	}
	if out.Timeout != "" {
		if duration, err := time.ParseDuration(out.Timeout); err != nil || duration <= 0 {
			return out, fmt.Errorf("通知节点 %s 的 notify.timeout 必须是正时长，例如 10s", nodeID)
		}
	}
	switch out.Channel {
	case NotifyChannelWebhook:
		if out.URL == "" && out.Webhook == "" {
			return out, fmt.Errorf("通知节点 %s 的 Webhook 地址必填", nodeID)
		}
		for label, target := range map[string]string{"url": out.URL, "webhook": out.Webhook} {
			if target == "" || strings.Contains(target, "{{") {
				continue
			}
			if !strings.HasPrefix(strings.ToLower(target), "http://") && !strings.HasPrefix(strings.ToLower(target), "https://") {
				return out, fmt.Errorf("通知节点 %s 的 %s 只支持 http:// 或 https://", nodeID, label)
			}
		}
	case NotifyChannelWeCom:
		if out.Webhook == "" {
			return out, fmt.Errorf("通知节点 %s 的企业微信机器人 Webhook 必填", nodeID)
		}
		if out.SMTPHost != "" || len(out.To) > 0 {
			return out, fmt.Errorf("通知节点 %s 的邮件参数不能用于企业微信通道", nodeID)
		}
	case NotifyChannelEmail:
		out.SMTPHost = strings.TrimSpace(out.SMTPHost)
		out.SMTPTLS = strings.ToLower(strings.TrimSpace(out.SMTPTLS))
		if out.SMTPTLS == "" {
			out.SMTPTLS = NotifyTLSStartTLS
		}
		validTLS := false
		for _, mode := range notifyTLSModes {
			if out.SMTPTLS == mode {
				validTLS = true
				break
			}
		}
		if !validTLS {
			return out, fmt.Errorf("通知节点 %s 的 notify.smtp_tls 只支持 starttls、tls 或 none", nodeID)
		}
		if out.SMTPHost == "" {
			return out, fmt.Errorf("通知节点 %s 的邮件通道需要配置 notify.smtp_host", nodeID)
		}
		if out.SMTPPort < 0 || out.SMTPPort > 65535 {
			return out, fmt.Errorf("通知节点 %s 的 notify.smtp_port 必须在 1..65535 之间", nodeID)
		}
		if out.SMTPPort == 0 {
			if out.SMTPTLS == NotifyTLSTLS {
				out.SMTPPort = 465
			} else {
				out.SMTPPort = 587
			}
		}
		recipients := make([]string, 0, len(out.To))
		for _, item := range out.To {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if strings.Contains(item, "{{") {
				recipients = append(recipients, item)
				continue
			}
			if _, err := mail.ParseAddress(item); err != nil {
				return out, fmt.Errorf("通知节点 %s 的收件人无效: %s", nodeID, item)
			}
			recipients = append(recipients, item)
		}
		if len(recipients) == 0 {
			return out, fmt.Errorf("通知节点 %s 至少需要一个收件人", nodeID)
		}
		out.To = recipients
		out.From = strings.TrimSpace(out.From)
		if out.From == "" {
			out.From = out.SMTPUser
		}
		if strings.TrimSpace(out.From) == "" {
			return out, fmt.Errorf("通知节点 %s 的邮件通道需要配置发件人 notify.from 或 notify.smtp_user", nodeID)
		}
		if _, err := mail.ParseAddress(out.From); err != nil {
			return out, fmt.Errorf("通知节点 %s 的发件人无效: %s", nodeID, out.From)
		}
	}
	return out, nil
}

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

// NormalizeUploadTargetDir 校验上传目标目录。
// 相对路径按 runs/uploads 下的子目录处理；绝对路径只有在 allowedDirs 白名单内才被接受。
func NormalizeUploadTargetDir(value string) (string, error) {
	return NormalizeUploadTargetDirWithAllowed(value, nil)
}

func NormalizeUploadTargetDirWithAllowed(value string, allowedDirs []string) (string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", nil
	}
	if strings.Contains(raw, "://") {
		return "", fmt.Errorf("上传目标目录必须是目录路径，不能是 URL")
	}
	if isAbsoluteDirPath(raw) {
		cleanAbs, err := filepath.Abs(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(raw, "\\", "/"))))
		if err != nil {
			return "", fmt.Errorf("上传目标目录无效: %w", err)
		}
		if !dirWithinAllowed(cleanAbs, allowedDirs) {
			if len(allowedDirs) == 0 {
				return "", fmt.Errorf("上传目标目录不能是绝对路径；如需写入固定目录，请先在平台设置的上传白名单里配置")
			}
			return "", fmt.Errorf("上传目标目录不在白名单内: %s", cleanAbs)
		}
		return filepath.ToSlash(cleanAbs), nil
	}
	raw = strings.ReplaceAll(raw, "\\", "/")
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

func isAbsoluteDirPath(value string) bool {
	if len(value) >= 2 && value[1] == ':' {
		return true
	}
	normalized := strings.ReplaceAll(value, "\\", "/")
	return strings.HasPrefix(normalized, "/") || filepath.IsAbs(normalized)
}

func dirWithinAllowed(target string, allowedDirs []string) bool {
	for _, dir := range allowedDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		abs, err := filepath.Abs(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(dir, "\\", "/"))))
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(abs, target)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)) {
			return true
		}
	}
	return false
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

// ValidateScriptConfig 校验内置 Shell 节点：脚本必填，超时与工作目录格式合法。
func ValidateScriptConfig(nodeID string, item WorkflowScript) (WorkflowScript, error) {
	out := item
	if strings.TrimSpace(out.Script) == "" {
		return out, fmt.Errorf("Shell 节点 %s 的 script.script 必填", nodeID)
	}
	out.Timeout = strings.TrimSpace(out.Timeout)
	if out.Timeout != "" {
		if duration, err := time.ParseDuration(out.Timeout); err != nil || duration <= 0 {
			return out, fmt.Errorf("Shell 节点 %s 的 script.timeout 必须是正时长，例如 30s、2m", nodeID)
		}
	}
	for name := range out.Env {
		if !validScriptEnvName(name) {
			return out, fmt.Errorf("Shell 节点 %s 的 script.env 变量名 %q 只能包含字母、数字和下划线，且不能以数字开头", nodeID, name)
		}
	}
	return out, nil
}

func validScriptEnvName(name string) bool {
	if name == "" {
		return false
	}
	for index, char := range name {
		if char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

// ValidateChecksumConfig 校验文件校验节点：路径必填，算法只支持 md5/sha256。
func ValidateChecksumConfig(nodeID string, item WorkflowChecksum) (WorkflowChecksum, error) {
	out := item
	out.Path = strings.TrimSpace(out.Path)
	if out.Path == "" {
		return out, fmt.Errorf("文件校验节点 %s 的 checksum.path 必填", nodeID)
	}
	algorithm := strings.ToLower(strings.TrimSpace(out.Algorithm))
	if algorithm == "" {
		algorithm = "sha256"
	}
	if algorithm != "md5" && algorithm != "sha256" {
		return out, fmt.Errorf("文件校验节点 %s 的 checksum.algorithm 只支持 md5 或 sha256", nodeID)
	}
	out.Algorithm = algorithm
	out.Expected = strings.TrimSpace(out.Expected)
	// 模板引用保留大小写，渲染后的摘要由执行器再次校验。
	if out.Expected != "" && !strings.Contains(out.Expected, "{{") {
		out.Expected = strings.ToLower(out.Expected)
		want := 32
		if algorithm == "sha256" {
			want = 64
		}
		if len(out.Expected) != want {
			return out, fmt.Errorf("文件校验节点 %s 的 checksum.expected 长度应为 %d 位", nodeID, want)
		}
		for _, r := range out.Expected {
			if !strings.ContainsRune("0123456789abcdef", r) {
				return out, fmt.Errorf("文件校验节点 %s 的 checksum.expected 只能是十六进制", nodeID)
			}
		}
	}
	return out, nil
}

var archiveFormats = []string{"", "tar", "tar.gz", "tgz", "zip"}

// ValidateArchiveConfig 校验解压节点：源文件与目标目录必填，格式需受支持。
func ValidateArchiveConfig(nodeID string, item WorkflowArchive) (WorkflowArchive, error) {
	out := item
	out.Path = strings.TrimSpace(out.Path)
	out.TargetDir = strings.TrimSpace(out.TargetDir)
	if out.Path == "" {
		return out, fmt.Errorf("解压节点 %s 的 archive.path 必填", nodeID)
	}
	if out.TargetDir == "" {
		return out, fmt.Errorf("解压节点 %s 的 archive.target_dir 必填", nodeID)
	}
	out.Format = strings.ToLower(strings.TrimSpace(out.Format))
	valid := false
	for _, format := range archiveFormats {
		if out.Format == format {
			valid = true
			break
		}
	}
	if !valid {
		return out, fmt.Errorf("解压节点 %s 的 archive.format 只支持 tar、tar.gz、tgz、zip 或留空自动识别", nodeID)
	}
	if out.Strip < 0 {
		return out, fmt.Errorf("解压节点 %s 的 archive.strip_components 不能为负数", nodeID)
	}
	return out, nil
}

// ValidateWaitConfig 校验等待/轮询节点：固定等待或轮询二选一，轮询必须有结束条件。
func ValidateWaitConfig(nodeID string, item WorkflowWait) (WorkflowWait, error) {
	out := item
	out.PollURL = strings.TrimSpace(out.PollURL)
	out.PollFile = strings.TrimSpace(out.PollFile)
	out.Interval = strings.TrimSpace(out.Interval)
	out.MaxWait = strings.TrimSpace(out.MaxWait)
	if math.IsNaN(out.Seconds) || math.IsInf(out.Seconds, 0) || out.Seconds < 0 {
		return out, fmt.Errorf("等待节点 %s 的 wait.seconds 必须是有限的非负数", nodeID)
	}
	if out.MaxAttempts < 0 {
		return out, fmt.Errorf("等待节点 %s 的 wait.max_attempts 不能为负数", nodeID)
	}
	if out.PollURL != "" && out.PollFile != "" {
		return out, fmt.Errorf("等待节点 %s 的 wait.poll_url 与 wait.poll_file 只能配置一个", nodeID)
	}
	polling := out.PollURL != "" || out.PollFile != ""
	if !polling {
		if out.Seconds <= 0 {
			return out, fmt.Errorf("等待节点 %s 需要配置固定等待秒数或轮询条件", nodeID)
		}
		if out.Seconds > 3600 {
			return out, fmt.Errorf("等待节点 %s 的 wait.seconds 不能超过 3600", nodeID)
		}
		return out, nil
	}
	if out.Seconds > 0 {
		return out, fmt.Errorf("等待节点 %s 不能同时配置固定等待与轮询", nodeID)
	}
	if out.Interval != "" {
		if duration, err := time.ParseDuration(out.Interval); err != nil || duration <= 0 {
			return out, fmt.Errorf("等待节点 %s 的 wait.interval 必须是正时长，例如 5s、1m", nodeID)
		}
	}
	if out.MaxWait != "" {
		if duration, err := time.ParseDuration(out.MaxWait); err != nil || duration <= 0 {
			return out, fmt.Errorf("等待节点 %s 的 wait.max_wait 必须是正时长，例如 5m", nodeID)
		}
	}
	if out.PollURL != "" {
		lower := strings.ToLower(out.PollURL)
		if !strings.Contains(out.PollURL, "{{") && !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			return out, fmt.Errorf("等待节点 %s 的 wait.poll_url 只支持 http:// 或 https://", nodeID)
		}
		for _, code := range out.ExpectStatus {
			if code < 100 || code > 599 {
				return out, fmt.Errorf("等待节点 %s 的 wait.expect_status 必须在 100..599 之间", nodeID)
			}
		}
	}
	return out, nil
}

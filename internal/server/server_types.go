package server

// 本文件集中存放 catalog / run / log 等 API 响应类型。
import (
	"context"
	"sync"

	"shell_ops/internal/config"
	"shell_ops/internal/market"
	"shell_ops/internal/plugin"
	"shell_ops/internal/registry"
	"shell_ops/internal/runner"
	"shell_ops/internal/sshmgr"
)

type catalogResponse struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Categories  []categoryCatalogEntry `json:"categories"`
	Tools       []toolCatalogEntry     `json:"tools"`
	Workflows   []workflowCatalogEntry `json:"workflows"`
	Plugins     []pluginCatalogEntry   `json:"plugins"`
	Warnings    []plugin.Warning       `json:"warnings,omitempty"`
}

type categoryCatalogEntry struct {
	config.Category
	Disabled bool             `json:"disabled"`
	Source   *registry.Source `json:"source,omitempty"`
}

type toolCatalogEntry struct {
	config.ToolEntry
	Tags           []string               `json:"tags"`
	Execution      config.ExecutionConfig `json:"execution"`
	Parameters     []config.Parameter     `json:"parameters"`
	Outputs        []config.ToolOutput    `json:"outputs"`
	ConfigFiles    []string               `json:"config_files"`
	ConfigFileRefs []config.ConfigFileRef `json:"config_file_entries,omitempty"`
	Confirm        config.Confirmation    `json:"confirm"`
	Source         registry.Source        `json:"source"`
}

type workflowCatalogEntry struct {
	config.WorkflowRef
	Tags              []string                   `json:"tags"`
	SharedWith        []string                   `json:"shared_with,omitempty"`
	Parameters        []config.Parameter         `json:"parameters"`
	ConfigFiles       []config.ConfigFileRef     `json:"config_files,omitempty"`
	ReferencedPlugins []workflowReferencedPlugin `json:"referenced_plugins,omitempty"`
	Confirm           config.Confirmation        `json:"confirm"`
	Source            registry.Source            `json:"source"`
	Dependencies      []workflowDependency       `json:"dependencies,omitempty"`
	MissingTools      []string                   `json:"missing_tools,omitempty"`
}

// workflowReferencedPlugin 描述工作流节点引用到的插件，以及该插件声明的配置文件。
// 配置文件仍由插件持有，这里只是把它带到工作流配置视图里，方便就地查看与跳转编辑。
type workflowReferencedPlugin struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name,omitempty"`
	Version     string                 `json:"version,omitempty"`
	Disabled    bool                   `json:"disabled,omitempty"`
	Tools       []string               `json:"tools,omitempty"`
	ConfigFiles []config.ConfigFileRef `json:"config_files,omitempty"`
}

type workflowDependency struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Version  string `json:"version,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

type pluginCatalogEntry struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description,omitempty"`
	Disabled    bool              `json:"disabled"`
	Categories  []config.Category `json:"categories,omitempty"`
	Warnings    []plugin.Warning  `json:"warnings,omitempty"`
}

type runRequest struct {
	Params        map[string]interface{}                 `json:"params"`
	Confirm       bool                                   `json:"confirm"`
	Target        string                                 `json:"target,omitempty"`
	OverrideNodes bool                                   `json:"override_nodes,omitempty"`
	Targets       []string                               `json:"targets,omitempty"`
	IncludeLocal  bool                                   `json:"include_local,omitempty"`
	Workflow      *config.WorkflowConfig                 `json:"workflow,omitempty"`
	Uploads       map[string]config.WorkflowUploadResult `json:"uploads,omitempty"`
	ConfigVersion string                                 `json:"config_version,omitempty"`
}

type workflowSaveRequest struct {
	Workflow      config.WorkflowConfig `json:"workflow"`
	OwnerPluginID *string               `json:"owner_plugin_id"`
}

type workflowValidation struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

type runLogs struct {
	Stdout   string                    `json:"stdout,omitempty"`
	Stderr   string                    `json:"stderr,omitempty"`
	Steps    map[string]runLogs        `json:"steps,omitempty"`
	Items    []runLogItem              `json:"items,omitempty"`
	Attempts map[string]runAttemptLogs `json:"attempts,omitempty"`
}

// runAttemptLogs 保留恢复动作自己的证据，和原始步骤日志分开返回。
type runAttemptLogs struct {
	Summary string            `json:"summary,omitempty"`
	Stdout  map[string]string `json:"stdout,omitempty"`
	Stderr  map[string]string `json:"stderr,omitempty"`
}

type runLogItem struct {
	ID        string       `json:"id"`
	Kind      string       `json:"kind"`
	Title     string       `json:"title"`
	Status    string       `json:"status,omitempty"`
	Type      string       `json:"type,omitempty"`
	Tool      string       `json:"tool,omitempty"`
	HostID    string       `json:"host_id,omitempty"`
	HostName  string       `json:"host_name,omitempty"`
	Stdout    string       `json:"stdout,omitempty"`
	Stderr    string       `json:"stderr,omitempty"`
	Children  []runLogItem `json:"children,omitempty"`
	Iteration int          `json:"iteration,omitempty"`
}

type runLogEvent struct {
	Seq       int    `json:"seq"`
	RunID     string `json:"run_id"`
	ItemID    string `json:"item_id"`
	Kind      string `json:"kind"`
	StepID    string `json:"step_id,omitempty"`
	Iteration int    `json:"iteration,omitempty"`
	Stream    string `json:"stream"`
	Line      int    `json:"line"`
	Text      string `json:"text"`
	Status    string `json:"status"`
}

type runDetail struct {
	Record runner.RunRecord `json:"record"`
	Logs   runLogs          `json:"logs"`
	// Actionable 表示这条运行由当前服务进程执行；为 false 时审批与取消都无法在这里生效。
	Actionable *bool `json:"actionable,omitempty"`
}

type response struct {
	ID     string      `json:"id,omitempty"`
	Status string      `json:"status,omitempty"`
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
}

type serverState struct {
	mu             sync.RWMutex
	reg            *registry.Registry
	market         *market.Client
	ssh            *sshmgr.Manager
	marketSourceMu sync.Mutex
	marketBuildMu  sync.Mutex
	nexusMu        sync.Mutex
	runMu          sync.Mutex
	runFileMu      sync.Mutex
	active         map[string]context.CancelFunc
}

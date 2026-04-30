package config

import (
	"strconv"
	"time"
)

type RootConfig struct {
	App         AppConfig      `yaml:"app" json:"app"`
	Paths       PathsConfig    `yaml:"paths" json:"paths"`
	Server      ServerConfig   `yaml:"server" json:"server"`
	Menu        MenuConfig     `yaml:"menu" json:"menu"`
	Registry    RegistryConfig `yaml:"registry" json:"registry"`
	Plugins     PluginsConfig  `yaml:"plugins" json:"plugins"`
	UI          UIConfig       `yaml:"ui" json:"ui"`
	Name        string         `yaml:"name" json:"-"`
	Description string         `yaml:"description" json:"-"`
	Categories  []Category     `yaml:"categories" json:"-"`
	Tools       []ToolEntry    `yaml:"tools" json:"-"`
	Workflows   []WorkflowRef  `yaml:"workflows" json:"-"`
	HTTP        HTTPConfig     `yaml:"http" json:"-"`
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
	Paths    []string `yaml:"paths" json:"paths"`
	Strict   bool     `yaml:"strict" json:"strict"`
	Disabled []string `yaml:"disabled" json:"disabled"`
}

type UIConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Title   string `yaml:"title" json:"title"`
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
	ID           string            `yaml:"id" json:"id"`
	Name         string            `yaml:"name" json:"name"`
	Description  string            `yaml:"description" json:"description"`
	Version      string            `yaml:"version" json:"version"`
	Category     string            `yaml:"category" json:"category"`
	Tags         []string          `yaml:"tags" json:"tags"`
	Help         HelpConfig        `yaml:"help" json:"help"`
	Entry        string            `yaml:"entry" json:"entry"`
	Execution    ExecutionConfig   `yaml:"execution" json:"execution"`
	Parameters   []Parameter       `yaml:"parameters" json:"parameters"`
	PassMode     PassMode          `yaml:"pass_mode" json:"pass_mode"`
	Timeout      string            `yaml:"timeout" json:"timeout"`
	Confirm      Confirmation      `yaml:"confirm" json:"confirm"`
	Confirmation Confirmation      `yaml:"confirmation" json:"-"`
	Env          map[string]string `yaml:"env" json:"env"`
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
}

type WorkflowConfig struct {
	ID          string         `yaml:"id" json:"id"`
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description" json:"description"`
	Version     string         `yaml:"version" json:"version"`
	Category    string         `yaml:"category" json:"category"`
	Tags        []string       `yaml:"tags" json:"tags"`
	Parameters  []Parameter    `yaml:"parameters" json:"parameters"`
	Nodes       []WorkflowNode `yaml:"nodes" json:"nodes"`
	Edges       []WorkflowEdge `yaml:"edges" json:"edges"`
	Steps       []WorkflowNode `yaml:"steps" json:"-"`
	Confirm     Confirmation   `yaml:"confirm" json:"confirm"`
}

type WorkflowNode struct {
	ID        string                 `yaml:"id" json:"id"`
	Type      string                 `yaml:"type" json:"type"`
	Name      string                 `yaml:"name" json:"name"`
	Tool      string                 `yaml:"tool" json:"tool"`
	Condition WorkflowCondition      `yaml:"condition" json:"condition"`
	Loop      WorkflowLoop           `yaml:"loop" json:"loop"`
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
	WorkflowNodeTypeTool      = "tool"
	WorkflowNodeTypeCondition = "condition"
	WorkflowNodeTypeParallel  = "parallel"
	WorkflowNodeTypeJoin      = "join"
	WorkflowNodeTypeLoop      = "loop"
)

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
	if len(c.Menu.Categories) > 0 {
		return c.Menu.Categories
	}
	return c.Categories
}

func (c RootConfig) ListenAddr() string {
	if c.Server.Port > 0 {
		host := c.Server.Host
		if host == "" {
			host = "0.0.0.0"
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

package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

// ToolSnapshotBinding 只保存执行定义与入口的摘要，避免把工具配置中的凭据写入快照。
type ToolSnapshotBinding struct {
	Version      string `json:"version,omitempty"`
	ConfigSHA256 string `json:"config_sha256,omitempty"`
	EntrySHA256  string `json:"entry_sha256,omitempty"`
	Unavailable  bool   `json:"unavailable,omitempty"`
}

// remoteHostIdentityProvider 由生产 SSH 执行器提供；轻量测试执行器仍可只实现 RemoteExecutor。
type remoteHostIdentityProvider interface {
	HostIdentity(hostID string) (string, error)
}

func (r *Runner) captureRunSnapshotBindings(snapshot *RunSnapshot) {
	if snapshot == nil || snapshot.Workflow == nil {
		return
	}
	snapshot.Tools = make(map[string]ToolSnapshotBinding)
	for _, id := range snapshotToolIDs(snapshot.Workflow) {
		snapshot.Tools[id] = r.toolSnapshotBinding(id)
	}
	// 回滚工具不是正常运行的必备依赖，只固定运行开始时已经安装的版本。
	if _, err := r.Registry.Tool("config.deploy.rollback"); err == nil {
		snapshot.Tools["config.deploy.rollback"] = r.toolSnapshotBinding("config.deploy.rollback")
	}
	snapshot.HostIdentities = make(map[string]string)
	for _, id := range snapshotHostIDs(snapshot.Workflow) {
		identity, _ := r.snapshotHostIdentity(id)
		snapshot.HostIdentities[id] = identity
	}
}

func (r *Runner) validateRunSnapshotBindings(snapshot *RunSnapshot) error {
	if snapshot == nil || snapshot.Workflow == nil {
		return fmt.Errorf("运行快照缺失，无法安全恢复")
	}
	for _, id := range snapshotToolIDs(snapshot.Workflow) {
		if _, ok := snapshot.Tools[id]; !ok {
			return fmt.Errorf("运行快照未固定工具 %s 的版本和入口，无法安全恢复", id)
		}
	}
	for _, id := range sortedSnapshotToolIDs(snapshot.Tools) {
		previous := snapshot.Tools[id]
		current := r.toolSnapshotBinding(id)
		if previous.Unavailable || previous.ConfigSHA256 == "" || previous.EntrySHA256 == "" {
			return fmt.Errorf("运行快照无法确认工具 %s 的执行定义，无法安全恢复", id)
		}
		if current.Unavailable || previous != current {
			return fmt.Errorf("工具 %s 的版本、配置或入口已变更，无法按原运行快照恢复", id)
		}
	}
	for _, id := range snapshotHostIDs(snapshot.Workflow) {
		previous := snapshot.HostIdentities[id]
		if previous == "" {
			return fmt.Errorf("运行快照未固定操作机 %s 的连接身份，无法安全恢复", id)
		}
		current, err := r.snapshotHostIdentity(id)
		if err != nil {
			return fmt.Errorf("无法确认操作机 %s 的连接身份：%w", id, err)
		}
		if previous != current {
			return fmt.Errorf("操作机 %s 的连接身份已变更，无法按原运行快照恢复", id)
		}
	}
	return nil
}

func (r *Runner) toolSnapshotBinding(id string) ToolSnapshotBinding {
	tool, err := r.Registry.Tool(id)
	if err != nil || tool.Config == nil {
		return ToolSnapshotBinding{Unavailable: true}
	}
	binding := ToolSnapshotBinding{Version: tool.Config.Version}
	plugin := tool.Config.PluginConfig
	// PluginConfig 的 JSON 字段被隐藏，需显式包含参与配置解析的值。
	definition := struct {
		Config    *config.ToolConfig
		Source    registry.Source
		Dir       string
		Defaults  config.Values
		GlobalEnv config.Values
		Plugin    map[string]interface{}
	}{
		Config: tool.Config, Source: tool.Source, Dir: tool.Dir,
		Defaults: r.Registry.Root.ConfigDefaults, GlobalEnv: r.Registry.GlobalEnv,
		Plugin: map[string]interface{}{
			"id": plugin.ID, "dir": plugin.Dir, "shared": plugin.SharedConfig,
			"defaults": plugin.PackageDefaultConfig, "host": plugin.HostConfig,
			"sensitive_paths": plugin.SensitivePaths,
		},
	}
	data, err := json.Marshal(definition)
	if err != nil {
		binding.Unavailable = true
		return binding
	}
	digest := sha256.Sum256(data)
	binding.ConfigSHA256 = hex.EncodeToString(digest[:])
	entry := resolveEntry(tool.Dir, tool.Config.Execution.Entry)
	if entry == "" {
		binding.Unavailable = true
		return binding
	}
	if !strings.ContainsAny(entry, `/\\`) {
		entry, err = exec.LookPath(entry)
		if err != nil {
			binding.Unavailable = true
			return binding
		}
	}
	file, err := os.Open(entry)
	if err != nil {
		binding.Unavailable = true
		return binding
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		binding.Unavailable = true
		return binding
	}
	binding.EntrySHA256 = hex.EncodeToString(hash.Sum(nil))
	return binding
}

func (r *Runner) snapshotHostIdentity(id string) (string, error) {
	if r.remote == nil {
		return "", fmt.Errorf("当前运行器未配置远程执行能力")
	}
	if provider, ok := r.remote.(remoteHostIdentityProvider); ok {
		return provider.HostIdentity(id)
	}
	host, err := r.remote.DescribeHost(id)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(host)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func snapshotToolIDs(wf *config.WorkflowConfig) []string {
	ids := map[string]bool{}
	for _, node := range wf.Nodes {
		if id := strings.TrimSpace(node.Tool); id != "" {
			ids[id] = true
		}
		if id := strings.TrimSpace(node.Loop.Tool); id != "" {
			ids[id] = true
		}
	}
	return sortedSnapshotIDs(ids)
}

func snapshotHostIDs(wf *config.WorkflowConfig) []string {
	ids := map[string]bool{}
	for _, node := range wf.Nodes {
		if id := strings.TrimSpace(node.Host); id != "" && id != LocalTargetID {
			ids[id] = true
		}
	}
	return sortedSnapshotIDs(ids)
}

func sortedSnapshotToolIDs(tools map[string]ToolSnapshotBinding) []string {
	ids := make(map[string]bool, len(tools))
	for id := range tools {
		ids[id] = true
	}
	return sortedSnapshotIDs(ids)
}

func sortedSnapshotIDs(values map[string]bool) []string {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

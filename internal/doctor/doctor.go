package doctor

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"shell_ops/internal/registry"
)

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	BaseDir     string    `json:"base_dir"`
	AppName     string    `json:"app_name,omitempty"`
	AppVersion  string    `json:"app_version,omitempty"`
	Checks      []Check   `json:"checks"`
}

func Run(reg *registry.Registry) Report {
	report := Report{
		GeneratedAt: time.Now(),
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		BaseDir:     reg.BaseDir,
		AppName:     reg.Root.DisplayName(),
		AppVersion:  reg.Root.App.Version,
	}
	report.Checks = append(report.Checks,
		checkPath("配置文件", filepath.Join(reg.BaseDir, "configs", "ops.yaml"), false),
		checkDirWritable("运行目录", filepath.Join(reg.BaseDir, filepath.FromSlash(reg.Root.Paths.Runs))),
		checkDirWritable("日志目录", filepath.Join(reg.BaseDir, filepath.FromSlash(reg.Root.Paths.Logs))),
		checkPlugins(reg),
		checkExecutable("bash"),
		checkExecutable("powershell"),
		checkListenAddress(reg.Root.ListenAddr()),
	)
	return report
}

func WriteReport(reg *registry.Registry, report Report) (string, error) {
	outDir := filepath.Join(reg.BaseDir, filepath.FromSlash(reg.Root.Paths.Runs))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(outDir, "doctor-report.json")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

func HasFailure(report Report) bool {
	for _, check := range report.Checks {
		if check.Status == "failed" {
			return true
		}
	}
	return false
}

func checkPath(name, path string, dir bool) Check {
	info, err := os.Stat(path)
	if err != nil {
		return Check{Name: name, Status: "failed", Message: err.Error()}
	}
	if dir && !info.IsDir() {
		return Check{Name: name, Status: "failed", Message: "不是目录"}
	}
	if !dir && !info.Mode().IsRegular() {
		return Check{Name: name, Status: "failed", Message: "不是普通文件"}
	}
	return Check{Name: name, Status: "ok", Message: path}
}

func checkDirWritable(name, path string) Check {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return Check{Name: name, Status: "failed", Message: err.Error()}
	}
	tmp, err := os.CreateTemp(path, ".opsctl-doctor-*")
	if err != nil {
		return Check{Name: name, Status: "failed", Message: "不可写: " + err.Error()}
	}
	tmpName := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(tmpName)
	return Check{Name: name, Status: "ok", Message: path}
}

func checkPlugins(reg *registry.Registry) Check {
	if len(reg.Tools) == 0 && len(reg.Workflows) == 0 {
		return Check{Name: "插件目录", Status: "warning", Message: "未加载任何工具或工作流"}
	}
	message := fmt.Sprintf("工具 %d 个，工作流 %d 个", len(reg.Tools), len(reg.Workflows))
	if len(reg.Warnings) > 0 {
		return Check{Name: "插件目录", Status: "warning", Message: fmt.Sprintf("%s，警告 %d 条", message, len(reg.Warnings))}
	}
	return Check{Name: "插件目录", Status: "ok", Message: message}
}

func checkExecutable(name string) Check {
	path, err := exec.LookPath(name)
	if err != nil {
		return Check{Name: name, Status: "warning", Message: "未找到，可忽略不需要该运行时的插件"}
	}
	return Check{Name: name, Status: "ok", Message: path}
}

func checkListenAddress(addr string) Check {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return Check{Name: "监听地址", Status: "warning", Message: "无法解析监听地址: " + addr}
	}
	if host == "" {
		host = "0.0.0.0"
	}
	status := "ok"
	message := addr
	if host == "0.0.0.0" || host == "::" {
		status = "warning"
		message = "监听所有网卡，请确认只在可信网络中使用: " + addr
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return Check{Name: "监听端口", Status: "warning", Message: err.Error()}
	}
	_ = ln.Close()
	return Check{Name: "监听端口", Status: status, Message: strings.TrimSpace(message)}
}

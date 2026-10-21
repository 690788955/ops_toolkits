package sshmgr

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type fakeProber struct {
	check  func(host Host) (ProbeResult, error)
	exec   func(host Host, command string) (ExecResult, error)
	stream func(host Host, command string, stdin io.Reader) (StreamResult, error)
}

func (f *fakeProber) Check(_ context.Context, host Host, _ time.Duration) (ProbeResult, error) {
	if f.check == nil {
		return ProbeResult{Latency: 12 * time.Millisecond, Fingerprint: "SHA256:test"}, nil
	}
	return f.check(host)
}

func (f *fakeProber) Exec(_ context.Context, host Host, _ time.Duration, command string) (ExecResult, error) {
	if f.exec == nil {
		return ExecResult{Stdout: command, ExitCode: 0}, nil
	}
	return f.exec(host, command)
}

func (f *fakeProber) Stream(_ context.Context, host Host, _ time.Duration, command string, stdin io.Reader, _, _ io.Writer) (StreamResult, error) {
	if f.stream == nil {
		return StreamResult{ExitCode: 0}, nil
	}
	return f.stream(host, command, stdin)
}

func TestManagerCreateUpdateDelete(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(dir)
	manager.probe = &fakeProber{}

	view, err := manager.Create(HostInput{
		Name:     "Web 01",
		Address:  "10.0.0.10",
		Port:     22,
		User:     "root",
		Auth:     AuthPassword,
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	if view.ID == "" || !view.HasPassword || view.Status != StateUnknown {
		t.Fatalf("unexpected created view: %+v", view)
	}
	configPath := filepath.Join(dir, "configs", configFileName)
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(content), "secret") {
		t.Fatalf("config should persist password: %s", content)
	}
	if info, err := os.Stat(configPath); err == nil && runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config permissions = %o, want no group/other access", info.Mode().Perm())
	}

	overview, err := manager.Overview()
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	encoded, _ := json.Marshal(overview)
	if strings.Contains(string(encoded), `"secret"`) {
		t.Fatalf("overview must not expose password: %s", encoded)
	}

	updated, err := manager.Update(view.ID, HostInput{
		Name:    "Web 01 renamed",
		Address: "10.0.0.10",
		Port:    2222,
		User:    "root",
		Auth:    AuthPassword,
	})
	if err != nil {
		t.Fatalf("update host: %v", err)
	}
	if updated.Name != "Web 01 renamed" || updated.Port != 2222 || !updated.HasPassword {
		t.Fatalf("unexpected updated view: %+v", updated)
	}

	if err := manager.UpdateSettings(SettingsInput{IntervalSeconds: 15, TimeoutSeconds: 5}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	overview, err = manager.Overview()
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.IntervalSeconds != 15 || overview.TimeoutSeconds != 5 {
		t.Fatalf("settings = %d/%d, want 15/5", overview.IntervalSeconds, overview.TimeoutSeconds)
	}

	if err := manager.Delete(view.ID); err != nil {
		t.Fatalf("delete host: %v", err)
	}
	overview, err = manager.Overview()
	if err != nil {
		t.Fatalf("overview after delete: %v", err)
	}
	if len(overview.Hosts) != 0 {
		t.Fatalf("hosts after delete = %d, want 0", len(overview.Hosts))
	}
}

func TestManagerBundleBaseDirValidationAndNormalization(t *testing.T) {
	manager := NewManager(t.TempDir())
	view, err := manager.Create(HostInput{
		Address:       "10.0.0.10",
		User:          "root",
		Auth:          AuthKey,
		BundleBaseDir: " /data/ops/../opsctl/ ",
	})
	if err != nil {
		t.Fatalf("create host with bundle base dir: %v", err)
	}
	if view.BundleBaseDir != "/data/opsctl" {
		t.Fatalf("bundle base dir = %q, want /data/opsctl", view.BundleBaseDir)
	}

	marshaled, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(marshaled), `"bundle_base_dir":"/data/opsctl"`) {
		t.Fatalf("view should expose bundle_base_dir: %s", marshaled)
	}

	for _, invalid := range []string{"data/opsctl", `C:\opsctl`, "/", "/data/\nopsctl"} {
		if _, err := manager.Create(HostInput{
			Address:       "10.0.0.11",
			User:          "root",
			Auth:          AuthKey,
			BundleBaseDir: invalid,
		}); err == nil {
			t.Fatalf("expected bundle base dir %q to be rejected", invalid)
		}
	}

	updated, err := manager.Update(view.ID, HostInput{
		Address:       "10.0.0.10",
		User:          "root",
		Auth:          AuthKey,
		BundleBaseDir: "",
	})
	if err != nil {
		t.Fatalf("clear bundle base dir: %v", err)
	}
	if updated.BundleBaseDir != "" {
		t.Fatalf("bundle base dir = %q, want empty", updated.BundleBaseDir)
	}
}

func TestManagerValidation(t *testing.T) {
	manager := NewManager(t.TempDir())
	if _, err := manager.Create(HostInput{Address: "10.0.0.10", User: "root", Auth: AuthPassword}); err == nil {
		t.Fatal("expected missing password error")
	}
	if _, err := manager.Create(HostInput{Address: "10.0.0.10:22", User: "root", Auth: AuthKey}); err == nil {
		t.Fatal("expected address with port error")
	}
	if _, err := manager.Create(HostInput{Address: "10.0.0.10", User: "root", Auth: "unknown"}); err == nil {
		t.Fatal("expected unsupported auth error")
	}
	if err := manager.UpdateSettings(SettingsInput{IntervalSeconds: 1, TimeoutSeconds: 8}); err == nil {
		t.Fatal("expected invalid interval error")
	}
}

func TestManagerCheckUpdatesStatus(t *testing.T) {
	manager := NewManager(t.TempDir())
	manager.probe = &fakeProber{
		check: func(host Host) (ProbeResult, error) {
			if host.ID == "offline" {
				return ProbeResult{Latency: 5 * time.Millisecond}, errors.New("connection refused")
			}
			return ProbeResult{Latency: 18 * time.Millisecond, Fingerprint: "SHA256:abc"}, nil
		},
	}
	view, err := manager.Create(HostInput{Name: "Online", Address: "10.0.0.10", User: "root", Auth: AuthKey})
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	checked, err := manager.Check(context.Background(), view.ID)
	if err != nil {
		t.Fatalf("check host: %v", err)
	}
	if checked.Status != StateOnline || checked.LatencyMS != 18 || checked.Fingerprint != "SHA256:abc" {
		t.Fatalf("unexpected checked view: %+v", checked)
	}

	failing, err := manager.Create(HostInput{Name: "Offline", Address: "10.0.0.11", User: "root", Auth: AuthKey})
	if err != nil {
		t.Fatalf("create failing host: %v", err)
	}
	manager.probe.(*fakeProber).check = func(host Host) (ProbeResult, error) {
		return ProbeResult{Latency: 5 * time.Millisecond}, errors.New("connection refused")
	}
	checked, err = manager.Check(context.Background(), failing.ID)
	if err != nil {
		t.Fatalf("check failing host: %v", err)
	}
	if checked.Status != StateOffline || !strings.Contains(checked.LastError, "connection refused") {
		t.Fatalf("unexpected offline view: %+v", checked)
	}
}

func TestManagerScanAllSkipsDisabledHosts(t *testing.T) {
	manager := NewManager(t.TempDir())
	manager.probe = &fakeProber{}
	disabled := false
	view, err := manager.Create(HostInput{Name: "Disabled", Address: "10.0.0.12", User: "root", Auth: AuthKey, Enabled: &disabled})
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	manager.scanAll()
	overview, err := manager.Overview()
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(overview.Hosts) != 1 || overview.Hosts[0].ID != view.ID {
		t.Fatalf("unexpected hosts: %+v", overview.Hosts)
	}
	if overview.Hosts[0].Status != StateUnknown {
		t.Fatalf("disabled host status = %s, want unknown", overview.Hosts[0].Status)
	}
}

func TestManagerExecUpdatesStatus(t *testing.T) {
	manager := NewManager(t.TempDir())
	manager.probe = &fakeProber{
		exec: func(host Host, command string) (ExecResult, error) {
			return ExecResult{Stdout: "remote:" + command, LatencyMS: 9, Fingerprint: "SHA256:def"}, nil
		},
	}
	view, err := manager.Create(HostInput{Name: "Exec", Address: "10.0.0.13", User: "root", Auth: AuthKey})
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	result, err := manager.Exec(context.Background(), view.ID, "uptime")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.Stdout != "remote:uptime" {
		t.Fatalf("stdout = %q", result.Stdout)
	}
	checked := manager.hostView(Host{ID: view.ID})
	if checked.Status != StateOnline || checked.LatencyMS != 9 {
		t.Fatalf("unexpected status after exec: %+v", checked)
	}
}

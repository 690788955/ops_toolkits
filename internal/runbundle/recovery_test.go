package runbundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
	"shell_ops/internal/runner"
)

func TestExportRedactsRecoverySnapshotWithoutChangingSource(t *testing.T) {
	reg := &registry.Registry{BaseDir: t.TempDir(), Root: &config.RootConfig{Paths: config.PathsConfig{Runs: "runs", Logs: "runs/logs"}}}
	record := runner.RunRecord{
		ID: "recovery-export", Kind: "workflow", Target: "demo.flow", Status: "pending",
		Params: map[string]string{"access_code": "opaque-credential", "password": "snapshot-secret-value", "name": "demo"},
		Config: map[string]interface{}{"access_code": "opaque-credential"},
		Snapshot: &runner.RunSnapshot{
			Params: map[string]string{"access_code": "opaque-credential", "password": "snapshot-secret-value", "name": "demo"},
			Workflow: &config.WorkflowConfig{
				ID:          "demo.flow",
				Parameters:  []config.Parameter{{Name: "access_code", Sensitive: true, Default: "default-credential"}},
				ExtraFields: map[string]interface{}{"x_options": map[string]interface{}{"password": "workflow-extension-credential", "label": "workflow-visible"}},
				Stages:      []config.WorkflowStage{{ID: "verify", ExtraFields: map[string]interface{}{"x_auth": map[string]interface{}{"token": "stage-extension-credential", "label": "stage-visible"}}}},
				Edges:       []config.WorkflowEdge{{From: "check", To: "done", ExtraFields: map[string]interface{}{"x_route": map[string]interface{}{"secret": "edge-extension-credential", "label": "edge-visible"}}}},
				Nodes: []config.WorkflowNode{{
					ID: "check", Params: map[string]interface{}{"password": "node-credential"},
					HTTP:   config.WorkflowHTTP{Headers: map[string]string{"Authorization": "header-credential"}},
					Script: config.WorkflowScript{Env: map[string]string{"APP_CREDENTIAL": "shell-env-credential"}},
					Wait:   config.WorkflowWait{Headers: map[string]string{"X-Access": "poll-credential"}},
					ExtraFields: map[string]interface{}{"x_accounts": []interface{}{
						map[string]interface{}{"password": "array-extension-credential", "name": "array-visible"},
						[]interface{}{map[string]interface{}{"api_token": "nested-array-credential"}},
					}},
				}},
				Recovery: &config.WorkflowRecovery{Notification: &config.WorkflowNotification{URL: "https://example.invalid/hook", Headers: map[string]string{"Authorization": "webhook-credential"}}},
			},
		},
	}
	runDir, err := RunDir(reg, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]interface{}{"result.json": record, "snapshot.json": record.Snapshot} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := json.Marshal(record)
	temporaryScriptDir := filepath.Join(runDir, "shell", ".builtin")
	if err := os.MkdirAll(temporaryScriptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temporaryScriptDir, "run.sh"), []byte("echo rendered-shell-credential"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = redactedRecordJSON(record)
	after, _ := json.Marshal(record)
	if !bytes.Equal(before, after) {
		t.Fatal("导出脱敏修改了原运行快照")
	}
	data, err := ExportZip(reg, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	seenSnapshot := false
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"opaque-credential", "snapshot-secret-value", "default-credential", "node-credential", "header-credential", "webhook-credential", "shell-env-credential", "poll-credential", "rendered-shell-credential", "workflow-extension-credential", "stage-extension-credential", "edge-extension-credential", "array-extension-credential", "nested-array-credential"} {
			if strings.Contains(string(body), secret) {
				t.Errorf("%s 泄漏敏感值 %q", file.Name, secret)
			}
		}
		if file.Name == "run/snapshot.json" {
			seenSnapshot = true
			for _, visible := range []string{"demo.flow", "workflow-visible", "stage-visible", "edge-visible", "array-visible"} {
				if !bytes.Contains(body, []byte(visible)) {
					t.Errorf("快照丢失非敏感信息 %q", visible)
				}
			}
		}
	}
	if !seenSnapshot {
		t.Fatal("导出遗漏运行快照")
	}
}

func TestCleanupKeepsPendingAndActiveRecoveryEvidence(t *testing.T) {
	reg := &registry.Registry{BaseDir: t.TempDir(), Root: &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}}}
	for _, record := range []runner.RunRecord{
		{ID: "pending", Status: "pending"},
		{ID: "running", Status: "running"},
		{ID: "interrupted", Status: "failed", Pending: &runner.PendingState{Status: "pending"}},
		{ID: "recovering", Status: "failed", Attempts: []runner.AttemptRecord{{Status: "running"}}},
		{ID: "done", Status: "succeeded"},
	} {
		runDir, err := RunDir(reg, record.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(record)
		if err := os.WriteFile(filepath.Join(runDir, "result.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Cleanup(reg, CleanupOptions{Keep: 0, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.KeptIDs) != 4 || len(result.DeletedIDs) != 1 || result.DeletedIDs[0] != "done" {
		t.Fatalf("清理计划包含未完成的恢复证据: %+v", result)
	}
}

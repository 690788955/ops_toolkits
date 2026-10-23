package runner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell_ops/internal/config"
)

func pendingRecoveryFixture(t *testing.T) (*recoveryFixture, *RunRecord, string) {
	t.Helper()
	f := newRecoveryFixture(t)
	wf := f.workflow("demo.recovery-safety", f.healthLog, &config.WorkflowRecovery{
		PendingOnFailure: true,
		ChangeNodeIDs:    []string{"deploy"},
		ServiceNodeIDs:   []string{"service"},
		HealthNodeIDs:    []string{"health"},
	})
	record, _ := f.runner.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if record.Status != RunStatusPending {
		t.Fatalf("record = %#v, want pending", record)
	}
	return f, record, filepath.Join(f.runner.RunsDir, record.ID)
}

func TestPendingChecksRejectWritesBeforeExecutingAnyNode(t *testing.T) {
	f, source, runDir := pendingRecoveryFixture(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	for _, nodeID := range []string{"deploy", "service", "http-write"} {
		t.Run(nodeID, func(t *testing.T) {
			data, _ := json.Marshal(source)
			var record RunRecord
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			wf := record.Snapshot.Workflow
			wf.Nodes = append(wf.Nodes, config.WorkflowNode{ID: "http-write", Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{Method: "POST", URL: server.URL}})
			// 首个合法健康检查也不能在后续目标被识别为写操作前执行。
			wf.Recovery.HealthNodeIDs = []string{"health", nodeID}
			if err := f.runner.RetryPendingCheck(context.Background(), &record, runDir); err == nil || !strings.Contains(err.Error(), "只读检查") {
				t.Fatalf("RetryPendingCheck error = %v, want unsafe check rejection", err)
			}
			if len(record.Attempts) != 1 || record.Attempts[0].Status != "failed" {
				t.Fatalf("unsafe check attempt = %#v", record.Attempts)
			}
		})
	}
	if readCount(t, f.deployLog) != 1 || readCount(t, f.serviceLog) != 1 || readCount(t, f.healthLog) != 1 || requests != 0 {
		t.Fatal("unsafe retry executed a workflow node")
	}
}

func TestRollbackRequiresFingerprintAndUsesRecordedHost(t *testing.T) {
	f, record, runDir := pendingRecoveryFixture(t)
	originalSHA := record.Backups[0].SHA256
	record.Backups[0].SHA256 = ""
	if err := f.runner.RollbackPendingRun(context.Background(), record, runDir); err == nil || !strings.Contains(err.Error(), "SHA256") {
		t.Fatalf("missing fingerprint error = %v", err)
	}
	if readCount(t, f.rollbackLog) != 0 {
		t.Fatal("rollback tool executed without fingerprint")
	}
	record.Backups[0].SHA256 = originalSHA
	record.Backups[0].HostID = "original-host"
	record.Snapshot.Workflow.Nodes[0].Host = "different-host"
	remote := &fakeRemoteExecutor{
		hosts: map[string]RemoteHost{
			"original-host":  {ID: "original-host", Name: "原机器"},
			"different-host": {ID: "different-host", Name: "其他机器"},
		},
		stdout: "{\"status\":\"ready\",\"target_path\":\"/etc/app.conf\",\"sha256\":\"old-sha\"}\n",
	}
	f.runner.SetRemoteExecutor(remote)
	f.runner.captureRunSnapshotBindings(record.Snapshot)
	if err := os.WriteFile(filepath.FromSlash(f.control), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.runner.RollbackPendingRun(context.Background(), record, runDir); err != nil {
		t.Fatal(err)
	}
	if len(remote.requests) != 2 {
		t.Fatalf("remote requests = %#v", remote.requests)
	}
	for _, request := range remote.requests {
		if request.HostID != "original-host" {
			t.Fatalf("rollback executed on %s, want original-host", request.HostID)
		}
	}
}

func TestRollbackResumesRealRestoredFileAfterReload(t *testing.T) {
	f, record, runDir := pendingRecoveryFixture(t)
	backup := &record.Backups[0]
	backup.TargetPath = filepath.ToSlash(filepath.Join(f.dir, "app.conf"))
	backup.BackupPath = backup.TargetPath + ".bak"
	oldContent, newContent := []byte("before deployment\n"), []byte("after deployment\n")
	for path, content := range map[string][]byte{backup.TargetPath: newContent, backup.BackupPath: oldContent} {
		if err := os.WriteFile(filepath.FromSlash(path), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	backup.SHA256 = fmt.Sprintf("%x", sha256.Sum256(newContent))
	oldSHA := fmt.Sprintf("%x", sha256.Sum256(oldContent))
	tool := f.runner.Registry.Tools["config.deploy.rollback"]
	scriptPath, err := filepath.Abs("../../plugins/config.deploy/scripts/rollback-config.sh")
	if err != nil {
		t.Fatal(err)
	}
	tool.Config.Execution.Entry = scriptPath
	tool.Config.Execution.Args = []string{"--action", "{{ .action }}", "--target", "{{ .target_path }}", "--backup", "{{ .backup_path }}", "--expected-sha256", "{{ .expected_sha256 }}"}
	f.runner.captureRunSnapshotBindings(record.Snapshot)
	if err := f.runner.RollbackPendingRun(context.Background(), record, runDir); err == nil || !strings.Contains(err.Error(), "健康检查") {
		t.Fatalf("first rollback error = %v, want health check failure", err)
	}
	if got := readFile(t, filepath.FromSlash(backup.TargetPath)); got != string(oldContent) {
		t.Fatalf("restored content = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(runDir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RunRecord
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if reloaded.Backups[0].RestoredSHA256 != oldSHA || reloaded.Backups[0].SHA256 != backup.SHA256 || reloaded.Attempts[0].Status != "failed" {
		t.Fatalf("persisted progress = %#v", reloaded)
	}
	// 模拟文件已经替换、完成标记尚未落盘时进程退出；回滚意图摘要已先于写入持久化。
	reloaded.Backups[0].RestoredSHA256 = ""
	if err := os.WriteFile(filepath.FromSlash(backup.TargetPath), []byte("external edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.runner.RollbackPendingRun(context.Background(), &reloaded, runDir); err == nil || !strings.Contains(err.Error(), "外部修改") {
		t.Fatalf("external change error = %v", err)
	}
	if err := os.WriteFile(filepath.FromSlash(backup.TargetPath), oldContent, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.FromSlash(backup.BackupPath), []byte("modified backup"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.runner.RollbackPendingRun(context.Background(), &reloaded, runDir); err == nil || !strings.Contains(err.Error(), "备份文件") {
		t.Fatalf("modified backup error = %v", err)
	}
	if reloaded.Backups[0].RestoredSHA256 != "" {
		t.Fatal("备份校验失败后错误地标记了恢复完成")
	}
	if err := os.WriteFile(filepath.FromSlash(backup.BackupPath), oldContent, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.FromSlash(f.control), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.runner.RollbackPendingRun(context.Background(), &reloaded, runDir); err != nil {
		t.Fatalf("resume after reload: %v", err)
	}
	if reloaded.Status != RunStatusRolledBack || len(reloaded.Attempts) != 4 || !strings.Contains(readFile(t, filepath.Join(runDir, reloaded.Attempts[3].LogDir, "summary.log")), "已恢复") {
		t.Fatalf("resumed record = %#v", reloaded)
	}
	if _, err := os.Stat(filepath.Join(runDir, reloaded.Attempts[2].LogDir, "rollback")); !os.IsNotExist(err) {
		t.Fatalf("completed file restored again: %v", err)
	}
}

func TestRecoveryAttemptIsPersistedBeforeAction(t *testing.T) {
	f, record, runDir := pendingRecoveryFixture(t)
	attempt, dir, err := f.runner.startRecoveryAttempt(record, runDir, AttemptKindCheck, []string{"health"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(runDir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored RunRecord
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Attempts) != 1 || stored.Attempts[0].ID != attempt.ID || stored.Attempts[0].Status != "running" {
		t.Fatalf("stored attempt = %#v", stored.Attempts)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
	// 上次恢复未完成时，新尝试必须有自己的目录，不能覆盖已有证据。
	next, _, err := f.runner.startRecoveryAttempt(record, runDir, AttemptKindCheck, []string{"health"})
	if err != nil || next.LogDir == attempt.LogDir {
		t.Fatalf("next attempt = %#v, error = %v", next, err)
	}
}

func TestRecoveryNotificationsReflectResolution(t *testing.T) {
	f, source, runDir := pendingRecoveryFixture(t)
	events := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		events = append(events, fmt.Sprint(payload["event"]))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := os.WriteFile(filepath.FromSlash(f.control), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"check", "closed", "rolled-back-check"} {
		data, _ := json.Marshal(source)
		var record RunRecord
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		record.Snapshot.Workflow.Recovery.Notification = &config.WorkflowNotification{URL: server.URL}
		var err error
		if action == "closed" {
			err = f.runner.ResolvePendingRun(&record, "已人工处理")
		} else {
			if action == "rolled-back-check" {
				record.Backups[0].RestoredSHA256 = "old-sha"
			}
			err = f.runner.RetryPendingCheck(context.Background(), &record, runDir)
		}
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if action == "rolled-back-check" && (record.Status != RunStatusRolledBack || record.Error == "") {
			t.Fatalf("rollback health retry lost original failure: %#v", record)
		}
	}
	if strings.Join(events, ",") != "succeeded,failed,rolled_back" {
		t.Fatalf("notification events = %v", events)
	}
}

func TestSkippedConfigDoesNotCreatePendingRecovery(t *testing.T) {
	record := &RunRecord{
		Snapshot: &RunSnapshot{Workflow: &config.WorkflowConfig{Recovery: &config.WorkflowRecovery{PendingOnFailure: true}}},
		Backups:  []BackupRecord{{Status: "skipped"}},
	}
	if shouldPauseForRecovery(record, config.WorkflowNode{ID: "health", OnFailure: "pending"}) {
		t.Fatal("unchanged configuration should not require recovery")
	}
}

func TestExplicitDiffPolicyDoesNotUseOtherDiffResults(t *testing.T) {
	f := newRecoveryFixture(t)
	if err := os.WriteFile(filepath.FromSlash(f.control), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	wf := f.diffWorkflow()
	wf.Stages = []config.WorkflowStage{{ID: "main", Name: "配置变更", ExecutionMode: config.WorkflowStageExecutionSerial}}
	ignored := wf.Nodes[0]
	ignored.ID = "diff-ignore"
	ignored.Order = 5
	changedConfig := *f.runner.Registry.Tools["demo.diff"].Config
	changedConfig.ID = "demo.diff-changed"
	changedTool := *f.runner.Registry.Tools["demo.diff"]
	changedTool.Config = &changedConfig
	changedTool.Dir = writeTool(t, f.dir, "diff-changed", `#!/usr/bin/env bash
set -euo pipefail
printf '{"status":"different","changed":true,"target_path":"%s"}\n' "$OPS_PARAM_TARGET"
`)
	f.runner.Registry.Tools["demo.diff-changed"] = &changedTool
	wf.Nodes[0].Tool = "demo.diff-changed"
	wf.Nodes = append([]config.WorkflowNode{ignored}, wf.Nodes...)
	for index := range wf.Nodes {
		wf.Nodes[index].StageID = "main"
	}
	record, err := f.runner.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatal(err)
	}
	if findStep(t, record, "deploy").Status != "succeeded" || readCount(t, f.deployLog) != 1 || readCount(t, f.serviceLog) != 1 {
		t.Fatalf("unselected no-diff node skipped changes: %#v", record.Steps)
	}
	wf.Recovery.ChangeNodeIDs = []string{"chosen-change"}
	wf.Recovery.ServiceNodeIDs = []string{"chosen-service"}
	if got := f.runner.changeNodeIDs(wf); len(got) != 1 || !got["chosen-change"] {
		t.Fatalf("explicit change nodes = %#v", got)
	}
	if got := f.runner.serviceNodeIDs(wf); len(got) != 1 || !got["chosen-service"] {
		t.Fatalf("explicit service nodes = %#v", got)
	}
	if err := f.runner.validateRecoveryCheck(wf, wf.Nodes[2], nil); err == nil {
		t.Fatal("explicit change list allowed another deployment tool as read-only check")
	}
}

func TestRerunRejectsAllRecoveryHistoryWithoutChangingEvidence(t *testing.T) {
	f, source, runDir := pendingRecoveryFixture(t)
	for _, status := range []string{PendingStatusPending, PendingStatusResolved, PendingStatusClosed, PendingStatusRolledBack, "attempt-only"} {
		data, _ := json.Marshal(source)
		var record RunRecord
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		record.Pending.Status = status
		if status == "attempt-only" {
			record.Pending = nil
			record.Attempts = []AttemptRecord{{ID: "original-attempt", Kind: AttemptKindCheck, Status: "succeeded"}}
		}
		before, _ := json.Marshal(record)
		_, err := f.runner.RerunWorkflowNodeWithParams(context.Background(), record.Snapshot.Workflow, &record, runDir, "deploy", nil, false, nilWriter{}, nilWriter{})
		if err == nil || !strings.Contains(err.Error(), "保留原始失败证据") {
			t.Fatalf("%s: error = %v", status, err)
		}
		after, _ := json.Marshal(record)
		if string(before) != string(after) {
			t.Fatalf("%s: rejected rerun modified evidence", status)
		}
	}
	if readCount(t, f.deployLog) != 1 {
		t.Fatal("recovery-history rerun executed deployment")
	}
}

func TestRecoveryRejectsRollbackAsHealthOrDeploymentAsService(t *testing.T) {
	f, record, runDir := pendingRecoveryFixture(t)
	wf := record.Snapshot.Workflow
	wf.Nodes = append(wf.Nodes, config.WorkflowNode{
		ID: "rollback-check", Type: config.WorkflowNodeTypeTool, Tool: "config.deploy.rollback",
		Params: map[string]interface{}{"action": "rollback", "target_path": record.Backups[0].TargetPath, "backup_path": record.Backups[0].BackupPath},
	})
	wf.Recovery.HealthNodeIDs = []string{"rollback-check"}
	if err := f.runner.RetryPendingCheck(context.Background(), record, runDir); err == nil || !strings.Contains(err.Error(), "只读检查") {
		t.Fatalf("rollback health check error = %v", err)
	}
	wf.Recovery.HealthNodeIDs = []string{"health"}
	wf.Recovery.ChangeNodeIDs = []string{"other-change"}
	wf.Recovery.ServiceNodeIDs = []string{"deploy"}
	if err := f.runner.RollbackPendingRun(context.Background(), record, runDir); err == nil || !strings.Contains(err.Error(), "服务操作工具") {
		t.Fatalf("deployment service error = %v", err)
	}
	if readCount(t, f.deployLog) != 1 || readCount(t, f.rollbackLog) != 0 {
		t.Fatal("unsafe recovery target performed a configuration write")
	}
}

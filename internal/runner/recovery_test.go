package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

type recoveryFixture struct {
	runner      *Runner
	dir         string
	deployLog   string
	healthLog   string
	serviceLog  string
	rollbackLog string
	control     string
}

func newRecoveryFixture(t *testing.T) *recoveryFixture {
	t.Helper()
	fixture := &recoveryFixture{dir: t.TempDir()}
	fixture.deployLog = filepath.ToSlash(filepath.Join(fixture.dir, "deploy.log"))
	fixture.healthLog = filepath.ToSlash(filepath.Join(fixture.dir, "health.log"))
	fixture.serviceLog = filepath.ToSlash(filepath.Join(fixture.dir, "service.log"))
	fixture.rollbackLog = filepath.ToSlash(filepath.Join(fixture.dir, "rollback.log"))
	fixture.control = filepath.ToSlash(filepath.Join(fixture.dir, "health-ok"))

	deployDir := writeTool(t, fixture.dir, "deploy", `#!/usr/bin/env bash
set -euo pipefail
printf 'deploy\n' >> "$OPS_PARAM_COUNTER"
printf '{"status":"updated","target_path":"%s","backup_path":"%s","sha256":"new-sha"}\n' "$OPS_PARAM_TARGET" "$OPS_PARAM_BACKUP"
`)
	healthDir := writeTool(t, fixture.dir, "health", `#!/usr/bin/env bash
set -euo pipefail
printf 'health\n' >> "$OPS_PARAM_COUNTER"
if [ -f "$OPS_PARAM_CONTROL" ]; then
  printf '{"status":"ok"}\n'
  exit 0
fi
echo "health check failed" >&2
exit 1
`)
	serviceDir := writeTool(t, fixture.dir, "service", `#!/usr/bin/env bash
set -euo pipefail
printf 'service\n' >> "$OPS_PARAM_COUNTER"
printf '{"service":"%s","action":"%s","status":"ok"}\n' "$OPS_PARAM_SERVICE" "$OPS_PARAM_ACTION"
`)
	rollbackDir := writeTool(t, fixture.dir, "rollback", `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$OPS_PARAM_ACTION" >> "$OPS_PARAM_COUNTER"
printf '{"status":"%s","target_path":"%s","sha256":"old-sha"}\n' "$OPS_PARAM_ACTION" "$OPS_PARAM_TARGET_PATH"
`)
	diffDir := writeTool(t, fixture.dir, "diff", `#!/usr/bin/env bash
set -euo pipefail
printf '{"status":"identical","changed":false,"target_path":"%s"}\n' "$OPS_PARAM_TARGET"
`)

	deployConfig := toolConfig("demo.deploy")
	deployConfig.Parameters = []config.Parameter{
		{Name: "target", Required: true},
		{Name: "backup", Required: true},
		{Name: "counter", Required: true},
	}
	deployConfig.Outputs = []config.ToolOutput{
		{Name: "target_path", JSONPath: "target_path", Required: true},
		{Name: "backup_path", JSONPath: "backup_path"},
		{Name: "sha256", JSONPath: "sha256"},
		{Name: "status", JSONPath: "status"},
	}
	healthConfig := toolConfig("demo.health")
	healthConfig.Parameters = []config.Parameter{
		{Name: "counter", Required: true},
		{Name: "control", Required: true},
	}
	serviceConfig := toolConfig("demo.service")
	serviceConfig.Parameters = []config.Parameter{
		{Name: "service", Required: true},
		{Name: "action", Required: true},
		{Name: "counter", Required: true},
	}
	serviceConfig.Outputs = []config.ToolOutput{{Name: "status", JSONPath: "status", Required: true}}
	rollbackConfig := toolConfig("config.deploy.rollback")
	rollbackConfig.Parameters = []config.Parameter{
		{Name: "action", Required: true},
		{Name: "target_path", Required: true},
		{Name: "backup_path", Required: true},
		{Name: "expected_sha256"},
		{Name: "force"},
		{Name: "counter", Default: fixture.rollbackLog},
	}
	rollbackConfig.Outputs = []config.ToolOutput{
		{Name: "target_path", JSONPath: "target_path", Required: true},
		{Name: "sha256", JSONPath: "sha256", Required: true},
		{Name: "status", JSONPath: "status", Required: true},
	}
	diffConfig := toolConfig("demo.diff")
	diffConfig.Parameters = []config.Parameter{{Name: "target", Required: true}}
	diffConfig.Outputs = []config.ToolOutput{
		{Name: "target_path", JSONPath: "target_path", Required: true},
		{Name: "changed", JSONPath: "changed"},
		{Name: "status", JSONPath: "status"},
	}

	reg := &registry.Registry{
		BaseDir: fixture.dir,
		Root:    &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools: map[string]*registry.Tool{
			"demo.deploy":            {Entry: config.ToolEntry{ID: "demo.deploy", Category: "demo"}, Config: deployConfig, Dir: deployDir},
			"demo.health":            {Entry: config.ToolEntry{ID: "demo.health", Category: "demo"}, Config: healthConfig, Dir: healthDir},
			"demo.service":           {Entry: config.ToolEntry{ID: "demo.service", Category: "demo"}, Config: serviceConfig, Dir: serviceDir},
			"demo.diff":              {Entry: config.ToolEntry{ID: "demo.diff", Category: "demo"}, Config: diffConfig, Dir: diffDir},
			"config.deploy.rollback": {Entry: config.ToolEntry{ID: "config.deploy.rollback", Category: "config"}, Config: rollbackConfig, Dir: rollbackDir},
		},
		Workflows: map[string]*registry.Workflow{},
	}
	fixture.runner = New(reg)
	return fixture
}

func (f *recoveryFixture) workflow(id, healthLog string, policy *config.WorkflowRecovery) *config.WorkflowConfig {
	target := filepath.ToSlash(filepath.Join(f.dir, "app.conf"))
	backup := target + ".bak"
	wf := &config.WorkflowConfig{
		ID:       id,
		Recovery: policy,
		Nodes: []config.WorkflowNode{
			{
				ID:     "deploy",
				Type:   config.WorkflowNodeTypeTool,
				Tool:   "demo.deploy",
				Order:  10,
				Params: map[string]interface{}{"target": target, "backup": backup, "counter": f.deployLog},
			},
			{
				ID:     "service",
				Type:   config.WorkflowNodeTypeTool,
				Tool:   "demo.service",
				Order:  20,
				Params: map[string]interface{}{"service": "demo", "action": "restart", "counter": f.serviceLog},
			},
			{
				ID:     "health",
				Type:   config.WorkflowNodeTypeTool,
				Tool:   "demo.health",
				Order:  30,
				Params: map[string]interface{}{"counter": healthLog, "control": f.control},
			},
		},
	}
	return wf
}

func (f *recoveryFixture) diffWorkflow() *config.WorkflowConfig {
	target := filepath.ToSlash(filepath.Join(f.dir, "app.conf"))
	return &config.WorkflowConfig{
		ID: "demo.no-diff",
		Recovery: &config.WorkflowRecovery{
			SkipChangeOnNoDiff: true,
			DiffNodeID:         "diff",
			ChangeNodeIDs:      []string{"deploy"},
			ServiceNodeIDs:     []string{"service"},
			HealthNodeIDs:      []string{"health"},
		},
		Nodes: []config.WorkflowNode{
			{ID: "diff", Type: config.WorkflowNodeTypeTool, Tool: "demo.diff", Order: 10, Params: map[string]interface{}{"target": target}},
			{ID: "deploy", Type: config.WorkflowNodeTypeTool, Tool: "demo.deploy", Order: 20, Params: map[string]interface{}{"target": target, "backup": target + ".bak", "counter": f.deployLog}},
			{ID: "service", Type: config.WorkflowNodeTypeTool, Tool: "demo.service", Order: 30, Params: map[string]interface{}{"service": "demo", "action": "restart", "counter": f.serviceLog}},
			{ID: "health", Type: config.WorkflowNodeTypeTool, Tool: "demo.health", Order: 40, Params: map[string]interface{}{"counter": f.healthLog, "control": f.control}},
		},
	}
}

func readCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "\n")
}

func findStep(t *testing.T, record *RunRecord, id string) StepRecord {
	t.Helper()
	for _, step := range record.Steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("step %s not found in %#v", id, record.Steps)
	return StepRecord{}
}

func TestWorkflowFailureAfterChangesBecomesPendingAndRetryCheckIsIsolated(t *testing.T) {
	fixture := newRecoveryFixture(t)
	webhookEvents := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var payload map[string]interface{}
		_ = json.NewDecoder(req.Body).Decode(&payload)
		webhookEvents = append(webhookEvents, payload["event"].(string))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	policy := &config.WorkflowRecovery{
		PendingOnFailure: true,
		ChangeNodeIDs:    []string{"deploy"},
		ServiceNodeIDs:   []string{"service"},
		HealthNodeIDs:    []string{"health"},
		Notification:     &config.WorkflowNotification{URL: server.URL, Events: []string{"pending"}},
	}
	wf := fixture.workflow("demo.pending", fixture.healthLog, policy)
	record, err := fixture.runner.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err == nil {
		t.Fatal("expected health check failure")
	}
	if record.Status != RunStatusPending || record.Pending == nil || record.Pending.StepID != "health" {
		t.Fatalf("record = %#v, want pending at health", record)
	}
	if len(record.Backups) != 1 || record.Backups[0].NodeID != "deploy" || record.Backups[0].TargetPath == "" {
		t.Fatalf("backups = %#v", record.Backups)
	}
	if len(record.Notifications) != 1 || record.Notifications[0].Event != config.WorkflowNotifyPending || record.Notifications[0].Status != "sent" {
		t.Fatalf("notifications = %#v", record.Notifications)
	}
	runDir := filepath.Join(fixture.runner.RunsDir, record.ID)
	if _, err := os.Stat(filepath.Join(runDir, "snapshot.json")); err != nil {
		t.Fatalf("snapshot.json missing: %v", err)
	}
	originalSteps := len(record.Steps)

	if err := fixture.runner.RetryPendingCheck(context.Background(), record, runDir); err == nil {
		t.Fatal("expected retry check failure")
	}
	if record.Status != RunStatusPending || record.Pending.Status != PendingStatusPending {
		t.Fatalf("retry failure changed state: %#v", record.Pending)
	}
	if len(record.Attempts) != 1 || record.Attempts[0].Kind != AttemptKindCheck || record.Attempts[0].Status != "failed" {
		t.Fatalf("attempts = %#v", record.Attempts)
	}
	if len(record.Steps) != originalSteps {
		t.Fatalf("retry check modified original steps: %d -> %d", originalSteps, len(record.Steps))
	}
	if count := readCount(t, fixture.deployLog); count != 1 {
		t.Fatalf("deploy ran %d times during retry check, want 1", count)
	}
	if count := readCount(t, fixture.serviceLog); count != 1 {
		t.Fatalf("service ran %d times during retry check, want 1", count)
	}
	if count := readCount(t, fixture.healthLog); count != 2 {
		t.Fatalf("health ran %d times, want 2", count)
	}

	if err := os.WriteFile(filepath.FromSlash(fixture.control), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fixture.runner.RetryPendingCheck(context.Background(), record, runDir); err != nil {
		t.Fatalf("retry check after recovery: %v", err)
	}
	if record.Status != "succeeded" || record.Pending.Status != PendingStatusResolved {
		t.Fatalf("record after retry = %#v", record)
	}
	if len(record.Attempts) != 2 || record.Attempts[1].Status != "succeeded" {
		t.Fatalf("attempts = %#v", record.Attempts)
	}
	if count := readCount(t, fixture.deployLog); count != 1 {
		t.Fatalf("retry check reran deploy: %d", count)
	}
	if err := fixture.runner.RetryNotification(context.Background(), record, 0); err != nil {
		t.Fatalf("retry notification: %v", err)
	}
	if len(webhookEvents) != 2 || record.Notifications[0].Attempts != 2 {
		t.Fatalf("webhook events = %v, notifications = %#v", webhookEvents, record.Notifications)
	}
}

func TestRollbackFailureKeepsPendingAndSuccessMarksRolledBack(t *testing.T) {
	fixture := newRecoveryFixture(t)
	policy := &config.WorkflowRecovery{
		PendingOnFailure: true,
		ChangeNodeIDs:    []string{"deploy"},
		ServiceNodeIDs:   []string{"service"},
		HealthNodeIDs:    []string{"health"},
	}
	wf := fixture.workflow("demo.rollback", fixture.healthLog, policy)
	record, _ := fixture.runner.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if record.Status != RunStatusPending {
		t.Fatalf("record = %#v, want pending", record)
	}
	runDir := filepath.Join(fixture.runner.RunsDir, record.ID)

	if err := fixture.runner.RollbackPendingRun(context.Background(), record, runDir); err == nil {
		t.Fatal("expected rollback health check failure")
	}
	if record.Status != RunStatusPending || record.Pending.Status != PendingStatusPending {
		t.Fatalf("failed rollback must keep pending: %#v", record.Pending)
	}
	if len(record.Attempts) != 1 || record.Attempts[0].Kind != AttemptKindRollback || record.Attempts[0].Status != "failed" {
		t.Fatalf("attempts = %#v", record.Attempts)
	}
	if len(record.Backups) != 1 {
		t.Fatalf("backups must be retained after failure: %#v", record.Backups)
	}

	if err := os.WriteFile(filepath.FromSlash(fixture.control), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fixture.runner.RollbackPendingRun(context.Background(), record, runDir); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if record.Status != RunStatusRolledBack || record.Pending.Status != PendingStatusRolledBack {
		t.Fatalf("record after rollback = %#v", record)
	}
	if len(record.Attempts) != 2 || record.Attempts[1].Status != "succeeded" {
		t.Fatalf("attempts = %#v", record.Attempts)
	}
	if count := readCount(t, fixture.rollbackLog); count != 3 {
		t.Fatalf("rollback actions = %d, want 3 (initial check+rollback, resumed check)", count)
	}
	if count := readCount(t, fixture.serviceLog); count != 3 {
		t.Fatalf("service runs = %d, want 3 (initial + two rollback restarts)", count)
	}
	if count := readCount(t, fixture.deployLog); count != 1 {
		t.Fatalf("rollback reran deploy: %d", count)
	}
	if step := findStep(t, record, "health"); step.Status != "failed" {
		t.Fatalf("original failed conclusion lost: %#v", step)
	}
}

func TestNoDiffSkipsChangesAndServiceButStillChecksHealth(t *testing.T) {
	fixture := newRecoveryFixture(t)
	if err := os.WriteFile(filepath.FromSlash(fixture.control), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	wf := fixture.diffWorkflow()
	record, err := fixture.runner.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("run no-diff workflow: %v", err)
	}
	if record.Status != "succeeded" {
		t.Fatalf("record = %#v", record)
	}
	if step := findStep(t, record, "deploy"); step.Status != "skipped" || step.SkippedReason == "" {
		t.Fatalf("deploy step = %#v, want skipped", step)
	}
	if step := findStep(t, record, "service"); step.Status != "skipped" {
		t.Fatalf("service step = %#v, want skipped", step)
	}
	if step := findStep(t, record, "health"); step.Status != "succeeded" {
		t.Fatalf("health step = %#v, want succeeded", step)
	}
	if count := readCount(t, fixture.deployLog); count != 0 {
		t.Fatalf("deploy ran %d times", count)
	}
	if count := readCount(t, fixture.serviceLog); count != 0 {
		t.Fatalf("service ran %d times", count)
	}
	if count := readCount(t, fixture.healthLog); count != 1 {
		t.Fatalf("health ran %d times", count)
	}
	if len(record.Backups) != 0 {
		t.Fatalf("no-diff run recorded backups: %#v", record.Backups)
	}
}

func TestResolvePendingRunMarksFailedAndKeepsBackups(t *testing.T) {
	fixture := newRecoveryFixture(t)
	policy := &config.WorkflowRecovery{
		PendingOnFailure: true,
		ChangeNodeIDs:    []string{"deploy"},
		HealthNodeIDs:    []string{"health"},
	}
	wf := fixture.workflow("demo.resolve", fixture.healthLog, policy)
	record, _ := fixture.runner.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if record.Status != RunStatusPending {
		t.Fatalf("record = %#v, want pending", record)
	}
	if err := fixture.runner.ResolvePendingRun(record, "已人工确认保留当前配置"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if record.Status != "failed" || record.Pending.Status != PendingStatusClosed {
		t.Fatalf("record after resolve = %#v", record)
	}
	if record.Pending.Resolution != "已人工确认保留当前配置" {
		t.Fatalf("resolution = %q", record.Pending.Resolution)
	}
	if len(record.Backups) != 1 {
		t.Fatalf("backups = %#v", record.Backups)
	}
	if len(record.Attempts) != 1 || record.Attempts[0].Kind != AttemptKindResolve {
		t.Fatalf("attempts = %#v", record.Attempts)
	}
}

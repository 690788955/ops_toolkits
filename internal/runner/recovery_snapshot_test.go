package runner

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell_ops/internal/config"
)

func TestRunSnapshotBindingsRejectToolDrift(t *testing.T) {
	tests := []struct {
		name   string
		change func(*testing.T, *recoveryFixture)
	}{
		{"version", func(t *testing.T, f *recoveryFixture) {
			f.runner.Registry.Tools["demo.health"].Config.Version = "2.0.0"
		}},
		{"arguments", func(t *testing.T, f *recoveryFixture) {
			f.runner.Registry.Tools["demo.health"].Config.Execution.Args = []string{"--different"}
		}},
		{"plugin version", func(t *testing.T, f *recoveryFixture) {
			f.runner.Registry.Tools["demo.health"].Source.PluginVersion = "2.0.0"
		}},
		{"plugin defaults", func(t *testing.T, f *recoveryFixture) {
			f.runner.Registry.Tools["demo.health"].Config.PluginConfig.HostConfig = map[string]interface{}{"token": "changed-secret"}
		}},
		{"entry content", func(t *testing.T, f *recoveryFixture) {
			tool := f.runner.Registry.Tools["demo.health"]
			if err := os.WriteFile(resolveEntry(tool.Dir, tool.Config.Execution.Entry), []byte("#!/bin/sh\necho changed\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing entry", func(t *testing.T, f *recoveryFixture) {
			f.runner.Registry.Tools["demo.health"].Config.Execution.Entry = "./missing.sh"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newRecoveryFixture(t)
			snapshot := newRunSnapshot(fixture.workflow("demo.binding", fixture.healthLog, nil), nil)
			fixture.runner.captureRunSnapshotBindings(snapshot)
			if err := fixture.runner.validateRunSnapshotBindings(snapshot); err != nil {
				t.Fatalf("original snapshot: %v", err)
			}
			before, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			tt.change(t, fixture)
			if err := fixture.runner.validateRunSnapshotBindings(snapshot); err == nil || !strings.Contains(err.Error(), "原运行快照恢复") {
				t.Fatalf("changed execution context must be rejected: %v", err)
			}
			after, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("registry mutations changed the original snapshot")
			}
		})
	}
}

type snapshotIdentityRemote struct {
	identity string
}

func (f *snapshotIdentityRemote) HostIdentity(string) (string, error) {
	return f.identity, nil
}

func (f *snapshotIdentityRemote) DescribeHost(id string) (RemoteHost, error) {
	return RemoteHost{ID: id, Name: "target"}, nil
}

func (f *snapshotIdentityRemote) RunTool(context.Context, RemoteToolRequest, io.Writer, io.Writer) error {
	return nil
}

func TestRunSnapshotBindingsRejectHostIdentityChange(t *testing.T) {
	fixture := newRecoveryFixture(t)
	remote := &snapshotIdentityRemote{identity: "original-address-user-hash"}
	fixture.runner.SetRemoteExecutor(remote)
	wf := fixture.workflow("demo.host-binding", fixture.healthLog, nil)
	wf.Nodes[0].Host = "host-a"
	snapshot := newRunSnapshot(wf, nil)
	fixture.runner.captureRunSnapshotBindings(snapshot)
	if err := fixture.runner.validateRunSnapshotBindings(snapshot); err != nil {
		t.Fatal(err)
	}
	remote.identity = "new-address-user-hash"
	if err := fixture.runner.validateRunSnapshotBindings(snapshot); err == nil || !strings.Contains(err.Error(), "连接身份已变更") {
		t.Fatalf("same ID with changed connection must be rejected: %v", err)
	}
	if snapshot.HostIdentities["host-a"] != "original-address-user-hash" {
		t.Fatal("host mutation changed stored identity")
	}
}

func TestRunSnapshotBindingsSupportRemoteWithoutIdentityProvider(t *testing.T) {
	fixture := newRecoveryFixture(t)
	fixture.runner.SetRemoteExecutor(&fakeRemoteExecutor{hosts: map[string]RemoteHost{"host-a": {ID: "host-a", Name: "A"}}})
	wf := fixture.workflow("demo.fake-host", fixture.healthLog, nil)
	wf.Nodes[0].Host = "host-a"
	snapshot := newRunSnapshot(wf, nil)
	fixture.runner.captureRunSnapshotBindings(snapshot)
	if snapshot.HostIdentities["host-a"] == "" {
		t.Fatal("fake remote descriptor was not captured")
	}
	if err := fixture.runner.validateRunSnapshotBindings(snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestRunSnapshotBindingsRejectUnboundLegacyToolsAndHosts(t *testing.T) {
	fixture := newRecoveryFixture(t)
	wf := fixture.workflow("demo.legacy-snapshot", fixture.healthLog, nil)
	snapshot := newRunSnapshot(wf, nil)
	if err := fixture.runner.validateRunSnapshotBindings(snapshot); err == nil || !strings.Contains(err.Error(), "未固定工具") {
		t.Fatalf("legacy tool snapshot should be rejected: %v", err)
	}
	fixture.runner.captureRunSnapshotBindings(snapshot)
	snapshot.Workflow.Nodes[0].Host = "host-a"
	if err := fixture.runner.validateRunSnapshotBindings(snapshot); err == nil || !strings.Contains(err.Error(), "未固定操作机") {
		t.Fatalf("unbound target should be rejected: %v", err)
	}
}

func TestPrepareWorkflowRunCapturesBindingsWithoutOptionalRollback(t *testing.T) {
	fixture := newRecoveryFixture(t)
	delete(fixture.runner.Registry.Tools, "config.deploy.rollback")
	wf := fixture.workflow("demo.prepare-snapshot", fixture.healthLog, nil)
	record, _, runDir, err := fixture.runner.prepareWorkflowRun(wf, map[string]string{"value": "original"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := record.Snapshot.Tools["config.deploy.rollback"]; ok {
		t.Fatal("missing optional rollback tool was bound")
	}
	if err := fixture.runner.validateRunSnapshotBindings(record.Snapshot); err != nil {
		t.Fatal(err)
	}
	wf.Nodes[0].Tool = "different.tool"
	if record.Snapshot.Workflow.Nodes[0].Tool != "demo.deploy" {
		t.Fatal("workflow edit mutated the original snapshot")
	}
	data, err := os.ReadFile(filepath.Join(runDir, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored RunSnapshot
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Tools["demo.health"].EntrySHA256 == "" {
		t.Fatal("persisted snapshot is missing executable binding")
	}
}

func TestRecoveryActionsRejectChangedBindingsBeforeSideEffects(t *testing.T) {
	fixture := newRecoveryFixture(t)
	wf := fixture.workflow("demo.binding-actions", fixture.healthLog, &config.WorkflowRecovery{HealthNodeIDs: []string{"health"}})
	snapshot := newRunSnapshot(wf, nil)
	fixture.runner.captureRunSnapshotBindings(snapshot)
	fixture.runner.Registry.Tools["demo.health"].Config.Version = "2.0.0"
	for _, action := range []struct {
		name string
		run  func(context.Context, *RunRecord, string) error
	}{
		{"check", fixture.runner.RetryPendingCheck},
		{"rollback", fixture.runner.RollbackPendingRun},
	} {
		t.Run(action.name, func(t *testing.T) {
			record := &RunRecord{
				ID: "original", Status: RunStatusPending, Snapshot: snapshot,
				Pending: &PendingState{Status: PendingStatusPending, StepID: "health"},
				Backups: []BackupRecord{{NodeID: "deploy", TargetPath: "/app.conf", BackupPath: "/app.conf.bak", SHA256: "original"}},
			}
			if err := action.run(context.Background(), record, t.TempDir()); err == nil || !strings.Contains(err.Error(), "原运行快照恢复") {
				t.Fatalf("expected binding error before recovery: %v", err)
			}
			if len(record.Attempts) != 0 || readCount(t, fixture.healthLog) != 0 || readCount(t, fixture.rollbackLog) != 0 {
				t.Fatal("recovery performed side effects after binding drift")
			}
		})
	}
}

func TestRunSnapshotBindingDoesNotPersistToolSecrets(t *testing.T) {
	fixture := newRecoveryFixture(t)
	fixture.runner.Registry.Tools["demo.health"].Config.Env = map[string]string{"TOKEN": "private-tool-secret"}
	snapshot := newRunSnapshot(fixture.workflow("demo.binding-secrets", fixture.healthLog, nil), nil)
	fixture.runner.captureRunSnapshotBindings(snapshot)
	data, err := json.Marshal(snapshot.Tools)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-tool-secret") || strings.Contains(string(data), "TOKEN") {
		t.Fatalf("tool secret leaked into bindings: %s", data)
	}
}

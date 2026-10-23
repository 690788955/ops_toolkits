package runner

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"shell_ops/internal/config"
)

func TestParallelMutationOrderHonorsReverseExplicitDependency(t *testing.T) {
	var mu sync.Mutex
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		paths = append(paths, req.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID:       "demo.parallel-reverse-mutations",
		Stages:   []config.WorkflowStage{{ID: "parallel", ExecutionMode: config.WorkflowStageExecutionParallel}},
		Recovery: &config.WorkflowRecovery{ChangeNodeIDs: []string{"first", "second"}},
		Nodes: []config.WorkflowNode{
			{ID: "first", StageID: "parallel", Order: 10, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL + "/first"}},
			{ID: "second", StageID: "parallel", Order: 20, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL + "/second"}},
		},
		Edges: []config.WorkflowEdge{{From: "second", To: "first"}},
	}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/second" || paths[1] != "/first" {
		t.Fatalf("execution = %v, record = %#v", paths, record)
	}
}

func TestArchiveDispatchIgnoresRemoteHostAndRecordsLocalExecution(t *testing.T) {
	r, dir := newHTTPTestRunner(t)
	archivePath := filepath.Join(dir, "local.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("app.conf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(entry, "local config\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	wf := &config.WorkflowConfig{ID: "demo.archive-local-host", Nodes: []config.WorkflowNode{{ID: "archive", Type: config.WorkflowNodeTypeArchive, Host: "missing-remote-host", Archive: config.WorkflowArchive{Path: archivePath, TargetDir: filepath.Join(dir, "out")}}}}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Steps) != 1 || record.Steps[0].HostID != "" || record.Steps[0].HostName != "" || readFile(t, filepath.Join(dir, "out", "app.conf")) != "local config\n" {
		t.Fatalf("archive host metadata = %#v", record.Steps)
	}
}

func TestParallelOutputSupportsNilAndSharedBuffer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	for _, nilOutput := range []bool{true, false} {
		r, _ := newHTTPTestRunner(t)
		wf := &config.WorkflowConfig{ID: "demo.parallel-output", Stages: []config.WorkflowStage{{ID: "parallel", ExecutionMode: config.WorkflowStageExecutionParallel, MaxConcurrency: 12}}}
		for index := 0; index < 24; index++ {
			wf.Nodes = append(wf.Nodes, config.WorkflowNode{ID: fmt.Sprintf("http-%d", index), StageID: "parallel", Order: index + 1, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL}})
		}
		var buffer bytes.Buffer
		var output io.Writer
		if !nilOutput {
			output = &buffer
		}
		if _, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, output, output); err != nil {
			t.Fatal(err)
		}
		if !nilOutput && bytes.Count(buffer.Bytes(), []byte("GET ")) != len(wf.Nodes) {
			t.Fatalf("lost shared output: %s", buffer.String())
		}
	}
}

func TestWorkflowNodeTimeoutIsFailureInBothStageModes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		select {
		case <-req.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	for _, mode := range []string{config.WorkflowStageExecutionSerial, config.WorkflowStageExecutionParallel} {
		r, _ := newHTTPTestRunner(t)
		wf := &config.WorkflowConfig{ID: "demo.parallel-timeout", Stages: []config.WorkflowStage{{ID: "stage", ExecutionMode: mode}}, Nodes: []config.WorkflowNode{{ID: "timeout", StageID: "stage", Order: 10, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL, Timeout: "40ms"}}}}
		record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
		if err == nil || record.Status != "failed" || len(record.Steps) != 1 || record.Steps[0].Status != "failed" {
			t.Fatalf("%s timeout result: %v, %#v", mode, err, record)
		}
	}
}

func TestParallelSkippedReverseDependencyReachesTerminalState(t *testing.T) {
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{ID: "demo.reverse-skipped", Stages: []config.WorkflowStage{{ID: "parallel", ExecutionMode: config.WorkflowStageExecutionParallel}}, Nodes: []config.WorkflowNode{
		{ID: "route", StageID: "parallel", Order: 10, Type: config.WorkflowNodeTypeCondition, Condition: config.WorkflowCondition{Input: "no", Cases: []config.ConditionCase{{ID: "yes", Operator: "eq", Values: []string{"yes"}}}}},
		{ID: "child", StageID: "parallel", Order: 20, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: "http://127.0.0.1:1/never"}},
		{ID: "parent", StageID: "parallel", Order: 30, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: "http://127.0.0.1:1/never"}},
	}, Edges: []config.WorkflowEdge{{From: "route", To: "parent", Case: "yes"}, {From: "parent", To: "child"}}}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err != nil || record.Status != "succeeded" || findStep(t, record, "child").Status != "skipped" {
		t.Fatalf("inactive reverse dependency: %v, %#v", err, record)
	}
}

type lateParallelMutation struct {
	started, healthFailed chan struct{}
	returnCancelled       bool
}

func (remote *lateParallelMutation) DescribeHost(id string) (RemoteHost, error) {
	return RemoteHost{ID: id, Name: "变更机器"}, nil
}

func (remote *lateParallelMutation) RunTool(ctx context.Context, req RemoteToolRequest, out, errOut io.Writer) error {
	close(remote.started)
	<-remote.healthFailed
	// 远端命令可能已完成写入，只是在取消发出后才返回成功证据。
	time.Sleep(40 * time.Millisecond)
	_, err := io.WriteString(out, `{"status":"updated","target_path":"/etc/app.conf","backup_path":"/etc/app.conf.bak","sha256":"new-sha"}`+"\n")
	if err == nil && remote.returnCancelled {
		<-ctx.Done()
		return ctx.Err()
	}
	return err
}

func TestParallelFailureBecomesPendingAfterLateMutationEvidence(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancelled=%t", cancelled), func(t *testing.T) {
			testParallelLateMutationEvidence(t, cancelled)
		})
	}
}

func testParallelLateMutationEvidence(t *testing.T, cancelled bool) {
	t.Helper()
	f := newRecoveryFixture(t)
	remote := &lateParallelMutation{started: make(chan struct{}), healthFailed: make(chan struct{}), returnCancelled: cancelled}
	f.runner.SetRemoteExecutor(remote)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		<-remote.started
		close(remote.healthFailed)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	wf := f.workflow("demo.parallel-late-backup", f.healthLog, &config.WorkflowRecovery{PendingOnFailure: true, ChangeNodeIDs: []string{"deploy"}, HealthNodeIDs: []string{"health"}})
	wf.Stages = []config.WorkflowStage{{ID: "parallel", ExecutionMode: config.WorkflowStageExecutionParallel}}
	wf.Nodes = []config.WorkflowNode{wf.Nodes[0], {ID: "health", Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL}}}
	wf.Nodes[0].Host = "remote"
	for index := range wf.Nodes {
		wf.Nodes[index].StageID = "parallel"
		wf.Nodes[index].Order = (index + 1) * 10
	}
	record, err := f.runner.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err == nil || record.Status != RunStatusPending || record.Pending == nil || record.Pending.StepID != "health" || len(record.Backups) != 1 {
		t.Fatalf("late backup failure: %v, %#v", err, record)
	}
}

func TestParallelRerunPreservesIndependentSibling(t *testing.T) {
	var mu sync.Mutex
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		counts[req.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{ID: "demo.parallel-rerun", Stages: []config.WorkflowStage{{ID: "parallel", ExecutionMode: config.WorkflowStageExecutionParallel}, {ID: "verify"}}, Nodes: []config.WorkflowNode{
		{ID: "target", StageID: "parallel", Order: 10, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL + "/target"}},
		{ID: "sibling", StageID: "parallel", Order: 20, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL + "/sibling"}},
		{ID: "dependent", StageID: "parallel", Order: 30, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL + "/dependent"}},
		{ID: "verify", StageID: "verify", Order: 10, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL + "/verify"}},
	}, Edges: []config.WorkflowEdge{{From: "target", To: "dependent"}}}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.RerunWorkflowNodeWithParams(context.Background(), wf, record, r.RunsDir+"/"+record.ID, "target", nil, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatal(err)
	}
	if counts["/target"] != 2 || counts["/dependent"] != 2 || counts["/verify"] != 2 || counts["/sibling"] != 1 {
		t.Fatalf("rerun counts = %v", counts)
	}
}

func TestParallelInactiveBranchDoesNotActivateDescendants(t *testing.T) {
	var mu sync.Mutex
	paths := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		paths[req.URL.Path] = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID:     "demo.parallel-inactive-descendants",
		Stages: []config.WorkflowStage{{ID: "parallel", ExecutionMode: config.WorkflowStageExecutionParallel}},
		Nodes: []config.WorkflowNode{
			{ID: "route", StageID: "parallel", Order: 10, Type: config.WorkflowNodeTypeCondition, Condition: config.WorkflowCondition{Input: "yes", Cases: []config.ConditionCase{{ID: "yes", Operator: "eq", Values: []string{"yes"}}}, DefaultCase: "no"}},
			{ID: "chosen", StageID: "parallel", Order: 20, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL + "/chosen"}},
			{ID: "not-chosen", StageID: "parallel", Order: 30, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL + "/not-chosen"}},
			{ID: "not-chosen-child", StageID: "parallel", Order: 40, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL + "/not-chosen-child"}},
			{ID: "join", StageID: "parallel", Order: 50, Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL + "/join"}},
		},
		Edges: []config.WorkflowEdge{{From: "route", To: "chosen", Case: "yes"}, {From: "route", To: "not-chosen", Case: "no"}, {From: "not-chosen", To: "not-chosen-child"}, {From: "chosen", To: "join"}, {From: "not-chosen-child", To: "join"}},
	}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatal(err)
	}
	if paths["/not-chosen"] || paths["/not-chosen-child"] || !paths["/chosen"] || !paths["/join"] {
		t.Fatalf("executed paths = %v, steps = %#v", paths, record.Steps)
	}
}

package runner

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shell_ops/internal/config"
)

// parallelProbe 记录每个节点请求的开始与结束，用来判断并行分支是否真的重叠执行。
type parallelProbe struct {
	mu      sync.Mutex
	starts  map[string]time.Time
	ends    map[string]time.Time
	active  int32
	peak    int32
	release chan struct{}
}

func newParallelProbe() *parallelProbe {
	return &parallelProbe{starts: map[string]time.Time{}, ends: map[string]time.Time{}, release: make(chan struct{})}
}

func (p *parallelProbe) handler(delay time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&p.active, 1)
		for {
			peak := atomic.LoadInt32(&p.peak)
			if current <= peak || atomic.CompareAndSwapInt32(&p.peak, peak, current) {
				break
			}
		}
		p.mu.Lock()
		p.starts[r.URL.Path] = time.Now()
		p.mu.Unlock()
		time.Sleep(delay)
		p.mu.Lock()
		p.ends[r.URL.Path] = time.Now()
		p.mu.Unlock()
		atomic.AddInt32(&p.active, -1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func (p *parallelProbe) overlap(left, right string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	leftStart, leftOK := p.starts[left]
	rightStart, rightOK := p.starts[right]
	leftEnd, leftEndOK := p.ends[left]
	rightEnd, rightEndOK := p.ends[right]
	if !leftOK || !rightOK || !leftEndOK || !rightEndOK {
		return false
	}
	return leftStart.Before(rightEnd) && rightStart.Before(leftEnd)
}

func TestRunWorkflowParallelStageOverlapsBranchExecution(t *testing.T) {
	probe := newParallelProbe()
	server := httptest.NewServer(probe.handler(250 * time.Millisecond))
	defer server.Close()
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID: "demo.parallel",
		Stages: []config.WorkflowStage{
			{ID: "fanout", Name: "并行", ExecutionMode: config.WorkflowStageExecutionParallel, MaxConcurrency: 2},
		},
		Nodes: []config.WorkflowNode{
			{ID: "left", Type: config.WorkflowNodeTypeHTTP, StageID: "fanout", Order: 10, HTTP: config.WorkflowHTTP{URL: server.URL + "/left"}},
			{ID: "right", Type: config.WorkflowNodeTypeHTTP, StageID: "fanout", Order: 20, HTTP: config.WorkflowHTTP{URL: server.URL + "/right"}},
		},
	}
	started := time.Now()
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	elapsed := time.Since(started)
	if !probe.overlap("/left", "/right") {
		t.Fatalf("parallel branches did not overlap: starts=%v ends=%v", probe.starts, probe.ends)
	}
	if elapsed > 480*time.Millisecond {
		t.Fatalf("parallel stage took %v, want less than the serial sum of two 250ms nodes", elapsed)
	}
	if record.Status != "succeeded" || len(record.Steps) != 2 {
		t.Fatalf("record status = %s steps = %+v", record.Status, record.Steps)
	}
}

func TestRunWorkflowParallelStageHonorsMaxConcurrency(t *testing.T) {
	probe := newParallelProbe()
	server := httptest.NewServer(probe.handler(120 * time.Millisecond))
	defer server.Close()
	r, _ := newHTTPTestRunner(t)
	nodes := []config.WorkflowNode{}
	for index := 1; index <= 4; index++ {
		id := fmt.Sprintf("node-%d", index)
		nodes = append(nodes, config.WorkflowNode{
			ID:      id,
			Type:    config.WorkflowNodeTypeHTTP,
			StageID: "fanout",
			Order:   index * 10,
			HTTP:    config.WorkflowHTTP{URL: server.URL + "/" + id},
		})
	}
	wf := &config.WorkflowConfig{
		ID:     "demo.parallel-limit",
		Stages: []config.WorkflowStage{{ID: "fanout", ExecutionMode: config.WorkflowStageExecutionParallel, MaxConcurrency: 2}},
		Nodes:  nodes,
	}
	if _, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{}); err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	if peak := atomic.LoadInt32(&probe.peak); peak != 2 {
		t.Fatalf("peak concurrency = %d, want 2", peak)
	}
}

func TestRunWorkflowParallelStageRespectsExplicitDependencyAndBarrier(t *testing.T) {
	probe := newParallelProbe()
	server := httptest.NewServer(probe.handler(80 * time.Millisecond))
	defer server.Close()
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID: "demo.parallel-edges",
		Stages: []config.WorkflowStage{
			{ID: "fanout", ExecutionMode: config.WorkflowStageExecutionParallel},
			{ID: "verify"},
		},
		Nodes: []config.WorkflowNode{
			{ID: "prepare", Type: config.WorkflowNodeTypeHTTP, StageID: "fanout", Order: 10, HTTP: config.WorkflowHTTP{URL: server.URL + "/prepare"}},
			{ID: "write-a", Type: config.WorkflowNodeTypeHTTP, StageID: "fanout", Order: 20, HTTP: config.WorkflowHTTP{URL: server.URL + "/write-a"}},
			{ID: "write-b", Type: config.WorkflowNodeTypeHTTP, StageID: "fanout", Order: 30, HTTP: config.WorkflowHTTP{URL: server.URL + "/write-b"}},
			{ID: "health", Type: config.WorkflowNodeTypeHTTP, StageID: "verify", Order: 10, HTTP: config.WorkflowHTTP{URL: server.URL + "/health"}},
		},
		Edges: []config.WorkflowEdge{
			{From: "prepare", To: "write-a"},
			{From: "prepare", To: "write-b"},
			{From: "write-a", To: "health"},
			{From: "write-b", To: "health"},
		},
	}
	if _, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{}); err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	if probe.overlap("/prepare", "/write-a") || probe.overlap("/prepare", "/write-b") {
		t.Fatalf("write nodes started before their explicit dependency finished")
	}
	if !probe.overlap("/write-a", "/write-b") {
		t.Fatalf("independent write nodes should overlap")
	}
	for _, path := range []string{"/write-a", "/write-b"} {
		if probe.overlap(path, "/health") {
			t.Fatalf("%s overlapped with the next stage: stage barrier is broken", path)
		}
	}
}

func TestRunWorkflowParallelStageFailureDoesNotHang(t *testing.T) {
	release := make(chan struct{})
	var slowFinished atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fast":
			w.WriteHeader(http.StatusInternalServerError)
		case "/slow":
			select {
			case <-release:
			case <-time.After(2 * time.Second):
			}
			slowFinished.Store(true)
		case "/after-slow":
			t.Errorf("dependent node must not run after the stage failed")
		}
	}))
	defer server.Close()
	defer close(release)
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID:     "demo.parallel-fail",
		Stages: []config.WorkflowStage{{ID: "fanout", ExecutionMode: config.WorkflowStageExecutionParallel}},
		Nodes: []config.WorkflowNode{
			{ID: "fast", Type: config.WorkflowNodeTypeHTTP, StageID: "fanout", Order: 10, HTTP: config.WorkflowHTTP{URL: server.URL + "/fast"}},
			{ID: "slow", Type: config.WorkflowNodeTypeHTTP, StageID: "fanout", Order: 20, HTTP: config.WorkflowHTTP{URL: server.URL + "/slow"}},
			{ID: "after-slow", Type: config.WorkflowNodeTypeHTTP, StageID: "fanout", Order: 30, HTTP: config.WorkflowHTTP{URL: server.URL + "/after-slow"}},
		},
		Edges: []config.WorkflowEdge{{From: "slow", To: "after-slow"}},
	}
	done := make(chan struct{})
	var record *RunRecord
	var err error
	go func() {
		record, err = r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("parallel stage hung after a node failed")
	}
	if err == nil || record.Status != "failed" {
		t.Fatalf("err = %v status = %s, want failure", err, record.Status)
	}
	statuses := map[string]string{}
	for _, step := range record.Steps {
		statuses[step.ID] = step.Status
	}
	if statuses["fast"] != "failed" || statuses["after-slow"] != "skipped" {
		t.Fatalf("step statuses = %v, want fast failed and after-slow skipped", statuses)
	}
}

func TestRunWorkflowParallelStageCancelsSiblingsAfterFailure(t *testing.T) {
	probe := newParallelProbe()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/boom":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			probe.handler(10*time.Second)(w, r)
		}
	}))
	defer server.Close()
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID:     "demo.parallel-cancel-siblings",
		Stages: []config.WorkflowStage{{ID: "fanout", ExecutionMode: config.WorkflowStageExecutionParallel}},
		Nodes: []config.WorkflowNode{
			{ID: "boom", Type: config.WorkflowNodeTypeHTTP, StageID: "fanout", Order: 10, HTTP: config.WorkflowHTTP{URL: server.URL + "/boom"}},
			{ID: "slow", Type: config.WorkflowNodeTypeHTTP, StageID: "fanout", Order: 20, HTTP: config.WorkflowHTTP{URL: server.URL + "/slow"}},
		},
	}
	started := time.Now()
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err == nil || record.Status != "failed" {
		t.Fatalf("err = %v status = %s, want failure", err, record.Status)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("stage took %v, want siblings cancelled as soon as one node failed", elapsed)
	}
	statuses := map[string]string{}
	for _, step := range record.Steps {
		statuses[step.ID] = step.Status
	}
	if statuses["boom"] != "failed" || statuses["slow"] != "cancelled" {
		t.Fatalf("step statuses = %v, want boom failed and slow cancelled", statuses)
	}
}

func TestRunWorkflowParallelStageSkipsInactiveConditionBranch(t *testing.T) {
	probe := newParallelProbe()
	server := httptest.NewServer(probe.handler(20 * time.Millisecond))
	defer server.Close()
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID:     "demo.parallel-condition",
		Stages: []config.WorkflowStage{{ID: "fanout", ExecutionMode: config.WorkflowStageExecutionParallel}},
		Nodes: []config.WorkflowNode{
			{
				ID:      "route",
				Type:    config.WorkflowNodeTypeCondition,
				StageID: "fanout",
				Order:   10,
				Condition: config.WorkflowCondition{
					Input:       "{{ .mode }}",
					Cases:       []config.ConditionCase{{ID: "fast", Operator: "eq", Values: []string{"fast"}}},
					DefaultCase: "slow",
				},
			},
			{ID: "fast-path", Type: config.WorkflowNodeTypeHTTP, StageID: "fanout", Order: 20, HTTP: config.WorkflowHTTP{URL: server.URL + "/fast-path"}},
			{ID: "slow-path", Type: config.WorkflowNodeTypeHTTP, StageID: "fanout", Order: 30, HTTP: config.WorkflowHTTP{URL: server.URL + "/slow-path"}},
		},
		Edges: []config.WorkflowEdge{
			{From: "route", To: "fast-path", Case: "fast"},
			{From: "route", To: "slow-path", Case: "slow"},
		},
	}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, map[string]string{"mode": "fast"}, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	statuses := map[string]string{}
	for _, step := range record.Steps {
		statuses[step.ID] = step.Status
	}
	if statuses["fast-path"] != "succeeded" || statuses["slow-path"] != "skipped" {
		t.Fatalf("step statuses = %v, want fast-path succeeded and slow-path skipped", statuses)
	}
	if _, ok := probe.starts["/slow-path"]; ok {
		t.Fatalf("inactive branch should not be requested")
	}
}

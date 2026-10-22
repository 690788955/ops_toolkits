package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

func newHTTPTestRunner(t *testing.T) (*Runner, string) {
	t.Helper()
	dir := t.TempDir()
	reg := &registry.Registry{
		BaseDir:   dir,
		Root:      &config.RootConfig{Paths: config.PathsConfig{Logs: "runs/logs"}},
		Tools:     map[string]*registry.Tool{},
		Workflows: map[string]*registry.Workflow{},
	}
	return New(reg), dir
}

func TestRunWorkflowHTTPNodeRendersRequestAndSavesResponse(t *testing.T) {
	runner, dir := newHTTPTestRunner(t)
	var gotMethod, gotToken, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotToken = r.Header.Get("X-Token")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"code":0,"message":"deployed","env":"` + r.URL.Query().Get("env") + `"}`))
	}))
	defer server.Close()

	wf := &config.WorkflowConfig{
		ID:         "demo.http",
		Parameters: []config.Parameter{{Name: "name", Required: true}},
		Nodes: []config.WorkflowNode{{
			ID:   "call",
			Type: config.WorkflowNodeTypeHTTP,
			HTTP: config.WorkflowHTTP{
				Method:       "post",
				URL:          server.URL + "/deploy?env={{ .env }}",
				Headers:      map[string]string{"X-Token": "{{ .token }}"},
				BodyType:     "json",
				Body:         `{"name":"{{ .name }}"}`,
				ExpectStatus: []int{201},
				SaveResponse: "response/deploy.json",
			},
		}},
	}
	record, err := runner.RunWorkflowConfigWithConfirmation(context.Background(), wf, map[string]string{"name": "opsctl", "env": "prod", "token": "t-123"}, true, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflowConfigWithConfirmation error: %v", err)
	}
	if len(record.Steps) != 1 || record.Steps[0].Status != "succeeded" {
		t.Fatalf("steps = %+v, want single succeeded step", record.Steps)
	}
	if record.Steps[0].HTTPStatus != http.StatusCreated {
		t.Fatalf("HTTPStatus = %d, want 201", record.Steps[0].HTTPStatus)
	}
	if gotMethod != "POST" {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotToken != "t-123" {
		t.Fatalf("header X-Token = %q, want t-123", gotToken)
	}
	if gotBody != `{"name":"opsctl"}` {
		t.Fatalf("body = %q", gotBody)
	}
	stepDir := filepath.Join(dir, "runs", "logs", record.ID, "call")
	stdout, err := os.ReadFile(filepath.Join(stepDir, "stdout.log"))
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if !strings.Contains(string(stdout), `"env":"prod"`) {
		t.Fatalf("stdout = %q, want env prod", stdout)
	}
	saved, err := os.ReadFile(filepath.Join(stepDir, "response", "deploy.json"))
	if err != nil {
		t.Fatalf("read saved response: %v", err)
	}
	if string(saved) != string(stdout) {
		t.Fatalf("saved response = %q, want same as stdout %q", saved, stdout)
	}
	metaBytes, err := os.ReadFile(filepath.Join(stepDir, httpResponseMetaFile))
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	var meta httpResponseMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("meta json: %v", err)
	}
	if meta.Status != 201 || meta.ContentType != "application/json" || meta.SavedPath != "response/deploy.json" {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestRunWorkflowHTTPNodeExposesContextToDownstream(t *testing.T) {
	runner, _ := newHTTPTestRunner(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer server.Close()

	wf := &config.WorkflowConfig{
		ID: "demo.http.context",
		Nodes: []config.WorkflowNode{
			{ID: "probe", Type: config.WorkflowNodeTypeHTTP, HTTP: config.WorkflowHTTP{URL: server.URL + "/health"}},
			{
				ID:   "route",
				Type: config.WorkflowNodeTypeCondition,
				Condition: config.WorkflowCondition{
					Input: `{{ .steps.probe.http.status }}-{{ .steps.probe.stdout }}`,
					Cases: []config.ConditionCase{{ID: "ok", Name: "健康", Operator: "contains", Values: []string{"200", "healthy"}}},
				},
			},
		},
		Edges: []config.WorkflowEdge{{From: "probe", To: "route"}},
	}
	record, err := runner.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, true, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if len(record.Steps) != 2 {
		t.Fatalf("steps = %+v", record.Steps)
	}
	if record.Steps[1].ConditionInput != `200-{"status":"healthy"}` {
		t.Fatalf("condition input = %q", record.Steps[1].ConditionInput)
	}
	if record.Steps[1].MatchedCase != "ok" {
		t.Fatalf("matched case = %q, want ok", record.Steps[1].MatchedCase)
	}
}

func TestRunWorkflowHTTPNodeFailsOnUnexpectedStatus(t *testing.T) {
	runner, _ := newHTTPTestRunner(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	wf := &config.WorkflowConfig{
		ID: "demo.http.fail",
		Nodes: []config.WorkflowNode{{
			ID:   "call",
			Type: config.WorkflowNodeTypeHTTP,
			HTTP: config.WorkflowHTTP{URL: server.URL + "/fail", ExpectStatus: []int{200}},
		}},
	}
	record, err := runner.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, true, nilWriter{}, nilWriter{})
	if err == nil || !strings.Contains(err.Error(), "状态码 500") {
		t.Fatalf("error = %v, want 状态码 500", err)
	}
	if record.Steps[0].Status != "failed" || record.Steps[0].HTTPStatus != 500 {
		t.Fatalf("step = %+v", record.Steps[0])
	}
}

func TestRunWorkflowHTTPNodeToleratesStatusWhenFailOnErrorDisabled(t *testing.T) {
	runner, _ := newHTTPTestRunner(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"missing"}`))
	}))
	defer server.Close()

	failOnError := false
	wf := &config.WorkflowConfig{
		ID: "demo.http.tolerant",
		Nodes: []config.WorkflowNode{{
			ID:   "call",
			Type: config.WorkflowNodeTypeHTTP,
			HTTP: config.WorkflowHTTP{URL: server.URL + "/missing", ExpectStatus: []int{200}, FailOnError: &failOnError},
		}},
	}
	record, err := runner.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, true, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if record.Steps[0].Status != "succeeded" || record.Steps[0].HTTPStatus != 404 {
		t.Fatalf("step = %+v", record.Steps[0])
	}
}

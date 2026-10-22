package registry

import (
	"strings"
	"testing"

	"shell_ops/internal/config"
)

func TestValidateWorkflowAcceptsHTTPNode(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{}}
	wf := &config.WorkflowConfig{
		ID: "demo.http",
		Nodes: []config.WorkflowNode{{
			ID:   "call",
			Type: config.WorkflowNodeTypeHTTP,
			HTTP: config.WorkflowHTTP{
				Method:       "POST",
				URL:          "https://example.com/api/deploy",
				Headers:      map[string]string{"Authorization": "Bearer {{ .token }}"},
				BodyType:     "json",
				Body:         `{"env":"{{ .env }}"}`,
				ExpectStatus: []int{200, 201},
				SaveResponse: "response.json",
			},
		}},
	}
	if err := reg.ValidateWorkflow(wf); err != nil {
		t.Fatalf("ValidateWorkflow error = %v", err)
	}
}

func TestValidateWorkflowAcceptsTemplatedHTTPURL(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{}}
	wf := &config.WorkflowConfig{
		ID: "demo.http.tpl",
		Nodes: []config.WorkflowNode{{
			ID:   "call",
			Type: config.WorkflowNodeTypeHTTP,
			HTTP: config.WorkflowHTTP{URL: "{{ .base_url }}/healthz"},
		}},
	}
	if err := reg.ValidateWorkflow(wf); err != nil {
		t.Fatalf("ValidateWorkflow error = %v", err)
	}
}

func TestValidateWorkflowRejectsHTTPNodeWithoutURL(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{}}
	wf := &config.WorkflowConfig{
		ID: "demo.http.empty",
		Nodes: []config.WorkflowNode{{
			ID:   "call",
			Type: config.WorkflowNodeTypeHTTP,
			HTTP: config.WorkflowHTTP{Method: "GET"},
		}},
	}
	err := reg.ValidateWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "http.url 必填") {
		t.Fatalf("ValidateWorkflow error = %v, want http.url 必填", err)
	}
}

func TestValidateWorkflowRejectsHTTPNodeWithUnsupportedScheme(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{}}
	wf := &config.WorkflowConfig{
		ID: "demo.http.scheme",
		Nodes: []config.WorkflowNode{{
			ID:   "call",
			Type: config.WorkflowNodeTypeHTTP,
			HTTP: config.WorkflowHTTP{URL: "file:///etc/passwd"},
		}},
	}
	err := reg.ValidateWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "只支持 http:// 或 https://") {
		t.Fatalf("ValidateWorkflow error = %v, want scheme error", err)
	}
}

func TestValidateWorkflowRejectsHTTPNodeWithEscapingSavePath(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{}}
	wf := &config.WorkflowConfig{
		ID: "demo.http.path",
		Nodes: []config.WorkflowNode{{
			ID:   "call",
			Type: config.WorkflowNodeTypeHTTP,
			HTTP: config.WorkflowHTTP{URL: "https://example.com", SaveResponse: "../escape.json"},
		}},
	}
	err := reg.ValidateWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "响应保存路径") {
		t.Fatalf("ValidateWorkflow error = %v, want 响应保存路径", err)
	}
}

func TestValidateWorkflowRejectsHTTPConfigOnToolNode(t *testing.T) {
	reg := &Registry{Tools: map[string]*Tool{"demo.tool": {}}}
	wf := &config.WorkflowConfig{
		ID: "demo.http.conflict",
		Nodes: []config.WorkflowNode{{
			ID:   "first",
			Tool: "demo.tool",
			HTTP: config.WorkflowHTTP{URL: "https://example.com"},
		}},
	}
	err := reg.ValidateWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "不能配置 http") {
		t.Fatalf("ValidateWorkflow error = %v, want 不能配置 http", err)
	}
}

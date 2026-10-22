package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWorkflowNodeOmitsEmptyHTTPBlock(t *testing.T) {
	node := WorkflowNode{ID: "first", Type: WorkflowNodeTypeTool, Tool: "demo.tool"}
	data, err := yaml.Marshal(node)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "http:") {
		t.Fatalf("yaml = %s, want no http block", data)
	}
}

func TestWorkflowNodeMarshalsHTTPBlock(t *testing.T) {
	node := WorkflowNode{
		ID:   "call",
		Type: WorkflowNodeTypeHTTP,
		HTTP: WorkflowHTTP{Method: "POST", URL: "https://example.com/api", ExpectStatus: []int{201}},
	}
	data, err := yaml.Marshal(node)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	for _, want := range []string{"type: http_request", "url: https://example.com/api", "expect_status:", "- 201"} {
		if !strings.Contains(text, want) {
			t.Fatalf("yaml = %s, want %q", text, want)
		}
	}
}

func TestNormalizeHTTPMethodAndBodyType(t *testing.T) {
	if method, err := NormalizeHTTPMethod(" post "); err != nil || method != "POST" {
		t.Fatalf("method = %q, err = %v", method, err)
	}
	if method, err := NormalizeHTTPMethod(""); err != nil || method != "GET" {
		t.Fatalf("default method = %q, err = %v", method, err)
	}
	if _, err := NormalizeHTTPMethod("TRACE"); err == nil {
		t.Fatalf("TRACE should be rejected")
	}
	if bodyType, err := NormalizeHTTPBodyType("", `{"a":1}`); err != nil || bodyType != "json" {
		t.Fatalf("body type = %q, err = %v", bodyType, err)
	}
	if bodyType, err := NormalizeHTTPBodyType("", ""); err != nil || bodyType != "none" {
		t.Fatalf("empty body type = %q, err = %v", bodyType, err)
	}
}

func TestValidateHTTPConfigExpectStatusAndSavePath(t *testing.T) {
	if _, err := ValidateHTTPConfig("call", WorkflowHTTP{URL: "https://example.com", ExpectStatus: []int{999}}); err == nil {
		t.Fatalf("999 should be rejected")
	}
	if _, err := ValidateHTTPConfig("call", WorkflowHTTP{URL: "https://example.com", SaveResponse: "/abs.json"}); err == nil {
		t.Fatalf("absolute save path should be rejected")
	}
	if _, err := ValidateHTTPConfig("call", WorkflowHTTP{URL: "{{ .base }}/health"}); err != nil {
		t.Fatalf("templated url error: %v", err)
	}
}

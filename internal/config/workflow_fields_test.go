package config

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWorkflowExtraFieldsRoundTrip(t *testing.T) {
	source := []byte(`{
		"id": "demo.stages",
		"x_workflow": {"enabled": true, "items": [null, {"name": "保留", "large": 9007199254740993}], "zero": 0, "fraction": 1.5},
		"x_null": null,
		"stages": [{"id": "prepare", "name": "准备", "execution_mode": "serial", "x_stage": {"limit": 18446744073709551615}}],
		"nodes": [{"id": "first", "type": "tool", "tool": "demo.tool", "stage_id": "prepare", "order": 10,
			"timeout": "2m", "confirm": true, "optional": true, "x_node": {"min": -9223372036854775808, "empty": [], "object": {}}}],
		"edges": [{"from": "first", "to": "second", "x_edge": [false, "说明", null]}]
	}`)
	var workflow WorkflowConfig
	if err := json.Unmarshal(source, &workflow); err != nil {
		t.Fatal(err)
	}
	yamlData, err := yaml.Marshal(workflow)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded WorkflowConfig
	if err := yaml.Unmarshal(yamlData, &reloaded); err != nil {
		t.Fatalf("YAML reload failed: %v\n%s", err, yamlData)
	}
	encoded, err := json.Marshal(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	want := decodeWorkflowJSONForTest(t, source)
	got := decodeWorkflowJSONForTest(t, encoded)
	for _, key := range []string{"x_workflow", "x_null"} {
		if !reflect.DeepEqual(got[key], want[key]) {
			t.Errorf("workflow %s = %#v, want %#v", key, got[key], want[key])
		}
	}
	if _, ok := got["x_null"]; !ok {
		t.Error("显式 null 扩展字段被丢弃")
	}
	for collection, extension := range map[string]string{"stages": "x_stage", "nodes": "x_node", "edges": "x_edge"} {
		gotItem := got[collection].([]interface{})[0].(map[string]interface{})
		wantItem := want[collection].([]interface{})[0].(map[string]interface{})
		if !reflect.DeepEqual(gotItem[extension], wantItem[extension]) {
			t.Errorf("%s extension = %#v, want %#v", collection, gotItem[extension], wantItem[extension])
		}
		for key, value := range wantItem {
			if !reflect.DeepEqual(gotItem[key], value) {
				t.Errorf("%s known field %s = %#v, want %#v", collection, key, gotItem[key], value)
			}
		}
	}
}

func TestWorkflowYAMLExtraFieldsSurviveJSON(t *testing.T) {
	source := []byte(`id: demo.flow
x_workflow: {nested: [null, true, 9007199254740993]}
stages:
  - id: prepare
    x_stage: 配置
nodes:
  - id: first
    type: tool
    tool: demo.tool
    x_node: {flag: false}
edges:
  - from: first
    to: second
    x_edge: null
`)
	var original WorkflowConfig
	if err := yaml.Unmarshal(source, &original); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded WorkflowConfig
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	actual, err := yaml.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded WorkflowConfig
	if err := yaml.Unmarshal(actual, &reloaded); err != nil {
		t.Fatal(err)
	}
	for _, fields := range []struct {
		name string
		got  map[string]interface{}
		want map[string]interface{}
	}{
		{"workflow", reloaded.ExtraFields, original.ExtraFields},
		{"stage", reloaded.Stages[0].ExtraFields, original.Stages[0].ExtraFields},
		{"node", reloaded.Nodes[0].ExtraFields, original.Nodes[0].ExtraFields},
		{"edge", reloaded.Edges[0].ExtraFields, original.Edges[0].ExtraFields},
	} {
		if !reflect.DeepEqual(fields.got, fields.want) {
			t.Errorf("%s YAML extensions = %#v, want %#v", fields.name, fields.got, fields.want)
		}
	}
}

func TestWorkflowExtraFieldsCannotOverrideKnownJSONFields(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
		key   string
		want  interface{}
	}{
		{"workflow", WorkflowConfig{ID: "actual", ExtraFields: map[string]interface{}{"id": "shadow", "stages": "invalid", "steps": "hidden", "x": true}}, "id", "actual"},
		{"stage", WorkflowStage{ID: "actual", ExtraFields: map[string]interface{}{"ID": "shadow", "execution_mode": "parallel", "x": true}}, "id", "actual"},
		{"node", WorkflowNode{Order: 10, ExtraFields: map[string]interface{}{"order": -1, "stage_id": "shadow", "x": true}}, "order", json.Number("10")},
		{"edge", WorkflowEdge{From: "actual", ExtraFields: map[string]interface{}{"from": "shadow", "x": true}}, "from", "actual"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(test.value)
			if err != nil {
				t.Fatal(err)
			}
			got := decodeWorkflowJSONForTest(t, data)
			if !reflect.DeepEqual(got[test.key], test.want) || got["x"] != true {
				t.Errorf("encoded = %s", data)
			}
			for _, key := range []string{"ExtraFields", "stages", "steps", "execution_mode", "stage_id", "ID"} {
				if _, ok := got[key]; ok {
					t.Errorf("shadow field %s should remain omitted: %s", key, data)
				}
			}
		})
	}
}

func TestWorkflowExtraFieldsJSONDecodingErrors(t *testing.T) {
	cases := []struct {
		name   string
		data   string
		target interface{}
	}{
		{"workflow known field", `{"id":42}`, &WorkflowConfig{}},
		{"stage known field", `{"max_concurrency":"two"}`, &WorkflowStage{}},
		{"node known field", `{"order":"first"}`, &WorkflowNode{}},
		{"edge known field", `{"from":false}`, &WorkflowEdge{}},
		{"unknown invalid value", `{"x":}`, &WorkflowConfig{}},
		{"array instead of object", `[]`, &WorkflowConfig{}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(test.data), test.target); err == nil {
				t.Fatalf("json.Unmarshal(%s) should fail", test.data)
			}
		})
	}
}

func TestWorkflowExtraFieldsCannotOverrideKnownYAMLFields(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
		key   string
		want  interface{}
	}{
		{"workflow", WorkflowConfig{ID: "actual", ExtraFields: map[string]interface{}{"id": "shadow", "stages": "invalid", "x": true}}, "id", "actual"},
		{"stage", WorkflowStage{ID: "actual", ExtraFields: map[string]interface{}{"id": "shadow", "execution_mode": "parallel", "x": true}}, "id", "actual"},
		{"node", WorkflowNode{Order: 10, ExtraFields: map[string]interface{}{"order": -1, "stage_id": "shadow", "x": true}}, "order", 10},
		{"edge", WorkflowEdge{From: "actual", ExtraFields: map[string]interface{}{"from": "shadow", "x": true}}, "from", "actual"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			data, err := yaml.Marshal(test.value)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]interface{}
			if err := yaml.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got[test.key], test.want) || got["x"] != true {
				t.Errorf("encoded = %s", data)
			}
			for _, key := range []string{"ExtraFields", "stages", "execution_mode", "stage_id"} {
				if _, ok := got[key]; ok {
					t.Errorf("shadow field %s should remain omitted: %s", key, data)
				}
			}
		})
	}
}

func TestWorkflowExtraFieldsJSONEncodingErrors(t *testing.T) {
	workflow := WorkflowConfig{ExtraFields: map[string]interface{}{"x": make(chan int)}}
	if _, err := json.Marshal(workflow); err == nil {
		t.Fatal("不支持的扩展字段类型应返回编码错误")
	}
}

func TestWorkflowKnownJSONFieldsDoNotBecomeExtensions(t *testing.T) {
	var workflow WorkflowConfig
	if err := json.Unmarshal([]byte(`{"ID":"demo.flow","stages":[{"id":"prepare"}],"nodes":[{"id":"a","order":10}],"edges":[{"from":"a","to":"b"}]}`), &workflow); err != nil {
		t.Fatal(err)
	}
	if workflow.ID != "demo.flow" || len(workflow.ExtraFields) != 0 || len(workflow.Stages[0].ExtraFields) != 0 || len(workflow.Nodes[0].ExtraFields) != 0 || len(workflow.Edges[0].ExtraFields) != 0 {
		t.Fatalf("已知字段被捕获为扩展: %#v", workflow)
	}
}

func TestWorkflowExtraFieldsCannotShadowUnicodeKnownFields(t *testing.T) {
	var workflow WorkflowConfig
	if err := yaml.Unmarshal([]byte("id: demo.unicode\nstages: [{id: prepare}]\nſtages: [{id: replaced}]\nnodes:\n  - id: first\n    type: shell\n    script: {script: echo safe}\n    ſcript: {script: echo changed}\nedges: [{from: first, to: second, case: original, caſe: replaced}]\n"), &workflow); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(workflow)
	if err != nil {
		t.Fatal(err)
	}
	var clone WorkflowConfig
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	if clone.Stages[0].ID != "prepare" || clone.Nodes[0].Script.Script != "echo safe" || clone.Edges[0].Case != "original" {
		t.Errorf("Unicode 扩展字段改变了 JSON 克隆后的执行配置: %s", data)
	}
	encodedYAML, err := yaml.Marshal(workflow)
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"ſtages", "ſcript", "caſe"} {
		if bytes.Contains(data, []byte(alias)) || bytes.Contains(encodedYAML, []byte(alias)) {
			t.Errorf("等价于已知字段的 Unicode 扩展 %q 未被过滤", alias)
		}
	}
}

func TestWorkflowKnownUnicodeJSONFieldsDoNotBecomeExtensions(t *testing.T) {
	var workflow WorkflowConfig
	if err := json.Unmarshal([]byte("{\"ſtages\":[{\"id\":\"prepare\"}],\"nodes\":[{\"ſcript\":{\"script\":\"true\"}}],\"edges\":[{\"caſe\":\"choice\"}]}"), &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.ExtraFields) != 0 || len(workflow.Nodes[0].ExtraFields) != 0 || len(workflow.Edges[0].ExtraFields) != 0 {
		t.Fatalf("JSON 已识别的 Unicode 字段不应再次保存为扩展: %+v", workflow)
	}
	if workflow.Stages[0].ID != "prepare" || workflow.Nodes[0].Script.Script != "true" || workflow.Edges[0].Case != "choice" {
		t.Fatalf("已知 Unicode 字段未按 JSON 规则解码: %+v", workflow)
	}
}

func TestWorkflowExtraFieldsRejectUnreloadableYAMLNumbers(t *testing.T) {
	for _, number := range []string{"18446744073709551616", "-9223372036854775809", "1e309", "-1e309"} {
		t.Run(number, func(t *testing.T) {
			var workflow WorkflowConfig
			if err := json.Unmarshal([]byte("{\"x_nested\":{\"items\":["+number+"]}}"), &workflow); err != nil {
				t.Fatal(err)
			}
			data, err := yaml.Marshal(workflow)
			if err == nil {
				var reloaded WorkflowConfig
				t.Fatalf("不应写出无法重载的 YAML 数值 %s: reload=%v\n%s", number, yaml.Unmarshal(data, &reloaded), data)
			}
			if !strings.Contains(err.Error(), number) || !strings.Contains(err.Error(), "YAML") {
				t.Fatalf("数值范围错误应包含具体数值及目标格式: %v", err)
			}
		})
	}
}

func TestWorkflowYAMLIntegerMapKeysSurviveJSON(t *testing.T) {
	var workflow WorkflowConfig
	if err := yaml.Unmarshal([]byte("id: demo.keys\nx_statuses:\n  200: ok\n  503: {retry: true}\nx_nested:\n  - details: {404: missing}\n"), &workflow); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(workflow)
	if err != nil {
		t.Fatalf("普通 YAML 整数映射键应归一化为 JSON 对象键: %v", err)
	}
	got := decodeWorkflowJSONForTest(t, data)
	want := map[string]interface{}{"200": "ok", "503": map[string]interface{}{"retry": true}}
	if !reflect.DeepEqual(got["x_statuses"], want) {
		t.Fatalf("整数映射键归一化丢失数据: %#v", got["x_statuses"])
	}
	if nested := got["x_nested"].([]interface{})[0].(map[string]interface{})["details"]; !reflect.DeepEqual(nested, map[string]interface{}{"404": "missing"}) {
		t.Fatalf("嵌套数组中的整数映射键未归一化: %#v", nested)
	}
	if _, ok := workflow.ExtraFields["x_statuses"].(map[interface{}]interface{}); !ok {
		t.Fatal("JSON 编码修改了原始 YAML 扩展映射")
	}
	var clone WorkflowConfig
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	yamlData, err := yaml.Marshal(clone)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded WorkflowConfig
	if err := yaml.Unmarshal(yamlData, &reloaded); err != nil {
		t.Fatalf("归一化扩展字段写入的 YAML 无法重载: %v", err)
	}
	encoded, err := json.Marshal(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if actual := decodeWorkflowJSONForTest(t, encoded); !reflect.DeepEqual(actual["x_statuses"], want) {
		t.Fatalf("保存重载后扩展映射变化: %#v", actual["x_statuses"])
	}
}

func decodeWorkflowJSONForTest(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var value map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

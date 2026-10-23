package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"shell_ops/internal/registry"
)

func TestWorkflowSaveReloadPreservesStageExtensions(t *testing.T) {
	reg := testRegistry(t)
	body := `{"workflow":{
		"id":"demo.stage-roundtrip","name":"阶段往返","category":"demo",
		"x_workflow":{"enabled":true},
		"stages":[{"id":"prepare","name":"准备","x_stage":["one",null]}],
		"nodes":[
			{"id":"first","tool":"demo.hello","stage_id":"prepare","order":10,"timeout":"2m","x_node":{"nested":true}},
			{"id":"second","tool":"demo.hello","stage_id":"prepare","order":20}
		],
		"edges":[{"from":"first","to":"second","x_edge":"业务依赖"}]
	}}`
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.stage-roundtrip/save", strings.NewReader(body))
	res := httptest.NewRecorder()
	NewHandler(reg).ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", res.Code, res.Body.String())
	}
	installDemoToolPlugin(t, reg.BaseDir)
	reloaded, err := registry.Load(reg.BaseDir)
	if err != nil {
		t.Fatal(err)
	}
	wf := reloaded.Workflows["demo.stage-roundtrip"]
	if wf == nil {
		t.Fatal("保存的工作流未被重新加载")
	}
	raw, err := json.Marshal(wf.Config)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	stage := got["stages"].([]interface{})[0].(map[string]interface{})
	node := got["nodes"].([]interface{})[0].(map[string]interface{})
	edge := got["edges"].([]interface{})[0].(map[string]interface{})
	if !reflect.DeepEqual(got["x_workflow"], map[string]interface{}{"enabled": true}) ||
		!reflect.DeepEqual(stage["x_stage"], []interface{}{"one", nil}) ||
		!reflect.DeepEqual(node["x_node"], map[string]interface{}{"nested": true}) ||
		edge["x_edge"] != "业务依赖" {
		t.Fatalf("扩展字段往返后发生变化: %s", raw)
	}
	if node["stage_id"] != "prepare" || node["order"] != float64(10) || node["timeout"] != "2m" {
		t.Fatalf("节点字段往返后发生变化: %+v", node)
	}
}

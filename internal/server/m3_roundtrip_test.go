package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

func TestWorkflowRunTargetOnlyAppliesToNodesThatExecuteOnHosts(t *testing.T) {
	reg := testRegistry(t)
	writeTestSSHHosts(t, reg.BaseDir)
	wf := &config.WorkflowConfig{ID: "demo.targets", Nodes: []config.WorkflowNode{
		{ID: "tool", Tool: "demo.hello"},
		{ID: "shell", Type: "shell", Script: config.WorkflowScript{Script: "true"}},
		{ID: "checksum", Type: "file_checksum", Checksum: config.WorkflowChecksum{Path: "file"}},
		{ID: "archive", Type: "archive", Host: "stale-host"},
		{ID: "http", Type: "http_request"},
		{ID: "wait", Type: "wait"},
	}}
	state := newServerState(reg)
	got, err := applyWorkflowRunTarget(state, wf, "ops-live", true)
	if err != nil {
		t.Fatal(err)
	}
	for index, node := range got.Nodes {
		want := "ops-live"
		if index >= 3 {
			want = ""
		}
		if node.Host != want {
			t.Errorf("节点 %s 的 host = %q, want %q", node.ID, node.Host, want)
		}
	}
	if wf.Nodes[3].Host != "stale-host" {
		t.Fatal("运行目标覆盖修改了原始工作流")
	}
	if err := validateWorkflowTargets(nil, &config.WorkflowConfig{Nodes: wf.Nodes[3:]}); err != nil {
		t.Fatalf("本地节点不应依赖 SSH 配置: %v", err)
	}
}

func TestWorkflowSaveReloadPreservesM3ExecutionConfig(t *testing.T) {
	reg := testRegistry(t)
	body := `{"workflow":{
		"id":"demo.m3-roundtrip","name":"M3 往返","category":"demo",
		"stages":[{"id":"prepare","execution_mode":"parallel","max_concurrency":2}],
		"nodes":[
			{"id":"shell","type":"shell","host":"original-host","stage_id":"prepare","order":10,
			 "script":{"script":"printf %s \"$APP_VALUE\"","workdir":"/tmp/app","env":{"APP_VALUE":"{{ .Value }}"},"fail_on_exit":false,"timeout":"2m"}},
			{"id":"digest","type":"file_checksum","host":"original-host","stage_id":"prepare","order":20,
			 "checksum":{"path":"/tmp/app/data","algorithm":"sha256","expected":"{{ .Digest }}"}},
			{"id":"archive","type":"archive","stage_id":"prepare","order":30,
			 "archive":{"path":"bundle.zip","target_dir":"out","format":"zip","strip_components":1,"overwrite":true}},
			{"id":"poll","type":"wait","stage_id":"prepare","order":40,
			 "wait":{"poll_url":"https://example.invalid/ready","headers":{"X-Access":"value"},"expect_status":[200,204],"interval":"1s","max_wait":"20s","max_attempts":3}}
		],
		"edges":[{"from":"shell","to":"digest"}]
	}}`
	res := httptest.NewRecorder()
	NewHandler(reg).ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/workflows/demo.m3-roundtrip/save", strings.NewReader(body)))
	if res.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", res.Code, res.Body.String())
	}
	installDemoToolPlugin(t, reg.BaseDir)
	reloaded, err := registry.Load(reg.BaseDir)
	if err != nil {
		t.Fatal(err)
	}
	wf := reloaded.Workflows["demo.m3-roundtrip"]
	if wf == nil {
		t.Fatal("保存的 M3 工作流未被重载")
	}
	stage := wf.Config.Stages[0]
	if stage.ExecutionMode != "parallel" || stage.MaxConcurrency != 2 || len(wf.Config.Edges) != 1 {
		t.Fatalf("并行计划往返后变化: %+v", wf.Config)
	}
	nodes := wf.Config.Nodes
	if nodes[0].Host != "original-host" || nodes[1].Host != "original-host" ||
		nodes[0].Script.Env["APP_VALUE"] != "{{ .Value }}" || nodes[0].Script.Workdir != "/tmp/app" ||
		nodes[0].Script.Timeout != "2m" || nodes[0].Script.FailOnExit == nil || *nodes[0].Script.FailOnExit ||
		nodes[1].Checksum.Expected != "{{ .Digest }}" {
		t.Fatalf("远程脚本或摘要配置往返后变化: %+v", nodes)
	}
	if nodes[2].Archive.Strip != 1 || !nodes[2].Archive.Overwrite || nodes[3].Wait.MaxAttempts != 3 ||
		nodes[3].Wait.MaxWait != "20s" || nodes[3].Wait.Headers["X-Access"] != "value" || len(nodes[3].Wait.ExpectStatus) != 2 {
		t.Fatalf("本地解压或轮询配置往返后变化: %+v", nodes)
	}
}

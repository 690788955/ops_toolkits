package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"shell_ops/internal/runbundle"
)

// 工作流编辑器左栏要的是“这个工作流的运行记录”，只能用精确的 kind/target 过滤；
// 走 q 模糊搜会把参数或审批提示里恰好提到该 ID 的运行也带出来。
func TestRunListFiltersByKindAndTarget(t *testing.T) {
	reg := testRegistry(t)
	state := newServerState(reg)
	writeRunFixture(t, reg.BaseDir, "wf-a-1", `{"id":"wf-a-1","kind":"workflow","target":"demo.flow","status":"succeeded"}`)
	writeRunFixture(t, reg.BaseDir, "wf-b-1", `{"id":"wf-b-1","kind":"workflow","target":"other.flow","status":"failed"}`)
	writeRunFixture(t, reg.BaseDir, "tool-1", `{"id":"tool-1","kind":"tool","target":"demo.flow","status":"succeeded","params":{"note":"wf-a-1"}}`)

	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "no filter", query: "", want: []string{"wf-a-1", "wf-b-1", "tool-1"}},
		{name: "kind workflow", query: "kind=workflow", want: []string{"wf-a-1", "wf-b-1"}},
		{name: "kind tool", query: "kind=tool", want: []string{"tool-1"}},
		{name: "target only", query: "target=demo.flow", want: []string{"wf-a-1", "tool-1"}},
		{name: "workflow of target", query: "kind=workflow&target=demo.flow", want: []string{"wf-a-1"}},
		{name: "unknown target", query: "kind=workflow&target=nope", want: nil},
		// q 仍然模糊命中参数文本，说明精确过滤不能靠它。
		{name: "q matches params", query: "q=wf-a-1", want: []string{"wf-a-1", "tool-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/runs/?"+tc.query, nil)
			res := httptest.NewRecorder()
			runsHandler(state)(res, req)
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
			}
			var body struct {
				Data struct {
					Runs  []runbundle.Summary `json:"runs"`
					Total int                 `json:"total"`
				} `json:"data"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			got := make(map[string]bool, len(body.Data.Runs))
			for _, item := range body.Data.Runs {
				got[item.ID] = true
			}
			if len(got) != len(tc.want) {
				t.Fatalf("runs = %v, want %v", got, tc.want)
			}
			for _, id := range tc.want {
				if !got[id] {
					t.Fatalf("runs = %v, want %v", got, tc.want)
				}
			}
			if body.Data.Total != len(tc.want) {
				t.Fatalf("total = %d, want %d", body.Data.Total, len(tc.want))
			}
		})
	}
}

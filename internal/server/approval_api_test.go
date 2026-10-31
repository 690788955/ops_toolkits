package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// approvalDraftBody 只包含审批节点：审批接口的测试不需要真实工具，避免依赖 bash。
const approvalDraftBody = `{"params":{},"workflow":{"id":"demo.approval-api","name":"审批草稿","stages":[{"id":"change","name":"变更","execution_mode":"serial"}],"nodes":[{"id":"gate","type":"approval","stage_id":"change","order":10,"approval":{"message":"改好配置后继续"}}],"edges":[]}}`

func TestRunApprovalAPIApprovesWaitingNode(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	runID := startDraftRun(t, handler, approvalDraftBody)
	waitForRunBody(t, handler, runID, `"status":"waiting"`)
	// 等待期间运行本身仍是 running，前端据此保持轮询与审批条。
	waitForRunBody(t, handler, runID, `"approvals":[{"node_id":"gate"`)

	body := `{"note":"配置已改好","reviewer":"tester"}`
	req := httptest.NewRequest(http.MethodPost, "/api/runs/"+runID+"/approvals/gate/approve", strings.NewReader(body))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("approve status = %d, body = %s", res.Code, res.Body.String())
	}
	final := waitForRunBody(t, handler, runID, `"status":"succeeded"`)
	for _, want := range []string{`"status":"approved"`, `"note":"配置已改好"`, `"reviewer":"tester"`} {
		if !strings.Contains(final, want) {
			t.Fatalf("审批结果缺少 %s: %s", want, final)
		}
	}
}

func TestRunApprovalAPIHandlesEscapedNodeIDs(t *testing.T) {
	for _, nodeID := range []string{"gate/1", "gate%2F1", "gate+1"} {
		t.Run(nodeID, func(t *testing.T) {
			reg := testRegistry(t)
			handler := NewHandler(reg)
			draft := strings.Replace(approvalDraftBody, `"id":"gate"`, `"id":"`+nodeID+`"`, 1)
			runID := startDraftRun(t, handler, draft)
			waitForRunBody(t, handler, runID, `"node_id":"`+nodeID+`"`)
			req := httptest.NewRequest(http.MethodPost, "/api/runs/"+runID+"/approvals/"+url.PathEscape(nodeID)+"/approve", strings.NewReader(`{}`))
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != http.StatusOK {
				t.Fatalf("approve status = %d, body = %s", res.Code, res.Body.String())
			}
			waitForRunBody(t, handler, runID, `"status":"succeeded"`)
		})
	}
}

// 列表接口要能单独筛出“卡在人工审批”的运行：状态仍是 running，只有 approvals 能区分。
func TestRunListAPIExposesAwaitingApproval(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	runID := startDraftRun(t, handler, approvalDraftBody)
	waitForRunBody(t, handler, runID, `"status":"waiting"`)

	var list struct {
		Data struct {
			Runs  []map[string]interface{} `json:"runs"`
			Total int                      `json:"total"`
		} `json:"data"`
	}
	body := getJSONBody(t, handler, "/api/runs/?awaiting=1&limit=0")
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("解析运行列表失败: %v; body=%s", err, body)
	}
	if list.Data.Total != 1 {
		t.Fatalf("awaiting total = %d, want 1; body=%s", list.Data.Total, body)
	}

	full := getJSONBody(t, handler, "/api/runs/?awaiting=1")
	if err := json.Unmarshal([]byte(full), &list); err != nil {
		t.Fatalf("解析运行列表失败: %v; body=%s", err, full)
	}
	if len(list.Data.Runs) != 1 || list.Data.Runs[0]["awaiting_approval"] != true {
		t.Fatalf("runs = %#v, want one awaiting approval entry", list.Data.Runs)
	}
	approval, _ := list.Data.Runs[0]["approval"].(map[string]interface{})
	if approval["node_id"] != "gate" || approval["message"] != "改好配置后继续" {
		t.Fatalf("approval = %#v", approval)
	}
	// 这条运行由本进程执行，Web 端提交审批才是可行的。
	if list.Data.Runs[0]["approval_actionable"] != true {
		t.Fatalf("approval_actionable = %#v, want true", list.Data.Runs[0]["approval_actionable"])
	}

	// 审批后该运行不再出现在 awaiting 过滤里。
	req := httptest.NewRequest(http.MethodPost, "/api/runs/"+runID+"/approvals/gate/approve", strings.NewReader(`{}`))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("approve status = %d, body = %s", res.Code, res.Body.String())
	}
	waitForRunBody(t, handler, runID, `"status":"succeeded"`)
	if after := getJSONBody(t, handler, "/api/runs/?awaiting=1&limit=0"); !strings.Contains(after, `"total":0`) {
		t.Fatalf("awaiting total after approve = %s, want 0", after)
	}
}

func getJSONBody(t *testing.T, handler http.Handler, path string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, body = %s", path, res.Code, res.Body.String())
	}
	return res.Body.String()
}

func TestRunApprovalAPIRejectsWaitingNode(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	runID := startDraftRun(t, handler, approvalDraftBody)
	waitForRunBody(t, handler, runID, `"status":"waiting"`)

	req := httptest.NewRequest(http.MethodPost, "/api/runs/"+runID+"/approvals/gate/reject", strings.NewReader(`{"note":"配置写错了"}`))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("reject status = %d, body = %s", res.Code, res.Body.String())
	}
	final := waitForRunBody(t, handler, runID, `"status":"failed"`)
	for _, want := range []string{`"status":"rejected"`, "驳回", "配置写错了"} {
		if !strings.Contains(final, want) {
			t.Fatalf("驳回结果缺少 %s: %s", want, final)
		}
	}
}

func TestRunApprovalAPIRejectsUnknownWaitingNode(t *testing.T) {
	reg := testRegistry(t)
	handler := NewHandler(reg)
	runID := startDraftRun(t, handler, approvalDraftBody)
	waitForRunBody(t, handler, runID, `"status":"waiting"`)

	req := httptest.NewRequest(http.MethodPost, "/api/runs/"+runID+"/approvals/missing/approve", strings.NewReader(`{}`))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("unknown node status = %d, want 400; body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "不在等待人工审批") {
		t.Fatalf("body = %s, want 不在等待人工审批", res.Body.String())
	}
}

func startDraftRun(t *testing.T, handler http.Handler, body string) string {
	t.Helper()
	startReq := httptest.NewRequest(http.MethodPost, "/api/workflows/demo.approval-api/run?async=true", strings.NewReader(body))
	startRes := httptest.NewRecorder()
	handler.ServeHTTP(startRes, startReq)
	if startRes.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", startRes.Code, startRes.Body.String())
	}
	var started response
	if err := json.Unmarshal(startRes.Body.Bytes(), &started); err != nil {
		t.Fatalf("解析运行响应失败: %v", err)
	}
	if strings.TrimSpace(started.ID) == "" {
		t.Fatalf("运行响应缺少 ID: %s", startRes.Body.String())
	}
	return started.ID
}

// waitForRunBody 轮询运行详情，直到出现期望片段；返回最后一次响应体便于断言完整内容。
func waitForRunBody(t *testing.T, handler http.Handler, runID, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		runReq := httptest.NewRequest(http.MethodGet, "/api/runs/"+runID, nil)
		runRes := httptest.NewRecorder()
		handler.ServeHTTP(runRes, runReq)
		body := runRes.Body.String()
		if runRes.Code == http.StatusOK && strings.Contains(body, want) {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf("运行未到达期望状态 %s: code=%d body=%s", want, runRes.Code, body)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

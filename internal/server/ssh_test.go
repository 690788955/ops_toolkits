package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shell_ops/internal/sshmgr"
)

func TestSSHHostsAPI(t *testing.T) {
	handler := NewHandler(testRegistry(t))

	createBody := `{"name":"Web 01","address":"127.0.0.1","port":1,"user":"root","auth":"password","password":"secret","strict_host_key":false,"enabled":true}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/ssh/hosts", strings.NewReader(createBody))
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createRes.Code, createRes.Body.String())
	}
	var created struct {
		Data sshmgr.HostView `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Data.ID == "" || !created.Data.HasPassword {
		t.Fatalf("unexpected created host: %+v", created.Data)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/ssh/hosts", nil)
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRes.Code, listRes.Body.String())
	}
	if strings.Contains(listRes.Body.String(), `"secret"`) {
		t.Fatalf("list response must not expose password: %s", listRes.Body.String())
	}
	var overview struct {
		Data sshmgr.Overview `json:"data"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &overview); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(overview.Data.Hosts) != 1 || overview.Data.Hosts[0].ID != created.Data.ID {
		t.Fatalf("unexpected overview: %+v", overview.Data)
	}

	updateBody := `{"name":"Web 01 renamed","address":"127.0.0.1","port":2,"user":"root","auth":"password"}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/ssh/hosts/"+created.Data.ID, strings.NewReader(updateBody))
	updateRes := httptest.NewRecorder()
	handler.ServeHTTP(updateRes, updateReq)
	if updateRes.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateRes.Code, updateRes.Body.String())
	}
	var updated struct {
		Data sshmgr.HostView `json:"data"`
	}
	if err := json.Unmarshal(updateRes.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.Data.Name != "Web 01 renamed" || updated.Data.Port != 2 || !updated.Data.HasPassword {
		t.Fatalf("unexpected updated host: %+v", updated.Data)
	}

	settingsReq := httptest.NewRequest(http.MethodPut, "/api/ssh/settings", strings.NewReader(`{"interval_seconds":15,"timeout_seconds":5}`))
	settingsRes := httptest.NewRecorder()
	handler.ServeHTTP(settingsRes, settingsReq)
	if settingsRes.Code != http.StatusOK {
		t.Fatalf("settings status = %d, body = %s", settingsRes.Code, settingsRes.Body.String())
	}
	if !strings.Contains(settingsRes.Body.String(), `"interval_seconds":15`) {
		t.Fatalf("settings response missing interval: %s", settingsRes.Body.String())
	}

	checkAllReq := httptest.NewRequest(http.MethodPost, "/api/ssh/check", nil)
	checkAllRes := httptest.NewRecorder()
	handler.ServeHTTP(checkAllRes, checkAllReq)
	if checkAllRes.Code != http.StatusOK {
		t.Fatalf("check all status = %d, body = %s", checkAllRes.Code, checkAllRes.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/ssh/hosts/"+created.Data.ID, nil)
	deleteRes := httptest.NewRecorder()
	handler.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRes.Code, deleteRes.Body.String())
	}
}

func TestSSHHostsAPIValidation(t *testing.T) {
	handler := NewHandler(testRegistry(t))

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{name: "missing password", method: http.MethodPost, path: "/api/ssh/hosts", body: `{"address":"127.0.0.1","user":"root","auth":"password"}`, status: http.StatusBadRequest},
		{name: "invalid interval", method: http.MethodPut, path: "/api/ssh/settings", body: `{"interval_seconds":1,"timeout_seconds":5}`, status: http.StatusBadRequest},
		{name: "unknown host update", method: http.MethodPut, path: "/api/ssh/hosts/nope", body: `{"address":"127.0.0.1","user":"root","auth":"key"}`, status: http.StatusNotFound},
		{name: "unknown host check", method: http.MethodPost, path: "/api/ssh/hosts/nope/check", body: `{}`, status: http.StatusNotFound},
		{name: "empty command", method: http.MethodPost, path: "/api/ssh/hosts/nope/exec", body: `{"command":""}`, status: http.StatusBadRequest},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			req := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != testCase.status {
				t.Fatalf("status = %d, want %d, body = %s", res.Code, testCase.status, res.Body.String())
			}
		})
	}
}

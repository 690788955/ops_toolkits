package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"shell_ops/internal/runbundle"
)

func TestRecoveryClaimsRunBeforeReadingState(t *testing.T) {
	for _, action := range []string{"resolve", "notification"} {
		t.Run(action, func(t *testing.T) {
			reg := testRegistry(t)
			state := newServerState(reg)
			writeRunFixture(t, reg.BaseDir, "pending-run", `{"id":"pending-run","kind":"workflow","status":"pending","pending":{"status":"pending"},"notifications":[{"status":"failed"}]}`)
			// 阻塞状态读取，确保领取发生在读取之前，避免旧状态在前一请求完成后被再次使用。
			state.runFileMu.Lock()
			done := make(chan struct{})
			go func() {
				defer close(done)
				req := httptest.NewRequest(http.MethodPost, "/api/runs/pending-run/recovery/resolve", nil)
				res := httptest.NewRecorder()
				if action == "notification" {
					handleRunNotificationRetry(res, req, state, reg, "pending-run", 0)
				} else {
					handleRunRecovery(res, req, state, reg, "pending-run", action)
				}
			}()
			deadline := time.Now().Add(time.Second)
			for !state.hasActiveRun("pending-run") && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			claimed := state.hasActiveRun("pending-run")
			state.runFileMu.Unlock()
			<-done
			if !claimed {
				t.Fatal("读取状态前未领取运行，恢复动作可能使用过期记录")
			}
			if state.hasActiveRun("pending-run") {
				t.Fatal("请求结束后没有释放运行")
			}
		})
	}
}

func TestRecoveryReconcilesInterruptedAttemptAfterRestart(t *testing.T) {
	reg := testRegistry(t)
	writeRunFixture(t, reg.BaseDir, "pending-run", `{"id":"pending-run","kind":"workflow","status":"pending","pending":{"status":"pending"},"steps":[{"id":"health","status":"failed","error":"original failure"}],"attempts":[{"id":"attempt-1-rollback","kind":"rollback","status":"running","log_dir":"attempts/attempt-1-rollback"}]}`)
	state := newServerState(reg)
	state.reconcileOrphanRuns(reg)
	record, err := runbundle.LoadRecord(reg, "pending-run")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "pending" || record.Pending.Status != "pending" || record.Steps[0].Error != "original failure" {
		t.Fatalf("中断恢复修改了原始证据或待处理状态: %+v", record)
	}
	if len(record.Attempts) != 1 || record.Attempts[0].Status != "failed" || record.Attempts[0].Error == "" || record.Attempts[0].EndedAt.IsZero() {
		t.Fatalf("恢复尝试未记录中断结果: %+v", record.Attempts)
	}
}

func TestRecoveryClaimReconcilesAttemptBeforeAction(t *testing.T) {
	reg := testRegistry(t)
	writeRunFixture(t, reg.BaseDir, "pending-run", `{"id":"pending-run","kind":"workflow","status":"pending","pending":{"status":"pending"},"attempts":[{"id":"interrupted","kind":"check","status":"running"}]}`)
	state := newServerState(reg)
	req := httptest.NewRequest(http.MethodPost, "/api/runs/pending-run/recovery/resolve", nil)
	res := httptest.NewRecorder()
	handleRunRecovery(res, req, state, reg, "pending-run", "resolve")
	if res.Code != http.StatusOK {
		t.Fatalf("resolve status = %d, body = %s", res.Code, res.Body.String())
	}
	record, err := runbundle.LoadRecord(reg, "pending-run")
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Attempts) != 2 || record.Attempts[0].Status != "failed" || record.Attempts[1].Kind != "resolve" {
		t.Fatalf("领取后未保留中断证据: %+v", record.Attempts)
	}
}

func TestRecoveryRejectsDuplicateAndReloadsFinalState(t *testing.T) {
	reg := testRegistry(t)
	state := newServerState(reg)
	writeRunFixture(t, reg.BaseDir, "pending-run", `{"id":"pending-run","kind":"workflow","status":"pending","pending":{"status":"pending"}}`)
	_, cancel, ok := state.claimRun("pending-run")
	if !ok {
		t.Fatal("claim failed")
	}
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/runs/pending-run/recovery/resolve", nil)
	res := httptest.NewRecorder()
	handleRunRecovery(res, req, state, reg, "pending-run", "resolve")
	if res.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, body = %s", res.Code, res.Body.String())
	}
	state.finishRun("pending-run")
	res = httptest.NewRecorder()
	handleRunRecovery(res, req, state, reg, "pending-run", "resolve")
	if res.Code != http.StatusOK {
		t.Fatalf("resolve status = %d, body = %s", res.Code, res.Body.String())
	}
	res = httptest.NewRecorder()
	handleRunRecovery(res, req, state, reg, "pending-run", "resolve")
	if res.Code != http.StatusBadRequest {
		t.Fatalf("closed status = %d, body = %s", res.Code, res.Body.String())
	}
}

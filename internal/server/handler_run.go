package server

// 本文件是运行列表、取消、重跑、节点上传与 SSE 事件 handler。
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
	"shell_ops/internal/runbundle"
	"shell_ops/internal/runner"
)

func runsHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reg := state.registry()
		id := strings.TrimPrefix(req.URL.Path, "/api/runs/")
		if req.Method == http.MethodPost && strings.Trim(id, "/") == "cleanup" {
			handleRunsCleanup(w, req, state, reg)
			return
		}
		if req.Method == http.MethodPost {
			if runID, ok := strings.CutSuffix(strings.Trim(id, "/"), "/cancel"); ok {
				handleRunCancel(w, state, reg, strings.Trim(runID, "/"))
				return
			}
			if runID, nodeID, ok := parseRunNodeRerunPath(id); ok {
				handleRunNodeRerun(w, req, state, reg, strings.Trim(runID, "/"), strings.Trim(nodeID, "/"))
				return
			}
			if runID, nodeID, action, ok := parseRunUploadPath(id); ok {
				handleRunUploadNode(w, req, state, reg, strings.Trim(runID, "/"), strings.Trim(nodeID, "/"), action)
				return
			}
			if runID, action, ok := parseRunRecoveryPath(id); ok {
				handleRunRecovery(w, req, state, reg, strings.Trim(runID, "/"), action)
				return
			}
			if runID, index, ok := parseRunNotificationRetryPath(id); ok {
				handleRunNotificationRetry(w, req, state, reg, strings.Trim(runID, "/"), index)
				return
			}
			methodNotAllowed(w)
			return
		}
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if strings.Trim(id, "/") == "" {
			items, err := listRuns(state, reg)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
				return
			}
			query := req.URL.Query()
			status := strings.ToLower(strings.TrimSpace(query.Get("status")))
			keyword := strings.ToLower(strings.TrimSpace(query.Get("q")))
			filtered := make([]runbundle.Summary, 0, len(items))
			for _, item := range items {
				if status != "" && item.Status != status {
					continue
				}
				if keyword != "" && !runSummaryMatches(item, keyword) {
					continue
				}
				filtered = append(filtered, item)
			}
			total := len(filtered)
			limit := parseQueryInt(query.Get("limit"), 0)
			offset := parseQueryInt(query.Get("offset"), 0)
			if limit < 0 {
				limit = 0
			}
			if offset < 0 {
				offset = 0
			}
			if offset > total {
				offset = total
			}
			end := total
			if limit > 0 && offset+limit < end {
				end = offset + limit
			}
			writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{
				"runs":   filtered[offset:end],
				"total":  total,
				"limit":  limit,
				"offset": offset,
			}})
			return
		}
		if runID, ok := strings.CutSuffix(id, "/support.zip"); ok {
			handleRunSupportZip(w, reg, strings.Trim(runID, "/"))
			return
		}
		if runID, ok := strings.CutSuffix(id, "/events"); ok {
			handleRunEvents(w, req, state, reg, strings.Trim(runID, "/"))
			return
		}
		if isSummaryRunRequest(req) {
			record, err := loadRunRecord(state, reg, strings.Trim(id, "/"))
			if err != nil {
				writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, response{Data: map[string]interface{}{"record": record}})
			return
		}
		detail, err := loadRunDetail(state, reg, id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Data: detail})
	}
}

func parseRunNodeRerunPath(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 4 || parts[1] != "nodes" || parts[3] != "rerun" {
		return "", "", false
	}
	return parts[0], parts[2], true
}

func parseRunUploadPath(path string) (string, string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if (len(parts) != 3 && len(parts) != 4) || len(parts) >= 2 && parts[1] != "uploads" {
		return "", "", "", false
	}
	action := ""
	if len(parts) == 4 {
		action = parts[3]
	}
	return parts[0], parts[2], action, true
}

func parseRunRecoveryPath(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[1] != "recovery" {
		return "", "", false
	}
	switch parts[2] {
	case "check", "rollback", "resolve":
		return parts[0], parts[2], true
	default:
		return "", "", false
	}
}

func parseRunNotificationRetryPath(path string) (string, int, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 4 || parts[1] != "notifications" || parts[3] != "retry" {
		return "", 0, false
	}
	index, err := strconv.Atoi(parts[2])
	if err != nil || index < 0 {
		return "", 0, false
	}
	return parts[0], index, true
}

type runRecoveryResolveRequest struct {
	Note string `json:"note"`
}

// handleRunRecovery 领取一次性恢复动作，并始终回写最新记录和独立 attempt 证据。
func handleRunRecovery(w http.ResponseWriter, req *http.Request, state *serverState, reg *registry.Registry, runID, action string) {
	if strings.TrimSpace(runID) == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	ctx, cancel, ok := state.claimRun(runID)
	if !ok {
		writeJSON(w, http.StatusConflict, response{Error: "该运行已有恢复动作正在执行，请稍后刷新"})
		return
	}
	defer func() {
		state.finishRun(runID)
		cancel()
	}()
	// 领取后再读取和校验，避免并发请求在前一动作完成后使用旧的待处理记录。
	record, err := loadClaimedRunRecord(state, reg, runID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	if record.Status != runner.RunStatusPending || record.Pending == nil || record.Pending.Status != runner.PendingStatusPending {
		writeJSON(w, http.StatusBadRequest, response{Status: record.Status, Error: "运行不在待处理状态，无法执行恢复动作"})
		return
	}
	runDir, err := runbundle.RunDir(reg, runID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	r := state.newRunner(reg)
	var actionErr error
	switch action {
	case "check":
		actionErr = r.RetryPendingCheck(ctx, &record, runDir)
	case "rollback":
		actionErr = r.RollbackPendingRun(ctx, &record, runDir)
	case "resolve":
		var body runRecoveryResolveRequest
		if req.Body != nil {
			_ = json.NewDecoder(req.Body).Decode(&body)
		}
		actionErr = r.ResolvePendingRun(&record, body.Note)
	}
	if saveErr := saveRunRecord(reg, &record); saveErr != nil && actionErr == nil {
		actionErr = saveErr
	}
	writeReconciledRun(w, reg, runID, record, actionErr)
}

func handleRunNotificationRetry(w http.ResponseWriter, req *http.Request, state *serverState, reg *registry.Registry, runID string, index int) {
	if strings.TrimSpace(runID) == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	ctx, cancel, ok := state.claimRun(runID)
	if !ok {
		writeJSON(w, http.StatusConflict, response{Error: "该运行正在执行其他动作，请稍后刷新"})
		return
	}
	defer func() {
		state.finishRun(runID)
		cancel()
	}()
	record, err := loadClaimedRunRecord(state, reg, runID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	if index < 0 || index >= len(record.Notifications) {
		writeJSON(w, http.StatusNotFound, response{Error: "通知记录不存在"})
		return
	}
	r := state.newRunner(reg)
	actionErr := r.RetryNotification(ctx, &record, index)
	if saveErr := saveRunRecord(reg, &record); saveErr != nil && actionErr == nil {
		actionErr = saveErr
	}
	writeReconciledRun(w, reg, runID, record, actionErr)
}

func writeReconciledRun(w http.ResponseWriter, reg *registry.Registry, runID string, record runner.RunRecord, actionErr error) {
	detail, err := loadRunDetail(nil, reg, runID)
	if err != nil {
		detail = runDetail{Record: record}
	}
	status := http.StatusOK
	if actionErr != nil {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, response{ID: record.ID, Status: record.Status, Data: detail, Error: errorText(actionErr)})
}

func handleRunCancel(w http.ResponseWriter, state *serverState, reg *registry.Registry, id string) {
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	detail, err := loadRunDetail(state, reg, id)
	if err == nil && detail.Record.Status != "running" {
		writeJSON(w, http.StatusBadRequest, response{Status: detail.Record.Status, Data: detail, Error: "只能取消运行中的任务"})
		return
	}
	if state.hasActiveRun(id) {
		if !state.cancelRun(id) {
			writeJSON(w, http.StatusConflict, response{Error: "当前进程没有该运行任务的取消句柄，可能已结束或由其他进程启动"})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "cancelling", Data: map[string]string{"id": id, "status": "cancelling"}})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	if !state.cancelRun(id) {
		writeJSON(w, http.StatusConflict, response{Error: "当前进程没有该运行任务的取消句柄，可能已结束或由其他进程启动"})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "cancelling", Data: map[string]string{"id": id, "status": "cancelling"}})
}

func handleRunNodeRerun(w http.ResponseWriter, req *http.Request, state *serverState, reg *registry.Registry, runID, nodeID string) {
	if req.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(nodeID) == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	if state.hasActiveRun(runID) {
		writeJSON(w, http.StatusConflict, response{Error: "运行任务正在执行，不能重跑节点"})
		return
	}
	reqBody, err := decodeRunRequest(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	detail, err := loadRunDetail(state, reg, runID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	if detail.Record.Status == "running" {
		writeJSON(w, http.StatusBadRequest, response{Error: "运行中的工作流不能重跑节点"})
		return
	}
	workflowConfig := reqBody.Workflow
	if workflowConfig == nil {
		wf, err := reg.Workflow(detail.Record.Target)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		workflowConfig = wf.Config
	}
	if workflowConfig.ID != detail.Record.Target {
		writeJSON(w, http.StatusBadRequest, response{Error: "请求工作流与运行记录不一致"})
		return
	}
	if err := validateWorkflowTargets(state, workflowConfig); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	if workflowConfig.Confirm.Required && !reqBody.Confirm {
		writeJSON(w, http.StatusBadRequest, response{Error: "该工作流需要确认后执行"})
		return
	}
	if err := confirmWorkflowTools(reg, workflowConfig, reqBody.Confirm); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	rerunParams := config.ValuesToStringMap(config.MergeParamsValues(workflowConfig.Parameters, config.InterfaceMapToValues(detail.Record.Config), config.InterfaceMapToValues(reqBody.Params)))
	if len(detail.Record.Config) == 0 {
		rerunParams = config.ValuesToStringMap(config.MergeParamsValues(workflowConfig.Parameters, config.StringMapToValues(detail.Record.Params), config.InterfaceMapToValues(reqBody.Params)))
	}
	if err := config.ValidateRequired(workflowConfig.Parameters, rerunParams); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	runDir, err := runbundle.RunDir(reg, runID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	r := state.newRunner(reg)
	record := detail.Record
	ctx, cancel := context.WithCancel(context.Background())
	state.registerRun(runID, cancel)
	defer func() {
		state.finishRun(runID)
		cancel()
	}()
	updated, err := r.RerunWorkflowNodeWithParams(ctx, workflowConfig, &record, runDir, nodeID, rerunParams, reqBody.Confirm, io.Discard, io.Discard)
	if err != nil {
		writeRunResponse(w, updated, err)
		return
	}
	rerunDetail, err := loadRunDetail(state, reg, runID)
	if err != nil {
		writeJSON(w, http.StatusOK, response{ID: updated.ID, Status: updated.Status, Data: runDetail{Record: *updated}})
		return
	}
	writeJSON(w, http.StatusOK, response{ID: updated.ID, Status: updated.Status, Data: rerunDetail})
}

func handleRunUploadNode(w http.ResponseWriter, req *http.Request, state *serverState, reg *registry.Registry, runID, nodeID, action string) {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(nodeID) == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	if !state.hasActiveRun(runID) {
		writeJSON(w, http.StatusConflict, response{Error: "运行任务未处于活动状态，无法上传节点文件"})
		return
	}
	detail, err := loadRunDetail(state, reg, runID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	if detail.Record.Status != "running" {
		writeJSON(w, http.StatusBadRequest, response{Error: "只能向运行中的工作流上传节点文件"})
		return
	}
	step, ok := findRunStep(detail.Record, nodeID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, response{Error: "上传节点尚未进入等待状态"})
		return
	}
	if step.Type != config.WorkflowNodeTypeUpload {
		writeJSON(w, http.StatusBadRequest, response{Error: "目标步骤不是上传节点"})
		return
	}
	if step.Status != "waiting" {
		writeJSON(w, http.StatusBadRequest, response{Error: "上传节点当前不在等待上传状态"})
		return
	}
	if action != "" {
		handleRunUploadNodeChunked(w, req, reg, runID, nodeID, action)
		return
	}
	req.Body = http.MaxBytesReader(w, req.Body, maxPlatformFileUploadBytes+1)
	result, err := savePlatformUpload(reg, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	runDir, err := runbundle.RunDir(reg, runID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	if err := runner.WriteWorkflowUploadResult(runDir, nodeID, result); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("写入上传节点结果失败: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "uploaded", Data: result})
}

func handleRunUploadNodeChunked(w http.ResponseWriter, req *http.Request, reg *registry.Registry, runID, nodeID, action string) {
	switch action {
	case "start":
		var body chunkedUploadStartRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		session, err := startChunkedPlatformUpload(reg, body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "started", Data: map[string]interface{}{"id": session.ID, "chunk_size": maxPlatformUploadChunkBytes}})
	case "chunk":
		sessionID := strings.TrimSpace(req.URL.Query().Get("session_id"))
		fileIndex, err := strconv.Atoi(req.URL.Query().Get("file_index"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "上传文件索引无效"})
			return
		}
		offset, err := strconv.ParseInt(req.URL.Query().Get("offset"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "上传分片偏移无效"})
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, maxPlatformUploadChunkBytes+1)
		session, err := appendChunkedPlatformUpload(reg, sessionID, fileIndex, offset, req.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "chunked", Data: map[string]interface{}{"id": session.ID, "received_size": session.ReceivedSize, "received": session.Received}})
	case "finish":
		var body chunkedUploadFinishRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		result, err := finishChunkedPlatformUpload(reg, body.SessionID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		runDir, err := runbundle.RunDir(reg, runID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		if err := runner.WriteWorkflowUploadResult(runDir, nodeID, result); err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: fmt.Sprintf("写入上传节点结果失败: %v", err)})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "uploaded", Data: result})
	default:
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
	}
}

func findRunStep(record runner.RunRecord, nodeID string) (runner.StepRecord, bool) {
	for _, step := range record.Steps {
		if step.ID == nodeID {
			return step, true
		}
	}
	return runner.StepRecord{}, false
}

func handleRunsCleanup(w http.ResponseWriter, req *http.Request, state *serverState, reg *registry.Registry) {
	var body runbundle.CleanupOptions
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	state.runFileMu.Lock()
	defer state.runFileMu.Unlock()
	result, err := runbundle.Cleanup(reg, body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "ok", Data: result})
}

func handleRunEvents(w http.ResponseWriter, req *http.Request, state *serverState, reg *registry.Registry, id string) {
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	detail, err := loadRunDetail(state, reg, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	runDir, err := runbundle.RunDir(reg, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, response{Error: "streaming is not supported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	tailer := newRunLogTailer(id, runDir, detail.Record)
	seq := 0
	sendLogEvents := func() bool {
		events := tailer.read(detail.Record.Status)
		for _, event := range events {
			seq++
			event.Seq = seq
			if err := writeSSEEvent(w, "log", event); err != nil {
				return false
			}
		}
		flusher.Flush()
		return true
	}
	if !sendLogEvents() {
		return
	}
	if detail.Record.Status != "running" {
		_ = writeSSEEvent(w, "complete", map[string]string{"run_id": id, "status": detail.Record.Status})
		flusher.Flush()
		return
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-req.Context().Done():
			return
		case <-ticker.C:
			next, err := loadRunDetail(nil, reg, id)
			if err == nil {
				detail = next
				tailer.record = next.Record
			}
			if !sendLogEvents() {
				return
			}
			if err != nil || detail.Record.Status != "running" {
				status := detail.Record.Status
				if status == "" {
					status = "unknown"
				}
				_ = writeSSEEvent(w, "complete", map[string]string{"run_id": id, "status": status})
				flusher.Flush()
				return
			}
		}
	}
}

func writeSSEEvent(w io.Writer, event string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

func handleRunSupportZip(w http.ResponseWriter, reg *registry.Registry, id string) {
	data, err := runbundle.ExportZip(reg, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, runbundle.Filename(id)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

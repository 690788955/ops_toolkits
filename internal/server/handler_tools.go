package server

// 本文件是工具列表与批量执行 handler。
import (
	"context"
	"io"
	"net/http"
	"strings"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
	"shell_ops/internal/runner"
)

func toolsHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reg := state.registry()
		r := state.newRunner(reg)
		path := strings.TrimPrefix(req.URL.Path, "/api/tools/")
		if req.Method == http.MethodGet {
			id := strings.Trim(path, "/")
			if id == "" {
				writeJSON(w, http.StatusNotFound, response{Error: "not found"})
				return
			}
			tool, err := reg.Tool(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, response{Data: tool})
			return
		}
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		id, ok := strings.CutSuffix(path, "/run")
		if !ok || id == "" {
			writeJSON(w, http.StatusNotFound, response{Error: "not found"})
			return
		}
		reqBody, err := decodeRunRequest(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		tool, err := reg.Tool(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
			return
		}
		params := config.ValuesToStringMap(config.MergeParamsValues(tool.Config.Parameters, nil, config.InterfaceMapToValues(reqBody.Params)))
		if err := config.ValidateRequired(tool.Config.Parameters, params); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		target := strings.TrimSpace(reqBody.Target)
		targets := normalizeRunTargets(reqBody.Targets)
		if len(targets) == 0 {
			if err := validateRemoteTarget(state, target); err != nil {
				writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
				return
			}
		} else {
			for _, hostID := range targets {
				if err := validateRemoteTarget(state, hostID); err != nil {
					writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
					return
				}
			}
		}
		if tool.Config.Confirm.Required && !reqBody.Confirm {
			writeJSON(w, http.StatusBadRequest, response{Error: "该工具需要确认后执行"})
			return
		}
		if len(targets) > 0 {
			if reqBody.IncludeLocal {
				targets = append(targets, runner.LocalTargetID)
			}
			handleBatchToolRun(w, req, state, reg, r, id, targets, config.InterfaceMapToValues(reqBody.Params))
			return
		}
		if isAsyncRunRequest(req) {
			ctx, cancel := context.WithCancel(context.Background())
			record, err := r.StartToolValuesOn(ctx, id, target, config.InterfaceMapToValues(reqBody.Params), io.Discard, io.Discard)
			if err != nil {
				cancel()
				writeRunResponse(w, record, err)
				return
			}
			state.registerRun(record.ID, cancel)
			go watchRunCompletion(state, reg, record.ID, cancel)
			writeRunResponse(w, record, err)
			return
		}
		record, err := r.RunToolValuesOn(context.Background(), id, target, config.InterfaceMapToValues(reqBody.Params), io.Discard, io.Discard)
		writeRunResponse(w, record, err)
	}
}

func handleBatchToolRun(w http.ResponseWriter, req *http.Request, state *serverState, reg *registry.Registry, r *runner.Runner, id string, targets []string, params map[string]interface{}) {
	if isAsyncRunRequest(req) {
		batch, err := r.StartToolValuesOnTargets(context.Background(), id, targets, params, io.Discard, io.Discard)
		if err != nil {
			status := http.StatusInternalServerError
			body := response{Error: err.Error()}
			if batch != nil {
				body.Data = batchRunData(batch)
			}
			writeJSON(w, status, body)
			return
		}
		for _, run := range batch.Runs {
			cancel := batch.Cancels[run.ID]
			if cancel == nil {
				cancel = func() {}
			}
			state.registerRun(run.ID, cancel)
			go watchRunCompletion(state, reg, run.ID, cancel)
		}
		writeJSON(w, http.StatusOK, response{Status: "running", Data: batchRunData(batch)})
		return
	}
	batch, err := r.RunToolValuesOnTargets(context.Background(), id, targets, params, io.Discard, io.Discard)
	status := http.StatusOK
	if err != nil {
		status = http.StatusInternalServerError
	}
	if batch == nil {
		writeJSON(w, status, response{Error: errorText(err)})
		return
	}
	writeJSON(w, status, response{Status: "completed", Data: batchRunData(batch), Error: errorText(err)})
}

func batchRunData(batch *runner.BatchRun) map[string]interface{} {
	return map[string]interface{}{"batch_id": batch.BatchID, "runs": batch.Runs}
}

func normalizeRunTargets(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		hostID := strings.TrimSpace(item)
		if hostID == "" {
			continue
		}
		if _, ok := seen[hostID]; ok {
			continue
		}
		seen[hostID] = struct{}{}
		out = append(out, hostID)
	}
	return out
}

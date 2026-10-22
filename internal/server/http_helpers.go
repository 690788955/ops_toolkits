package server

// 本文件是通用的 HTTP / JSON 响应辅助函数。
import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"shell_ops/internal/runner"
)

func relativePath(baseDir, path string) string {
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func writeRunResponse(w http.ResponseWriter, record *runner.RunRecord, err error) {
	if record == nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: errorText(err)})
		return
	}
	status := http.StatusOK
	if err != nil {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, response{ID: record.ID, Status: record.Status, Error: errorText(err)})
}

func decodeRunRequest(req *http.Request) (*runRequest, error) {
	defer req.Body.Close()
	var body runRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Params == nil {
		body.Params = map[string]interface{}{}
	}
	return &body, nil
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, response{Error: "method not allowed"})
}

func writeJSON(w http.ResponseWriter, status int, body response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

const maxPluginConfigFileBytes int64 = 1024 * 1024

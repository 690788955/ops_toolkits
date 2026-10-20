package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"shell_ops/internal/sshmgr"
)

const sshRequestBodyLimit = 1 << 20

func sshAPIHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/api/ssh")
		if path == "" || path == "/" {
			writeJSON(w, http.StatusNotFound, response{Error: "not found"})
			return
		}
		switch {
		case path == "/hosts":
			handleSSHHosts(w, req, state)
		case path == "/settings":
			handleSSHSettings(w, req, state)
		case path == "/check":
			handleSSHCheckAll(w, req, state)
		case strings.HasPrefix(path, "/hosts/"):
			handleSSHHost(w, req, state, strings.TrimPrefix(path, "/hosts/"))
		default:
			writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		}
	}
}

func handleSSHHosts(w http.ResponseWriter, req *http.Request, state *serverState) {
	switch req.Method {
	case http.MethodGet:
		state.ssh.Start()
		overview, err := state.ssh.Overview()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Data: overview})
	case http.MethodPost:
		var input sshmgr.HostInput
		if err := decodeSSHBody(req, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "请求格式错误: " + err.Error()})
			return
		}
		view, err := state.ssh.Create(input)
		if err != nil {
			writeSSHError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "created", Data: view})
	default:
		methodNotAllowed(w)
	}
}

func handleSSHHost(w http.ResponseWriter, req *http.Request, state *serverState, rest string) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		switch req.Method {
		case http.MethodPut:
			var input sshmgr.HostInput
			if err := decodeSSHBody(req, &input); err != nil {
				writeJSON(w, http.StatusBadRequest, response{Error: "请求格式错误: " + err.Error()})
				return
			}
			view, err := state.ssh.Update(id, input)
			if err != nil {
				writeSSHError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response{Status: "updated", Data: view})
		case http.MethodDelete:
			if err := state.ssh.Delete(id); err != nil {
				writeSSHError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, response{Status: "deleted", ID: id})
		default:
			methodNotAllowed(w)
		}
		return
	}
	if len(parts) != 2 {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	switch parts[1] {
	case "check":
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		state.ssh.Start()
		view, err := state.ssh.Check(req.Context(), id)
		if err != nil {
			writeSSHError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response{Status: view.Status, Data: view})
	case "exec":
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var body struct {
			Command string `json:"command"`
		}
		if err := decodeSSHBody(req, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: "请求格式错误: " + err.Error()})
			return
		}
		if strings.TrimSpace(body.Command) == "" {
			writeJSON(w, http.StatusBadRequest, response{Error: "命令不能为空"})
			return
		}
		result, err := state.ssh.Exec(req.Context(), id, body.Command)
		if err != nil {
			if errors.Is(err, sshmgr.ErrHostNotFound) {
				writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusBadGateway, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "executed", Data: result})
	default:
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
	}
}

func handleSSHSettings(w http.ResponseWriter, req *http.Request, state *serverState) {
	if req.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}
	var input sshmgr.SettingsInput
	if err := decodeSSHBody(req, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "请求格式错误: " + err.Error()})
		return
	}
	if err := state.ssh.UpdateSettings(input); err != nil {
		writeSSHError(w, err)
		return
	}
	overview, err := state.ssh.Overview()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "saved", Data: overview})
}

func handleSSHCheckAll(w http.ResponseWriter, req *http.Request, state *serverState) {
	if req.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	state.ssh.Start()
	state.ssh.ScanNow()
	writeJSON(w, http.StatusOK, response{
		Status: "scheduled",
		Data:   map[string]string{"message": "已触发操作机巡检"},
	})
}

func decodeSSHBody(req *http.Request, target interface{}) error {
	defer req.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(req.Body, sshRequestBodyLimit))
	return decoder.Decode(target)
}

func writeSSHError(w http.ResponseWriter, err error) {
	if errors.Is(err, sshmgr.ErrHostNotFound) {
		writeJSON(w, http.StatusNotFound, response{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
}

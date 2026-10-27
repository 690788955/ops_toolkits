package server

// 本文件负责路由装配与本地 token 鉴权中间件。
import (
	"net/http"
	"strings"

	"shell_ops/internal/registry"
)

func ListenAndServe(addr string, reg *registry.Registry) error {
	return http.ListenAndServe(addr, NewHandler(reg))
}

func ListenAndServeWithToken(addr string, reg *registry.Registry, token string) error {
	return http.ListenAndServe(addr, tokenMiddleware(NewHandler(reg), token))
}

func NewHandler(reg *registry.Registry) http.Handler {
	state := newServerState(reg)
	state.startRunReconciler()
	mux := http.NewServeMux()
	registerWeb(mux)
	mux.HandleFunc("/api/catalog", catalogHandler(state))
	mux.HandleFunc("/api/ui/preferences", uiPreferencesHandler(state))
	mux.HandleFunc("/api/ssh/", sshAPIHandler(state))
	mux.HandleFunc("/api/config/global", globalConfigHandler(state))
	mux.HandleFunc("/api/config/global-env", globalEnvConfigHandler(state))
	mux.HandleFunc("/api/config/global/versions/", func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/api/config/global/versions/")
		if path == "" {
			// /api/config/global/versions
			handleConfigVersions(w, req, state, "global", "ops")
			return
		}
		parts := strings.Split(path, "/")
		if len(parts) == 2 && parts[1] == "default" {
			// /api/config/global/versions/{version}/default
			handleSetDefaultVersion(w, req, state, "global", "ops", parts[0])
			return
		}
		if len(parts) == 1 {
			// /api/config/global/versions/{version}
			handleConfigVersion(w, req, state, "global", "ops", parts[0])
			return
		}
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
	})
	mux.HandleFunc("/api/files/upload", fileUploadHandler(state))
	mux.HandleFunc("/api/dev/toolkit.zip", toolDevKitHandler())
	mux.HandleFunc("/api/plugins/user-workflows.zip", userWorkflowPluginExportHandler(state))
	mux.HandleFunc("/api/market", marketCatalogHandler(state))
	mux.HandleFunc("/api/market/install", marketInstallHandler(state))
	mux.HandleFunc("/api/market/sources", marketSourcesHandler(state))
	mux.HandleFunc("/api/market/sources/test", marketSourceTestHandler(state))
	mux.HandleFunc("/api/nexus/plugins", nexusPluginsHandler(state))
	mux.HandleFunc("/api/nexus/plugin", nexusDeleteHandler(state))
	mux.HandleFunc("/api/nexus/download", nexusDownloadHandler(state))
	mux.HandleFunc("/api/nexus/upload", nexusUploadHandler(state))
	mux.HandleFunc("/api/nexus/rebuild", nexusRebuildHandler(state))
	mux.HandleFunc("/api/nexus/share/targets", nexusShareTargetsHandler(state))
	mux.HandleFunc("/api/plugins/upload", pluginUploadHandler(state))
	mux.HandleFunc("/api/plugins/rescan", pluginRescanHandler(state))
	mux.HandleFunc("/api/plugins/", pluginDownloadHandler(state))
	mux.HandleFunc("/api/tools/", toolsHandler(state))
	mux.HandleFunc("/api/workflows/export", workflowExportHandler(state))
	mux.HandleFunc("/api/workflows/import", workflowImportHandler(state))
	mux.HandleFunc("/api/workflows/", workflowsHandler(state))
	mux.HandleFunc("/api/runs/", runsHandler(state))
	return mux
}

func tokenMiddleware(next http.Handler, token string) http.Handler {
	if strings.TrimSpace(token) == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if tokenAuthorized(req, token) {
			if req.URL.Query().Get("token") == token {
				http.SetCookie(w, &http.Cookie{Name: "ops_token", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
			}
			next.ServeHTTP(w, req)
			return
		}
		if req.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("缺少或无效的本地访问 token，请使用启动命令输出的 Web UI 地址访问。"))
			return
		}
		writeJSON(w, http.StatusUnauthorized, response{Error: "unauthorized"})
	})
}

func tokenAuthorized(req *http.Request, token string) bool {
	if req.URL.Query().Get("token") == token {
		return true
	}
	if req.Header.Get("X-Ops-Token") == token {
		return true
	}
	cookie, err := req.Cookie("ops_token")
	return err == nil && cookie.Value == token
}

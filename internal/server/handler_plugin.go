package server

// 本文件是开发包、用户工作流导出与插件下载 handler。
import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func toolDevKitHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		data, err := buildToolDevKitZip()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, response{Error: err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="ops-plugin-template.zip"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func userWorkflowPluginExportHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		query := req.URL.Query()
		includeDependencies := query.Get("include_dependencies") == "1" || query.Get("include_dependencies") == "true"
		options := userWorkflowExportOptions{WorkflowIDs: query["workflow"], IncludeDependencies: includeDependencies}
		data, err := buildUserWorkflowExportZip(state.registry(), options)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		filename := "user.workflows.zip"
		if includeDependencies {
			filename = "user-workflows-bundle.zip"
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func pluginDownloadHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		name := strings.TrimPrefix(req.URL.Path, "/api/plugins/")

		// 配置文件路由：/api/plugins/{id}/files
		if strings.HasSuffix(name, "/files") || strings.Contains(name, "/files/") {
			parts := strings.Split(strings.Trim(name, "/"), "/")
			if len(parts) >= 2 && parts[1] == "files" {
				pluginID := parts[0]
				if len(parts) == 2 {
					// /api/plugins/{id}/files - 列出配置文件
					handlePluginConfigFiles(w, req, state, pluginID)
				} else {
					// /api/plugins/{id}/files/{fileID} - 读取/保存/删除配置文件；legacy 插件内路径允许编码后的斜杠，但必须命中声明。
					fileID, err := url.PathUnescape(strings.Join(parts[2:], "/"))
					if err != nil {
						writeJSON(w, http.StatusBadRequest, response{Error: "配置文件 ID 格式错误"})
						return
					}
					handlePluginConfigFile(w, req, state, pluginID, fileID)
				}
				return
			}
		}

		if pluginID, ok := strings.CutSuffix(name, "/disable"); ok {
			handlePluginDisable(w, req, state, pluginID)
			return
		}
		if pluginID, ok := strings.CutSuffix(name, "/enable"); ok {
			handlePluginEnable(w, req, state, pluginID)
			return
		}
		// 分享插件到远程插件市场（Nexus）
		if pluginID, ok := strings.CutSuffix(name, "/share"); ok {
			handlePluginShare(w, req, state, pluginID)
			return
		}
		// 版本管理路由已移除，业务配置已废弃
		if req.Method == http.MethodDelete {
			handlePluginDelete(w, req, state, strings.Trim(name, "/"))
			return
		}
		if req.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if strings.HasSuffix(name, "/runtime.zip") || strings.HasSuffix(name, "/runtime.tar.gz") {
			handlePluginRuntimeDownload(w, req, state, name)
			return
		}
		pluginID, ok := strings.CutSuffix(name, ".zip")
		if !ok || strings.TrimSpace(pluginID) == "" {
			writeJSON(w, http.StatusNotFound, response{Error: "not found"})
			return
		}
		data, err := buildPluginExportZip(state.registry(), pluginID)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errPluginNotFound) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, response{Error: err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, pluginID))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func handlePluginRuntimeDownload(w http.ResponseWriter, req *http.Request, state *serverState, name string) {
	format := "zip"
	pluginID, ok := strings.CutSuffix(name, "/runtime.zip")
	if !ok {
		format = "tar.gz"
		pluginID, ok = strings.CutSuffix(name, "/runtime.tar.gz")
	}
	if !ok || strings.TrimSpace(pluginID) == "" {
		writeJSON(w, http.StatusNotFound, response{Error: "not found"})
		return
	}
	goos := strings.TrimSpace(req.URL.Query().Get("goos"))
	goarch := strings.TrimSpace(req.URL.Query().Get("goarch"))
	data, err := buildPluginRuntimePackage(state.registry(), strings.Trim(pluginID, "/"), goos, goarch, format)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errPluginNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, response{Error: err.Error()})
		return
	}
	contentType := "application/zip"
	fileName := fmt.Sprintf("%s-opsctl-%s-%s.zip", pluginID, goos, goarch)
	if format == "tar.gz" {
		contentType = "application/gzip"
		fileName = fmt.Sprintf("%s-opsctl-%s-%s.tar.gz", pluginID, goos, goarch)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

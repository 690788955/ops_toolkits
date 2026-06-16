package server

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"shell_ops/internal/config"
	"shell_ops/internal/registry"
)

const maxPlatformFileUploadBytes int64 = 20 * 1024 * 1024 * 1024
const maxPlatformUploadChunkBytes int64 = 64 * 1024 * 1024

type chunkedUploadSession struct {
	ID           string                     `json:"id"`
	UploadDir    string                     `json:"upload_dir"`
	BaseDir      string                     `json:"base_dir"`
	Files        []chunkedUploadSessionFile `json:"files"`
	ReceivedSize int64                      `json:"received_size"`
	Received     map[int]int64              `json:"received"`
}

type chunkedUploadSessionFile struct {
	Index        int    `json:"index"`
	FileName     string `json:"file_name"`
	RelativePath string `json:"relative_path"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	Received     int64  `json:"received"`
}

type chunkedUploadStartRequest struct {
	TargetDir string                   `json:"target_dir"`
	Files     []chunkedUploadStartFile `json:"files"`
}

type chunkedUploadStartFile struct {
	Name         string `json:"name"`
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
}

type chunkedUploadFinishRequest struct {
	SessionID string `json:"session_id"`
}

func fileUploadHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, maxPlatformFileUploadBytes+1)
		result, err := savePlatformUpload(state.registry(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "uploaded", Data: result})
	}
}

func savePlatformUpload(reg *registry.Registry, req *http.Request) (config.WorkflowUploadResult, error) {
	if reg == nil {
		return config.WorkflowUploadResult{}, fmt.Errorf("运行注册表未初始化")
	}
	runsPath := ""
	if reg != nil && reg.Root != nil {
		runsPath = reg.Root.Paths.Runs
	}
	return savePlatformUploadFromMultipart(reg.BaseDir, runsPath, req)
}

func platformRunsPath(reg *registry.Registry) string {
	if reg == nil || reg.Root == nil || strings.TrimSpace(reg.Root.Paths.Runs) == "" {
		return "runs"
	}
	return reg.Root.Paths.Runs
}

func savePlatformUploadFromMultipart(baseDir, runsPath string, req *http.Request) (config.WorkflowUploadResult, error) {
	if err := req.ParseMultipartForm(32 << 20); err != nil {
		return config.WorkflowUploadResult{}, fmt.Errorf("读取上传文件失败: %w", err)
	}
	files, err := platformMultipartFiles(req.MultipartForm)
	if err != nil {
		return config.WorkflowUploadResult{}, err
	}
	targetDir, err := config.NormalizeUploadTargetDir(req.FormValue("target_dir"))
	if err != nil {
		return config.WorkflowUploadResult{}, err
	}
	uploadID := fmt.Sprintf("upload-%d", time.Now().UnixNano())
	if strings.TrimSpace(runsPath) == "" {
		runsPath = "runs"
	}
	uploadParts := []string{baseDir, filepath.FromSlash(runsPath), "uploads"}
	if targetDir != "" {
		uploadParts = append(uploadParts, filepath.FromSlash(targetDir))
	}
	uploadParts = append(uploadParts, uploadID)
	uploadDir := filepath.Join(uploadParts...)
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return config.WorkflowUploadResult{}, fmt.Errorf("创建上传目录失败: %w", err)
	}
	relativePaths := req.MultipartForm.Value["relative_path"]
	uploadedFiles := make([]config.WorkflowUploadFile, 0, len(files))
	var totalSize int64
	for index, header := range files {
		item, err := savePlatformUploadFile(baseDir, uploadDir, header, relativePathAt(relativePaths, index))
		if err != nil {
			_ = os.RemoveAll(uploadDir)
			return config.WorkflowUploadResult{}, err
		}
		uploadedFiles = append(uploadedFiles, item)
		totalSize += item.Size
		if totalSize > maxPlatformFileUploadBytes {
			_ = os.RemoveAll(uploadDir)
			return config.WorkflowUploadResult{}, fmt.Errorf("上传文件总大小超过 %s", formatUploadSizeLimit(maxPlatformFileUploadBytes))
		}
	}
	first := uploadedFiles[0]
	return config.WorkflowUploadResult{
		ID:           uploadID,
		FileName:     first.FileName,
		Path:         first.Path,
		RelativePath: first.RelativePath,
		Size:         first.Size,
		Files:        uploadedFiles,
		Count:        len(uploadedFiles),
		TotalSize:    totalSize,
	}, nil
}

func startChunkedPlatformUpload(reg *registry.Registry, req chunkedUploadStartRequest) (chunkedUploadSession, error) {
	if reg == nil {
		return chunkedUploadSession{}, fmt.Errorf("运行注册表未初始化")
	}
	targetDir, err := config.NormalizeUploadTargetDir(req.TargetDir)
	if err != nil {
		return chunkedUploadSession{}, err
	}
	if len(req.Files) == 0 {
		return chunkedUploadSession{}, fmt.Errorf("缺少上传文件")
	}
	uploadID := fmt.Sprintf("upload-%d", time.Now().UnixNano())
	uploadParts := []string{reg.BaseDir, filepath.FromSlash(platformRunsPath(reg)), "uploads"}
	if targetDir != "" {
		uploadParts = append(uploadParts, filepath.FromSlash(targetDir))
	}
	uploadParts = append(uploadParts, uploadID)
	uploadDir := filepath.Join(uploadParts...)
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return chunkedUploadSession{}, fmt.Errorf("创建上传目录失败: %w", err)
	}
	session := chunkedUploadSession{
		ID:        uploadID,
		UploadDir: uploadDir,
		BaseDir:   reg.BaseDir,
		Received:  map[int]int64{},
	}
	var totalSize int64
	for index, file := range req.Files {
		if file.Size < 0 {
			_ = os.RemoveAll(uploadDir)
			return chunkedUploadSession{}, fmt.Errorf("上传文件大小无效")
		}
		totalSize += file.Size
		if totalSize > maxPlatformFileUploadBytes {
			_ = os.RemoveAll(uploadDir)
			return chunkedUploadSession{}, fmt.Errorf("上传文件总大小超过 %s", formatUploadSizeLimit(maxPlatformFileUploadBytes))
		}
		cleanRelativePath, err := sanitizeUploadedRelativePath(file.RelativePath, file.Name)
		if err != nil {
			_ = os.RemoveAll(uploadDir)
			return chunkedUploadSession{}, err
		}
		targetPath := filepath.Join(uploadDir, filepath.FromSlash(cleanRelativePath))
		if !pathWithin(uploadDir, targetPath) {
			_ = os.RemoveAll(uploadDir)
			return chunkedUploadSession{}, fmt.Errorf("上传文件路径不能逃逸上传目录")
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			_ = os.RemoveAll(uploadDir)
			return chunkedUploadSession{}, fmt.Errorf("创建上传子目录失败: %w", err)
		}
		if err := os.WriteFile(targetPath, nil, 0o644); err != nil {
			_ = os.RemoveAll(uploadDir)
			return chunkedUploadSession{}, fmt.Errorf("创建上传文件失败: %w", err)
		}
		session.Files = append(session.Files, chunkedUploadSessionFile{
			Index:        index,
			FileName:     filepath.Base(cleanRelativePath),
			RelativePath: cleanRelativePath,
			Path:         targetPath,
			Size:         file.Size,
		})
	}
	if err := writeChunkedUploadSession(session); err != nil {
		_ = os.RemoveAll(uploadDir)
		return chunkedUploadSession{}, err
	}
	return session, nil
}

func appendChunkedPlatformUpload(reg *registry.Registry, sessionID string, fileIndex int, offset int64, chunk io.Reader) (chunkedUploadSession, error) {
	session, err := readChunkedUploadSession(reg, sessionID)
	if err != nil {
		return chunkedUploadSession{}, err
	}
	if fileIndex < 0 || fileIndex >= len(session.Files) {
		return chunkedUploadSession{}, fmt.Errorf("上传文件索引无效")
	}
	file := session.Files[fileIndex]
	if offset != file.Received {
		return chunkedUploadSession{}, fmt.Errorf("上传分片偏移不匹配，期望 %d，收到 %d", file.Received, offset)
	}
	if maxRemain := file.Size - file.Received; maxRemain < 0 {
		return chunkedUploadSession{}, fmt.Errorf("上传文件大小状态无效")
	}
	in := io.LimitReader(chunk, maxPlatformUploadChunkBytes+1)
	out, err := os.OpenFile(file.Path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return chunkedUploadSession{}, fmt.Errorf("打开上传文件失败: %w", err)
	}
	written, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return chunkedUploadSession{}, fmt.Errorf("保存上传分片失败: %w", copyErr)
	}
	if closeErr != nil {
		return chunkedUploadSession{}, fmt.Errorf("关闭上传文件失败: %w", closeErr)
	}
	if written > maxPlatformUploadChunkBytes {
		return chunkedUploadSession{}, fmt.Errorf("上传分片超过 %s", formatUploadSizeLimit(maxPlatformUploadChunkBytes))
	}
	if file.Received+written > file.Size {
		return chunkedUploadSession{}, fmt.Errorf("上传文件超过声明大小")
	}
	file.Received += written
	session.Files[fileIndex] = file
	session.ReceivedSize += written
	if session.Received == nil {
		session.Received = map[int]int64{}
	}
	session.Received[fileIndex] = file.Received
	if err := writeChunkedUploadSession(session); err != nil {
		return chunkedUploadSession{}, err
	}
	return session, nil
}

func finishChunkedPlatformUpload(reg *registry.Registry, sessionID string) (config.WorkflowUploadResult, error) {
	session, err := readChunkedUploadSession(reg, sessionID)
	if err != nil {
		return config.WorkflowUploadResult{}, err
	}
	uploadedFiles := make([]config.WorkflowUploadFile, 0, len(session.Files))
	var totalSize int64
	for _, file := range session.Files {
		if file.Received != file.Size {
			return config.WorkflowUploadResult{}, fmt.Errorf("上传文件 %s 尚未完成", file.FileName)
		}
		info, err := os.Stat(file.Path)
		if err != nil {
			return config.WorkflowUploadResult{}, fmt.Errorf("读取上传文件失败: %w", err)
		}
		if info.Size() != file.Size {
			return config.WorkflowUploadResult{}, fmt.Errorf("上传文件 %s 大小不匹配", file.FileName)
		}
		rel, err := filepath.Rel(session.BaseDir, file.Path)
		if err != nil {
			rel = file.Path
		}
		uploadedFiles = append(uploadedFiles, config.WorkflowUploadFile{
			FileName:     file.FileName,
			Path:         file.Path,
			RelativePath: filepath.ToSlash(rel),
			Size:         file.Size,
		})
		totalSize += file.Size
	}
	if len(uploadedFiles) == 0 {
		return config.WorkflowUploadResult{}, fmt.Errorf("缺少上传文件")
	}
	first := uploadedFiles[0]
	return config.WorkflowUploadResult{
		ID:           session.ID,
		FileName:     first.FileName,
		Path:         first.Path,
		RelativePath: first.RelativePath,
		Size:         first.Size,
		Files:        uploadedFiles,
		Count:        len(uploadedFiles),
		TotalSize:    totalSize,
	}, nil
}

func chunkedUploadSessionPath(uploadDir string) string {
	return filepath.Join(uploadDir, ".upload-session.json")
}

func writeChunkedUploadSession(session chunkedUploadSession) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	path := chunkedUploadSessionPath(session.UploadDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readChunkedUploadSession(reg *registry.Registry, sessionID string) (chunkedUploadSession, error) {
	if reg == nil {
		return chunkedUploadSession{}, fmt.Errorf("运行注册表未初始化")
	}
	if !safeUploadID(sessionID) {
		return chunkedUploadSession{}, fmt.Errorf("上传会话无效")
	}
	root := filepath.Join(reg.BaseDir, filepath.FromSlash(platformRunsPath(reg)), "uploads")
	var found string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return err
		}
		if !d.IsDir() || d.Name() != sessionID {
			return nil
		}
		if !pathWithin(root, path) {
			return fmt.Errorf("上传会话路径无效")
		}
		found = path
		return filepath.SkipDir
	})
	if err != nil {
		return chunkedUploadSession{}, err
	}
	if found == "" {
		return chunkedUploadSession{}, fmt.Errorf("上传会话不存在")
	}
	data, err := os.ReadFile(chunkedUploadSessionPath(found))
	if err != nil {
		return chunkedUploadSession{}, err
	}
	var session chunkedUploadSession
	if err := json.Unmarshal(data, &session); err != nil {
		return chunkedUploadSession{}, err
	}
	if !pathWithin(root, session.UploadDir) {
		return chunkedUploadSession{}, fmt.Errorf("上传会话路径无效")
	}
	return session, nil
}

func safeUploadID(id string) bool {
	id = strings.TrimSpace(id)
	return strings.HasPrefix(id, "upload-") && !strings.ContainsAny(id, `/\`) && !strings.Contains(id, "\x00")
}

func savePlatformUploadFile(baseDir, uploadDir string, header *multipart.FileHeader, relativePath string) (config.WorkflowUploadFile, error) {
	if header.Size > maxPlatformFileUploadBytes {
		return config.WorkflowUploadFile{}, fmt.Errorf("上传文件超过 %s", formatUploadSizeLimit(maxPlatformFileUploadBytes))
	}
	file, err := header.Open()
	if err != nil {
		return config.WorkflowUploadFile{}, err
	}
	defer file.Close()
	cleanRelativePath, err := sanitizeUploadedRelativePath(relativePath, header.Filename)
	if err != nil {
		return config.WorkflowUploadFile{}, err
	}
	targetPath := filepath.Join(uploadDir, filepath.FromSlash(cleanRelativePath))
	if !pathWithin(uploadDir, targetPath) {
		return config.WorkflowUploadFile{}, fmt.Errorf("上传文件路径不能逃逸上传目录")
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return config.WorkflowUploadFile{}, fmt.Errorf("创建上传子目录失败: %w", err)
	}
	out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return config.WorkflowUploadFile{}, fmt.Errorf("创建上传文件失败: %w", err)
	}
	written, copyErr := io.Copy(out, io.LimitReader(file, maxPlatformFileUploadBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(targetPath)
		return config.WorkflowUploadFile{}, fmt.Errorf("保存上传文件失败: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(targetPath)
		return config.WorkflowUploadFile{}, fmt.Errorf("关闭上传文件失败: %w", closeErr)
	}
	if written > maxPlatformFileUploadBytes {
		_ = os.Remove(targetPath)
		return config.WorkflowUploadFile{}, fmt.Errorf("上传文件超过 %s", formatUploadSizeLimit(maxPlatformFileUploadBytes))
	}
	rel, err := filepath.Rel(baseDir, targetPath)
	if err != nil {
		rel = targetPath
	}
	return config.WorkflowUploadFile{
		FileName:     filepath.Base(cleanRelativePath),
		Path:         targetPath,
		RelativePath: filepath.ToSlash(rel),
		Size:         written,
	}, nil
}

func formatUploadSizeLimit(size int64) string {
	const gb int64 = 1024 * 1024 * 1024
	if size%gb == 0 {
		return fmt.Sprintf("%dGB", size/gb)
	}
	return fmt.Sprintf("%d bytes", size)
}

func platformMultipartFiles(form *multipart.Form) ([]*multipart.FileHeader, error) {
	if form == nil || form.File == nil {
		return nil, fmt.Errorf("缺少上传文件")
	}
	files := form.File["file"]
	if len(files) == 0 {
		return nil, fmt.Errorf("缺少上传文件")
	}
	return files, nil
}

func sanitizeUploadedFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Trim(name, ". ")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", "\x00", "_")
	return replacer.Replace(name)
}

func sanitizeUploadedRelativePath(relativePath, fallbackName string) (string, error) {
	relativePath = strings.TrimSpace(strings.ReplaceAll(relativePath, "\\", "/"))
	if relativePath == "" {
		fileName := sanitizeUploadedFileName(fallbackName)
		if fileName == "" {
			return "", fmt.Errorf("上传文件名无效")
		}
		return fileName, nil
	}
	if strings.Contains(relativePath, "://") || strings.HasPrefix(relativePath, "/") || filepath.IsAbs(relativePath) || len(relativePath) >= 2 && relativePath[1] == ':' {
		return "", fmt.Errorf("上传相对路径无效")
	}
	parts := strings.Split(relativePath, "/")
	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := sanitizeUploadedFileName(part)
		if clean == "" || clean == "." || clean == ".." {
			return "", fmt.Errorf("上传相对路径不能包含空路径、. 或 ..")
		}
		cleanParts = append(cleanParts, clean)
	}
	return strings.Join(cleanParts, "/"), nil
}

func relativePathAt(paths []string, index int) string {
	if index < 0 || index >= len(paths) {
		return ""
	}
	return paths[index]
}

func pathWithin(baseDir, path string) bool {
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".."
}

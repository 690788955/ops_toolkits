package runbundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"shell_ops/internal/config"
	"shell_ops/internal/doctor"
	"shell_ops/internal/registry"
	"shell_ops/internal/runner"
)

type Summary struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Target    string            `json:"target"`
	Status    string            `json:"status"`
	StartedAt time.Time         `json:"started_at"`
	EndedAt   time.Time         `json:"ended_at"`
	Params    map[string]string `json:"params,omitempty"`
	Error     string            `json:"error,omitempty"`
}

type CleanupOptions struct {
	Keep   int  `json:"keep"`
	DryRun bool `json:"dry_run"`
}

type CleanupResult struct {
	Keep       int      `json:"keep"`
	DryRun     bool     `json:"dry_run"`
	Total      int      `json:"total"`
	Deleted    int      `json:"deleted"`
	DeletedIDs []string `json:"deleted_ids"`
	KeptIDs    []string `json:"kept_ids"`
}

func List(reg *registry.Registry) ([]Summary, error) {
	runsDir := runsDir(reg)
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Summary{}, nil
		}
		return nil, err
	}
	out := []Summary{}
	for _, entry := range entries {
		if !entry.IsDir() || !safeRunID(entry.Name()) {
			continue
		}
		record, err := LoadRecord(reg, entry.Name())
		if err != nil {
			continue
		}
		out = append(out, summaryFromRecord(record))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, nil
}

func Cleanup(reg *registry.Registry, opts CleanupOptions) (CleanupResult, error) {
	if opts.Keep < 0 {
		return CleanupResult{}, fmt.Errorf("保留数量不能小于 0")
	}
	items, err := List(reg)
	if err != nil {
		return CleanupResult{}, err
	}
	result := CleanupResult{
		Keep:   opts.Keep,
		DryRun: opts.DryRun,
		Total:  len(items),
	}
	for index, item := range items {
		if index < opts.Keep {
			result.KeptIDs = append(result.KeptIDs, item.ID)
			continue
		}
		result.DeletedIDs = append(result.DeletedIDs, item.ID)
		if opts.DryRun {
			continue
		}
		if err := os.RemoveAll(filepath.Join(runsDir(reg), item.ID)); err != nil {
			return result, err
		}
		result.Deleted++
	}
	if opts.DryRun {
		result.Deleted = len(result.DeletedIDs)
	}
	return result, nil
}

func LoadRecord(reg *registry.Registry, id string) (runner.RunRecord, error) {
	if !safeRunID(id) {
		return runner.RunRecord{}, os.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(runsDir(reg), id, "result.json"))
	if err != nil {
		return runner.RunRecord{}, err
	}
	var record runner.RunRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return runner.RunRecord{}, err
	}
	return record, nil
}

func ExportZip(reg *registry.Registry, id string) ([]byte, error) {
	if !safeRunID(id) {
		return nil, os.ErrNotExist
	}
	runDir := filepath.Join(runsDir(reg), id)
	info, err := os.Stat(runDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("运行记录不是目录: %s", id)
	}
	record, err := LoadRecord(reg, id)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := addSummary(zw, reg, record); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := addSupportReport(zw, reg, record, runDir); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := addDoctorReport(zw, reg); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := filepath.WalkDir(runDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("不支持导出特殊文件: %s", path)
		}
		rel, err := filepath.Rel(runDir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join("run", rel))
		if unsafeArchiveName(name) {
			return fmt.Errorf("运行记录文件路径不安全: %s", rel)
		}
		if filepath.ToSlash(rel) == "result.json" {
			return addZipFile(zw, name, redactedRecordJSON(record), info.Mode())
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = name
		header.Method = zip.Deflate
		file, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(file, in)
		return err
	}); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Filename(id string) string {
	if !safeRunID(id) {
		return "ops-support.zip"
	}
	return id + "-support.zip"
}

func RunDir(reg *registry.Registry, id string) (string, error) {
	if !safeRunID(id) {
		return "", os.ErrNotExist
	}
	return filepath.Join(runsDir(reg), id), nil
}

func summaryFromRecord(record runner.RunRecord) Summary {
	return Summary{
		ID:        record.ID,
		Kind:      record.Kind,
		Target:    record.Target,
		Status:    record.Status,
		StartedAt: record.StartedAt,
		EndedAt:   record.EndedAt,
		Params:    redactFlatParams(record.Params),
		Error:     record.Error,
	}
}

func addSummary(zw *zip.Writer, reg *registry.Registry, record runner.RunRecord) error {
	doctorReport := doctor.Run(reg)
	body := map[string]interface{}{
		"app": map[string]interface{}{
			"name":    reg.Root.DisplayName(),
			"version": reg.Root.App.Version,
		},
		"base_dir":       relativePath(reg.BaseDir, reg.BaseDir),
		"run":            summaryFromRecord(record),
		"doctor_summary": summarizeDoctor(doctorReport),
		"warnings":       reg.Warnings,
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	return addZipFile(zw, "support-summary.json", data, 0o644)
}

func addSupportReport(zw *zip.Writer, reg *registry.Registry, record runner.RunRecord, runDir string) error {
	report := supportMarkdown(reg, record, runDir, doctor.Run(reg))
	return addZipFile(zw, "support-report.md", []byte(report), 0o644)
}

func addDoctorReport(zw *zip.Writer, reg *registry.Registry) error {
	data, err := json.MarshalIndent(doctor.Run(reg), "", "  ")
	if err != nil {
		return err
	}
	return addZipFile(zw, "doctor-report.json", data, 0o644)
}

func supportMarkdown(reg *registry.Registry, record runner.RunRecord, runDir string, doctorReport doctor.Report) string {
	var b strings.Builder
	b.WriteString("# 运行支持报告\n\n")
	b.WriteString("## 基本信息\n\n")
	fmt.Fprintf(&b, "- 应用: %s %s\n", reg.Root.DisplayName(), reg.Root.App.Version)
	fmt.Fprintf(&b, "- 运行 ID: %s\n", record.ID)
	fmt.Fprintf(&b, "- 类型: %s\n", record.Kind)
	fmt.Fprintf(&b, "- 目标: %s\n", record.Target)
	fmt.Fprintf(&b, "- 状态: %s\n", record.Status)
	if !record.StartedAt.IsZero() {
		fmt.Fprintf(&b, "- 开始时间: %s\n", record.StartedAt.Format(time.RFC3339))
	}
	if !record.EndedAt.IsZero() {
		fmt.Fprintf(&b, "- 结束时间: %s\n", record.EndedAt.Format(time.RFC3339))
	}
	if record.Error != "" {
		fmt.Fprintf(&b, "- 错误: %s\n", record.Error)
	}
	b.WriteString("\n## 参数摘要\n\n")
	params := redactFlatParams(record.Params)
	if len(params) == 0 {
		b.WriteString("- 无参数记录\n")
	} else {
		for _, key := range sortedParamKeys(params) {
			fmt.Fprintf(&b, "- `%s`: `%s`\n", key, params[key])
		}
	}
	b.WriteString("\n## 诊断摘要\n\n")
	for _, check := range doctorReport.Checks {
		fmt.Fprintf(&b, "- %s / %s: %s\n", check.Status, check.Name, check.Message)
	}
	b.WriteString("\n## 日志尾部\n\n")
	writeLogTail(&b, "stdout.log", filepath.Join(runDir, "stdout.log"))
	writeLogTail(&b, "stderr.log", filepath.Join(runDir, "stderr.log"))
	if len(record.Steps) > 0 {
		b.WriteString("\n## 工作流步骤\n\n")
		for _, step := range record.Steps {
			fmt.Fprintf(&b, "- %s [%s] %s", step.ID, step.Type, step.Status)
			if step.Tool != "" {
				fmt.Fprintf(&b, " tool=%s", step.Tool)
			}
			if step.Error != "" {
				fmt.Fprintf(&b, " error=%s", step.Error)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func writeLogTail(b *strings.Builder, title, path string) {
	b.WriteString("### " + title + "\n\n")
	lines := tailLines(readText(path), 80)
	if strings.TrimSpace(lines) == "" {
		b.WriteString("无日志内容。\n\n")
		return
	}
	b.WriteString("```text\n")
	b.WriteString(lines)
	if !strings.HasSuffix(lines, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```\n\n")
}

func summarizeDoctor(report doctor.Report) map[string]int {
	out := map[string]int{"ok": 0, "warning": 0, "failed": 0}
	for _, check := range report.Checks {
		out[check.Status]++
	}
	return out
}

func redactedRecordJSON(record runner.RunRecord) []byte {
	record.Params = redactFlatParams(record.Params)
	record.Config = config.RedactSensitive(record.Config, nil)
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return data
}

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func tailLines(text string, limit int) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= limit {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-limit:], "\n")
}

func sortedParamKeys(params map[string]string) []string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func addZipFile(zw *zip.Writer, name string, data []byte, mode os.FileMode) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o644)
	if mode != 0 {
		header.SetMode(mode)
	}
	file, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	return err
}

func runsDir(reg *registry.Registry) string {
	return filepath.Join(reg.BaseDir, filepath.FromSlash(reg.Root.Paths.Logs))
}

func safeRunID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || id == "." || id == ".." {
		return false
	}
	return !strings.ContainsAny(id, `/\`) && !strings.Contains(id, "\x00")
}

func unsafeArchiveName(name string) bool {
	if strings.TrimSpace(name) == "" || filepath.IsAbs(name) || strings.Contains(name, "\\") {
		return true
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	return clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator))
}

func redactFlatParams(params map[string]string) map[string]string {
	out := make(map[string]string, len(params))
	for key, value := range params {
		if config.IsSensitivePath(key) {
			out[key] = "******"
		} else {
			out[key] = value
		}
	}
	return out
}

func relativePath(baseDir, path string) string {
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

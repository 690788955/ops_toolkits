package market

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"shell_ops/internal/config"
	"shell_ops/internal/plugin"
)

var fixedZipTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

const userWorkflowPluginID = "user.workflows"

type BuildOptions struct {
	BaseDir   string
	Plugins   config.PluginsConfig
	OutputDir string
	Name      string
}

type BuildResult struct {
	IndexPath   string
	PackagesDir string
	Plugins     int
	Warnings    []plugin.Warning
}

func BuildIndex(opts BuildOptions) (BuildResult, error) {
	baseDir := strings.TrimSpace(opts.BaseDir)
	if baseDir == "" {
		return BuildResult{}, fmt.Errorf("项目根目录不能为空")
	}
	pluginCfg := opts.Plugins
	pluginCfg.Disabled = nil
	loaded, err := plugin.Load(baseDir, pluginCfg)
	if err != nil {
		return BuildResult{}, err
	}
	for _, warning := range loaded.Warnings {
		if warning.Code == "PLUGIN_LOAD_SKIPPED" {
			return BuildResult{}, fmt.Errorf("插件校验失败: %s", warning.Message)
		}
	}
	packages := make([]plugin.Package, 0, len(loaded.Packages))
	warnings := append([]plugin.Warning(nil), loaded.Warnings...)
	for _, pkg := range loaded.Packages {
		if pkg.Manifest.ID == userWorkflowPluginID {
			warnings = append(warnings, plugin.Warning{
				Code:       "MARKET_PLUGIN_SKIPPED",
				PluginID:   pkg.Manifest.ID,
				Message:    "本地用户工作流插件不参与市场发布",
				Suggestion: "如需共享工作流，请导出用户工作流 ZIP 后作为独立插件发布。",
			})
			continue
		}
		packages = append(packages, pkg)
	}
	if len(packages) == 0 {
		return BuildResult{}, fmt.Errorf("没有找到可发布的插件")
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Manifest.ID < packages[j].Manifest.ID })

	outputDir := strings.TrimSpace(opts.OutputDir)
	if outputDir == "" {
		outputDir = "market"
	}
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(baseDir, outputDir)
	}
	packagesDir := filepath.Join(outputDir, "packages")
	if err := os.MkdirAll(packagesDir, 0o755); err != nil {
		return BuildResult{}, fmt.Errorf("创建市场输出目录失败: %w", err)
	}

	index := Index{
		SchemaVersion: SchemaVersion,
		Name:          strings.TrimSpace(opts.Name),
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Plugins:       make([]Plugin, 0, len(packages)),
	}
	for _, pkg := range packages {
		data, err := buildPackageZip(pkg)
		if err != nil {
			return BuildResult{}, err
		}
		fileName, err := packageFileName(pkg.Manifest.ID, pkg.Manifest.Version)
		if err != nil {
			return BuildResult{}, err
		}
		if err := os.WriteFile(filepath.Join(packagesDir, fileName), data, 0o644); err != nil {
			return BuildResult{}, fmt.Errorf("写入插件包 %s 失败: %w", fileName, err)
		}
		sum := sha256.Sum256(data)
		entry := pluginEntry(pkg.Manifest)
		entry.Package = Package{
			URL:    "packages/" + fileName,
			SHA256: hex.EncodeToString(sum[:]),
			Size:   int64(len(data)),
		}
		index.Plugins = append(index.Plugins, entry)
	}

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return BuildResult{}, err
	}
	data = append(data, '\n')
	indexPath := filepath.Join(outputDir, "index.json")
	if err := os.WriteFile(indexPath, data, 0o644); err != nil {
		return BuildResult{}, fmt.Errorf("写入市场索引失败: %w", err)
	}
	return BuildResult{
		IndexPath:   indexPath,
		PackagesDir: packagesDir,
		Plugins:     len(index.Plugins),
		Warnings:    warnings,
	}, nil
}

func pluginEntry(manifest plugin.Manifest) Plugin {
	categories := make([]Category, 0, len(manifest.Contributes.Categories))
	for _, category := range manifest.Contributes.Categories {
		categories = append(categories, Category{
			ID:          category.ID,
			Name:        category.Name,
			Description: category.Description,
		})
	}
	tagSet := map[string]bool{}
	for _, tool := range manifest.Contributes.Tools {
		for _, tag := range tool.Tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tagSet[tag] = true
			}
		}
	}
	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return Plugin{
		ID:            manifest.ID,
		Name:          manifest.Name,
		Version:       manifest.Version,
		Description:   manifest.Description,
		Author:        manifest.Author,
		Categories:    categories,
		Tags:          tags,
		Compatibility: Compatibility{Opsctl: strings.TrimSpace(manifest.Compatibility.Opsctl)},
		ToolCount:     len(manifest.Contributes.Tools),
		WorkflowCount: len(manifest.Contributes.Workflows),
	}
}

func buildPackageZip(pkg plugin.Package) ([]byte, error) {
	rootName := pkg.Manifest.ID
	if !ValidPluginID(rootName) {
		return nil, fmt.Errorf("插件 ID 不安全: %q", rootName)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.WalkDir(pkg.Dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == pkg.Dir {
			return nil
		}
		rel, err := filepath.Rel(pkg.Dir, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("插件包含不支持的特殊文件: %s", rel)
		}
		name := rootName + "/" + filepath.ToSlash(rel)
		if hasUnsafeZipPath(name) {
			return fmt.Errorf("插件包含不安全路径: %s", rel)
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.Modified = fixedZipTime
		header.SetMode(0o755)
		writer, err := zw.CreateHeader(header)
		if err != nil {
			_ = file.Close()
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if int64(buf.Len()) > MaxPackageBytes {
		return nil, fmt.Errorf("插件包 %s 超过 %d MB 限制", pkg.Manifest.ID, MaxPackageBytes>>20)
	}
	return buf.Bytes(), nil
}

func packageFileName(id, version string) (string, error) {
	if !ValidPluginID(id) {
		return "", fmt.Errorf("插件 ID 不安全: %q", id)
	}
	safeVersion := sanitizeFileComponent(version)
	if safeVersion == "" {
		return "", fmt.Errorf("插件 %s 的版本号不安全: %q", id, version)
	}
	return id + "-" + safeVersion + ".zip", nil
}

func sanitizeFileComponent(value string) string {
	var builder strings.Builder
	for _, ch := range strings.TrimSpace(value) {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == '-' {
			builder.WriteRune(ch)
			continue
		}
		builder.WriteRune('_')
	}
	return strings.Trim(builder.String(), ".")
}

func hasUnsafeZipPath(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return true
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return true
		}
	}
	return false
}

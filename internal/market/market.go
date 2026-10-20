package market

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	SchemaVersion   = 1
	MaxIndexBytes   = 2 << 20
	MaxPackageBytes = 20 << 20
	DefaultCacheTTL = 60 * time.Second

	httpTimeout = 60 * time.Second
)

type Index struct {
	SchemaVersion int      `json:"schema_version"`
	Name          string   `json:"name,omitempty"`
	GeneratedAt   string   `json:"generated_at,omitempty"`
	Plugins       []Plugin `json:"plugins"`
}

type Plugin struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Version       string        `json:"version"`
	Description   string        `json:"description,omitempty"`
	Author        string        `json:"author,omitempty"`
	Categories    []Category    `json:"categories,omitempty"`
	Tags          []string      `json:"tags,omitempty"`
	Compatibility Compatibility `json:"compatibility,omitempty"`
	ToolCount     int           `json:"tool_count,omitempty"`
	WorkflowCount int           `json:"workflow_count,omitempty"`
	Package       Package       `json:"package"`
}

type Category struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type Compatibility struct {
	Opsctl string `json:"opsctl,omitempty"`
}

type Package struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size,omitempty"`
}

func (idx *Index) Validate() error {
	if idx == nil {
		return fmt.Errorf("市场索引为空")
	}
	if idx.SchemaVersion != SchemaVersion {
		return fmt.Errorf("不支持的市场索引版本: %d，当前支持 %d", idx.SchemaVersion, SchemaVersion)
	}
	seen := map[string]bool{}
	for i := range idx.Plugins {
		item := &idx.Plugins[i]
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.Version = strings.TrimSpace(item.Version)
		item.Description = strings.TrimSpace(item.Description)
		item.Author = strings.TrimSpace(item.Author)
		item.Compatibility.Opsctl = strings.TrimSpace(item.Compatibility.Opsctl)
		item.Package.URL = strings.TrimSpace(item.Package.URL)
		item.Package.SHA256 = strings.ToLower(strings.TrimSpace(item.Package.SHA256))
		if !ValidPluginID(item.ID) {
			return fmt.Errorf("市场插件 ID 不安全: %q", item.ID)
		}
		if item.Name == "" {
			return fmt.Errorf("市场插件 %s 缺少 name", item.ID)
		}
		if item.Version == "" {
			return fmt.Errorf("市场插件 %s 缺少 version", item.ID)
		}
		key := item.ID + "@" + item.Version
		if seen[key] {
			return fmt.Errorf("市场插件版本重复: %s", key)
		}
		seen[key] = true
		if item.Package.URL == "" {
			return fmt.Errorf("市场插件 %s 缺少 package.url", item.ID)
		}
		if !validSHA256(item.Package.SHA256) {
			return fmt.Errorf("市场插件 %s 的 package.sha256 无效", item.ID)
		}
		if item.Package.Size < 0 {
			return fmt.Errorf("市场插件 %s 的 package.size 无效", item.ID)
		}
		for j := range item.Categories {
			item.Categories[j].ID = strings.TrimSpace(item.Categories[j].ID)
			item.Categories[j].Name = strings.TrimSpace(item.Categories[j].Name)
			item.Categories[j].Description = strings.TrimSpace(item.Categories[j].Description)
		}
		tags := item.Tags[:0]
		for _, tag := range item.Tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
		item.Tags = tags
	}
	return nil
}

func ValidPluginID(id string) bool {
	if id == "" || id != strings.TrimSpace(id) || id == "." || id == ".." || strings.ContainsAny(id, `/\\`) {
		return false
	}
	for _, ch := range id {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == '-' {
			continue
		}
		return false
	}
	return true
}

type SourceKind string

const (
	SourceRemote SourceKind = "remote"
	SourceLocal  SourceKind = "local"
)

type Source struct {
	Kind     SourceKind
	Location string
	LocalDir string
}

func ResolveSource(raw, baseDir string) (Source, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Source{}, fmt.Errorf("市场源地址不能为空")
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
		parsed, err := url.Parse(raw)
		if err != nil {
			return Source{}, fmt.Errorf("市场源 URL 无效: %w", err)
		}
		if strings.TrimSpace(parsed.Host) == "" {
			return Source{}, fmt.Errorf("市场源 URL 缺少主机名")
		}
		if parsed.User != nil {
			return Source{}, fmt.Errorf("市场源 URL 不支持内嵌用户名密码")
		}
		return Source{Kind: SourceRemote, Location: parsed.String()}, nil
	case strings.HasPrefix(lower, "file://"):
		parsed, err := url.Parse(raw)
		if err != nil {
			return Source{}, fmt.Errorf("市场源文件 URL 无效: %w", err)
		}
		if parsed.User != nil {
			return Source{}, fmt.Errorf("市场源 URL 不支持内嵌用户名密码")
		}
		if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
			return Source{}, fmt.Errorf("市场源文件 URL 只支持本机路径")
		}
		value, err := url.PathUnescape(parsed.Path)
		if err != nil {
			return Source{}, fmt.Errorf("市场源文件 URL 无效: %w", err)
		}
		trimmed := strings.TrimPrefix(value, "/")
		if len(value) > 1 && filepath.VolumeName(trimmed) != "" {
			value = trimmed
		}
		return resolveLocalSource(value, baseDir)
	case strings.Contains(raw, "://"):
		return Source{}, fmt.Errorf("市场源只支持 http/https 地址或本地目录")
	default:
		return resolveLocalSource(raw, baseDir)
	}
}

func resolveLocalSource(value, baseDir string) (Source, error) {
	value = filepath.FromSlash(strings.TrimSpace(value))
	if value == "" {
		return Source{}, fmt.Errorf("市场源路径不能为空")
	}
	if !filepath.IsAbs(value) {
		if strings.TrimSpace(baseDir) == "" {
			baseDir = "."
		}
		value = filepath.Join(baseDir, value)
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return Source{}, fmt.Errorf("解析市场源路径失败: %w", err)
	}
	info, err := os.Stat(abs)
	switch {
	case err == nil && info.IsDir():
		abs = filepath.Join(abs, "index.json")
	case err == nil && !info.Mode().IsRegular():
		return Source{}, fmt.Errorf("市场源路径不是文件或目录: %s", abs)
	case err != nil && !os.IsNotExist(err):
		return Source{}, fmt.Errorf("读取市场源路径失败: %w", err)
	case os.IsNotExist(err) && !strings.EqualFold(filepath.Ext(abs), ".json"):
		abs = filepath.Join(abs, "index.json")
	}
	return Source{Kind: SourceLocal, Location: abs, LocalDir: filepath.Dir(abs)}, nil
}

type Client struct {
	HTTPClient *http.Client
	BaseDir    string
	CacheTTL   time.Duration

	mu    sync.Mutex
	cache map[string]cachedIndex
}

type cachedIndex struct {
	Index     *Index
	FetchedAt time.Time
}

func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: httpTimeout},
		CacheTTL:   DefaultCacheTTL,
		cache:      map[string]cachedIndex{},
	}
}

func (c *Client) Fetch(ctx context.Context, sourceURL string, force bool) (*Index, time.Time, error) {
	source, err := ResolveSource(sourceURL, c.baseDir())
	if err != nil {
		return nil, time.Time{}, err
	}
	if source.Kind == SourceLocal {
		return fetchLocalIndex(source)
	}
	ttl := c.CacheTTL
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	if !force {
		c.mu.Lock()
		entry, ok := c.cache[source.Location]
		c.mu.Unlock()
		if ok && time.Since(entry.FetchedAt) < ttl {
			return entry.Index, entry.FetchedAt, nil
		}
	}
	index, fetchedAt, err := fetchIndex(ctx, c.httpClient(), source.Location)
	if err != nil {
		return nil, time.Time{}, err
	}
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]cachedIndex{}
	}
	c.cache[source.Location] = cachedIndex{Index: index, FetchedAt: fetchedAt}
	c.mu.Unlock()
	return index, fetchedAt, nil
}

func (c *Client) baseDir() string {
	if c == nil {
		return ""
	}
	return c.BaseDir
}

func (c *Client) Download(ctx context.Context, indexURL string, pkg Package) ([]byte, error) {
	source, err := ResolveSource(indexURL, c.baseDir())
	if err != nil {
		return nil, err
	}
	if source.Kind == SourceLocal {
		return readLocalPackage(source, pkg)
	}
	resolved, err := resolvePackageURL(source.Location, pkg.URL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/zip, application/octet-stream")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载插件包失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载插件包失败: HTTP %d", resp.StatusCode)
	}
	if final := resp.Request.URL; !strings.EqualFold(final.Scheme, resolved.Scheme) || !strings.EqualFold(final.Host, resolved.Host) {
		return nil, fmt.Errorf("插件包重定向到了非同源地址: %s", final.Host)
	}
	if resp.ContentLength > MaxPackageBytes {
		return nil, fmt.Errorf("插件包超过 %d MB 限制", MaxPackageBytes>>20)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxPackageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取插件包失败: %w", err)
	}
	if int64(len(data)) > MaxPackageBytes {
		return nil, fmt.Errorf("插件包超过 %d MB 限制", MaxPackageBytes>>20)
	}
	if err := verifyPackage(data, pkg); err != nil {
		return nil, err
	}
	return data, nil
}

func fetchLocalIndex(source Source) (*Index, time.Time, error) {
	info, err := os.Stat(source.Location)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, time.Time{}, fmt.Errorf("本地市场索引不存在: %s，可运行 opsctl market index 生成", source.Location)
		}
		return nil, time.Time{}, fmt.Errorf("读取本地市场索引失败: %w", err)
	}
	data, exceeded, err := readLocalFile(source.Location, MaxIndexBytes)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("读取本地市场索引失败: %w", err)
	}
	if exceeded {
		return nil, time.Time{}, fmt.Errorf("市场索引超过 %d MB 限制", MaxIndexBytes>>20)
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, time.Time{}, fmt.Errorf("解析本地市场索引失败: %w", err)
	}
	if err := index.Validate(); err != nil {
		return nil, time.Time{}, fmt.Errorf("本地市场索引无效: %w", err)
	}
	return &index, info.ModTime().UTC(), nil
}

func readLocalPackage(source Source, pkg Package) ([]byte, error) {
	raw := strings.TrimSpace(pkg.URL)
	if raw == "" {
		return nil, fmt.Errorf("插件包地址为空")
	}
	if strings.Contains(raw, "://") {
		return nil, fmt.Errorf("本地市场的插件包地址必须是相对路径")
	}
	value, err := url.PathUnescape(raw)
	if err != nil {
		return nil, fmt.Errorf("插件包地址无效: %w", err)
	}
	value = filepath.FromSlash(value)
	if filepath.IsAbs(value) {
		return nil, fmt.Errorf("本地市场的插件包地址必须是相对路径")
	}
	root, err := filepath.EvalSymlinks(source.LocalDir)
	if err != nil {
		return nil, fmt.Errorf("读取市场目录失败: %w", err)
	}
	target := filepath.Join(root, value)
	if !pathWithin(root, target) {
		return nil, fmt.Errorf("插件包路径超出市场目录: %s", pkg.URL)
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return nil, fmt.Errorf("读取插件包失败: %w", err)
	}
	if !pathWithin(root, realTarget) {
		return nil, fmt.Errorf("插件包路径超出市场目录: %s", pkg.URL)
	}
	data, exceeded, err := readLocalFile(realTarget, MaxPackageBytes)
	if err != nil {
		return nil, fmt.Errorf("读取插件包失败: %w", err)
	}
	if exceeded {
		return nil, fmt.Errorf("插件包超过 %d MB 限制", MaxPackageBytes>>20)
	}
	if err := verifyPackage(data, pkg); err != nil {
		return nil, err
	}
	return data, nil
}

func readLocalFile(path string, limit int64) ([]byte, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%s 不是普通文件", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > limit {
		return nil, true, nil
	}
	return data, false, nil
}

func verifyPackage(data []byte, pkg Package) error {
	if pkg.Size > 0 && int64(len(data)) != pkg.Size {
		return fmt.Errorf("插件包大小不匹配: 期望 %d 字节，实际 %d 字节", pkg.Size, len(data))
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actual, pkg.SHA256) {
		return fmt.Errorf("插件包 SHA256 校验失败: 期望 %s，实际 %s", pkg.SHA256, actual)
	}
	return nil
}

func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func (c *Client) httpClient() *http.Client {
	if c != nil && c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: httpTimeout}
}

func fetchIndex(ctx context.Context, client *http.Client, sourceURL string) (*Index, time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("获取市场索引失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("获取市场索引失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxIndexBytes+1))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("读取市场索引失败: %w", err)
	}
	if int64(len(data)) > MaxIndexBytes {
		return nil, time.Time{}, fmt.Errorf("市场索引超过 %d MB 限制", MaxIndexBytes>>20)
	}
	var index Index
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&index); err != nil {
		return nil, time.Time{}, fmt.Errorf("解析市场索引失败: %w", err)
	}
	if err := index.Validate(); err != nil {
		return nil, time.Time{}, fmt.Errorf("市场索引无效: %w", err)
	}
	return &index, time.Now().UTC(), nil
}

func resolvePackageURL(indexURL, packageURL string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(indexURL))
	if err != nil {
		return nil, fmt.Errorf("市场源 URL 无效: %w", err)
	}
	ref, err := url.Parse(strings.TrimSpace(packageURL))
	if err != nil {
		return nil, fmt.Errorf("插件包 URL 无效: %w", err)
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return nil, fmt.Errorf("插件包 URL 只支持 http/https")
	}
	if !strings.EqualFold(resolved.Scheme, base.Scheme) || !strings.EqualFold(resolved.Host, base.Host) {
		return nil, fmt.Errorf("插件包 URL 必须与市场源同源")
	}
	if resolved.User != nil {
		return nil, fmt.Errorf("插件包 URL 不支持内嵌用户名密码")
	}
	return resolved, nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

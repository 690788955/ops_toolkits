package nexus

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"shell_ops/internal/market"
)

const (
	httpTimeout = 60 * time.Second
	maxPages    = 200
)

type Client struct {
	Config Config
	HTTP   *http.Client
}

func NewClient(cfg Config) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.InsecureSkipVerify {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		// 用户显式选择跳过校验，用于自签证书的内网 Nexus。
		transport.TLSClientConfig.InsecureSkipVerify = true
	}
	return &Client{
		Config: cfg,
		HTTP:   &http.Client{Timeout: httpTimeout, Transport: transport},
	}
}

type TestResult struct {
	Reachable   bool   `json:"reachable"`
	IndexExists bool   `json:"index_exists"`
	IndexValid  bool   `json:"index_valid"`
	PluginCount int    `json:"plugin_count"`
	IndexURL    string `json:"index_url"`
	Message     string `json:"message"`
}

type Asset struct {
	Path         string `json:"path"`
	Size         int64  `json:"size,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

func (c *Client) Test(ctx context.Context) TestResult {
	result := TestResult{IndexURL: c.Config.IndexURL()}
	data, found, err := c.getAsset(ctx, c.Config.IndexPath, market.MaxIndexBytes)
	if err != nil {
		result.Message = err.Error()
		return result
	}
	result.Reachable = true
	if !found {
		result.Message = "连接成功，但仓库中还没有索引文件；上传插件或重建索引后会自动创建。"
		return result
	}
	result.IndexExists = true
	var index market.Index
	if err := json.Unmarshal(data, &index); err != nil {
		result.Message = "已找到索引文件，但内容不是有效的 JSON: " + err.Error()
		return result
	}
	if err := index.Validate(); err != nil {
		result.Message = "已找到索引文件，但索引校验失败: " + err.Error()
		return result
	}
	result.IndexValid = true
	result.PluginCount = len(index.Plugins)
	result.Message = fmt.Sprintf("连接成功，索引包含 %d 个插件版本。", len(index.Plugins))
	return result
}

func (c *Client) FetchIndex(ctx context.Context) (*market.Index, bool, error) {
	data, found, err := c.getAsset(ctx, c.Config.IndexPath, market.MaxIndexBytes)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return &market.Index{SchemaVersion: market.SchemaVersion, Plugins: []market.Plugin{}}, false, nil
	}
	var index market.Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, true, fmt.Errorf("解析 Nexus 市场索引失败: %w", err)
	}
	if err := index.Validate(); err != nil {
		return nil, true, fmt.Errorf("Nexus 市场索引无效: %w", err)
	}
	return &index, true, nil
}

func (c *Client) PutIndex(ctx context.Context, index *market.Index) error {
	if index.SchemaVersion == 0 {
		index.SchemaVersion = market.SchemaVersion
	}
	if index.Plugins == nil {
		index.Plugins = []market.Plugin{}
	}
	if err := index.Validate(); err != nil {
		return fmt.Errorf("市场索引无效: %w", err)
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化市场索引失败: %w", err)
	}
	data = append(data, '\n')
	return c.putAsset(ctx, c.Config.IndexPath, data, "application/json")
}

func (c *Client) UploadPackage(ctx context.Context, assetPath string, data []byte) error {
	return c.putAsset(ctx, assetPath, data, "application/zip")
}

func (c *Client) DownloadPackage(ctx context.Context, assetPath string) ([]byte, error) {
	data, found, err := c.getAsset(ctx, assetPath, market.MaxPackageBytes)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("Nexus 插件包不存在: %s", assetPath)
	}
	return data, nil
}

func (c *Client) AssetExists(ctx context.Context, assetPath string) (bool, error) {
	req, err := c.newRequest(ctx, http.MethodHead, c.contentURL(assetPath), nil)
	if err != nil {
		return false, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return false, fmt.Errorf("访问 Nexus 文件 %s 失败: %w", assetPath, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return true, nil
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	case resp.StatusCode == http.StatusMethodNotAllowed:
		return c.existsByRange(ctx, assetPath)
	default:
		return false, fmt.Errorf("检查 Nexus 文件 %s 失败: %s", assetPath, httpStatusDetail(resp))
	}
}

func (c *Client) existsByRange(ctx context.Context, assetPath string) (bool, error) {
	req, err := c.newRequest(ctx, http.MethodGet, c.contentURL(assetPath), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return false, fmt.Errorf("访问 Nexus 文件 %s 失败: %w", assetPath, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return true, nil
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("检查 Nexus 文件 %s 失败: %s", assetPath, httpStatusDetail(resp))
	}
}

func (c *Client) DeleteAsset(ctx context.Context, assetPath string) (bool, error) {
	req, err := c.newRequest(ctx, http.MethodDelete, c.contentURL(assetPath), nil)
	if err != nil {
		return false, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return false, fmt.Errorf("删除 Nexus 文件 %s 失败: %w", assetPath, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return true, nil
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("删除 Nexus 文件 %s 失败: %s", assetPath, httpStatusDetail(resp))
	}
}

func (c *Client) ListPackages(ctx context.Context) ([]Asset, error) {
	prefix := c.Config.PackagePrefix()
	assets := []Asset{}
	token := ""
	for page := 0; page < maxPages; page++ {
		values := url.Values{}
		values.Set("repository", c.Config.Repository)
		if token != "" {
			values.Set("continuationToken", token)
		}
		endpoint := strings.TrimRight(c.Config.BaseURL, "/") + "/service/rest/v1/search/assets?" + values.Encode()
		req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.httpClient().Do(req)
		if err != nil {
			return nil, fmt.Errorf("查询 Nexus 仓库文件失败: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			detail := httpStatusDetail(resp)
			resp.Body.Close()
			return nil, fmt.Errorf("查询 Nexus 仓库文件失败: %s（请确认匿名用户具有仓库浏览和搜索权限）", detail)
		}
		var body struct {
			Items []struct {
				Path         string `json:"path"`
				FileSize     int64  `json:"fileSize"`
				LastModified string `json:"lastModified"`
			} `json:"items"`
			ContinuationToken string `json:"continuationToken"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, market.MaxIndexBytes)).Decode(&body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("解析 Nexus 文件列表失败: %w", err)
		}
		for _, item := range body.Items {
			itemPath := strings.TrimPrefix(strings.TrimSpace(item.Path), "/")
			if !strings.HasPrefix(itemPath, prefix) || !strings.HasSuffix(strings.ToLower(itemPath), ".zip") {
				continue
			}
			assets = append(assets, Asset{Path: itemPath, Size: item.FileSize, LastModified: item.LastModified})
		}
		if body.ContinuationToken == "" {
			break
		}
		token = body.ContinuationToken
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	return assets, nil
}

func UpsertPlugin(index *market.Index, entry market.Plugin) bool {
	for i := range index.Plugins {
		if index.Plugins[i].ID == entry.ID && index.Plugins[i].Version == entry.Version {
			index.Plugins[i] = entry
			return true
		}
	}
	index.Plugins = append(index.Plugins, entry)
	return false
}

func FindPlugin(index *market.Index, id, version string) (int, bool) {
	for i := range index.Plugins {
		item := index.Plugins[i]
		if item.ID == id && item.Version == version {
			return i, true
		}
	}
	return -1, false
}

func RemovePlugin(index *market.Index, id, version string) bool {
	position, ok := FindPlugin(index, id, version)
	if !ok {
		return false
	}
	index.Plugins = append(index.Plugins[:position], index.Plugins[position+1:]...)
	return true
}

func RelativeAssetURL(indexPath, assetPath string) string {
	indexDir := path.Dir(indexPath)
	if indexDir == "." || indexDir == "" {
		return assetPath
	}
	indexParts := strings.Split(indexDir, "/")
	assetParts := strings.Split(assetPath, "/")
	common := 0
	for common < len(indexParts) && common < len(assetParts)-1 && indexParts[common] == assetParts[common] {
		common++
	}
	parts := make([]string, 0, len(indexParts)-common+len(assetParts)-common)
	for i := common; i < len(indexParts); i++ {
		parts = append(parts, "..")
	}
	parts = append(parts, assetParts[common:]...)
	return strings.Join(parts, "/")
}

func ResolveAssetPath(indexPath, packageURL string) (string, error) {
	raw := strings.TrimSpace(packageURL)
	if raw == "" {
		return "", fmt.Errorf("插件包地址为空")
	}
	if strings.Contains(raw, "://") || strings.HasPrefix(raw, "//") {
		return "", fmt.Errorf("插件包地址必须是仓库内的相对路径")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("插件包地址无效: %w", err)
	}
	if parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("插件包地址必须是仓库内的相对路径")
	}
	if strings.HasPrefix(parsed.Path, "/") {
		return "", fmt.Errorf("插件包地址必须是仓库内的相对路径")
	}
	joined := path.Join(path.Dir(indexPath), parsed.Path)
	if joined == "." || joined == ".." || strings.HasPrefix(joined, "../") {
		return "", fmt.Errorf("插件包地址超出仓库范围")
	}
	return normalizeAssetPath(joined, "", "插件包地址")
}

func (c *Client) getAsset(ctx context.Context, assetPath string, limit int64) ([]byte, bool, error) {
	req, err := c.newRequest(ctx, http.MethodGet, c.contentURL(assetPath), nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("访问 Nexus 文件 %s 失败: %w", assetPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("获取 Nexus 文件 %s 失败: %s", assetPath, httpStatusDetail(resp))
	}
	if resp.ContentLength > limit {
		return nil, false, fmt.Errorf("Nexus 文件 %s 超过 %d MB 限制", assetPath, limit>>20)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, false, fmt.Errorf("读取 Nexus 文件 %s 失败: %w", assetPath, err)
	}
	if int64(len(data)) > limit {
		return nil, false, fmt.Errorf("Nexus 文件 %s 超过 %d MB 限制", assetPath, limit>>20)
	}
	return data, true, nil
}

func (c *Client) putAsset(ctx context.Context, assetPath string, data []byte, contentType string) error {
	req, err := c.newRequest(ctx, http.MethodPut, c.contentURL(assetPath), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("上传 Nexus 文件 %s 失败: %w", assetPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("上传 Nexus 文件 %s 失败: %s", assetPath, httpStatusDetail(resp))
	}
	return nil
}

func (c *Client) contentURL(assetPath string) string {
	return contentURL(c.Config.BaseURL, c.Config.Repository, assetPath)
}

func contentURL(baseURL, repository, assetPath string) string {
	return strings.TrimRight(baseURL, "/") + "/repository/" + repository + "/" + strings.TrimPrefix(assetPath, "/")
}

func (c *Client) newRequest(ctx context.Context, method, target string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if c != nil && c.Config.IsBasicAuth() {
		req.SetBasicAuth(c.Config.Username, c.Config.Password)
	}
	return req, nil
}

func (c *Client) httpClient() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: httpTimeout}
}

func httpStatusDetail(resp *http.Response) string {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	detail := strings.TrimSpace(string(data))
	if detail == "" {
		return resp.Status
	}
	return fmt.Sprintf("%s: %s", resp.Status, detail)
}

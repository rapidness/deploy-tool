// Package cluster 实现多节点部署：通过各节点 HTTP API 分发文件并触发部署。
package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"update-app/internal/config"
	"update-app/internal/service"
)

// File 待分发到远端节点的一个文件。
type File struct {
	Name string
	Data []byte
}

// Client 访问单个远端节点的 API 客户端。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient 根据节点配置创建客户端。
func NewClient(node config.ClusterNode) *Client {
	return &Client{
		baseURL: strings.TrimRight(node.URL, "/"),
		token:   node.APIToken,
		// 远端包含停服等待（最长 30s）+ 文件覆盖，超时需足够宽松
		http: &http.Client{Timeout: 10 * time.Minute},
	}
}

// Upload 上传文件到远端节点，返回该节点上的 upload_id。
func (c *Client) Upload(files []File) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("没有待上传的文件")
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		part, err := mw.CreateFormFile("files", f.Name)
		if err != nil {
			return "", fmt.Errorf("构造上传请求失败: %w", err)
		}
		if _, err := part.Write(f.Data); err != nil {
			return "", fmt.Errorf("写入上传请求失败: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("结束上传请求失败: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/upload", &buf)
	if err != nil {
		return "", fmt.Errorf("构造上传请求失败: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	c.setAuth(req)

	body, err := c.do(req, "上传")
	if err != nil {
		return "", err
	}
	var out struct {
		UploadID string `json:"upload_id"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("解析上传响应失败: %w", err)
	}
	if out.Error != "" {
		return "", fmt.Errorf("%s", out.Error)
	}
	if out.UploadID == "" {
		return "", fmt.Errorf("上传响应缺少 upload_id")
	}
	return out.UploadID, nil
}

// Deploy 触发远端节点执行同步部署，并返回其执行结果。
func (c *Client) Deploy(uploadID, serviceName, targetDir string, filesOnly bool) (service.DeployResult, error) {
	payload, err := json.Marshal(struct {
		UploadID  string `json:"upload_id"`
		Service   string `json:"service"`
		TargetDir string `json:"target_dir"`
		FilesOnly bool   `json:"files_only"`
	}{
		UploadID:  uploadID,
		Service:   serviceName,
		TargetDir: targetDir,
		FilesOnly: filesOnly,
	})
	if err != nil {
		return service.DeployResult{}, fmt.Errorf("编码部署请求失败: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/deploy", bytes.NewReader(payload))
	if err != nil {
		return service.DeployResult{}, fmt.Errorf("构造部署请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)

	body, err := c.do(req, "部署")
	if err != nil {
		return service.DeployResult{}, err
	}
	var out struct {
		service.DeployResult
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return service.DeployResult{}, fmt.Errorf("解析部署响应失败: %w", err)
	}
	if out.Error != "" {
		return service.DeployResult{}, fmt.Errorf("%s", out.Error)
	}
	return out.DeployResult, nil
}

// Get 请求远端节点的相对路径并返回响应体，用于代理日志查询/查看/下载。
func (c *Client) Get(pathWithQuery, action string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+pathWithQuery, nil)
	if err != nil {
		return nil, fmt.Errorf("构造%s请求失败: %w", action, err)
	}
	c.setAuth(req)
	return c.do(req, action)
}

// do 发送请求并返回响应体，非 200 状态视为错误。
func (c *Client) do(req *http.Request, action string) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s请求失败: %w", action, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取%s响应失败: %w", action, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s失败(HTTP %d): %s", action, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (c *Client) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

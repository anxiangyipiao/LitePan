// Package qb 封装 qBittorrent WebUI API v2 客户端：登录、添加磁链/种子、查询任务、删除任务。
package qb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"litepan/internal/httpx"
)

const (
	apiLogin   = "/api/v2/auth/login"
	apiLogout  = "/api/v2/auth/logout"
	apiAdd     = "/api/v2/torrents/add"
	apiInfo    = "/api/v2/torrents/info"
	apiDelete  = "/api/v2/torrents/delete"
	defaultTimeout = 20 * time.Second
)

type Options struct {
	BaseURL  string
	Username string
	Password string
	Timeout  time.Duration
}

type Client struct {
	baseURL string
	user    string
	pass    string
	http    *http.Client
}

// Task 是归一化后的 qB 下载任务。
type Task struct {
	Hash     string  `json:"hash"`
	Name     string  `json:"name"`
	State    string  `json:"state"`    // 归一的 LitePan 状态：pending/running/seeding/paused/error/finished
	Progress int     `json:"progress"` // 0-100
	Size     int64   `json:"size"`
	SavePath string  `json:"save_path"`
	AddedOn  int64   `json:"added_on"`
	Error    string  `json:"error,omitempty"`
}

func NewClient(opts Options) *Client {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	jar, _ := cookiejar.New(nil)
	hc := httpx.NewClient(httpx.ClientOptions{Timeout: timeout})
	hc.Jar = jar
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/"),
		user:    strings.TrimSpace(opts.Username),
		pass:    opts.Password,
		http:    hc,
	}
}

// Test 登录并取 qB 版本号，用于连通性测试。
func (c *Client) Test(ctx context.Context) (version string, err error) {
	if err := c.login(ctx); err != nil {
		return "", err
	}
	body, err := c.get(ctx, "/api/v2/app/version", nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

// Add 添加磁链/HTTP 链接（多行）。savePath 为空时使用 qB 默认下载目录。
func (c *Client) Add(ctx context.Context, urls []string, savePath string) error {
	if err := c.login(ctx); err != nil {
		return err
	}
	form := url.Values{}
	if u := joinURLs(urls); u != "" {
		form.Set("urls", u)
	}
	if savePath = strings.TrimSpace(savePath); savePath != "" {
		form.Set("savepath", savePath)
	}
	if len(form) == 0 {
		return fmt.Errorf("qB: 没有可添加的链接")
	}
	if _, err := c.postForm(ctx, apiAdd, form); err != nil {
		return err
	}
	return nil
}

// List 查询全部下载任务。
func (c *Client) List(ctx context.Context) ([]Task, error) {
	if err := c.login(ctx); err != nil {
		return nil, err
	}
	body, err := c.get(ctx, apiInfo, nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Hash      string  `json:"hash"`
		Name      string  `json:"name"`
		State     string  `json:"state"`
		Progress  float64 `json:"progress"`
		Size      int64   `json:"size"`
		SavePath  string  `json:"save_path"`
		AddedOn   int64   `json:"added_on"`
		Error     string  `json:"error"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("qB: 解析任务列表失败: %w", err)
	}
	out := make([]Task, 0, len(raw))
	for _, t := range raw {
		out = append(out, Task{
			Hash:     t.Hash,
			Name:     t.Name,
			State:    normalizeState(t.State),
			Progress: int(t.Progress * 100),
			Size:     t.Size,
			SavePath: t.SavePath,
			AddedOn:  t.AddedOn,
			Error:    t.Error,
		})
	}
	return out, nil
}

// Delete 删除任务。deleteFiles 为 true 时同时删除已下载文件。
func (c *Client) Delete(ctx context.Context, hashes []string, deleteFiles bool) error {
	if err := c.login(ctx); err != nil {
		return err
	}
	form := url.Values{}
	form.Set("hashes", strings.Join(hashes, "|"))
	form.Set("deleteFiles", strconv.FormatBool(deleteFiles))
	if _, err := c.postForm(ctx, apiDelete, form); err != nil {
		return err
	}
	return nil
}

// login 登录并保持 SID Cookie。
func (c *Client) login(ctx context.Context) error {
	form := url.Values{}
	form.Set("username", c.user)
	form.Set("password", c.pass)
	body, err := c.postForm(ctx, apiLogin, form)
	if err != nil {
		return err
	}
	if !strings.Contains(strings.ToLower(string(body)), "ok") {
		return fmt.Errorf("qB 登录失败（%s%s）：请检查 WebUI 地址、用户名与密码", c.baseURL, apiLogin)
	}
	return nil
}

func joinURLs(urls []string) string {
	var kept []string
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u != "" {
			kept = append(kept, u)
		}
	}
	return strings.Join(kept, "\n")
}

func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values) ([]byte, error) {
	fullURL := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, body, err := httpx.Execute(c.http, req, 1<<20)
	if err != nil {
		return nil, err
	}
	if resp != nil && resp.StatusCode >= http.StatusBadRequest {
		return body, fmt.Errorf("qB 请求 %s 返回 HTTP %d：%s", fullURL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (c *Client) get(ctx context.Context, endpoint string, query url.Values) ([]byte, error) {
	resp, body, err := httpx.DoJSON(ctx, c.http, http.MethodGet, c.baseURL+endpoint, query, nil, nil, 1<<20)
	if err != nil {
		return nil, err
	}
	if resp != nil && resp.StatusCode >= http.StatusBadRequest {
		return body, fmt.Errorf("qB 请求 %s%s 返回 HTTP %d：%s", c.baseURL, endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// normalizeState 把 qB 原生状态映射为 LitePan 展示状态。
func normalizeState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "downloading", "forceddl", "metadl", "forcedmetadl", "stalleddl", "checkingdl":
		return "running"
	case "uploading", "forcedup", "stalledup", "checkingup", "queueddl", "queuedup":
		return "seeding"
	case "pauseddl", "pausedup":
		return "paused"
	case "error", "missingfiles":
		return "error"
	case "completed":
		return "finished"
	default:
		return "running"
	}
}

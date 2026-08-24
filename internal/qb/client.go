package qb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"litepan/internal/httpx"
)

type Options struct {
	BaseURL  string
	Username string
	Password string
	Timeout  time.Duration
}

type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

func NewClient(opts Options) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		baseURL:  baseURL,
		username: strings.TrimSpace(opts.Username),
		password: opts.Password,
		http: httpx.NewClient(httpx.ClientOptions{
			Timeout: timeout,
		}),
	}
}

func (c *Client) baseOK() error {
	if c.baseURL == "" {
		return fmt.Errorf("qBittorrent 地址未配置，请先在系统设置里填写 qB WebUI 地址")
	}
	return nil
}

// Test 登录一次验证连通性。
func (c *Client) Test(ctx context.Context) error {
	if err := c.baseOK(); err != nil {
		return err
	}
	_, err := c.login(ctx)
	return err
}

// AddMagnet 将磁力链推送到 qBittorrent。savePath/category 为可选。
func (c *Client) AddMagnet(ctx context.Context, magnet, savePath, category string) error {
	if err := c.baseOK(); err != nil {
		return err
	}
	magnet = strings.TrimSpace(magnet)
	if magnet == "" {
		return fmt.Errorf("磁力链为空")
	}
	lower := strings.ToLower(magnet)
	if !strings.HasPrefix(lower, "magnet:") &&
		!strings.HasPrefix(lower, "http://") &&
		!strings.HasPrefix(lower, "https://") {
		return fmt.Errorf("不是有效的磁力链或种子链接")
	}
	cookie, err := c.login(ctx)
	if err != nil {
		return err
	}
	form := url.Values{}
	form.Set("urls", magnet)
	if s := strings.TrimSpace(savePath); s != "" {
		form.Set("savepath", s)
	}
	if cat := strings.TrimSpace(category); cat != "" {
		form.Set("category", cat)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/torrents/add", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("推送到 qB 失败：%w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("qB 返回 %d：%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	// qB 成功时通常返回空或 "Ok."
	return nil
}

func (c *Client) login(ctx context.Context) (string, error) {
	form := url.Values{}
	form.Set("username", c.username)
	form.Set("password", c.password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("连接 qB 失败：%w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	text := strings.TrimSpace(string(body))
	// 未设置鉴权时 qB 也返回 Ok.
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("qB 登录失败 %d：%s", resp.StatusCode, text)
	}
	if text != "Ok." && text != "" {
		// 某些版本返回 Fails.
		if strings.Contains(text, "Fails") || strings.Contains(text, "fail") {
			return "", fmt.Errorf("qB 登录失败：%s（请检查用户名/密码）", text)
		}
	}
	cookie := ""
	for _, ck := range resp.Cookies() {
		if ck.Name == "SID" {
			cookie = ck.String()
			break
		}
	}
	if cookie == "" {
		// 回退：从 Header 直接取
		if h := resp.Header.Get("Set-Cookie"); h != "" {
			cookie = h
		}
	}
	return cookie, nil
}

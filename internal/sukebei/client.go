// Package sukebei 是 Nyaa 成人镜像站 sukebei.nyaa.si 的搜索客户端。
// 它调用该站公开的 REST 搜索 API（GET /api/search），返回磁力链与种子统计。
// 站点可能受网络限制（用户环境可配代理），baseURL 与代理均可由上层设置注入。
package sukebei

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"litepan/internal/httpx"
)

const defaultBaseURL = "https://sukebei.nyaa.si"

type Options struct {
	BaseURL        string
	ProxyURL       string
	Timeout        time.Duration
	MaxRetries     int
	RetryBaseDelay time.Duration
}

// Result 是单条搜索结果的 TMDB 无关形状，直接对应用户展示。
type Result struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	Size      int64  `json:"size"`
	Date      int64  `json:"date"`
	Seeders   int    `json:"seeders"`
	Leechers  int    `json:"leechers"`
	Downloads int    `json:"downloads"`
	Hash      string `json:"hash"`
	Magnet    string `json:"magnet"`
	ViewURL   string `json:"view_url"`
}

type Client struct {
	baseURL        string
	http           *http.Client
	maxRetries     int
	retryBaseDelay time.Duration
}

func NewClient(opts Options) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	var proxy func(*http.Request) (*url.URL, error)
	if u := strings.TrimSpace(opts.ProxyURL); u != "" {
		if parsed, err := url.Parse(u); err == nil {
			proxy = http.ProxyURL(parsed)
		}
	}
	maxRetries := opts.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	if maxRetries > 3 {
		maxRetries = 3
	}
	retryBaseDelay := opts.RetryBaseDelay
	if maxRetries > 0 && retryBaseDelay <= 0 {
		retryBaseDelay = time.Second
	}
	return &Client{
		baseURL:        baseURL,
		maxRetries:     maxRetries,
		retryBaseDelay: retryBaseDelay,
		http: httpx.NewClient(httpx.ClientOptions{
			Timeout: timeout,
			Proxy:   proxy,
		}),
	}
}

// BuildProxyURL 把「代理地址 + 用户名 + 密码」拼成带认证的代理 URL；无认证时原样返回地址。
func BuildProxyURL(raw, username, password string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	user := strings.TrimSpace(username)
	pwd := strings.TrimSpace(password)
	if user == "" || pwd == "" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.User = url.UserPassword(user, pwd)
	return parsed.String()
}

// Search 按关键词搜索，返回结果列表。limit <= 0 时默认 20，上限 50。
// 类别固定 c=0_0（全部），排序用站点默认。
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("sukebei: 搜索关键词为空")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("c", "0_0")
	q.Set("limit", strconv.Itoa(limit))
	body, err := c.get(ctx, "/api/search", q)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("sukebei: 响应解析失败：%w", err)
	}
	out := make([]Result, 0, len(payload.Results))
	for _, raw := range payload.Results {
		if r := decodeResult(raw, c.baseURL); r.Name != "" || r.Magnet != "" {
			out = append(out, r)
		}
	}
	return out, nil
}

func decodeResult(raw json.RawMessage, baseURL string) Result {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return Result{}
	}
	r := Result{
		ID:        int64Of(m["id"]),
		Name:      strings.TrimSpace(anyString(m["name"])),
		Category:  strings.TrimSpace(anyString(m["category"])),
		Size:      int64Of(m["size"]),
		Date:      int64Of(m["date"]),
		Seeders:   intOf(m["seeders"]),
		Leechers:  intOf(m["leechers"]),
		Downloads: intOf(m["downloads"]),
		Hash:      strings.TrimSpace(anyString(m["hash"])),
		Magnet:    strings.TrimSpace(anyString(m["magnet"])),
	}
	if r.ID > 0 {
		r.ViewURL = fmt.Sprintf("%s/view/%d", baseURL, r.ID)
	}
	return r
}

func (c *Client) get(ctx context.Context, endpoint string, query url.Values) ([]byte, error) {
	rawURL := c.baseURL + endpoint
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		resp, body, err := httpx.DoJSON(ctx, c.http, http.MethodGet, rawURL, query, nil, nil, 2<<20)
		if err == nil && resp != nil && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			return body, nil
		}
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("sukebei: http status %d", status)
		}
		if attempt == c.maxRetries || ctx.Err() != nil || (err == nil && !isRetryableHTTPStatus(status)) {
			return nil, lastErr
		}
		if err := waitRetry(ctx, retryDelay(c.retryBaseDelay, attempt)); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func isRetryableHTTPStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func retryDelay(base time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	delay := base * time.Duration(attempt*2+1)
	if delay > 15*time.Second {
		return 15 * time.Second
	}
	return delay
}

func waitRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func anyString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func intOf(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(t))
		return i
	default:
		return 0
	}
}

func int64Of(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case json.Number:
		i, _ := t.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return i
	default:
		return 0
	}
}

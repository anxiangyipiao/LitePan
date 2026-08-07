// Package sukebei 是 Nyaa 成人镜像站 sukebei.nyaa.si 的搜索客户端。
// 该站 /api/search 在某些部署上返回 404，可靠的方式是抓 HTML 搜索页
// （GET /?q=..&s=seeders&o=desc）并解析表格行，这与项目此前验证过的方法一致。
// 站点可能受网络限制，baseURL 与代理（支持 http/https/socks5/socks5h）均由上层设置注入。
package sukebei

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"litepan/internal/httpx"
)

const defaultBaseURL = "https://sukebei.nyaa.si"

const maxHTMLBytes = 5 << 20

type Options struct {
	BaseURL        string
	ProxyURL       string
	Timeout        time.Duration
	MaxRetries     int
	RetryBaseDelay time.Duration
}

// Result 是单条搜索结果。
type Result struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
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
// 支持 http/https/socks5/socks5h 前缀。
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

// Search 按关键词抓 HTML 搜索页并按做种数降序解析，返回结果列表。
// limit <= 0 时默认 20，返回结果上限 50。
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
	q.Set("s", "seeders")
	q.Set("o", "desc")
	body, err := c.get(ctx, c.baseURL+"/", q)
	if err != nil {
		return nil, err
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sukebei: 页面解析失败：%w", err)
	}
	results := parseResultsHTML(doc, c.baseURL)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (c *Client) get(ctx context.Context, rawURL string, query url.Values) ([]byte, error) {
	requestURL := rawURL
	if len(query) > 0 {
		requestURL = rawURL + "?" + query.Encode()
	}
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
		resp, err := c.http.Do(req)
		if err == nil && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxHTMLBytes+1))
			_ = resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else if len(data) > maxHTMLBytes {
				return nil, fmt.Errorf("sukebei: 页面过大")
			} else {
				return data, nil
			}
			if attempt == c.maxRetries || ctx.Err() != nil {
				return nil, lastErr
			}
			if err := waitRetry(ctx, retryDelay(c.retryBaseDelay, attempt)); err != nil {
				return nil, err
			}
			continue
		}
		status := 0
		if resp != nil {
			status = resp.StatusCode
			_ = resp.Body.Close()
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

func parseResultsHTML(doc *html.Node, baseURL string) []Result {
	var out []Result
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tbody" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "tr" {
					if r := parseRow(c, baseURL); r != nil {
						out = append(out, *r)
					}
				}
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func parseRow(tr *html.Node, baseURL string) *Result {
	var r Result
	var sizeCell string
	var numericCells []string
	var dateUnix int64
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		switch n.Data {
		case "a":
			href := attrVal(n, "href")
			if strings.HasPrefix(href, "/view/") {
				if id, err := strconv.ParseInt(strings.TrimPrefix(href, "/view/"), 10, 64); err == nil {
					r.ID = id
				}
				r.ViewURL = baseURL + href
				if t := strings.TrimSpace(attrVal(n, "title")); t != "" {
					r.Name = t
				} else {
					r.Name = strings.TrimSpace(nodeText(n))
				}
			} else if strings.HasPrefix(href, "magnet:") {
				r.Magnet = href
				r.Hash = magnetHash(href)
			}
		case "td":
			if strings.Contains(attrVal(n, "class"), "text-center") {
				text := strings.TrimSpace(nodeText(n))
				switch {
				case sizeCell == "" && parseSize(text) > 0:
					sizeCell = text
				case isNumeric(text):
					numericCells = append(numericCells, text)
				}
			}
		}
		if ts := attrVal(n, "data-timestamp"); ts != "" {
			if u, err := strconv.ParseInt(ts, 10, 64); err == nil {
				dateUnix = u
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(tr)
	if r.Magnet == "" {
		return nil
	}
	// 按内容特征识别：含单位单元格=大小；纯数字单元格依次=做种/下载/完成
	if sizeCell != "" {
		r.Size = parseSize(sizeCell)
	}
	if len(numericCells) > 0 {
		r.Seeders = parseIntText(numericCells[0])
	}
	if len(numericCells) > 1 {
		r.Leechers = parseIntText(numericCells[1])
	}
	if len(numericCells) > 2 {
		r.Downloads = parseIntText(numericCells[2])
	}
	if dateUnix > 0 {
		r.Date = dateUnix
	}
	if r.Name == "" {
		r.Name = fmt.Sprintf("torrent-%d", r.ID)
	}
	return &r
}

func isNumeric(text string) bool {
	s := strings.ReplaceAll(strings.TrimSpace(text), ",", "")
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(m *html.Node)
	walk = func(m *html.Node) {
		if m.Type == html.TextNode {
			b.WriteString(m.Data)
			return
		}
		for c := m.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func parseIntText(text string) int {
	digits := strings.TrimFunc(text, func(r rune) bool {
		return r < '0' || r > '9'
	})
	if digits == "" {
		return 0
	}
	n, _ := strconv.Atoi(digits)
	return n
}

func parseSize(text string) int64 {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) < 2 {
		return 0
	}
	val, err := strconv.ParseFloat(strings.ReplaceAll(parts[0], ",", ""), 64)
	if err != nil {
		return 0
	}
	units := map[string]float64{
		"B": 1, "KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40,
	}
	mult, ok := units[parts[1]]
	if !ok {
		return 0
	}
	return int64(val * mult)
}

func magnetHash(magnet string) string {
	const prefix = "urn:btih:"
	i := strings.Index(magnet, prefix)
	if i < 0 {
		return ""
	}
	rest := magnet[i+len(prefix):]
	if j := strings.IndexAny(rest, "&?"); j >= 0 {
		rest = rest[:j]
	}
	return rest
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

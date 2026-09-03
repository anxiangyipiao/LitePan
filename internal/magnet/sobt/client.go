// Package sobt 实现 sobt10.vip（种子搜索）的搜索客户端。
//
// 搜索方式：POST / 带 q 参数，需要 cookie 维持 session。
// 结果页直接包含 torrent hash（即 btih），可直接拼 magnet，无需 N+1 请求。
package sobt

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	defaultBaseURL = "https://sobt10.vip"
	maxHTMLBytes   = 5 << 20
)

type Options struct {
	BaseURL  string
	ProxyURL string
	Timeout  time.Duration
}

// Result 是单条搜索结果。
type Result struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Date     int64  `json:"date"`
	Hash     string `json:"hash"`
	Magnet   string `json:"magnet"`
	ViewURL  string `json:"view_url"`
	Category string `json:"category"`
}

type Client struct {
	baseURL string
	http    *http.Client
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
	jar, _ := cookiejar.New(nil)
	tr := &http.Transport{
		Proxy:             proxy,
		DisableKeepAlives: false,
	}
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout:   timeout,
			Jar:       jar,
			Transport: tr,
			// 不自动跟随 POST 重定向（302 → GET 会丢 cookie）
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 1 && via[0].Method == http.MethodPost {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}
}

// Search 搜索 sobt，返回结果列表，每条结果含 magnet。
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("sobt: 搜索关键词为空")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	// 1. 先 GET 首页建立 session cookie
	if err := c.ensureSession(ctx); err != nil {
		return nil, fmt.Errorf("sobt: 建立会话失败：%w", err)
	}

	// 2. POST 搜索
	body, err := c.postSearch(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("sobt: 搜索请求失败：%w", err)
	}
	results, err := parseSearchHTML(body, c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("sobt: 搜索页解析失败：%w", err)
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (c *Client) ensureSession(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func (c *Client) postSearch(ctx context.Context, query string) ([]byte, error) {
	form := url.Values{}
	form.Set("q", query)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", c.baseURL+"/")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// POST 返回 302，手动跟随到 GET
	if resp.StatusCode == http.StatusFound {
		loc := resp.Header.Get("Location")
		if loc == "" {
			return nil, fmt.Errorf("sobt: 重定向无 Location")
		}
		if !strings.HasPrefix(loc, "http") {
			loc = c.baseURL + loc
		}
		return c.get(ctx, loc)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxHTMLBytes {
		return nil, fmt.Errorf("sobt: 页面过大")
	}
	return data, nil
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxHTMLBytes {
		return nil, fmt.Errorf("sobt: 页面过大")
	}
	return data, nil
}

// parseSearchHTML 从搜索结果页 HTML 提取结果列表。
func parseSearchHTML(data []byte, baseURL string) ([]Result, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var results []Result
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" && attrContainsClass(n, "search-item") {
			if r := parseSearchItem(n, baseURL); r != nil {
				results = append(results, *r)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return results, nil
}

func parseSearchItem(n *html.Node, baseURL string) *Result {
	var r Result
	var title, hash string

	// 提取 <a href="/torrent/{hash}.html">{title}</a>
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		if c.Data == "div" && attrContainsClass(c, "item-title") {
			if a := findFirstChild(c, "a"); a != nil {
				href := attrVal(a, "href")
				if strings.HasPrefix(href, "/torrent/") && strings.HasSuffix(href, ".html") {
					hash = strings.TrimPrefix(href, "/torrent/")
					hash = strings.TrimSuffix(hash, ".html")
					hash = strings.ToUpper(hash)
				}
				title = strings.TrimSpace(nodeText(a))
			}
		}
		if c.Data == "div" && attrContainsClass(c, "item-bar") {
			parseItemBar(c, &r)
		}
	}

	if hash == "" {
		return nil
	}
	r.Hash = hash
	r.Name = title
	r.Magnet = "magnet:?xt=urn:btih:" + hash
	r.ViewURL = baseURL + "/torrent/" + strings.ToLower(hash) + ".html"
	return &r
}

func parseItemBar(n *html.Node, r *Result) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "span" {
			continue
		}
		text := strings.TrimSpace(nodeText(c))
		switch {
		case strings.Contains(text, "创建时间"):
			dateStr := extractBoldText(c)
			if t, err := time.Parse("2006-01-02", dateStr); err == nil {
				r.Date = t.Unix()
			}
		case strings.Contains(text, "文件大小"):
			sizeStr := extractBoldText(c)
			r.Size = parseSizeStr(sizeStr)
		}
	}
}

func extractBoldText(n *html.Node) string {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "b" {
			return strings.TrimSpace(nodeText(c))
		}
	}
	return ""
}

func findFirstChild(n *html.Node, tag string) *html.Node {
	var result *html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if result != nil {
			return
		}
		if node.Type == html.ElementNode && node.Data == tag {
			result = node
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return result
}

// --- HTML helpers ---

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func attrContainsClass(n *html.Node, cls string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				if c == cls {
					return true
				}
			}
		}
	}
	return false
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
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

func parseSizeStr(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ToUpper(s)
	suffixes := []struct {
		suffix string
		mult   float64
	}{
		{"TB", 1099511627776},
		{"GB", 1073741824},
		{"G", 1073741824},
		{"MB", 1048576},
		{"M", 1048576},
		{"KB", 1024},
		{"K", 1024},
		{"B", 1},
	}
	for _, sf := range suffixes {
		if strings.HasSuffix(s, sf.suffix) {
			numStr := strings.TrimSuffix(s, sf.suffix)
			numStr = strings.TrimSpace(numStr)
			val, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0
			}
			return int64(val * sf.mult)
		}
	}
	val, _ := strconv.ParseFloat(s, 64)
	return int64(val)
}

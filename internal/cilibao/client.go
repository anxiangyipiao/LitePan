// Package cilibao 实现磁力宝（cilibao）的搜索客户端。
//
// 搜索方式：GET /s/{base64}，纯静态 HTML，无需 cookie/session。
// 详情页 /detail/{shortId} 有 magnet。
package cilibao

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const (
	defaultBaseURL = "https://163.192.12.82:12580"
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
	tr := &http.Transport{Proxy: proxy}
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout:   timeout,
			Transport: tr,
		},
	}
}

// Search 搜索 cilibao，返回结果列表。
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("cilibao: 搜索关键词为空")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	wd := base64.RawStdEncoding.EncodeToString([]byte(query))
	searchURL := c.baseURL + "/s/" + url.PathEscape(wd)
	body, err := c.get(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("cilibao: 搜索请求失败：%w", err)
	}
	items, err := parseSearchHTML(body)
	if err != nil {
		return nil, fmt.Errorf("cilibao: 搜索页解析失败：%w", err)
	}
	if len(items) > limit {
		items = items[:limit]
	}
	if len(items) == 0 {
		return nil, nil
	}

	results := make([]Result, len(items))
	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(idx int, it searchItem) {
			defer wg.Done()
			results[idx] = c.fetchDetail(ctx, it)
		}(i, item)
	}
	wg.Wait()

	out := make([]Result, 0, len(results))
	for _, r := range results {
		if r.Magnet != "" {
			out = append(out, r)
		}
	}
	return out, nil
}

type searchItem struct {
	shortID string
	title   string
	size    string
	date    string
}

func (c *Client) fetchDetail(ctx context.Context, item searchItem) Result {
	r := Result{
		Name:    item.title,
		ViewURL: c.baseURL + "/detail/" + item.shortID,
		Size:    parseSizeStr(item.size),
	}
	if t, err := time.Parse("2006-01-02", item.date); err == nil {
		r.Date = t.Unix()
	}

	detailURL := c.baseURL + "/detail/" + item.shortID
	body, err := c.get(ctx, detailURL)
	if err != nil {
		return r
	}
	magnet := extractMagnet(body)
	if magnet == "" {
		return r
	}
	r.Magnet = magnet
	r.Hash = magnetHash(magnet)
	return r
}

func extractMagnet(data []byte) string {
	s := string(data)
	// <textarea class="textarea" ...>magnet:...</textarea>
	const marker = `class="textarea"`
	idx := strings.Index(s, marker)
	if idx >= 0 {
		start := strings.IndexByte(s[idx:], '>')
		if start < 0 {
			return ""
		}
		start += idx + 1
		end := strings.Index(s[start:], "</textarea>")
		if end < 0 {
			return ""
		}
		magnet := strings.TrimSpace(s[start : start+end])
		if strings.HasPrefix(magnet, "magnet:") {
			return magnet
		}
	}
	// 备用：<a href="magnet:?xt=urn:btih:...">
	const prefix = `href="magnet:?xt=urn:btih:`
	idx = strings.Index(s, prefix)
	if idx < 0 {
		return ""
	}
	start := idx + len(prefix)
	end := strings.IndexByte(s[start:], '"')
	if end < 0 {
		return ""
	}
	return "magnet:?xt=urn:btih:" + s[start:start+end]
}

func parseSearchHTML(data []byte) ([]searchItem, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var items []searchItem
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" && attrContainsClass(n, "search-item") {
			if it := parseSearchItem(n); it.shortID != "" {
				items = append(items, it)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return items, nil
}

func parseSearchItem(n *html.Node) searchItem {
	var it searchItem
	var find func(*html.Node)
	find = func(node *html.Node) {
		if it.shortID != "" {
			return
		}
		if node.Type == html.ElementNode && node.Data == "a" {
			href := attrVal(node, "href")
			if strings.HasPrefix(href, "/detail/") {
				it.shortID = strings.TrimPrefix(href, "/detail/")
				it.title = strings.TrimSpace(nodeText(node))
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(n)

	// 找 item-bar
	var walk2 func(*html.Node)
	walk2 = func(node *html.Node) {
		if it.size != "" && it.date != "" {
			return
		}
		if node.Type == html.ElementNode && node.Data == "div" && attrContainsClass(node, "item-bar") {
			text := nodeText(node)
			if idx := strings.Index(text, "创建时间："); idx >= 0 {
				rest := text[idx+len("创建时间："):]
				if end := strings.Index(rest, "文件大小："); end >= 0 {
					it.date = strings.TrimSpace(rest[:end])
				}
			}
			if idx := strings.Index(text, "文件大小："); idx >= 0 {
				rest := text[idx+len("文件大小："):]
				end := strings.Index(rest, "下载热度：")
				if end < 0 {
					end = len(rest)
				}
				it.size = strings.TrimSpace(rest[:end])
			}
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk2(c)
		}
	}
	walk2(n)
	return it
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cilibao: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxHTMLBytes {
		return nil, fmt.Errorf("cilibao: 页面过大")
	}
	return data, nil
}

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
	return strings.ToUpper(rest)
}

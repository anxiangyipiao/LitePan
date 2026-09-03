// Package seedhub 实现 seedhub.cc 的搜索客户端。
//
// seedhub.cc 是影视资源分享站，搜索结果是电影/剧集级别，
// 每个结果下有多个种子（seed），magnet 链接通过 /link_start/ 页面的
// base64 编码 JS 变量获取。
//
// 流程：搜索 → 详情页取 seed_id → /link_start/ 解 base64 拿 magnet。
package seedhub

import (
	"bytes"
	"context"
	"crypto/tls"
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
	defaultBaseURL = "https://www.seedhub.cc"
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
	// 自定义 transport：宽松 TLS 配置 + 禁用 keep-alive，兼容各种代理/CDN 环境
	tr := &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives:   true,
		MaxIdleConnsPerHost: 0,
		Proxy:               proxy,
	}
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout:   timeout,
			Transport: tr,
		},
	}
}

// Search 搜索 seedhub，返回结果列表，每条结果含 magnet。
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("seedhub: 搜索关键词为空")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	// 1. 搜索页 → 电影列表
	searchURL := c.baseURL + "/s/" + url.PathEscape(query) + "/"
	body, err := c.get(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("seedhub: 搜索请求失败：%w", err)
	}
	movies, err := parseSearchHTML(body)
	if err != nil {
		return nil, fmt.Errorf("seedhub: 搜索页解析失败：%w", err)
	}
	if len(movies) > limit {
		movies = movies[:limit]
	}
	if len(movies) == 0 {
		return nil, nil
	}

	// 2. 并发抓详情页 + magnet
	var mu sync.Mutex
	var allResults []Result
	var wg sync.WaitGroup
	for _, m := range movies {
		wg.Add(1)
		go func(mv movieCard) {
			defer wg.Done()
			seeds := c.fetchMovieSeeds(ctx, mv)
			mu.Lock()
			allResults = append(allResults, seeds...)
			mu.Unlock()
		}(m)
	}
	wg.Wait()

	// 过滤掉没有 magnet 的结果
	out := make([]Result, 0, len(allResults))
	for _, r := range allResults {
		if r.Magnet != "" {
			out = append(out, r)
		}
	}
	return out, nil
}

type movieCard struct {
	id    int64
	title string
	genre string
}

// fetchMovieSeeds 对一部电影：抓详情页取所有 seed_id，再并发抓 magnet。
func (c *Client) fetchMovieSeeds(ctx context.Context, mv movieCard) []Result {
	detailURL := c.baseURL + "/movies/" + strconv.FormatInt(mv.id, 10) + "/"
	body, err := c.get(ctx, detailURL)
	if err != nil {
		return nil
	}
	seeds := parseDetailSeeds(body)
	if len(seeds) == 0 {
		return nil
	}

	// 并发抓每个 seed 的 magnet
	results := make([]Result, len(seeds))
	var wg sync.WaitGroup
	for i, s := range seeds {
		wg.Add(1)
		go func(idx int, si seedInfo) {
			defer wg.Done()
			linkURL := c.baseURL + "/link_start/?seed_id=" + si.seedID + "&movie_title=" + url.PathEscape(mv.title)
			linkBody, err := c.get(ctx, linkURL)
			if err != nil {
				return
			}
			magnet := extractMagnetFromBody(linkBody)
			if magnet == "" {
				return
			}
			results[idx] = Result{
				ID:       mv.id,
				Name:     si.title,
				Size:     parseSizeStr(si.size),
				Hash:     magnetHash(magnet),
				Magnet:   magnet,
				ViewURL:  c.baseURL + "/movies/" + strconv.FormatInt(mv.id, 10) + "/",
				Category: mv.genre,
			}
		}(i, s)
	}
	wg.Wait()
	return results
}

// parseSearchHTML 从搜索页 HTML 提取电影卡片列表。
func parseSearchHTML(data []byte) ([]movieCard, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var out []movieCard
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" && attrContainsClass(n, "cover") {
			mc := parseCoverDiv(n)
			if mc.id > 0 {
				out = append(out, mc)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out, nil
}

func parseCoverDiv(n *html.Node) movieCard {
	var mc movieCard
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		if c.Data == "a" {
			href := attrVal(c, "href")
			if strings.HasPrefix(href, "/movies/") {
				idStr := strings.TrimSuffix(strings.TrimPrefix(href, "/movies/"), "/")
				if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
					mc.id = id
				}
				if t := strings.TrimSpace(attrVal(c, "title")); t != "" {
					mc.title = t
				}
			}
		}
		if c.Data == "ul" {
			for li := c.FirstChild; li != nil; li = li.NextSibling {
				if li.Type != html.ElementNode || li.Data != "li" {
					continue
				}
				text := strings.TrimSpace(nodeText(li))
				// 年份/类型/国家信息行，格式如 "2023 / 电影 / 韩国 / 韩语 / 黄政民 郑雨盛"
				if strings.Contains(text, " / ") && len(text) > 10 && !strings.HasPrefix(text, "豆瓣") {
					mc.genre = text
				}
			}
		}
	}
	return mc
}

// seedInfo 是详情页中单个种子的信息。
type seedInfo struct {
	seedID string
	title  string
	size   string
}

// parseDetailSeeds 从详情页 HTML 提取所有 seed 的 seed_id、标题、大小。
func parseDetailSeeds(data []byte) []seedInfo {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	var seeds []seedInfo
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "ul" && attrContainsClass(n, "seeds") {
			for li := n.FirstChild; li != nil; li = li.NextSibling {
				if li.Type != html.ElementNode || li.Data != "li" {
					continue
				}
				var si seedInfo
				for c := li.FirstChild; c != nil; c = c.NextSibling {
					if c.Type != html.ElementNode {
						continue
					}
					if c.Data == "a" {
						href := attrVal(c, "href")
						if strings.Contains(href, "seed_id=") {
							si.seedID = extractQueryParam(href, "seed_id")
							si.title = strings.TrimSpace(attrVal(c, "title"))
							if si.title == "" {
								si.title = strings.TrimSpace(nodeText(c))
							}
						}
					}
					if c.Data == "code" && attrContainsClass(c, "size") {
						si.size = strings.TrimSpace(nodeText(c))
					}
				}
				if si.seedID != "" {
					seeds = append(seeds, si)
				}
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return seeds
}

// extractMagnetFromBody 从 /link_start/ 页面 JS 中提取 base64 编码的 magnet。
// 匹配：const data = "base64_string";
func extractMagnetFromBody(data []byte) string {
	s := string(data)
	const marker = `const data = "`
	idx := strings.Index(s, marker)
	if idx < 0 {
		return ""
	}
	start := idx + len(marker)
	end := strings.IndexByte(s[start:], '"')
	if end < 0 {
		return ""
	}
	b64 := s[start : start+end]
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return ""
	}
	magnet := strings.TrimSpace(string(decoded))
	if !strings.HasPrefix(magnet, "magnet:") {
		return ""
	}
	return magnet
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	data, err := c.doGet(ctx, rawURL)
	if err != nil && strings.HasPrefix(rawURL, "https://") {
		// HTTPS 失败，降级到 HTTP 重试（服务器 TLS 兼容性问题）
		httpURL := "http://" + rawURL[len("https://"):]
		if alt, altErr := c.doGet(ctx, httpURL); altErr == nil {
			return alt, nil
		}
	}
	return data, err
}

func (c *Client) doGet(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLBytes+1))
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("seedhub: HTTP %d", resp.StatusCode)
		}
		if len(data) > maxHTMLBytes {
			return nil, fmt.Errorf("seedhub: 页面过大")
		}
		return data, nil
	}
	return nil, fmt.Errorf("seedhub: 请求失败（重试3次）：%w", lastErr)
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

func extractQueryParam(rawURL, key string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Query().Get(key)
}

func parseSizeStr(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ToUpper(s)
	// 按后缀长度倒序，避免 "B" 先匹配 "GB"
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
	return rest
}

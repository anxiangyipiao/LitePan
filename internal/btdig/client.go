// Package btdig 实现 btdig.com 的 HTML 搜索结果解析。
// btdig.com 是一个 DHT 搜索引擎，搜索结果页返回 HTML，
// 每条结果在 <div class="one_result"> 内，包含标题、大小、磁力链。
package btdig

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"litepan/internal/httpx"
)

var sizeRe = regexp.MustCompile(`([\d.,]+)\s*(KB|MB|GB|TB)`)

type Options struct {
	BaseURL        string
	ProxyURL       string
	Timeout        time.Duration
}

type Result struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Date     int64  `json:"date"`
	Seeders  int    `json:"seeders"`
	Leechers int    `json:"leechers"`
	Downloads int   `json:"downloads"`
	Hash     string `json:"hash"`
	Magnet   string `json:"magnet"`
	ViewURL  string `json:"view_url"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(opts Options) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://btdig.com"
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
	return &Client{
		baseURL: baseURL,
		http: httpx.NewClient(httpx.ClientOptions{
			Timeout: timeout,
			Proxy:   proxy,
		}),
	}
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	u := c.baseURL + "/search?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("btdig %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return parseHTML(body, limit), nil
}

func parseHTML(data []byte, limit int) []Result {
	var results []Result

	// 每条结果在 <div class="one_result"> ... </div> 中
	// 用正则逐条拆分
	parts := bytes.Split(data, []byte(`class="one_result"`))
	for i := 1; i < len(parts) && len(results) < limit; i++ {
		chunk := parts[i]
		r := parseOneResult(chunk)
		if r.Magnet != "" {
			r.ID = int64(len(results) + 1)
			results = append(results, r)
		}
	}
	return results
}

func parseOneResult(data []byte) Result {
	var r Result

	// 标题：<div class="torrent_name"><a href="URL">TITLE</a></div>
	if m := extractBetween(data, []byte(`class="torrent_name"><a href="`), []byte(`">`)); m != nil {
		r.ViewURL = strings.TrimSpace(string(m))
	}
	if m := extractBetween(data, []byte(`class="torrent_name"><a href="`), []byte(`</a>`)); m != nil {
		// 提取纯文本（去掉 HTML 标签）
		name := string(m)
		if idx := strings.Index(name, `">`); idx >= 0 {
			name = name[idx+2:]
		}
		r.Name = stripTags(strings.TrimSpace(name))
	}

	// 大小：<span class="torrent_size">134.06 MB</span>
	if m := extractBetween(data, []byte(`class="torrent_size">`), []byte(`</span>`)); m != nil {
		r.Size = parseSize(strings.TrimSpace(string(m)))
	}

	// 磁力链：<a href="magnet:?xt=urn:btih:...">
	if m := extractBetween(data, []byte(`href="magnet:?xt=urn:btih:`), []byte(`"`)); m != nil {
		hash := strings.TrimSpace(string(m))
		// 截断 tracker 参数
		if idx := strings.IndexByte(hash, '&'); idx >= 0 {
			hash = hash[:idx]
		}
		r.Hash = strings.ToLower(hash)
		// 重建完整 magnet
		r.Magnet = "magnet:?xt=urn:btih:" + hash
	}

	return r
}

func extractBetween(s, start, end []byte) []byte {
	i := bytes.Index(s, start)
	if i < 0 {
		return nil
	}
	s = s[i+len(start):]
	j := bytes.Index(s, end)
	if j < 0 {
		return nil
	}
	return s[:j]
}

func stripTags(s string) string {
	// 简单去 HTML 标签
	var out []byte
	inTag := false
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			inTag = true
		} else if s[i] == '>' {
			inTag = false
		} else if !inTag {
			out = append(out, s[i])
		}
	}
	return strings.TrimSpace(string(out))
}

func parseSize(s string) int64 {
	m := sizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	val, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	switch strings.ToUpper(m[2]) {
	case "KB":
		return int64(val * 1024)
	case "MB":
		return int64(val * 1024 * 1024)
	case "GB":
		return int64(val * 1024 * 1024 * 1024)
	case "TB":
		return int64(val * 1024 * 1024 * 1024 * 1024)
	}
	return 0
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

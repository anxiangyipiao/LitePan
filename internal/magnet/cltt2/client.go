// Package cltt2 实现 cltt2.shop（磁力天堂）的搜索客户端。
//
// 搜索 API：
//
//	POST /api/ssbc
//	Content-Type: application/x-www-form-urlencoded
//	key=<关键词>&from=<页码>&type=all
//
// 返回 JSON，hits 里直接包含 infohash_IK（40位hex），
// 可直接拼 magnet:?xt=urn:btih:<hash>&dn=<name>，无需 N+1 请求。
package cltt2

import (
	"context"
	"crypto/cipher"
	"crypto/des"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"litepan/internal/httpx"
)

const (
	defaultBaseURL = "https://cltt2.shop"
	pageSize       = 20
	maxResponseMB  = 2
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
	return &Client{
		baseURL: baseURL,
		http: httpx.NewClient(httpx.ClientOptions{
			Timeout: timeout,
			Proxy:   proxy,
		}),
	}
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("cltt2: 搜索关键词为空")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	payload, err := c.searchAPI(ctx, query)
	if err != nil {
		return nil, err
	}
	hits := payload.Data.Infos.Hits
	if len(hits) > limit {
		hits = hits[:limit]
	}
	results := make([]Result, 0, len(hits))
	for _, h := range hits {
		results = append(results, h.toResult(c.baseURL))
	}
	return results, nil
}

func (c *Client) searchAPI(ctx context.Context, query string) (*apiPayload, error) {
	desKey := desCBCEncrypt([]byte(query))
	b64 := base64Encode(desKey)
	form := url.Values{}
	form.Set("key", b64)
	form.Set("from", "1")
	form.Set("type", "all")

	u := c.baseURL + "/api/ssbc"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cltt2: 搜索请求失败：%w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxResponseMB)<<20))
	if err != nil {
		return nil, fmt.Errorf("cltt2: 读取搜索响应失败：%w", err)
	}

	var payload apiPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("cltt2: 搜索响应不是合法 JSON：%w", err)
	}
	if payload.Code != 200 {
		msg := payload.Msg
		if msg == "" {
			msg = string(body)
		}
		return nil, fmt.Errorf("cltt2: 搜索返回错误(%d)：%s", payload.Code, msg)
	}
	return &payload, nil
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

// --- JSON payload ---

type apiPayload struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Infos struct {
			Sum  int      `json:"sum"`
			Page int      `json:"page"`
			Hits []apiHit `json:"torrent"`
		} `json:"infos"`
	} `json:"data"`
}

type apiHit struct {
	ID_IK       string `json:"id_IK"`
	InfoHash_IK string `json:"infohash_IK"`
	Name_Simple string `json:"name_simple"`
	Size        string `json:"size"`
	Last_Seen   string `json:"last_seen"`
	Category    string `json:"category"`
}

func (h apiHit) toResult(baseURL string) Result {
	// infohash_IK 是40位hex BTIH
	hash := strings.TrimSpace(h.InfoHash_IK)
	hash = strings.ToLower(hash)

	magnet := "magnet:?xt=urn:btih:" + hash
	if name := strings.TrimSpace(h.Name_Simple); name != "" {
		magnet += "&dn=" + url.QueryEscape(name)
	}

	size, _ := strconv.ParseInt(h.Size, 10, 64)
	date, _ := parseDate(h.Last_Seen)

	return Result{
		Name:    h.Name_Simple,
		Size:    size,
		Date:    date,
		Hash:    hash,
		Magnet:  magnet,
		ViewURL: baseURL + "/info/" + h.ID_IK + ".html",
	}
}

func parseDate(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", s)
	}
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

// --- DES-CBC 加密 ---

var (
	desKeyBytes = []byte("12345678")
	desIVBytes  = []byte("12345678")
)

func desCBCEncrypt(plain []byte) []byte {
	pt := pkcs7Pad(plain, 8)
	block, _ := des.NewCipher(desKeyBytes)
	ct := make([]byte, len(pt))
	cipher.NewCBCEncrypter(block, desIVBytes).CryptBlocks(ct, pt)
	return ct
}

func pkcs7Pad(b []byte, blockSize int) []byte {
	padLen := blockSize - len(b)%blockSize
	for i := 0; i < padLen; i++ {
		b = append(b, byte(padLen))
	}
	return b
}

func base64Encode(data []byte) string {
	const encode = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var buf strings.Builder
	buf.Grow((len(data) + 2) / 3 * 4)
	for i := 0; i < len(data); i += 3 {
		var b [3]byte
		b[0] = data[i]
		if i+1 < len(data) {
			b[1] = data[i+1]
		}
		if i+2 < len(data) {
			b[2] = data[i+2]
		}
		buf.WriteByte(encode[(b[0]>>2)&0x3F])
		buf.WriteByte(encode[((b[0]&0x3)<<4)|(b[1]>>4)&0xF])
		if i+1 < len(data) {
			buf.WriteByte(encode[((b[1]&0xF)<<2)|(b[2]>>6)&0x3])
		} else {
			buf.WriteByte('=')
		}
		if i+2 < len(data) {
			buf.WriteByte(encode[b[2]&0x3F])
		} else {
			buf.WriteByte('=')
		}
	}
	return buf.String()
}

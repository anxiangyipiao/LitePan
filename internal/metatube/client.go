package metatube

import (
	"context"
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

// MetaTube 是一个自托管的媒体元数据聚合服务（metatube-community/metatube-server）。
// 本包实现其 REST v1 API（GET /v1/movies/search、GET /v1/movies/{provider}/{id}、
// GET /v1/images/{kind}/{provider}/{id} 等），并把返回数据翻译成与 TMDB 兼容的
// 通用 JSON 形状，供 STRM 刮削复用同一套下游逻辑。
//
// 注意：该 API 无结果时返回 HTTP 404 {"error":{"code":404,"message":"info not found"}}，
// 需要把"无候选"与真正的错误区分开。

const (
	defaultTimeout   = 60 * time.Second
	defaultSearchNum = 6
	mediaTypeMovie   = "movie"
	mediaTypeTV      = "tv"
)

type Options struct {
	BaseURL        string
	Timeout        time.Duration
	MaxRetries     int
	RetryBaseDelay time.Duration
}

type Client struct {
	baseURL        string
	http           *http.Client
	maxRetries     int
	retryBaseDelay time.Duration
}

func NewClient(opts Options) *Client {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
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
		baseURL:        strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/"),
		maxRetries:     maxRetries,
		retryBaseDelay: retryBaseDelay,
		http: httpx.NewClient(httpx.ClientOptions{
			Timeout: timeout,
		}),
	}
}

// Search 按 query（番号或标题）搜索影片，返回 TMDB 形状的命中列表。
// 命中按番号去重（多 provider 同番号取 score 最高者），并降序排列，避免同一部影片
// 被不同 provider 重复命中而误触发"存疑"状态。
func (c *Client) Search(ctx context.Context, query string, _ *int, _ string) ([]json.RawMessage, error) {
	if c == nil {
		return nil, fmt.Errorf("metatube: nil client")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("metatube: empty query")
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("limit", strconv.Itoa(defaultSearchNum))
	body, err := c.get(ctx, "/v1/movies/search", q)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	items := dedupeSearchItems(payload.Data)
	out := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		out = append(out, mustRaw(searchItemToTMDBShape(item)))
	}
	return out, nil
}

// Lookup 按存储的 ID 解析一部影片并返回其 TMDB 形状的完整详情。
// ID 通常就是番号（NFO 的 tmdbid）；MetaTube REST 没有按番号直接查详情的端点，
// 因此先搜索再匹配番号 / provider id，命中后拉详情。
func (c *Client) Lookup(ctx context.Context, id string, mediaType string) (json.RawMessage, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("metatube: empty id")
	}
	raws, err := c.Search(ctx, id, nil, mediaType)
	if err != nil {
		return nil, err
	}
	for _, raw := range raws {
		m, ok := rawMap(raw)
		if !ok {
			continue
		}
		if strings.TrimSpace(anyString(m["_metatube_number"])) == id ||
			strings.TrimSpace(anyString(m["_metatube_id"])) == id {
			return c.EnrichSearchResult(ctx, raw, mediaType)
		}
	}
	return nil, fmt.Errorf("metatube: 未找到 ID %s", id)
}

// EnrichSearchResult 用详情端点补齐搜索命中的 summary / genres / director / actors /
// maker / runtime 等字段，并把 poster_path 换成 provider/id 以便经图片端点下载海报。
func (c *Client) EnrichSearchResult(ctx context.Context, raw json.RawMessage, mediaType string) (json.RawMessage, error) {
	m, ok := rawMap(raw)
	if !ok {
		return raw, nil
	}
	provider := strings.TrimSpace(anyString(m["_metatube_provider"]))
	metaID := strings.TrimSpace(anyString(m["_metatube_id"]))
	if provider == "" || metaID == "" {
		// 命中里没有 provider/id，无法拉详情，原样返回
		return raw, nil
	}
	body, err := c.get(ctx, "/v1/movies/"+url.PathEscape(provider)+"/"+url.PathEscape(metaID), nil)
	if err != nil {
		// 详情失败时降级为搜索命中（已有标题/缩略图）
		return raw, nil
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Data == nil {
		return raw, nil
	}
	return mustRaw(detailToTMDBShape(m, payload.Data)), nil
}

// FetchTVSeasons 仅电影源，剧集返回空列表。
func (c *Client) FetchTVSeasons(ctx context.Context, showID string) ([]json.RawMessage, error) {
	return []json.RawMessage{}, nil
}

// FetchTVSeason 仅电影源，剧集不支持。
func (c *Client) FetchTVSeason(ctx context.Context, showID string, season int) (json.RawMessage, error) {
	return nil, fmt.Errorf("metatube: 不支持剧集刮削")
}

// FetchMovieBackdrops 仅电影源，MetaTube 无背景图数据，返回空。
func (c *Client) FetchMovieBackdrops(ctx context.Context, id string) ([]string, error) {
	return nil, nil
}

// DownloadImage 下载图片。imagePath 为绝对 URL 时直接下载；为 "backdrop/provider/id" 时经
// /v1/images/backdrop 下载背景图；为 provider/id 时经 /v1/images/primary 下载海报。
// size 参数被忽略（MetaTube 图片端点无尺寸裁剪）。
func (c *Client) DownloadImage(ctx context.Context, imagePath, size string) ([]byte, error) {
	imagePath = strings.TrimSpace(imagePath)
	if imagePath == "" {
		return nil, fmt.Errorf("metatube: empty image path")
	}
	endpoint := imagePath
	if !strings.Contains(imagePath, "://") {
		switch {
		case strings.HasPrefix(imagePath, "backdrop/"):
			endpoint = "/v1/images/backdrop/" + strings.TrimPrefix(imagePath, "backdrop/")
		case strings.Contains(imagePath, "/"):
			endpoint = "/v1/images/primary/" + strings.TrimPrefix(imagePath, "/")
		default:
			return nil, fmt.Errorf("metatube: invalid image path %q", imagePath)
		}
	}
	return c.getBytes(ctx, endpoint)
}

// ValidateConnection 连通性测试：探活服务根路径（快速，不触发慢的 provider 搜索）。
func (c *Client) ValidateConnection(ctx context.Context) bool {
	if c == nil || c.baseURL == "" {
		return false
	}
	body, err := c.getRetry(ctx, "/", nil)
	if err != nil {
		return false
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return false
	}
	return len(envelope.Data) > 0
}

// get 请求 JSON。MetaTube 无候选时返回 404 {"error":{"code":404,"message":"info not found"}}，
// 这里把它规整为 data=[] 的空结果，而非硬错误。
func (c *Client) get(ctx context.Context, endpoint string, query url.Values) ([]byte, error) {
	body, err := c.getRetry(ctx, endpoint, query)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("metatube: 无法解析响应：%w", err)
	}
	if envelope.Error != nil {
		if envelope.Error.Code == http.StatusNotFound && strings.Contains(strings.ToLower(envelope.Error.Message), "info not found") {
			return json.Marshal(map[string]any{"data": []any{}})
		}
		return nil, fmt.Errorf("metatube: %s", envelope.Error.Message)
	}
	if envelope.Data == nil || string(envelope.Data) == "null" {
		return json.Marshal(map[string]any{"data": []any{}})
	}
	// 规整为带 data 字段的包络，调用方按 {data: ...} 解析
	return json.Marshal(map[string]any{"data": envelope.Data})
}

func (c *Client) getRetry(ctx context.Context, endpoint string, query url.Values) ([]byte, error) {
	rawURL := c.baseURL + endpoint
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		resp, body, err := httpx.DoJSON(ctx, c.http, http.MethodGet, rawURL, query, nil, nil, 1<<20)
		if err == nil && resp != nil && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			return body, nil
		}
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		// 404 属"无候选"，把响应体交给上层 get 解析成空结果
		if err == nil && status == http.StatusNotFound {
			return body, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("metatube: http status %d", status)
		}
		if attempt == c.maxRetries || ctx.Err() != nil || (err == nil && !isRetryableHTTPStatus(status)) {
			return nil, retryExhausted(lastErr, attempt)
		}
		if err := waitRetry(ctx, retryDelay(c.retryBaseDelay, attempt)); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) getBytes(ctx context.Context, endpoint string) ([]byte, error) {
	rawURL := endpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		rawURL = c.baseURL + endpoint
	}
	client := c.http
	if client == nil {
		client = http.DefaultClient
	}
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt == c.maxRetries || ctx.Err() != nil {
				return nil, retryExhausted(lastErr, attempt)
			}
			if err := waitRetry(ctx, retryDelay(c.retryBaseDelay, attempt)); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			status := resp.StatusCode
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("metatube: image http status %d", status)
			if attempt == c.maxRetries || !isRetryableHTTPStatus(status) {
				return nil, retryExhausted(lastErr, attempt)
			}
			if err := waitRetry(ctx, retryDelay(c.retryBaseDelay, attempt)); err != nil {
				return nil, err
			}
			continue
		}
		const maxImage = 8 << 20
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxImage+1))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt == c.maxRetries || ctx.Err() != nil {
				return nil, retryExhausted(lastErr, attempt)
			}
			if err := waitRetry(ctx, retryDelay(c.retryBaseDelay, attempt)); err != nil {
				return nil, err
			}
			continue
		}
		if len(data) > maxImage {
			return nil, fmt.Errorf("metatube: image too large")
		}
		return data, nil
	}
	return nil, lastErr
}

// dedupeSearchItems 按番号去重，同番号取 score 最高者；按 score 降序。
func dedupeSearchItems(items []map[string]any) []map[string]any {
	best := map[string]map[string]any{}
	order := make([]string, 0, len(items))
	for _, item := range items {
		number := strings.ToLower(strings.TrimSpace(anyString(item["number"])))
		if number == "" {
			number = strings.ToLower(strings.TrimSpace(anyString(item["id"])))
		}
		if number == "" {
			continue
		}
		if prev, ok := best[number]; ok && floatOf(item["score"]) <= floatOf(prev["score"]) {
			continue
		}
		if _, ok := best[number]; !ok {
			order = append(order, number)
		}
		best[number] = item
	}
	// 按 score 降序，保证高评分 provider 结果在前
	ordered := make([]map[string]any, 0, len(order))
	scoreSort := make([]map[string]any, 0, len(order))
	for _, number := range order {
		scoreSort = append(scoreSort, best[number])
	}
	for i := 1; i < len(scoreSort); i++ {
		for j := i; j > 0 && floatOf(scoreSort[j]["score"]) > floatOf(scoreSort[j-1]["score"]); j-- {
			scoreSort[j], scoreSort[j-1] = scoreSort[j-1], scoreSort[j]
		}
	}
	ordered = append(ordered, scoreSort...)
	return ordered
}

// searchItemToTMDBShape 把搜索命中翻译成 TMDB 兼容形状。
// id=番号、original_title=番号（保证番号查询能被标题匹配逻辑命中）、
// title=「番号 标题」（海报墙/搜索结果都能看到番号，按番号可检索）、poster_path=缩略图 URL（前端可直接渲染）。
func searchItemToTMDBShape(item map[string]any) map[string]any {
	number := strings.TrimSpace(anyString(item["number"]))
	if number == "" {
		number = strings.TrimSpace(anyString(item["id"]))
	}
	metaID := strings.TrimSpace(anyString(item["id"]))
	if metaID == "" {
		metaID = number
	}
	out := map[string]any{
		"id":              number,
		"title":           javDisplayTitle(number, strings.TrimSpace(anyString(item["title"]))),
		"original_title":  number,
		"release_date":    strings.TrimSpace(anyString(item["release_date"])),
		"poster_path":     strings.TrimSpace(anyString(item["thumb_url"])),
		"media_type":      mediaTypeMovie,
		"_metatube_number": number,
		"_metatube_id":    metaID,
		"_metatube_provider": strings.TrimSpace(anyString(item["provider"])),
	}
	if actors := stringListField(item["actors"]); len(actors) > 0 {
		out["actors"] = actors
	}
	return out
}

// detailToTMDBShape 把详情数据合并进 TMDB 形状的命中，补齐刮削所需字段。
// poster_path 换成 provider/id 路径，供 DownloadImage 经 /v1/images/primary 下载海报。
func detailToTMDBShape(hit map[string]any, detail map[string]any) map[string]any {
	out := make(map[string]any, len(hit)+8)
	for k, v := range hit {
		out[k] = v
	}
	number := strings.TrimSpace(anyString(detail["number"]))
	if number == "" {
		number = strings.TrimSpace(anyString(hit["_metatube_number"]))
	}
	metaID := strings.TrimSpace(anyString(detail["id"]))
	if metaID == "" {
		metaID = strings.TrimSpace(anyString(hit["_metatube_id"]))
	}
	provider := strings.TrimSpace(anyString(detail["provider"]))
	if provider == "" {
		provider = strings.TrimSpace(anyString(hit["_metatube_provider"]))
	}
	if number != "" {
		out["id"] = number
		out["_metatube_number"] = number
		out["original_title"] = number
	}
	if metaID != "" {
		out["_metatube_id"] = metaID
	}
	if provider != "" && metaID != "" {
		out["_metatube_provider"] = provider
		out["poster_path"] = provider + "/" + metaID
		// backdrop_path 用哨兵路径，DownloadImage 会路由到 /v1/images/backdrop/{provider}/{id}。
		out["backdrop_path"] = "backdrop/" + provider + "/" + metaID
	}
	if t := strings.TrimSpace(anyString(detail["title"])); t != "" {
		out["title"] = javDisplayTitle(number, t)
	}
	if s := strings.TrimSpace(anyString(detail["summary"])); s != "" {
		out["overview"] = s
	}
	if d := strings.TrimSpace(anyString(detail["release_date"])); d != "" {
		out["release_date"] = d
	}
	if maker := strings.TrimSpace(anyString(detail["maker"])); maker != "" {
		out["studio"] = maker
	}
	if d := strings.TrimSpace(anyString(detail["director"])); d != "" {
		out["director"] = d
	}
	if genres := stringListField(detail["genres"]); len(genres) > 0 {
		out["genres"] = genres
	}
	// 预览截图（多为竖图）作为多张轮播背景图的素材，写进 _metatube_preview_images 供刮削侧读取。
	if imgs := stringListField(detail["preview_images"]); len(imgs) > 0 {
		out["_metatube_preview_images"] = imgs
	}
	if actors := stringListField(detail["actors"]); len(actors) > 0 {
		out["actors"] = actors
	}
	if n := intOf(detail["runtime"]); n > 0 {
		out["runtime"] = n
	}
	return out
}

// javDisplayTitle 把 JAV 标题统一成「番号 标题」：海报墙/搜索结果都能看到番号，且按番号可检索。
func javDisplayTitle(number, title string) string {
	number = strings.TrimSpace(number)
	title = strings.TrimSpace(title)
	if number == "" {
		return title
	}
	if title == "" {
		return number
	}
	if strings.HasPrefix(strings.ToLower(title), strings.ToLower(number)) {
		return title
	}
	return number + " " + title
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

func retryExhausted(err error, retries int) error {
	if err == nil || retries <= 0 {
		return err
	}
	return fmt.Errorf("%w（已重试 %d 次）", err, retries)
}

func mustRaw(m map[string]any) json.RawMessage {
	b, _ := json.Marshal(m)
	return b
}

func rawMap(raw json.RawMessage) (map[string]any, bool) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}
	return m, true
}

func anyString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func floatOf(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f
	default:
		return 0
	}
}

func intOf(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
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

// stringListField 兼容字符串数组与 [{"name": ...}] 两种形状。
func stringListField(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range arr {
		if s := strings.TrimSpace(anyString(item)); s != "" && s != "<nil>" && s != "map[]" {
			out = append(out, s)
			continue
		}
		if m, ok := item.(map[string]any); ok {
			if n := strings.TrimSpace(anyString(m["name"])); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

package tmdb

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

const (
	baseURL          = "https://api.themoviedb.org/3"
	defaultTimeout   = 10 * time.Second
	maxSearchResults = 10
	mediaTypeMovie   = "movie"
	mediaTypeTV      = "tv"
	mediaTypeAuto    = "auto"
)

type ProxyConfig struct {
	Enabled  bool
	URL      string
	Username string
	Password string
}

type Client struct {
	apiKey         string
	language       string
	http           *http.Client
	maxRetries     int
	retryBaseDelay time.Duration
}

type Options struct {
	APIKey         string
	Language       string
	ProxyURL       string
	Timeout        time.Duration
	MaxRetries     int
	RetryBaseDelay time.Duration
}

func NewClient(opts Options) *Client {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	var proxy func(*http.Request) (*url.URL, error)
	if u := strings.TrimSpace(opts.ProxyURL); u != "" {
		parsed, err := url.Parse(u)
		if err == nil {
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
		apiKey:         strings.TrimSpace(opts.APIKey),
		language:       strings.TrimSpace(opts.Language),
		maxRetries:     maxRetries,
		retryBaseDelay: retryBaseDelay,
		http: httpx.NewClient(httpx.ClientOptions{
			Timeout: timeout,
			Proxy:   proxy,
		}),
	}
}

func BuildProxyURL(cfg ProxyConfig) string {
	if !cfg.Enabled {
		return ""
	}
	raw := strings.TrimSpace(cfg.URL)
	if raw == "" {
		return ""
	}
	user := strings.TrimSpace(cfg.Username)
	pwd := strings.TrimSpace(cfg.Password)
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

func (c *Client) ValidateConnection(ctx context.Context) bool {
	if c == nil || c.apiKey == "" {
		return false
	}
	_, err := c.searchMovie(ctx, "test", nil)
	return err == nil
}

func (c *Client) Search(ctx context.Context, query string, year *int, mediaType string) ([]json.RawMessage, error) {
	if c == nil || c.apiKey == "" {
		return nil, fmt.Errorf("tmdb: missing api key")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("tmdb: empty query")
	}
	normalized := strings.ToLower(strings.TrimSpace(mediaType))
	if normalized == "" {
		normalized = mediaTypeMovie
	}
	switch normalized {
	case mediaTypeTV:
		return c.searchTV(ctx, query, year)
	case mediaTypeMovie:
		return c.searchMovie(ctx, query, year)
	case mediaTypeAuto:
		results, err := c.searchMovie(ctx, query, year)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return results, nil
		}
		return c.searchTV(ctx, query, year)
	default:
		return nil, fmt.Errorf("tmdb: unsupported media type %q", mediaType)
	}
}

func (c *Client) Lookup(ctx context.Context, tmdbID string, mediaType string) (json.RawMessage, error) {
	if c == nil || c.apiKey == "" {
		return nil, fmt.Errorf("tmdb: missing api key")
	}
	id, err := strconv.Atoi(strings.TrimSpace(tmdbID))
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("tmdb: invalid id %q", tmdbID)
	}
	normalized := strings.ToLower(strings.TrimSpace(mediaType))
	if normalized == "" {
		normalized = mediaTypeMovie
	}
	switch normalized {
	case mediaTypeTV:
		return c.lookupTV(ctx, id)
	case mediaTypeMovie:
		return c.lookupMovie(ctx, id)
	default:
		return nil, fmt.Errorf("tmdb: unsupported media type %q", mediaType)
	}
}

func (c *Client) FetchTVSeasons(ctx context.Context, tmdbID string) ([]json.RawMessage, error) {
	info, err := c.Lookup(ctx, tmdbID, mediaTypeTV)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Seasons []json.RawMessage `json:"seasons"`
	}
	if err := json.Unmarshal(info, &payload); err != nil {
		return nil, err
	}
	if payload.Seasons == nil {
		return []json.RawMessage{}, nil
	}
	return payload.Seasons, nil
}

// FetchTVSeason 拉取单季详情（含 episodes 列表与 still_path）。
func (c *Client) FetchTVSeason(ctx context.Context, tmdbID string, seasonNumber int) (json.RawMessage, error) {
	if c == nil || c.apiKey == "" {
		return nil, fmt.Errorf("tmdb: missing api key")
	}
	id, err := strconv.Atoi(strings.TrimSpace(tmdbID))
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("tmdb: invalid id %q", tmdbID)
	}
	if seasonNumber < 0 {
		return nil, fmt.Errorf("tmdb: invalid season %d", seasonNumber)
	}
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	if c.language != "" {
		q.Set("language", c.language)
	}
	return c.get(ctx, fmt.Sprintf("%s/tv/%d/season/%d", baseURL, id, seasonNumber), q)
}

// FetchMovieImages 拉取电影图片集（backdrops/posters/logos），返回原始 JSON 由调用方解析。
func (c *Client) FetchMovieImages(ctx context.Context, tmdbID string) (json.RawMessage, error) {
	if c == nil || c.apiKey == "" {
		return nil, fmt.Errorf("tmdb: missing api key")
	}
	id, err := strconv.Atoi(strings.TrimSpace(tmdbID))
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("tmdb: invalid id %q", tmdbID)
	}
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	if c.language != "" {
		q.Set("language", c.language)
	}
	return c.get(ctx, fmt.Sprintf("%s/movie/%d/images", baseURL, id), q)
}

func (c *Client) searchMovie(ctx context.Context, query string, year *int) ([]json.RawMessage, error) {
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	q.Set("query", query)
	if c.language != "" {
		q.Set("language", c.language)
	}
	if year != nil && *year > 0 {
		q.Set("year", strconv.Itoa(*year))
	}
	return c.search(ctx, baseURL+"/search/movie", q)
}

func (c *Client) searchTV(ctx context.Context, query string, year *int) ([]json.RawMessage, error) {
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	q.Set("query", query)
	if c.language != "" {
		q.Set("language", c.language)
	}
	if year != nil && *year > 0 {
		q.Set("first_air_date_year", strconv.Itoa(*year))
	}
	return c.search(ctx, baseURL+"/search/tv", q)
}

func (c *Client) lookupMovie(ctx context.Context, id int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	if c.language != "" {
		q.Set("language", c.language)
	}
	return c.get(ctx, fmt.Sprintf("%s/movie/%d", baseURL, id), q)
}

func (c *Client) lookupTV(ctx context.Context, id int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	if c.language != "" {
		q.Set("language", c.language)
	}
	return c.get(ctx, fmt.Sprintf("%s/tv/%d", baseURL, id), q)
}

func (c *Client) search(ctx context.Context, endpoint string, query url.Values) ([]json.RawMessage, error) {
	body, err := c.get(ctx, endpoint, query)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if len(payload.Results) > maxSearchResults {
		payload.Results = payload.Results[:maxSearchResults]
	}
	if payload.Results == nil {
		return []json.RawMessage{}, nil
	}
	return payload.Results, nil
}

func (c *Client) get(ctx context.Context, endpoint string, query url.Values) (json.RawMessage, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		resp, body, err := httpx.DoJSON(ctx, c.http, http.MethodGet, endpoint, query, nil, nil, 1<<20)
		if err == nil && resp != nil && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			return json.RawMessage(body), nil
		}
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("tmdb: http status %d", status)
		}
		if attempt == c.maxRetries || ctx.Err() != nil || (err == nil && !isRetryableHTTPStatus(status)) {
			return nil, retryExhausted(lastErr, attempt)
		}
		if err := waitRetry(ctx, retryDelay(c.retryBaseDelay, attempt, resp)); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

const imageBaseURL = "https://image.tmdb.org/t/p"

// DownloadImage 下载 TMDB 图片。posterPath 形如 "/abc.jpg"；size 常用 w500 / original。
func (c *Client) DownloadImage(ctx context.Context, posterPath, size string) ([]byte, error) {
	posterPath = strings.TrimSpace(posterPath)
	if posterPath == "" {
		return nil, fmt.Errorf("tmdb: empty poster path")
	}
	if !strings.HasPrefix(posterPath, "/") {
		posterPath = "/" + posterPath
	}
	size = strings.TrimSpace(size)
	if size == "" {
		size = "w500"
	}
	endpoint := imageBaseURL + "/" + size + posterPath
	client := c.http
	if client == nil {
		client = http.DefaultClient
	}
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt == c.maxRetries || ctx.Err() != nil {
				return nil, retryExhausted(lastErr, attempt)
			}
			if err := waitRetry(ctx, retryDelay(c.retryBaseDelay, attempt, nil)); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			status := resp.StatusCode
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("tmdb: image http status %d", status)
			if attempt == c.maxRetries || !isRetryableHTTPStatus(status) {
				return nil, retryExhausted(lastErr, attempt)
			}
			if err := waitRetry(ctx, retryDelay(c.retryBaseDelay, attempt, resp)); err != nil {
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
			if err := waitRetry(ctx, retryDelay(c.retryBaseDelay, attempt, nil)); err != nil {
				return nil, err
			}
			continue
		}
		if len(data) > maxImage {
			return nil, fmt.Errorf("tmdb: image too large")
		}
		return data, nil
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

func retryDelay(base time.Duration, attempt int, resp *http.Response) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	delay := base * time.Duration(attempt*2+1)
	if resp != nil {
		if seconds, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && seconds >= 0 {
			delay = time.Duration(seconds) * time.Second
		} else if at, err := http.ParseTime(resp.Header.Get("Retry-After")); err == nil {
			if serverDelay := time.Until(at); serverDelay > 0 {
				delay = serverDelay
			}
		}
	}
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

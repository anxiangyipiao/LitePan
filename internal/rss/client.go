package rss

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

// Client 抓取 RSS/Atom 订阅源，支持代理（复用磁力搜索的代理设置）。
type Client struct {
	http       *http.Client
	maxRetries int
	retryBase  time.Duration
}

func NewClient(proxyURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	var proxy func(*http.Request) (*url.URL, error)
	if u := strings.TrimSpace(proxyURL); u != "" {
		if parsed, err := url.Parse(u); err == nil {
			proxy = http.ProxyURL(parsed)
		}
	}
	return &Client{
		maxRetries: 2,
		retryBase:  time.Second,
		http: httpx.NewClient(httpx.ClientOptions{
			Timeout: timeout,
			Proxy:   proxy,
		}),
	}
}

// Fetch 抓取订阅源内容，限制体积并重试 429/502/503/504。
func (c *Client) Fetch(ctx context.Context, feedURL string) ([]byte, error) {
	feedURL = strings.TrimSpace(feedURL)
	if feedURL == "" {
		return nil, fmt.Errorf("订阅地址为空")
	}
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9, */*;q=0.8")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
		resp, err := c.http.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes+1))
			_ = resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else if len(data) > maxFeedBytes {
				return nil, fmt.Errorf("订阅源过大（>5MB）")
			} else {
				return data, nil
			}
			if attempt == c.maxRetries || ctx.Err() != nil {
				return nil, lastErr
			}
			if err := waitRetry(ctx, retryDelay(c.retryBase, attempt)); err != nil {
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
			lastErr = fmt.Errorf("http 状态码 %d", status)
		}
		if attempt == c.maxRetries || ctx.Err() != nil || (err == nil && !retryableStatus(status)) {
			return nil, lastErr
		}
		if err := waitRetry(ctx, retryDelay(c.retryBase, attempt)); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func retryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func retryDelay(base time.Duration, attempt int) time.Duration {
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

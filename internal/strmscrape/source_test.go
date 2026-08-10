package strmscrape

import (
	"context"
	"encoding/json"
	"testing"

	"litepan/internal/mediaorganize/tmdb"
)

func TestParseMovieBackdrops(t *testing.T) {
	raw := json.RawMessage(`{"backdrops":[{"file_path":"/a.jpg"},{"file_path":"/b.jpg"}]}`)
	got := parseMovieBackdrops(raw)
	if len(got) != 2 || got[0] != "/a.jpg" || got[1] != "/b.jpg" {
		t.Fatalf("parseMovieBackdrops = %v", got)
	}
}

func TestParseMovieBackdropsEmptyAndMalformed(t *testing.T) {
	if got := parseMovieBackdrops(json.RawMessage(`{"backdrops":[]}`)); len(got) != 0 {
		t.Fatalf("空 backdrops = %v", got)
	}
	if got := parseMovieBackdrops(json.RawMessage(`not-json`)); got != nil {
		t.Fatalf("坏 JSON 应返回 nil, got=%v", got)
	}
	// 空 file_path 被过滤
	got := parseMovieBackdrops(json.RawMessage(`{"backdrops":[{"file_path":""},{"file_path":"/c.jpg"}]}`))
	if len(got) != 1 || got[0] != "/c.jpg" {
		t.Fatalf("空 file_path 过滤后 = %v", got)
	}
}

func TestTMDBScrapeSourceFetchMovieBackdropsMissingKey(t *testing.T) {
	src := tmdbScrapeSource{client: tmdb.NewClient(tmdb.Options{})}
	if _, err := src.FetchMovieBackdrops(context.Background(), "1"); err == nil {
		t.Fatal("未配置 API Key 应返回错误")
	}
}

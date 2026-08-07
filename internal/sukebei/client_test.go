package sukebei

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchParsesResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("q"); got != "ABC-123" {
			t.Errorf("q = %q, want ABC-123", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"id": float64(42), "name": "ABC-123 测试资源", "category": "1_0",
					"size": float64(1073741824), "date": float64(1700000000),
					"seeders": float64(15), "leechers": float64(3), "downloads": float64(1200),
					"hash": "abcd1234", "magnet": "magnet:?xt=urn:btih:abcd1234",
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL, MaxRetries: 0})
	results, err := c.Search(context.Background(), "ABC-123", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	got := results[0]
	if got.Name != "ABC-123 测试资源" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.ID != 42 || got.Size != 1073741824 || got.Date != 1700000000 {
		t.Errorf("numeric fields wrong: %+v", got)
	}
	if got.Seeders != 15 || got.Leechers != 3 || got.Downloads != 1200 {
		t.Errorf("stats wrong: %+v", got)
	}
	if got.Magnet != "magnet:?xt=urn:btih:abcd1234" || got.Hash != "abcd1234" {
		t.Errorf("magnet/hash wrong: %+v", got)
	}
	if want := srv.URL + "/view/42"; got.ViewURL != want {
		t.Errorf("ViewURL = %q, want %q", got.ViewURL, want)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	c := NewClient(Options{MaxRetries: 0})
	if _, err := c.Search(context.Background(), "   ", 5); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestBuildProxyURL(t *testing.T) {
	if got := BuildProxyURL("http://127.0.0.1:7890", "user", "pass"); got != "http://user:pass@127.0.0.1:7890" {
		t.Errorf("BuildProxyURL = %q", got)
	}
	if got := BuildProxyURL("http://127.0.0.1:7890", "", ""); got != "http://127.0.0.1:7890" {
		t.Errorf("BuildProxyURL no auth = %q", got)
	}
	if got := BuildProxyURL("  ", "u", "p"); got != "" {
		t.Errorf("BuildProxyURL empty = %q", got)
	}
}

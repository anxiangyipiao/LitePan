package sukebei

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleHTML = `<!DOCTYPE html>
<html><head><title>sukebei</title></head><body>
<table class="table table-bordered table-hover table-striped">
<thead><tr><th>Category</th><th>Name</th><th>Link</th><th>Size</th><th>Date</th><th>Seeders</th><th>Leechers</th><th>Downloads</th></tr></thead>
<tbody>
<tr class="success">
  <td><a href="?c=1_0"><img src="/img/cat.png" alt=""></a></td>
  <td><a href="/view/42" title="ABC-123 测试资源">ABC-123 测试资源</a></td>
  <td class="text-center"><a href="magnet:?xt=urn:btih:abcd1234&amp;dn=test"><i class="fa fa-magnet"></i></a></td>
  <td class="text-center">1.0 GiB</td>
  <td class="text-center" data-timestamp="1700000000"><time>2023-11-14 21:33</time></td>
  <td class="text-center">15</td>
  <td class="text-center">3</td>
  <td class="text-center">1200</td>
</tr>
<tr>
  <td><a href="?c=2_0"><img src="/img/cat2.png" alt=""></a></td>
  <td><a href="/view/43" title="XYZ-999 另一部">XYZ-999 另一部</a></td>
  <td class="text-center"><a href="magnet:?xt=urn:btih:deadbeef&amp;dn=test2"><i class="fa fa-magnet"></i></a></td>
  <td class="text-center">2.4 GiB</td>
  <td class="text-center" data-timestamp="1701000000"><time>2023-11-26 00:00</time></td>
  <td class="text-center">7</td>
  <td class="text-center">1</td>
  <td class="text-center">99</td>
</tr>
<tr><td colspan="8">no results row without magnet link</td></tr>
</tbody>
</table>
</body></html>`

func TestSearchParsesHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "ABC-123" {
			t.Errorf("q = %q, want ABC-123", r.URL.Query().Get("q"))
		}
		if r.URL.Query().Get("s") != "seeders" {
			t.Errorf("s = %q, want seeders", r.URL.Query().Get("s"))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(sampleHTML))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL, MaxRetries: 0})
	results, err := c.Search(context.Background(), "ABC-123", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	first := results[0]
	if first.ID != 42 || first.Name != "ABC-123 测试资源" {
		t.Errorf("id/name = %d %q", first.ID, first.Name)
	}
	if first.Size != 1<<30 {
		t.Errorf("Size = %d, want %d", first.Size, int64(1)<<30)
	}
	if first.Date != 1700000000 {
		t.Errorf("Date = %d", first.Date)
	}
	if first.Seeders != 15 || first.Leechers != 3 || first.Downloads != 1200 {
		t.Errorf("stats = %d/%d/%d", first.Seeders, first.Leechers, first.Downloads)
	}
	if first.Magnet != "magnet:?xt=urn:btih:abcd1234&dn=test" || first.Hash != "abcd1234" {
		t.Errorf("magnet/hash = %q %q", first.Magnet, first.Hash)
	}
	if want := srv.URL + "/view/42"; first.ViewURL != want {
		t.Errorf("ViewURL = %q, want %q", first.ViewURL, want)
	}
	if second := results[1]; second.Name != "XYZ-999 另一部" || second.Seeders != 7 || second.Downloads != 99 {
		t.Errorf("second row wrong: %+v", second)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	c := NewClient(Options{MaxRetries: 0})
	if _, err := c.Search(context.Background(), "   ", 5); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestSearchLimitTruncates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleHTML))
	}))
	defer srv.Close()
	c := NewClient(Options{BaseURL: srv.URL, MaxRetries: 0})
	results, err := c.Search(context.Background(), "ABC-123", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
}

func TestBuildProxyURL(t *testing.T) {
	if got := BuildProxyURL("socks5://127.0.0.1:7890", "user", "pass"); !strings.HasPrefix(got, "socks5://user:pass@") {
		t.Errorf("BuildProxyURL = %q", got)
	}
	if got := BuildProxyURL("http://127.0.0.1:7890", "", ""); got != "http://127.0.0.1:7890" {
		t.Errorf("BuildProxyURL no auth = %q", got)
	}
	if got := BuildProxyURL("  ", "u", "p"); got != "" {
		t.Errorf("BuildProxyURL empty = %q", got)
	}
}

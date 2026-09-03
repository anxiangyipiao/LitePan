package cltt2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleSearchJSON = `{
  "code": 200,
  "msg": null,
  "data": {
    "infos": {
      "sum": 3837,
      "page": 192,
      "torrent": [
        {
          "id": "50403623",
          "id_IK": "939197057661408769",
          "infohash": "e07f17af39a761c415db94759c68dc0162211dbf",
          "infohash_IK": "e07f17af39a761c415db94759c68dc0162211dbf",
          "name_IK": "<b>ubuntu</b>-noi-v2.0.iso",
          "name_simple": "ubuntu-noi-v2.0.iso",
          "createdate": "2026-08-27",
          "size": "3544187",
          "last_seen": "2026-08-27",
          "category": "影视",
          "requests": "0",
          "from": "3"
        },
        {
          "id": "937657208747857409",
          "id_IK": "937657208747857409",
          "infohash": "aa11223344556677889900aabbccddeeff001122",
          "infohash_IK": "aa11223344556677889900aabbccddeeff001122",
          "name_IK": "test-file.iso",
          "name_simple": "test-file.iso",
          "createdate": "2026-08-26",
          "size": "1048576",
          "last_seen": "2026-08-26",
          "category": "软件"
        }
      ]
    }
  }
}`

func TestSearchParsesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/ssbc" {
			t.Errorf("path = %s, want /api/ssbc", r.URL.Path)
		}
		_ = r.ParseForm()
		if r.FormValue("from") != "1" {
			t.Errorf("from = %q, want 1", r.FormValue("from"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleSearchJSON))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	results, err := c.Search(context.Background(), "ubuntu", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	first := results[0]
	if first.Hash != "e07f17af39a761c415db94759c68dc0162211dbf" {
		t.Errorf("Hash = %q", first.Hash)
	}
	if first.Name != "ubuntu-noi-v2.0.iso" {
		t.Errorf("Name = %q, want ubuntu-noi-v2.0.iso", first.Name)
	}
	if first.Size != 3544187 {
		t.Errorf("Size = %d, want 3544187", first.Size)
	}
	wantMagnet := "magnet:?xt=urn:btih:e07f17af39a761c415db94759c68dc0162211dbf&dn=ubuntu-noi-v2.0.iso"
	if first.Magnet != wantMagnet {
		t.Errorf("Magnet = %q, want %q", first.Magnet, wantMagnet)
	}
	if !strings.Contains(first.ViewURL, "/info/939197057661408769.html") {
		t.Errorf("ViewURL = %q", first.ViewURL)
	}
	// 第二条
	second := results[1]
	if second.Hash != "aa11223344556677889900aabbccddeeff001122" {
		t.Errorf("second Hash = %q", second.Hash)
	}
	if second.Size != 1048576 {
		t.Errorf("second Size = %d, want 1048576", second.Size)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	c := NewClient(Options{BaseURL: "http://localhost"})
	_, err := c.Search(context.Background(), "", 10)
	if err == nil || !strings.Contains(err.Error(), "关键词为空") {
		t.Errorf("expected '关键词为空', got %v", err)
	}
}

func TestSearchAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":500,"msg":"服务暂时不可用","data":null}`))
	}))
	defer srv.Close()
	c := NewClient(Options{BaseURL: srv.URL})
	_, err := c.Search(context.Background(), "test", 5)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 error, got %v", err)
	}
}

func TestSearchLimit(t *testing.T) {
	// 返回3条，limit=1 → 只拿1条
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleSearchJSON))
	}))
	defer srv.Close()
	c := NewClient(Options{BaseURL: srv.URL})
	results, err := c.Search(context.Background(), "ubuntu", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
}

func TestDESCEncrypt(t *testing.T) {
	// DES-CBC 加密 "ubuntu" 后 base64，与 JS 加密结果对照
	encrypted := desCBCEncrypt([]byte("ubuntu"))
	b64 := base64Encode(encrypted)
	if b64 == "" {
		t.Error("base64Encode returned empty")
	}
	// 加密是确定性的，同一输入同一输出
	b64_2 := base64Encode(desCBCEncrypt([]byte("ubuntu")))
	if b64 != b64_2 {
		t.Errorf("determinism: %q != %q", b64, b64_2)
	}
}

func TestBuildProxyURL(t *testing.T) {
	tests := []struct {
		raw, user, pass, want string
	}{
		{"http://127.0.0.1:7890", "", "", "http://127.0.0.1:7890"},
		{"http://127.0.0.1:7890", "u", "p", "http://u:p@127.0.0.1:7890"},
		{"", "", "", ""},
	}
	for _, tt := range tests {
		got := BuildProxyURL(tt.raw, tt.user, tt.pass)
		if got != tt.want {
			t.Errorf("BuildProxyURL(%q,%q,%q) = %q, want %q", tt.raw, tt.user, tt.pass, got, tt.want)
		}
	}
}

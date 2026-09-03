package btkitty

import (
	"encoding/base64"
	"net/url"
	"testing"
)

func TestDecodeSearchBody(t *testing.T) {
	// 模拟 btkitty POST 响应中的 base64 内容
	inner := url.QueryEscape(`<table><tr><td class="name"><a href="/info/ABC123">Test Movie</a></td><td class="size">1.5 GB</td></tr></table>`)
	b64 := base64.StdEncoding.EncodeToString([]byte(inner))
	body := []byte(`<script>document.getElementById("search-box").innerHTML=decodeURIComponent(escape(atob("` + b64 + `")));</script>`)

	decoded, err := decodeSearchBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) == 0 {
		t.Fatal("decoded is empty")
	}
	// 检查解码后包含预期内容
	if !contains(string(decoded), "ABC123") {
		t.Errorf("decoded does not contain ABC123: %s", string(decoded)[:200])
	}
}

func TestDecodeSearchBody_NoMatch(t *testing.T) {
	body := []byte(`<html>no atob here</html>`)
	_, err := decodeSearchBody(body)
	if err == nil {
		t.Error("expected error")
	}
}

func TestExtractMagnet(t *testing.T) {
	html := []byte(`<textarea id="thread_share_text">magnet:?xt=urn:btih:ABCDEF1234567890abcdef1234567890abcdef12</textarea>`)
	m := extractMagnet(html)
	if m != "magnet:?xt=urn:btih:ABCDEF1234567890abcdef1234567890abcdef12" {
		t.Errorf("got %q", m)
	}
}

func TestExtractMagnet_Input(t *testing.T) {
	html := []byte(`<input id="mag-link" value="magnet:?xt=urn:btih:ABCDEF1234567890abcdef1234567890abcdef12">`)
	m := extractMagnet(html)
	if m != "magnet:?xt=urn:btih:ABCDEF1234567890abcdef1234567890abcdef12" {
		t.Errorf("got %q", m)
	}
}

func TestParseSearchHTML(t *testing.T) {
	html := []byte(`<table id="archiveResult"><tbody>
<tr><td class="name"><a href="/info/NlgPYKbFD38r9">肖申克的救赎.mp4</a></td><td class="size">581.08 MB</td></tr>
<tr><td class="name"><a href="/info/B1r65q8f4ybwM">肖申克的救赎</a></td><td class="size">900.69 MB</td></tr>
</tbody></table>`)
	items, err := parseSearchHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].shortID != "NlgPYKbFD38r9" {
		t.Errorf("shortID = %q", items[0].shortID)
	}
	if items[0].title != "肖申克的救赎.mp4" {
		t.Errorf("title = %q", items[0].title)
	}
	if items[0].size != "581.08 MB" {
		t.Errorf("size = %q", items[0].size)
	}
}

func TestMagnetHash(t *testing.T) {
	m := magnetHash("magnet:?xt=urn:btih:abcdef1234567890&dn=test")
	if m != "ABCDEF1234567890" {
		t.Errorf("got %q", m)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

package cilibao

import (
	"testing"
)

func TestExtractMagnet(t *testing.T) {
	html := []byte(`<textarea class="textarea" onfocus="this.select()">magnet:?xt=urn:btih:d3a0e767ac518bf6d19435b4b57b84a82b249833</textarea>`)
	m := extractMagnet(html)
	if m != "magnet:?xt=urn:btih:d3a0e767ac518bf6d19435b4b57b84a82b249833" {
		t.Errorf("got %q", m)
	}
}

func TestExtractMagnet_Link(t *testing.T) {
	html := []byte(`<a href="magnet:?xt=urn:btih:ABCDEF1234567890" class="download">打开链接</a>`)
	m := extractMagnet(html)
	if m != "magnet:?xt=urn:btih:ABCDEF1234567890" {
		t.Errorf("got %q", m)
	}
}

func TestExtractMagnet_NoMatch(t *testing.T) {
	if extractMagnet([]byte(`<div>no magnet</div>`)) != "" {
		t.Error("expected empty")
	}
}

func TestParseSearchHTML(t *testing.T) {
	html := []byte(`<div class="search-item">
<div class="item-title"><h3><a href="/detail/6eEEwV">长安的荔枝27-28</a></h3></div>
<div class="item-bar"><span>创建时间： <b>2025-06-20</b></span><span>文件大小：<b>919.87 Mb</b></span></div>
</div>`)
	items, err := parseSearchHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	it := items[0]
	if it.shortID != "6eEEwV" {
		t.Errorf("shortID = %q", it.shortID)
	}
	if it.title != "长安的荔枝27-28" {
		t.Errorf("title = %q", it.title)
	}
	if it.date != "2025-06-20" {
		t.Errorf("date = %q", it.date)
	}
	if it.size != "919.87 Mb" {
		t.Errorf("size = %q", it.size)
	}
}

func TestMagnetHash(t *testing.T) {
	m := magnetHash("magnet:?xt=urn:btih:abcdef1234567890&dn=test")
	if m != "ABCDEF1234567890" {
		t.Errorf("got %q", m)
	}
}

func TestParseSizeStr(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"919.87 Mb", 964352000},
		{"2.1 Gb", 2254857830},
		{"15.48 Gb", 16621486080},
	}
	for _, tt := range tests {
		got := parseSizeStr(tt.input)
		diff := got - tt.want
		if diff < 0 {
			diff = -diff
		}
		if tt.want > 0 && diff > tt.want/50 {
			t.Errorf("parseSizeStr(%q) = %d, want ~%d", tt.input, got, tt.want)
		}
	}
}

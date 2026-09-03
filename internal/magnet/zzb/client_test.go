package zzb

import (
	"testing"
)

func TestExtractMagnet(t *testing.T) {
	html := []byte(`<textarea id="magnetLink" class="magnet-link well">magnet:?xt=urn:btih:D245F92A81B3D4401C67E3009D598B7B0C4C766C</textarea>`)
	m := extractMagnet(html)
	if m != "magnet:?xt=urn:btih:D245F92A81B3D4401C67E3009D598B7B0C4C766C" {
		t.Errorf("got %q", m)
	}
}

func TestExtractMagnet_Comment(t *testing.T) {
	html := []byte(`<!--d245f92a81b3d4401c67e3009d598b7b0c4c766c,length:154216208-->`)
	m := extractMagnet(html)
	if m != "magnet:?xt=urn:btih:D245F92A81B3D4401C67E3009D598B7B0C4C766C" {
		t.Errorf("got %q", m)
	}
}

func TestExtractMagnet_NoMatch(t *testing.T) {
	html := []byte(`<div>no magnet</div>`)
	if extractMagnet(html) != "" {
		t.Error("expected empty")
	}
}

func TestParseSearchHTML(t *testing.T) {
	html := []byte(`<li class="media"><div class="media-body">
<h4 class="media-heading"><a href="/seed/NjR5D4" title="Test Movie">Test Movie</a></h4>
<div class="search-info">日期：<span class="s_b">2014-04-15</span>大小：<span class="s_b">147.07 MB</span>点击：<span class="s_b">16</span></div>
</div></li>`)
	items, err := parseSearchHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	it := items[0]
	if it.shortID != "NjR5D4" {
		t.Errorf("shortID = %q", it.shortID)
	}
	if it.title != "Test Movie" {
		t.Errorf("title = %q", it.title)
	}
	if it.size != "147.07 MB" {
		t.Errorf("size = %q", it.size)
	}
	if it.date != "2014-04-15" {
		t.Errorf("date = %q", it.date)
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
		{"147.07 MB", 154216208},
		{"2.46 GB", 2640902863},
		{"18.86 GB", 20253024256},
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

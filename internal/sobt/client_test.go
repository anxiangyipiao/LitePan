package sobt

import (
	"testing"
)

func TestParseSearchHTML(t *testing.T) {
	html := []byte(`<div class="search-item">
  <div class="item-title"><h3><a href="/torrent/79832136d8a2b1d25a27d3a8316ce2f7727e6c21.html">【王牌特工：特工学院】【TS-RMVB.中字】</a></h3></div>
  <div class="item-bar">
    <span>创建时间： <b>2015-04-22</b></span>
    <span>文件大小：<b >737.31 MB</b></span>
    <span>下载热度：<b>779</b></span>
  </div>
</div>`)
	results, err := parseSearchHTML(html, "https://sobt10.vip")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	r := results[0]
	if r.Hash != "79832136D8A2B1D25A27D3A8316CE2F7727E6C21" {
		t.Errorf("hash = %q", r.Hash)
	}
	if r.Magnet != "magnet:?xt=urn:btih:79832136D8A2B1D25A27D3A8316CE2F7727E6C21" {
		t.Errorf("magnet = %q", r.Magnet)
	}
	if r.Name != "【王牌特工：特工学院】【TS-RMVB.中字】" {
		t.Errorf("name = %q", r.Name)
	}
	if r.Size != 773125570 {
		t.Errorf("size = %d, want ~773125570", r.Size)
	}
	if r.Date != 1429660800 {
		t.Errorf("date = %d, want 1429660800", r.Date)
	}
}

func TestParseSizeStr(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"737.31 MB", 773125570},
		{"2 GB", 2147483648},
		{"4.4 GB", 4724464025},
		{"1.29 GB", 1384120320},
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

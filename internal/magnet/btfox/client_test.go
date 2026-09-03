package btfox

import (
	"testing"
)

func TestExtractMagnet(t *testing.T) {
	html := []byte(`<textarea id="thread_share_text" rows="2" class="w_100" readonly="">
magnet:?xt=urn:btih:0b0c73257adc43432b00db7b8f78805242a29436</textarea>`)
	magnet := extractMagnet(html)
	expected := "magnet:?xt=urn:btih:0b0c73257adc43432b00db7b8f78805242a29436"
	if magnet != expected {
		t.Errorf("got %q, want %q", magnet, expected)
	}
}

func TestExtractMagnet_Input(t *testing.T) {
	html := []byte(`<input id="mag-link" value="magnet:?xt=urn:btih:ABCDEF1234567890abcdef1234567890abcdef12">`)
	magnet := extractMagnet(html)
	if magnet != "magnet:?xt=urn:btih:ABCDEF1234567890abcdef1234567890abcdef12" {
		t.Errorf("got %q", magnet)
	}
}

func TestExtractMagnet_NoMatch(t *testing.T) {
	html := []byte(`<div>no magnet here</div>`)
	if extractMagnet(html) != "" {
		t.Error("expected empty")
	}
}

func TestParseSearchHTML(t *testing.T) {
	html := []byte(`<div class="item"><div class="box_line"><div class="threadlist_content">
<div class="threadlist_subject"><div class="thread_check"><div>
<a href="/info/1yY8NW" title="肖申克的救赎.mp4">肖申克的救赎.mp4</a>
</div></div></div>
<div class="threadlist_note">.mp4&nbsp;<span class="gray">length：</span>581.08 MB&nbsp;<span class="gray">date：</span>2026-09-02</div>
</div></div></div>`)
	items, err := parseSearchHTML(html)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	it := items[0]
	if it.shortID != "1yY8NW" {
		t.Errorf("shortID = %q", it.shortID)
	}
	if it.title != "肖申克的救赎.mp4" {
		t.Errorf("title = %q", it.title)
	}
	if it.size != "581.08 MB" {
		t.Errorf("size = %q", it.size)
	}
	if it.date != "2026-09-02" {
		t.Errorf("date = %q", it.date)
	}
}

func TestMagnetHash(t *testing.T) {
	magnet := "magnet:?xt=urn:btih:0b0c73257adc43432b00db7b8f78805242a29436&dn=test"
	hash := magnetHash(magnet)
	if hash != "0B0C73257ADC43432B00DB7B8F78805242A29436" {
		t.Errorf("got %q", hash)
	}
}

func TestParseSizeStr(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"581.08 MB", 609246889},
		{"8.18 GB", 8781829120},
		{"1.04 GB", 1115820032},
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

package clm64

import (
	"encoding/base64"
	"net/url"
	"testing"
)

func TestDecodeBody(t *testing.T) {
	inner := url.QueryEscape(`<div>test <a href="magnet:?xt=urn:btih:ABCDEF1234567890">magnet</a></div>`)
	b64 := base64.StdEncoding.EncodeToString([]byte(inner))
	body := []byte(`<script>document.getElementById("search-box").innerHTML=decodeURIComponent(window.atob("` + b64 + `"));</script>`)
	decoded, err := decodeBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) == 0 {
		t.Fatal("empty")
	}
}

func TestExtractMagnetFromDecoded(t *testing.T) {
	html := []byte(`<a href="magnet:?xt=urn:btih:26123C7AB45825A0A2485316135F870E13B595A6" class="magnet">magnet</a>`)
	m := extractMagnetFromDecoded(html)
	if m != "magnet:?xt=urn:btih:26123C7AB45825A0A2485316135F870E13B595A6" {
		t.Errorf("got %q", m)
	}
}

func TestExtractMagnetFromDecoded_NoMatch(t *testing.T) {
	if extractMagnetFromDecoded([]byte(`<div>no magnet</div>`)) != "" {
		t.Error("expected empty")
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
		{"1.04 GB", 1115820032},
		{"13.16 GB", 14136199168},
		{"5.01 GB", 5378527232},
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

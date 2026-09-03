package seedhub

import (
	"testing"
)

func TestExtractMagnetFromBody(t *testing.T) {
	// 模拟 /link_start/ 页面内容
	html := []byte(`<script>
	const data = "bWFnbmV0Oj94dD11cm46YnRpaDo4NzQ2ODMxNzJiM2E2ZGQyMjQ0OWNlMzQ1MTIxYzdiNWIwNzQ4ZmQ2";
	$("#thunder").click(function(e) {e.preventDefault();thunderLink.newTask({tasks: [{url: window.atob(data)}]});return false;});
	</script>`)
	magnet := extractMagnetFromBody(html)
	expected := "magnet:?xt=urn:btih:874683172b3a6dd22449ce345121c7b5b0748fd6"
	if magnet != expected {
		t.Errorf("got %q, want %q", magnet, expected)
	}
}

func TestExtractMagnetFromBody_NoMatch(t *testing.T) {
	html := []byte(`<script>var x = "not a magnet";</script>`)
	magnet := extractMagnetFromBody(html)
	if magnet != "" {
		t.Errorf("expected empty, got %q", magnet)
	}
}

func TestMagnetHash(t *testing.T) {
	magnet := "magnet:?xt=urn:btih:874683172b3a6dd22449ce345121c7b5b0748fd6&dn=test"
	hash := magnetHash(magnet)
	if hash != "874683172b3a6dd22449ce345121c7b5b0748fd6" {
		t.Errorf("got %q", hash)
	}
}

func TestParseSizeStr(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"6.94G", 7451771858},
		{"14.61GB", 15687368048},
		{"40.41G", 43389896547},
		{"3.34G", 3586297293},
		{"12G", 12884901888},
		{"1.28G", 1374389534},
	}
	for _, tt := range tests {
		got := parseSizeStr(tt.input)
		diff := got - tt.want
		if diff < 0 {
			diff = -diff
		}
		if tt.want > 0 && diff > tt.want/100 {
			t.Errorf("parseSizeStr(%q) = %d, want ~%d", tt.input, got, tt.want)
		}
	}
}

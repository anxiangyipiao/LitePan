package security

import (
	"net/http/httptest"
	"testing"
)

// 三种真实场景：
// 1. web 端 https 域名：Origin=https → Secure=true（正常登录保持）
// 2. 飞牛 app 应用入口 http 内网：Origin=http，反代却带 X-Forwarded-Proto:https
//    → 应返回 false，否则 http 下 WebView 拒绝保存 Secure cookie 导致登录失效
// 3. 纯后端 API（无 Origin/Referer）：回退 X-Forwarded-Proto/连接判断
func TestSecureCookieFrontendScheme(t *testing.T) {
	cases := []struct {
		name      string
		origin    string
		referer   string
		xproto    string
		wantSecure bool
	}{
		{"web https origin", "https://litepan.anxiangyi.fnos.net", "", "https", true},
		{"fnos app http origin + https xproto", "http://192.168.1.10:5211", "", "https", false},
		{"fnos app http referer + https xproto", "", "http://192.168.1.10:5211/admin", "https", false},
		{"https via referer", "", "https://litepan.example.com/login", "https", true},
		{"no frontend header, xproto https", "", "", "https", true},
		{"no frontend header, xproto http", "", "", "http", false},
		{"no frontend header, no xproto", "", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://127.0.0.1:5211/api/auth/login", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			if tc.xproto != "" {
				req.Header.Set("X-Forwarded-Proto", tc.xproto)
			}
			if got := SecureCookie(req); got != tc.wantSecure {
				t.Fatalf("SecureCookie() = %v, want %v", got, tc.wantSecure)
			}
		})
	}
}

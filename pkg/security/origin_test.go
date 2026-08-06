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

// 飞牛 app 应用入口场景：反代带 X-Forwarded-Proto:https，但前端真实走 http 内网。
// 此时登录 cookie 已按前端真实协议(http)处理，写请求的 Origin 校验也必须放行同源。
func TestRequestOriginAllowedFrontendScheme(t *testing.T) {
	cases := []struct {
		name      string
		host      string
		xhost     string
		origin    string
		referer   string
		wantAllow bool
	}{
		{
			name: "fnos app http origin + same host, https xproto",
			host: "192.168.1.10:5211",
			origin: "http://192.168.1.10:5211",
			wantAllow: true,
		},
		{
			name: "fnos app http referer + same host",
			host: "192.168.1.10:5211",
			referer: "http://192.168.1.10:5211/admin",
			wantAllow: true,
		},
		{
			name: "same host via x-forwarded-host",
			host: "10.0.0.8:5211",
			xhost: "litepan.anxiangyi.fnos.net",
			origin: "https://litepan.anxiangyi.fnos.net",
			wantAllow: true,
		},
		{
			name: "http origin + https proto mismatch, same host still allowed",
			host: "192.168.1.10:5211",
			origin: "http://192.168.1.10:5211",
			wantAllow: true,
		},
		{
			name: "cross-site host denied",
			host: "192.168.1.10:5211",
			origin: "https://evil.example.com",
			wantAllow: false,
		},
		{
			name: "same host different port denied",
			host: "192.168.1.10:5211",
			origin: "http://192.168.1.10:9999",
			wantAllow: false,
		},
		{
			name: "no origin header allowed",
			host: "192.168.1.10:5211",
			wantAllow: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "http://"+tc.host+"/api/admin/settings", nil)
			req.Host = tc.host
			if tc.xhost != "" {
				req.Header.Set("X-Forwarded-Host", tc.xhost)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			if got := RequestOriginAllowed(req, nil); got != tc.wantAllow {
				t.Fatalf("RequestOriginAllowed() = %v, want %v", got, tc.wantAllow)
			}
		})
	}
}

// 反代改写 Host 头的场景（如飞牛 app 经 FN Connect 隧道访问）：
// r.Host 与 X-Forwarded-Host 都是公网域名，但浏览器 Origin 是 http 内网入口。
// 仅靠主机比对无法识别，需靠登录时记录的会话可信来源兜底。
func TestRequestOriginAllowedTrustedOrigin(t *testing.T) {
	cases := []struct {
		name      string
		host      string
		xhost     string
		origin    string
		trusted   string
		wantAllow bool
	}{
		{
			name:      "host rewritten to public domain, origin is app internal entry",
			host:      "litepan.anxiangyi.fnos.net",
			xhost:     "litepan.anxiangyi.fnos.net",
			origin:    "http://192.168.1.10:5211",
			trusted:   "192.168.1.10:5211",
			wantAllow: true,
		},
		{
			name:      "no trusted origin recorded",
			host:      "litepan.anxiangyi.fnos.net",
			xhost:     "litepan.anxiangyi.fnos.net",
			origin:    "http://192.168.1.10:5211",
			trusted:   "",
			wantAllow: false,
		},
		{
			name:      "cross-site origin still denied with trusted host present",
			host:      "litepan.anxiangyi.fnos.net",
			xhost:     "litepan.anxiangyi.fnos.net",
			origin:    "https://evil.example.com",
			trusted:   "192.168.1.10:5211",
			wantAllow: false,
		},
		{
			name:      "trusted origin host matches https public origin",
			host:      "litepan.anxiangyi.fnos.net",
			xhost:     "litepan.anxiangyi.fnos.net",
			origin:    "https://litepan.anxiangyi.fnos.net",
			trusted:   "litepan.anxiangyi.fnos.net",
			wantAllow: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "https://"+tc.host+"/api/admin/strm/scrape/run", nil)
			req.Host = tc.host
			if tc.xhost != "" {
				req.Header.Set("X-Forwarded-Host", tc.xhost)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if got := RequestOriginAllowed(req, nil, tc.trusted); got != tc.wantAllow {
				t.Fatalf("RequestOriginAllowed(trusted=%q) = %v, want %v", tc.trusted, got, tc.wantAllow)
			}
		})
	}
}

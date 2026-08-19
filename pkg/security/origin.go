package security

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

var defaultDevCORSOrigins = []string{
	"http://127.0.0.1:5211",
	"http://localhost:5211",
	"http://127.0.0.1:5173",
	"http://localhost:5173",
	"http://[::1]:5211",
	"http://[::1]:5173",
}

func AllowedCORSOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("LITEPAN_CORS_ORIGINS"))
	if raw == "" {
		return append([]string(nil), defaultDevCORSOrigins...)
	}
	raw = strings.NewReplacer(";", ",", "\n", ",").Replace(raw)
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		normalized := normalizeOrigin(item)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

// RequestOriginHost 返回请求来源的主机（含端口）：优先 Origin，其次 Referer，最后回退到
// 反代推导的 BaseURL 主机。登录时用它记录前端真实来源，供后续写请求的来源校验使用——
// 反代可能改写 Host 头，但浏览器 Origin/Referer 反映的是前端实际地址（如飞牛 app 的 http 内网入口）。
func RequestOriginHost(r *http.Request) string {
	origin := requestOrigin(r)
	if origin == "" {
		origin = requestBaseURL(r)
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}

func RequestOriginAllowed(r *http.Request, allowed []string, trustedOriginHost ...string) bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("LITEPAN_DISABLE_CSRF_CHECK")), "true") {
		return true
	}
	requestOrigin := requestOrigin(r)
	if requestOrigin == "" {
		return true
	}
	if originHostMatches(r, requestOrigin) {
		return true
	}
	if len(trustedOriginHost) > 0 {
		trusted := strings.TrimSpace(trustedOriginHost[0])
		if trusted != "" {
			if u, err := url.Parse(requestOrigin); err == nil && u.Host != "" &&
				strings.EqualFold(u.Host, trusted) {
				return true
			}
		}
	}
	if requestOrigin == normalizeOrigin(requestBaseURL(r)) {
		return true
	}
	if len(allowed) == 0 {
		allowed = AllowedCORSOrigins()
	}
	for _, item := range allowed {
		if requestOrigin == normalizeOrigin(item) {
			return true
		}
	}
	return false
}

// originHostMatches 判断请求来源（Origin/Referer）的主机（含端口）是否与后端实际承载主机一致，
// 且不比较协议。原因：FN Connect 等反代对外 https、对内转发 http 并注入 X-Forwarded-Proto:https，
// 飞牛 app 应用入口走 http 内网时 Origin 协议是 http，若同时要求协议一致会把同源写请求误判为
// 不可信来源。CSRF 防护关注的是跨站，主机一致即同站；协议差异无法由后端感知，交给 Secure cookie 判定。
func originHostMatches(r *http.Request, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	for _, candidate := range []string{
		strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]),
		strings.TrimSpace(r.Host),
	} {
		if candidate != "" && strings.EqualFold(candidate, host) {
			return true
		}
	}
	return false
}

func requestOrigin(r *http.Request) string {
	if origin := normalizeOrigin(r.Header.Get("Origin")); origin != "" {
		return origin
	}
	return normalizeOrigin(r.Header.Get("Referer"))
}

func RequestBaseURL(r *http.Request) string {
	return requestBaseURL(r)
}

func requestBaseURL(r *http.Request) string {
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	host := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0])
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	scheme := proto
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	if host == "" {
		return strings.ToLower(strings.TrimSuffix(r.URL.String(), r.URL.RequestURI()))
	}
	return strings.ToLower(scheme + "://" + host)
}

func normalizeOrigin(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	u, err := url.Parse(text)
	if err == nil && u.Scheme != "" && u.Host != "" {
		return strings.ToLower(u.Scheme + "://" + u.Host)
	}
	return strings.ToLower(strings.TrimSuffix(text, "/"))
}

// SecureCookie 决定登录 cookie 是否带 Secure 标志。
// 优先用浏览器真实协议（Origin/Referer）判定：前端是 https 才种 Secure cookie，
// 避免「反代带 https 头但实际前端走 http 内网」时（如飞牛 app 应用入口）种下
// Secure cookie 导致 http 环境下 WebView 拒绝保存、登录失效。无反代头时回退到连接本身。
func SecureCookie(r *http.Request) bool {
	if front := frontendScheme(r); front != "" {
		return strings.EqualFold(front, "https")
	}
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if strings.EqualFold(proto, "https") {
		return true
	}
	if strings.EqualFold(proto, "http") {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(r.Host))
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "[::1]") {
		return false
	}
	return proto != ""
}

// frontendScheme 从浏览器侧头推导真实前端协议，避免信任仅由反代注入的 X-Forwarded-Proto。
func frontendScheme(r *http.Request) string {
	for _, name := range []string{"Origin", "Referer"} {
		raw := strings.TrimSpace(r.Header.Get(name))
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" {
			continue
		}
		return strings.ToLower(u.Scheme)
	}
	return ""
}

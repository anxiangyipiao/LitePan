package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"litepan/internal/cltt2"
	"litepan/internal/domain"
	"litepan/internal/seedhub"
	"litepan/internal/settings"
	"litepan/internal/sobt"
	"litepan/internal/sukebei"
)

// magnetSearchSiteDTO 是 /magnet-search/sites 的返回项，前端按此渲染 tab 栏。
type magnetSearchSiteDTO struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

// magnetSearchSites 返回用户配置的站点 URL 列表，前端直接渲染 tab。
func (h *Handler) magnetSearchSites(w http.ResponseWriter, r *http.Request) {
	sites := h.magnetSites()
	out := make([]magnetSearchSiteDTO, 0, len(sites))
	for _, u := range sites {
		out = append(out, magnetSearchSiteDTO{URL: u, Label: siteLabel(u)})
	}
	writeOK(w, out)
}

// magnetSearch 按前端 tab 选定的站点抓取。?site=<url> 直接用 URL。
func (h *Handler) magnetSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeErr(w, domain.Errorf(domain.CodeValidation, "请输入搜索关键词"))
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}

	proxy := sukebei.BuildProxyURL(
		h.settingString(settings.KeyMagnetSearchProxyURL),
		h.settingString(settings.KeyMagnetSearchProxyUsername),
		h.settingString(settings.KeyMagnetSearchProxyPassword),
	)

	siteURL := strings.TrimSpace(r.URL.Query().Get("site"))
	if siteURL == "" {
		sites := h.magnetSites()
		if len(sites) > 0 {
			siteURL = sites[0]
		}
	}
	if siteURL == "" {
		writeErr(w, domain.Errorf(domain.CodeValidation, "未配置任何磁力搜索站点"))
		return
	}

	if strings.Contains(siteURL, "sobt") {
		sc := sobt.NewClient(sobt.Options{BaseURL: siteURL, ProxyURL: proxy})
		results, err := sc.Search(r.Context(), query, limit)
		if err != nil {
			h.log.Warn("磁力搜索失败", "q", query, "site", siteURL, "err", err)
			writeErr(w, domain.Errorf(domain.CodeDriverError, "磁力搜索失败：%v", err))
			return
		}
		writeOK(w, results)
		return
	}
	if strings.Contains(siteURL, "seedhub") {
		sc := seedhub.NewClient(seedhub.Options{BaseURL: siteURL, ProxyURL: proxy})
		results, err := sc.Search(r.Context(), query, limit)
		if err != nil {
			h.log.Warn("磁力搜索失败", "q", query, "site", siteURL, "err", err)
			writeErr(w, domain.Errorf(domain.CodeDriverError, "磁力搜索失败：%v", err))
			return
		}
		writeOK(w, results)
		return
	}
	if strings.Contains(siteURL, "cltt2") {
		cc := cltt2.NewClient(cltt2.Options{BaseURL: siteURL, ProxyURL: proxy})
		results, err := cc.Search(r.Context(), query, limit)
		if err != nil {
			h.log.Warn("磁力搜索失败", "q", query, "site", siteURL, "err", err)
			writeErr(w, domain.Errorf(domain.CodeDriverError, "磁力搜索失败：%v", err))
			return
		}
		writeOK(w, results)
		return
	}
	c := sukebei.NewClient(sukebei.Options{BaseURL: siteURL, ProxyURL: proxy})
	results, serr := c.Search(r.Context(), query, limit)
	if serr != nil {
		h.log.Warn("磁力搜索失败", "q", query, "site", siteURL, "err", serr)
		writeErr(w, domain.Errorf(domain.CodeDriverError, "磁力搜索失败：%v", serr))
		return
	}
	writeOK(w, results)
}

// magnetSites 从 settings 读取站点 URL 列表。
func (h *Handler) magnetSites() []string {
	raw := strings.TrimSpace(h.settingString(settings.KeyMagnetSearchSites))
	if raw == "" {
		return nil
	}
	var urls []string
	if err := json.Unmarshal([]byte(raw), &urls); err != nil {
		return nil
	}
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		out = append(out, u)
	}
	return out
}

// siteLabel 从 URL 提取 host 作为显示名。
func siteLabel(raw string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return raw
	}
	return s
}

func (h *Handler) settingString(key string) string {
	if h.settings == nil {
		return ""
	}
	return h.settings.String(key)
}

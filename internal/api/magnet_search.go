package api

import (
	"net/http"
	"strconv"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/settings"
	"litepan/internal/sukebei"
)

// magnetSearch 代理 sukebei.nyaa.si 的磁力搜索。站点地址与代理均来自系统设置。
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
	client := sukebei.NewClient(sukebei.Options{
		BaseURL: h.magnetSearchBaseURL(),
		ProxyURL: sukebei.BuildProxyURL(
			h.settingString(settings.KeyMagnetSearchProxyURL),
			h.settingString(settings.KeyMagnetSearchProxyUsername),
			h.settingString(settings.KeyMagnetSearchProxyPassword),
		),
	})
	results, err := client.Search(r.Context(), query, limit)
	if err != nil {
		h.log.Warn("磁力搜索失败", "q", query, "err", err)
		writeErr(w, domain.Errorf(domain.CodeDriverError, "磁力搜索失败：%v", err))
		return
	}
	if results == nil {
		results = []sukebei.Result{}
	}
	writeOK(w, results)
}

func (h *Handler) magnetSearchBaseURL() string {
	raw := strings.TrimSpace(h.settingString(settings.KeyMagnetSearchBaseURL))
	if raw == "" {
		return "https://sukebei.nyaa.si"
	}
	return raw
}

func (h *Handler) settingString(key string) string {
	if h.settings == nil {
		return ""
	}
	return h.settings.String(key)
}

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/magnetsearch"
	"litepan/internal/settings"
	"litepan/internal/sukebei"
)

// magnetSearchSiteDTO 描述一个镜像，前端用于渲染站点多选 UI。
type magnetSearchSiteDTO struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	BaseURL string `json:"base_url"`
	Enabled bool   `json:"enabled"`
}

// magnetSearchSites 返回内置 + 用户自定义镜像清单，并附当前是否启用。
// 前端用此接口渲染多选 UI。
func (h *Handler) magnetSearchSites(w http.ResponseWriter, r *http.Request) {
	custom := parseCustomSites(h.settingString(settings.KeyMagnetSearchCustomSites))
	enabled := magnetsearch.ParseEnabledSites(h.settingString(settings.KeyMagnetSearchEnabledSites))
	out := make([]magnetSearchSiteDTO, 0, len(magnetsearch.Builtin)+len(custom))
	for _, s := range magnetsearch.Builtin {
		out = append(out, magnetSearchSiteDTO{
			ID:      s.ID,
			Label:   s.Label,
			BaseURL: s.BaseURL,
			Enabled: siteEnabled(enabled, s.ID),
		})
	}
	for _, raw := range custom {
		out = append(out, magnetSearchSiteDTO{
			ID:      "custom:" + raw,
			Label:   customSiteLabel(raw),
			BaseURL: raw,
			Enabled: siteEnabled(enabled, "custom:"+raw),
		})
	}
	writeOK(w, out)
}

// customSiteLabel 把自定义 base_url 简化为 host 部分作为 label。
func customSiteLabel(raw string) string {
	label := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(raw), "https://"), "http://")
	if i := strings.IndexByte(label, '/'); i >= 0 {
		label = label[:i]
	}
	if label == "" {
		return "custom"
	}
	return label
}

// siteEnabled 兼容 enabled == nil（全启用）情况。
func siteEnabled(enabled map[string]struct{}, id string) bool {
	if enabled == nil {
		return true
	}
	_, ok := enabled[id]
	return ok
}

// parseCustomSites 解析自定义镜像 JSON 数组；空 / 失败返回 nil。
func parseCustomSites(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

// magnetSearch 并发抓取所有启用镜像的搜索结果，按 infohash 去重后按 seeders 排序。
// 旧 KeyMagnetSearchBaseURL 作为单站兜底：若用户没启用任何新站点（清空了 enabled list），
// 回退到 BaseURL 设置里的单站，向后兼容老配置。
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

	sites := h.enabledMagnetSites()
	if len(sites) == 0 {
		// 兜底：旧配置的 BaseURL
		if base := h.magnetSearchBaseURL(); base != "" {
			sites = []magnetsearch.Site{{ID: "legacy", Label: "Legacy", BaseURL: base}}
		}
	}
	if len(sites) == 0 {
		writeErr(w, domain.Errorf(domain.CodeValidation, "未启用任何磁力搜索镜像"))
		return
	}

	results, err := magnetsearch.Search(r.Context(), sites, proxy, query, limit, magnetsearch.Options{})
	if err != nil {
		h.log.Warn("磁力搜索失败", "q", query, "err", err, "sites", siteIDs(sites))
		writeErr(w, domain.Errorf(domain.CodeDriverError, "磁力搜索失败：%v", err))
		return
	}
	if results == nil {
		results = []magnetsearch.Result{}
	}
	writeOK(w, results)
}

// enabledMagnetSites 解析 settings 中的启用列表 + 自定义镜像，返回实际要抓的站点。
// 解析失败时（无 key / JSON 损坏）按"全启用"处理，保持向后兼容。
func (h *Handler) enabledMagnetSites() []magnetsearch.Site {
	enabled := magnetsearch.ParseEnabledSites(h.settingString(settings.KeyMagnetSearchEnabledSites))
	custom := parseCustomSites(h.settingString(settings.KeyMagnetSearchCustomSites))
	return magnetsearch.EnabledSites(enabled, custom)
}

func siteIDs(sites []magnetsearch.Site) []string {
	out := make([]string, 0, len(sites))
	for _, s := range sites {
		out = append(out, s.ID)
	}
	return out
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

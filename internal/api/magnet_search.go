package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/settings"
	"litepan/internal/sukebei"
)

// magnetSite 描述一个 nyaa/sukebei 系镜像（HTML 表格格式）的元数据。
// 不同镜像的 HTML 结构与 sukebei.nyaa.si 一致，因此共用 sukebei 解析器。
type magnetSite struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	BaseURL string `json:"base_url"`
}

// magnetBuiltin 内置镜像清单。顺序即前端 tab 默认顺序。
var magnetBuiltin = []magnetSite{
	{ID: "sukebei", Label: "Sukebei", BaseURL: "https://sukebei.nyaa.si"},
	{ID: "nyaa", Label: "Nyaa", BaseURL: "https://nyaa.net"},
	{ID: "sukebei_cn", Label: "Sukebei CN", BaseURL: "https://sukebei.cn.nyaa.net"},
}

// magnetResult 是单站搜索返回的一条结果，嵌入 sukebei.Result 复用其 JSON tag。
// Source 字段记录该条来自哪个镜像 id（前端单站模式时仅展示，但保留以备未来扩展）。
type magnetResult struct {
	sukebei.Result
	Source string `json:"source"`
}

// magnetSearchSiteDTO 是 /magnet-search/sites 的返回项。
type magnetSearchSiteDTO struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	BaseURL string `json:"base_url"`
	Enabled bool   `json:"enabled"`
}

// magnetSearchSites 返回内置 + 用户自定义镜像清单，附当前是否启用。
// 前端按此渲染 tab 栏（只显示 enabled=true 的）。
func (h *Handler) magnetSearchSites(w http.ResponseWriter, r *http.Request) {
	custom := parseMagnetCustomSites(h.settingString(settings.KeyMagnetSearchCustomSites))
	enabled := parseMagnetEnabledSites(h.settingString(settings.KeyMagnetSearchEnabledSites))
	out := make([]magnetSearchSiteDTO, 0, len(magnetBuiltin)+len(custom))
	for _, s := range magnetBuiltin {
		out = append(out, magnetSearchSiteDTO{
			ID:      s.ID,
			Label:   s.Label,
			BaseURL: s.BaseURL,
			Enabled: magnetSiteEnabled(enabled, s.ID),
		})
	}
	for _, raw := range custom {
		out = append(out, magnetSearchSiteDTO{
			ID:      "custom:" + raw,
			Label:   customSiteLabel(raw),
			BaseURL: raw,
			Enabled: magnetSiteEnabled(enabled, "custom:"+raw),
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

// magnetSiteEnabled 兼容 enabled == nil（全启用）情况。
func magnetSiteEnabled(enabled map[string]struct{}, id string) bool {
	if enabled == nil {
		return true
	}
	_, ok := enabled[id]
	return ok
}

// parseMagnetEnabledSites 解析 settings 存的 JSON 字符串为启用的 site id 集合。
// 空 / 失败 → 返回 nil（前端按"全启用"处理，保持向后兼容）。
func parseMagnetEnabledSites(raw string) map[string]struct{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out[id] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseMagnetCustomSites 解析自定义镜像 JSON 数组；空 / 失败返回 nil。
func parseMagnetCustomSites(raw string) []string {
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

// magnetSearch 按前端 tab 选定的单站抓取，结果不跨站合并。
// ?site=<id>：sukebei / nyaa / sukebei_cn / custom:<url>。
// 若 site 缺失或对应镜像找不到 → 回退到旧 KeyMagnetSearchBaseURL 单站（"legacy"），保持向后兼容。
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

	site, err := h.resolveMagnetSite(r.URL.Query().Get("site"))
	if err != nil {
		writeErr(w, err)
		return
	}

	c := sukebei.NewClient(sukebei.Options{BaseURL: site.BaseURL, ProxyURL: proxy})
	results, err := c.Search(r.Context(), query, limit)
	if err != nil {
		h.log.Warn("磁力搜索失败", "q", query, "site", site.ID, "err", err)
		writeErr(w, domain.Errorf(domain.CodeDriverError, "磁力搜索失败：%v", err))
		return
	}
	out := make([]magnetResult, 0, len(results))
	for _, it := range results {
		out = append(out, magnetResult{Result: it, Source: site.ID})
	}
	writeOK(w, out)
}

// resolveMagnetSite 解析前端传来的 site id；找不到时回退到旧 BaseURL 配置。
func (h *Handler) resolveMagnetSite(raw string) (magnetSite, error) {
	id := strings.TrimSpace(raw)
	if id != "" {
		for _, s := range h.allMagnetSites() {
			if s.ID == id {
				return s, nil
			}
		}
	}
	if base := h.magnetSearchBaseURL(); base != "" {
		return magnetSite{ID: "legacy", Label: "Legacy", BaseURL: base}, nil
	}
	return magnetSite{}, domain.Errorf(domain.CodeValidation, "未配置任何磁力搜索镜像")
}

// allMagnetSites 返回内置 + 自定义镜像清单。
func (h *Handler) allMagnetSites() []magnetSite {
	custom := parseMagnetCustomSites(h.settingString(settings.KeyMagnetSearchCustomSites))
	out := make([]magnetSite, 0, len(magnetBuiltin)+len(custom))
	out = append(out, magnetBuiltin...)
	for _, raw := range custom {
		out = append(out, magnetSite{
			ID:      "custom:" + raw,
			Label:   customSiteLabel(raw),
			BaseURL: strings.TrimRight(raw, "/"),
		})
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

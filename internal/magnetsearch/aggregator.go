// Package magnetsearch 聚合多 nyaa/sukebei 镜像站的磁力搜索。
// 复用 internal/sukebei 的 HTML 解析器，每个镜像站构造独立 Client，
// 并发抓取后按 infohash 去重，按 seeders 降序返回。
package magnetsearch

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"litepan/internal/sukebei"
)

// Site 描述一个 nyaa/sukebei 系镜像的最小元数据。
// 不同镜像的 HTML 表格结构由 sukebei 客户端统一处理（与 sukebei.nyaa.si 一致），
// 因此仅需 base_url。
type Site struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	BaseURL string `json:"base_url"`
}

// Builtin 内置镜像清单。所有镜像的页面结构均与 sukebei.nyaa.si 一致（nyaa 表格）。
// 顺序即前端默认勾选顺序。
var Builtin = []Site{
	{ID: "sukebei", Label: "Sukebei", BaseURL: "https://sukebei.nyaa.si"},
	{ID: "nyaa", Label: "Nyaa", BaseURL: "https://nyaa.net"},
	{ID: "sukebei_cn", Label: "Sukebei CN", BaseURL: "https://sukebei.cn.nyaa.net"},
}

// LookupBuiltinID 把 base_url 映射回内置 ID；找不到返回空串。
func LookupBuiltinID(baseURL string) string {
	norm := strings.ToLower(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	for _, s := range Builtin {
		if strings.ToLower(strings.TrimRight(s.BaseURL, "/")) == norm {
			return s.ID
		}
	}
	return ""
}

// Result 是跨站聚合后的单条结果。Source 字段标识来自哪个镜像，
// 便于前端展示与排查；Hash 作为去重主键，跨站相同 infohash 合并。
type Result struct {
	sukebei.Result
	Source string `json:"source"` // 镜像 ID（sites 列表中的 id），非内置自定义 base_url 时为 "custom:<host>"
}

// ParseEnabledSites 把 settings 存的 JSON 字符串解析为启用的 site ID 集合。
// 输入空 / 解析失败 / 全空 → 返回 nil（调用方按"全部启用"处理，保持向后兼容）。
func ParseEnabledSites(raw string) map[string]struct{} {
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

// EnabledSites 根据 enabled 集合从 Builtin 里挑出实际要抓的镜像。
// enabled == nil 表示"全部启用"（向后兼容老配置）。
// 自定义 site ID 形如 "custom:<base_url>"，由 caller 解析后追加。
func EnabledSites(enabled map[string]struct{}, custom []string) []Site {
	out := make([]Site, 0, len(Builtin)+len(custom))
	if enabled == nil {
		out = append(out, Builtin...)
	} else {
		for _, s := range Builtin {
			if _, ok := enabled[s.ID]; ok {
				out = append(out, s)
			}
		}
	}
	for _, raw := range custom {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if enabled != nil {
			if _, ok := enabled["custom:"+raw]; !ok {
				continue
			}
		}
		out = append(out, Site{
			ID:      "custom:" + raw,
			Label:   shortHost(raw),
			BaseURL: strings.TrimRight(raw, "/"),
		})
	}
	return out
}

// shortHost 提取 host 部分作为 label，回退到原串前 24 字符。
func shortHost(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	if i := strings.IndexByte(raw, '/'); i >= 0 {
		raw = raw[:i]
	}
	if raw == "" {
		return "custom"
	}
	return raw
}

// Options 控制并发行为。
type Options struct {
	Timeout time.Duration // 单站超时（默认 20s）
}

// Search 并发抓取每个站点，去重合并后按 seeders 降序、date 降序、name 升序返回。
// 任意单站失败不影响其他站；整体只把"全部失败"当作错误返回。
func Search(ctx context.Context, sites []Site, proxy string, q string, limit int, opt Options) ([]Result, error) {
	if len(sites) == 0 {
		return nil, nil
	}
	timeout := opt.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	perSite := limit
	if perSite < 1 {
		perSite = 20
	}

	type siteOut struct {
		results []sukebei.Result
		err     error
		site    Site
	}
	out := make(chan siteOut, len(sites))
	var wg sync.WaitGroup
	for _, s := range sites {
		wg.Add(1)
		go func(s Site) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			c := sukebei.NewClient(sukebei.Options{
				BaseURL:  s.BaseURL,
				ProxyURL: proxy,
				Timeout:  timeout,
			})
			rs, err := c.Search(cctx, q, perSite)
			out <- siteOut{results: rs, err: err, site: s}
		}(s)
	}
	wg.Wait()
	close(out)

	// 合并：按 infohash 去重，同 hash 取 seeders 高的（不同时取 size 大的、name 长的）。
	merged := make(map[string]Result)
	var okCount int
	for so := range out {
		if so.err != nil || len(so.results) == 0 {
			continue
		}
		okCount++
		for _, r := range so.results {
			if r.Hash == "" {
				continue
			}
			existing, hit := merged[r.Hash]
			if !hit {
				merged[r.Hash] = Result{Result: r, Source: so.site.ID}
				continue
			}
			// 同 hash 多源：取 seeders 大的，否则保持先到者。
			if r.Seeders > existing.Seeders {
				merged[r.Hash] = Result{Result: r, Source: so.site.ID}
			}
		}
	}

	if okCount == 0 {
		// 全部失败：返回最具诊断价值的错误（取首个）
		var firstErr error
		for so := range out {
			if so.err != nil {
				firstErr = so.err
				break
			}
		}
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, nil
	}

	results := make([]Result, 0, len(merged))
	for _, r := range merged {
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Seeders != results[j].Seeders {
			return results[i].Seeders > results[j].Seeders
		}
		if results[i].Date != results[j].Date {
			return results[i].Date > results[j].Date
		}
		return results[i].Name < results[j].Name
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

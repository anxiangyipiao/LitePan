package rss

import (
	"strconv"
	"strings"

	"litepan/internal/domain"
)

// MatchResult 匹配结果。Reason 供预览 UI 展示未命中原因。
type MatchResult struct {
	Matched bool
	Reason  string
}

// Match 按订阅过滤规则匹配条目。调度器与预览共用，保证所见即所得。
// 规则顺序：标题关键词 → 排除词 → 集数区间 → 画质关键词。
func Match(sub *domain.RSSSubscription, item *FeedItem) MatchResult {
	if kw := strings.TrimSpace(sub.TitleKeyword); kw != "" && !containsFold(item.Title, kw) {
		return MatchResult{Matched: false, Reason: "标题未包含「" + kw + "」"}
	}
	for _, ex := range splitKeywords(sub.ExcludeKeywords) {
		if ex != "" && containsFold(item.Title, ex) {
			return MatchResult{Matched: false, Reason: "命中排除词「" + ex + "」"}
		}
	}
	if sub.EpisodeMin > 0 || sub.EpisodeMax > 0 {
		ep := ExtractEpisode(item.Title)
		if !ep.Found {
			// 配置了集数区间但解析不出集数：跳过（防误推剧场版/OST/特典）。
			return MatchResult{Matched: false, Reason: "无法解析集数"}
		}
		if sub.EpisodeMax > 0 && ep.Start > sub.EpisodeMax {
			return MatchResult{Matched: false, Reason: "集数超出上限 " + itoa(sub.EpisodeMax)}
		}
		if sub.EpisodeMin > 0 && ep.End < sub.EpisodeMin {
			return MatchResult{Matched: false, Reason: "集数低于下限 " + itoa(sub.EpisodeMin)}
		}
	}
	if q := strings.TrimSpace(sub.QualityKeyword); q != "" && !containsFold(item.Title, q) {
		return MatchResult{Matched: false, Reason: "质量不符合「" + q + "」"}
	}
	return MatchResult{Matched: true}
}

func splitKeywords(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ' ', ',', '，', '\t', '\n', ';', '；':
			return true
		default:
			return false
		}
	})
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

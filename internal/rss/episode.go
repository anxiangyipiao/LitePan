package rss

import (
	"regexp"
	"strconv"
)

// EpisodeRange 单个集数或合集区间。Start==End 表示单集。
type EpisodeRange struct {
	Start, End int
	Found      bool
}

var (
	// 显式中文标记：第N话/話/集/卷/回，允许长连载（≤9999）。
	reCnMarker = regexp.MustCompile(`第\s*(\d+)\s*[话話集卷回]`)
	// 显式 EP 标记：EP12 / Ep. 12（≤9999）。
	reEPMarker = regexp.MustCompile(`(?i)\bEP\.?\s*(\d+)`)
	// 中英文括号数字：[01] / (02) / （03）。3 位上限以排除 1080p/2160p 与年份。
	reBracket = regexp.MustCompile(`[\[\(（]\s*(\d{1,3})\s*[\]\)）]`)
	// 标题结尾的单个或区间集数：" - 01"、" - 01-12"。3 位上限排除年份(2024)/4K(2160p)。
	reTrailing = regexp.MustCompile(`(?:^|[^0-9])-?\s*(\d{1,3})\s*(?:-\s*(\d{1,3}))?\s*$`)
	// 后缀中文标记："2話"、"12集"。前置非数字守卫，允许长连载（≤9999）。
	reSuffixCN = regexp.MustCompile(`[^0-9](\d{1,4})\s*[话話集回]`)
)

// ExtractEpisode 从标题中尽力解析集数。无法解析时 Found=false。
func ExtractEpisode(title string) EpisodeRange {
	if m := reCnMarker.FindStringSubmatch(title); m != nil {
		if n := parseEp(m[1]); n > 0 {
			return EpisodeRange{Start: n, End: n, Found: true}
		}
	}
	if m := reEPMarker.FindStringSubmatch(title); m != nil {
		if n := parseEp(m[1]); n > 0 {
			return EpisodeRange{Start: n, End: n, Found: true}
		}
	}
	if m := reBracket.FindStringSubmatch(title); m != nil {
		if n := parseEp(m[1]); n > 0 {
			return EpisodeRange{Start: n, End: n, Found: true}
		}
	}
	if m := reTrailing.FindStringSubmatch(title); m != nil {
		start := parseEp(m[1])
		end := start
		if m[2] != "" {
			if e := parseEp(m[2]); e > end {
				end = e
			}
		}
		if start > 0 {
			return EpisodeRange{Start: start, End: end, Found: true}
		}
	}
	if m := reSuffixCN.FindStringSubmatch(title); m != nil {
		if n := parseEp(m[1]); n > 0 {
			return EpisodeRange{Start: n, End: n, Found: true}
		}
	}
	return EpisodeRange{}
}

func parseEp(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

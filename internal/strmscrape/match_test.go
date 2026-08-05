package strmscrape

import (
	"encoding/json"
	"testing"

	"litepan/internal/mediaorganize/rules"
)

func TestPickTMDBScrapeMatchUsesControlledAdjacentYearDoubt(t *testing.T) {
	year := 2026
	results := rules.RawJSONListToMaps([]json.RawMessage{
		json.RawMessage(`{"id":2025,"title":"测试电影","release_date":"2025-12-20"}`),
	})
	selected, doubt := pickTMDBScrapeMatch(results, &year, MediaTypeMovie, "测试电影")
	if id, _, _, _ := rules.ExtractTMDBDisplayFields(selected, MediaTypeMovie); id != "2025" || !doubt {
		t.Fatalf("唯一强同名 ±1 年候选应命中并标记存疑，id=%q doubt=%v", id, doubt)
	}
}

func TestPickTMDBScrapeMatchPrefersExactYear(t *testing.T) {
	year := 2026
	results := rules.RawJSONListToMaps([]json.RawMessage{
		json.RawMessage(`{"id":2025,"title":"测试电影","release_date":"2025-01-01"}`),
		json.RawMessage(`{"id":2026,"title":"测试电影","release_date":"2026-01-01"}`),
	})
	selected, doubt := pickTMDBScrapeMatch(results, &year, MediaTypeMovie, "测试电影")
	if id, _, _, _ := rules.ExtractTMDBDisplayFields(selected, MediaTypeMovie); id != "2026" || doubt {
		t.Fatalf("完全相等年份必须优先且不存疑，id=%q doubt=%v", id, doubt)
	}
}

func TestPickJAVMatchExactNumber(t *testing.T) {
	results := rules.RawJSONListToMaps([]json.RawMessage{
		json.RawMessage(`{"_metatube_number":"JAC-132","_metatube_id":"x1","title":"A"}`),
		json.RawMessage(`{"_metatube_number":"SSIS-123","_metatube_id":"x2","title":"B"}`),
	})
	best, _ := pickJAVMatch(results, "SSIS-123")
	if best == nil || nonNilString(best["_metatube_id"]) != "x2" {
		t.Fatalf("精确番号应命中 x2，got %v", best)
	}
}

func TestPickJAVMatchSuffixNormalized(t *testing.T) {
	// MetaTube 可能规范化掉数字前缀：390JAC-132 → JAC-132
	results := rules.RawJSONListToMaps([]json.RawMessage{
		json.RawMessage(`{"_metatube_number":"JAC-132","_metatube_id":"x1","title":"無自覚な"}`),
		json.RawMessage(`{"_metatube_number":"OTHER-9","_metatube_id":"x2","title":"B"}`),
	})
	best, _ := pickJAVMatch(results, "390JAC-132")
	if best == nil || nonNilString(best["_metatube_id"]) != "x1" {
		t.Fatalf("后缀规范化番号应命中 x1，got %v", best)
	}
}

func TestPickJAVMatchTitleFallback(t *testing.T) {
	results := rules.RawJSONListToMaps([]json.RawMessage{
		json.RawMessage(`{"_metatube_number":"ZZZ-999","_metatube_id":"x1","title":"別の作品","original_title":"abc"}`),
	})
	best, _ := pickJAVMatch(results, "abc")
	if best == nil || nonNilString(best["_metatube_id"]) != "x1" {
		t.Fatalf("标题兼容兜底应命中，got %v", best)
	}
}

func TestPickJAVMatchNoHit(t *testing.T) {
	results := rules.RawJSONListToMaps([]json.RawMessage{
		json.RawMessage(`{"_metatube_number":"ZZZ-999","_metatube_id":"x1","title":"無関係","original_title":"xyz"}`),
	})
	if best, _ := pickJAVMatch(results, "SSIS-123"); best != nil {
		t.Fatalf("无命中应返回 nil，got %v", best)
	}
}

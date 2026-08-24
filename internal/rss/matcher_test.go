package rss

import (
	"testing"

	"litepan/internal/domain"
)

func sub(overrides map[string]any) *domain.RSSSubscription {
	s := &domain.RSSSubscription{}
	for k, v := range overrides {
		switch k {
		case "TitleKeyword":
			s.TitleKeyword = v.(string)
		case "ExcludeKeywords":
			s.ExcludeKeywords = v.(string)
		case "EpisodeMin":
			s.EpisodeMin = v.(int)
		case "EpisodeMax":
			s.EpisodeMax = v.(int)
		case "QualityKeyword":
			s.QualityKeyword = v.(string)
		}
	}
	return s
}

func item(title string) *FeedItem {
	return &FeedItem{Title: title}
}

func TestMatchKeywords(t *testing.T) {
	s := sub(map[string]any{"TitleKeyword": "孤独摇滚"})
	if r := Match(s, item("孤独摇滚 第1话 1080p")); !r.Matched {
		t.Fatalf("keyword hit should match: %+v", r)
	}
	if r := Match(s, item("另外一部 第1话")); r.Matched {
		t.Fatalf("keyword miss should not match: %+v", r)
	}
	s2 := sub(map[string]any{"TitleKeyword": "孤独摇滚", "ExcludeKeywords": "生肉 合集"})
	if r := Match(s2, item("孤独摇滚 第1话 生肉")); r.Matched {
		t.Fatalf("exclude hit should not match: %+v", r)
	}
}

func TestMatchEpisodeRange(t *testing.T) {
	s := sub(map[string]any{"EpisodeMin": 2, "EpisodeMax": 5})
	if r := Match(s, item("某番 第3话")); !r.Matched {
		t.Fatalf("ep in range should match: %+v", r)
	}
	if r := Match(s, item("某番 第1话")); r.Matched {
		t.Fatalf("ep below min should not match: %+v", r)
	}
	if r := Match(s, item("某番 第6话")); r.Matched {
		t.Fatalf("ep above max should not match: %+v", r)
	}
	// 配置区间但解析不出集数 → 不命中（防误推剧场版/OST）
	if r := Match(s, item("某番 剧场版")); r.Matched {
		t.Fatalf("unparseable ep with range should not match: %+v", r)
	}
	// 合集重叠：01-12 覆盖 [1,1]
	single := sub(map[string]any{"EpisodeMin": 1, "EpisodeMax": 1})
	if r := Match(single, item("某番 - 01-12")); !r.Matched {
		t.Fatalf("batch overlap should match: %+v", r)
	}
	// 无区间约束时，无法解析集数不应拦截
	noRange := sub(map[string]any{"TitleKeyword": "某番"})
	if r := Match(noRange, item("某番 剧场版")); !r.Matched {
		t.Fatalf("no range should not block movie: %+v", r)
	}
}

func TestMatchQuality(t *testing.T) {
	s := sub(map[string]any{"QualityKeyword": "1080p"})
	if r := Match(s, item("某番 第1话 1080p")); !r.Matched {
		t.Fatalf("quality hit should match: %+v", r)
	}
	if r := Match(s, item("某番 第1话 720p")); r.Matched {
		t.Fatalf("quality miss should not match: %+v", r)
	}
}

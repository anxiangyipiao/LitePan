package store_test

import (
	"context"
	"testing"

	"litepan/internal/domain"
)

func TestRSSSubscriptionCRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	id, err := s.RSSSubscriptions.Create(ctx, &domain.RSSSubscription{
		Name:                 "孤独摇滚",
		FeedURL:              "https://mikanani.me/RSS/Bangumi?bangumiId=1",
		Enabled:              true,
		TitleKeyword:         "孤独摇滚",
		EpisodeMin:           1,
		TargetType:           domain.RSSTargetQB,
		TargetDisplayPath:    "/",
		FetchIntervalMinutes: 30,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	got, err := s.RSSSubscriptions.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "孤独摇滚" || !got.Enabled || got.EpisodeMin != 1 {
		t.Fatalf("unexpected sub: %+v", got)
	}

	got.Enabled = false
	got.TitleKeyword = "波奇酱"
	if err := s.RSSSubscriptions.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := s.RSSSubscriptions.Get(ctx, id)
	if got2.Enabled || got2.TitleKeyword != "波奇酱" {
		t.Fatalf("update not persisted: %+v", got2)
	}

	if err := s.RSSSubscriptions.UpdateFetchState(ctx, id, "error", "boom", 1, got2.LastFetchAt); err != nil {
		t.Fatalf("update fetch state: %v", err)
	}
	got3, _ := s.RSSSubscriptions.Get(ctx, id)
	if got3.LastFetchStatus != "error" || got3.ConsecutiveFailures != 1 {
		t.Fatalf("fetch state not persisted: %+v", got3)
	}

	all, err := s.RSSSubscriptions.ListAll(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("list all: %d, %v", len(all), err)
	}
	enabled, _ := s.RSSSubscriptions.ListEnabled(ctx)
	if len(enabled) != 0 {
		t.Fatalf("disabled sub should not be listed as enabled, got %d", len(enabled))
	}
}

func TestRSSHistoryDedup(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	subID, _ := s.RSSSubscriptions.Create(ctx, &domain.RSSSubscription{Name: "订阅", FeedURL: "https://e.com/feed"})

	mk := func(guid, infohash string) *domain.RSSDownloadHistory {
		return &domain.RSSDownloadHistory{
			SubscriptionID: subID,
			FeedGUID:       guid,
			InfoHash:       infohash,
			Title:          "条目 " + guid,
			Status:         domain.RSSStatusMatched,
		}
	}
	if _, err := s.RSSDownloadHistory.Create(ctx, mk("g1", "HASH1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	ok, err := s.RSSDownloadHistory.ExistsByGUID(ctx, subID, "g1")
	if err != nil || !ok {
		t.Fatalf("ExistsByGUID(g1): %v, %v", ok, err)
	}
	ok, _ = s.RSSDownloadHistory.ExistsByGUID(ctx, subID, "g2")
	if ok {
		t.Fatal("ExistsByGUID(g2) should be false")
	}
	ok, err = s.RSSDownloadHistory.ExistsByInfoHash(ctx, "HASH1")
	if err != nil || !ok {
		t.Fatalf("ExistsByInfoHash(HASH1): %v, %v", ok, err)
	}

	// 唯一索引 (subscription_id, feed_guid)
	if _, err := s.RSSDownloadHistory.Create(ctx, mk("g1", "HASH2")); err == nil {
		t.Fatal("expected unique violation for duplicate guid")
	}
}

func TestRSSHistoryListSkipsSkipped(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	subID, _ := s.RSSSubscriptions.Create(ctx, &domain.RSSSubscription{Name: "订阅", FeedURL: "https://e.com/feed"})
	mk := func(guid, status string) *domain.RSSDownloadHistory {
		return &domain.RSSDownloadHistory{SubscriptionID: subID, FeedGUID: guid, Status: status}
	}
	for i, st := range []string{domain.RSSStatusPushed, domain.RSSStatusSkipped, domain.RSSStatusQueued, domain.RSSStatusSkipped, domain.RSSStatusFailed} {
		if _, err := s.RSSDownloadHistory.Create(ctx, mk("g"+string(rune('a'+i)), st)); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	recent, err := s.RSSDownloadHistory.ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("ListRecent should exclude skipped, got %d rows: %+v", len(recent), recent)
	}
	for _, r := range recent {
		if r.Status == domain.RSSStatusSkipped {
			t.Fatalf("skipped row leaked into ListRecent: %+v", r)
		}
	}
	bySub, err := s.RSSDownloadHistory.ListBySubscription(ctx, subID, 10, 0)
	if err != nil || len(bySub) != 3 {
		t.Fatalf("ListBySubscription should exclude skipped, got %d rows, err=%v", len(bySub), err)
	}
	// 去重查询仍能看到 skipped 行（占 GUID 槽）
	ok, _ := s.RSSDownloadHistory.ExistsByGUID(ctx, subID, "gb")
	if !ok {
		t.Fatal("ExistsByGUID should still see skipped rows")
	}
}

func TestRSSHistoryCascadeDelete(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	subID, _ := s.RSSSubscriptions.Create(ctx, &domain.RSSSubscription{Name: "订阅", FeedURL: "https://e.com/feed"})
	if _, err := s.RSSDownloadHistory.Create(ctx, &domain.RSSDownloadHistory{
		SubscriptionID: subID,
		FeedGUID:       "g1",
		Status:         domain.RSSStatusMatched,
	}); err != nil {
		t.Fatalf("create history: %v", err)
	}
	if err := s.RSSSubscriptions.Delete(ctx, subID); err != nil {
		t.Fatalf("delete sub: %v", err)
	}
	recent, err := s.RSSDownloadHistory.ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(recent) != 0 {
		t.Fatalf("history should cascade-delete, got %d rows", len(recent))
	}
}

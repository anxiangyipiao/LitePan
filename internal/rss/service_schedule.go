package rss

import (
	"context"
	"sync"
	"time"

	"litepan/internal/domain"
	"litepan/internal/settings"
)

const (
	schedulerTick      = 60 * time.Second
	maxItemsPerFetch   = 200
	maxFetchCtxTimeout = 30 * time.Second
	maxConcurrentFetch = 3
	backoffCapMinutes  = 6 * 60 // 失败退避上限 6 小时
)

func (s *Service) schedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(schedulerTick)
	defer ticker.Stop()
	s.scheduleOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scheduleOnce(ctx)
		}
	}
}

func (s *Service) scheduleOnce(ctx context.Context) {
	subs, err := s.subs.ListEnabled(ctx)
	if err != nil {
		s.logWarn("scheduler list subscriptions", err)
		return
	}
	now := time.Now()
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentFetch)
	for _, sub := range subs {
		if !s.subscriptionDue(sub, now) {
			continue
		}
		if !s.tryAcquire(sub.ID) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(sub *domain.RSSSubscription) {
			defer wg.Done()
			defer func() { <-sem }()
			defer s.release(sub.ID)
			s.fetchAndMatchOne(ctx, sub)
		}(sub)
	}
	wg.Wait()
}

// subscriptionDue 判断订阅是否到抓取时间（间隔 + 失败指数退避 + 启动抖动）。
func (s *Service) subscriptionDue(sub *domain.RSSSubscription, now time.Time) bool {
	iv := sub.FetchIntervalMinutes
	if iv <= 0 {
		iv = 30
		if s.settings != nil {
			iv = s.settings.Int(settings.KeyRSSDefaultFetchInterval)
			if iv <= 0 {
				iv = 30
			}
		}
	}
	if iv < 1 {
		iv = 1
	}
	if sub.ConsecutiveFailures > 0 {
		mult := 1 << uint(min(sub.ConsecutiveFailures, 10))
		eff := iv * mult
		if eff > backoffCapMinutes {
			eff = backoffCapMinutes
		}
		iv = eff
	}
	interval := time.Duration(iv) * time.Minute
	if !sub.LastFetchAt.IsZero() && now.Sub(sub.LastFetchAt) < interval {
		return false
	}
	// 从未抓取过的订阅：按 ID 抖动，避免启动时齐轰所有源。
	if sub.LastFetchAt.IsZero() {
		jitter := time.Duration(sub.ID%12) * 10 * time.Second
		if now.Before(sub.CreatedAt.Add(jitter)) {
			return false
		}
	}
	return true
}

func (s *Service) tryAcquire(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running == nil {
		s.running = make(map[int64]bool)
	}
	if s.running[id] {
		return false
	}
	s.running[id] = true
	return true
}

func (s *Service) release(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, id)
}

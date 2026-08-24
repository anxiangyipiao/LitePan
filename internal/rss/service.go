package rss

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"litepan/internal/domain"
	"litepan/internal/eventbus"
	"litepan/internal/offlinedownload"
	"litepan/internal/settings"
)

// Service 管理 RSS 订阅：定时抓取 → 过滤匹配 → 推送到 qB / 网盘离线下载。
type Service struct {
	subs     domain.RSSSubscriptionRepository
	history  domain.RSSDownloadHistoryRepository
	accounts domain.AccountRepository
	offline  *offlinedownload.Service
	settings *settings.Service
	bus      *eventbus.Bus
	log      *slog.Logger

	mu      sync.Mutex
	started bool
	appCtx  context.Context
	running map[int64]bool // 订阅当前是否在抓取，避免并发重叠
}

type Options struct {
	Subscriptions domain.RSSSubscriptionRepository
	History       domain.RSSDownloadHistoryRepository
	Accounts      domain.AccountRepository
	Offline       *offlinedownload.Service
	Settings      *settings.Service
	Bus           *eventbus.Bus
	Log           *slog.Logger
}

func New(opts Options) *Service {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		subs:     opts.Subscriptions,
		history:  opts.History,
		accounts: opts.Accounts,
		offline:  opts.Offline,
		settings: opts.Settings,
		bus:      opts.Bus,
		log:      log,
	}
}

// Register 订阅 eventbus。当前仅持有 bus 用于发站内通知。
func (s *Service) Register(bus *eventbus.Bus) {
	if s == nil || bus == nil {
		return
	}
	s.bus = bus
}

// Start 启动后台调度器，退出由 ctx 取消驱动。
func (s *Service) Start(ctx context.Context) {
	if s == nil || s.subs == nil {
		return
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.appCtx = ctx
	s.running = make(map[int64]bool)
	s.mu.Unlock()
	go s.schedulerLoop(ctx)
}

// ---------- 订阅 CRUD ----------

func (s *Service) ListSubscriptions(ctx context.Context) ([]*domain.RSSSubscription, error) {
	return s.subs.ListAll(ctx)
}

func (s *Service) GetSubscription(ctx context.Context, id int64) (*domain.RSSSubscription, error) {
	return s.subs.Get(ctx, id)
}

func (s *Service) CreateSubscription(ctx context.Context, in *domain.RSSSubscription) (*domain.RSSSubscription, error) {
	sub := *in
	if err := s.normalizeSubscription(&sub); err != nil {
		return nil, err
	}
	id, err := s.subs.Create(ctx, &sub)
	if err != nil {
		return nil, err
	}
	sub.ID = id
	return &sub, nil
}

func (s *Service) UpdateSubscription(ctx context.Context, id int64, in *domain.RSSSubscription) (*domain.RSSSubscription, error) {
	existing, err := s.subs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	sub := *in
	sub.ID = id
	// 抓取状态不允许通过编辑覆盖。
	sub.ConsecutiveFailures = existing.ConsecutiveFailures
	sub.LastFetchAt = existing.LastFetchAt
	sub.LastFetchStatus = existing.LastFetchStatus
	sub.LastFetchMessage = existing.LastFetchMessage
	if err := s.normalizeSubscription(&sub); err != nil {
		return nil, err
	}
	if err := s.subs.Update(ctx, &sub); err != nil {
		return nil, err
	}
	return s.subs.Get(ctx, id)
}

func (s *Service) DeleteSubscription(ctx context.Context, id int64) error {
	return s.subs.Delete(ctx, id) // 历史记录经外键级联删除
}

func (s *Service) ToggleSubscription(ctx context.Context, id int64) (*domain.RSSSubscription, error) {
	sub, err := s.subs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	sub.Enabled = !sub.Enabled
	if err := s.subs.Update(ctx, sub); err != nil {
		return nil, err
	}
	return s.subs.Get(ctx, id)
}

func (s *Service) normalizeSubscription(sub *domain.RSSSubscription) error {
	sub.Name = strings.TrimSpace(sub.Name)
	sub.FeedURL = strings.TrimSpace(sub.FeedURL)
	if sub.Name == "" {
		sub.Name = sub.FeedURL
	}
	if sub.FeedURL == "" {
		return domain.Errorf(domain.CodeValidation, "订阅地址不能为空")
	}
	lower := strings.ToLower(sub.FeedURL)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return domain.Errorf(domain.CodeValidation, "订阅地址必须以 http:// 或 https:// 开头")
	}
	sub.TitleKeyword = strings.TrimSpace(sub.TitleKeyword)
	sub.ExcludeKeywords = strings.TrimSpace(sub.ExcludeKeywords)
	sub.QualityKeyword = strings.TrimSpace(sub.QualityKeyword)
	if sub.EpisodeMin < 0 {
		sub.EpisodeMin = 0
	}
	if sub.EpisodeMax < 0 {
		sub.EpisodeMax = 0
	}
	if sub.EpisodeMax > 0 && sub.EpisodeMin > 0 && sub.EpisodeMax < sub.EpisodeMin {
		return domain.Errorf(domain.CodeValidation, "集数上限不能小于下限")
	}
	if sub.TargetType == "" {
		sub.TargetType = domain.RSSTargetQB
	}
	switch sub.TargetType {
	case domain.RSSTargetQB:
	case domain.RSSTargetOffline:
		if sub.AccountID <= 0 {
			return domain.Errorf(domain.CodeValidation, "离线到网盘需要选择目标网盘账号")
		}
	default:
		return domain.Errorf(domain.CodeValidation, "不支持的目标类型")
	}
	if sub.FetchIntervalMinutes < 0 {
		sub.FetchIntervalMinutes = 0
	}
	sub.QBSavePath = strings.TrimSpace(sub.QBSavePath)
	sub.QBCategory = strings.TrimSpace(sub.QBCategory)
	sub.TargetParentID = strings.TrimSpace(sub.TargetParentID)
	if strings.TrimSpace(sub.TargetDisplayPath) == "" {
		sub.TargetDisplayPath = "/"
	}
	return nil
}

// ---------- 历史记录 ----------

func (s *Service) ListHistory(ctx context.Context, subscriptionID int64, limit, offset int) ([]*domain.RSSDownloadHistory, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	if subscriptionID > 0 {
		return s.history.ListBySubscription(ctx, subscriptionID, limit, offset)
	}
	return s.history.ListRecent(ctx, limit)
}

func (s *Service) DeleteHistory(ctx context.Context, id int64) error {
	return s.history.Delete(ctx, id)
}

func (s *Service) ClearHistory(ctx context.Context, subscriptionID int64) (int, error) {
	return s.history.Clear(ctx, subscriptionID)
}

// RetryHistory 重推一条失败/跳过记录。
func (s *Service) RetryHistory(ctx context.Context, historyID int64) (*domain.RSSDownloadHistory, error) {
	rec, err := s.history.Get(ctx, historyID)
	if err != nil {
		return nil, err
	}
	if rec.Status != domain.RSSStatusFailed && rec.Status != domain.RSSStatusSkipped {
		return nil, domain.Errorf(domain.CodeValidation, "仅支持重推失败或跳过的记录")
	}
	sub, err := s.rebuildSubForHistory(ctx, rec)
	if err != nil {
		return nil, err
	}
	s.pushItem(ctx, sub, rec)
	return s.history.Get(ctx, historyID)
}

func (s *Service) rebuildSubForHistory(ctx context.Context, rec *domain.RSSDownloadHistory) (*domain.RSSSubscription, error) {
	sub := &domain.RSSSubscription{TargetType: rec.TargetType}
	if rec.TargetType == domain.RSSTargetOffline {
		real, err := s.subs.Get(ctx, rec.SubscriptionID)
		if err != nil {
			return nil, err
		}
		sub.AccountID = real.AccountID
		sub.TargetParentID = real.TargetParentID
		sub.TargetDisplayPath = real.TargetDisplayPath
	}
	return sub, nil
}

// ---------- 预览（保存前先看命中结果） ----------

type PreviewInput struct {
	FeedURL         string
	TitleKeyword    string
	ExcludeKeywords string
	EpisodeMin      int
	EpisodeMax      int
	QualityKeyword  string
	Limit           int
}

type PreviewItem struct {
	Title      string    `json:"title"`
	GUID       string    `json:"guid"`
	Link       string    `json:"link"`
	PubDate    time.Time `json:"pub_date"`
	TorrentURL string    `json:"torrent_url"`
	InfoHash   string    `json:"infohash"`
	Episode    int       `json:"episode"` // 0 = 未解析
	Matched    bool      `json:"matched"`
	Reason     string    `json:"reason"`
}

type PreviewResult struct {
	FeedTitle string        `json:"feed_title"`
	Items     []PreviewItem `json:"items"`
	Total     int           `json:"total"`
	FetchedAt time.Time     `json:"fetched_at"`
}

func (s *Service) PreviewFeed(ctx context.Context, in PreviewInput) (*PreviewResult, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	fetchCtx, cancel := context.WithTimeout(ctx, maxFetchCtxTimeout)
	defer cancel()
	body, err := s.newClient().Fetch(fetchCtx, in.FeedURL)
	if err != nil {
		return nil, domain.Errorf(domain.CodeDriverError, "抓取订阅源失败：%v", err)
	}
	feedTitle, items, err := ParseFeed(body)
	if err != nil {
		return nil, domain.Wrap(domain.CodeValidation, err)
	}
	if len(items) > limit {
		items = items[:limit]
	}
	sub := &domain.RSSSubscription{
		TitleKeyword:    strings.TrimSpace(in.TitleKeyword),
		ExcludeKeywords: strings.TrimSpace(in.ExcludeKeywords),
		QualityKeyword:  strings.TrimSpace(in.QualityKeyword),
	}
	if in.EpisodeMin > 0 {
		sub.EpisodeMin = in.EpisodeMin
	}
	if in.EpisodeMax > 0 {
		sub.EpisodeMax = in.EpisodeMax
	}
	out := make([]PreviewItem, 0, len(items))
	for i := range items {
		it := &items[i]
		ep := ExtractEpisode(it.Title)
		epNum := 0
		if ep.Found {
			epNum = ep.Start
		}
		res := Match(sub, it)
		out = append(out, PreviewItem{
			Title:      it.Title,
			GUID:       it.GUID,
			Link:       it.Link,
			PubDate:    it.PubDate,
			TorrentURL: it.TorrentURL,
			InfoHash:   it.InfoHash,
			Episode:    epNum,
			Matched:    res.Matched,
			Reason:     res.Reason,
		})
	}
	return &PreviewResult{
		FeedTitle: feedTitle,
		Items:     out,
		Total:     len(out),
		FetchedAt: time.Now(),
	}, nil
}

// ---------- 内部辅助 ----------

func (s *Service) accountName(ctx context.Context, accountID int64) string {
	if s.accounts == nil || accountID <= 0 {
		return ""
	}
	acc, err := s.accounts.Get(ctx, accountID)
	if err != nil || acc == nil {
		return ""
	}
	return acc.Name
}

func (s *Service) notify(level, category, title, message string, refID int64) {
	if s == nil || s.bus == nil {
		return
	}
	s.bus.Publish(context.Background(), eventbus.NotificationCreated{
		Level:     level,
		Category:  category,
		Title:     title,
		Message:   message,
		AccountID: 0,
		RefID:     refID,
	})
}

func (s *Service) logWarn(msg string, err error) {
	if s == nil || s.log == nil {
		return
	}
	s.log.Warn("rss: "+msg, "err", err)
}

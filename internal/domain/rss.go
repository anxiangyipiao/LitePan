package domain

import (
	"context"
	"time"
)

// RSS 订阅（自动追番/追剧）领域模型与仓储契约。

const (
	// RSSHistoryStatus 下载记录状态。
	RSSStatusMatched   = "matched"
	RSSStatusPushed    = "pushed"
	RSSStatusQueued    = "queued"
	RSSStatusCompleted = "completed"
	RSSStatusFailed    = "failed"
	RSSStatusSkipped   = "skipped"

	// RSSTargetType 推送目标类型。
	RSSTargetQB      = "qb"
	RSSTargetOffline = "offline"
)

// RSSSubscription 一条 RSS 订阅。TargetType=qb 时推送本地 qBittorrent；
// TargetType=offline 时推送到指定网盘账号的离线下载（须支持 magnet/http 链接）。
type RSSSubscription struct {
	ID                   int64     `json:"id"`
	Name                 string    `json:"name"`
	FeedURL              string    `json:"feed_url"`
	Enabled              bool      `json:"enabled"`
	TitleKeyword         string    `json:"title_keyword"`
	ExcludeKeywords      string    `json:"exclude_keywords"`
	EpisodeMin           int       `json:"episode_min"`
	EpisodeMax           int       `json:"episode_max"` // 0 = 不限
	QualityKeyword       string    `json:"quality_keyword"`
	TargetType           string    `json:"target_type"`
	QBSavePath           string    `json:"qb_save_path"`
	QBCategory           string    `json:"qb_category"`
	AccountID            int64     `json:"account_id"`
	TargetParentID       string    `json:"target_parent_id"`
	TargetDisplayPath    string    `json:"target_display_path"`
	ConvertTorrentToMagnet bool    `json:"convert_torrent_to_magnet"` // 仅 offline 目标：http .torrent 转磁力
	FetchIntervalMinutes int       `json:"fetch_interval_minutes"`    // 0 = 用系统默认
	ConsecutiveFailures  int       `json:"consecutive_failures"`
	LastFetchAt          time.Time `json:"last_fetch_at"`
	LastFetchStatus      string    `json:"last_fetch_status"`
	LastFetchMessage     string    `json:"last_fetch_message"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// RSSDownloadHistory 一条已匹配/已推送记录。feed_guid 与 (subscription_id) 联合唯一。
type RSSDownloadHistory struct {
	ID             int64     `json:"id"`
	SubscriptionID int64     `json:"subscription_id"`
	FeedGUID       string    `json:"feed_guid"`
	InfoHash       string    `json:"infohash"`
	Title          string    `json:"title"`
	Episode        int       `json:"episode"` // 0 = 未解析
	Link           string    `json:"link"`
	TorrentURL     string    `json:"torrent_url"`
	TargetType     string    `json:"target_type"`
	TargetRef      string    `json:"target_ref"` // qB 地址或网盘账号名
	Status         string    `json:"status"`
	Message        string    `json:"message"`
	Error          string    `json:"error"`
	CreatedAt      time.Time `json:"created_at"`
	PushedAt       time.Time `json:"pushed_at"`
}

// RSSSubscriptionRepository 订阅仓储。
type RSSSubscriptionRepository interface {
	Create(ctx context.Context, sub *RSSSubscription) (int64, error)
	Update(ctx context.Context, sub *RSSSubscription) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*RSSSubscription, error)
	ListEnabled(ctx context.Context) ([]*RSSSubscription, error)
	ListAll(ctx context.Context) ([]*RSSSubscription, error)
	// UpdateFetchState 单语句更新抓取状态，避免与并发编辑竞态。
	UpdateFetchState(ctx context.Context, id int64, status, message string, failures int, lastFetchAt time.Time) error
}

// RSSDownloadHistoryRepository 下载记录仓储。
type RSSDownloadHistoryRepository interface {
	Create(ctx context.Context, rec *RSSDownloadHistory) (int64, error)
	Update(ctx context.Context, rec *RSSDownloadHistory) error
	Get(ctx context.Context, id int64) (*RSSDownloadHistory, error)
	ListBySubscription(ctx context.Context, subscriptionID int64, limit, offset int) ([]*RSSDownloadHistory, error)
	ListRecent(ctx context.Context, limit int) ([]*RSSDownloadHistory, error)
	Delete(ctx context.Context, id int64) error
	Clear(ctx context.Context, subscriptionID int64) (int, error)
	ExistsByGUID(ctx context.Context, subscriptionID int64, guid string) (bool, error)
	ExistsByInfoHash(ctx context.Context, infohash string) (bool, error)
}

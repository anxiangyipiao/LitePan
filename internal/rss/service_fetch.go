package rss

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/offlinedownload"
	"litepan/internal/qb"
	"litepan/internal/settings"
	"litepan/internal/sukebei"
)

// FetchNowResult 单次抓取的汇总，供手动「立即抓取」与调度器复用。
type FetchNowResult struct {
	FetchedAt   time.Time `json:"fetched_at"`
	ItemsParsed int       `json:"items_parsed"`
	Matched     int       `json:"matched"`
	Pushed      int       `json:"pushed"`
	Failed      int       `json:"failed"`
	Message     string    `json:"message"`
}

// FetchNow 立即抓取一个订阅（忽略间隔，尊重运行中守卫）。
func (s *Service) FetchNow(ctx context.Context, id int64) (FetchNowResult, error) {
	sub, err := s.subs.Get(ctx, id)
	if err != nil {
		return FetchNowResult{}, err
	}
	if !s.tryAcquire(id) {
		return FetchNowResult{}, domain.Errorf(domain.CodeValidation, "该订阅正在抓取中")
	}
	defer s.release(id)
	return s.fetchAndMatchOne(ctx, sub), nil
}

func (s *Service) newClient() *Client {
	proxyURL := ""
	if s.settings != nil {
		proxyURL = sukebei.BuildProxyURL(
			s.settings.String(settings.KeyMagnetSearchProxyURL),
			s.settings.String(settings.KeyMagnetSearchProxyUsername),
			s.settings.String(settings.KeyMagnetSearchProxyPassword),
		)
	}
	return NewClient(proxyURL, maxFetchCtxTimeout)
}

// settingString 防御性读取设置（settings 未注入时回落空值）。
func (s *Service) settingString(key string) string {
	if s.settings == nil {
		return ""
	}
	return s.settings.String(key)
}

// fetchAndMatchOne 抓取单个订阅：拉取 → 解析 → 记录状态 → 匹配去重 → 推送。
func (s *Service) fetchAndMatchOne(ctx context.Context, sub *domain.RSSSubscription) FetchNowResult {
	res := FetchNowResult{FetchedAt: time.Now()}
	fetchCtx, cancel := context.WithTimeout(ctx, maxFetchCtxTimeout)
	defer cancel()
	body, err := s.newClient().Fetch(fetchCtx, sub.FeedURL)
	if err != nil {
		s.recordFetchFailure(ctx, sub, err)
		res.Message = "抓取失败：" + err.Error()
		return res
	}
	_, items, err := ParseFeed(body)
	if err != nil {
		s.recordFetchFailure(ctx, sub, err)
		res.Message = "解析失败：" + err.Error()
		return res
	}
	if err := s.subs.UpdateFetchState(ctx, sub.ID, "ok", fmt.Sprintf("解析 %d 条", len(items)), 0, res.FetchedAt); err != nil {
		s.logWarn("update fetch state", err)
	}
	items = topNewest(items, maxItemsPerFetch)
	res.ItemsParsed = len(items)
	for i := range items {
		item := &items[i]
		// 每条 GUID 已处理过（任何结果）则跳过：幂等 + 占槽。
		if ok, _ := s.history.ExistsByGUID(ctx, sub.ID, item.GUID); ok {
			continue
		}
		// 全局种子去重：该 infohash 已推送过。
		if item.InfoHash != "" {
			if ok, _ := s.history.ExistsByInfoHash(ctx, item.InfoHash); ok {
				s.createHistory(ctx, sub, item, domain.RSSStatusSkipped, "全局去重：该种子已被推送过")
				continue
			}
		}
		match := Match(sub, item)
		if !match.Matched {
			continue // 未命中不落历史，每条抓取重新评估
		}
		res.Matched++
		rec := s.createHistory(ctx, sub, item, domain.RSSStatusMatched, "")
		if rec == nil {
			continue // 唯一索引冲突（并发已处理），跳过
		}
		s.pushItem(ctx, sub, rec)
		switch rec.Status {
		case domain.RSSStatusPushed, domain.RSSStatusQueued:
			res.Pushed++
		case domain.RSSStatusFailed:
			res.Failed++
		}
	}
	if res.Matched > 0 || res.Pushed > 0 || res.Failed > 0 {
		res.Message = fmt.Sprintf("匹配 %d 条，推送 %d 条，失败 %d 条", res.Matched, res.Pushed, res.Failed)
	} else {
		res.Message = "未命中新条目"
	}
	return res
}

func (s *Service) recordFetchFailure(ctx context.Context, sub *domain.RSSSubscription, err error) {
	failures := sub.ConsecutiveFailures + 1
	wasFirst := sub.ConsecutiveFailures == 0
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	if err := s.subs.UpdateFetchState(ctx, sub.ID, "error", msg, failures, time.Now()); err != nil {
		s.logWarn("update fetch state", err)
	}
	if wasFirst {
		s.notify("warn", "rss", "RSS 订阅抓取失败", sub.Name+"："+msg, 0)
	}
}

func (s *Service) createHistory(ctx context.Context, sub *domain.RSSSubscription, item *FeedItem, status, message string) *domain.RSSDownloadHistory {
	rec := &domain.RSSDownloadHistory{
		SubscriptionID: sub.ID,
		FeedGUID:       item.GUID,
		InfoHash:       item.InfoHash,
		Title:          item.Title,
		Link:           item.Link,
		TorrentURL:     item.TorrentURL,
		TargetType:     sub.TargetType,
		Status:         status,
		Message:        message,
	}
	if ep := ExtractEpisode(item.Title); ep.Found {
		rec.Episode = ep.Start
	}
	id, err := s.history.Create(ctx, rec)
	if err != nil {
		s.logWarn("create history", err)
		return nil
	}
	rec.ID = id
	return rec
}

// pushItem 按目标类型推送并原地更新记录状态（去重检查只在上游，重推不受自己的 infohash 拦截）。
func (s *Service) pushItem(ctx context.Context, sub *domain.RSSSubscription, rec *domain.RSSDownloadHistory) {
	if rec == nil {
		return
	}
	torrentURL := strings.TrimSpace(rec.TorrentURL)
	switch sub.TargetType {
	case domain.RSSTargetQB:
		s.pushToQB(ctx, sub, rec, torrentURL)
	case domain.RSSTargetOffline:
		s.pushToOffline(ctx, sub, rec, torrentURL)
	default:
		s.markHistory(ctx, rec, domain.RSSStatusFailed, "", "不支持的目标类型")
	}
}

func (s *Service) pushToQB(ctx context.Context, sub *domain.RSSSubscription, rec *domain.RSSDownloadHistory, torrentURL string) {
	lower := strings.ToLower(torrentURL)
	if !isMagnet(torrentURL) &&
		!strings.HasPrefix(lower, "http://") &&
		!strings.HasPrefix(lower, "https://") {
		s.markHistory(ctx, rec, domain.RSSStatusSkipped, "qB 目标仅支持磁力链或种子链接", "")
		return
	}
	savePath := strings.TrimSpace(sub.QBSavePath)
	if savePath == "" {
		savePath = strings.TrimSpace(s.settings.String(settings.KeyQBSavePath))
	}
	category := strings.TrimSpace(sub.QBCategory)
	if category == "" {
		category = strings.TrimSpace(s.settings.String(settings.KeyQBCategory))
	}
	cl := qb.NewClient(qb.Options{
		BaseURL:  s.settings.String(settings.KeyQBURL),
		Username: s.settings.String(settings.KeyQBUsername),
		Password: s.settings.String(settings.KeyQBPassword),
		Timeout:  15 * time.Second,
	})
	if err := cl.AddMagnet(ctx, torrentURL, savePath, category); err != nil {
		s.markHistory(ctx, rec, domain.RSSStatusFailed, "", err.Error())
		s.notify("error", "rss", "RSS 推送失败", "《"+rec.Title+"》推送失败："+err.Error(), rec.ID)
		return
	}
	rec.TargetRef = s.settings.String(settings.KeyQBURL)
	s.markHistory(ctx, rec, domain.RSSStatusPushed, "已推送到 qBittorrent", "")
	s.notify("info", "rss", "RSS 已推送", "《"+rec.Title+"》已推送到 qBittorrent", rec.ID)
}

func (s *Service) pushToOffline(ctx context.Context, sub *domain.RSSSubscription, rec *domain.RSSDownloadHistory, torrentURL string) {
	if torrentURL == "" {
		s.markHistory(ctx, rec, domain.RSSStatusSkipped, "无可用种子链接", "")
		return
	}
	// http .torrent 链接直接离线只会把种子文件本体下载下来：先下载并解析出 infohash，
	// 转成磁力链再走正常离线流。
	if isHttpTorrentURL(torrentURL) {
		magnet, hash, err := s.downloadTorrentToMagnet(ctx, torrentURL, rec.Title)
		if err != nil {
			s.markHistory(ctx, rec, domain.RSSStatusFailed, "", "种子文件转磁力链失败："+err.Error())
			return
		}
		rec.TorrentURL = magnet
		rec.InfoHash = hash
		torrentURL = magnet
	}
	if sub.AccountID <= 0 {
		s.markHistory(ctx, rec, domain.RSSStatusFailed, "", "未配置目标网盘账号")
		return
	}
	caps, err := s.offline.Capabilities(ctx, sub.AccountID)
	if err != nil {
		s.markHistory(ctx, rec, domain.RSSStatusFailed, "", err.Error())
		return
	}
	if !schemeSupported(caps.URLSchemes, torrentURL) {
		s.markHistory(ctx, rec, domain.RSSStatusFailed, "", "目标网盘不支持该链接类型")
		s.notify("error", "rss", "RSS 推送失败", "《"+rec.Title+"》推送失败：目标网盘不支持该链接类型", rec.ID)
		return
	}
	accountName := s.accountName(ctx, sub.AccountID)
	tasks, err := s.offline.AddURLs(ctx, offlinedownload.AddURLParams{
		AccountID:         sub.AccountID,
		URLs:              []string{torrentURL},
		FileName:          rec.Title,
		TargetParentID:    sub.TargetParentID,
		TargetDisplayPath: sub.TargetDisplayPath,
	})
	if err != nil {
		s.markHistory(ctx, rec, domain.RSSStatusFailed, "", err.Error())
		s.notify("error", "rss", "RSS 推送失败", "《"+rec.Title+"》推送失败："+err.Error(), rec.ID)
		return
	}
	allFailed := len(tasks) > 0
	firstErr := ""
	for _, t := range tasks {
		if t.Status != driver.OfflineStatusFailed {
			allFailed = false
			break
		}
		if firstErr == "" && t.Error != "" {
			firstErr = t.Error
		}
	}
	if allFailed {
		if firstErr == "" {
			firstErr = "网盘离线下载提交失败"
		}
		s.markHistory(ctx, rec, domain.RSSStatusFailed, "", firstErr)
		s.notify("error", "rss", "RSS 推送失败", "《"+rec.Title+"》推送失败："+firstErr, rec.ID)
		return
	}
	rec.TargetRef = accountName
	s.markHistory(ctx, rec, domain.RSSStatusQueued, "已提交到网盘离线下载", "")
	s.notify("info", "rss", "RSS 已推送", "《"+rec.Title+"》已提交网盘离线下载", rec.ID)
}

// downloadTorrentToMagnet 下载 http .torrent 文件并解析 infohash，返回构造的磁力链与大写 infohash。
func (s *Service) downloadTorrentToMagnet(ctx context.Context, torrentURL, title string) (magnet, hash string, err error) {
	body, err := s.newClient().Fetch(ctx, torrentURL)
	if err != nil {
		return "", "", err
	}
	h, err := torrentInfoHash(body)
	if err != nil {
		return "", "", err
	}
	hash = strings.ToUpper(h)
	magnet = buildMagnetFromHash(h, title)
	if magnet == "" {
		return "", "", fmt.Errorf("构造磁力链失败")
	}
	return magnet, hash, nil
}

func (s *Service) markHistory(ctx context.Context, rec *domain.RSSDownloadHistory, status, message, errText string) {
	rec.Status = status
	rec.Message = message
	rec.Error = errText
	if status == domain.RSSStatusPushed || status == domain.RSSStatusQueued {
		rec.PushedAt = time.Now()
	}
	if err := s.history.Update(ctx, rec); err != nil {
		s.logWarn("update history", err)
	}
}

func schemeSupported(schemes []string, torrentURL string) bool {
	u, err := url.Parse(torrentURL)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	for _, s := range schemes {
		if strings.EqualFold(strings.TrimSpace(s), scheme) {
			return true
		}
	}
	return false
}

func topNewest(items []FeedItem, n int) []FeedItem {
	if len(items) <= n {
		return items
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].PubDate.IsZero() {
			return false
		}
		if items[j].PubDate.IsZero() {
			return true
		}
		return items[i].PubDate.After(items[j].PubDate)
	})
	return items[:n]
}

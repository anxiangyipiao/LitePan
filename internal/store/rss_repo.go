package store

import (
	"context"
	"database/sql"
	"time"

	"litepan/internal/domain"
)

type subscriptionRepo struct{ db *DB }

func (r *subscriptionRepo) Create(ctx context.Context, sub *domain.RSSSubscription) (int64, error) {
	res, err := r.db.write.ExecContext(ctx,
		`INSERT INTO rss_subscriptions
		  (name, feed_url, enabled, title_keyword, exclude_keywords, episode_min, episode_max, quality_keyword,
		   target_type, qb_save_path, qb_category, account_id, target_parent_id, target_display_path,
		   convert_torrent_to_magnet, fetch_interval_minutes, consecutive_failures, last_fetch_at, last_fetch_status, last_fetch_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sub.Name, sub.FeedURL, boolToInt(sub.Enabled), sub.TitleKeyword, sub.ExcludeKeywords,
		sub.EpisodeMin, sub.EpisodeMax, sub.QualityKeyword, sub.TargetType, sub.QBSavePath, sub.QBCategory,
		sub.AccountID, sub.TargetParentID, sub.TargetDisplayPath, boolToInt(sub.ConvertTorrentToMagnet), sub.FetchIntervalMinutes,
		sub.ConsecutiveFailures, tsValue(sub.LastFetchAt), sub.LastFetchStatus, sub.LastFetchMessage)
	if err != nil {
		return 0, wrapDB(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, wrapDB(err)
	}
	return id, nil
}

func (r *subscriptionRepo) Update(ctx context.Context, sub *domain.RSSSubscription) error {
	_, err := r.db.write.ExecContext(ctx,
		`UPDATE rss_subscriptions
		 SET name=?, feed_url=?, enabled=?, title_keyword=?, exclude_keywords=?, episode_min=?, episode_max=?, quality_keyword=?,
		     target_type=?, qb_save_path=?, qb_category=?, account_id=?, target_parent_id=?, target_display_path=?,
		     convert_torrent_to_magnet=?, fetch_interval_minutes=?, updated_at=CURRENT_TIMESTAMP
		 WHERE id=?`,
		sub.Name, sub.FeedURL, boolToInt(sub.Enabled), sub.TitleKeyword, sub.ExcludeKeywords,
		sub.EpisodeMin, sub.EpisodeMax, sub.QualityKeyword, sub.TargetType, sub.QBSavePath, sub.QBCategory,
		sub.AccountID, sub.TargetParentID, sub.TargetDisplayPath, boolToInt(sub.ConvertTorrentToMagnet), sub.FetchIntervalMinutes, sub.ID)
	return wrapDB(err)
}

func (r *subscriptionRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.write.ExecContext(ctx, `DELETE FROM rss_subscriptions WHERE id=?`, id)
	return wrapDB(err)
}

func (r *subscriptionRepo) Get(ctx context.Context, id int64) (*domain.RSSSubscription, error) {
	row := r.db.read.QueryRowContext(ctx, selectRSSSubscriptionCols+` WHERE id=?`, id)
	return scanRSSSubscription(row)
}

func (r *subscriptionRepo) ListEnabled(ctx context.Context) ([]*domain.RSSSubscription, error) {
	rows, err := r.db.read.QueryContext(ctx, selectRSSSubscriptionCols+` WHERE enabled=1 ORDER BY id ASC`)
	if err != nil {
		return nil, wrapDB(err)
	}
	defer rows.Close()
	var out []*domain.RSSSubscription
	for rows.Next() {
		sub, err := scanRSSSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, wrapDB(rows.Err())
}

func (r *subscriptionRepo) ListAll(ctx context.Context) ([]*domain.RSSSubscription, error) {
	rows, err := r.db.read.QueryContext(ctx, selectRSSSubscriptionCols+` ORDER BY id DESC`)
	if err != nil {
		return nil, wrapDB(err)
	}
	defer rows.Close()
	var out []*domain.RSSSubscription
	for rows.Next() {
		sub, err := scanRSSSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, wrapDB(rows.Err())
}

func (r *subscriptionRepo) UpdateFetchState(ctx context.Context, id int64, status, message string, failures int, lastFetchAt time.Time) error {
	_, err := r.db.write.ExecContext(ctx,
		`UPDATE rss_subscriptions
		 SET consecutive_failures=?, last_fetch_at=?, last_fetch_status=?, last_fetch_message=?, updated_at=CURRENT_TIMESTAMP
		 WHERE id=?`,
		failures, tsValue(lastFetchAt), status, message, id)
	return wrapDB(err)
}

const selectRSSSubscriptionCols = `SELECT id, name, feed_url, enabled, title_keyword, exclude_keywords, episode_min, episode_max,
  quality_keyword, target_type, qb_save_path, qb_category, account_id, target_parent_id, target_display_path,
  convert_torrent_to_magnet, fetch_interval_minutes, consecutive_failures, last_fetch_at, last_fetch_status, last_fetch_message, created_at, updated_at
FROM rss_subscriptions`

func scanRSSSubscription(s rowScanner) (*domain.RSSSubscription, error) {
	var (
		sub         domain.RSSSubscription
		enabledInt  int
		convertInt  int
		lastFetchAt sql.NullString
		createdAt   sql.NullString
		updatedAt   sql.NullString
	)
	err := s.Scan(
		&sub.ID, &sub.Name, &sub.FeedURL, &enabledInt, &sub.TitleKeyword, &sub.ExcludeKeywords,
		&sub.EpisodeMin, &sub.EpisodeMax, &sub.QualityKeyword, &sub.TargetType, &sub.QBSavePath, &sub.QBCategory,
		&sub.AccountID, &sub.TargetParentID, &sub.TargetDisplayPath, &convertInt, &sub.FetchIntervalMinutes,
		&sub.ConsecutiveFailures, &lastFetchAt, &sub.LastFetchStatus, &sub.LastFetchMessage, &createdAt, &updatedAt)
	if err != nil {
		return nil, wrapDB(err)
	}
	sub.Enabled = enabledInt != 0
	sub.ConvertTorrentToMagnet = convertInt != 0
	sub.LastFetchAt = parseTS(lastFetchAt)
	sub.CreatedAt = parseTS(createdAt)
	sub.UpdatedAt = parseTS(updatedAt)
	return &sub, nil
}

type historyRepo struct{ db *DB }

func (r *historyRepo) Create(ctx context.Context, rec *domain.RSSDownloadHistory) (int64, error) {
	res, err := r.db.write.ExecContext(ctx,
		`INSERT INTO rss_download_history
		  (subscription_id, feed_guid, infohash, title, episode, link, torrent_url, target_type, target_ref,
		   status, message, error, pushed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.SubscriptionID, rec.FeedGUID, rec.InfoHash, rec.Title, rec.Episode, rec.Link, rec.TorrentURL,
		rec.TargetType, rec.TargetRef, rec.Status, rec.Message, rec.Error, tsValue(rec.PushedAt))
	if err != nil {
		return 0, wrapDB(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, wrapDB(err)
	}
	return id, nil
}

func (r *historyRepo) Update(ctx context.Context, rec *domain.RSSDownloadHistory) error {
	_, err := r.db.write.ExecContext(ctx,
		`UPDATE rss_download_history
		 SET infohash=?, title=?, episode=?, link=?, torrent_url=?, target_type=?, target_ref=?,
		     status=?, message=?, error=?, pushed_at=?
		 WHERE id=?`,
		rec.InfoHash, rec.Title, rec.Episode, rec.Link, rec.TorrentURL, rec.TargetType, rec.TargetRef,
		rec.Status, rec.Message, rec.Error, tsValue(rec.PushedAt), rec.ID)
	return wrapDB(err)
}

func (r *historyRepo) Get(ctx context.Context, id int64) (*domain.RSSDownloadHistory, error) {
	row := r.db.read.QueryRowContext(ctx, selectRSSHistoryCols+` WHERE id=?`, id)
	return scanRSSHistory(row)
}

func (r *historyRepo) ListBySubscription(ctx context.Context, subscriptionID int64, limit, offset int) ([]*domain.RSSDownloadHistory, error) {
	// 跳过项（去重/链接不支持）会大量占满记录，默认不展示，去重查询不受影响。
	query := selectRSSHistoryCols + ` WHERE subscription_id=? AND status<>? ORDER BY id DESC LIMIT ? OFFSET ?`
	rows, err := r.db.read.QueryContext(ctx, query, subscriptionID, domain.RSSStatusSkipped, limit, offset)
	if err != nil {
		return nil, wrapDB(err)
	}
	defer rows.Close()
	var out []*domain.RSSDownloadHistory
	for rows.Next() {
		rec, err := scanRSSHistory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, wrapDB(rows.Err())
}

func (r *historyRepo) ListRecent(ctx context.Context, limit int) ([]*domain.RSSDownloadHistory, error) {
	query := selectRSSHistoryCols + ` WHERE status<>? ORDER BY id DESC LIMIT ?`
	rows, err := r.db.read.QueryContext(ctx, query, domain.RSSStatusSkipped, limit)
	if err != nil {
		return nil, wrapDB(err)
	}
	defer rows.Close()
	var out []*domain.RSSDownloadHistory
	for rows.Next() {
		rec, err := scanRSSHistory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, wrapDB(rows.Err())
}

func (r *historyRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.write.ExecContext(ctx, `DELETE FROM rss_download_history WHERE id=?`, id)
	return wrapDB(err)
}

func (r *historyRepo) Clear(ctx context.Context, subscriptionID int64) (int, error) {
	// subscriptionID<=0 时清空全部；否则只清该订阅。
	query := `DELETE FROM rss_download_history`
	args := []any{}
	if subscriptionID > 0 {
		query += ` WHERE subscription_id=?`
		args = append(args, subscriptionID)
	}
	res, err := r.db.write.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, wrapDB(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, wrapDB(err)
	}
	return int(n), nil
}

func (r *historyRepo) ExistsByGUID(ctx context.Context, subscriptionID int64, guid string) (bool, error) {
	var one int
	err := r.db.read.QueryRowContext(ctx,
		`SELECT 1 FROM rss_download_history WHERE subscription_id=? AND feed_guid=? LIMIT 1`,
		subscriptionID, guid).Scan(&one)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, wrapDB(err)
}

func (r *historyRepo) ExistsByInfoHash(ctx context.Context, infohash string) (bool, error) {
	var one int
	err := r.db.read.QueryRowContext(ctx,
		`SELECT 1 FROM rss_download_history WHERE infohash=? LIMIT 1`, infohash).Scan(&one)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, wrapDB(err)
}

const selectRSSHistoryCols = `SELECT id, subscription_id, feed_guid, infohash, title, episode, link, torrent_url,
  target_type, target_ref, status, message, error, created_at, pushed_at
FROM rss_download_history`

func scanRSSHistory(s rowScanner) (*domain.RSSDownloadHistory, error) {
	var (
		rec       domain.RSSDownloadHistory
		pushedAt  sql.NullString
		createdAt sql.NullString
	)
	err := s.Scan(
		&rec.ID, &rec.SubscriptionID, &rec.FeedGUID, &rec.InfoHash, &rec.Title, &rec.Episode, &rec.Link, &rec.TorrentURL,
		&rec.TargetType, &rec.TargetRef, &rec.Status, &rec.Message, &rec.Error, &createdAt, &pushedAt)
	if err != nil {
		return nil, wrapDB(err)
	}
	rec.CreatedAt = parseTS(createdAt)
	rec.PushedAt = parseTS(pushedAt)
	return &rec, nil
}

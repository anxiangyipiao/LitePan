CREATE TABLE rss_download_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  subscription_id INTEGER NOT NULL,
  feed_guid TEXT NOT NULL,
  infohash TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  episode INTEGER NOT NULL DEFAULT 0,
  link TEXT NOT NULL DEFAULT '',
  torrent_url TEXT NOT NULL DEFAULT '',
  target_type TEXT NOT NULL DEFAULT '',
  target_ref TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'matched',
  message TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  pushed_at TEXT DEFAULT '',
  FOREIGN KEY(subscription_id) REFERENCES rss_subscriptions(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_rss_history_guid ON rss_download_history(subscription_id, feed_guid);
CREATE INDEX idx_rss_history_infohash ON rss_download_history(infohash);
CREATE INDEX idx_rss_history_status ON rss_download_history(status);

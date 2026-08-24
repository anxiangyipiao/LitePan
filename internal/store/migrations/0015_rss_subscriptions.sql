CREATE TABLE rss_subscriptions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  feed_url TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  title_keyword TEXT NOT NULL DEFAULT '',
  exclude_keywords TEXT NOT NULL DEFAULT '',
  episode_min INTEGER NOT NULL DEFAULT 0,
  episode_max INTEGER NOT NULL DEFAULT 0,
  quality_keyword TEXT NOT NULL DEFAULT '',
  target_type TEXT NOT NULL DEFAULT 'qb',
  qb_save_path TEXT NOT NULL DEFAULT '',
  qb_category TEXT NOT NULL DEFAULT '',
  account_id INTEGER NOT NULL DEFAULT 0,
  target_parent_id TEXT NOT NULL DEFAULT '',
  target_display_path TEXT NOT NULL DEFAULT '/',
  fetch_interval_minutes INTEGER NOT NULL DEFAULT 0,
  consecutive_failures INTEGER NOT NULL DEFAULT 0,
  last_fetch_at TEXT DEFAULT '',
  last_fetch_status TEXT NOT NULL DEFAULT '',
  last_fetch_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_rss_subscriptions_enabled ON rss_subscriptions(enabled);
CREATE INDEX idx_rss_subscriptions_last_fetch ON rss_subscriptions(last_fetch_at);

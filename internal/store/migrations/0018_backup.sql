CREATE TABLE backup_jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL DEFAULT '',
  source_path TEXT NOT NULL,
  target_account_id INTEGER NOT NULL DEFAULT 0,
  target_parent_id TEXT NOT NULL DEFAULT '',
  target_display_path TEXT NOT NULL DEFAULT '/',
  method TEXT NOT NULL DEFAULT 'sha1',
  schedule_mode TEXT NOT NULL DEFAULT 'manual',
  time TEXT NOT NULL DEFAULT '',
  start_time TEXT NOT NULL DEFAULT '',
  interval_hours INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  next_run_at TEXT DEFAULT '',
  last_run_at TEXT DEFAULT '',
  last_run_status TEXT NOT NULL DEFAULT '',
  last_run_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE backup_file_states (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id INTEGER NOT NULL,
  rel_path TEXT NOT NULL,
  size INTEGER NOT NULL DEFAULT 0,
  mtime TEXT DEFAULT '',
  hash TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'uploaded',
  error TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(job_id) REFERENCES backup_jobs(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_backup_states_job_path ON backup_file_states(job_id, rel_path);

CREATE TABLE backup_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id INTEGER NOT NULL,
  started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at TEXT DEFAULT '',
  status TEXT NOT NULL DEFAULT 'running',
  total INTEGER NOT NULL DEFAULT 0,
  skipped INTEGER NOT NULL DEFAULT 0,
  uploaded INTEGER NOT NULL DEFAULT 0,
  rapid INTEGER NOT NULL DEFAULT 0,
  failed INTEGER NOT NULL DEFAULT 0,
  message TEXT NOT NULL DEFAULT '',
  FOREIGN KEY(job_id) REFERENCES backup_jobs(id) ON DELETE CASCADE
);

CREATE INDEX idx_backup_runs_job ON backup_runs(job_id, id);
CREATE INDEX idx_backup_jobs_enabled ON backup_jobs(enabled);

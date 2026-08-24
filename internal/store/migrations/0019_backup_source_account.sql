-- 备份源从「本地绝对路径」改为「网盘账号 + 目录」（跨盘备份）。
ALTER TABLE backup_jobs DROP COLUMN source_path;
ALTER TABLE backup_jobs ADD COLUMN source_account_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE backup_jobs ADD COLUMN source_parent_id TEXT NOT NULL DEFAULT '';
ALTER TABLE backup_jobs ADD COLUMN source_display_path TEXT NOT NULL DEFAULT '/';

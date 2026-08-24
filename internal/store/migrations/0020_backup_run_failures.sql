-- 备份运行记录新增失败文件明细（JSON 数组：rel_path/name/error）。
ALTER TABLE backup_runs ADD COLUMN failed_files TEXT NOT NULL DEFAULT '[]';

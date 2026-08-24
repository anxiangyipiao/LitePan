package domain

import (
	"context"
	"time"
)

// 定时备份：本地文件夹 → 目标网盘（增量秒级同步）领域模型与仓储契约。

const (
	// BackupScheduleMode 调度方式。
	BackupScheduleManual   = "manual"
	BackupScheduleDaily    = "daily"
	BackupScheduleInterval = "interval"

	// BackupRunStatus 运行状态。
	BackupRunRunning = "running"
	BackupRunSuccess = "success"
	BackupRunPartial = "partial"
	BackupRunFailed  = "failed"

	// BackupFileStatus 单文件备份结果状态。
	BackupFileUploaded = "uploaded"
	BackupFileFailed   = "failed"

	// BackupMethodNone 不秒传：直接上传（部分网盘不支持秒传，算哈希反而更慢）。
	BackupMethodNone = "none"
)

// BackupJob 一条备份任务：把源网盘账号的目录增量备份到目标网盘账号的目标目录（跨盘备份）。
type BackupJob struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	SourceAccountID    int64     `json:"source_account_id"`    // 源网盘账号
	SourceParentID     string    `json:"source_parent_id"`     // 源目录 file_id
	SourceDisplayPath  string    `json:"source_display_path"`  // 源目录展示路径
	TargetAccountID    int64     `json:"target_account_id"`    // 目标网盘账号
	TargetParentID     string    `json:"target_parent_id"`     // 目标目录 file_id
	TargetDisplayPath  string    `json:"target_display_path"`  // 目标目录展示路径
	Method            string    `json:"method"`              // sha1 | md5（秒传哈希方法）
	ScheduleMode      string    `json:"schedule_mode"`       // manual | daily | interval
	Time              string    `json:"time"`                // daily: "HH:MM"
	StartTime         string    `json:"start_time"`          // interval: "HH:MM"
	IntervalHours     int       `json:"interval_hours"`      // interval: 每隔 N 小时
	Enabled           bool      `json:"enabled"`
	NextRunAt         time.Time `json:"next_run_at"`
	LastRunAt         time.Time `json:"last_run_at"`
	LastRunStatus     string    `json:"last_run_status"`
	LastRunMessage    string    `json:"last_run_message"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// BackupRun 一次备份执行的记录。
type BackupRun struct {
	ID         int64     `json:"id"`
	JobID      int64     `json:"job_id"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Status     string    `json:"status"` // running | success | partial | failed
	Total      int       `json:"total"`
	Skipped    int       `json:"skipped"`
	Uploaded   int       `json:"uploaded"`
	Rapid      int       `json:"rapid"`
	Failed     int       `json:"failed"`
	Message    string    `json:"message"`
}

// BackupFileState 单文件指纹，增量跳过的依据：(job_id, rel_path) 唯一。
type BackupFileState struct {
	ID        int64     `json:"id"`
	JobID     int64     `json:"job_id"`
	RelPath   string    `json:"rel_path"`
	Size      int64     `json:"size"`
	MTime     time.Time `json:"mtime"`
	Hash      string    `json:"hash"`
	Status    string    `json:"status"` // uploaded | failed
	Error     string    `json:"error"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BackupJobRepository 备份任务仓储。
type BackupJobRepository interface {
	Create(ctx context.Context, job *BackupJob) (int64, error)
	Update(ctx context.Context, job *BackupJob) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*BackupJob, error)
	ListAll(ctx context.Context) ([]*BackupJob, error)
	ListEnabled(ctx context.Context) ([]*BackupJob, error)
	// UpdateLastRun 单语句更新最近运行结果，避免与并发编辑竞态。next_run_at 由调度器独占维护。
	UpdateLastRun(ctx context.Context, id int64, status, message string, lastRunAt time.Time) error
}

// BackupRunRepository 备份运行记录仓储。
type BackupRunRepository interface {
	Create(ctx context.Context, run *BackupRun) (int64, error)
	Update(ctx context.Context, run *BackupRun) error
	ListByJob(ctx context.Context, jobID int64, limit int) ([]*BackupRun, error)
	DeleteByJob(ctx context.Context, jobID int64) error
}

// BackupFileStateRepository 单文件指纹仓储。
type BackupFileStateRepository interface {
	// UpsertByRelPath 按 (job_id, rel_path) 插入或更新单文件状态。
	UpsertByRelPath(ctx context.Context, st *BackupFileState) error
	ListByJob(ctx context.Context, jobID int64) ([]*BackupFileState, error)
	DeleteByJob(ctx context.Context, jobID int64) error
}

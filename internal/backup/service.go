package backup

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"litepan/internal/core/driverexec"
	"litepan/internal/domain"
	"litepan/internal/file"
	"litepan/internal/playback"
)

// Service 管理定时备份：源网盘账号目录 → 目标网盘账号目录（跨盘增量备份）。
// 每次运行通过驱动列目录，跳过未变化的文件（本地源按 size+mtime，云盘源按 size+hash），
// 变化文件优先秒传、未命中再本地直传或下载转传。
type Service struct {
	exec     *driverexec.Executor
	files    *file.Service
	playback *playback.Service
	jobs     domain.BackupJobRepository
	runs     domain.BackupRunRepository
	states   domain.BackupFileStateRepository
	dataDir  string
	log      *slog.Logger

	mu      sync.Mutex
	started bool
	appCtx  context.Context

	runMu     sync.Mutex
	runActive bool
	runJobID  int64
	runBroad  *runBroadcaster
}

type Options struct {
	Exec     *driverexec.Executor
	Files    *file.Service
	Playback *playback.Service
	DataDir  string
	Jobs     domain.BackupJobRepository
	Runs     domain.BackupRunRepository
	States   domain.BackupFileStateRepository
	Log      *slog.Logger
}

func New(opts Options) *Service {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		exec:     opts.Exec,
		files:    opts.Files,
		playback: opts.Playback,
		jobs:     opts.Jobs,
		runs:     opts.Runs,
		states:   opts.States,
		dataDir:  opts.DataDir,
		log:      log,
	}
}

// Start 启动后台调度器，退出由 ctx 取消驱动。
func (s *Service) Start(ctx context.Context) {
	if s == nil || s.jobs == nil {
		return
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.appCtx = ctx
	s.mu.Unlock()
	s.resetStaleRuns(ctx)
	go s.schedulerLoop(ctx)
}

// resetStaleRuns 服务重启时把上次中断仍在 running 的任务/运行记录收尾为 failed，避免 UI 卡在「备份中」。
func (s *Service) resetStaleRuns(ctx context.Context) {
	if s == nil || s.jobs == nil {
		return
	}
	jobs, err := s.jobs.ListAll(ctx)
	if err != nil {
		s.log.Warn("backup reset stale runs: list jobs failed", "err", err)
		return
	}
	now := time.Now()
	for _, job := range jobs {
		if job.LastRunStatus != domain.BackupRunRunning {
			continue
		}
		if err := s.jobs.UpdateLastRun(ctx, job.ID, domain.BackupRunFailed, "服务重启，上次备份中断", now); err != nil {
			s.log.Warn("backup reset stale run failed", "job_id", job.ID, "err", err)
		}
		runs, err := s.runs.ListByJob(ctx, job.ID, 100)
		if err != nil {
			continue
		}
		for _, run := range runs {
			if run.Status != domain.BackupRunRunning {
				continue
			}
			run.Status = domain.BackupRunFailed
			run.FinishedAt = now
			run.Message = "服务重启，上次备份中断"
			if err := s.runs.Update(ctx, run); err != nil {
				s.log.Warn("backup reset stale run row failed", "run_id", run.ID, "err", err)
			}
		}
	}
}

// ---------- CRUD ----------

func (s *Service) ListJobs(ctx context.Context) ([]*domain.BackupJob, error) {
	return s.jobs.ListAll(ctx)
}

func (s *Service) GetJob(ctx context.Context, id int64) (*domain.BackupJob, error) {
	return s.jobs.Get(ctx, id)
}

func (s *Service) CreateJob(ctx context.Context, in *domain.BackupJob) (*domain.BackupJob, error) {
	job := *in
	if err := s.normalizeJob(ctx, &job); err != nil {
		return nil, err
	}
	id, err := s.jobs.Create(ctx, &job)
	if err != nil {
		return nil, err
	}
	return s.jobs.Get(ctx, id)
}

func (s *Service) UpdateJob(ctx context.Context, id int64, in *domain.BackupJob) (*domain.BackupJob, error) {
	existing, err := s.jobs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	job := *in
	job.ID = id
	// 运行结果不允许通过编辑覆盖。
	job.LastRunAt = existing.LastRunAt
	job.LastRunStatus = existing.LastRunStatus
	job.LastRunMessage = existing.LastRunMessage
	if err := s.normalizeJob(ctx, &job); err != nil {
		return nil, err
	}
	if err := s.jobs.Update(ctx, &job); err != nil {
		return nil, err
	}
	return s.jobs.Get(ctx, id)
}

func (s *Service) DeleteJob(ctx context.Context, id int64) error {
	return s.jobs.Delete(ctx, id) // states/runs 经外键级联删除
}

func (s *Service) ToggleJob(ctx context.Context, id int64) (*domain.BackupJob, error) {
	job, err := s.jobs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	job.Enabled = !job.Enabled
	if job.Enabled {
		job.NextRunAt = computeNextRun(job.ScheduleMode, scheduleCfg(job), time.Now())
	} else {
		job.NextRunAt = time.Time{}
	}
	if err := s.jobs.Update(ctx, job); err != nil {
		return nil, err
	}
	return s.jobs.Get(ctx, id)
}

func (s *Service) ListRuns(ctx context.Context, jobID int64, limit int) ([]*domain.BackupRun, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return s.runs.ListByJob(ctx, jobID, limit)
}

func (s *Service) normalizeJob(ctx context.Context, job *domain.BackupJob) error {
	job.Name = strings.TrimSpace(job.Name)
	if job.Name == "" {
		return domain.Errorf(domain.CodeValidation, "请填写任务名称")
	}
	if job.SourceAccountID <= 0 {
		return domain.Errorf(domain.CodeValidation, "请选择源网盘账号")
	}
	if strings.TrimSpace(job.SourceParentID) == "" {
		return domain.Errorf(domain.CodeValidation, "请选择源目录（要备份的文件夹）")
	}
	if strings.TrimSpace(job.SourceDisplayPath) == "" {
		job.SourceDisplayPath = "/"
	}
	if job.TargetAccountID <= 0 {
		return domain.Errorf(domain.CodeValidation, "请选择目标网盘账号")
	}
	if strings.TrimSpace(job.TargetParentID) == "" {
		return domain.Errorf(domain.CodeValidation, "请选择目标目录")
	}
	if strings.TrimSpace(job.TargetDisplayPath) == "" {
		job.TargetDisplayPath = "/"
	}
	if job.SourceAccountID == job.TargetAccountID && job.SourceParentID == job.TargetParentID {
		return domain.Errorf(domain.CodeValidation, "源目录与目标目录相同")
	}
	if job.Method == "" {
		job.Method = "sha1"
	}
	switch job.Method {
	case "sha1", "md5":
	default:
		return domain.Errorf(domain.CodeValidation, "未知的哈希方法：%s", job.Method)
	}
	if job.ScheduleMode == "" {
		job.ScheduleMode = domain.BackupScheduleManual
	}
	switch job.ScheduleMode {
	case domain.BackupScheduleManual, domain.BackupScheduleDaily, domain.BackupScheduleInterval:
	default:
		return domain.Errorf(domain.CodeValidation, "不支持的调度方式：%s", job.ScheduleMode)
	}
	if job.ScheduleMode == domain.BackupScheduleDaily {
		h, m := parseClock(job.Time)
		job.Time = fmt.Sprintf("%02d:%02d", h, m)
	}
	if job.ScheduleMode == domain.BackupScheduleInterval {
		h, m := parseClock(job.StartTime)
		job.StartTime = fmt.Sprintf("%02d:%02d", h, m)
		if job.IntervalHours < 1 {
			job.IntervalHours = 1
		}
		if job.IntervalHours > 24*365 {
			job.IntervalHours = 24 * 365
		}
	}
	if job.ScheduleMode == domain.BackupScheduleManual {
		job.NextRunAt = time.Time{}
	} else if job.NextRunAt.IsZero() {
		job.NextRunAt = computeNextRun(job.ScheduleMode, scheduleCfg(job), time.Now())
	}
	return nil
}

// scheduleCfg 把任务上的调度字段规整为调度计算所需的 map。
func scheduleCfg(job *domain.BackupJob) map[string]any {
	return map[string]any{
		"time":           job.Time,
		"start_time":     job.StartTime,
		"interval_hours": job.IntervalHours,
	}
}

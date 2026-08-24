package backup

import (
	"context"
	"strconv"
	"strings"
	"time"

	"litepan/internal/domain"
)

const schedulerInterval = 10 * time.Second

func (s *Service) schedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(schedulerInterval)
	defer ticker.Stop()
	s.scheduleOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scheduleOnce(ctx)
		}
	}
}

func (s *Service) scheduleOnce(ctx context.Context) {
	jobs, err := s.jobs.ListEnabled(ctx)
	if err != nil {
		s.log.Warn("backup scheduler list failed", "err", err)
		return
	}
	now := time.Now()
	for _, job := range jobs {
		if job.ScheduleMode == domain.BackupScheduleManual {
			continue
		}
		cfg := scheduleCfg(job)
		next := job.NextRunAt
		if next.IsZero() {
			next = computeNextRun(job.ScheduleMode, cfg, now)
		}
		if next.IsZero() || next.After(now) {
			continue
		}
		// 到期：先推进下次运行时间（即便本次失败也不丢进度），再启动运行。
		job.NextRunAt = advanceNextRun(job.ScheduleMode, cfg, next)
		if err := s.jobs.Update(ctx, job); err != nil {
			s.log.Warn("backup schedule advance next run failed", "job_id", job.ID, "err", err)
			continue
		}
		if err := s.launchRun(job); err != nil {
			s.log.Warn("backup scheduled run failed", "job_id", job.ID, "err", err)
		}
	}
}

// launchRun 同步创建运行广播器后后台执行，保证 runNow/run/stream 返回时即可订阅到进度；
// 同一时刻只允许一个备份运行。
func (s *Service) launchRun(job *domain.BackupJob) error {
	bc := newRunBroadcaster()
	s.runMu.Lock()
	if s.runActive {
		s.runMu.Unlock()
		return domain.Errorf(domain.CodeValidation, "已有备份正在运行，请稍后再试")
	}
	s.runActive = true
	s.runJobID = job.ID
	s.runBroad = bc
	s.runMu.Unlock()
	go s.execute(job, bc)
	return nil
}

// ---------- 调度时间计算（daily / interval） ----------

// computeNextRun 返回距 base 最近的下一次运行时刻（未来）。
func computeNextRun(scheduleMode string, cfg map[string]any, base time.Time) time.Time {
	switch scheduleMode {
	case domain.BackupScheduleDaily:
		return computeDailyRunAt(base, cfg)
	case domain.BackupScheduleInterval:
		return computeIntervalStartRunAt(base, cfg)
	default:
		return time.Time{}
	}
}

// advanceNextRun 从 current 推进到下一次运行时刻。
func advanceNextRun(scheduleMode string, cfg map[string]any, current time.Time) time.Time {
	if current.IsZero() {
		return time.Time{}
	}
	switch scheduleMode {
	case domain.BackupScheduleDaily:
		return advanceDailyRunAt(wallClockTime(current), cfg)
	case domain.BackupScheduleInterval:
		return advanceIntervalRunAt(wallClockTime(current), cfg)
	default:
		return time.Time{}
	}
}

func computeDailyRunAt(base time.Time, cfg map[string]any) time.Time {
	base = wallClockTime(base)
	h, m := parseClock(anyString(cfg["time"]))
	next := time.Date(base.Year(), base.Month(), base.Day(), h, m, 0, 0, base.Location())
	if !next.After(base) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func advanceDailyRunAt(current time.Time, cfg map[string]any) time.Time {
	h, m := parseClock(anyString(cfg["time"]))
	nextDay := current.AddDate(0, 0, 1)
	return time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), h, m, 0, 0, nextDay.Location())
}

func computeIntervalStartRunAt(base time.Time, cfg map[string]any) time.Time {
	base = wallClockTime(base)
	h, m := parseClock(anyString(cfg["start_time"]))
	next := time.Date(base.Year(), base.Month(), base.Day(), h, m, 0, 0, base.Location())
	if next.After(base) {
		return next
	}
	nextDay := next.AddDate(0, 0, 1)
	return time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), h, m, 0, 0, nextDay.Location())
}

func advanceIntervalRunAt(current time.Time, cfg map[string]any) time.Time {
	interval := clampInt(anyInt(cfg["interval_hours"]), 1, 24*365)
	candidate := current.Add(time.Duration(interval) * time.Hour)
	if sameLocalDay(candidate, current) {
		return candidate
	}
	h, m := parseClock(anyString(cfg["start_time"]))
	nextDay := current.AddDate(0, 0, 1)
	return time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), h, m, 0, 0, nextDay.Location())
}

func sameLocalDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func wallClockTime(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	loc := t.Location()
	if loc == nil || loc == time.UTC {
		loc = time.Local
	}
	return t.In(loc)
}

func parseClock(text string) (int, int) {
	parts := strings.Split(strings.TrimSpace(text), ":")
	if len(parts) != 2 {
		return 0, 0
	}
	return clampInt(anyInt(parts[0]), 0, 23), clampInt(anyInt(parts[1]), 0, 59)
}

func anyInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	default:
		return 0
	}
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

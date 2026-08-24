package backup

import (
	"context"

	"litepan/internal/domain"
)

// RunNow 手动触发一次备份：独占运行锁后立即返回，实际执行在后台。
func (s *Service) RunNow(ctx context.Context, jobID int64) (*domain.BackupJob, error) {
	job, err := s.jobs.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if err := s.launchRun(job); err != nil {
		return nil, err
	}
	return job, nil
}

// SubscribeRun 订阅指定任务当前进行中的运行事件；无活动运行时返回 nil 通道。
func (s *Service) SubscribeRun(jobID int64) (chan StreamEvent, func()) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.runActive && s.runJobID == jobID && s.runBroad != nil {
		ch, unsub := s.runBroad.subscribe()
		return ch, unsub
	}
	return nil, func() {}
}

// StreamRunProgress 把进行中的运行进度流式转发给 emit（NDJSON）。无活动运行时回放最近一次运行结果。
func (s *Service) StreamRunProgress(ctx context.Context, jobID int64, emit func(StreamEvent) error) error {
	ch, unsub := s.SubscribeRun(jobID)
	if ch == nil {
		runs, err := s.runs.ListByJob(ctx, jobID, 1)
		if err != nil {
			return err
		}
		if len(runs) > 0 {
			r := runs[0]
			return emit(StreamEvent{
				"event": "end", "run_id": r.ID, "status": r.Status, "message": r.Message,
				"total": r.Total, "skipped": r.Skipped, "uploaded": r.Uploaded, "rapid": r.Rapid, "failed": r.Failed,
			})
		}
		return emit(StreamEvent{"event": "end", "status": "none", "message": "该任务还没有运行记录"})
	}
	defer unsub()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := emit(ev); err != nil {
				return err
			}
			if ev["event"] == "end" || ev["event"] == "error" {
				return nil
			}
		case <-ctx.Done():
			return nil
		}
	}
}

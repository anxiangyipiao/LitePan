package backup

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"litepan/internal/core/driverexec"
	"litepan/internal/crosstransfer"
	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/driver/uploadutil"
)

// StreamEvent 运行进度事件，API 层以 NDJSON 逐行推送。
type StreamEvent map[string]any

// runBroadcaster 向订阅者广播当前运行的事件（仿 crosstransfer.RelayManager 订阅模式）。
type runBroadcaster struct {
	mu   sync.Mutex
	subs map[chan StreamEvent]struct{}
	last StreamEvent
}

func newRunBroadcaster() *runBroadcaster {
	return &runBroadcaster{subs: make(map[chan StreamEvent]struct{})}
}

func (b *runBroadcaster) subscribe() (chan StreamEvent, func()) {
	ch := make(chan StreamEvent, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	last := b.last
	b.mu.Unlock()
	if last != nil {
		select {
		case ch <- last:
		default:
		}
	}
	return ch, func() { b.unsubscribe(ch) }
}

func (b *runBroadcaster) unsubscribe(ch chan StreamEvent) {
	b.mu.Lock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *runBroadcaster) broadcast(ev StreamEvent) {
	b.mu.Lock()
	b.last = ev
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
		}
	}
	b.mu.Unlock()
}

func (b *runBroadcaster) close() {
	b.mu.Lock()
	for ch := range b.subs {
		close(ch)
	}
	b.subs = make(map[chan StreamEvent]struct{})
	b.mu.Unlock()
}

// execute 是每次备份运行的后台入口：广播进度、跑完清理。广播器由 launchRun 同步创建。
func (s *Service) execute(job *domain.BackupJob, bc *runBroadcaster) {
	bc.broadcast(StreamEvent{
		"event":              "start",
		"job_id":             job.ID,
		"source_path":        job.SourcePath,
		"target_display_path": job.TargetDisplayPath,
		"target_account_id":  job.TargetAccountID,
	})
	s.runBackup(job, bc.broadcast)

	s.runMu.Lock()
	s.runActive = false
	s.runJobID = 0
	s.runBroad = nil
	s.runMu.Unlock()
	bc.close()
}

func (s *Service) runBackup(job *domain.BackupJob, emit func(StreamEvent)) {
	ctx := context.Background()
	if s.appCtx != nil {
		ctx = s.appCtx
	}
	now := time.Now()
	run := &domain.BackupRun{JobID: job.ID, StartedAt: now, Status: domain.BackupRunRunning}
	runID, err := s.runs.Create(ctx, run)
	if err != nil {
		s.log.Error("backup create run failed", "job_id", job.ID, "err", err)
		emit(StreamEvent{"event": "error", "message": "创建运行记录失败：" + err.Error()})
		_ = s.jobs.UpdateLastRun(ctx, job.ID, domain.BackupRunFailed, "创建运行记录失败", now)
		return
	}
	run.ID = runID

	states, err := s.states.ListByJob(ctx, job.ID)
	if err != nil {
		s.log.Warn("backup load states failed", "job_id", job.ID, "err", err)
	}
	stateByPath := make(map[string]*domain.BackupFileState, len(states))
	for _, st := range states {
		stateByPath[st.RelPath] = st
	}

	var total, skipped, uploaded, rapid, failed int
	dirCache := map[string]string{"": job.TargetParentID}
	emitCounters := func() StreamEvent {
		return StreamEvent{
			"total": total, "skipped": skipped, "uploaded": uploaded, "rapid": rapid, "failed": failed,
		}
	}

	walkErr := filepath.WalkDir(job.SourcePath, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			failed++
			emit(StreamEvent{
				"event": "file", "path": path, "mode": "error", "error": err.Error(),
			}.Merge(emitCounters()))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(job.SourcePath, path)
		if relErr != nil {
			failed++
			emit(StreamEvent{
				"event": "file", "path": path, "mode": "error", "error": relErr.Error(),
			}.Merge(emitCounters()))
			return nil
		}
		rel = filepath.ToSlash(rel)
		info, infoErr := d.Info()
		if infoErr != nil {
			failed++
			emit(StreamEvent{
				"event": "file", "rel_path": rel, "mode": "error", "error": infoErr.Error(),
			}.Merge(emitCounters()))
			return nil
		}
		total++

		// 快速跳过：size+mtime 均未变即认为已备份，不产生网络请求。
		if st, ok := stateByPath[rel]; ok && st.Size == info.Size() && st.MTime.Equal(info.ModTime()) {
			skipped++
			emit(StreamEvent{
				"event": "file", "rel_path": rel, "name": d.Name(), "size": info.Size(), "mode": "skip",
			}.Merge(emitCounters()))
			return nil
		}

		res := s.backupFile(ctx, job, rel, path, info, dirCache)
		switch res.mode {
		case "rapid":
			rapid++
		case "upload":
			uploaded++
		case "error":
			failed++
		}
		if res.mode == "rapid" || res.mode == "upload" {
			st := &domain.BackupFileState{
				JobID: job.ID, RelPath: rel, Size: info.Size(), MTime: info.ModTime(),
				Hash: res.hash, Status: domain.BackupFileUploaded,
			}
			if err := s.states.UpsertByRelPath(ctx, st); err != nil {
				s.log.Warn("backup upsert state failed", "job_id", job.ID, "rel", rel, "err", err)
			}
		}
		emit(StreamEvent{
			"event": "file", "rel_path": rel, "name": d.Name(), "size": info.Size(), "mode": res.mode, "error": res.errMsg,
		}.Merge(emitCounters()))
		return nil
	})
	if walkErr != nil && walkErr != context.Canceled {
		s.log.Warn("backup walk aborted", "job_id", job.ID, "err", walkErr)
	}

	status, msg := finalizeRunSummary(total, skipped, uploaded, rapid, failed)
	finishedAt := time.Now()
	run.Status = status
	run.FinishedAt = finishedAt
	run.Total = total
	run.Skipped = skipped
	run.Uploaded = uploaded
	run.Rapid = rapid
	run.Failed = failed
	run.Message = msg
	if err := s.runs.Update(ctx, run); err != nil {
		s.log.Error("backup finalize run failed", "run_id", runID, "err", err)
	}
	if err := s.jobs.UpdateLastRun(ctx, job.ID, status, msg, finishedAt); err != nil {
		s.log.Error("backup update job last run failed", "job_id", job.ID, "err", err)
	}
	emit(StreamEvent{
		"event": "end", "run_id": runID, "status": status, "message": msg,
		"total": total, "skipped": skipped, "uploaded": uploaded, "rapid": rapid, "failed": failed,
	})
}

// fileBackupResult 单文件备份结果。
type fileBackupResult struct {
	mode   string // rapid | upload | error
	hash   string
	errMsg string
}

// backupFile 把单个本地文件推送到目标网盘：先秒传，未命中再真实上传（overwrite 覆盖）。
func (s *Service) backupFile(ctx context.Context, job *domain.BackupJob, rel, localPath string, info fs.FileInfo, dirCache map[string]string) fileBackupResult {
	relDir := strings.TrimSuffix(filepath.ToSlash(filepath.Dir(rel)), ".")
	relDir = strings.Trim(relDir, "/")
	folderID, err := crosstransfer.EnsureTargetDir(ctx, s.files, job.TargetAccountID, job.TargetParentID, relDir, dirCache, nil)
	if err != nil {
		return fileBackupResult{mode: "error", errMsg: "创建目标目录失败：" + err.Error()}
	}

	md5, sha1, err := uploadutil.HashMD5SHA1(ctx, localPath)
	if err != nil {
		return fileBackupResult{mode: "error", errMsg: "计算文件指纹失败：" + err.Error()}
	}
	hash := sha1
	if job.Method == "md5" {
		hash = md5
	}

	// 优先秒传：目标网盘已存在同内容文件则免上传。
	var rapidID string
	rapidHit := false
	rapidErr := s.exec.Run(ctx, job.TargetAccountID, func(drv driver.Driver) error {
		rap, ok := drv.(driver.RapidUploader)
		if !ok {
			return nil
		}
		res, err := rap.RapidUploadByHash(ctx, driver.RapidUploadRequest{
			ParentID:  folderID,
			FileName:  info.Name(),
			Method:    job.Method,
			Hash:      hash,
			Size:      info.Size(),
			Duplicate: 2, // 覆盖同名
		})
		if err != nil {
			return err
		}
		rapidHit = res.Reuse
		rapidID = res.FileID
		return nil
	})
	if rapidErr != nil {
		// 秒传失败（如目标不支持/网络抖动）不中断备份，退回真实上传。
		s.log.Warn("backup rapid upload failed", "rel", rel, "err", rapidErr)
	}
	if rapidHit {
		if rapidID != "" {
			s.files.NotifyCreated(ctx, job.TargetAccountID, folderID, rapidID, info.Name(), info.Size(), false)
		}
		return fileBackupResult{mode: "rapid", hash: hash}
	}

	var uploadRes *driver.LocalUploadResult
	mt := info.ModTime()
	err = s.exec.Run(ctx, job.TargetAccountID, func(drv driver.Driver) error {
		up, err := driverexec.Require[driver.LocalUploader](drv)
		if err != nil {
			return err
		}
		res, err := up.UploadLocalFile(ctx, driver.LocalUploadRequest{
			LocalPath:      localPath,
			FileName:       info.Name(),
			ParentID:       folderID,
			ConflictPolicy: "overwrite",
			ModTime:        &mt,
		})
		if err != nil {
			return err
		}
		uploadRes = res
		return nil
	})
	if err != nil {
		return fileBackupResult{mode: "error", errMsg: "上传失败：" + err.Error()}
	}
	if uploadRes != nil && uploadRes.FileID != "" {
		s.files.NotifyCreated(ctx, job.TargetAccountID, folderID, uploadRes.FileID, uploadRes.FileName, uploadRes.Size, false)
	}
	return fileBackupResult{mode: "upload", hash: hash}
}

func finalizeRunSummary(total, skipped, uploaded, rapid, failed int) (string, string) {
	if total == 0 {
		return domain.BackupRunSuccess, "源目录为空或没有文件"
	}
	done := uploaded + rapid
	switch {
	case failed > 0 && done == 0:
		return domain.BackupRunFailed, fmt.Sprintf("备份失败：共 %d 个文件，全部失败", total)
	case failed > 0:
		return domain.BackupRunPartial, fmt.Sprintf("备份部分完成：共 %d 个文件，跳过 %d，新增/更新 %d（秒传 %d），失败 %d", total, skipped, done, rapid, failed)
	default:
		return domain.BackupRunSuccess, fmt.Sprintf("备份完成：共 %d 个文件，跳过 %d，新增/更新 %d（秒传 %d）", total, skipped, done, rapid)
	}
}

// Merge 把计数合并进事件。
func (ev StreamEvent) Merge(other StreamEvent) StreamEvent {
	for k, v := range other {
		ev[k] = v
	}
	return ev
}

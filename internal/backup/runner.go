package backup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"litepan/internal/core/driverexec"
	"litepan/internal/crosstransfer"
	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/driver/uploadutil"
	"litepan/internal/upload"
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
		"source_display_path": job.SourceDisplayPath,
		"target_display_path": job.TargetDisplayPath,
	})
	s.runBackup(job, bc.broadcast)

	s.runMu.Lock()
	s.runActive = false
	s.runJobID = 0
	s.runBroad = nil
	s.runMu.Unlock()
	bc.close()
}

// sourceScanFile 源目录扫描出的单个文件。
type sourceScanFile struct {
	ID      string // 源文件 file_id；IsLocal 时即本地绝对路径
	RelPath string
	RelDir  string
	Name    string
	Size    int64
	ModTime time.Time
	Hash    string // 驱动元数据提供的哈希，可能为空
	IsLocal bool   // IDKind == IDPath：文件在本机，可直读直传
}

type scanNode struct {
	id        string
	relPrefix string
}

// scanSource 通过源网盘驱动递归列目录，构建相对路径文件清单。
func (s *Service) scanSource(ctx context.Context, job *domain.BackupJob) ([]sourceScanFile, error) {
	var out []sourceScanFile
	queue := []scanNode{{id: job.SourceParentID, relPrefix: ""}}
	for len(queue) > 0 {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		node := queue[0]
		queue = queue[1:]
		// forceRefresh=true：备份需要拿到目录最新状态，不能吃缓存。
		items, err := s.files.List(ctx, job.SourceAccountID, node.id, true)
		if err != nil {
			return out, fmt.Errorf("扫描目录失败: %w", err)
		}
		for _, item := range items {
			if item.IsDir {
				queue = append(queue, scanNode{id: item.ID, relPrefix: node.relPrefix + item.Name + "/"})
				continue
			}
			out = append(out, sourceScanFile{
				ID:      item.ID,
				Name:    item.Name,
				RelPath: node.relPrefix + item.Name,
				RelDir:  strings.Trim(node.relPrefix, "/"),
				Size:    item.Size,
				ModTime: item.ModTime,
				Hash:    driver.HashFromItem(&item, job.Method),
				IsLocal: item.IDKind == domain.IDPath,
			})
		}
	}
	return out, nil
}

// unchanged 判定文件是否与上次备份一致：本地源用 size+mtime（免重读），云盘源用 size+hash（来自元数据）。
func (s *Service) unchanged(f *sourceScanFile, st *domain.BackupFileState) bool {
	if f.Size != st.Size {
		return false
	}
	if f.IsLocal && !f.ModTime.IsZero() && !st.MTime.IsZero() {
		return st.MTime.Equal(f.ModTime)
	}
	if st.Hash != "" && f.Hash != "" {
		return st.Hash == f.Hash
	}
	return false
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
	// 先把任务标记为进行中，前端据此显示「备份中」并禁用重复触发。
	if err := s.jobs.UpdateLastRun(ctx, job.ID, domain.BackupRunRunning, "备份进行中", now); err != nil {
		s.log.Warn("backup mark running failed", "job_id", job.ID, "err", err)
	}

	states, err := s.states.ListByJob(ctx, job.ID)
	if err != nil {
		s.log.Warn("backup load states failed", "job_id", job.ID, "err", err)
	}
	stateByPath := make(map[string]*domain.BackupFileState, len(states))
	for _, st := range states {
		stateByPath[st.RelPath] = st
	}

	files, scanErr := s.scanSource(ctx, job)
	if scanErr != nil {
		status := domain.BackupRunFailed
		msg := "备份失败：扫描源目录失败：" + scanErr.Error()
		if ctx.Err() != nil {
			msg += "；服务停止，备份中断"
		}
		s.finalizeRun(ctx, job, run, status, msg, 0, 0, 0, 0, 0)
		emit(StreamEvent{"event": "end", "run_id": runID, "status": status, "message": msg})
		return
	}

	total := len(files)
	var skipped, uploaded, rapid, failed int
	dirCache := map[string]string{"": job.TargetParentID}
	emitCounters := func() StreamEvent {
		return StreamEvent{
			"total": total, "skipped": skipped, "uploaded": uploaded, "rapid": rapid, "failed": failed,
		}
	}

	for i := range files {
		f := &files[i]
		if ctx.Err() != nil {
			break
		}
		if st, ok := stateByPath[f.RelPath]; ok && s.unchanged(f, st) {
			skipped++
			emit(StreamEvent{
				"event": "file", "rel_path": f.RelPath, "name": f.Name, "size": f.Size, "mode": "skip",
			}.Merge(emitCounters()))
			continue
		}

		res := s.backupFile(ctx, job, f, dirCache)
		switch res.mode {
		case "rapid":
			rapid++
		case "upload":
			uploaded++
		case "error":
			failed++
		}
		if res.mode == "rapid" || res.mode == "upload" {
			hash := res.hash
			if hash == "" {
				hash = f.Hash
			}
			st := &domain.BackupFileState{
				JobID: job.ID, RelPath: f.RelPath, Size: f.Size, MTime: f.ModTime,
				Hash: hash, Status: domain.BackupFileUploaded,
			}
			if err := s.states.UpsertByRelPath(ctx, st); err != nil {
				s.log.Warn("backup upsert state failed", "job_id", job.ID, "rel", f.RelPath, "err", err)
			}
		}
		emit(StreamEvent{
			"event": "file", "rel_path": f.RelPath, "name": f.Name, "size": f.Size, "mode": res.mode, "error": res.errMsg,
		}.Merge(emitCounters()))
	}

	status, msg := finalizeRunSummary(total, skipped, uploaded, rapid, failed)
	if ctx.Err() != nil {
		// 服务停止/取消：避免把中断的半成品标成成功。
		if status == domain.BackupRunSuccess {
			status = domain.BackupRunPartial
		}
		msg += "；服务停止，备份中断"
	}
	s.finalizeRun(ctx, job, run, status, msg, total, skipped, uploaded, rapid, failed)
	emit(StreamEvent{
		"event": "end", "run_id": runID, "status": status, "message": msg,
		"total": total, "skipped": skipped, "uploaded": uploaded, "rapid": rapid, "failed": failed,
	})
}

// finalizeRun 收尾一次运行：写运行记录 + 更新任务最近运行状态。
func (s *Service) finalizeRun(ctx context.Context, job *domain.BackupJob, run *domain.BackupRun, status, msg string, total, skipped, uploaded, rapid, failed int) {
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
		s.log.Error("backup finalize run failed", "run_id", run.ID, "err", err)
	}
	if err := s.jobs.UpdateLastRun(ctx, job.ID, status, msg, finishedAt); err != nil {
		s.log.Error("backup update job last run failed", "job_id", job.ID, "err", err)
	}
}

// fileBackupResult 单文件备份结果。
type fileBackupResult struct {
	mode   string // rapid | upload | error
	hash   string
	errMsg string
}

// backupFile 把源文件推送到目标网盘：先秒传，未命中再本地直传或下载转传。
func (s *Service) backupFile(ctx context.Context, job *domain.BackupJob, f *sourceScanFile, dirCache map[string]string) fileBackupResult {
	folderID, err := crosstransfer.EnsureTargetDir(ctx, s.files, job.TargetAccountID, job.TargetParentID, f.RelDir, dirCache, nil)
	if err != nil {
		return fileBackupResult{mode: "error", errMsg: "创建目标目录失败：" + err.Error()}
	}

	// 解析哈希（秒传与指纹用）：本地文件直接读盘，云盘取元数据或流式解析。
	hash := f.Hash
	if f.IsLocal {
		md5, sha1, herr := uploadutil.HashMD5SHA1(ctx, f.ID)
		if herr != nil {
			return fileBackupResult{mode: "error", errMsg: "计算文件指纹失败：" + herr.Error()}
		}
		if job.Method == "md5" {
			hash = md5
		} else {
			hash = sha1
		}
	} else if hash == "" {
		hash = s.resolveCloudHash(ctx, job.SourceAccountID, f, job.Method)
	}

	// 优先秒传：目标网盘已存在同内容文件则免传输。
	rapidID := ""
	rapidHit := false
	if hash != "" {
		rapidErr := s.exec.Run(ctx, job.TargetAccountID, func(drv driver.Driver) error {
			rap, ok := drv.(driver.RapidUploader)
			if !ok {
				return nil
			}
			res, err := rap.RapidUploadByHash(ctx, driver.RapidUploadRequest{
				ParentID:  folderID,
				FileName:  f.Name,
				Method:    job.Method,
				Hash:      hash,
				Size:      f.Size,
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
			s.log.Warn("backup rapid upload failed", "rel", f.RelPath, "err", rapidErr)
		}
	}
	if rapidHit {
		if rapidID != "" {
			s.files.NotifyCreated(ctx, job.TargetAccountID, folderID, rapidID, f.Name, f.Size, false)
		}
		return fileBackupResult{mode: "rapid", hash: hash}
	}

	// 兜底：本地源直接传本地路径，云盘源下载到临时目录再传。
	if f.IsLocal {
		up, uerr := s.uploadLocal(ctx, job.TargetAccountID, folderID, f.Name, f.ID, f.ModTime)
		if uerr != nil {
			return fileBackupResult{mode: "error", errMsg: "上传失败：" + uerr.Error()}
		}
		if up != nil && up.FileID != "" {
			s.files.NotifyCreated(ctx, job.TargetAccountID, folderID, up.FileID, up.FileName, up.Size, false)
		}
		return fileBackupResult{mode: "upload", hash: hash}
	}
	return s.relayUpload(ctx, job, f, folderID, hash)
}

// resolveCloudHash 云盘源文件哈希为空时，尝试驱动流式解析或元数据。
func (s *Service) resolveCloudHash(ctx context.Context, accountID int64, f *sourceScanFile, method string) string {
	hash := ""
	_ = s.exec.Run(ctx, accountID, func(drv driver.Driver) error {
		if resolver, ok := drv.(driver.TransferHashResolver); ok {
			got, err := resolver.ResolveTransferHash(ctx, &domain.FileItem{ID: f.ID, Name: f.Name, Size: f.Size}, method, true)
			if err == nil && got != "" {
				hash = got
				return nil
			}
		}
		if info, ok := drv.(driver.InfoGetter); ok {
			it, err := info.GetFileInfo(ctx, f.ID)
			if err == nil {
				hash = driver.HashFromItem(it, method)
			}
		}
		return nil
	})
	return hash
}

// relayUpload 云盘源：下载到临时目录 → 上传目标，并补算哈希用于指纹。
func (s *Service) relayUpload(ctx context.Context, job *domain.BackupJob, f *sourceScanFile, folderID, knownHash string) fileBackupResult {
	tempPath, downloaded, err := s.downloadToTemp(ctx, job.SourceAccountID, f.ID, f.Name)
	if err != nil {
		return fileBackupResult{mode: "error", errMsg: "下载源文件失败：" + err.Error()}
	}
	defer os.Remove(tempPath)
	if downloaded <= 0 {
		return fileBackupResult{mode: "error", errMsg: "源文件下载为空"}
	}

	hash := knownHash
	if hash == "" {
		md5, sha1, herr := uploadutil.HashMD5SHA1(ctx, tempPath)
		if herr == nil {
			if job.Method == "md5" {
				hash = md5
			} else {
				hash = sha1
			}
		}
	}

	up, uerr := s.uploadLocal(ctx, job.TargetAccountID, folderID, f.Name, tempPath, time.Time{})
	if uerr != nil {
		return fileBackupResult{mode: "error", errMsg: "上传失败：" + uerr.Error()}
	}
	if up != nil && up.FileID != "" {
		s.files.NotifyCreated(ctx, job.TargetAccountID, folderID, up.FileID, up.FileName, up.Size, false)
	}
	return fileBackupResult{mode: "upload", hash: hash}
}

// uploadLocal 经目标驱动上传本地文件（本地路径或临时文件），overwrite 覆盖同名。
func (s *Service) uploadLocal(ctx context.Context, accountID int64, folderID, name, localPath string, modTime time.Time) (*driver.LocalUploadResult, error) {
	var res *driver.LocalUploadResult
	err := s.exec.Run(ctx, accountID, func(drv driver.Driver) error {
		up, err := driverexec.Require[driver.LocalUploader](drv)
		if err != nil {
			return err
		}
		req := driver.LocalUploadRequest{
			LocalPath:      localPath,
			FileName:       name,
			ParentID:       folderID,
			ConflictPolicy: "overwrite",
		}
		if !modTime.IsZero() {
			req.ModTime = &modTime
		}
		got, err := up.UploadLocalFile(ctx, req)
		if err != nil {
			return err
		}
		res = got
		return nil
	})
	return res, err
}

// downloadToTemp 把源网盘文件下载到临时目录（解析成本地文件时直接读盘拷贝）。
func (s *Service) downloadToTemp(ctx context.Context, accountID int64, fileID, name string) (string, int64, error) {
	if s.playback == nil {
		return "", 0, domain.Errorf(domain.CodeInternal, "播放服务未就绪")
	}
	res, err := s.playback.Resolve(ctx, accountID, fileID, "", false)
	if err != nil {
		return "", 0, err
	}

	var reader io.Reader
	var closer io.Closer
	switch {
	case res.Link.LocalPath != "":
		f, ferr := os.Open(res.Link.LocalPath)
		if ferr != nil {
			return "", 0, domain.Wrap(domain.CodeDriverError, ferr)
		}
		reader = f
		closer = f
	case res.Link.URL != "":
		req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, res.Link.URL, nil)
		if rerr != nil {
			return "", 0, domain.Wrap(domain.CodeInternal, rerr)
		}
		for k, vals := range res.Link.Headers {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}
		client := &http.Client{Timeout: 0}
		resp, derr := client.Do(req)
		if derr != nil {
			return "", 0, domain.Wrap(domain.CodeDriverError, derr)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			return "", 0, domain.Errorf(domain.CodeDriverError, "源盘下载 HTTP %d", resp.StatusCode)
		}
		reader = resp.Body
		closer = resp.Body
	default:
		return "", 0, domain.Errorf(domain.CodeDriverError, "无法解析源盘下载地址")
	}
	defer closer.Close()

	tempDir := upload.TempDir(s.dataDir)
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", 0, domain.Wrap(domain.CodeInternal, err)
	}
	tmp, err := os.CreateTemp(tempDir, "backup-*"+filepath.Ext(name))
	if err != nil {
		return "", 0, domain.Wrap(domain.CodeInternal, err)
	}
	path := tmp.Name()
	defer tmp.Close()
	n, copyErr := io.Copy(tmp, reader)
	if copyErr != nil {
		_ = os.Remove(path)
		return "", 0, domain.Wrap(domain.CodeDriverError, copyErr)
	}
	return path, n, nil
}

func finalizeRunSummary(total, skipped, uploaded, rapid, failed int) (string, string) {
	done := uploaded + rapid
	switch {
	case failed > 0 && done == 0:
		// 全部失败（含源目录缺失/不可读：此时 total 可能为 0）。
		return domain.BackupRunFailed, fmt.Sprintf("备份失败：共 %d 个文件，全部失败", total)
	case total == 0:
		return domain.BackupRunSuccess, "源目录为空或没有文件"
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

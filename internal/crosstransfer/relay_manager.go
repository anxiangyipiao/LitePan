package crosstransfer

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"litepan/internal/core/driverexec"
	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/driver/uploadutil"
	"litepan/internal/file"
	"litepan/internal/playback"
	"litepan/internal/upload"
	"litepan/pkg/speedsmoother"
)

type RelayTaskInput struct {
	SourceAccountID   int64
	SourceAccountName string
	SourceDriverType  string
	TargetAccountID   int64
	TargetAccountName string
	TargetDriverType  string
	SourceFileID      string
	FileName          string
	RelPath           string
	RelDir            string
	TargetParentID    string
	TargetDisplayPath string
	TotalBytes        int64
	Method            string
	ConflictPolicy    string
}

type RelayTask struct {
	TaskID              string         `json:"task_id"`
	SourceAccountID     int64          `json:"source_account_id"`
	SourceAccountName   string         `json:"source_account_name"`
	SourceDriverType    string         `json:"source_driver_type"`
	TargetAccountID     int64          `json:"target_account_id"`
	TargetAccountName   string         `json:"target_account_name"`
	TargetDriverType    string         `json:"target_driver_type"`
	SourceFileID        string         `json:"source_file_id"`
	FileName            string         `json:"file_name"`
	RelPath             string         `json:"rel_path"`
	RelDir              string         `json:"rel_dir"`
	TargetParentID      string         `json:"target_parent_id"`
	TargetDisplayPath   string         `json:"target_display_path"`
	TotalBytes          int64          `json:"total_bytes"`
	Method              string         `json:"method"`
	ConflictPolicy      string         `json:"conflict_policy"`
	Status              string         `json:"status"`
	Phase               string         `json:"phase"`
	Progress            int            `json:"progress"`
	DownloadedBytes     int64          `json:"downloaded_bytes"`
	UploadedBytes       int64          `json:"uploaded_bytes"`
	SpeedBytesPerSecond float64        `json:"speed_bytes_per_second"`
	Message             string         `json:"message"`
	Error               string         `json:"error"`
	Result              map[string]any `json:"result,omitempty"`
	QueueOrder          int            `json:"queue_order"`
	CreatedAt           float64        `json:"created_at"`
	UpdatedAt           float64        `json:"updated_at"`

	localPath string
	cancel    context.CancelFunc
}

type RelayManager struct {
	exec     *driverexec.Executor
	files    *file.Service
	playback *playback.Service
	tempDir  string
	log      *slog.Logger

	mu            sync.Mutex
	tasks         map[string]*RelayTask
	queueOrder    int
	running       int
	runCond       sync.Cond
	subscribers   map[chan string]struct{}
	subscribersMu sync.Mutex
}

type RelayOptions struct {
	Exec     *driverexec.Executor
	Files    *file.Service
	Playback *playback.Service
	DataDir  string
	Log      *slog.Logger
}

func NewRelayManager(opts RelayOptions) *RelayManager {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	m := &RelayManager{
		exec:     opts.Exec,
		files:    opts.Files,
		playback: opts.Playback,
		tempDir:  upload.TempDir(opts.DataDir),
		log:      log,
		tasks:    make(map[string]*RelayTask),
	}
	m.runCond.L = &m.mu
	return m
}

func (m *RelayManager) ListTasks() []RelayTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RelayTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		out = append(out, task.public())
	}
	sortRelayTasks(out)
	return out
}

func sortRelayTasks(tasks []RelayTask) {
	slices.SortFunc(tasks, func(a, b RelayTask) int {
		ra, rb := relayTaskActivityRank(a), relayTaskActivityRank(b)
		if ra != rb {
			return ra - rb
		}
		if a.QueueOrder != b.QueueOrder {
			if a.QueueOrder < b.QueueOrder {
				return -1
			}
			return 1
		}
		if c := strings.Compare(a.RelPath, b.RelPath); c != 0 {
			return c
		}
		return strings.Compare(a.TaskID, b.TaskID)
	})
}

func relayTaskActivityRank(task RelayTask) int {
	switch task.Status {
	case "running":
		return 0
	case "pending":
		return 1
	default:
		return 2
	}
}

func (t *RelayTask) public() RelayTask {
	cp := *t
	cp.localPath = ""
	cp.cancel = nil
	return cp
}

func (m *RelayManager) CreateTask(ctx context.Context, in RelayTaskInput) (RelayTask, error) {
	if err := os.MkdirAll(m.tempDir, 0o755); err != nil {
		return RelayTask{}, domain.Wrap(domain.CodeInternal, err)
	}
	taskID := strings.ReplaceAll(uuid.NewString(), "-", "")
	suffix := filepath.Ext(in.FileName)
	localPath := filepath.Join(m.tempDir, taskID+suffix)
	now := float64(time.Now().UnixNano()) / 1e9
	m.mu.Lock()
	m.queueOrder++
	order := m.queueOrder
	m.mu.Unlock()
	task := &RelayTask{
		TaskID:            taskID,
		SourceAccountID:   in.SourceAccountID,
		SourceAccountName: in.SourceAccountName,
		SourceDriverType:  in.SourceDriverType,
		TargetAccountID:   in.TargetAccountID,
		TargetAccountName: in.TargetAccountName,
		TargetDriverType:  in.TargetDriverType,
		SourceFileID:      in.SourceFileID,
		FileName:          in.FileName,
		RelPath:           in.RelPath,
		RelDir:            in.RelDir,
		TargetParentID:    in.TargetParentID,
		TargetDisplayPath: in.TargetDisplayPath,
		TotalBytes:        in.TotalBytes,
		Method:            in.Method,
		ConflictPolicy:    in.ConflictPolicy,
		Status:            "pending",
		Phase:             "pending",
		Message:           "等待中继",
		QueueOrder:        order,
		CreatedAt:         now,
		UpdatedAt:         now,
		localPath:         localPath,
	}
	runCtx, cancel := context.WithCancel(context.Background())
	task.cancel = cancel

	m.mu.Lock()
	m.tasks[taskID] = task
	m.pruneLocked()
	m.mu.Unlock()
	m.broadcast()

	go m.runTask(runCtx, taskID)
	return task.public(), nil
}

func (m *RelayManager) DeleteTasks(taskIDs []string) int {
	removed := 0
	m.mu.Lock()
	for _, id := range taskIDs {
		task, ok := m.tasks[id]
		if !ok {
			continue
		}
		if task.cancel != nil {
			task.cancel()
		}
		delete(m.tasks, id)
		removeLocalFile(task.localPath)
		removed++
	}
	m.mu.Unlock()
	if removed > 0 {
		m.broadcast()
	}
	return removed
}

func (m *RelayManager) Subscribe() chan string {
	ch := make(chan string, 8)
	m.subscribersMu.Lock()
	if m.subscribers == nil {
		m.subscribers = make(map[chan string]struct{})
	}
	m.subscribers[ch] = struct{}{}
	m.subscribersMu.Unlock()
	ch <- m.snapshotPayload()
	return ch
}

func (m *RelayManager) Unsubscribe(ch chan string) {
	m.subscribersMu.Lock()
	delete(m.subscribers, ch)
	m.subscribersMu.Unlock()
}

func (m *RelayManager) snapshotPayload() string {
	payload, _ := json.Marshal(map[string]any{"tasks": m.ListTasks()})
	return string(payload)
}

func (m *RelayManager) broadcast() {
	m.subscribersMu.Lock()
	subs := make([]chan string, 0, len(m.subscribers))
	for ch := range m.subscribers {
		subs = append(subs, ch)
	}
	m.subscribersMu.Unlock()
	if len(subs) == 0 {
		return
	}
	payload := m.snapshotPayload()
	for _, ch := range subs {
		select {
		case ch <- payload:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- payload:
			default:
			}
		}
	}
}

func (m *RelayManager) runTask(ctx context.Context, taskID string) {
	m.mu.Lock()
	for m.running >= relayConcurrency {
		m.runCond.Wait()
	}
	m.running++
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.running--
		m.runCond.Signal()
		m.mu.Unlock()
	}()

	task := m.getObject(taskID)
	if task == nil {
		return
	}

	m.update(taskID, map[string]any{
		"status":   "running",
		"phase":    "downloading",
		"message":  "正在从源盘下载",
		"progress": 0,
	})

	downloaded, err := m.downloadSource(ctx, task, func(downloaded, total int64, message string, speed float64) {
		totalBytes := total
		if totalBytes <= 0 {
			totalBytes = task.TotalBytes
		}
		progress := 0
		if totalBytes > 0 {
			progress = int(downloaded * 100 / totalBytes)
		}
		m.update(taskID, map[string]any{
			"phase":                  "downloading",
			"downloaded_bytes":       downloaded,
			"progress":               min(100, progress),
			"speed_bytes_per_second": speed,
			"message":                message,
		})
	})
	if err != nil {
		if ctx.Err() != nil {
			m.update(taskID, map[string]any{"status": "canceled", "message": "任务已取消", "error": "任务已取消"})
			return
		}
		m.update(taskID, map[string]any{
			"status":  "failed",
			"phase":   "failed",
			"message": "兜底传输失败",
			"error":   err.Error(),
		})
		return
	}
	if downloaded <= 0 {
		m.update(taskID, map[string]any{
			"status":  "failed",
			"phase":   "failed",
			"message": "兜底传输失败",
			"error":   "源盘下载为空文件",
		})
		return
	}

	m.update(taskID, map[string]any{
		"phase":                  "uploading",
		"downloaded_bytes":       downloaded,
		"progress":               0,
		"speed_bytes_per_second": 0.0,
		"message":                "正在上传到目标盘",
	})

	folderID, err := m.resolveTargetFolder(ctx, task)
	if err != nil {
		m.update(taskID, map[string]any{
			"status":  "failed",
			"phase":   "failed",
			"message": "兜底传输失败",
			"error":   err.Error(),
		})
		return
	}

	uploadSpeed := speedsmoother.NewDefault()
	result, err := m.uploadTemp(ctx, task, folderID, func(uploaded, total int64, message string) {
		totalBytes := total
		if totalBytes <= 0 {
			totalBytes = task.TotalBytes
		}
		progress := 0
		if totalBytes > 0 {
			progress = int(uploaded * 100 / totalBytes)
		}
		if progress > 99 {
			progress = 99
		}
		speed := uploadSpeed.Sample(uploaded, time.Now(), speedsmoother.PhaseKey(message)).Display
		m.update(taskID, map[string]any{
			"phase":                  "uploading",
			"uploaded_bytes":         uploaded,
			"progress":               progress,
			"speed_bytes_per_second": speed,
			"message":                message,
		})
	})
	if err != nil {
		if ctx.Err() != nil {
			m.update(taskID, map[string]any{"status": "canceled", "message": "任务已取消", "error": "任务已取消"})
			return
		}
		m.update(taskID, map[string]any{
			"status":  "failed",
			"phase":   "failed",
			"message": "兜底传输失败",
			"error":   err.Error(),
		})
		return
	}

	m.update(taskID, map[string]any{
		"status":                 "success",
		"phase":                  "done",
		"progress":               100,
		"uploaded_bytes":         task.TotalBytes,
		"speed_bytes_per_second": 0.0,
		"message":                "兜底传输完成",
		"result":                 result,
		"error":                  "",
	})
	final := m.getObject(taskID)
	if final != nil && final.Status == "success" {
		removeLocalFile(final.localPath)
	}
}

func (m *RelayManager) getObject(taskID string) *RelayTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tasks[taskID]
}

func (m *RelayManager) update(taskID string, fields map[string]any) {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	if ok {
		applyRelayFields(task, fields)
		task.UpdatedAt = float64(time.Now().UnixNano()) / 1e9
	}
	m.mu.Unlock()
	if ok {
		m.broadcast()
	}
}

func applyRelayFields(task *RelayTask, fields map[string]any) {
	for key, value := range fields {
		switch key {
		case "status":
			task.Status, _ = value.(string)
		case "phase":
			task.Phase, _ = value.(string)
		case "progress":
			switch v := value.(type) {
			case int:
				task.Progress = v
			case int64:
				task.Progress = int(v)
			case float64:
				task.Progress = int(v)
			}
		case "downloaded_bytes":
			switch v := value.(type) {
			case int64:
				task.DownloadedBytes = v
			case int:
				task.DownloadedBytes = int64(v)
			case float64:
				task.DownloadedBytes = int64(v)
			}
		case "uploaded_bytes":
			switch v := value.(type) {
			case int64:
				task.UploadedBytes = v
			case int:
				task.UploadedBytes = int64(v)
			case float64:
				task.UploadedBytes = int64(v)
			}
		case "speed_bytes_per_second":
			task.SpeedBytesPerSecond, _ = value.(float64)
		case "message":
			task.Message, _ = value.(string)
		case "error":
			task.Error, _ = value.(string)
		case "result":
			task.Result, _ = value.(map[string]any)
		}
	}
}

func (m *RelayManager) resolveTargetFolder(ctx context.Context, task *RelayTask) (string, error) {
	if strings.Trim(task.RelDir, "/") == "" {
		return task.TargetParentID, nil
	}
	cache := map[string]string{"": task.TargetParentID}
	return EnsureTargetDir(ctx, m.files, task.TargetAccountID, task.TargetParentID, task.RelDir, cache, nil)
}

func (m *RelayManager) downloadSource(ctx context.Context, task *RelayTask, onProgress func(downloaded, total int64, message string, speed float64)) (int64, error) {
	if m.playback == nil {
		return 0, domain.Errorf(domain.CodeInternal, "播放服务未就绪")
	}
	res, err := m.playback.Resolve(ctx, task.SourceAccountID, task.SourceFileID, "", false)
	if err != nil {
		return 0, err
	}
	total := task.TotalBytes
	if res.File.Size > 0 {
		total = res.File.Size
	}
	if res.Link.URL == "" {
		return 0, domain.Errorf(domain.CodeDriverError, "无法解析源盘下载地址")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, res.Link.URL, nil)
	if err != nil {
		return 0, domain.Wrap(domain.CodeInternal, err)
	}
	for k, vals := range res.Link.Headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return 0, domain.Wrap(domain.CodeDriverError, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, domain.Errorf(domain.CodeDriverError, "源盘下载 HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(task.localPath)
	if err != nil {
		return 0, domain.Wrap(domain.CodeInternal, err)
	}
	defer f.Close()

	var downloaded int64
	downloadSpeed := speedsmoother.NewDefault()
	lastEmit := time.Now()
	buf := make([]byte, 256*1024)
	emitProgress := func(message string, speed float64) {
		if onProgress == nil {
			return
		}
		onProgress(downloaded, total, message, speed)
	}
	for {
		if ctx.Err() != nil {
			return downloaded, ctx.Err()
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return downloaded, domain.Wrap(domain.CodeInternal, werr)
			}
			downloaded += int64(n)
			now := time.Now()
			speed := downloadSpeed.Sample(downloaded, now, "download").Display
			if now.Sub(lastEmit) >= 250*time.Millisecond {
				emitProgress("正在从源盘下载", speed)
				lastEmit = now
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return downloaded, domain.Wrap(domain.CodeDriverError, readErr)
		}
	}
	emitProgress("源盘下载完成", 0)
	return downloaded, nil
}

func (m *RelayManager) uploadTemp(ctx context.Context, task *RelayTask, folderID string, onProgress func(uploaded, total int64, message string)) (map[string]any, error) {
	var result *driver.LocalUploadResult
	req := driver.LocalUploadRequest{
		LocalPath:      task.localPath,
		FileName:       task.FileName,
		ParentID:       folderID,
		ConflictPolicy: uploadutil.NormalizeConflictPolicy(task.ConflictPolicy),
		OnProgress: func(uploaded, total int64, message string) {
			if onProgress != nil {
				onProgress(uploaded, total, message)
			}
		},
	}
	var err error
	if m.files != nil {
		result, err = m.files.UploadLocal(ctx, task.TargetAccountID, req)
	} else {
		err = m.exec.Run(ctx, task.TargetAccountID, func(drv driver.Driver) error {
			uploader, err := driverexec.Require[driver.LocalUploader](drv)
			if err != nil {
				return err
			}
			got, err := uploader.UploadLocalFile(ctx, req)
			if err != nil {
				return err
			}
			result = got
			return nil
		})
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"file_id":   result.FileID,
		"parent_id": result.ParentID,
		"file_name": result.FileName,
		"size":      result.Size,
		"message":   result.Message,
	}, nil
}

func (m *RelayManager) pruneLocked() {
	if len(m.tasks) <= maxRelayTasks {
		return
	}
	var finished []*RelayTask
	for _, task := range m.tasks {
		switch task.Status {
		case "success", "failed", "canceled":
			finished = append(finished, task)
		}
	}
	for i := 0; i < len(finished); i++ {
		for j := i + 1; j < len(finished); j++ {
			if finished[j].UpdatedAt < finished[i].UpdatedAt {
				finished[i], finished[j] = finished[j], finished[i]
			}
		}
	}
	for _, task := range finished {
		if len(m.tasks) <= maxRelayTasks {
			break
		}
		delete(m.tasks, task.TaskID)
	}
}

func removeLocalFile(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}

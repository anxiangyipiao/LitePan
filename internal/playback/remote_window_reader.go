package playback

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"litepan/internal/domain"
)

const (
	defaultRemotePartSize   = 4 << 20
	defaultRemoteWindowSize = 8 << 20
	maximumRemoteWindowSize = 32 << 20
	remoteSequentialGap     = 1 << 20
)

// remoteWindowReader 用有界窗口合并 FUSE 小读，并按驱动建议并发预取顺序数据。
//
// 并发模型：
//   - mu（读写锁）保护 window/windowOff/progressEnd/prefetch；窗口内拷贝持读锁可并行，
//     不再让单个读请求串行化整条读取链路；
//   - fetchMu 单飞网络拉取，多个读者需要同一缺失窗口时只拉一次；
//   - 预取不持有 fetchMu，可被同步读取取消，避免卡死/阻塞其它窗口的拉取。
type remoteWindowReader struct {
	svc         *Service
	ctx         context.Context
	lh          *linkHolder
	size        int64
	partSize    int64
	concurrency int
	windowSize  int64

	mu          sync.RWMutex
	fetchMu     sync.Mutex
	window      []byte
	windowOff   int64
	prefetch    *remoteWindowPrefetch
	prefetchWG  sync.WaitGroup
	progressEnd int64
}

type remoteWindowPrefetch struct {
	off    int64
	cancel context.CancelFunc
	done   chan remoteWindowResult
}

type remoteWindowResult struct {
	data []byte
	err  error
}

func newRemoteWindowReader(svc *Service, ctx context.Context, lh *linkHolder, link domain.DownloadInfo, size int64) *remoteWindowReader {
	partSize := link.ChunkSize
	if partSize <= 0 {
		partSize = defaultRemotePartSize
	}
	if partSize > maximumRemoteWindowSize {
		partSize = maximumRemoteWindowSize
	}
	concurrency := link.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > maximumRangeConcurrency {
		concurrency = maximumRangeConcurrency
	}
	windowSize := partSize * int64(concurrency)
	if windowSize < defaultRemoteWindowSize {
		windowSize = defaultRemoteWindowSize
	}
	if windowSize > maximumRemoteWindowSize {
		windowSize = maximumRemoteWindowSize
	}
	return &remoteWindowReader{
		svc:         svc,
		ctx:         ctx,
		lh:          lh,
		size:        size,
		partSize:    partSize,
		concurrency: concurrency,
		windowSize:  windowSize,
		progressEnd: -1,
	}
}

func (r *remoteWindowReader) readAt(p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if off < 0 {
		return 0, fmt.Errorf("negative read offset")
	}
	if r.size > 0 && off >= r.size {
		return 0, io.EOF
	}

	eofAfterRead := false
	if r.size > 0 && int64(len(p)) > r.size-off {
		p = p[:r.size-off]
		eofAfterRead = true
	}
	written := 0
	for written < len(p) {
		curOff := off + int64(written)
		r.mu.RLock()
		if r.contains(curOff) {
			start := curOff - r.windowOff
			n := copy(p[written:], r.window[start:])
			r.mu.RUnlock()
			if n == 0 {
				return written, io.ErrNoProgress
			}
			written += n
			r.observeRead(curOff, int64(n))
			continue
		}
		r.mu.RUnlock()
		if err := r.loadWindow(curOff); err != nil {
			return written, err
		}
	}
	if eofAfterRead {
		return written, io.EOF
	}
	return written, nil
}

func (r *remoteWindowReader) contains(off int64) bool {
	return len(r.window) > 0 && off >= r.windowOff && off < r.windowOff+int64(len(r.window))
}

func (r *remoteWindowReader) windowBounds(off int64) (int64, int64) {
	windowOff := off / r.windowSize * r.windowSize
	length := r.windowSize
	if r.size > 0 {
		if windowOff >= r.size {
			return windowOff, 0
		}
		if windowOff+length > r.size {
			length = r.size - windowOff
		}
	}
	return windowOff, length
}

func (r *remoteWindowReader) loadWindow(off int64) error {
	windowOff, length := r.windowBounds(off)
	if length <= 0 {
		return io.EOF
	}

	// 命中匹配的预取：等它完成并直接复用（不持有 fetchMu，避免阻塞其它窗口拉取）。
	r.mu.RLock()
	pref := r.prefetch
	r.mu.RUnlock()
	if pref != nil && pref.off == windowOff {
		select {
		case res := <-pref.done:
			r.mu.Lock()
			if r.prefetch == pref {
				r.prefetch = nil
			}
			r.mu.Unlock()
			if res.err == nil && len(res.data) > 0 {
				r.mu.Lock()
				r.window = res.data
				r.windowOff = windowOff
				r.progressEnd = windowOff
				r.mu.Unlock()
				return nil
			}
			if r.ctx.Err() != nil {
				return r.ctx.Err()
			}
			// 预取被并发取消/失败：回落到同步拉取。
		case <-r.ctx.Done():
			pref.cancel()
			return r.ctx.Err()
		}
	}

	// 无关预取：取消掉，避免占用网络带宽。
	if pref != nil {
		pref.cancel()
		r.mu.Lock()
		if r.prefetch == pref {
			r.prefetch = nil
		}
		r.mu.Unlock()
	}

	// 同步单飞拉取：fetchMu 串行，重复需要的窗口只拉一次。
	r.fetchMu.Lock()
	defer r.fetchMu.Unlock()
	r.mu.RLock()
	if r.contains(off) {
		r.mu.RUnlock()
		return nil
	}
	r.mu.RUnlock()

	data, err := r.fetchWindow(r.ctx, windowOff, length)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.window = data
	r.windowOff = windowOff
	r.progressEnd = windowOff
	r.mu.Unlock()
	return nil
}

func (r *remoteWindowReader) observeRead(off, length int64) {
	if length <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	windowEnd := r.windowOff + int64(len(r.window))
	if off < r.windowOff || off >= windowEnd {
		return
	}
	if r.progressEnd < r.windowOff {
		r.progressEnd = r.windowOff
	}
	// 允许 FUSE 预读交错一个 I/O 请求大小，但大跨度跳读不触发预取。
	if off > r.progressEnd+remoteSequentialGap {
		return
	}
	if end := off + length; end > r.progressEnd {
		r.progressEnd = end
	}
	if r.progressEnd-r.windowOff >= int64(len(r.window))/2 {
		r.startNextPrefetchLocked()
	}
}

// startNextPrefetchLocked 必须在持有 r.mu 写锁时调用。
func (r *remoteWindowReader) startNextPrefetchLocked() {
	if len(r.window) == 0 {
		return
	}
	nextOff := r.windowOff + int64(len(r.window))
	if r.size > 0 && nextOff >= r.size {
		return
	}
	if r.prefetch != nil {
		if r.prefetch.off == nextOff {
			return
		}
		r.prefetch.cancel()
	}
	length := r.windowSize
	if r.size > 0 && nextOff+length > r.size {
		length = r.size - nextOff
	}
	if length <= 0 {
		r.prefetch = nil
		return
	}
	ctx, cancel := context.WithCancel(r.ctx)
	prefetch := &remoteWindowPrefetch{
		off:    nextOff,
		cancel: cancel,
		done:   make(chan remoteWindowResult, 1),
	}
	r.prefetch = prefetch
	r.prefetchWG.Add(1)
	// 预取不持有 fetchMu：同步读取可直接取消它，避免卡住其它窗口的拉取。
	go func() {
		defer r.prefetchWG.Done()
		data, err := r.fetchWindow(ctx, nextOff, length)
		prefetch.done <- remoteWindowResult{data: data, err: err}
	}()
}

func (r *remoteWindowReader) fetchWindow(ctx context.Context, off, length int64) ([]byte, error) {
	if length <= 0 {
		return []byte{}, nil
	}
	data := make([]byte, int(length))
	parts := int((length + r.partSize - 1) / r.partSize)
	workers := r.concurrency
	if workers > parts {
		workers = parts
	}
	if workers <= 1 {
		if err := r.readRangeInto(ctx, off, data); err != nil {
			return nil, err
		}
		return data, nil
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	worker := func() {
		defer wg.Done()
		for {
			select {
			case <-workerCtx.Done():
				return
			case index, ok := <-jobs:
				if !ok {
					return
				}
				begin := int64(index) * r.partSize
				end := begin + r.partSize
				if end > length {
					end = length
				}
				if err := r.readRangeInto(workerCtx, off+begin, data[begin:end]); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					errMu.Unlock()
					return
				}
			}
		}
	}
	wg.Add(workers)
	for range workers {
		go worker()
	}
	dispatching := true
	for index := 0; index < parts && dispatching; index++ {
		select {
		case jobs <- index:
		case <-workerCtx.Done():
			dispatching = false
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func (r *remoteWindowReader) readRangeInto(ctx context.Context, off int64, dest []byte) error {
	written := 0
	for attempt := 0; written < len(dest); attempt++ {
		n, err := r.readRangeOnce(ctx, off+int64(written), dest[written:])
		written += n
		if written == len(dest) {
			return nil
		}
		if err == nil {
			err = io.ErrNoProgress
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if attempt >= 1 {
			return err
		}
	}
	return nil
}

func (r *remoteWindowReader) readRangeOnce(ctx context.Context, off int64, dest []byte) (int, error) {
	if len(dest) == 0 {
		return 0, nil
	}
	end := off + int64(len(dest)) - 1
	link := r.lh.snapshot()
	for try := 0; try < 2; try++ {
		resp, err := r.svc.doRangeRequest(ctx, r.lh.accountID, link, off, end)
		if err != nil {
			return 0, err
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			_ = resp.Body.Close()
			newLink, refreshed, refreshErr := r.lh.refreshAfterFailure(ctx, link)
			if refreshErr != nil {
				return 0, refreshErr
			}
			if !refreshed {
				return 0, domain.Errorf(domain.CodeDriverError, "上游认证失败")
			}
			link = newLink
			continue
		}
		validFullResponse := resp.StatusCode == http.StatusOK && off == 0 && r.size == int64(len(dest))
		if resp.StatusCode != http.StatusPartialContent && !validFullResponse {
			_ = resp.Body.Close()
			return 0, domain.Errorf(domain.CodeDriverError, "上游 Range 返回 %d", resp.StatusCode)
		}
		if resp.StatusCode == http.StatusPartialContent {
			if start, ok := contentRangeStart(resp.Header.Get("Content-Range")); ok && start != off {
				_ = resp.Body.Close()
				return 0, domain.Errorf(domain.CodeDriverError, "上游 Range 起点不匹配")
			}
		}
		n, readErr := io.ReadFull(resp.Body, dest)
		_ = resp.Body.Close()
		return n, readErr
	}
	return 0, domain.Errorf(domain.CodeDriverError, "上游认证失败")
}

func contentRangeStart(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if len(value) < len("bytes ") || !strings.EqualFold(value[:len("bytes ")], "bytes ") {
		return 0, false
	}
	rangePart := strings.SplitN(value[len("bytes "):], "/", 2)[0]
	startPart := strings.SplitN(rangePart, "-", 2)[0]
	start, err := strconv.ParseInt(strings.TrimSpace(startPart), 10, 64)
	return start, err == nil && start >= 0
}

func (r *remoteWindowReader) close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.prefetch != nil {
		r.prefetch.cancel()
		r.prefetch = nil
	}
	r.mu.Unlock()
	r.prefetchWG.Wait()
	return nil
}

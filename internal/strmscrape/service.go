package strmscrape

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"log/slog"

	"litepan/internal/domain"
	"litepan/internal/eventbus"
	"litepan/internal/mediaorganize"
	"litepan/internal/metatube"
	"litepan/internal/settings"
	"litepan/internal/strm"
)

const (
	defaultItemListLimit = 120
	maxItemListLimit     = 200
)

type Options struct {
	Strm     *strm.Service
	Settings *settings.Service
	Bus      *eventbus.Bus
	DataDir  string
	StrmDir  string
	Log      *slog.Logger
}

type Service struct {
	strm     *strm.Service
	settings *settings.Service
	bus      *eventbus.Bus
	dataDir  string
	strmDir  string
	log      *slog.Logger

	mu          sync.Mutex
	operationMu sync.Mutex
	progress    Progress
	cancel      context.CancelFunc
	indexLocks  sync.Map // taskID -> *sync.Mutex
}

func New(opts Options) *Service {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	strmDir := strings.TrimSpace(opts.StrmDir)
	if strmDir == "" {
		strmDir = filepath.Join(filepath.Dir(filepath.Clean(opts.DataDir)), "strm")
	}
	return &Service{
		strm:     opts.Strm,
		settings: opts.Settings,
		bus:      opts.Bus,
		dataDir:  opts.DataDir,
		strmDir:  strmDir,
		log:      log,
	}
}

func (s *Service) GetProgress() Progress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress
}

func (s *Service) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func normalizeItemListQuery(in ItemListQuery) ItemListQuery {
	out := in
	if out.Offset < 0 {
		out.Offset = 0
	}
	switch {
	case out.Limit <= 0:
		out.Limit = defaultItemListLimit
	case out.Limit > maxItemListLimit:
		out.Limit = maxItemListLimit
	}
	out.Keyword = strings.TrimSpace(out.Keyword)
	out.Status = strings.TrimSpace(out.Status)
	out.MediaType = strings.TrimSpace(out.MediaType)
	out.TVState = strings.TrimSpace(out.TVState)
	switch out.Sort {
	case ItemListSortTitleAsc, ItemListSortYearDesc, ItemListSortYearAsc, ItemListSortAddedAsc, ItemListSortAddedDesc:
	default:
		out.Sort = ItemListSortAddedDesc
	}
	if out.Status != "" && out.Status != ItemStatusOK && out.Status != ItemStatusMiss && out.Status != ItemStatusDoubt {
		out.Status = ""
	}
	if out.MediaType != "" && out.MediaType != MediaTypeMovie && out.MediaType != MediaTypeTV {
		out.MediaType = ""
	}
	if out.TVState != "" && out.TVState != TVStateEnded && out.TVState != TVStateUpdating {
		out.TVState = ""
	}
	return out
}

func (s *Service) ListItems(ctx context.Context, strmTaskID int64, root string, query ItemListQuery) (ItemListResult, error) {
	query = normalizeItemListQuery(query)
	sr, err := s.resolveScrapeRoot(ctx, strmTaskID, root)
	if err != nil {
		return ItemListResult{}, err
	}
	var out ItemListResult
	err = s.withTaskIndexLock(sr.indexKey, func() error {
		if err := s.ensureIndexLocked(ctx, sr.indexKey, sr.root); err != nil {
			return err
		}
		items, err := s.listIndexItems(sr.indexKey, query)
		if err != nil {
			return err
		}
		out = items
		return nil
	})
	return out, err
}

// GetItem 按条目 id 查询单条（影视模式详情页用）。
func (s *Service) GetItem(ctx context.Context, strmTaskID int64, root, id string) (*Item, error) {
	sr, err := s.resolveScrapeRoot(ctx, strmTaskID, root)
	if err != nil {
		return nil, err
	}
	var out *Item
	err = s.withTaskIndexLock(sr.indexKey, func() error {
		if err := s.ensureIndexLocked(ctx, sr.indexKey, sr.root); err != nil {
			return err
		}
		db, err := openTaskIndexDB(s.indexPath(sr.indexKey))
		if err != nil {
			return err
		}
		defer db.Close()
		storedRoot, _ := readIndexMeta(db, "root")
		it, err := getIndexItemByID(db, storedRoot, id)
		if err != nil {
			return err
		}
		out = it
		return nil
	})
	return out, err
}

// RefreshIndex 扫盘重建索引并返回列表（海报墙刷新按钮）。
func (s *Service) RefreshIndex(ctx context.Context, strmTaskID int64, root string, query ItemListQuery) (ItemListResult, error) {
	query = normalizeItemListQuery(query)
	sr, err := s.resolveScrapeRoot(ctx, strmTaskID, root)
	if err != nil {
		return ItemListResult{}, err
	}
	if err := s.RebuildIndex(ctx, strmTaskID, root); err != nil {
		return ItemListResult{}, err
	}
	return s.listIndexItems(sr.indexKey, query)
}

func (s *Service) RunAsync(ctx context.Context, req RunRequest) error {
	if req.StrmTaskID <= 0 && strings.TrimSpace(req.Root) == "" {
		return domain.Errorf(domain.CodeValidation, "strm_task_id 无效")
	}
	_ = ctx // 后台任务不随启动请求结束
	return s.startAsyncOperation(req.StrmTaskID, req.Root, 0, "准备刮削", "刮削完成", "strm scrape failed", func(runCtx context.Context) error {
		return s.run(runCtx, req)
	})
}

func (s *Service) startAsyncOperation(taskID int64, root string, total int, message, doneMessage, logMessage string, run func(context.Context) error) error {
	if !s.operationMu.TryLock() {
		return domain.Errorf(domain.CodeValidation, "刮削任务进行中")
	}
	releaseFiles := func() {}
	// 自定义目录（taskID<=0）与 STRM 任务无文件操作关联，跳过并发守卫。
	if s.strm != nil && taskID > 0 {
		var ok bool
		releaseFiles, ok = s.strm.TryBeginTaskFileOperation(taskID)
		if !ok {
			s.operationMu.Unlock()
			return domain.Errorf(domain.CodeValidation, "该 STRM 任务正在运行，请稍后再刮削")
		}
	}
	s.mu.Lock()
	if s.progress.Running {
		s.mu.Unlock()
		releaseFiles()
		s.operationMu.Unlock()
		return domain.Errorf(domain.CodeValidation, "刮削任务进行中")
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.progress = Progress{
		Running:   true,
		TaskID:    taskID,
		Total:     total,
		Message:   message,
		StartedAt: time.Now().Format(time.RFC3339),
		Root:      root,
	}
	s.mu.Unlock()

	go func() {
		defer releaseFiles()
		defer s.operationMu.Unlock()
		defer cancel()
		err := run(runCtx)
		s.mu.Lock()
		s.progress.Running = false
		s.progress.CurrentItemID = ""
		s.cancel = nil
		if err != nil {
			s.progress.Error = err.Error()
			if s.progress.Message == "" {
				s.progress.Message = "刮削失败"
			}
			s.log.Warn(logMessage, "task_id", taskID, "err", err)
		} else if s.progress.Message == "" {
			s.progress.Message = doneMessage
		}
		s.mu.Unlock()
	}()
	return nil
}

// overwriteForMatch：换 TMDB ID 一律覆盖；同 ID 跟随全局写入策略。
func (s *Service) overwriteForMatch(sameID bool) bool {
	if !sameID {
		return true
	}
	return normalizeWriteMode(s.GetSettings().WriteMode) == WriteModeOverwrite
}

// Rematch 对同 ID 沿用写入策略，换 ID 时强制覆盖并统一走 writeMatchedOpts。
func (s *Service) Rematch(ctx context.Context, req RematchRequest) (*Item, bool, error) {
	if (req.StrmTaskID <= 0 && strings.TrimSpace(req.Root) == "") || strings.TrimSpace(req.ItemID) == "" || strings.TrimSpace(req.TMDBID) == "" {
		return nil, false, domain.Errorf(domain.CodeValidation, "参数不完整")
	}
	sr, err := s.resolveScrapeRoot(ctx, req.StrmTaskID, req.Root)
	if err != nil {
		return nil, false, err
	}
	g, err := findWorkByID(sr.root, req.ItemID)
	if err != nil {
		return nil, false, err
	}
	mediaType := strings.ToLower(strings.TrimSpace(req.MediaType))
	if mediaType == "" {
		mediaType = resolveWorkMediaType(g)
	}
	sameID := workTMDBIDMatches(g, mediaType, req.TMDBID)
	overwrite := s.overwriteForMatch(sameID)
	if _, err := s.requireScrapeClient(); err != nil {
		return nil, false, err
	}
	display := strings.TrimSpace(req.Title)
	if display == "" {
		display = workDisplayName(g)
	}
	current := buildItem(sr.root, g)
	err = s.startAsyncOperation(req.StrmTaskID, req.Root, 1, "正在刮削："+display, "已重新匹配："+display, "strm rematch failed", func(runCtx context.Context) error {
		s.setProgress(func(p *Progress) {
			p.CurrentItemID = req.ItemID
		})
		updated, applyErr := s.applyRematch(runCtx, req, sr.indexKey, sr.root, g, mediaType, overwrite)
		if applyErr != nil {
			s.setProgress(func(p *Progress) {
				p.Done = 1
				p.Failed = 1
				p.CurrentItemID = ""
				p.Message = "重新匹配失败：" + display
			})
			return applyErr
		}
		s.setProgress(func(p *Progress) {
			p.Done = 1
			p.CurrentItemID = ""
			p.ItemRevision++
			p.UpdatedItem = updated
			p.Message = "已重新匹配：" + display
		})
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &current, true, nil
}

func (s *Service) applyRematch(ctx context.Context, req RematchRequest, indexKey int64, root string, g workGroup, mediaType string, overwrite bool) (*Item, error) {
	client, err := s.requireScrapeClient()
	if err != nil {
		return nil, err
	}
	scrapeCtx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()
	raw, err := client.Lookup(scrapeCtx, req.TMDBID, mediaType)
	if err != nil {
		return nil, domain.Errorf(domain.CodeDriverError, "元数据查询失败：%v", err)
	}
	info, err := decodeTMDBInfo(raw, mediaType)
	if err != nil {
		return nil, err
	}
	if title := strings.TrimSpace(req.Title); title != "" {
		info.Title = title
	}
	if req.Year != nil {
		info.Year = req.Year
	}
	info.Doubt = false // 用户手动选定，不再存疑
	_, err = s.writeMatchedOpts(scrapeCtx, client, g, info, overwrite, true)
	if err != nil {
		return nil, err
	}
	s.upsertIndexItem(scrapeCtx, indexKey, root, g)
	item := buildItem(root, g)
	return &item, nil
}

func workTMDBIDMatches(g workGroup, mediaType, tmdbID string) bool {
	meta, ok := readWorkNFOMeta(g, mediaType)
	return ok && strings.TrimSpace(meta.TMDBID) == strings.TrimSpace(tmdbID)
}

func confirmExistingMatch(g workGroup, mediaType string) {
	if pending, ok := readPendingState(g); ok && pending.Status == PendingDoubt {
		finalizeAfterScrape(g, mediaType, pending.EpTMDB, false)
	}
}

func (s *Service) MarkNormal(ctx context.Context, req MarkNormalRequest) (*Item, error) {
	if (req.StrmTaskID <= 0 && strings.TrimSpace(req.Root) == "") || strings.TrimSpace(req.ItemID) == "" {
		return nil, domain.Errorf(domain.CodeValidation, "参数不完整")
	}
	if !s.operationMu.TryLock() {
		return nil, domain.Errorf(domain.CodeValidation, "刮削任务进行中")
	}
	defer s.operationMu.Unlock()
	sr, err := s.resolveScrapeRoot(ctx, req.StrmTaskID, req.Root)
	if err != nil {
		return nil, err
	}
	g, err := findWorkByID(sr.root, req.ItemID)
	if err != nil {
		return nil, err
	}
	mediaType := resolveWorkMediaType(g)
	if pending, ok := readPendingState(g); ok && pending.Status == PendingDoubt {
		if !workHasNFO(g, mediaType) || !workHasPoster(g, mediaType) {
			return nil, domain.Errorf(domain.CodeValidation, "%v", errRootMetaIncomplete)
		}
		confirmExistingMatch(g, mediaType)
	} else {
		if err := markWorkNormal(g, mediaType); err != nil {
			return nil, domain.Errorf(domain.CodeValidation, "%v", err)
		}
	}
	s.upsertIndexItem(ctx, sr.indexKey, sr.root, g)
	item := buildItem(sr.root, g)
	return &item, nil
}

// Rescrape：沿用原 TMDB ID，走与「开始刮削」相同的 writeMatchedOpts（含正片季/finale 集数与 pending）。
func (s *Service) Rescrape(ctx context.Context, req RescrapeRequest) (*Item, bool, error) {
	if (req.StrmTaskID <= 0 && strings.TrimSpace(req.Root) == "") || strings.TrimSpace(req.ItemID) == "" {
		return nil, false, domain.Errorf(domain.CodeValidation, "参数不完整")
	}
	sr, err := s.resolveScrapeRoot(ctx, req.StrmTaskID, req.Root)
	if err != nil {
		return nil, false, err
	}
	g, err := findWorkByID(sr.root, req.ItemID)
	if err != nil {
		return nil, false, err
	}
	mediaType := resolveWorkMediaType(g)
	meta, ok := readWorkNFOMeta(g, mediaType)
	if !ok || strings.TrimSpace(meta.TMDBID) == "" {
		return nil, false, domain.Errorf(domain.CodeValidation, "缺少 TMDB ID，请先重新匹配")
	}
	display := strings.TrimSpace(meta.Title)
	if display == "" {
		display = workDisplayName(g)
	}
	current := buildItem(sr.root, g)
	overwrite := s.overwriteForMatch(true)
	if _, err := s.requireScrapeClient(); err != nil {
		return nil, false, err
	}
	err = s.startAsyncOperation(req.StrmTaskID, req.Root, 1, "正在重新刮削："+display, "已重新刮削："+display, "strm rescrape failed", func(runCtx context.Context) error {
		s.setProgress(func(p *Progress) {
			p.CurrentItemID = req.ItemID
		})
		updated, applyErr := s.applyRematch(runCtx, RematchRequest{
			StrmTaskID: req.StrmTaskID,
			ItemID:     req.ItemID,
			TMDBID:     meta.TMDBID,
			MediaType:  mediaType,
			Title:      meta.Title,
			Year:       meta.Year,
			Root:       req.Root,
		}, sr.indexKey, sr.root, g, mediaType, overwrite)
		if applyErr != nil {
			s.setProgress(func(p *Progress) {
				p.Done = 1
				p.Failed = 1
				p.CurrentItemID = ""
				p.Message = "重新刮削失败：" + display
			})
			return applyErr
		}
		s.setProgress(func(p *Progress) {
			p.Done = 1
			p.CurrentItemID = ""
			p.ItemRevision++
			p.UpdatedItem = updated
			p.Message = "已重新刮削：" + display
		})
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &current, true, nil
}

// ResolvePosterFile 解析海报文件的本地绝对路径。
// root 非空时为自定义目录直接使用；否则回退到 STRM 任务的输出目录（兼容旧海报 URL）。
func (s *Service) ResolvePosterFile(ctx context.Context, strmTaskID int64, root, rel string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		sr, serr := s.resolveScrapeRoot(ctx, strmTaskID, "")
		if serr != nil {
			return "", serr
		}
		root = sr.root
	} else {
		if abs, aerr := filepath.Abs(root); aerr == nil {
			root = abs
		}
		st, serr := os.Stat(root)
		if serr != nil || !st.IsDir() {
			return "", domain.Errorf(domain.CodeValidation, "目录不存在或不是文件夹：%s", root)
		}
	}
	rel = filepath.Clean("/" + strings.TrimSpace(rel))
	rel = strings.TrimPrefix(rel, "/")
	full := filepath.Join(root, rel)
	if !isInside(root, full) {
		return "", domain.Errorf(domain.CodeValidation, "非法路径")
	}
	base := strings.ToLower(filepath.Base(full))
	if !strings.HasSuffix(base, ".jpg") && !strings.HasSuffix(base, ".png") && !strings.HasSuffix(base, ".webp") {
		return "", domain.Errorf(domain.CodeValidation, "仅允许图片文件")
	}
	if !fileExists(full) {
		return "", domain.Errorf(domain.CodeNotFound, "海报不存在")
	}
	return full, nil
}

func (s *Service) run(ctx context.Context, req RunRequest) error {
	sr, err := s.resolveScrapeRoot(ctx, req.StrmTaskID, req.Root)
	if err != nil {
		return err
	}
	task, root := sr.task, sr.root
	failures := make([]ScrapeFailure, 0)
	defer func() {
		s.notifyScrapeFailures(task, failures)
	}()
	mode := normalizeWriteMode(s.GetSettings().WriteMode)
	if strings.TrimSpace(req.WriteMode) != "" {
		mode = normalizeWriteMode(req.WriteMode)
	}
	client, err := s.requireScrapeClient()
	if err != nil {
		return err
	}
	if abs, aerr := filepath.Abs(root); aerr == nil {
		root = abs
	}
	if st, serr := os.Stat(root); serr != nil || !st.IsDir() {
		if serr != nil && os.IsNotExist(serr) {
			return domain.Errorf(domain.CodeValidation, "STRM 输出目录不存在：%s", root)
		}
		return domain.Errorf(domain.CodeValidation, "STRM 输出目录无效：%s", root)
	}
	works, err := scanWorks(root)
	if err != nil {
		return err
	}
	if len(works) == 0 {
		s.setProgress(func(p *Progress) {
			p.Total = 0
			p.Done = 0
			p.Skipped = 0
			p.Failed = 0
			p.CurrentItemID = ""
			p.Message = "未找到 .strm 文件"
			p.Error = ""
		})
		return domain.Errorf(domain.CodeValidation, "输出目录中没有 .strm 文件：%s", root)
	}
	s.setProgress(func(p *Progress) {
		p.Total = len(works)
		p.Done = 0
		p.Skipped = 0
		p.Failed = 0
		p.Message = "扫描完成，按作品开始匹配"
		p.Error = ""
	})

	interval := time.Duration(s.GetSettings().TmdbRequestIntervalMS) * time.Millisecond
	if interval < 200*time.Millisecond {
		interval = 300 * time.Millisecond
	}

	for i, g := range works {
		if err := ctx.Err(); err != nil {
			return err
		}
		displayName := workDisplayName(g)
		item := buildItem(root, g)
		need := mode == WriteModeOverwrite || workNeedsScrape(g, item.MediaType)
		if !need {
			s.setProgress(func(p *Progress) {
				p.Done = i + 1
				p.Skipped++
				p.CurrentItemID = ""
			})
			continue
		}
		s.setProgress(func(p *Progress) {
			p.CurrentItemID = item.ID
			p.Message = "正在刮削：" + displayName
		})
		info, matchErr := s.matchWork(ctx, client, g)
		if err := ctx.Err(); err != nil {
			return err
		}
		if matchErr != nil || info == nil || info.TMDBID == "" {
			reason := "未返回有效的 TMDB 匹配结果"
			if matchErr != nil {
				reason = matchErr.Error()
			}
			failures = append(failures, s.logScrapeFailure(task, g, displayName, ScrapeFailureStageMatch, reason))
			s.setProgress(func(p *Progress) {
				p.Done = i + 1
				p.Failed++
				p.CurrentItemID = ""
				p.Message = "匹配失败：" + displayName
			})
			time.Sleep(interval)
			continue
		}
		title := strings.TrimSpace(info.Title)
		if title == "" {
			title = displayName
		}
		s.setProgress(func(p *Progress) {
			p.Message = "正在刮削：" + title
		})
		if err := s.writeMatched(ctx, client, g, *info, mode == WriteModeOverwrite); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			failures = append(failures, s.logScrapeFailure(task, g, title, ScrapeFailureStageWrite, err.Error()))
			s.setProgress(func(p *Progress) {
				p.Done = i + 1
				p.Failed++
				p.CurrentItemID = ""
				p.Message = "写入失败：" + title
			})
			time.Sleep(interval)
			continue
		}
		s.upsertIndexItem(ctx, sr.indexKey, root, g)
		updated := buildItem(root, g)
		s.setProgress(func(p *Progress) {
			p.Done = i + 1
			p.CurrentItemID = ""
			p.ItemRevision++
			p.UpdatedItem = &updated
			p.Message = "已刮削：" + title
		})
		time.Sleep(interval)
	}
	// 全量对账一次，去掉已删除作品
	_ = s.RebuildIndex(ctx, req.StrmTaskID, req.Root)
	s.setProgress(func(p *Progress) {
		p.CurrentItemID = ""
		p.Message = fmt.Sprintf("完成：成功 %d，跳过 %d，失败 %d", p.Done-p.Skipped-p.Failed, p.Skipped, p.Failed)
	})
	return nil
}

func (s *Service) resolveTask(ctx context.Context, id int64) (*domain.StrmTask, string, error) {
	if s.strm == nil {
		return nil, "", domain.Errorf(domain.CodeInternal, "strm 服务未装配")
	}
	task, err := s.strm.GetTask(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if task == nil {
		return nil, "", domain.Errorf(domain.CodeNotFound, "STRM 任务不存在")
	}
	root := strm.TaskOutputDir(s.strmDir, task.OutputFolder)
	if root == "" {
		return nil, "", domain.Errorf(domain.CodeValidation, "输出目录无效")
	}
	// 统一成绝对路径，避免 upsert 时 Abs(root) 与相对扫盘的 g.absDir 拼出错误 poster_rel。
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return task, root, nil
}

// scrapeRoot 是一次刮削操作的根目录解析结果。
type scrapeRoot struct {
	task     *domain.StrmTask // 自定义目录模式下为 nil
	root     string           // 刮削根目录绝对路径
	indexKey int64            // 索引键：自定义目录为路径派生负数键，任务模式为任务 ID
}

// resolveScrapeRoot 解析本次刮削的根目录：
// root 非空 → 自定义目录模式：校验目录存在，索引键用路径派生的负数键（task 为 nil）；
// root 为空 → 任务模式：走 resolveTask，索引键即任务 ID。
func (s *Service) resolveScrapeRoot(ctx context.Context, strmTaskID int64, root string) (scrapeRoot, error) {
	if rt := strings.TrimSpace(root); rt != "" {
		if abs, err := filepath.Abs(rt); err == nil {
			rt = abs
		}
		st, err := os.Stat(rt)
		if err != nil || !st.IsDir() {
			return scrapeRoot{}, domain.Errorf(domain.CodeValidation, "自定义目录不存在或不是文件夹：%s", rt)
		}
		return scrapeRoot{root: rt, indexKey: CustomRootTaskID(rt)}, nil
	}
	task, resolved, err := s.resolveTask(ctx, strmTaskID)
	if err != nil {
		return scrapeRoot{}, err
	}
	return scrapeRoot{task: task, root: resolved, indexKey: strmTaskID}, nil
}

func (s *Service) setProgress(fn func(*Progress)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.progress)
}

// Search 按当前刮削数据源搜索候选，返回带 media_type 的 TMDB 形状命中（手动重新匹配用）。
func (s *Service) Search(ctx context.Context, query string, year *int, mediaType string) ([]json.RawMessage, error) {
	client, err := s.requireScrapeClient()
	if err != nil {
		return nil, err
	}
	mt := strings.ToLower(strings.TrimSpace(mediaType))
	if mt == "" {
		mt = "auto"
	}
	if mt == "auto" {
		// MetaTube 源仅电影，避免同一批结果按 movie/tv 各查一遍造成重复。
		if s.GetSettings().Source == SourceMetaTube {
			results, err := client.Search(ctx, query, year, MediaTypeMovie)
			if err != nil {
				return nil, err
			}
			return injectScrapeMediaType(results, MediaTypeMovie), nil
		}
		movies, err := client.Search(ctx, query, year, MediaTypeMovie)
		if err != nil {
			return nil, err
		}
		tvs, err := client.Search(ctx, query, year, MediaTypeTV)
		if err != nil {
			return nil, err
		}
		out := make([]json.RawMessage, 0, len(movies)+len(tvs))
		out = append(out, injectScrapeMediaType(movies, MediaTypeMovie)...)
		out = append(out, injectScrapeMediaType(tvs, MediaTypeTV)...)
		return out, nil
	}
	results, err := client.Search(ctx, query, year, mt)
	if err != nil {
		return nil, err
	}
	return injectScrapeMediaType(results, mt), nil
}

// TestProvider 测试当前刮削数据源连通性，返回可读的测试结果。
// overrideMetaTubeURL 用于测试表单里尚未保存的地址（空则用已保存的配置）。
func (s *Service) TestProvider(ctx context.Context, overrideMetaTubeURL string) (map[string]any, error) {
	cfg := s.GetSettings()
	if cfg.Source == SourceMetaTube {
		baseURL := strings.TrimSpace(cfg.MetaTubeURL)
		if url := strings.TrimSpace(overrideMetaTubeURL); url != "" {
			baseURL = url
		}
		if baseURL == "" {
			return nil, domain.Errorf(domain.CodeValidation, "请先填写 MetaTube API 地址再测试")
		}
		client := metatube.NewClient(metatube.Options{BaseURL: baseURL})
		if !client.ValidateConnection(ctx) {
			return nil, domain.Errorf(domain.CodeValidation, "MetaTube 不可达，请检查地址或网络")
		}
		return map[string]any{"ok": true, "source": SourceMetaTube, "url": baseURL}, nil
	}
	// TMDB 测试沿用目录整理的校验（共用同一套 TMDB/代理配置）。
	enriched := mediaorganize.EnrichPlannerSettings(s.settings, nil)
	tmdbClient := s.newTMDBClient()
	if tmdbClient == nil {
		return nil, domain.Errorf(domain.CodeValidation, "请先填写 TMDB API Key 再测试")
	}
	ok := tmdbClient.ValidateConnection(ctx)
	if !ok {
		return nil, domain.Errorf(domain.CodeValidation, "TMDB 不可达，请检查 API Key、网络或代理配置")
	}
	return map[string]any{"ok": true, "source": SourceTMDB, "language": mediaorganize.PlannerTMDBLanguage(enriched)}, nil
}

func injectScrapeMediaType(results []json.RawMessage, mediaType string) []json.RawMessage {
	if len(results) == 0 {
		return results
	}
	out := make([]json.RawMessage, 0, len(results))
	for _, raw := range results {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			out = append(out, raw)
			continue
		}
		m["media_type"] = mediaType
		b, err := json.Marshal(m)
		if err != nil {
			out = append(out, raw)
			continue
		}
		out = append(out, b)
	}
	return out
}

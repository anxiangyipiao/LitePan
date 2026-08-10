package strmscrape

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"litepan/internal/mediaorganize/rules"
	"litepan/internal/mediaorganize/tmdb"
)

type tmdbInfo struct {
	TMDBID       string
	Title        string
	Original     string
	Year         *int
	Plot         string
	PosterPath   string
	BackdropPath string
	MediaType    string
	Doubt        bool
	EpisodeCount int // 默认全剧集数；刮削时会按本地已有季收窄

	// 富化字段（电影 NFO）：TMDB 与 MetaTube 均可提供，缺失时留空省略。
	Genres   []string
	Studio   string
	Director string
	Actors   []string
	Runtime  int

	// MetaTube 源附加字段：MetaTubeID 为 provider 侧 ID（详情/图片端点用），
	// MetaTubeNumber 为番号（NFO uniqueid 用），MetaTubeProvider 为 provider 名（背景图端点用）。
	MetaTubeID       string
	MetaTubeNumber   string
	MetaTubeProvider string

	// PreviewImages 是详情返回的预览截图（MetaTube），用作多张轮播背景图素材；TMDB 源为空。
	PreviewImages []string
}

func (s *Service) matchWork(ctx context.Context, client scrapeSource, g workGroup) (*tmdbInfo, error) {
	mediaType := inferMediaType(g)
	folderName := workDisplayName(g)
	dirParsed := rules.NormalizeParsedMedia(rules.ParseDirName(folderName))

	var fileParses []rules.ParsedMedia
	for _, e := range g.entries {
		stem := strings.TrimSuffix(filepath.Base(e.absPath), filepath.Ext(e.absPath))
		fileParses = append(fileParses, rules.NormalizeParsedMedia(rules.ParseFilenameStrict(stem+".mkv")))
	}

	if id := rules.FindTMDBIDInName(folderName); id != "" {
		if info, err := lookupTMDBInfo(ctx, client, id, mediaType); err == nil {
			return info, nil
		}
	}
	for _, e := range g.entries {
		stem := strings.TrimSuffix(filepath.Base(e.absPath), filepath.Ext(e.absPath))
		if id := rules.FindTMDBIDInName(stem); id != "" {
			if info, err := lookupTMDBInfo(ctx, client, id, mediaType); err == nil {
				return info, nil
			}
		}
	}

	// JAV 番号：番号本身唯一标识作品，走专用匹配（搜索番号→精确匹配→不存疑），
	// 避免现有解析器把数字当成季/集号导致查询残缺，也避免模糊标题匹配触发存疑。
	if jav := workJAVNumber(g); jav != "" {
		return s.matchJAVNumber(ctx, client, jav, mediaType)
	}

	title := strings.TrimSpace(dirParsed.Title)
	year := dirParsed.Year
	if title == "" {
		for _, p := range fileParses {
			if strings.TrimSpace(p.Title) != "" {
				title = strings.TrimSpace(p.Title)
				if year == nil {
					year = p.Year
				}
				break
			}
		}
	}
	if title == "" {
		title = folderName
	}
	if title == "" {
		return nil, fmt.Errorf("无法解析标题")
	}

	info, err := searchTMDBInfo(ctx, client, title, year, mediaType)
	if err != nil && mediaType == MediaTypeTV {
		// 误判成剧集时回退电影搜索
		info, err = searchTMDBInfo(ctx, client, title, year, MediaTypeMovie)
	}
	if err != nil {
		return nil, err
	}
	if info.EpisodeCount == 0 && info.MediaType == MediaTypeTV {
		if raw, lerr := client.Lookup(ctx, info.TMDBID, MediaTypeTV); lerr == nil {
			if full, derr := decodeTMDBInfo(raw, MediaTypeTV); derr == nil && full.EpisodeCount > 0 {
				info.EpisodeCount = full.EpisodeCount
			}
		}
	}
	return info, nil
}

func lookupTMDBInfo(ctx context.Context, client scrapeSource, id, mediaType string) (*tmdbInfo, error) {
	order := []string{mediaType}
	if mediaType == MediaTypeTV {
		order = append(order, MediaTypeMovie)
	} else {
		order = append(order, MediaTypeTV)
	}
	var lastErr error
	for _, mt := range order {
		raw, err := client.Lookup(ctx, id, mt)
		if err != nil {
			lastErr = err
			continue
		}
		info, derr := decodeTMDBInfo(raw, mt)
		if derr != nil {
			lastErr = derr
			continue
		}
		return &info, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("TMDB 查询失败")
	}
	return nil, lastErr
}

func searchTMDBInfo(ctx context.Context, client scrapeSource, title string, year *int, mediaType string) (*tmdbInfo, error) {
	results, err := client.Search(ctx, title, year, mediaType)
	if err != nil {
		return nil, err
	}
	var best map[string]any
	doubt := false
	if year == nil {
		best, doubt = pickTMDBScrapeMatch(rules.RawJSONListToMaps(results), nil, mediaType, title)
	} else {
		// 带年份的第一次查询只接受完全相等；±1 年必须在不限年份的完整候选中判断唯一性。
		best = rules.PickTMDBSearchMatchForYear(rules.RawJSONListToMaps(results), year, mediaType, title)
	}
	if best == nil && year != nil {
		results, err = client.Search(ctx, title, nil, mediaType)
		if err != nil {
			return nil, err
		}
		best, doubt = pickTMDBScrapeMatch(rules.RawJSONListToMaps(results), year, mediaType, title)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("无搜索结果")
	}
	if best == nil {
		if year != nil {
			return nil, fmt.Errorf("没有标题相符且年份为 %d 或唯一相邻年份的结果", *year)
		}
		return nil, fmt.Errorf("没有标题相符的结果")
	}
	// MetaTube 搜索命中缺 summary/genres 等详情，经 EnrichSearchResult 补齐；TMDB 源原样返回。
	raw, err := client.EnrichSearchResult(ctx, mustRaw(best), mediaType)
	if err != nil {
		raw = mustRaw(best)
	}
	info, err := decodeTMDBInfo(raw, mediaType)
	if err != nil {
		return nil, err
	}
	info.Doubt = doubt
	return &info, nil
}

func pickTMDBScrapeMatch(results []map[string]any, year *int, mediaType, title string) (map[string]any, bool) {
	if best := rules.PickTMDBSearchMatchForYear(results, year, mediaType, title); best != nil {
		return best, year == nil && len(results) > 1
	}
	if best := rules.PickUniqueTMDBAdjacentYearMatch(results, year, mediaType, title); best != nil {
		return best, true
	}
	return nil, false
}

// matchJAVNumber 按 JAV 番号专用匹配：搜索番号 → 精确命中番号 → 前缀/后缀或标题兼容兜底。
// 番号查询本身足够精确，命中不触发存疑（避免多 provider / 规范化番号导致的误判）。
func (s *Service) matchJAVNumber(ctx context.Context, client scrapeSource, javNumber, mediaType string) (*tmdbInfo, error) {
	raws, err := client.Search(ctx, javNumber, nil, mediaType)
	if err != nil {
		return nil, err
	}
	if len(raws) == 0 {
		return nil, fmt.Errorf("无搜索结果")
	}
	best, _ := pickJAVMatch(rules.RawJSONListToMaps(raws), javNumber)
	if best == nil {
		return nil, fmt.Errorf("没有番号相符的结果")
	}
	raw, err := client.EnrichSearchResult(ctx, mustRaw(best), mediaType)
	if err != nil {
		raw = mustRaw(best)
	}
	info, err := decodeTMDBInfo(raw, mediaType)
	if err != nil {
		return nil, err
	}
	info.Doubt = false
	return &info, nil
}

// pickJAVMatch 在番号搜索结果中挑出与查询番号一致的命中。优先番号精确相等，
// 其次查询是存储番号的前缀/后缀（MetaTube 可能规范化掉数字前缀，如 390JAC-132 → JAC-132），
// 最后按标题兼容兜底。
func pickJAVMatch(results []map[string]any, query string) (map[string]any, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, false
	}
	numberKeys := func(item map[string]any) []string {
		var out []string
		for _, k := range []string{"_metatube_number", "_metatube_id", "original_title", "id"} {
			if v := strings.ToLower(nonNilString(item[k])); v != "" {
				out = append(out, v)
			}
		}
		return out
	}
	for _, item := range results {
		for _, v := range numberKeys(item) {
			if v == q {
				return item, false
			}
		}
	}
	for _, item := range results {
		for _, v := range numberKeys(item) {
			if v != "" && (strings.HasSuffix(q, v) || strings.HasSuffix(v, q)) {
				return item, false
			}
		}
	}
	for _, item := range results {
		_, t, o, _ := rules.ExtractTMDBDisplayFields(item, MediaTypeMovie)
		if rules.IsTMDBTitleCompatible(query, t, o) {
			return item, false
		}
	}
	return nil, false
}

func (s *Service) writeMatched(ctx context.Context, client scrapeSource, g workGroup, info tmdbInfo, overwrite bool) error {
	_, err := s.writeMatchedOpts(ctx, client, g, info, overwrite, true)
	return err
}

func (s *Service) writeMatchedOpts(ctx context.Context, client scrapeSource, g workGroup, info tmdbInfo, overwrite, withTVExtras bool) (epTMDB int, err error) {
	mediaType := info.MediaType
	if mediaType == "" {
		mediaType = inferMediaType(g)
		info.MediaType = mediaType
	}
	epTMDB = info.EpisodeCount
	if mediaType == MediaTypeTV && g.flatFile == "" && strings.TrimSpace(info.TMDBID) != "" {
		if n, cerr := tmdbEpisodeCountForLocalSeasons(ctx, client, g, info.TMDBID); cerr == nil && n > 0 {
			epTMDB = n
		}
	}
	epLocal, _ := countTVEpisodeProgress(g)
	if err := writePendingState(g, scrapeState{
		Status:  PendingRunning,
		EpLocal: epLocal,
		EpTMDB:  epTMDB,
	}); err != nil {
		return 0, err
	}
	needTVExtras := mediaType == MediaTypeTV && g.flatFile == "" && strings.TrimSpace(info.TMDBID) != ""
	nfo, poster := workMetaPaths(g, mediaType)
	if overwrite || !fileExists(nfo) {
		if mediaType == MediaTypeTV {
			if err := writeTVShowNFO(nfo, info.Title, info.TMDBID, info.Plot, info.Year); err != nil {
				return 0, err
			}
		} else if err := writeMovieNFO(nfo, info); err != nil {
			return 0, err
		}
	}
	if (overwrite || !fileExists(poster)) && strings.TrimSpace(info.PosterPath) != "" {
		data, err := client.DownloadImage(ctx, info.PosterPath, "w500")
		if err != nil {
			return 0, err
		}
		if err := writeImageFile(poster, data); err != nil {
			return 0, err
		}
	}
	if mediaType == MediaTypeMovie {
		if err := s.writeMovieExtras(ctx, client, g, info, overwrite); err != nil {
			return epTMDB, fmt.Errorf("补写电影背景图失败：%w", err)
		}
	}
	if withTVExtras && needTVExtras {
		if err := s.writeTVExtras(ctx, client, g, info, overwrite); err != nil {
			return epTMDB, fmt.Errorf("补写季/集元数据失败：%w", err)
		}
	}
	// 异步补季/集时由调用方 finalize；此处同步路径直接收尾
	if withTVExtras || !needTVExtras {
		finalizeAfterScrape(g, mediaType, epTMDB, info.Doubt)
	}
	return epTMDB, nil
}

// tmdbEpisodeCountForLocalSeasons 按 finale 截断正片季，避免跨季绝对集号被误当总集数。
func tmdbEpisodeCountForLocalSeasons(ctx context.Context, client scrapeSource, g workGroup, tmdbID string) (int, error) {
	seasons := listLocalRegularSeasonNumbers(g)
	if client == nil || strings.TrimSpace(tmdbID) == "" || len(seasons) == 0 {
		return 0, fmt.Errorf("无本地正片季")
	}
	rawSeasons, err := client.FetchTVSeasons(ctx, tmdbID)
	if err != nil {
		return 0, err
	}
	fallback := tmdbSeasonEpisodeCountMap(rawSeasons)
	total := 0
	for _, sn := range seasons {
		n := 0
		if detail, derr := fetchSeasonDetail(ctx, client, tmdbID, sn); derr == nil {
			n = effectiveSeasonEpisodeCount(detail, fallback[sn])
		} else {
			n = fallback[sn]
		}
		if n > 0 {
			total += n
		}
	}
	if total <= 0 {
		return 0, fmt.Errorf("本地季在 TMDB 无集数")
	}
	return total, nil
}

func tmdbSeasonEpisodeCountMap(rawSeasons []json.RawMessage) map[int]int {
	out := map[int]int{}
	for _, raw := range rawSeasons {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		num := asInt(m["season_number"])
		ep := asInt(m["episode_count"])
		if num == nil || ep == nil || *num <= 0 || *ep <= 0 {
			continue
		}
		out[*num] = *ep
	}
	return out
}

func sumTMDBSeasonEpisodeCounts(rawSeasons []json.RawMessage, seasons []int) int {
	counts := tmdbSeasonEpisodeCountMap(rawSeasons)
	total := 0
	for _, sn := range seasons {
		if sn > 0 {
			total += counts[sn]
		}
	}
	return total
}

// effectiveSeasonEpisodeCount 有 finale 时按集列表计数，否则保留 episode_count。
func effectiveSeasonEpisodeCount(detail *tmdbSeasonDetail, fallback int) int {
	fin := finaleEpisodeNumber(detail)
	if fin <= 0 {
		return fallback
	}
	n := 0
	for _, ep := range detail.Episodes {
		if ep.EpisodeNumber > 0 && ep.EpisodeNumber <= fin {
			n++
		}
	}
	if n > 0 {
		return n
	}
	return fallback
}

func finaleEpisodeNumber(detail *tmdbSeasonDetail) int {
	if detail == nil {
		return 0
	}
	best := 0
	for _, ep := range detail.Episodes {
		if ep.EpisodeType != "finale" || ep.EpisodeNumber <= 0 {
			continue
		}
		if ep.EpisodeNumber > best {
			best = ep.EpisodeNumber
		}
	}
	return best
}

func (s *Service) writeSeasonPosters(ctx context.Context, client scrapeSource, showDir, tmdbID string, overwrite bool) error {
	seasons := listLocalSeasonNumbers(showDir)
	if len(seasons) == 0 {
		return nil
	}
	rawSeasons, err := client.FetchTVSeasons(ctx, tmdbID)
	if err != nil {
		return err
	}
	byNum := map[int]string{}
	for _, raw := range rawSeasons {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		num := asInt(m["season_number"])
		if num == nil {
			continue
		}
		poster := strings.TrimSpace(anyString(m["poster_path"]))
		if poster == "" {
			continue
		}
		byNum[*num] = poster
	}
	for _, season := range seasons {
		posterPath := byNum[season]
		if posterPath == "" {
			continue
		}
		out := seasonPosterPath(showDir, season)
		if !overwrite && fileExists(out) {
			continue
		}
		if err := s.writeOptionalArtwork(ctx, client, posterPath, out, fmt.Sprintf("第 %d 季海报", season)); err != nil {
			return err
		}
	}
	return nil
}

func asInt(v any) *int {
	switch t := v.(type) {
	case float64:
		n := int(t)
		return &n
	case int:
		return &t
	case int64:
		n := int(t)
		return &n
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			return nil
		}
		n := int(i)
		return &n
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return nil
		}
		return &i
	default:
		return nil
	}
}

func decodeTMDBInfo(raw json.RawMessage, mediaType string) (tmdbInfo, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return tmdbInfo{}, err
	}
	id, title, original, year := rules.ExtractTMDBDisplayFields(m, mediaType)
	plot := nonNilString(m["overview"])
	poster := nonNilString(m["poster_path"])
	backdrop := nonNilString(m["backdrop_path"])
	if id == "" || title == "" {
		return tmdbInfo{}, fmt.Errorf("TMDB 结果缺少标题")
	}
	epCount := 0
	if n := asInt(m["number_of_episodes"]); n != nil && *n > 0 {
		epCount = *n
	}
	info := tmdbInfo{
		TMDBID:       id,
		Title:        title,
		Original:     original,
		Year:         year,
		Plot:         plot,
		PosterPath:   poster,
		BackdropPath: backdrop,
		MediaType:    mediaType,
		EpisodeCount: epCount,
		Genres:       extractStringList(m["genres"]),
		Studio:       firstNonEmpty(nonNilString(m["studio"]), nonNilString(m["maker"])),
		Director:     nonNilString(m["director"]),
		Actors:       extractStringList(m["actors"]),
		MetaTubeID:       nonNilString(m["_metatube_id"]),
		MetaTubeNumber:   nonNilString(m["_metatube_number"]),
		MetaTubeProvider: nonNilString(m["_metatube_provider"]),
		PreviewImages:    extractStringList(m["_metatube_preview_images"]),
	}
	if n := asInt(m["runtime"]); n != nil && *n > 0 {
		info.Runtime = *n
	}
	return info, nil
}

// nonNilString 对 nil 安全的字符串取值，避免 anyString(nil) 返回 "<nil>"。
func nonNilString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(anyString(v))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// extractStringList 兼容字符串数组（MetaTube）与 [{"name": ...}]（TMDB genres）两种形状。
func extractStringList(v any) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range arr {
		if s := nonNilString(item); s != "" {
			out = append(out, s)
			continue
		}
		if m, ok := item.(map[string]any); ok {
			if n := nonNilString(m["name"]); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

func mustRaw(m map[string]any) json.RawMessage {
	b, _ := json.Marshal(m)
	return b
}

func (s *Service) newTMDBClient() *tmdb.Client {
	cfg := s.GetSettings()
	apiKey := strings.TrimSpace(cfg.TmdbAPIKey)
	if apiKey == "" {
		return nil
	}
	proxy := tmdb.BuildProxyURL(tmdb.ProxyConfig{
		Enabled:  cfg.ProxyEnabled,
		URL:      cfg.ProxyURL,
		Username: cfg.ProxyUsername,
		Password: cfg.ProxyPassword,
	})
	return tmdb.NewClient(tmdb.Options{
		APIKey:         apiKey,
		Language:       cfg.TmdbLanguage,
		ProxyURL:       proxy,
		Timeout:        20 * time.Second,
		MaxRetries:     2,
		RetryBaseDelay: time.Second,
	})
}

// Package medialibrary 提供「影视模式」：读取 STRM 刮削输出的电影/剧集库，
// 从可配置的服务器本地根目录聚合为海报墙数据，并解析出可播放地址。
package medialibrary

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/mediaorganize/rules"
	"litepan/internal/settings"
	"litepan/internal/strm"
	"litepan/internal/strmscrape"
)

// Root 一个影视库根目录配置（服务器本地刮削输出目录）。
type Root struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// Item 影视模式展示条目：strmscrape.Item 的公开子集 + 库 id 与播放地址。
type Item struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Year       *int   `json:"year,omitempty"`
	MediaType  string `json:"media_type"`
	Status     string `json:"status"`
	TMDBID     string `json:"tmdb_id,omitempty"`
	PosterURL  string `json:"poster_url,omitempty"`
	FolderName string `json:"folder_name,omitempty"`
	FileCount  int    `json:"file_count"`
	EpLocal    int    `json:"ep_local,omitempty"`
	EpTMDB     int    `json:"ep_tmdb,omitempty"`
	EpScraped  int    `json:"ep_scraped,omitempty"`
	TVState    string `json:"tv_state,omitempty"`
	LibID      string `json:"lib_id"`
	PlayURL    string `json:"play_url,omitempty"`
}

// ItemListResult 聚合后的条目列表。
type ItemListResult struct {
	Items   []Item `json:"items"`
	Total   int    `json:"total"`
	HasMore bool   `json:"has_more"`
}

// Episode 剧集的单集信息（详情页选集播放）。
type Episode struct {
	Season  int    `json:"season,omitempty"`
	Episode int    `json:"episode"`
	Title   string `json:"title,omitempty"`
	PlayURL string `json:"play_url"`
}

// Detail 影视条目详情：元数据 + 简介 + 背景图 + 播放地址（剧集含选集列表）。
type Detail struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Year            *int      `json:"year,omitempty"`
	MediaType       string    `json:"media_type"`
	TMDBID          string    `json:"tmdb_id,omitempty"`
	FolderName      string    `json:"folder_name,omitempty"`
	FileCount       int       `json:"file_count"`
	EpLocal         int       `json:"ep_local,omitempty"`
	EpTMDB          int       `json:"ep_tmdb,omitempty"`
	EpScraped       int       `json:"ep_scraped,omitempty"`
	TVState         string    `json:"tv_state,omitempty"`
	Status          string    `json:"status"`
	PosterURL       string    `json:"poster_url,omitempty"`
	BackdropURL     string    `json:"backdrop_url,omitempty"`
	Overview        string    `json:"overview,omitempty"`
	Genres          []string  `json:"genres,omitempty"`           // nfo 类型标签
	Runtime         string    `json:"runtime,omitempty"`         // 时长（分钟）
	Studio          string    `json:"studio,omitempty"`          // 制片/发行
	Director        string    `json:"director,omitempty"`
	Actors          []string  `json:"actors,omitempty"`          // nfo 演员列表
	ExtraFanartURLs []string  `json:"extra_fanart_urls,omitempty"` // extrafanart/ 轮播背景图
	PlayURL         string    `json:"play_url,omitempty"`        // 电影直播；剧集为首集
	Episodes        []Episode `json:"episodes,omitempty"`        // 剧集选集列表
}

// mergeCap 跨库合并时单库最多拉取的条目数（首几页分页正确即可，超限截断）。
const mergeCap = 2000

// Service 影视模式服务。依赖 settings（根目录配置）与 strmscrape（索引/海报）。
type Service struct {
	settings *settings.Service
	scrape   *strmscrape.Service
	log      *slog.Logger
}

func New(set *settings.Service, scrape *strmscrape.Service, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{settings: set, scrape: scrape, log: log}
}

// Roots 读取配置的影视库根目录。
func (s *Service) Roots(ctx context.Context) ([]Root, error) {
	raw := ""
	if s.settings != nil {
		raw = s.settings.String(settings.KeyMediaLibraryRoots)
	}
	roots := []Root{}
	raw = strings.TrimSpace(raw)
	if raw != "" && raw != "[]" {
		if err := json.Unmarshal([]byte(raw), &roots); err != nil {
			return nil, domain.Errorf(domain.CodeValidation, "影视库根目录配置格式错误")
		}
	}
	return roots, nil
}

// SaveRoots 保存影视库根目录配置（校验路径非空、去空项）。
func (s *Service) SaveRoots(ctx context.Context, roots []Root) error {
	if s.settings == nil {
		return domain.Errorf(domain.CodeInternal, "设置未就绪")
	}
	out := make([]Root, 0, len(roots))
	seen := map[string]struct{}{}
	for _, r := range roots {
		r.Name = strings.TrimSpace(r.Name)
		r.Path = strings.TrimSpace(r.Path)
		r.ID = strings.TrimSpace(r.ID)
		if r.Name == "" {
			r.Name = r.Path
		}
		if r.Path == "" {
			return domain.Errorf(domain.CodeValidation, "根目录路径不能为空")
		}
		if r.ID == "" {
			r.ID = fmt.Sprintf("lib%d", len(out)+1)
		}
		if _, dup := seen[r.ID]; dup {
			continue
		}
		seen[r.ID] = struct{}{}
		out = append(out, r)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return domain.Wrap(domain.CodeInternal, err)
	}
	return s.settings.Update(ctx, map[string]string{settings.KeyMediaLibraryRoots: string(b)})
}

// RootByID 按 id 查根目录。
func (s *Service) RootByID(ctx context.Context, id string) (*Root, error) {
	roots, err := s.Roots(ctx)
	if err != nil {
		return nil, err
	}
	for i := range roots {
		if roots[i].ID == id {
			return &roots[i], nil
		}
	}
	return nil, domain.Errorf(domain.CodeNotFound, "影视库不存在")
}

// Items 聚合查询影视条目。libID 为空时合并全部库；非空时只查该库。
func (s *Service) Items(ctx context.Context, libID string, query strmscrape.ItemListQuery) (ItemListResult, error) {
	roots, err := s.Roots(ctx)
	if err != nil {
		return ItemListResult{}, err
	}
	if len(roots) == 0 {
		return ItemListResult{}, nil
	}

	// 指定单库：透传查询（分页正确、索引直查）。
	if libID != "" {
		var root *Root
		for i := range roots {
			if roots[i].ID == libID {
				root = &roots[i]
				break
			}
		}
		if root == nil {
			return ItemListResult{}, domain.Errorf(domain.CodeNotFound, "影视库不存在")
		}
		return s.queryRoot(ctx, *root, query)
	}

	// 跨库合并：每库取前 (offset+limit) 条，合并排序后全局分页（best-effort）。
	fetch := query.Limit + query.Offset
	if fetch > mergeCap {
		fetch = mergeCap
	}
	q := query
	q.Offset = 0
	q.Limit = fetch

	merged := make([]Item, 0)
	total := 0
	for _, root := range roots {
		res, err := s.queryRoot(ctx, root, q)
		if err != nil {
			s.log.Warn("影视库查询失败，已跳过", "lib", root.ID, "path", root.Path, "err", err)
			continue
		}
		merged = append(merged, res.Items...)
		total += res.Total
	}
	sortLibraryItems(merged, query.Sort)
	start := query.Offset
	if start < 0 {
		start = 0
	}
	end := start + query.Limit
	if end > len(merged) {
		end = len(merged)
	}
	page := []Item{}
	if start < len(merged) {
		page = merged[start:end]
	}
	return ItemListResult{Items: page, Total: total, HasMore: end < len(merged) || end < total}, nil
}

// queryRoot 查询单个库并转换/补全条目（海报 URL 改指向公开端点、提取播放地址）。
func (s *Service) queryRoot(ctx context.Context, root Root, query strmscrape.ItemListQuery) (ItemListResult, error) {
	if s.scrape == nil {
		return ItemListResult{}, domain.Errorf(domain.CodeInternal, "刮削服务未就绪")
	}
	res, err := s.scrape.ListItems(ctx, 0, root.Path, query)
	if err != nil {
		return ItemListResult{}, err
	}
	out := make([]Item, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, s.toLibraryItem(ctx, root, it))
	}
	return ItemListResult{Items: out, Total: res.Total, HasMore: res.HasMore}, nil
}

func (s *Service) toLibraryItem(ctx context.Context, root Root, it strmscrape.Item) Item {
	item := Item{
		ID:         it.ID,
		Title:      it.Title,
		Year:       it.Year,
		MediaType:  it.MediaType,
		Status:     it.Status,
		TMDBID:     it.TMDBID,
		FolderName: it.FolderName,
		FileCount:  it.FileCount,
		EpLocal:    it.EpLocal,
		EpTMDB:     it.EpTMDB,
		EpScraped:  it.EpScraped,
		TVState:    it.TVState,
		LibID:      root.ID,
		PosterURL:  rebuildPosterURL(root.ID, it.PosterURL),
		PlayURL:    s.resolvePlayPath(root.Path, it),
	}
	return item
}

// rebuildPosterURL 把 strmscrape 的 admin 海报 URL 改写为影视模式公开端点，并用库 id 代替裸路径。
func rebuildPosterURL(libID, posterURL string) string {
	rel := ""
	if u, err := url.Parse(posterURL); err == nil {
		rel = u.Query().Get("rel")
	}
	if rel == "" {
		return ""
	}
	return "/api/media-library/poster?lib=" + url.QueryEscape(libID) + "&rel=" + url.QueryEscape(rel)
}

// resolvePlayPath 读取条目对应的本地 .strm 文件，提取播放相对路径。
// 剧集目录（StrmName 为空、多个 .strm）取排序后第一个。
func (s *Service) resolvePlayPath(rootPath string, it strmscrape.Item) string {
	dir := filepath.Join(rootPath, strings.TrimSpace(it.RelDir))
	name := strings.TrimSpace(it.StrmName)
	if name == "" {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.strm"))
		sort.Strings(matches)
		if len(matches) == 0 {
			return ""
		}
		data, err := os.ReadFile(matches[0])
		if err != nil {
			return ""
		}
		return strm.ExtractPlayPath(string(data))
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strm.ExtractPlayPath(string(data))
}

// Detail 返回影视条目详情：nfo 简介、背景图、播放地址（剧集含选集）。
func (s *Service) Detail(ctx context.Context, libID, id string) (*Detail, error) {
	if s.scrape == nil {
		return nil, domain.Errorf(domain.CodeInternal, "刮削服务未就绪")
	}
	root, err := s.RootByID(ctx, libID)
	if err != nil {
		return nil, err
	}
	it, err := s.scrape.GetItem(ctx, 0, root.Path, id)
	if err != nil {
		return nil, err
	}
	relDir := strings.TrimSpace(it.RelDir)
	info := readNFOInfo(root.Path, relDir, it.MediaType, it.StrmName)

	d := &Detail{
		ID:              it.ID,
		Title:           it.Title,
		Year:            it.Year,
		MediaType:       it.MediaType,
		TMDBID:          it.TMDBID,
		FolderName:      it.FolderName,
		FileCount:       it.FileCount,
		EpLocal:         it.EpLocal,
		EpTMDB:          it.EpTMDB,
		EpScraped:       it.EpScraped,
		TVState:         it.TVState,
		Status:          it.Status,
		PosterURL:       rebuildPosterURL(libID, it.PosterURL),
		Overview:        info.Plot,
		Genres:          info.Genres,
		Runtime:         info.Runtime,
		Studio:          info.Studio,
		Director:        info.Director,
		Actors:          info.Actors,
		BackdropURL:     s.resolveBackdropURL(libID, root.Path, relDir, it),
		ExtraFanartURLs: s.resolveExtraFanartURLs(libID, root.Path, relDir),
	}
	if it.MediaType == strmscrape.MediaTypeTV {
		d.Episodes = s.listEpisodes(root.Path, *it)
		if len(d.Episodes) > 0 {
			d.PlayURL = d.Episodes[0].PlayURL
		}
	} else {
		d.PlayURL = s.resolvePlayPath(root.Path, *it)
	}
	return d, nil
}

// resolveBackdropURL 尝试定位条目背景图（目录 backdrop.jpg / fanart.jpg / 单文件 {stem}-fanart.jpg）。
func (s *Service) resolveBackdropURL(libID, rootPath, relDir string, it *strmscrape.Item) string {
	candidates := []string{
		filepath.Join(relDir, "backdrop.jpg"),
		filepath.Join(relDir, "fanart.jpg"),
	}
	if it.MediaType == strmscrape.MediaTypeMovie {
		if stem := mediaStem(it.StrmName); stem != "" {
			candidates = append(candidates, filepath.Join(relDir, stem+"-fanart.jpg"))
		}
	}
	for _, rel := range candidates {
		if fileExists(filepath.Join(rootPath, rel)) {
			return "/api/media-library/poster?lib=" + url.QueryEscape(libID) + "&rel=" + url.QueryEscape(filepath.ToSlash(rel))
		}
	}
	return ""
}

// resolveExtraFanartURLs 扫描 extrafanart/ 目录下的所有图片文件，返回可用的背景图 URL 列表。
func (s *Service) resolveExtraFanartURLs(libID, rootPath, relDir string) []string {
	dir := filepath.Join(rootPath, relDir, "extrafanart")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var urls []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(relDir, "extrafanart", e.Name()))
		urls = append(urls, "/api/media-library/poster?lib="+url.QueryEscape(libID)+"&rel="+url.QueryEscape(rel))
	}
	return urls
}

// mediaStem 从 .strm 文件名提取媒体主名（Inception.mkv.strm → Inception）。
func mediaStem(strmName string) string {
	name := strings.TrimSuffix(strings.TrimSpace(strmName), ".strm")
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// listEpisodes 扫描剧集目录下所有 .strm，解析季/集号并排序。
func (s *Service) listEpisodes(rootPath string, it strmscrape.Item) []Episode {
	dir := filepath.Join(rootPath, strings.TrimSpace(it.RelDir))
	matches, err := filepath.Glob(filepath.Join(dir, "*.strm"))
	if err != nil {
		return nil
	}
	eps := make([]Episode, 0, len(matches))
	for _, m := range matches {
		stem := strings.TrimSuffix(filepath.Base(m), filepath.Ext(m)) // S01E01.mkv
		parsed := rules.NormalizeParsedMedia(rules.ParseFilenameStrict(stem + ".mkv"))
		data, rerr := os.ReadFile(m)
		if rerr != nil {
			continue
		}
		ep := Episode{PlayURL: strm.ExtractPlayPath(string(data))}
		if parsed.Season != nil {
			ep.Season = *parsed.Season
		}
		if parsed.Episode != nil {
			ep.Episode = *parsed.Episode
		}
		eps = append(eps, ep)
	}
	sort.SliceStable(eps, func(i, j int) bool {
		if eps[i].Season != eps[j].Season {
			return eps[i].Season < eps[j].Season
		}
		return eps[i].Episode < eps[j].Episode
	})
	return eps
}

// movieNFO / tvshowNFO 是 Kodi/Emby 兼容 nfo 的最小解析结构。
type nfoActor struct {
	Name string `xml:"name"`
}

type movieNFO struct {
	Title    string     `xml:"title"`
	Year     string     `xml:"year"`
	Plot     string     `xml:"plot"`
	Runtime  string     `xml:"runtime"`
	Genres   []string   `xml:"genre"`
	Studio   string     `xml:"studio"`
	Director string     `xml:"director"`
	Actors   []nfoActor `xml:"actor"`
}

type tvshowNFO struct {
	Title string `xml:"title"`
	Plot  string `xml:"plot"`
}

// nfoInfo nfo 解析出的可展示详情字段。
type nfoInfo struct {
	Plot     string
	Genres   []string
	Runtime  string
	Studio   string
	Director string
	Actors   []string
}

// readNFOInfo 读取条目目录下 movie.nfo / {stem}.nfo / tvshow.nfo 的详情字段。
// 兼容两种命名：Kodi 标准 movie.nfo 和刮削器按 stem 生成的 {stem}.nfo。
func readNFOInfo(rootPath, relDir, mediaType, strmName string) nfoInfo {
	if mediaType == strmscrape.MediaTypeTV {
		data, err := os.ReadFile(filepath.Join(rootPath, relDir, "tvshow.nfo"))
		if err != nil {
			return nfoInfo{}
		}
		var nf tvshowNFO
		if err := xml.Unmarshal(data, &nf); err != nil {
			return nfoInfo{}
		}
		return nfoInfo{Plot: strings.TrimSpace(nf.Plot)}
	}
	// 电影：先尝试 {stem}.nfo（刮削器命名），再回落 movie.nfo（Kodi 标准）
	dir := filepath.Join(rootPath, relDir)
	var data []byte
	if stem := mediaStem(strmName); stem != "" {
		data, _ = os.ReadFile(filepath.Join(dir, stem+".nfo"))
	}
	if len(data) == 0 {
		var err error
		data, err = os.ReadFile(filepath.Join(dir, "movie.nfo"))
		if err != nil {
			return nfoInfo{}
		}
	}
	var nf movieNFO
	if err := xml.Unmarshal(data, &nf); err != nil {
		return nfoInfo{}
	}
	genres := make([]string, 0, len(nf.Genres))
	for _, g := range nf.Genres {
		if t := strings.TrimSpace(g); t != "" {
			genres = append(genres, t)
		}
	}
	actors := make([]string, 0, len(nf.Actors))
	for _, a := range nf.Actors {
		if t := strings.TrimSpace(a.Name); t != "" {
			actors = append(actors, t)
		}
	}
	return nfoInfo{
		Plot:     strings.TrimSpace(nf.Plot),
		Genres:   genres,
		Runtime:  strings.TrimSpace(nf.Runtime),
		Studio:   strings.TrimSpace(nf.Studio),
		Director: strings.TrimSpace(nf.Director),
		Actors:   actors,
	}
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// Poster 返回某库某海报文件的本地路径（委托 strmscrape 校验）。
func (s *Service) Poster(ctx context.Context, libID, rel string) (string, error) {
	if s.scrape == nil {
		return "", domain.Errorf(domain.CodeInternal, "刮削服务未就绪")
	}
	root, err := s.RootByID(ctx, libID)
	if err != nil {
		return "", err
	}
	return s.scrape.ResolvePosterFile(ctx, 0, root.Path, rel)
}

// Refresh 强制重建各库索引并返回刷新后的条目。
func (s *Service) Refresh(ctx context.Context, libID string, query strmscrape.ItemListQuery) (ItemListResult, error) {
	roots, err := s.Roots(ctx)
	if err != nil {
		return ItemListResult{}, err
	}
	for _, root := range roots {
		if s.scrape == nil {
			continue
		}
		if _, err := s.scrape.RefreshIndex(ctx, 0, root.Path, query); err != nil {
			s.log.Warn("影视库索引刷新失败，已跳过", "lib", root.ID, "err", err)
		}
	}
	return s.Items(ctx, libID, query)
}

// sortLibraryItems 跨库合并结果按查询排序归一（title/year/added）。
func sortLibraryItems(items []Item, sortKey strmscrape.ItemListSort) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch sortKey {
		case strmscrape.ItemListSortYearAsc, strmscrape.ItemListSortYearDesc:
			av, bv := -1, -1
			if a.Year != nil {
				av = *a.Year
			}
			if b.Year != nil {
				bv = *b.Year
			}
			if av != bv {
				if sortKey == strmscrape.ItemListSortYearAsc {
					return av < bv
				}
				return av > bv
			}
			return a.Title < b.Title
		case strmscrape.ItemListSortAddedAsc, strmscrape.ItemListSortAddedDesc:
			if a.TMDBID != "" && b.TMDBID != "" {
				// 无独立 added 时间戳，退回标题序。
				return a.Title < b.Title
			}
			return a.Title < b.Title
		default: // title_asc
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		}
	})
}

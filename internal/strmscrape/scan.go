package strmscrape

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/mediaorganize/rules"
)

var explicitSeasonEpisodeFileRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s\d{1,3}e\d{1,4}(?:[^a-z0-9]|$)`)

type strmEntry struct {
	absPath string
	relPath string
}

// workGroup 一部作品：电影文件夹或剧集根目录；扁平散落的单个 .strm 各自成组。
type workGroup struct {
	relKey   string // 相对库根的稳定键（目录或单个 strm 相对路径）
	absDir   string // 元数据写入目录（扁平时为库根）
	flatFile string // 非空表示扁平单文件作品，值为 .strm 绝对路径
	entries  []strmEntry
}

func scanStrmFiles(root string) ([]strmEntry, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, domain.Errorf(domain.CodeValidation, "输出目录不是文件夹")
	}
	var out []strmEntry
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".strm") {
			return nil
		}
		out = append(out, strmEntry{absPath: path, relPath: relUnder(root, path)})
		return nil
	})
	return out, err
}

func groupWorks(root string, entries []strmEntry) []workGroup {
	// Step 1: 按目录分组，同时记录每个目录下的文件列表
	type dirBucket struct {
		absDir  string
		entries []strmEntry
		flat    bool // true = 文件直接在 library root 下
	}
	byDir := make(map[string]*dirBucket)
	dirOrder := make([]string, 0)
	for _, e := range entries {
		key, absDir, flatFile := workKeyForStrm(root, e)
		b, ok := byDir[key]
		if !ok {
			b = &dirBucket{absDir: absDir, flat: flatFile != ""}
			byDir[key] = b
			dirOrder = append(dirOrder, key)
		}
		b.entries = append(b.entries, e)
	}

	// Step 2: 按目录产出 workGroup。扁平文件（非 root、非剧集目录）各自独立。
	out := make([]workGroup, 0, len(entries))
	for _, key := range dirOrder {
		b := byDir[key]
		sort.Slice(b.entries, func(i, j int) bool {
			return b.entries[i].relPath < b.entries[j].relPath
		})
		needSplit := !b.flat && len(b.entries) > 1 && !isFlatTVShowDir(b.absDir)
		if needSplit {
			for _, e := range b.entries {
				out = append(out, workGroup{relKey: filepath.ToSlash(e.relPath), absDir: b.absDir, flatFile: e.absPath, entries: []strmEntry{e}})
			}
		} else {
			flatFile := ""
			if b.flat && len(b.entries) == 1 {
				flatFile = b.entries[0].absPath
			}
			out = append(out, workGroup{relKey: key, absDir: b.absDir, flatFile: flatFile, entries: b.entries})
		}
	}
	return out
}

func workKeyForStrm(root string, e strmEntry) (relKey, absDir, flatFile string) {
	workDir := resolveWorkDir(root, e.absPath)
	if sameFilePath(workDir, root) {
		// 直接散落在库根：每个 .strm 独立成一部作品，避免全部并成一张海报
		return filepath.ToSlash(e.relPath), root, e.absPath
	}
	return filepath.ToSlash(relUnder(root, workDir)), workDir, ""
}

// resolveWorkDir 从 .strm 所在目录向上跳过 Season / 特别篇目录，落到作品根。
func resolveWorkDir(libraryRoot, strmAbs string) string {
	dir := filepath.Dir(strmAbs)
	for {
		if !isInside(libraryRoot, dir) && !sameFilePath(dir, libraryRoot) {
			return filepath.Dir(strmAbs)
		}
		if sameFilePath(dir, libraryRoot) {
			return libraryRoot
		}
		if isStructuralWorkSubdir(dir) {
			parent := filepath.Dir(dir)
			if parent == dir || (!isInside(libraryRoot, parent) && !sameFilePath(parent, libraryRoot)) {
				return dir
			}
			dir = parent
			continue
		}
		return dir
	}
}

// workJAVNumber 从作品名（文件夹或首个分集文件）提取 JAV 番号（如 SSIS-123）。
// 命中时表示该作品按番号刮削（MetaTube 源按番号搜索），且一定是电影。
func workJAVNumber(g workGroup) string {
	if n := rules.FindJAVNumber(workDisplayName(g)); n != "" {
		return n
	}
	for _, e := range g.entries {
		stem := strings.TrimSuffix(filepath.Base(e.absPath), filepath.Ext(e.absPath))
		if n := rules.FindJAVNumber(stem); n != "" {
			return n
		}
	}
	return ""
}

func inferMediaType(g workGroup) string {
	// JAV 番号（如 SSIS-123）一定是电影：优先判定，避免数字被当成季/集号误判成剧集
	if workJAVNumber(g) != "" {
		return MediaTypeMovie
	}
	// 目录结构优先：存在 Season / 特别篇子目录，或文件位于此类目录下 → 剧集
	if g.flatFile == "" {
		if entries, err := os.ReadDir(g.absDir); err == nil {
			for _, d := range entries {
				if d.IsDir() && isStructuralWorkSubdir(filepath.Join(g.absDir, d.Name())) {
					return MediaTypeTV
				}
			}
		}
	}
	for _, e := range g.entries {
		parentDir := filepath.Dir(e.absPath)
		parent := filepath.Base(parentDir)
		if rules.IsSeasonDirName(parent) || (!sameFilePath(parentDir, g.absDir) && isStructuralWorkSubdir(parentDir)) {
			return MediaTypeTV
		}
	}

	seCount := 0
	for _, e := range g.entries {
		stem := strings.TrimSuffix(filepath.Base(e.absPath), filepath.Ext(e.absPath))
		if explicitSeasonEpisodeFileRe.MatchString(stem) {
			return MediaTypeTV
		}
		parsed := rules.NormalizeParsedMedia(rules.ParseFilenameStrict(stem + ".mkv"))
		if parsed.Season != nil && parsed.Episode != nil {
			seCount++
		}
	}
	// 多个解析为分集的文件也按剧集处理；两个以上可避开单个音轨标记误判。
	if seCount >= 2 {
		return MediaTypeTV
	}

	// 典型电影文件夹「片名 (年)」：避免单文件音轨标记（DTS5.1 / DDP2.0）误判成剧集
	folderName := workDisplayName(g)
	if isLikelyMovieWorkFolder(folderName) {
		return MediaTypeMovie
	}
	if seCount >= 1 {
		return MediaTypeTV
	}
	return MediaTypeMovie
}

func isLikelyMovieWorkFolder(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || rules.IsSeasonDirName(name) {
		return false
	}
	if rules.IsStandaloneMovieDirName(name) {
		return true
	}
	if rules.IsSpecialContentDirName(name) {
		return false
	}
	if rules.LooksLikeWorkDirName(name) {
		return true
	}
	parsed := rules.NormalizeParsedMedia(rules.ParseDirName(name))
	return parsed.Year != nil && strings.TrimSpace(parsed.Title) != ""
}

func isStructuralWorkSubdir(dir string) bool {
	name := filepath.Base(dir)
	if rules.IsSeasonDirName(name) {
		return true
	}
	if !rules.IsSpecialContentDirName(name) {
		return false
	}
	if !rules.IsStandaloneMovieDirName(name) {
		return true
	}
	if rules.FindTMDBIDInName(name) != "" {
		return false
	}
	return hasTVParentEvidence(filepath.Dir(dir), dir)
}

// isFlatTVShowDir 检测一个目录是否应保持分组：剧集目录（tvshow.nfo / Season 子目录 / 多文件含 SxxExx）或 JAV 番号目录。
func isFlatTVShowDir(dir string) bool {
	if fileExists(filepath.Join(dir, "tvshow.nfo")) {
		return true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && rules.IsSeasonDirName(e.Name()) {
			return true
		}
	}
	sxxCount := 0
	hasJAV := false
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".strm") {
			stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if explicitSeasonEpisodeFileRe.MatchString(stem) {
				sxxCount++
			}
			if rules.FindJAVNumber(stem) != "" {
				hasJAV = true
			}
		}
	}
	return sxxCount >= 2 || hasJAV
}

func hasTVParentEvidence(parentDir, currentDir string) bool {
	if fileExists(filepath.Join(parentDir, "tvshow.nfo")) {
		return true
	}
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		path := filepath.Join(parentDir, entry.Name())
		if entry.IsDir() {
			if !sameFilePath(path, currentDir) && rules.IsSeasonDirName(entry.Name()) {
				return true
			}
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".strm") {
			stem := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			if explicitSeasonEpisodeFileRe.MatchString(stem) {
				return true
			}
		}
	}
	return false
}

func workDisplayName(g workGroup) string {
	if g.flatFile != "" {
		return strings.TrimSuffix(filepath.Base(g.flatFile), filepath.Ext(g.flatFile))
	}
	return filepath.Base(g.absDir)
}

func findWorkByID(root, id string) (workGroup, error) {
	entries, err := scanStrmFiles(root)
	if err != nil {
		return workGroup{}, err
	}
	for _, g := range groupWorks(root, entries) {
		if pathToItemID(g.relKey) == id {
			return g, nil
		}
	}
	return workGroup{}, domain.Errorf(domain.CodeNotFound, "条目不存在")
}

func scanWorks(root string) ([]workGroup, error) {
	entries, err := scanStrmFiles(root)
	if err != nil {
		return nil, err
	}
	return groupWorks(root, entries), nil
}

func sameFilePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return aa == bb
}

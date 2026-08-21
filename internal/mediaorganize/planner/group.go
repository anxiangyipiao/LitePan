package planner

import (
	"fmt"
	"regexp"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/mediaorganize/rules"
)

type groupKey struct {
	mediaKind  string
	dirID      string
	dirName    string
	title      string
	year       int
	hasYear    bool
	season     int
	hasSeason  bool
	episode    int
	hasEpisode bool
}

type batchEntry struct {
	item          domain.FileItem
	sourceDirID   string
	sourceDirName string
	fileParsed    rules.ParsedMedia
	ancestors     []rules.Ancestor
	partLabel     string
	specialLabel  string
}

func (p *Planner) planBatch(entries []batchEntry, label string) error {
	if len(entries) == 0 {
		return nil
	}
	if err := p.checkStop(); err != nil {
		return err
	}
	p.processedBatches++
	p.scannedFiles += len(entries)
	p.currentDir = label
	p.log(fmt.Sprintf("[计划] 处理批次 #%d: %s，发现 %d 个媒体文件 (累计扫描 %d 个目录 / %d 个文件)",
		p.processedBatches, label, len(entries), p.scannedDirs, p.scannedFiles))
	p.emitProgress()

	groups, pendingSkips := p.groupEntries(entries)
	for _, ps := range pendingSkips {
		p.skip(ps.item, ps.reason)
	}
	p.log(fmt.Sprintf("[计划] 分组为 %d 个作品", len(groups)))
	for key, items := range groups {
		markerText := "有目录"
		if key.dirID == "" {
			markerText = "散落文件"
		}
		kindText := "电影"
		if key.mediaKind == "tv" {
			kindText = "剧集"
		}
		p.log(fmt.Sprintf("[计划]   组: %s | %s | 目录=%q | 标题=%q | %d个文件",
			kindText, markerText, key.dirName, key.title, len(items)))
		groupsDiag, _ := p.diagnostics["groups"].([]map[string]any)
		groupsDiag = append(groupsDiag, map[string]any{
			"media_kind": key.mediaKind,
			"dir_id":     key.dirID,
			"dir_name":   key.dirName,
			"title":      key.title,
			"count":      len(items),
		})
		p.diagnostics["groups"] = groupsDiag
	}

	alignDefaults := map[groupKey]map[bucketKey]map[string]any{}
	if p.alignMediaTags {
		alignDefaults = p.computeAlignDefaults(groups)
	}

	for key, items := range groups {
		if err := p.checkStop(); err != nil {
			return err
		}
		if p.maxWorksPerRun > 0 && p.plannedWorkCount >= p.maxWorksPerRun {
			p.quotaReached = true
			p.log(fmt.Sprintf("[计划] 已达到本次最多 %d 部作品上限，剩余作品将在下次重新生成计划时处理", p.maxWorksPerRun))
			return nil
		}
		before := len(p.actions)
		if err := p.planGroup(key, items, alignDefaults[key]); err != nil {
			return err
		}
		if len(p.actions) > before {
			p.plannedWorkCount++
		}
		p.emitProgress()
	}
	return nil
}

type pendingSkip struct {
	item   domain.FileItem
	reason string
}

type bucketKey struct {
	season int
	ext    string
}

func (p *Planner) groupEntries(entries []batchEntry) (map[groupKey][]batchEntry, []pendingSkip) {
	groups := map[groupKey][]batchEntry{}
	pending := make([]pendingSkip, 0)
	scanEntries := make([]rules.ScanEntry, len(entries))
	for i, e := range entries {
		anc := e.ancestors
		if len(anc) == 0 {
			anc = nil
		}
		scanEntries[i] = rules.ScanEntry{FileName: e.item.Name, Ancestors: anc}
	}
	layout := rules.AnalyzeTVTreeLayout(scanEntries)
	rangeLayouts := rules.AnalyzeEpisodeRangeLayouts(scanEntries)

	for _, raw := range entries {
		entry := raw
		if err := p.checkStop(); err != nil {
			return groups, pending
		}
		ancestors := cloneAncestors(entry.ancestors)
		rawFileParsed := rules.NormalizeParsedMedia(rules.ParseFilenameStrict(entry.item.Name))
		fileParsed := rawFileParsed
		dirParsed := rules.ParsedMedia{}
		rootParsed := rules.ParsedMedia{}
		nonSpecial := make([]rules.Ancestor, 0, len(ancestors))
		for _, anc := range ancestors {
			if rules.IsGenericMediaDir(anc.Name) || rules.IsSeasonDirName(anc.Name) || rules.IsEpisodeRangeDirName(anc.Name) {
				continue
			}
			if rules.IsCollectionContainerDir(anc.Name, nil) {
				continue
			}
			if rules.IsSpecialContentDirName(anc.Name) {
				continue
			}
			nonSpecial = append(nonSpecial, anc)
		}
		if len(nonSpecial) > 0 {
			dirParsed = rules.NormalizeParsedMedia(rules.ParseDirName(nonSpecial[len(nonSpecial)-1].Name))
		}
		if len(nonSpecial) >= 2 {
			rootParsed = rules.NormalizeParsedMedia(rules.ParseDirName(nonSpecial[len(nonSpecial)-2].Name))
		}
		fileParsed = rules.MergeThreeLayerParsed(fileParsed, dirParsed, rootParsed)
		fileParsed = rules.PrepareTVFileParsed(fileParsed, ancestors)
		var rangeOK bool
		fileParsed, rangeOK = rules.ApplyEpisodeRangeLayout(fileParsed, entry.item.Name, ancestors, rangeLayouts)
		if !rangeOK {
			pending = append(pending, pendingSkip{
				item:   entry.item,
				reason: "分集范围目录与文件集数不一致，请检查目录范围或文件编号",
			})
			continue
		}
		partLabel := rules.ExtractPartLabel(entry.item.Name)
		specialLabel := rules.ExtractSpecialLabel(entry.item.Name)

		sourceDirID := p.parentID
		sourceDirName := ""
		if len(ancestors) > 0 {
			sourceDirID = ancestors[len(ancestors)-1].ID
			sourceDirName = ancestors[len(ancestors)-1].Name
		}

		showDirID, showDirName, showParsed := rules.PickTVShowInfo(ancestors, fileParsed)
		if rules.IsAmbiguousRootTVScatter(ancestors, layout, showDirID) &&
			rules.IsBareEpisodeLikeFilename(entry.item.Name, fileParsed) {
			pending = append(pending, pendingSkip{
				item:   entry.item,
				reason: "检测到多季子目录，根目录散落文件无法确定季号，请移入对应季文件夹",
			})
			continue
		}

		nestedMovieID, _ := rules.FindNearestStandaloneMovieDir(ancestors)
		forceMovie := nestedMovieID != "" || shouldPreferStructuredMovieDir(rawFileParsed, dirParsed, ancestors, entry.item.Name)

		tvRule := rules.LooksLikeTVFileWithName(fileParsed, ancestors, entry.item.Name)
		isTV := !forceMovie && (p.taskMediaType == "tv" || (p.taskMediaType == "auto" && tvRule.Matched))

		if isTV {
			if showDirID == "" {
				showDirID, showDirName, showParsed = rules.PickTVShowInfo(ancestors, fileParsed)
			}
			if rules.IsAmbiguousRootTVScatter(ancestors, layout, showDirID) {
				pending = append(pending, pendingSkip{
					item:   entry.item,
					reason: "检测到多季子目录，根目录散落文件无法确定季号，请移入对应季文件夹",
				})
				continue
			}
			title := strings.TrimSpace(showParsed.Title)
			if title == "" {
				title = strings.TrimSpace(fileParsed.Title)
			}
			year := rules.ResolveTVGroupYear(showParsed)
			key := groupKey{mediaKind: "tv", dirID: showDirID, dirName: showDirName, title: title}
			key.setYear(year)
			entry.sourceDirID = sourceDirID
			entry.sourceDirName = sourceDirName
			entry.fileParsed = fileParsed
			entry.ancestors = ancestors
			entry.partLabel = partLabel
			entry.specialLabel = specialLabel
			groups[key] = append(groups[key], entry)
			continue
		}

		movieDirID := ""
		movieDirName := ""
		movieParsed := rules.ParsedMedia{}
		if forceMovie {
			movieDirID = nestedMovieID
			for _, anc := range ancestors {
				if anc.ID == nestedMovieID {
					movieDirName = anc.Name
					break
				}
			}
			movieParsed = rules.NormalizeParsedMedia(rules.ParseDirName(movieDirName))
		} else {
			for i := len(ancestors) - 1; i >= 0; i-- {
				anc := ancestors[i]
				if rules.IsGenericMediaDir(anc.Name) || rules.IsSeasonDirName(anc.Name) || rules.IsEpisodeRangeDirName(anc.Name) {
					continue
				}
				if rules.IsCollectionContainerDir(anc.Name, nil) {
					continue
				}
				parsed := rules.NormalizeParsedMedia(rules.ParseDirName(anc.Name))
				if parsed.Title == "" {
					continue
				}
				// 仅当目录名与文件标题对齐时才绑定为作品目录，避免
				// 父文件夹下子文件夹内含多部不同影片时，把容器目录误当作品目录
				// 导致错误合并/重复建目录（JAV 按番号对齐，其余按标题松匹配）。
				fileJav := rules.FindJAVNumber(entry.item.Name)
				dirJav := rules.FindJAVNumber(anc.Name)
				aligned := false
				if fileJav != "" || dirJav != "" {
					if fileJav != "" && dirJav != "" && sameLooseTitle(fileJav, dirJav) {
						aligned = true
					}
				} else if sameLooseTitle(parsed.Title, fileParsed.Title) || sameLooseTitle(parsed.Title, rawFileParsed.Title) {
					aligned = true
				}
				if !aligned {
					continue
				}
				movieDirID = anc.ID
				movieDirName = anc.Name
				movieParsed = parsed
				break
			}
		}
		if movieDirID == "" && len(ancestors) > 0 {
			anc := ancestors[len(ancestors)-1]
			if !rules.IsGenericMediaDir(anc.Name) && !rules.IsSeasonDirName(anc.Name) && !rules.IsEpisodeRangeDirName(anc.Name) {
				parsed := rules.NormalizeParsedMedia(rules.ParseDirName(anc.Name))
				if parsed.Title != "" {
					fileJav := rules.FindJAVNumber(entry.item.Name)
					dirJav := rules.FindJAVNumber(anc.Name)
					aligned := false
					if fileJav != "" || dirJav != "" {
						if fileJav != "" && dirJav != "" && sameLooseTitle(fileJav, dirJav) {
							aligned = true
						}
					} else if sameLooseTitle(parsed.Title, fileParsed.Title) || sameLooseTitle(parsed.Title, rawFileParsed.Title) {
						aligned = true
					}
					if aligned {
						movieDirID = anc.ID
						movieDirName = anc.Name
						movieParsed = parsed
					}
				}
			}
		}

		// 规范化电影分组：JAV 番号归一 + 剥离 CD/PART 分盘标签
		javCanonical := ""
		if jav := rules.FindJAVNumber(entry.item.Name); jav != "" {
			javCanonical = strings.ToUpper(jav)
		} else if movieDirName != "" {
			if jav := rules.FindJAVNumber(movieDirName); jav != "" {
				javCanonical = strings.ToUpper(jav)
			}
		}
		partForTitle := partLabel
		if partForTitle == "" && movieDirName != "" {
			partForTitle = rules.ExtractPartLabel(movieDirName)
		}

		var key groupKey
		if javCanonical != "" {
			// 多碟（CD1/CD2）按同一番号归一为同一组标题。
			// 散落文件（无目录）合并为一组；已位于作品目录内的 JAV 保留目录绑定，
			// 由 planGroup 按「已在目录中」跳过，避免把已归类的文件再挪进新目录。
			key = groupKey{
				mediaKind: "movie",
				dirID:     movieDirID,
				dirName:   movieDirName,
				title:     javCanonical,
			}
			key.setYear(fileParsed.Year)
			if movieParsed.Year != nil && key.yearPtr() == nil {
				key.setYear(movieParsed.Year)
			}
		} else if movieDirID != "" {
			groupTitle, groupYear := rules.ResolveMovieGroupIdentity(movieDirName, fileParsed)
			title := groupTitle
			if title == "" {
				title = movieParsed.Title
			}
			if partForTitle != "" {
				title = stripPartSuffixFromTitle(title, partForTitle)
			}
			year := groupYear
			if year == nil {
				year = movieParsed.Year
			}
			key = groupKey{
				mediaKind: "movie",
				dirID:     movieDirID,
				dirName:   movieDirName,
				title:     title,
			}
			key.setYear(year)
			key.setSeason(movieParsed.Season)
			key.setEpisode(movieParsed.Episode)
		} else if p.ScatterMoviePerFile && !isTV {
			isoTitle := scatteredMovieIsolationBase(entry.item.Name, fileParsed)
			if isoTitle == "" {
				isoTitle = strings.TrimSpace(fileParsed.Title)
			}
			if isoTitle == "" {
				isoTitle = strings.TrimSpace(entry.item.Name)
			}
			if partForTitle != "" {
				isoTitle = stripPartSuffixFromTitle(isoTitle, partForTitle)
				if isoTitle == "" {
					fallback := scatteredMovieIsolationBase(entry.item.Name, fileParsed)
					isoTitle = stripPartSuffixFromTitle(fallback, partForTitle)
				}
			}
			// abc-123 这类短番号按完整番号为准，避免 fileParsed.Title 被截断为 abc
			if jav := rules.FindJAVNumber(entry.item.Name); jav != "" {
				isoTitle = strings.ToUpper(jav)
			}
			key = groupKey{
				mediaKind: "movie",
				title:     isoTitle,
			}
			key.setYear(fileParsed.Year)
		} else {
			title := fileParsed.Title
			if partForTitle != "" {
				title = stripPartSuffixFromTitle(title, partForTitle)
			}
			key = groupKey{
				mediaKind: "movie",
				title:     title,
			}
			key.setYear(fileParsed.Year)
			key.setSeason(fileParsed.Season)
			key.setEpisode(fileParsed.Episode)
		}
		entry.sourceDirID = sourceDirID
		entry.sourceDirName = sourceDirName
		entry.fileParsed = fileParsed
		entry.ancestors = ancestors
		entry.partLabel = partLabel
		entry.specialLabel = specialLabel
		groups[key] = append(groups[key], entry)
	}
	return groups, pending
}

func stripPartSuffixFromTitle(title, partLabel string) string {
	title = strings.TrimSpace(title)
	partLabel = strings.TrimSpace(partLabel)
	if title == "" || partLabel == "" {
		return title
	}
	low := strings.ToLower(partLabel)
	_ = low
	// 常见：标题本身已包含分盘标签，直接去除尾缀
	re := regexp.MustCompile(`(?i)[\s._\-]*` + regexp.QuoteMeta(partLabel) + `\s*$`)
	stripped := strings.TrimSpace(re.ReplaceAllString(title, ""))
	if stripped != "" {
		return stripped
	}
	return title
}

func shouldPreferStructuredMovieDir(fileParsed, dirParsed rules.ParsedMedia, ancestors []rules.Ancestor, fileName string) bool {
	if strings.TrimSpace(fileParsed.Title) == "" || strings.TrimSpace(dirParsed.Title) == "" || dirParsed.Year == nil {
		return false
	}
	if !sameLooseTitle(fileParsed.Title, dirParsed.Title) {
		return false
	}
	if rules.HasExplicitSeasonToken(fileName) {
		return false
	}
	for _, anc := range ancestors {
		if rules.IsSeasonDirName(anc.Name) || rules.IsSpecialContentDirName(anc.Name) {
			return false
		}
	}
	return true
}

func scatteredMovieIsolationBase(fileName string, parsed rules.ParsedMedia) string {
	stem, _ := rules.SplitBasename(fileName)
	stem = rules.StripReleaseSitePrefix(stem)
	// 去除分盘后缀再取隔离目录名（避免 SIVR-498.CD2 vs SIVR-498.CD1 拆成两组）
	if stem != "" {
		stem = rules.StripPartSuffix(stem)
	}
	base := rules.SanitizeFilename(strings.TrimSpace(stem))
	if base != "" {
		return base
	}
	if t := strings.TrimSpace(parsed.Title); t != "" {
		return t
	}
	return strings.TrimSpace(fileName)
}

func sameLooseTitle(a, b string) bool {
	normalize := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		replacer := strings.NewReplacer(" ", "", ".", "", "_", "", "-", "", "·", "", "：", "", ":", "")
		return replacer.Replace(s)
	}
	return normalize(a) == normalize(b)
}

func (k *groupKey) setYear(v *int) {
	if v == nil {
		return
	}
	k.year = *v
	k.hasYear = true
}

func (k *groupKey) setSeason(v *int) {
	if v == nil {
		return
	}
	k.season = *v
	k.hasSeason = true
}

func (k *groupKey) setEpisode(v *int) {
	if v == nil {
		return
	}
	k.episode = *v
	k.hasEpisode = true
}

func (k groupKey) yearPtr() *int {
	if !k.hasYear {
		return nil
	}
	v := k.year
	return &v
}

func (k groupKey) seasonPtr() *int {
	if !k.hasSeason {
		return nil
	}
	v := k.season
	return &v
}

func (k groupKey) episodePtr() *int {
	if !k.hasEpisode {
		return nil
	}
	v := k.episode
	return &v
}

func (p *Planner) computeAlignDefaults(groups map[groupKey][]batchEntry) map[groupKey]map[bucketKey]map[string]any {
	out := map[groupKey]map[bucketKey]map[string]any{}
	for key, items := range groups {
		stats := map[bucketKey]map[string]map[string]int{}
		totals := map[bucketKey]int{}
		for _, entry := range items {
			ext := rules.FileExtension(entry.item.Name)
			season := 0
			if entry.fileParsed.Season != nil {
				season = *entry.fileParsed.Season
			}
			bk := bucketKey{season: season, ext: ext}
			totals[bk]++
			tagValues := map[string]any{
				"screen_size":    entry.fileParsed.ScreenSize,
				"frame_rate":     entry.fileParsed.FrameRate,
				"video_codec":    entry.fileParsed.VideoCodec,
				"audio_codec":    entry.fileParsed.AudioCodec,
				"audio_channels": entry.fileParsed.AudioChannels,
			}
			for _, tagKey := range rules.MediaTagFields {
				normalized := rules.NormalizeMediaTagValue(tagKey, tagValues[tagKey])
				if normalized == nil || normalized == "" {
					continue
				}
				if stats[bk] == nil {
					stats[bk] = map[string]map[string]int{}
				}
				if stats[bk][tagKey] == nil {
					stats[bk][tagKey] = map[string]int{}
				}
				stats[bk][tagKey][fmt.Sprint(normalized)]++
			}
		}
		defaultsByBucket := map[bucketKey]map[string]any{}
		for bk, tagStats := range stats {
			total := totals[bk]
			if total <= 0 {
				continue
			}
			defaults := map[string]any{}
			for tagKey, counter := range tagStats {
				bestVal := ""
				bestCount := 0
				for val, count := range counter {
					if count > bestCount {
						bestCount = count
						bestVal = val
					}
				}
				if float64(bestCount)/float64(total) > 0.6 {
					defaults[tagKey] = bestVal
				}
			}
			if len(defaults) > 0 {
				defaultsByBucket[bk] = defaults
			}
		}
		if len(defaultsByBucket) > 0 {
			out[key] = defaultsByBucket
		}
	}
	return out
}

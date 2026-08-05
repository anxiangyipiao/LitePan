package rules

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

func FindTMDBIDInName(name string) string {
	if name == "" {
		return ""
	}
	for _, re := range tmdbTagPatterns {
		if m := re.FindStringSubmatch(name); len(m) >= 2 {
			return m[1]
		}
	}
	return ""
}

// FindJAVNumber 从片名中提取 JAV 番号，覆盖各类格式：
//
//	有码：    SSSS-001 / ABC-100 / PPPC-326
//	素人：    390JAC-132 / 300MAAN-783
//	无码：    HEYZO-1234 / KIN8-1675 / 123456_999 / 543210-001 / 587234-01 /
//	         xxx-av-1234 / heydouga-1234-321 / c0930-ki897634 / h4610-ori98321
//	其他：    FC2-1234567 / MYWIFE-1394 / GETCHU-3456789
//
// 支持 -C 字幕后缀与 -cdN 多碟后缀（自动截断）。字母码要求全大写（2-3 位数字段可放宽），
// 排除 1900-2099 年份与 Title Case 普通片名（Room 1408 / Inception 2010 等）。未命中返回空串。
func FindJAVNumber(name string) string {
	raw := strings.TrimSpace(name)
	if raw == "" {
		return ""
	}
	// 1) 多段特殊格式（必须先于主字母码，避免 c0930-ki897634 被主模式截成 ki897634）
	for _, re := range javNumberPatterns[:3] {
		if m := re.FindStringSubmatch(raw); len(m) >= 2 && !yearLikeRe.MatchString(m[1]) {
			return normalizeJAVNumber(m[1])
		}
	}
	// 2) 主字母码（覆盖绝大多数格式），需要单独校验代码与数字段
	if m := javNumberPatterns[3].FindStringSubmatch(raw); len(m) >= 4 {
		if validateJAVCode(m[2], m[3]) && !yearLikeRe.MatchString(m[1]) {
			return normalizeJAVNumber(m[1])
		}
	}
	// 3) 纯数字无码
	if m := javNumberPatterns[4].FindStringSubmatch(raw); len(m) >= 2 {
		return normalizeJAVNumber(m[1])
	}
	return ""
}

// validateJAVCode 校验主字母码：字母段需全大写；字母段非全大写时仅接受 2-3 位数字段
// （允许 abc-100 这类小写番号，但拒绝 Room 1408 这种 4 位数 + 普通片名）。
func validateJAVCode(code, num string) bool {
	letters := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return r
		}
		return -1
	}, code)
	if letters == "" {
		return true
	}
	if letters != strings.ToUpper(letters) && len(num) == 4 && !yearLikeRe.MatchString(num) {
		return false
	}
	return true
}

// normalizeJAVNumber 把常见的点号/空格分隔符规范成 -（下划线保留，如 123456_999）。
func normalizeJAVNumber(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '.' || r == ' ' {
			return '-'
		}
		return r
	}, s)
}

// yearLikeRe 匹配独立出现的 1900-2099 年份，用于排除 Inception 2010 / Star Wars 1977 这类片名。
var yearLikeRe = regexp.MustCompile(`(?:^|[^0-9])(?:19|20)\d{2}(?:$|[^0-9])`)

func ExtractTMDBDisplayFields(result map[string]any, mediaType string) (id, title, original string, year *int) {
	_ = mediaType
	if len(result) == 0 {
		return "", "", "", nil
	}
	releaseDate := strVal(result["release_date"])
	if releaseDate == "" {
		releaseDate = strVal(result["first_air_date"])
	}
	if len(releaseDate) >= 4 {
		if y, err := parseInt(releaseDate[:4]); err == nil {
			year = intPtr(y)
		}
	}
	id = strings.TrimSpace(toString(result["id"]))
	title = strings.TrimSpace(strVal(result["title"]))
	if title == "" {
		title = strings.TrimSpace(strVal(result["name"]))
	}
	original = strings.TrimSpace(strVal(result["original_title"]))
	if original == "" {
		original = strings.TrimSpace(strVal(result["original_name"]))
	}
	return id, title, original, year
}

func IsTMDBTitleCompatible(query, resultTitle, resultOriginal string) bool {
	q := strings.TrimSpace(query)
	if q == "" {
		return false
	}
	ql := strings.ToLower(q)
	candidates := []string{strings.TrimSpace(resultTitle), strings.TrimSpace(resultOriginal)}
	for _, t := range candidates {
		if t != "" && ql == strings.ToLower(t) {
			return true
		}
	}

	enWords := enWordRe.FindAllString(ql, -1)
	if len(enWords) > 0 {
		blob := strings.ToLower(strings.Join(candidates, " "))
		for _, w := range enWords {
			if strings.Contains(blob, w) {
				return true
			}
		}
	}

	cnQ := hanOnlyRe.ReplaceAllString(q, "")
	if len([]rune(cnQ)) >= 2 {
		for _, t := range candidates {
			cnT := hanOnlyRe.ReplaceAllString(t, "")
			if cnT == "" {
				continue
			}
			if cnQ == cnT {
				return true
			}
			if strings.HasPrefix(cnT, cnQ) && len(cnT) > len(cnQ) {
				extra := cnT[len([]rune(cnQ)):]
				if hanOnlyRe.MatchString(extra) {
					return false
				}
			}
			if strings.HasSuffix(cnT, cnQ) && len(cnT) > len(cnQ) {
				prefix := cnT[:len(cnT)-len([]rune(cnQ))]
				if prefix != "" && docuPrefixRe.MatchString(prefix) {
					return false
				}
			}
			if strings.Contains(cnT, cnQ) {
				return true
			}
		}
		if len([]rune(cnQ)) <= 4 {
			return false
		}
		runes := []rune(cnQ)
		bigrams := make([]string, 0, len(runes)-1)
		for i := 0; i < len(runes)-1; i++ {
			bigrams = append(bigrams, string(runes[i:i+2]))
		}
		for _, t := range candidates {
			cnT := hanOnlyRe.ReplaceAllString(t, "")
			if cnT == "" || cnT == cnQ {
				continue
			}
			if strings.HasPrefix(cnT, cnQ) && len(cnT) > len(cnQ) {
				continue
			}
			hits := 0
			for _, bg := range bigrams {
				if strings.Contains(cnT, bg) {
					hits++
				}
			}
			if hits >= max(2, len(bigrams)/2) {
				return true
			}
		}
		return false
	}

	if len(ql) <= 1 {
		return true
	}
	for _, t := range candidates {
		tl := strings.ToLower(t)
		if tl != "" && (strings.Contains(ql, tl) || strings.Contains(tl, ql)) {
			return true
		}
	}
	return false
}

func PickTMDBMatchForYear(results []map[string]any, expectedYear *int, mediaType, queryTitle string) map[string]any {
	if len(results) == 0 {
		return nil
	}
	qt := strings.TrimSpace(queryTitle)
	compatible := func(item map[string]any) bool {
		if qt == "" {
			return true
		}
		_, t, o, _ := ExtractTMDBDisplayFields(item, mediaType)
		return IsTMDBTitleCompatible(qt, t, o)
	}
	if expectedYear != nil {
		for _, item := range results {
			_, _, _, resultYear := ExtractTMDBDisplayFields(item, mediaType)
			if resultYear != nil && *resultYear == *expectedYear && compatible(item) {
				return item
			}
		}
		return nil
	}
	if qt != "" {
		for _, item := range results {
			if compatible(item) {
				return item
			}
		}
		return nil
	}
	return results[0]
}

// PickTMDBSearchMatchForYear 仅在明确年份相等时接受标题不一致的 TMDB 原始搜索结果。
func PickTMDBSearchMatchForYear(results []map[string]any, expectedYear *int, mediaType, queryTitle string) map[string]any {
	if selected := PickTMDBMatchForYear(results, expectedYear, mediaType, queryTitle); selected != nil {
		return selected
	}
	if expectedYear == nil {
		return nil
	}
	for _, item := range results {
		_, _, _, resultYear := ExtractTMDBDisplayFields(item, mediaType)
		if resultYear != nil && *resultYear == *expectedYear {
			return item
		}
	}
	return nil
}

// PickUniqueTMDBAdjacentYearMatch 仅接受唯一、片名强相等且相差一年的候选，不使用别名放宽。
func PickUniqueTMDBAdjacentYearMatch(results []map[string]any, expectedYear *int, mediaType, queryTitle string) map[string]any {
	if expectedYear == nil || strings.TrimSpace(queryTitle) == "" {
		return nil
	}
	queryKey := strongTMDBTitleKey(queryTitle)
	if queryKey == "" {
		return nil
	}
	matches := make(map[string]map[string]any, 2)
	for _, item := range results {
		id, title, original, resultYear := ExtractTMDBDisplayFields(item, mediaType)
		if id == "" || resultYear == nil || absInt(*resultYear-*expectedYear) != 1 {
			continue
		}
		if strongTMDBTitleKey(title) != queryKey && strongTMDBTitleKey(original) != queryKey {
			continue
		}
		matches[id] = item
	}
	if len(matches) != 1 {
		return nil
	}
	for _, item := range matches {
		return item
	}
	return nil
}

func strongTMDBTitleKey(title string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, strings.TrimSpace(title))
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func BuildTMDBMatchAttempts(groupTitle string, groupYear *int, dirName string, fileParses []ParsedMedia) []TMDBMatchAttempt {
	dirParsed := ParsedMedia{}
	if dirName != "" {
		dirParsed = NormalizeParsedMedia(ParseDirName(dirName))
	}
	dirTitle := strings.TrimSpace(dirParsed.Title)
	dirYear := dirParsed.Year

	fileTitles := make([]string, 0, len(fileParses))
	fileYears := make([]int, 0, len(fileParses))
	for _, parsed := range fileParses {
		fp := NormalizeParsedMedia(parsed)
		if ft := strings.TrimSpace(fp.Title); ft != "" {
			fileTitles = append(fileTitles, ft)
		}
		if fp.Year != nil {
			fileYears = append(fileYears, *fp.Year)
		}
	}

	fileTitle := ""
	if len(fileTitles) > 0 {
		fileTitle = PickBestTitleForTMDB(fileTitles...)
	}
	var fileYear *int
	if len(fileYears) > 0 {
		fileYear = intPtr(fileYears[0])
	}
	mergedTitle := PickBestTitleForTMDB(dirTitle, fileTitle, groupTitle)
	mergedYear := dirYear
	if mergedYear == nil {
		mergedYear = fileYear
	}
	if mergedYear == nil {
		mergedYear = groupYear
	}

	attempts := make([]TMDBMatchAttempt, 0, 8)
	seen := map[string]struct{}{}
	add := func(title string, year *int, source string) {
		t := strings.TrimSpace(title)
		if t == "" {
			return
		}
		yKey := "nil"
		if year != nil {
			yKey = strconv.Itoa(*year)
		}
		key := strings.ToLower(t) + "|" + yKey
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		attempts = append(attempts, TMDBMatchAttempt{Title: t, Year: year, Source: source})
	}

	add(mergedTitle, mergedYear, "合并")
	if fileTitle != "" {
		y := fileYear
		if y == nil {
			y = mergedYear
		}
		add(fileTitle, y, "文件")
	}
	if dirTitle != "" && ScoreTitleForTMDB(dirTitle) >= 0.45 {
		y := dirYear
		if y == nil {
			y = mergedYear
		}
		add(dirTitle, y, "目录")
	}
	if groupTitle != "" {
		y := groupYear
		if y == nil {
			y = mergedYear
		}
		add(groupTitle, y, "默认")
	}

	snapshot := append([]TMDBMatchAttempt(nil), attempts...)
	for _, item := range snapshot {
		cnCore := ExtractChineseTitleCore(item.Title)
		if cnCore != "" && cnCore != item.Title {
			if hasNumericSuffixAfterChineseCore(item.Title, cnCore) {
				continue
			}
			add(cnCore, item.Year, item.Source+"-中文")
		}
	}
	return attempts
}

func hasNumericSuffixAfterChineseCore(title, core string) bool {
	suffix := strings.TrimLeft(strings.TrimPrefix(title, core), " ._-")
	return suffix != "" && suffix[0] >= '0' && suffix[0] <= '9'
}

func ScoreTitleForTMDB(title string) float64 {
	raw := strings.TrimSpace(title)
	if raw == "" {
		return 0
	}
	score := 1.0
	if titleNoiseRe.MatchString(raw) {
		score -= 0.85
	}
	if resolutionInTitleRe.MatchString(raw) {
		score -= 0.35
	}
	if sceneKeywordsRe.MatchString(raw) {
		score -= 0.25
	}
	tokenCount := len(tokenSplitRe.Split(raw, -1))
	if tokenCount > 10 {
		score -= 0.25
	} else if tokenCount > 7 {
		score -= 0.15
	}
	if len(raw) > 48 {
		score -= 0.1
	}
	if pureChineseTitleRe.MatchString(raw) {
		score += 0.2
	}
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func ExtractChineseTitleCore(title string) string {
	raw := strings.TrimSpace(title)
	m := chineseTitleCoreRe.FindStringSubmatch(raw)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func PickBestTitleForTMDB(candidates ...string) string {
	cleaned := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if t := strings.TrimSpace(c); t != "" {
			cleaned = append(cleaned, t)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	if len(cleaned) == 1 {
		return cleaned[0]
	}
	type scored struct {
		score float64
		title string
	}
	scoredList := make([]scored, len(cleaned))
	for i, title := range cleaned {
		scoredList[i] = scored{score: ScoreTitleForTMDB(title), title: title}
	}
	best := scoredList[0]
	for _, item := range scoredList[1:] {
		if item.score > best.score {
			best = item
		}
	}
	if best.score >= 0.45 {
		return best.title
	}
	for _, item := range scoredList {
		if item.score >= 0.35 {
			return item.title
		}
	}
	return cleaned[0]
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

var (
	hanOnlyRe    = regexp.MustCompile(`[^\p{Han}]`)
	docuPrefixRe = regexp.MustCompile(`(舞台剧|纪录片|歌剧|幕后|制作纪录)`)
	tokenSplitRe = regexp.MustCompile(`[\s._\-]+`)
)

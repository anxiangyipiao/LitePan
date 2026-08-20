package strm

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func BuildPlayPath(accountID int64, fileID, fileName, token string, signEnabled bool, secret []byte) string {
	account := strconv.FormatInt(accountID, 10)
	escapedName := url.PathEscape(fileName)
	path := fmt.Sprintf("/api/strm/play/%s/%s/t/%s/n/%s", account, EncodeFileKey(fileID), token, escapedName)
	if signEnabled {
		path += "/s/" + SignPath(path, secret)
	}
	return path
}

func BuildPlayURL(baseURL string, accountID int64, fileID, fileName, token string, signEnabled bool, secret []byte) string {
	path := BuildPlayPath(accountID, fileID, fileName, token, signEnabled, secret)
	base := NormalizeBaseURL(baseURL)
	if base == "" {
		return path
	}
	return base + path
}

// ExtractPlayPath 从 .strm 文件内容提取播放相对路径（/api/strm/play/...）。
// 内容可能带 http(s)://origin 前缀或空白，一律归一为相对路径；
// 匹配失败时返回去空白后的原串。
func ExtractPlayPath(line string) string {
	line = strings.TrimSpace(line)
	m := strmPlayURLPattern.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	// m[1]=account m[2]=file_key m[3]=token m[4]=filename m[5]=signature(可选)
	path := fmt.Sprintf("/api/strm/play/%s/%s/t/%s/n/%s", m[1], m[2], m[3], m[4])
	if m[5] != "" {
		path += "/s/" + m[5]
	}
	return path
}

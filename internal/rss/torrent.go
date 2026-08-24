package rss

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
)

var errTorrentParse = errors.New("torrent 文件解析失败")

// torrentInfoHash 解析 .torrent（bencode）的 info 段，返回其 SHA-1（hex，即 infohash）。
// 仅当 feed 只给出 http .torrent 链接、无磁力/无 infohash 时，用于构造磁力链。
func torrentInfoHash(data []byte) (string, error) {
	if len(data) > maxFeedBytes {
		return "", fmt.Errorf("torrent 文件过大")
	}
	// 顶层字典里定位 "4:info"，其后应为 'd' 开头的 info 字典。
	key := []byte("4:info")
	idx := bytes.Index(data, key)
	if idx < 0 || idx+len(key) >= len(data) || data[idx+len(key)] != 'd' {
		return "", errTorrentParse
	}
	start := idx + len(key)
	end, err := bencodeEnd(data, start)
	if err != nil {
		return "", err
	}
	sum := sha1.Sum(data[start:end])
	return hex.EncodeToString(sum[:]), nil
}

// bencodeEnd 从 pos（指向 'd'）按嵌套深度扫描到匹配的 'e'，返回其后位置。
// 仅遍历顶层结构；字符串值整体跳过，避免其中的字节被误当结构。
func bencodeEnd(data []byte, pos int) (int, error) {
	depth := 0
	i := pos
	for i < len(data) {
		c := data[i]
		switch {
		case c == 'd' || c == 'l':
			depth++
			i++
		case c == 'e':
			depth--
			if depth == 0 {
				return i + 1, nil
			}
			i++
		case c == 'i':
			// i<digits>e
			j := bytes.IndexByte(data[i:], 'e')
			if j < 0 {
				return 0, errTorrentParse
			}
			i += j + 1
		case c >= '0' && c <= '9':
			// <len>:<bytes>
			j := i
			for j < len(data) && data[j] >= '0' && data[j] <= '9' {
				j++
			}
			if j >= len(data) || data[j] != ':' {
				return 0, errTorrentParse
			}
			n := 0
			for k := i; k < j; k++ {
				n = n*10 + int(data[k]-'0')
				if n > 1<<24 {
					return 0, errTorrentParse
				}
			}
			if j+1+n > len(data) {
				return 0, errTorrentParse
			}
			i = j + 1 + n
		default:
			return 0, errTorrentParse
		}
	}
	return 0, errTorrentParse
}

package strmscrape

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

func relUnder(root, full string) string {
	if absRoot, err := filepath.Abs(root); err == nil {
		root = absRoot
	}
	if absFull, err := filepath.Abs(full); err == nil {
		full = absFull
	}
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return filepath.Base(full)
	}
	return rel
}

func isInside(root, full string) bool {
	if absRoot, err := filepath.Abs(root); err == nil {
		root = absRoot
	}
	if absFull, err := filepath.Abs(full); err == nil {
		full = absFull
	}
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func pathToItemID(rel string) string {
	sum := sha1.Sum([]byte(filepath.ToSlash(rel)))
	return hex.EncodeToString(sum[:8])
}

// CustomRootTaskID 为自定义刮削目录派生稳定的索引键。
// 取绝对路径的 sha1 前 8 字节，强制非负后再取负，落入负数空间——
// 与真实 STRM 任务 ID（正数自增）永不冲突；同一绝对路径总是得到同一键。
func CustomRootTaskID(root string) int64 {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = filepath.Clean(root)
	}
	sum := sha1.Sum([]byte(filepath.ToSlash(abs)))
	v := int64(binary.BigEndian.Uint64(sum[:8]) & 0x7FFFFFFFFFFFFFFF) // 符号位强制为 0
	if v == 0 {
		v = 1
	}
	return -v
}

func pathEscape(p string) string {
	return url.QueryEscape(filepath.ToSlash(p))
}

func anyString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprint(v)
	}
}

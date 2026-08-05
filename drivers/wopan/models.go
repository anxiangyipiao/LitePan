package wopan

import (
	"strings"
	"time"

	"litepan/internal/domain"

	wo "github.com/OpenListTeam/wopan-sdk-go"
)

// fileToItem 把平台文件转换为 LitePan 的 domain.FileItem。
// 平台 type：0 目录 / 1 文件。
func fileToItem(f *wo.File) domain.FileItem {
	return domain.FileItem{
		ID:      encodeWopanID(f.Id, f.Fid, f.Type == 0),
		Name:    f.Name,
		Size:    f.Size,
		IsDir:   f.Type == 0,
		ModTime: parseWopanTime(f.CreateTime),
		Thumb:   f.ThumbUrl,
		IDKind:  domain.IDStable,
	}
}

// parseWopanTime 解析平台时间（"20060102150405"，东八区）。
func parseWopanTime(s string) time.Time {
	t, err := time.ParseInLocation("20060102150405", strings.TrimSpace(s), time.FixedZone("UTC+8", 8*60*60))
	if err != nil {
		return time.Time{}
	}
	return t
}

package strmscrape

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxMovieFanart 多张轮播背景图的上限（extrafanart/fanart1..N）。
const maxMovieFanart = 5

// movieArtworkSource 是电影背景图写入所需的数据源能力，便于测试用小 fake 注入。
// *scrapeSource 实现天然满足。
type movieArtworkSource interface {
	tmdbImageDownloader
	FetchMovieBackdrops(ctx context.Context, id string) ([]string, error)
}

// movieBackdropPath 返回电影主背景图的写入路径：
// flatFile 单文件容器用 Kodi/Emby 兼容的 {stem}-fanart.jpg，否则目录内 backdrop.jpg。
func movieBackdropPath(g workGroup) string {
	if g.flatFile != "" {
		return strings.TrimSuffix(g.flatFile, filepath.Ext(g.flatFile)) + "-fanart.jpg"
	}
	return filepath.Join(g.absDir, "backdrop.jpg")
}

// movieExtrafanartDir 返回多张轮播背景图目录；flatFile 不建 extrafanart（避免污染库根），返回空。
func movieExtrafanartDir(g workGroup) string {
	if g.flatFile != "" {
		return ""
	}
	return filepath.Join(g.absDir, "extrafanart")
}

// writeArtworkWithSize 与 writeOptionalArtwork 同语义（下载失败仅警告不中断），仅多一个 size 参数。
func (s *Service) writeArtworkWithSize(ctx context.Context, client tmdbImageDownloader, imagePath, size, outputPath, label string) error {
	data, err := client.DownloadImage(ctx, imagePath, size)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if s != nil && s.log != nil {
			s.log.Warn("STRM 刮削可选图片下载失败，已跳过",
				"artwork", label,
				"output", outputPath,
				"error", err,
			)
		}
		return nil
	}
	if err := writeImageFile(outputPath, data); err != nil {
		return fmt.Errorf("写入%s：%w", label, err)
	}
	return nil
}

// writeMovieExtras 为电影补写背景图：主背景 backdrop.jpg + 多张轮播 extrafanart/fanartN.jpg。
// 主背景与轮播各自独立降级——下载/列表获取失败仅警告，本地写失败保留错误。
func (s *Service) writeMovieExtras(ctx context.Context, client movieArtworkSource, g workGroup, info tmdbInfo, overwrite bool) error {
	// 主背景图：详情 backdrop_path 缺失或下载失败仅警告，不中断刮削。
	out := movieBackdropPath(g)
	if (overwrite || !fileExists(out)) && strings.TrimSpace(info.BackdropPath) != "" {
		if err := s.writeArtworkWithSize(ctx, client, info.BackdropPath, "w1280", out, "电影背景"); err != nil {
			return err
		}
	}

	// 多张轮播背景图：仅普通目录（flatFile 已在 movieExtrafanartDir 返回空）。
	dir := movieExtrafanartDir(g)
	if dir == "" {
		return nil
	}
	// 轮播素材优先用详情已带回的预览截图（MetaTube，避免重复拉详情）；
	// TMDB 源 PreviewImages 为空，走 FetchMovieBackdrops 拉取 /movie/{id}/images。
	backdrops := info.PreviewImages
	if len(backdrops) == 0 {
		fetched, err := client.FetchMovieBackdrops(ctx, strings.TrimSpace(info.TMDBID))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if s != nil && s.log != nil {
				s.log.Warn("STRM 刮削背景图列表获取失败，已跳过",
					"output", dir,
					"error", err,
				)
			}
			return nil
		}
		backdrops = fetched
	}
	// 去掉与主背景重复的项与空串，取前 maxMovieFanart 张。
	filtered := make([]string, 0, len(backdrops))
	for _, b := range backdrops {
		if strings.TrimSpace(b) == "" || strings.TrimSpace(b) == info.BackdropPath {
			continue
		}
		filtered = append(filtered, b)
		if len(filtered) >= maxMovieFanart {
			break
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	// 目录存在即视为已刮过，非覆盖模式保留用户手动调整的图。
	if !overwrite && dirExists(dir) {
		return nil
	}
	if overwrite {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("清理 %s：%w", dir, err)
		}
	}
	for i, b := range filtered {
		outPath := filepath.Join(dir, fmt.Sprintf("fanart%d.jpg", i+1))
		if err := s.writeArtworkWithSize(ctx, client, b, "w1280", outPath, fmt.Sprintf("第 %d 张背景图", i+1)); err != nil {
			return err
		}
	}
	return nil
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

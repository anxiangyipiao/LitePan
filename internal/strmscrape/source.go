package strmscrape

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"litepan/internal/domain"
	"litepan/internal/mediaorganize/tmdb"
	"litepan/internal/metatube"
)

// scrapeSource 是 STRM 刮削的元数据源抽象。目前有两个实现：
//   - tmdbScrapeSource：包一层 *tmdb.Client，行为与改动前一致；
//   - *metatube.Client：MetaTube REST v1（仅电影，按番号刮削）。
//
// 两个实现都把数据规整成 TMDB 形状的 JSON，因此 decodeTMDBInfo 与写入管道可共用。
type scrapeSource interface {
	Search(ctx context.Context, query string, year *int, mediaType string) ([]json.RawMessage, error)
	// Lookup 按存储的 ID 解析一部作品，返回其完整元数据（TMDB 形状）。
	// TMDB 源按数字 ID 直查；MetaTube 源按番号搜索匹配。
	Lookup(ctx context.Context, id string, mediaType string) (json.RawMessage, error)
	// EnrichSearchResult 用详情数据补齐搜索命中（MetaTube 搜索命中缺 summary/genres 等）。
	// TMDB 源原样返回。
	EnrichSearchResult(ctx context.Context, raw json.RawMessage, mediaType string) (json.RawMessage, error)
	FetchTVSeasons(ctx context.Context, showID string) ([]json.RawMessage, error)
	FetchTVSeason(ctx context.Context, showID string, season int) (json.RawMessage, error)
	// FetchMovieBackdrops 返回电影的背景图 file_path 列表；数据源无背景图时返回空。
	FetchMovieBackdrops(ctx context.Context, id string) ([]string, error)
	DownloadImage(ctx context.Context, imagePath, size string) ([]byte, error)
}

// tmdbScrapeSource 让 *tmdb.Client 满足 scrapeSource。
type tmdbScrapeSource struct {
	client *tmdb.Client
}

func (t tmdbScrapeSource) Search(ctx context.Context, query string, year *int, mediaType string) ([]json.RawMessage, error) {
	return t.client.Search(ctx, query, year, mediaType)
}

func (t tmdbScrapeSource) Lookup(ctx context.Context, id string, mediaType string) (json.RawMessage, error) {
	return t.client.Lookup(ctx, id, mediaType)
}

func (t tmdbScrapeSource) EnrichSearchResult(ctx context.Context, raw json.RawMessage, mediaType string) (json.RawMessage, error) {
	return raw, nil
}

func (t tmdbScrapeSource) FetchTVSeasons(ctx context.Context, showID string) ([]json.RawMessage, error) {
	return t.client.FetchTVSeasons(ctx, showID)
}

func (t tmdbScrapeSource) FetchTVSeason(ctx context.Context, showID string, season int) (json.RawMessage, error) {
	return t.client.FetchTVSeason(ctx, showID, season)
}

func (t tmdbScrapeSource) DownloadImage(ctx context.Context, imagePath, size string) ([]byte, error) {
	return t.client.DownloadImage(ctx, imagePath, size)
}

func (t tmdbScrapeSource) FetchMovieBackdrops(ctx context.Context, id string) ([]string, error) {
	raw, err := t.client.FetchMovieImages(ctx, id)
	if err != nil {
		return nil, err
	}
	return parseMovieBackdrops(raw), nil
}

// parseMovieBackdrops 解析 /movie/{id}/images 返回里的 backdrops file_path 列表。
func parseMovieBackdrops(raw json.RawMessage) []string {
	var payload struct {
		Backdrops []struct {
			FilePath string `json:"file_path"`
		} `json:"backdrops"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	out := make([]string, 0, len(payload.Backdrops))
	for _, b := range payload.Backdrops {
		if s := strings.TrimSpace(b.FilePath); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// newScrapeClient 按当前设置返回可用的元数据源；未配置对应源时返回 nil。
func (s *Service) newScrapeClient() scrapeSource {
	cfg := s.GetSettings()
	if cfg.Source == SourceMetaTube {
		baseURL := cfg.MetaTubeURL
		if baseURL == "" {
			return nil
		}
		return metatube.NewClient(metatube.Options{
			BaseURL:        baseURL,
			Timeout:        60 * time.Second,
			MaxRetries:     2,
			RetryBaseDelay: time.Second,
		})
	}
	client := s.newTMDBClient()
	if client == nil {
		return nil
	}
	return tmdbScrapeSource{client: client}
}

// requireScrapeClient 返回可用的元数据源，未配置时给出针对当前源的中文提示。
func (s *Service) requireScrapeClient() (scrapeSource, error) {
	c := s.newScrapeClient()
	if c == nil {
		if s.GetSettings().Source == SourceMetaTube {
			return nil, domain.Errorf(domain.CodeValidation, "未配置 MetaTube API 地址，请先在设置中填写")
		}
		return nil, domain.Errorf(domain.CodeValidation, "未配置 TMDB API Key，请先在设置中填写")
	}
	return c, nil
}

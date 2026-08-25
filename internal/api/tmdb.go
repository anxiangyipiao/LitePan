package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// tmdbAPIKey 返回 TMDB API Key。
// 复用媒体整理功能的 TMDB API Key 设置项。
func (h *Handler) tmdbAPIKey() string {
	return h.settings.String("mo_tmdb_api_key")
}

const tmdbBaseURL = "https://api.themoviedb.org/3"

// newTmdbClient 创建支持代理的 HTTP 客户端。
func (h *Handler) newTmdbClient(ctx context.Context) (*http.Client, error) {
	enabled := h.settings.Bool("mo_proxy_enabled")
	proxyURL := h.settings.String("mo_proxy_url")

	transport := &http.Transport{}

	if enabled && proxyURL != "" {
		pu, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("代理地址解析失败: %w", err)
		}

		username := h.settings.String("mo_proxy_username")
		password := h.settings.String("mo_proxy_password")

		switch pu.Scheme {
		case "socks5":
			auth := &proxy.Auth{}
			if username != "" {
				auth.User = username
				auth.Password = password
			}
			dialer, err := proxy.SOCKS5("tcp", pu.Host, auth, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("SOCKS5 代理初始化失败: %w", err)
			}
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		case "http", "https":
			if username != "" {
				transport.Proxy = func(req *http.Request) (*url.URL, error) {
					u := *pu
					u.User = url.UserPassword(username, password)
					return &u, nil
				}
			} else {
				transport.Proxy = http.ProxyURL(pu)
			}
		default:
			return nil, fmt.Errorf("不支持的代理协议: %s", pu.Scheme)
		}
	}

	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
	}, nil
}

// errorResp 创建错误响应。
func errorResp(msg string) Resp {
	return Resp{Success: false, Message: msg, ErrorType: "TMDB_ERROR"}
}

// tmdbDo 发起 TMDB API 请求并解析响应。
// 支持缓存：列表类请求缓存 1 小时，详情类请求缓存 24 小时。
func (h *Handler) tmdbDo(ctx context.Context, apiKey, path string, query url.Values, target any, cacheTTL ...time.Duration) error {
	if apiKey == "" {
		return fmt.Errorf("TMDB API Key 未配置，请在系统设置中配置")
	}
	if query == nil {
		query = url.Values{}
	}

	// 构建缓存键
	cacheKey := fmt.Sprintf("tmdb:%s:%s", path, query.Encode())

	// 尝试从缓存读取
	ttl := time.Hour // 默认缓存 1 小时
	if len(cacheTTL) > 0 && cacheTTL[0] > 0 {
		ttl = cacheTTL[0]
	}

	if cached, ok := h.cache.Get(cacheKey); ok {
		if data, ok := cached.(json.RawMessage); ok {
			return json.Unmarshal(data, target)
		}
	}

	query.Set("api_key", apiKey)
	query.Set("language", "zh-CN")

	reqURL := fmt.Sprintf("%s%s?%s", tmdbBaseURL, path, query.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LitePan/1.0")

	client, err := h.newTmdbClient(ctx)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("TMDB 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取 TMDB 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("TMDB 返回错误 (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// 写入缓存
	h.cache.Set(cacheKey, json.RawMessage(body), ttl)

	return json.Unmarshal(body, target)
}

// tmdbSearch 搜索影视。
func (h *Handler) tmdbSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusBadRequest, errorResp("搜索关键词不能为空"))
		return
	}
	page := r.URL.Query().Get("page")
	if page == "" {
		page = "1"
	}

	type result struct {
		Page         int             `json:"page"`
		Results      json.RawMessage `json:"results"`
		TotalPages   int             `json:"total_pages"`
		TotalResults int             `json:"total_results"`
	}

	var res result
	query := url.Values{}
	query.Set("query", q)
	query.Set("page", page)
	query.Set("include_adult", "false")

	err := h.tmdbDo(r.Context(), h.tmdbAPIKey(), "/search/multi", query, &res)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, successRaw(res))
}

// tmdbPopular 获取热门影视。
func (h *Handler) tmdbPopular(w http.ResponseWriter, r *http.Request) {
	mediaType := r.URL.Query().Get("type")
	if mediaType == "" {
		mediaType = "movie"
	}
	page := r.URL.Query().Get("page")
	if page == "" {
		page = "1"
	}

	path := fmt.Sprintf("/%s/popular", mediaType)

	type result struct {
		Page         int             `json:"page"`
		Results      json.RawMessage `json:"results"`
		TotalPages   int             `json:"total_pages"`
		TotalResults int             `json:"total_results"`
	}

	var res result
	query := url.Values{}
	query.Set("page", page)

	err := h.tmdbDo(r.Context(), h.tmdbAPIKey(), path, query, &res)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, successRaw(res))
}

// tmdbTopRated 获取高分影视。
func (h *Handler) tmdbTopRated(w http.ResponseWriter, r *http.Request) {
	mediaType := r.URL.Query().Get("type")
	if mediaType == "" {
		mediaType = "movie"
	}
	page := r.URL.Query().Get("page")
	if page == "" {
		page = "1"
	}

	path := fmt.Sprintf("/%s/top_rated", mediaType)

	type result struct {
		Page         int             `json:"page"`
		Results      json.RawMessage `json:"results"`
		TotalPages   int             `json:"total_pages"`
		TotalResults int             `json:"total_results"`
	}

	var res result
	query := url.Values{}
	query.Set("page", page)

	err := h.tmdbDo(r.Context(), h.tmdbAPIKey(), path, query, &res)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, successRaw(res))
}

// tmdbNowPlaying 获取正在热映。
func (h *Handler) tmdbNowPlaying(w http.ResponseWriter, r *http.Request) {
	page := r.URL.Query().Get("page")
	if page == "" {
		page = "1"
	}

	type result struct {
		Page         int             `json:"page"`
		Results      json.RawMessage `json:"results"`
		TotalPages   int             `json:"total_pages"`
		TotalResults int             `json:"total_results"`
	}

	var res result
	query := url.Values{}
	query.Set("page", page)

	err := h.tmdbDo(r.Context(), h.tmdbAPIKey(), "/movie/now_playing", query, &res)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, successRaw(res))
}

// tmdbUpcoming 获取即将上映。
func (h *Handler) tmdbUpcoming(w http.ResponseWriter, r *http.Request) {
	page := r.URL.Query().Get("page")
	if page == "" {
		page = "1"
	}

	type result struct {
		Page         int             `json:"page"`
		Results      json.RawMessage `json:"results"`
		TotalPages   int             `json:"total_pages"`
		TotalResults int             `json:"total_results"`
	}

	var res result
	query := url.Values{}
	query.Set("page", page)

	err := h.tmdbDo(r.Context(), h.tmdbAPIKey(), "/movie/upcoming", query, &res)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, successRaw(res))
}

// tmdbMovieDetail 获取电影详情。
func (h *Handler) tmdbMovieDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResp("电影 ID 不能为空"))
		return
	}

	path := fmt.Sprintf("/movie/%s", id)

	var res json.RawMessage
	err := h.tmdbDo(r.Context(), h.tmdbAPIKey(), path, nil, &res, 24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, successRaw(json.RawMessage(res)))
}

// tmdbTvDetail 获取剧集详情。
func (h *Handler) tmdbTvDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResp("剧集 ID 不能为空"))
		return
	}

	path := fmt.Sprintf("/tv/%s", id)

	var res json.RawMessage
	err := h.tmdbDo(r.Context(), h.tmdbAPIKey(), path, nil, &res, 24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, successRaw(json.RawMessage(res)))
}

// tmdbCredits 获取演员列表。
func (h *Handler) tmdbCredits(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResp("ID 不能为空"))
		return
	}
	mediaType := r.URL.Query().Get("type")
	if mediaType == "" {
		mediaType = "movie"
	}

	path := fmt.Sprintf("/%s/%s/credits", mediaType, id)

	var res json.RawMessage
	err := h.tmdbDo(r.Context(), h.tmdbAPIKey(), path, nil, &res, 24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, successRaw(json.RawMessage(res)))
}

// tmdbImages 获取图片列表。
func (h *Handler) tmdbImages(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResp("ID 不能为空"))
		return
	}
	mediaType := r.URL.Query().Get("type")
	if mediaType == "" {
		mediaType = "movie"
	}

	path := fmt.Sprintf("/%s/%s/images", mediaType, id)

	var res json.RawMessage
	err := h.tmdbDo(r.Context(), h.tmdbAPIKey(), path, nil, &res, 24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, successRaw(json.RawMessage(res)))
}

// tmdbGenres 获取分类列表。
func (h *Handler) tmdbGenres(w http.ResponseWriter, r *http.Request) {
	mediaType := r.URL.Query().Get("type")
	if mediaType == "" {
		mediaType = "movie"
	}

	path := fmt.Sprintf("/genre/%s/list", mediaType)

	var res json.RawMessage
	err := h.tmdbDo(r.Context(), h.tmdbAPIKey(), path, nil, &res, 24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, successRaw(json.RawMessage(res)))
}

// tmdbDiscover 发现/推荐影视。
func (h *Handler) tmdbDiscover(w http.ResponseWriter, r *http.Request) {
	mediaType := r.URL.Query().Get("type")
	if mediaType == "" {
		mediaType = "movie"
	}
	sortBy := r.URL.Query().Get("sort_by")
	if sortBy == "" {
		sortBy = "popularity.desc"
	}
	genres := r.URL.Query().Get("with_genres")
	page := r.URL.Query().Get("page")
	if page == "" {
		page = "1"
	}

	path := fmt.Sprintf("/discover/%s", mediaType)

	type result struct {
		Page         int             `json:"page"`
		Results      json.RawMessage `json:"results"`
		TotalPages   int             `json:"total_pages"`
		TotalResults int             `json:"total_results"`
	}

	var res result
	query := url.Values{}
	query.Set("sort_by", sortBy)
	query.Set("page", page)
	if genres != "" {
		query.Set("with_genres", genres)
	}

	err := h.tmdbDo(r.Context(), h.tmdbAPIKey(), path, query, &res)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, successRaw(res))
}

// tmdbImage 代理 TMDB 图片请求，避免浏览器直连 image.tmdb.org（需要代理时更稳定）。
func (h *Handler) tmdbImage(w http.ResponseWriter, r *http.Request) {
	size := r.URL.Query().Get("s")
	if size == "" {
		size = "w500"
	}
	path := r.URL.Query().Get("p")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, errorResp("图片路径不能为空"))
		return
	}

	// 安全校验：path 必须以 / 开头且不含 ..
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		writeJSON(w, http.StatusBadRequest, errorResp("无效的图片路径"))
		return
	}

	imageURL := fmt.Sprintf("https://image.tmdb.org/t/p/%s%s", size, path)

	// 构建请求
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, imageURL, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}
	req.Header.Set("User-Agent", "LitePan/1.0")
	req.Header.Set("Accept", "image/*")

	// 使用代理客户端
	client, err := h.newTmdbClient(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(err.Error()))
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResp(fmt.Sprintf("图片请求失败: %v", err)))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeJSON(w, resp.StatusCode, errorResp(fmt.Sprintf("TMDB 图片返回 HTTP %d", resp.StatusCode)))
		return
	}

	// 返回图片数据
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	if resp.Header.Get("Content-Length") != "" {
		w.Header().Set("Content-Length", resp.Header.Get("Content-Length"))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, resp.Body)
}

// successRaw 直接返回原始 JSON 数据（已包装 success 结构）。
func successRaw(data any) Resp {
	bytes, err := json.Marshal(data)
	if err != nil {
		return Resp{Success: false, Message: "序列化失败", ErrorType: "INTERNAL_ERROR"}
	}
	raw := `{"success":true,"data":` + string(bytes) + `}`
	var wrapped map[string]any
	_ = json.Unmarshal([]byte(raw), &wrapped)
	return Resp{Success: true, Data: wrapped["data"]}
}
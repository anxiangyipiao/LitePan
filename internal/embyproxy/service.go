package embyproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"litepan/internal/domain"
	"litepan/internal/httpx"
	"litepan/internal/playback"
	"litepan/internal/settings"
	"litepan/internal/strm"
)

const maskedSecret = "******"

var (
	videoStreamPathRE    = regexp.MustCompile(`(?i)^(?:/?emby)?/?Videos/([^/]+)/(stream|original)(?:\.\w+)?$`)
	playbackInfoPathRE   = regexp.MustCompile(`(?i)^(?:/?emby)?/?Items/([^/]+)/PlaybackInfo$`)
	baseHTMLPlayerPathRE = regexp.MustCompile(`(?i)^(?:/?emby)?/?web/modules/htmlvideoplayer/basehtmlplayer\.js$`)
	strmPlayPathRE       = regexp.MustCompile(`(?i)^/api/strm/play/(\d+)/([^/]+)/t/([^/]+)/n/([^/?#\s]+)(?:/s/([^/?#\s]+))?$`)
	htmlCrossOriginRE    = regexp.MustCompile(`mediaSource\.IsRemote\s*&&\s*(?:"DirectPlay"\s*===\s*playMethod|playMethod\s*===\s*"DirectPlay")\s*\?\s*null\s*:\s*"anonymous"`)
	hopByHopHeaderNames  = map[string]struct{}{"connection": {}, "keep-alive": {}, "proxy-authenticate": {}, "proxy-authorization": {}, "te": {}, "trailers": {}, "transfer-encoding": {}, "upgrade": {}, "host": {}}
	testRequestTimeout   = 20 * time.Second
)

type Service struct {
	settings *settings.Service
	playback *playback.Service
	strm     *strm.Service
	log      *slog.Logger
	client   *http.Client

	servePlayback func(http.ResponseWriter, *http.Request, playback.Request, playback.Intent) error

	mu     sync.Mutex
	server *http.Server
	port   int
	err    string
}

type Options struct {
	Settings *settings.Service
	Playback *playback.Service
	Strm     *strm.Service
	Log      *slog.Logger
}

type Config struct {
	Enabled   bool   `json:"enabled"`
	EmbyURL   string `json:"emby_url"`
	APIKey    string `json:"api_key"`
	Port      string `json:"proxy_port"`
	ProxyURL  string `json:"proxy_url"`
	Running   bool   `json:"running"`
	LastError string `json:"last_error,omitempty"`
}

type UpdateRequest struct {
	Enabled bool   `json:"enabled"`
	EmbyURL string `json:"emby_url"`
	APIKey  string `json:"api_key"`
	Port    string `json:"proxy_port"`
}

type RefreshResult struct {
	Mode   string `json:"mode"`
	TaskID string `json:"task_id,omitempty"`
}

func New(opts Options) *Service {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	client := httpx.NewClient(httpx.ClientOptions{DisableCompression: true})
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Service{
		settings: opts.Settings,
		playback: opts.Playback,
		strm:     opts.Strm,
		log:      log,
		client:   client,
		servePlayback: func(w http.ResponseWriter, r *http.Request, req playback.Request, intent playback.Intent) error {
			if opts.Playback == nil {
				return domain.Errf(domain.CodeNotImplement)
			}
			return opts.Playback.ServeHTTP(w, r, req, intent)
		},
	}
}

func (s *Service) Snapshot(r *http.Request) Config {
	cfg := s.configFromSettings()
	cfg.APIKey = maskSecret(cfg.APIKey)
	if cfg.Port != "" {
		cfg.ProxyURL = publicBase(r, cfg.Port)
	}
	s.mu.Lock()
	cfg.Running = s.server != nil
	cfg.LastError = s.err
	s.mu.Unlock()
	return cfg
}

func (s *Service) Update(ctx context.Context, in UpdateRequest) (Config, error) {
	if s.settings == nil {
		return Config{}, domain.Errf(domain.CodeNotImplement)
	}
	embyURL, err := normalizeEmbyURL(in.EmbyURL, false)
	if err != nil {
		return Config{}, err
	}
	port, err := normalizeOptionalPort(in.Port)
	if err != nil {
		return Config{}, err
	}
	apiKey := strings.TrimSpace(in.APIKey)
	storedAPIKey := s.configFromSettings().APIKey
	effectiveAPIKey := apiKey
	if isStoredSecretInput(apiKey, storedAPIKey) {
		effectiveAPIKey = storedAPIKey
	}
	if in.Enabled && port != "" {
		if embyURL == "" {
			return Config{}, domain.Errorf(domain.CodeValidation, "启用 Emby 反代并填写端口时，需要填写 Emby 地址")
		}
		if effectiveAPIKey == "" {
			return Config{}, domain.Errorf(domain.CodeValidation, "启用 Emby 反代并填写端口时，需要填写 Emby API Key")
		}
		if err := s.checkPortConflict(port); err != nil {
			return Config{}, err
		}
	}
	changed := map[string]string{
		settings.KeyEmbyEnabled:   strconv.FormatBool(in.Enabled),
		settings.KeyEmbyURL:       embyURL,
		settings.KeyEmbyProxyPort: port,
	}
	if !isStoredSecretInput(apiKey, storedAPIKey) {
		changed[settings.KeyEmbyAPIKey] = apiKey
	}
	if err := s.settings.Update(ctx, changed); err != nil {
		return Config{}, err
	}
	if err := s.Sync(ctx); err != nil {
		return s.Snapshot(nil), err
	}
	return s.Snapshot(nil), nil
}

func (s *Service) checkPortConflict(port string) error {
	if s.settings == nil || port == "" {
		return nil
	}
	if !s.settings.Bool(settings.KeyFnosEnabled) {
		return nil
	}
	fnosPort := strings.TrimSpace(s.settings.String(settings.KeyFnosProxyPort))
	if fnosPort != "" && fnosPort == port {
		return domain.Errorf(domain.CodeValidation, "反代端口与飞牛影视反代端口冲突")
	}
	return nil
}

func (s *Service) Test(ctx context.Context) error {
	return s.TestConfig(ctx, s.configFromSettings())
}

func (s *Service) TestUpdate(ctx context.Context, in UpdateRequest) error {
	cfg, err := ConfigFromUpdate(in)
	if err != nil {
		return err
	}
	if isStoredSecretInput(in.APIKey, s.configFromSettings().APIKey) {
		cfg.APIKey = s.configFromSettings().APIKey
	}
	return s.TestConfig(ctx, cfg)
}

func (s *Service) TestConfig(ctx context.Context, cfg Config) error {
	if strings.TrimSpace(cfg.EmbyURL) == "" {
		return domain.Errorf(domain.CodeValidation, "请先填写 Emby 地址")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return domain.Errorf(domain.CodeValidation, "请先填写 Emby API Key")
	}
	testCtx, cancel := context.WithTimeout(ctx, testRequestTimeout)
	defer cancel()
	testURL := cfg.EmbyURL + "/System/Info?" + url.Values{"api_key": {cfg.APIKey}}.Encode()
	req, err := http.NewRequestWithContext(testCtx, http.MethodGet, testURL, nil)
	if err != nil {
		return domain.Errorf(domain.CodeValidation, "Emby 地址无效")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return embyTestConnectError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return embyTestHTTPError(resp.StatusCode)
	}
	return nil
}

func (s *Service) RefreshLibrary(ctx context.Context) (RefreshResult, error) {
	cfg := s.configFromSettings()
	if strings.TrimSpace(cfg.EmbyURL) == "" {
		return RefreshResult{}, domain.Errorf(domain.CodeValidation, "请先填写 Emby 地址")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return RefreshResult{}, domain.Errorf(domain.CodeValidation, "请先填写 Emby API Key")
	}
	query := url.Values{"api_key": {cfg.APIKey}}.Encode()
	base := strings.TrimRight(cfg.EmbyURL, "/")
	taskID, err := s.findLibraryRefreshTask(ctx, base, query)
	if err == nil && taskID != "" {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, base+"/ScheduledTasks/Running/"+taskID+"?"+query, nil)
		if reqErr != nil {
			return RefreshResult{}, domain.Wrap(domain.CodeInternal, reqErr)
		}
		resp, doErr := s.client.Do(req)
		if doErr != nil {
			return RefreshResult{}, embyTestConnectError(doErr)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 400 {
			return RefreshResult{Mode: "scheduled_task", TaskID: taskID}, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/Library/Refresh?"+query, nil)
	if err != nil {
		return RefreshResult{}, domain.Wrap(domain.CodeInternal, err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return RefreshResult{}, embyTestConnectError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return RefreshResult{}, embyTestHTTPError(resp.StatusCode)
	}
	return RefreshResult{Mode: "library_refresh"}, nil
}

func (s *Service) findLibraryRefreshTask(ctx context.Context, base, query string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/ScheduledTasks?"+query, nil)
	if err != nil {
		return "", err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", domain.Errorf(domain.CodeDriverError, "读取 Emby 计划任务失败")
	}
	var tasks []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return "", err
	}
	for _, task := range tasks {
		name := strings.ToLower(strings.TrimSpace(anyString(task["Name"])))
		key := strings.TrimSpace(anyString(task["Key"]))
		if strings.Contains(name, "扫描媒体库") || strings.Contains(name, "refresh library") || strings.Contains(name, "scan media library") || key == "RefreshLibrary" {
			return strings.TrimSpace(anyString(task["Id"])), nil
		}
	}
	return "", nil
}

func ConfigFromUpdate(in UpdateRequest) (Config, error) {
	embyURL, err := normalizeEmbyURL(in.EmbyURL, false)
	if err != nil {
		return Config{}, err
	}
	port, err := normalizeOptionalPort(in.Port)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Enabled: in.Enabled,
		EmbyURL: embyURL,
		APIKey:  strings.TrimSpace(in.APIKey),
		Port:    port,
	}, nil
}

func (s *Service) Start(ctx context.Context) {
	if err := s.Sync(ctx); err != nil {
		s.log.Warn("Emby 反代启动失败", "error", err)
	}
}

func (s *Service) Sync(ctx context.Context) error {
	cfg := s.configFromSettings()
	port, _ := strconv.Atoi(cfg.Port)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil && (port == 0 || !cfg.Enabled || s.port != port) {
		s.stopLocked(ctx)
	}
	s.err = ""
	if !cfg.Enabled || port == 0 {
		return nil
	}
	if cfg.EmbyURL == "" || cfg.APIKey == "" {
		s.err = "启用反代时需要填写 Emby 地址和 API Key"
		return domain.Errorf(domain.CodeValidation, "%s", s.err)
	}
	if err := s.checkPortConflict(cfg.Port); err != nil {
		s.err = err.Error()
		return err
	}
	if s.server != nil && s.port == port {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		s.err = fmt.Sprintf("Emby 反代端口 %d 监听失败：%v", port, err)
		return domain.Errorf(domain.CodeDriverError, "%s", s.err)
	}
	s.server = srv
	s.port = port
	go func() {
		s.log.Info("Emby 反代已监听", "addr", srv.Addr)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			s.err = err.Error()
			s.server = nil
			s.port = 0
			s.mu.Unlock()
			s.log.Error("Emby 反代服务异常退出", "error", err)
		}
	}()
	return nil
}

func (s *Service) Shutdown(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked(ctx)
}

func (s *Service) stopLocked(ctx context.Context) {
	if s.server == nil {
		return
	}
	srv := s.server
	s.server = nil
	s.port = 0
	stopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(stopCtx); err != nil {
		_ = srv.Close()
	}
}

func (s *Service) configFromSettings() Config {
	if s.settings == nil {
		return Config{}
	}
	return Config{
		Enabled: s.settings.Bool(settings.KeyEmbyEnabled),
		EmbyURL: strings.TrimRight(strings.TrimSpace(s.settings.String(settings.KeyEmbyURL)), "/"),
		APIKey:  strings.TrimSpace(s.settings.String(settings.KeyEmbyAPIKey)),
		Port:    strings.TrimSpace(s.settings.String(settings.KeyEmbyProxyPort)),
	}
}

func (s *Service) handle(w http.ResponseWriter, r *http.Request) {
	if strmPlayPathRE.MatchString(r.URL.Path) {
		s.serveSTRM(w, r)
		return
	}
	cfg := s.configFromSettings()
	if !cfg.Enabled || cfg.EmbyURL == "" || cfg.APIKey == "" {
		http.Error(w, "Emby proxy is not enabled", http.StatusNotFound)
		return
	}
	fullPath := strings.TrimPrefix(r.URL.Path, "/")
	if videoStreamPathRE.MatchString(fullPath) {
		s.redirectSTRMStream(w, r, cfg, fullPath)
		return
	}
	if playbackInfoPathRE.MatchString(fullPath) && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
		s.modifyPlaybackInfo(w, r, cfg, fullPath)
		return
	}
	if baseHTMLPlayerPathRE.MatchString(fullPath) && r.Method == http.MethodGet {
		s.modifyBaseHTMLPlayer(w, r, cfg, fullPath)
		return
	}
	s.proxyRequest(w, r, cfg, fullPath)
}

func (s *Service) serveSTRM(w http.ResponseWriter, r *http.Request) {
	if s.strm == nil || s.playback == nil {
		http.Error(w, "STRM playback is unavailable", http.StatusNotImplemented)
		return
	}
	m := strmPlayPathRE.FindStringSubmatch(r.URL.Path)
	if len(m) < 5 {
		http.NotFound(w, r)
		return
	}
	accountID, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || accountID <= 0 {
		http.Error(w, "invalid account id", http.StatusBadRequest)
		return
	}
	fileID, err := strm.DecodeFileKey(m[2])
	if err != nil {
		http.Error(w, "invalid file key", http.StatusBadRequest)
		return
	}
	ok, err := s.strm.MatchToken(r.Context(), m[3])
	if err != nil || !ok {
		http.Error(w, "permission denied", http.StatusForbidden)
		return
	}
	signature := ""
	if len(m) > 5 {
		signature = m[5]
	}
	if s.strm.SignatureEnabled() {
		if signature == "" {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		unsignedPath := strings.TrimSuffix(r.URL.EscapedPath(), "/s/"+signature)
		if !s.strm.VerifySignature(unsignedPath, signature) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
	}
	name, _ := url.PathUnescape(m[4])
	if err := s.playbackServe(w, r, playback.Request{AccountID: accountID, FileID: fileID}, playback.Intent{FileName: name}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Service) playbackServe(w http.ResponseWriter, r *http.Request, req playback.Request, intent playback.Intent) error {
	if s.servePlayback == nil {
		return domain.Errf(domain.CodeNotImplement)
	}
	return s.servePlayback(w, r, req, intent)
}

func (s *Service) serveLitePanPlayback(w http.ResponseWriter, r *http.Request, litepanURL string) bool {
	accountID, fileID, ok := parseLitePanSTRMURL(litepanURL)
	if !ok {
		return false
	}
	if err := s.playbackServe(w, r, playback.Request{AccountID: accountID, FileID: fileID}, playback.Intent{}); err != nil {
		s.log.Warn("Emby 反代解析 LitePan STRM 失败", "url", litepanURL, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	return true
}

func (s *Service) redirectSTRMStream(w http.ResponseWriter, r *http.Request, cfg Config, fullPath string) {
	if r.Method == http.MethodHead {
		s.proxyRequest(w, r, cfg, fullPath)
		return
	}
	mediaSourceID := queryValue(r, "mediasourceid")
	if mediaSourceID == "" {
		s.proxyRequest(w, r, cfg, fullPath)
		return
	}
	itemID := ""
	if m := videoStreamPathRE.FindStringSubmatch(fullPath); len(m) > 1 {
		itemID = m[1]
	}
	item := s.fetchEmbyItem(r.Context(), cfg, itemID)
	if item == nil {
		item = s.fetchEmbyItem(r.Context(), cfg, stripMediaSourcePrefix(mediaSourceID))
	}
	if item == nil {
		s.proxyRequest(w, r, cfg, fullPath)
		return
	}
	target := stripMediaSourcePrefix(mediaSourceID)
	for _, mediaSource := range mediaSources(item) {
		current := stripMediaSourcePrefix(stringValue(mediaSource, "Id", "ID"))
		if current != "" && current != target {
			continue
		}
		if redirectURL := s.extractLitePanSTRM(mediaSource, r, cfg); redirectURL != "" {
			if s.serveLitePanPlayback(w, r, redirectURL) {
				return
			}
		}
	}
	itemPath := normalizeMediaURL(stringValue(item, "Path"), r, cfg)
	if isLitePanSTRMURL(itemPath) {
		if s.serveLitePanPlayback(w, r, itemPath) {
			return
		}
	}
	s.proxyRequest(w, r, cfg, fullPath)
}

func (s *Service) modifyPlaybackInfo(w http.ResponseWriter, r *http.Request, cfg Config, fullPath string) {
	resp, body, err := s.requestUpstream(r, cfg, fullPath, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		writeUpstreamBody(w, resp, body)
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeUpstreamBody(w, resp, body)
		return
	}
	itemID := ""
	if m := playbackInfoPathRE.FindStringSubmatch(fullPath); len(m) > 1 {
		itemID = m[1]
	}
	item := s.fetchEmbyItem(r.Context(), cfg, itemID)
	changed := false
	for _, mediaSource := range mediaSources(payload) {
		mediaSourceID := stringValue(mediaSource, "Id", "ID")
		itemMediaSource := findMediaSourceByID(item, mediaSourceID)
		litepanURL := s.extractLitePanSTRM(mediaSource, r, cfg)
		if litepanURL == "" && itemMediaSource != nil {
			litepanURL = s.extractLitePanSTRM(itemMediaSource, r, cfg)
		}
		itemPath := normalizeMediaURL(stringValue(item, "Path"), r, cfg)
		if litepanURL == "" && isLitePanSTRMURL(itemPath) {
			litepanURL = itemPath
		}
		if litepanURL == "" {
			continue
		}
		directPath := proxiedVideoPath(r, cfg, firstNonEmpty(itemID, stripMediaSourcePrefix(mediaSourceID)), mediaSourceID)
		mediaSource["SupportsDirectPlay"] = true
		mediaSource["SupportsDirectStream"] = true
		mediaSource["SupportsTranscoding"] = false
		mediaSource["Protocol"] = "Http"
		mediaSource["DirectStreamUrl"] = directPath
		delete(mediaSource, "TranscodingUrl")
		delete(mediaSource, "TranscodingSubProtocol")
		delete(mediaSource, "TranscodingContainer")
		delete(mediaSource, "TranscodingLiveStartIndex")
		delete(mediaSource, "TrancodeLiveStartIndex")
		if info := s.inferLitePanMediaInfo(r.Context(), litepanURL, r.UserAgent()); info != nil {
			for k, v := range info {
				mediaSource[k] = v
			}
		}
		changed = true
	}
	if !changed {
		writeUpstreamBody(w, resp, body)
		return
	}
	out, _ := json.Marshal(payload)
	headers := responseHeaders(resp.Header)
	headers.Set("Content-Type", "application/json; charset=utf-8")
	headers.Set("Content-Length", strconv.Itoa(len(out)))
	headers.Del("Content-Encoding")
	writeHeaders(w, headers)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(out)
}

func (s *Service) modifyBaseHTMLPlayer(w http.ResponseWriter, r *http.Request, cfg Config, fullPath string) {
	resp, body, err := s.requestUpstream(r, cfg, fullPath, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	crossOriginGuard := []byte(";try{(function(){var s=Element.prototype.setAttribute;Element.prototype.setAttribute=function(n,v){if(this&&this.tagName&&/^(VIDEO|AUDIO)$/i.test(this.tagName)&&String(n).toLowerCase()==='crossorigin')return;return s.call(this,n,v)};try{Object.defineProperty(HTMLMediaElement.prototype,'crossOrigin',{get:function(){return null},set:function(){return null},configurable:true})}catch(e){}})()}catch(e){};")
	body = bytes.ReplaceAll(body, []byte(`mediaSource.IsRemote&&"DirectPlay"===playMethod?null:"anonymous"`), []byte("null"))
	body = htmlCrossOriginRE.ReplaceAll(body, []byte("null"))
	if !bytes.Contains(body, []byte("HTMLMediaElement.prototype,'crossOrigin'")) {
		body = append(crossOriginGuard, body...)
	}
	headers := responseHeaders(resp.Header)
	headers.Set("Content-Length", strconv.Itoa(len(body)))
	headers.Set("Cache-Control", "no-store")
	headers.Del("Content-Encoding")
	writeHeaders(w, headers)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func (s *Service) proxyRequest(w http.ResponseWriter, r *http.Request, cfg Config, fullPath string) {
	target, err := targetURL(cfg, fullPath, r.URL.RawQuery)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var body io.Reader
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		data, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, readErr.Error(), http.StatusBadRequest)
			return
		}
		if len(data) > 0 {
			body = bytes.NewReader(data)
		}
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	copyRequestHeaders(req.Header, r.Header, false)
	resp, err := s.client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	headers := responseHeaders(resp.Header)
	if loc := headers.Get("Location"); loc != "" {
		headers.Set("Location", rewriteLocation(loc, cfg, r))
	}
	writeHeaders(w, headers)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.CopyBuffer(w, resp.Body, make([]byte, 128*1024))
}

func (s *Service) requestUpstream(r *http.Request, cfg Config, fullPath string, identity bool) (*http.Response, []byte, error) {
	target, err := targetURL(cfg, fullPath, r.URL.RawQuery)
	if err != nil {
		return nil, nil, err
	}
	var body io.Reader
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		buf, _ := io.ReadAll(r.Body)
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		return nil, nil, err
	}
	copyRequestHeaders(req.Header, r.Header, identity)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return resp, data, nil
}

func (s *Service) fetchEmbyItem(ctx context.Context, cfg Config, itemID string) map[string]any {
	itemID = stripMediaSourcePrefix(itemID)
	if itemID == "" {
		return nil
	}
	params := url.Values{
		"Ids":       {itemID},
		"Limit":     {"1"},
		"Fields":    {"Path,MediaSources"},
		"Recursive": {"true"},
		"api_key":   {cfg.APIKey},
	}
	base := strings.TrimRight(cfg.EmbyURL, "/")
	candidates := []string{base + "/emby/Items", base + "/Items"}
	if strings.HasSuffix(strings.ToLower(base), "/emby") {
		candidates = []string{base + "/Items"}
	}
	for _, endpoint := range candidates {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
		if err != nil {
			continue
		}
		resp, err := s.client.Do(req)
		if err != nil {
			continue
		}
		var payload struct {
			Items []map[string]any `json:"Items"`
		}
		err = json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		if err == nil && resp.StatusCode < 400 && len(payload.Items) > 0 {
			return payload.Items[0]
		}
	}
	return nil
}

func (s *Service) extractLitePanSTRM(mediaSource map[string]any, r *http.Request, cfg Config) string {
	for _, key := range []string{"Path", "DirectStreamUrl", "DirectStreamURL", "TranscodingUrl"} {
		value := normalizeMediaURL(stringValue(mediaSource, key), r, cfg)
		if isLitePanSTRMURL(value) {
			return value
		}
	}
	return ""
}

func (s *Service) inferLitePanMediaInfo(ctx context.Context, litepanURL, ua string) map[string]any {
	if s.playback == nil {
		return nil
	}
	accountID, fileID, ok := parseLitePanSTRMURL(litepanURL)
	if !ok {
		return nil
	}
	res, err := s.playback.Resolve(ctx, accountID, fileID, ua, false)
	if err != nil {
		return nil
	}
	out := map[string]any{}
	name := strings.TrimSpace(res.File.Name)
	if name == "" {
		name = strings.TrimSpace(res.Link.FileName)
	}
	if name != "" {
		out["Name"] = name
		if ext := strings.TrimPrefix(strings.ToLower(path.Ext(name)), "."); ext != "" {
			out["Container"] = ext
		}
	}
	size := res.File.Size
	if size <= 0 {
		size = res.Link.Size
	}
	if size > 0 {
		out["Size"] = size
	}
	return out
}

func parseLitePanSTRMURL(value string) (int64, string, bool) {
	path := litepanPath(value)
	m := strmPlayPathRE.FindStringSubmatch(path)
	if len(m) < 3 {
		return 0, "", false
	}
	accountID, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || accountID <= 0 {
		return 0, "", false
	}
	fileID, err := strm.DecodeFileKey(m[2])
	if err != nil || fileID == "" {
		return 0, "", false
	}
	return accountID, fileID, true
}

func targetURL(cfg Config, fullPath, rawQuery string) (string, error) {
	baseStr := strings.TrimRight(strings.TrimSpace(cfg.EmbyURL), "/")
	path := strings.TrimPrefix(strings.TrimSpace(fullPath), "/")
	if strings.HasSuffix(strings.ToLower(baseStr), "/emby") && strings.HasPrefix(strings.ToLower(path), "emby/") {
		path = path[len("emby/"):]
	}
	base, err := url.Parse(baseStr + "/")
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	target := base.ResolveReference(ref)
	target.RawQuery = rawQuery
	return target.String(), nil
}

func normalizeEmbyURL(raw string, required bool) (string, error) {
	v := strings.TrimRight(strings.TrimSpace(raw), "/")
	if v == "" {
		if required {
			return "", domain.Errorf(domain.CodeValidation, "请填写 Emby 地址")
		}
		return "", nil
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", domain.Errorf(domain.CodeValidation, "Emby 地址格式不正确，示例：http://192.168.1.10:8096")
	}
	return v, nil
}

func normalizeOptionalPort(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return "", domain.Errorf(domain.CodeValidation, "反代端口必须是 1-65535")
	}
	return strconv.Itoa(n), nil
}

func publicBase(r *http.Request, port string) string {
	if r == nil {
		if port == "" {
			return ""
		}
		return "http://127.0.0.1:" + port
	}
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if port != "" {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = net.JoinHostPort(h, port)
		} else {
			host = net.JoinHostPort(strings.Split(host, ":")[0], port)
		}
	}
	return scheme + "://" + host
}

func proxiedVideoPath(r *http.Request, cfg Config, itemID, mediaSourceID string) string {
	q := r.URL.Query()
	q.Set("MediaSourceId", mediaSourceID)
	if q.Get("static") == "" {
		q.Set("static", "true")
	}
	if q.Get("api_key") == "" {
		q.Set("api_key", cfg.APIKey)
	}
	return "/Videos/" + itemID + "/stream?" + q.Encode()
}

func anyString(v any) string {
	switch got := v.(type) {
	case string:
		return got
	case json.Number:
		return got.String()
	case fmt.Stringer:
		return got.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	}
}

func normalizeMediaURL(candidate string, r *http.Request, cfg Config) string {
	value := cleanWrappedURL(candidate)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if strings.HasPrefix(value, "/") {
		return strings.TrimRight(publicBase(r, cfg.Port), "/") + value
	}
	return value
}

func isLitePanSTRMURL(value string) bool {
	return strings.HasPrefix(strings.ToLower(litepanPath(value)), "/api/strm/play/")
}

func litepanPath(value string) string {
	text := cleanWrappedURL(value)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
		u, err := url.Parse(text)
		if err != nil {
			return ""
		}
		return u.EscapedPath()
	}
	pathOnly, _, _ := strings.Cut(text, "?")
	return pathOnly
}

func cleanWrappedURL(value string) string {
	text := strings.TrimSpace(value)
	for {
		trimmed := strings.Trim(text, "`\"' ")
		if trimmed == text {
			return trimmed
		}
		text = strings.TrimSpace(trimmed)
	}
}

func responseHeaders(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for k, values := range src {
		if _, skip := hopByHopHeaderNames[strings.ToLower(k)]; skip {
			continue
		}
		for _, v := range values {
			dst.Add(k, v)
		}
	}
	return dst
}

func copyRequestHeaders(dst, src http.Header, identity bool) {
	for k, values := range src {
		if _, skip := hopByHopHeaderNames[strings.ToLower(k)]; skip {
			continue
		}
		for _, v := range values {
			dst.Add(k, v)
		}
	}
	if identity {
		dst.Set("Accept-Encoding", "identity")
	}
}

func writeHeaders(w http.ResponseWriter, headers http.Header) {
	for k, values := range headers {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
}

func writeUpstreamBody(w http.ResponseWriter, resp *http.Response, body []byte) {
	writeHeaders(w, responseHeaders(resp.Header))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func rewriteLocation(location string, cfg Config, r *http.Request) string {
	embyURL := strings.TrimRight(cfg.EmbyURL, "/")
	if strings.HasPrefix(location, embyURL) {
		return strings.TrimRight(publicBase(r, cfg.Port), "/") + strings.TrimPrefix(location, embyURL)
	}
	return location
}

func queryValue(r *http.Request, key string) string {
	target := strings.ToLower(key)
	for k, values := range r.URL.Query() {
		if strings.ToLower(k) == target && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func stripMediaSourcePrefix(value string) string {
	return strings.TrimPrefix(value, "mediasource_")
}

func stringValue(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return ""
}

func mediaSources(m map[string]any) []map[string]any {
	if m == nil {
		return nil
	}
	raw, ok := m["MediaSources"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if ms, ok := item.(map[string]any); ok {
			out = append(out, ms)
		}
	}
	return out
}

func findMediaSourceByID(item map[string]any, mediaSourceID string) map[string]any {
	target := stripMediaSourcePrefix(mediaSourceID)
	for _, mediaSource := range mediaSources(item) {
		current := stripMediaSourcePrefix(stringValue(mediaSource, "Id", "ID"))
		if current == "" || current == target {
			return mediaSource
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func maskSecret(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	n := len(v)
	if n <= 4 {
		if n <= 2 {
			return strings.Repeat("*", n)
		}
		return v[:1] + strings.Repeat("*", n-2) + v[n-1:]
	}
	if n <= 8 {
		return v[:2] + "****" + v[n-2:]
	}
	return v[:4] + "****" + v[n-4:]
}

func isStoredSecretInput(input, stored string) bool {
	input = strings.TrimSpace(input)
	stored = strings.TrimSpace(stored)
	if input == "" || stored == "" {
		return false
	}
	if input == stored || input == maskedSecret {
		return true
	}
	return input == maskSecret(stored)
}

func embyTestHTTPError(status int) *domain.AppError {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return domain.Errorf(domain.CodeDriverError, "Emby API Key 不正确")
	case http.StatusNotFound:
		return domain.Errorf(domain.CodeDriverError, "Emby 地址不正确，请检查服务地址")
	default:
		if status >= 500 {
			return domain.Errorf(domain.CodeDriverError, "Emby 服务异常，请稍后重试")
		}
		return domain.Errorf(domain.CodeDriverError, "Emby 地址无法访问")
	}
}

func embyTestConnectError(err error) *domain.AppError {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return domain.Errorf(domain.CodeDriverError, "Emby 地址连接超时，请检查网络与服务是否在线")
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection refused"):
		return domain.Errorf(domain.CodeDriverError, "Emby 地址无法连接，请检查地址和端口是否正确")
	case strings.Contains(msg, "no such host"), strings.Contains(msg, "cannot resolve"), strings.Contains(msg, "lookup"):
		return domain.Errorf(domain.CodeDriverError, "Emby 地址无法解析，请检查主机名或 IP")
	case strings.Contains(msg, "context deadline exceeded"), strings.Contains(msg, "timeout"):
		return domain.Errorf(domain.CodeDriverError, "Emby 地址连接超时，请检查网络与服务是否在线")
	default:
		return domain.Errorf(domain.CodeDriverError, "Emby 地址无法连接，请检查地址是否正确")
	}
}

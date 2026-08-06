package embyproxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"litepan/internal/playback"
	"litepan/internal/settings"
	"litepan/internal/store"
	"litepan/internal/strm"
)

func testEmbyProxyService(t *testing.T, embyURL string) *Service {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, store.Options{Memory: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	repos := store.New(db)
	settingsSvc, err := settings.New(ctx, repos.Configs)
	if err != nil {
		t.Fatal(err)
	}
	if err := settingsSvc.Update(ctx, map[string]string{
		settings.KeyEmbyEnabled:   "true",
		settings.KeyEmbyURL:       embyURL,
		settings.KeyEmbyAPIKey:    "test-key",
		settings.KeyEmbyProxyPort: "8097",
	}); err != nil {
		t.Fatal(err)
	}
	return New(Options{Settings: settingsSvc})
}

func TestRedirectSTRMStreamResolvesLitePanSTRMOnServer(t *testing.T) {
	fileID := "file-1"
	litepanURL := fmt.Sprintf("http://192.168.60.8:5211/api/strm/play/12/%s/t/token/n/demo.mkv", strm.EncodeFileKey(fileID))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/Items") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"Items":[{"Id":"123","MediaSources":[{"Id":"ms1","Path":%q}]}]}`, litepanURL))
	}))
	defer upstream.Close()

	svc := testEmbyProxyService(t, upstream.URL)
	var gotAccountID int64
	var gotFileID string
	var gotUA string
	svc.servePlayback = func(w http.ResponseWriter, r *http.Request, req playback.Request, intent playback.Intent) error {
		gotAccountID = req.AccountID
		gotFileID = req.FileID
		gotUA = r.UserAgent()
		w.Header().Set("Location", "https://cdn.example/video.mkv")
		w.WriteHeader(http.StatusFound)
		return nil
	}

	req := httptest.NewRequest(http.MethodGet, "http://litepan.test:8097/Videos/123/stream?MediaSourceId=ms1&api_key=test-key", nil)
	req.Header.Set("User-Agent", "MediaClient/8.5")
	rec := httptest.NewRecorder()
	svc.handle(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("状态码=%d，响应=%s", resp.StatusCode, rec.Body.String())
	}
	if got := resp.Header.Get("Location"); got != "https://cdn.example/video.mkv" {
		t.Fatalf("Location=%q", got)
	}
	if gotAccountID != 12 || gotFileID != fileID {
		t.Fatalf("播放请求解析错误：account=%d file=%q", gotAccountID, gotFileID)
	}
	if gotUA != "MediaClient/8.5" {
		t.Fatalf("播放请求 UA=%q，期望透传 MediaClient/8.5", gotUA)
	}
}

func TestRedirectSTRMStreamUsesPlaybackResponseForProxyMode(t *testing.T) {
	litepanURL := fmt.Sprintf("http://192.168.60.8:5211/api/strm/play/9/%s/t/token/n/demo.mkv", strm.EncodeFileKey("file-9"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/Items") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"Items":[{"Id":"123","MediaSources":[{"Id":"ms1","Path":%q}]}]}`, litepanURL))
	}))
	defer upstream.Close()

	svc := testEmbyProxyService(t, upstream.URL)
	svc.servePlayback = func(w http.ResponseWriter, r *http.Request, req playback.Request, intent playback.Intent) error {
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "proxied")
		return nil
	}

	req := httptest.NewRequest(http.MethodGet, "http://litepan.test:8097/Videos/123/stream?MediaSourceId=ms1&api_key=test-key", nil)
	rec := httptest.NewRecorder()
	svc.handle(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码=%d，响应=%s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Location"); got != "" {
		t.Fatalf("Location=%q，期望为空", got)
	}
	if got := string(body); got != "proxied" {
		t.Fatalf("响应体=%q", got)
	}
}

func TestRedirectSTRMStreamAcceptsLitePanURLWithSpaces(t *testing.T) {
	fileID := "file-with-spaces"
	fileName := "10间敢死队 (2026) [2160p].mkv"
	litepanURL := fmt.Sprintf("http://192.168.60.8:5211/api/strm/play/12/%s/t/token/n/%s", strm.EncodeFileKey(fileID), url.PathEscape(fileName))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/Items") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"Items":[{"Id":"123","MediaSources":[{"Id":"ms1","Path":%q}]}]}`, litepanURL))
	}))
	defer upstream.Close()

	svc := testEmbyProxyService(t, upstream.URL)
	var gotReq playback.Request
	svc.servePlayback = func(w http.ResponseWriter, r *http.Request, req playback.Request, intent playback.Intent) error {
		gotReq = req
		w.WriteHeader(http.StatusFound)
		return nil
	}

	req := httptest.NewRequest(http.MethodGet, "http://litepan.test:8097/Videos/123/stream?MediaSourceId=ms1&api_key=test-key", nil)
	rec := httptest.NewRecorder()
	svc.handle(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("状态码=%d，响应=%s", resp.StatusCode, rec.Body.String())
	}
	if gotReq.AccountID != 12 || gotReq.FileID != fileID {
		t.Fatalf("带空格文件名的 LitePan URL 解析错误：account=%d file=%q", gotReq.AccountID, gotReq.FileID)
	}
}

func TestParseLitePanSTRMURLFilenameRegressionCases(t *testing.T) {
	cases := []struct {
		name     string
		fileName string
	}{
		{name: "中文空格括号", fileName: "10间敢死队 (2026) [2160p].mkv"},
		{name: "英文加号百分号", fileName: "Movie.Name.2024.2160p.HDR10+ 100%.mkv"},
		{name: "波浪线与符号", fileName: "A&B ~ Director's Cut, Final!.mp4"},
		{name: "全角符号混排", fileName: "全角～波浪＋中文＆英文【特别版】.mkv"},
		{name: "井号分号等号", fileName: "Episode 01; part=2 #remux!.mkv"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fileID := fmt.Sprintf("file-regression-%d", i)
			playURL := fmt.Sprintf(
				"http://127.0.0.1:5211/api/strm/play/12/%s/t/token/n/%s",
				strm.EncodeFileKey(fileID),
				url.PathEscape(tc.fileName),
			)
			accountID, gotFileID, ok := parseLitePanSTRMURL(playURL)
			if !ok {
				t.Fatalf("parseLitePanSTRMURL 返回 false，url=%q", playURL)
			}
			if accountID != 12 || gotFileID != fileID {
				t.Fatalf("解析结果错误：account=%d file=%q", accountID, gotFileID)
			}
			if gotPath := litepanPath(playURL); strings.Contains(gotPath, " ") {
				t.Fatalf("编码路径不应出现空格：%q", gotPath)
			}
		})
	}
}

package strmscrape

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"litepan/internal/domain"
	"litepan/internal/settings"
	"litepan/internal/store"
	"litepan/internal/strm"
)

func TestCustomRootTaskID(t *testing.T) {
	a := CustomRootTaskID(filepath.Join("lib", "movies"))
	b := CustomRootTaskID(filepath.Join("lib", "movies"))
	if a != b {
		t.Fatalf("同一路径应得到同一键：%d != %d", a, b)
	}
	if a >= 0 {
		t.Fatalf("自定义目录键应为负数，got %d", a)
	}
	if c := CustomRootTaskID(filepath.Join("lib", "tv")); c == a {
		t.Fatal("不同路径的键不应相同")
	}
}

func TestResolveScrapeRootCustomDir(t *testing.T) {
	root := t.TempDir()
	svc := &Service{}
	sr, err := svc.resolveScrapeRoot(context.Background(), 0, root)
	if err != nil {
		t.Fatalf("resolveScrapeRoot: %v", err)
	}
	if sr.task != nil {
		t.Fatalf("自定义目录 task 应为 nil，got %+v", sr.task)
	}
	if sr.root == "" || sr.indexKey >= 0 {
		t.Fatalf("root=%q indexKey=%d", sr.root, sr.indexKey)
	}
	if sr.indexKey != CustomRootTaskID(root) {
		t.Fatalf("索引键与 CustomRootTaskID 不一致：%d != %d", sr.indexKey, CustomRootTaskID(root))
	}
}

func TestResolveScrapeRootCustomDirMissing(t *testing.T) {
	svc := &Service{}
	if _, err := svc.resolveScrapeRoot(context.Background(), 0, filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("不存在的自定义目录应报错")
	}
}

func TestResolveScrapeRootFallsBackToTask(t *testing.T) {
	strmRoot := t.TempDir()
	outputFolder := "电影"
	root := strm.TaskOutputDir(strmRoot, outputFolder)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	task := &domain.StrmTask{ID: 5, OutputFolder: outputFolder}
	strmSvc := strm.NewService(strm.ServiceOptions{Repo: &rematchTaskRepo{task: task}, StrmDir: strmRoot})
	svc := New(Options{Strm: strmSvc, StrmDir: strmRoot, DataDir: t.TempDir()})

	sr, err := svc.resolveScrapeRoot(context.Background(), 5, "")
	if err != nil {
		t.Fatalf("resolveScrapeRoot task mode: %v", err)
	}
	if sr.task == nil || sr.task.ID != 5 {
		t.Fatalf("任务模式应保留 task，got %+v", sr.task)
	}
	if sr.indexKey != 5 {
		t.Fatalf("任务模式索引键应为任务 ID，got %d", sr.indexKey)
	}
	if sr.root == "" {
		t.Fatal("任务模式 root 不应为空")
	}
}

func TestPosterURLFromRel(t *testing.T) {
	got := posterURLFromRel(filepath.Join("库", "电影"), "folder/poster.jpg")
	if !strings.Contains(got, "root=") {
		t.Fatalf("poster url 应内嵌 root：%s", got)
	}
	if !strings.Contains(got, "rel=folder%2Fposter.jpg") {
		t.Fatalf("rel 应转义：%s", got)
	}
	if posterURLFromRel("", "a.jpg") != "" {
		t.Fatal("空 root 时海报 URL 应为空")
	}
}

func TestResolvePosterFileCustomRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "folder", "poster.jpg"), []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &Service{}
	path, err := svc.ResolvePosterFile(context.Background(), 0, root, "folder/poster.jpg")
	if err != nil {
		t.Fatalf("ResolvePosterFile: %v", err)
	}
	if filepath.Base(path) != "poster.jpg" {
		t.Fatalf("path=%s", path)
	}
	if _, err := svc.ResolvePosterFile(context.Background(), 0, root, "../../etc/passwd"); err == nil {
		t.Fatal("越界路径应被拒绝")
	}
}

// newFakeMetaTubeServer 模拟 MetaTube REST v1 的关键端点，返回服务器与图片直链。
func newFakeMetaTubeServer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	img := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("img"))
	}
	mux.HandleFunc("/v1/images/primary/", img)
	mux.HandleFunc("/v1/images/backdrop/", img)
	mux.HandleFunc("/v1/movies/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		thumb := srv.URL + "/x.jpg"
		_, _ = w.Write([]byte(`{"data":[{"id":"ssis00123","number":"SSIS-123","provider":"JAV321","title":"無自覚なフリして","release_date":"2021-07-19T00:00:00Z","thumb_url":"` + thumb + `"}]}`))
	})
	mux.HandleFunc("/v1/movies/JAV321/ssis00123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"ssis00123","number":"SSIS-123","provider":"JAV321","title":"無自覚なフリして誘惑","summary":"这是一个简介","release_date":"2021-07-19T00:00:00Z","maker":"S1 NO.1","runtime":148,"genres":["美少女"],"actors":["乙白さやか"]}}`))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newMetaTubeSettingsService 装配指向 fake MetaTube 服务的设置。
func newMetaTubeSettingsService(t *testing.T, srvURL string) *settings.Service {
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
	settingsSvc, err := settings.New(ctx, store.New(db).Configs)
	if err != nil {
		t.Fatal(err)
	}
	if err := settingsSvc.Update(ctx, map[string]string{
		settings.KeyStrmScrapeSource:      SourceMetaTube,
		settings.KeyStrmScrapeMetaTubeURL: srvURL,
	}); err != nil {
		t.Fatal(err)
	}
	return settingsSvc
}

// TestRunAndListOnCustomRoot 端到端：对自定义目录执行刮削（无需 STRM 任务），
// 验证 nfo/海报落地、索引按负数键写入、列表与海报 URL 正常。
func TestRunAndListOnCustomRoot(t *testing.T) {
	srv := newFakeMetaTubeServer(t)
	settingsSvc := newMetaTubeSettingsService(t, srv.URL)

	root := t.TempDir()
	movie := filepath.Join(root, "SSIS-123")
	if err := os.MkdirAll(movie, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(movie, "SSIS-123.mkv.strm"), []byte("http://x/1.mkv"), 0o644); err != nil {
		t.Fatal(err)
	}

	dataDir := t.TempDir()
	svc := New(Options{Settings: settingsSvc, DataDir: dataDir})

	ctx := context.Background()
	if err := svc.run(ctx, RunRequest{StrmTaskID: 0, Root: root, WriteMode: WriteModeOverwrite}); err != nil {
		t.Fatalf("run on custom root: %v", err)
	}

	// 元数据写入自定义目录
	if _, err := os.Stat(filepath.Join(movie, "SSIS-123.mkv.nfo")); err != nil {
		t.Fatalf("nfo 未写入: %v", err)
	}
	if _, err := os.Stat(filepath.Join(movie, "poster.jpg")); err != nil {
		t.Fatalf("poster.jpg 未写入: %v", err)
	}

	// 索引按负数键落地
	key := CustomRootTaskID(root)
	if _, err := os.Stat(TaskIndexPath(dataDir, key)); err != nil {
		t.Fatalf("自定义索引未写入 %d.sqlite: %v", key, err)
	}

	// 列表与海报 URL
	result, err := svc.RefreshIndex(ctx, 0, root, ItemListQuery{Limit: defaultItemListLimit, Sort: ItemListSortAddedDesc})
	if err != nil {
		t.Fatalf("RefreshIndex: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("列表项数 = %d/%d, want 1", len(result.Items), result.Total)
	}
	it := result.Items[0]
	if !strings.HasPrefix(it.Title, "SSIS-123") || it.MediaType != MediaTypeMovie || it.Status != ItemStatusOK {
		t.Fatalf("item=%+v", it)
	}
	if it.Year == nil || *it.Year != 2021 {
		t.Fatalf("year=%v, want 2021", it.Year)
	}
	if !strings.Contains(it.PosterURL, "root=") {
		t.Fatalf("海报 URL 应内嵌 root，got %s", it.PosterURL)
	}

	// 海报文件可通过 root 解析
	if _, err := svc.ResolvePosterFile(ctx, 0, root, filepath.ToSlash(filepath.Base(it.RelDir))+"/poster.jpg"); err != nil {
		t.Fatalf("ResolvePosterFile: %v", err)
	}
}

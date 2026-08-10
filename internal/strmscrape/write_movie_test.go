package strmscrape

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stubMovieArtwork struct {
	data      []byte
	err       error
	backdrops []string
	fetchErr  error
	track     *[]string // 非空时记录每次 DownloadImage 的 imagePath
}

func (d stubMovieArtwork) DownloadImage(_ context.Context, imagePath, _ string) ([]byte, error) {
	if d.track != nil {
		*d.track = append(*d.track, imagePath)
	}
	return d.data, d.err
}

func (d stubMovieArtwork) FetchMovieBackdrops(context.Context, string) ([]string, error) {
	return d.backdrops, d.fetchErr
}

func testService() *Service {
	return &Service{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestMovieBackdropPath(t *testing.T) {
	flat := filepath.Join("lib", "Some.Movie.2020.mkv.strm")
	if got := movieBackdropPath(workGroup{flatFile: flat}); got != filepath.Join("lib", "Some.Movie.2020.mkv-fanart.jpg") {
		t.Fatalf("flatFile backdrop path = %s", got)
	}
	dir := filepath.Join("lib", "Some.Movie.2020")
	if got := movieBackdropPath(workGroup{absDir: dir}); got != filepath.Join(dir, "backdrop.jpg") {
		t.Fatalf("dir backdrop path = %s", got)
	}
}

func TestMovieExtrafanartDir(t *testing.T) {
	if got := movieExtrafanartDir(workGroup{flatFile: filepath.Join("lib", "a.strm")}); got != "" {
		t.Fatalf("flatFile extrafanart dir = %q, want empty", got)
	}
	dir := filepath.Join("lib", "Some.Movie.2020")
	if got := movieExtrafanartDir(workGroup{absDir: dir}); got != filepath.Join(dir, "extrafanart") {
		t.Fatalf("dir extrafanart = %s", got)
	}
}

func TestWriteMovieExtrasWritesBackdropAndExtrafanart(t *testing.T) {
	root := t.TempDir()
	g := workGroup{absDir: root}
	info := tmdbInfo{TMDBID: "123", BackdropPath: "/backdrop.jpg"}
	// 第一张与主背景重复，应被过滤
	src := stubMovieArtwork{data: []byte("img"), backdrops: []string{"/backdrop.jpg", "/c.jpg", "/d.jpg"}}

	if err := testService().writeMovieExtras(context.Background(), src, g, info, false); err != nil {
		t.Fatalf("writeMovieExtras: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "backdrop.jpg")); err != nil {
		t.Fatalf("backdrop.jpg 未写入: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "extrafanart", "fanart1.jpg")); err != nil {
		t.Fatalf("fanart1.jpg 未写入: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "extrafanart", "fanart2.jpg")); err != nil {
		t.Fatalf("fanart2.jpg 未写入: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "extrafanart", "fanart3.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fanart3.jpg 不应存在, err=%v", err)
	}
}

func TestWriteMovieExtrasSkipsWhenBackdropEmpty(t *testing.T) {
	root := t.TempDir()
	g := workGroup{absDir: root}
	src := stubMovieArtwork{data: []byte("img"), backdrops: []string{"/c.jpg"}}

	if err := testService().writeMovieExtras(context.Background(), src, g, tmdbInfo{TMDBID: "1"}, false); err != nil {
		t.Fatalf("writeMovieExtras: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "backdrop.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("BackdropPath 为空时不应写主背景, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "extrafanart", "fanart1.jpg")); err != nil {
		t.Fatalf("主背景缺失不应阻断轮播背景: %v", err)
	}
}

func TestWriteMovieExtrasDegradesDownloadFailure(t *testing.T) {
	var logs bytes.Buffer
	svc := &Service{log: slog.New(slog.NewTextHandler(&logs, nil))}
	root := t.TempDir()
	g := workGroup{absDir: root}
	src := stubMovieArtwork{err: errors.New("图片 404"), backdrops: []string{"/c.jpg"}}

	err := svc.writeMovieExtras(context.Background(), src, g, tmdbInfo{TMDBID: "1", BackdropPath: "/b.jpg"}, false)
	if err != nil {
		t.Fatalf("下载失败不应中断, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "backdrop.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("下载失败后不应生成主背景, err=%v", err)
	}
	if text := logs.String(); !strings.Contains(text, "下载失败") {
		t.Fatalf("未记录可选图片警告: %s", text)
	}
}

func TestWriteMovieExtrasPreservesWriteFailure(t *testing.T) {
	root := t.TempDir()
	block := filepath.Join(root, "block")
	if err := os.WriteFile(block, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := workGroup{absDir: block}
	src := stubMovieArtwork{data: []byte("img")}

	err := testService().writeMovieExtras(context.Background(), src, g, tmdbInfo{TMDBID: "1", BackdropPath: "/b.jpg"}, false)
	if err == nil || !strings.Contains(err.Error(), "写入电影背景") {
		t.Fatalf("本地写入失败必须保留, err=%v", err)
	}
}

func TestWriteMovieExtrasFetchBackdropsErrorStillWritesBackdrop(t *testing.T) {
	root := t.TempDir()
	g := workGroup{absDir: root}
	src := stubMovieArtwork{data: []byte("img"), fetchErr: errors.New("boom")}

	err := testService().writeMovieExtras(context.Background(), src, g, tmdbInfo{TMDBID: "1", BackdropPath: "/b.jpg"}, false)
	if err != nil {
		t.Fatalf("列表获取失败不应中断, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "backdrop.jpg")); err != nil {
		t.Fatalf("列表失败不应阻断主背景: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "extrafanart")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("列表失败后不应有 extrafanart, err=%v", err)
	}
}

func TestWriteMovieExtrasSkipsExistingDirUnlessOverwrite(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "extrafanart")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "fanart1.jpg")
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := workGroup{absDir: root}

	// 非覆盖：目录已存在则保留旧图
	src := stubMovieArtwork{data: []byte("img"), backdrops: []string{"/c.jpg"}}
	if err := testService().writeMovieExtras(context.Background(), src, g, tmdbInfo{TMDBID: "1", BackdropPath: "/b.jpg"}, false); err != nil {
		t.Fatalf("writeMovieExtras: %v", err)
	}
	got, _ := os.ReadFile(old)
	if string(got) != "old" {
		t.Fatalf("非覆盖模式不应重写 fanart1, got=%q", got)
	}

	// 覆盖：清空重建
	if err := testService().writeMovieExtras(context.Background(), src, g, tmdbInfo{TMDBID: "1", BackdropPath: "/b.jpg"}, true); err != nil {
		t.Fatalf("overwrite writeMovieExtras: %v", err)
	}
	got, _ = os.ReadFile(old)
	if string(got) != "img" {
		t.Fatalf("覆盖模式应重写 fanart1, got=%q", got)
	}
}

func TestWriteMovieExtrasFlatFileOnlyBackdrop(t *testing.T) {
	root := t.TempDir()
	flat := filepath.Join(root, "Some.Movie.2020.mkv.strm")
	g := workGroup{flatFile: flat, absDir: root}
	src := stubMovieArtwork{data: []byte("img"), backdrops: []string{"/c.jpg"}}

	if err := testService().writeMovieExtras(context.Background(), src, g, tmdbInfo{TMDBID: "1", BackdropPath: "/b.jpg"}, false); err != nil {
		t.Fatalf("writeMovieExtras: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Some.Movie.2020.mkv-fanart.jpg")); err != nil {
		t.Fatalf("flatFile 背景图未写入: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "extrafanart")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("flatFile 不应建 extrafanart 目录, err=%v", err)
	}
}

func TestWriteMovieExtrasCapsAtFive(t *testing.T) {
	root := t.TempDir()
	g := workGroup{absDir: root}
	var backdrops []string
	for i := 1; i <= 10; i++ {
		backdrops = append(backdrops, fmt.Sprintf("/b%d.jpg", i))
	}
	src := stubMovieArtwork{data: []byte("img"), backdrops: backdrops}

	if err := testService().writeMovieExtras(context.Background(), src, g, tmdbInfo{TMDBID: "1"}, false); err != nil {
		t.Fatalf("writeMovieExtras: %v", err)
	}
	for i := 1; i <= 5; i++ {
		if _, err := os.Stat(filepath.Join(root, "extrafanart", fmt.Sprintf("fanart%d.jpg", i))); err != nil {
			t.Fatalf("fanart%d.jpg 应写入: %v", i, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "extrafanart", "fanart6.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fanart6.jpg 不应写入（上限 5 张）, err=%v", err)
	}
}

func TestWriteMovieExtrasMetaTubeBackdropAndPreviewImages(t *testing.T) {
	root := t.TempDir()
	g := workGroup{absDir: root}
	info := tmdbInfo{
		TMDBID:        "SSIS-123",
		BackdropPath:  "backdrop/JAV321/ssis00123",
		PreviewImages: []string{"http://pics/1.jpg", "http://pics/2.jpg", "http://pics/3.jpg"},
	}
	var downloaded []string
	src := stubMovieArtwork{data: []byte("img"), track: &downloaded}

	if err := testService().writeMovieExtras(context.Background(), src, g, info, false); err != nil {
		t.Fatalf("writeMovieExtras: %v", err)
	}
	// 主背景走哨兵 backdrop 路径
	if len(downloaded) == 0 || downloaded[0] != "backdrop/JAV321/ssis00123" {
		t.Fatalf("主背景未走 backdrop 哨兵路径, downloaded=%v", downloaded)
	}
	if _, err := os.Stat(filepath.Join(root, "backdrop.jpg")); err != nil {
		t.Fatalf("backdrop.jpg 未写入: %v", err)
	}
	// 轮播用详情 PreviewImages（stub 无 backdrops 也不影响），无需再拉列表
	for i := 1; i <= 3; i++ {
		if _, err := os.Stat(filepath.Join(root, "extrafanart", fmt.Sprintf("fanart%d.jpg", i))); err != nil {
			t.Fatalf("fanart%d.jpg 未写入: %v", i, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "extrafanart", "fanart4.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fanart4.jpg 不应存在, err=%v", err)
	}
}

func TestWriteMovieExtrasMetaTubeStyleEmptySource(t *testing.T) {
	root := t.TempDir()
	g := workGroup{absDir: root}
	// MetaTube 风格：无 backdrop_path、无 backdrops 列表
	src := stubMovieArtwork{data: []byte("img")}

	if err := testService().writeMovieExtras(context.Background(), src, g, tmdbInfo{TMDBID: "1"}, false); err != nil {
		t.Fatalf("无背景图数据不应报错, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "backdrop.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("不应写主背景, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "extrafanart")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("不应建 extrafanart, err=%v", err)
	}
}

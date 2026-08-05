package favorites

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPutGetRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := newTestFavoritesService(dir)
	ctx := context.Background()
	state := State{
		Open: true,
		Items: []Item{{
			ID:        "folder",
			Name:      "收藏目录",
			AccountID: 11,
			Crumbs:    []Crumb{{ID: "root", Name: "根目录"}},
		}},
	}
	saved, err := svc.Put(ctx, state)
	if err != nil {
		t.Fatalf("保存收藏失败: %v", err)
	}
	if !saved.Open || len(saved.Items) != 1 || saved.Items[0].AccountID != 11 {
		t.Fatalf("保存结果异常: %#v", saved)
	}

	got, err := svc.Get(ctx)
	if err != nil {
		t.Fatalf("读取收藏失败: %v", err)
	}
	if !got.Open || len(got.Items) != 1 || got.Items[0].Name != "收藏目录" {
		t.Fatalf("读取结果异常: %#v", got)
	}
}

func TestDeleteRemovesTargetAccount(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := newTestFavoritesService(dir)
	ctx := context.Background()
	mk := func(id string, accountID int64) Item {
		return Item{ID: id, Name: id + "收藏", AccountID: accountID,
			Crumbs: []Crumb{{ID: "root", Name: "根目录"}}}
	}
	if _, err := svc.Put(ctx, State{Open: true, Items: []Item{mk("a", 11), mk("b", 22)}}); err != nil {
		t.Fatalf("保存收藏失败: %v", err)
	}

	if err := svc.Delete(ctx, 11); err != nil {
		t.Fatalf("删除账号收藏失败: %v", err)
	}
	got, err := svc.Get(ctx)
	if err != nil {
		t.Fatalf("读取收藏失败: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].AccountID != 22 {
		t.Fatalf("删除账号 11 后应只剩账号 22 的收藏: %#v", got.Items)
	}
}

func TestDeleteMissingAccountDoesNotRewriteFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := newTestFavoritesService(dir)
	ctx := context.Background()
	if _, err := svc.Put(ctx, State{Open: true, Items: []Item{{
		ID: "a", Name: "收藏", AccountID: 11,
		Crumbs: []Crumb{{ID: "root", Name: "根目录"}},
	}}}); err != nil {
		t.Fatalf("保存收藏失败: %v", err)
	}
	path := filepath.Join(dir, fileName)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取收藏文件信息失败: %v", err)
	}

	if err := svc.Delete(ctx, 999); err != nil {
		t.Fatalf("删除无收藏账号应幂等成功: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("再次读取收藏文件信息失败: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Fatalf("不存在的账号不应触发收藏文件重写")
	}
}

func TestGetMovesCorruptedFavoritesFileAndReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := newTestFavoritesService(dir)
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("写入损坏收藏夹文件失败: %v", err)
	}

	_, err := svc.Get(context.Background())
	if err == nil {
		t.Fatalf("期望返回损坏错误")
	}
	if !strings.Contains(err.Error(), "已损坏") {
		t.Fatalf("错误信息未指出文件损坏: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, fileName)); !os.IsNotExist(statErr) {
		t.Fatalf("损坏原文件应被转移，实际 stat err=%v", statErr)
	}
	matches, globErr := filepath.Glob(filepath.Join(dir, fileName+".corrupt-*"))
	if globErr != nil {
		t.Fatalf("查找损坏备份失败: %v", globErr)
	}
	if len(matches) != 1 {
		t.Fatalf("期望生成 1 个损坏备份，实际 %d 个: %#v", len(matches), matches)
	}
	raw, readErr := os.ReadFile(matches[0])
	if readErr != nil {
		t.Fatalf("读取损坏备份失败: %v", readErr)
	}
	if string(raw) != "{bad json" {
		t.Fatalf("损坏备份内容不匹配: %q", string(raw))
	}
}

func TestPutDoesNotSilentlyOverwriteCorruptedFavoritesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := newTestFavoritesService(dir)
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("写入损坏收藏夹文件失败: %v", err)
	}

	_, err := svc.Put(context.Background(), State{
		Open: true,
		Items: []Item{{
			ID: "1", Name: "电影", AccountID: 11,
			Crumbs: []Crumb{{ID: "root", Name: "根目录"}},
		}},
	})
	if err == nil {
		t.Fatalf("期望保存时返回损坏错误")
	}
	if _, statErr := os.Stat(filepath.Join(dir, fileName)); !os.IsNotExist(statErr) {
		t.Fatalf("损坏原文件应已被转移且当前不应生成新文件，stat err=%v", statErr)
	}
	matches, globErr := filepath.Glob(filepath.Join(dir, fileName+".corrupt-*"))
	if globErr != nil {
		t.Fatalf("查找损坏备份失败: %v", globErr)
	}
	if len(matches) != 1 {
		t.Fatalf("期望保留 1 份损坏备份，实际 %d 个: %#v", len(matches), matches)
	}
}

// 旧版按账号收藏格式迁移：合并为全局收藏，每条带上所属账号。
func TestMigrateLegacyAccountFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	legacy := map[string]accountState{
		"11": {Open: true, Items: []Item{{ID: "a", Name: "A盘收藏", Crumbs: []Crumb{{ID: "root", Name: "根目录"}}}}},
		"22": {Open: false, Items: []Item{{ID: "b", Name: "B盘收藏", Crumbs: []Crumb{{ID: "root", Name: "根目录"}}}}},
	}
	raw, _ := json.Marshal(map[string]any{"version": 1, "accounts": legacy})
	if err := os.WriteFile(filepath.Join(dir, fileName), raw, 0o644); err != nil {
		t.Fatalf("写入旧版收藏文件失败: %v", err)
	}

	svc := newTestFavoritesService(dir)
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("读取迁移后收藏失败: %v", err)
	}
	if !got.Open {
		t.Fatalf("迁移后 open 应为 true")
	}
	if len(got.Items) != 2 {
		t.Fatalf("迁移后应保留 2 条收藏，实际 %d: %#v", len(got.Items), got.Items)
	}
	byID := map[string]Item{}
	for _, it := range got.Items {
		byID[it.ID] = it
	}
	if byID["a"].AccountID != 11 || byID["b"].AccountID != 22 {
		t.Fatalf("迁移后收藏账号归属错误: %#v", got.Items)
	}

	// 落盘应为新版全局格式
	body, _ := os.ReadFile(filepath.Join(dir, fileName))
	var rawData rawSnapshot
	if err := json.Unmarshal(body, &rawData); err != nil {
		t.Fatalf("迁移后文件无法解析: %v", err)
	}
	if rawData.Version != 2 || len(rawData.Accounts) != 0 || len(rawData.Items) != 2 {
		t.Fatalf("迁移后文件格式异常: %#v", rawData)
	}
}

func newTestFavoritesService(dir string) *Service {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(filepath.Join(dir, "litepan.db"), logger)
}

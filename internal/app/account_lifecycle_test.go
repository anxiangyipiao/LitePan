package app

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"litepan/internal/favorites"
)

func TestAccountLifecycleDeleteRemovesFavorites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	favoritesSvc := favorites.NewService(filepath.Join(dir, "litepan.db"), logger)
	ctx := context.Background()
	mk := func(id string, accountID int64) favorites.Item {
		return favorites.Item{
			ID: id, Name: id + "收藏", AccountID: accountID,
			Crumbs: []favorites.Crumb{{ID: "root", Name: "根目录"}},
		}
	}
	if _, err := favoritesSvc.Put(ctx, favorites.State{Items: []favorites.Item{mk("f1", 11), mk("f2", 22)}}); err != nil {
		t.Fatalf("保存收藏失败: %v", err)
	}

	lifecycle := accountLifecycle{favorites: favoritesSvc}
	if err := lifecycle.OnAccountDeleted(ctx, 11); err != nil {
		t.Fatalf("执行账号删除生命周期失败: %v", err)
	}
	state, err := favoritesSvc.Get(ctx)
	if err != nil {
		t.Fatalf("读取收藏失败: %v", err)
	}
	if len(state.Items) != 1 || state.Items[0].AccountID != 22 {
		t.Fatalf("账号删除生命周期应只清理账号 11 的收藏: %#v", state.Items)
	}
}

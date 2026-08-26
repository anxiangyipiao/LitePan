package magnetfavorites

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testHash = "0123456789abcdef0123456789abcdef01234567"
const testMagnet = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=test"

func newItem(name string) Item {
	return Item{
		Hash:     testHash,
		Name:     name,
		Size:     1234,
		Seeders:  10,
		Leechers: 1,
		Magnet:   testMagnet,
		Category: "Books",
		ViewURL:  "https://example.com/view/1",
	}
}

func TestAddDeduplicatesByHash(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := newTestService(dir)
	ctx := context.Background()

	if _, err := svc.Add(ctx, newItem("第一次")); err != nil {
		t.Fatalf("首次添加失败: %v", err)
	}
	if _, err := svc.Add(ctx, newItem("第二次同 hash")); err != nil {
		t.Fatalf("重复添加应幂等，不应报错: %v", err)
	}

	state, err := svc.Get(ctx)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(state.Items) != 1 {
		t.Fatalf("期望去重后剩 1 条，实际 %d 条", len(state.Items))
	}
	if state.Items[0].Name != "第一次" {
		t.Fatalf("期望保留先加入的名称，实际 %q", state.Items[0].Name)
	}
}

func TestAddRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := newTestService(dir)
	ctx := context.Background()

	cases := []struct {
		name string
		item Item
	}{
		{"hash 长度不对", func() Item {
			i := newItem("x")
			i.Hash = "abc"
			return i
		}()},
		{"magnet 不合法", func() Item {
			i := newItem("x")
			i.Magnet = "https://example.com/not-a-magnet"
			return i
		}()},
		{"name 为空", func() Item {
			i := newItem("")
			return i
		}()},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if _, err := svc.Add(ctx, c.item); err == nil {
				t.Fatalf("期望 %s 失败", c.name)
			}
		})
	}
}

func TestRemoveByHash(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := newTestService(dir)
	ctx := context.Background()

	other := newItem("另一条")
	other.Hash = "ffffffffffffffffffffffffffffffffffffffff"
	other.Magnet = "magnet:?xt=urn:btih:ffffffffffffffffffffffffffffffffffffffff&dn=other"

	if _, err := svc.Add(ctx, newItem("目标")); err != nil {
		t.Fatalf("添加失败: %v", err)
	}
	if _, err := svc.Add(ctx, other); err != nil {
		t.Fatalf("添加失败: %v", err)
	}

	state, err := svc.Remove(ctx, testHash)
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if len(state.Items) != 1 || state.Items[0].Hash != other.Hash {
		t.Fatalf("删除后剩余错误: %#v", state.Items)
	}

	// 再次删除同一个 hash 应幂等
	if _, err := svc.Remove(ctx, testHash); err != nil {
		t.Fatalf("幂等删除失败: %v", err)
	}
}

func TestRemoveAcceptsUppercaseHash(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := newTestService(dir)
	ctx := context.Background()

	if _, err := svc.Add(ctx, newItem("test")); err != nil {
		t.Fatalf("添加失败: %v", err)
	}
	upper := strings.ToUpper(testHash)
	if _, err := svc.Remove(ctx, upper); err != nil {
		t.Fatalf("大写 hash 删除失败: %v", err)
	}
	state, _ := svc.Get(ctx)
	if len(state.Items) != 0 {
		t.Fatalf("大写 hash 应能匹配删除，实际剩余: %d", len(state.Items))
	}
}

func TestGetPersistsAcrossInstances(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc1 := newTestService(dir)
	ctx := context.Background()
	if _, err := svc1.Add(ctx, newItem("持久化测试")); err != nil {
		t.Fatalf("添加失败: %v", err)
	}

	svc2 := newTestService(dir)
	state, err := svc2.Get(ctx)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(state.Items) != 1 || state.Items[0].Name != "持久化测试" {
		t.Fatalf("持久化异常: %#v", state.Items)
	}
}

func TestGetMovesCorruptedFileAndReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := newTestService(dir)
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("写入损坏文件失败: %v", err)
	}

	_, err := svc.Get(context.Background())
	if err == nil {
		t.Fatalf("期望返回损坏错误")
	}
	if !strings.Contains(err.Error(), "已损坏") {
		t.Fatalf("错误信息未指出文件损坏: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, fileName+".corrupt-*"))
	if len(matches) != 1 {
		t.Fatalf("期望生成 1 个损坏备份，实际 %d 个", len(matches))
	}
}

func TestClearEmptiesAll(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := newTestService(dir)
	ctx := context.Background()
	if _, err := svc.Add(ctx, newItem("a")); err != nil {
		t.Fatalf("添加失败: %v", err)
	}
	if err := svc.Clear(ctx); err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	state, _ := svc.Get(ctx)
	if len(state.Items) != 0 {
		t.Fatalf("清空后应为空，实际: %d", len(state.Items))
	}
}

func newTestService(dir string) *Service {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(filepath.Join(dir, "litepan.db"), logger)
}

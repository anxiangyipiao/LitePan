package alias

import (
	"context"
	"testing"
	"time"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

func TestParseTargets(t *testing.T) {
	// 冒号分隔：账号名:路径
	got := parseTargets("网盘A:/电影/悬疑, 网盘B")
	if len(got) != 2 {
		t.Fatalf("targets=%d, want 2", len(got))
	}
	if got[0].accountName != "网盘A" || len(got[0].segments) != 2 || got[0].segments[0] != "电影" {
		t.Fatalf("target[0]=%+v", got[0])
	}
	if got[1].accountName != "网盘B" || len(got[1].segments) != 0 {
		t.Fatalf("target[1]=%+v", got[1])
	}

	// 斜杠分隔：账号名/路径（兼容写法）
	slash := parseTargets("联通云盘/视频")
	if len(slash) != 1 || slash[0].accountName != "联通云盘" || len(slash[0].segments) != 1 || slash[0].segments[0] != "视频" {
		t.Fatalf("斜杠写法解析错误: %+v", slash)
	}

	// 空目标
	if len(parseTargets("")) != 0 {
		t.Fatal("空目标应返回空")
	}
}

func TestParseID(t *testing.T) {
	cases := []struct {
		in          string
		kind        idKind
		accountID   int64
		realID, key string
	}{
		{"", idRoot, 0, "", ""},
		{"virt:电影", idVirtual, 0, "", "电影"},
		{"acct:123:xyz", idReal, 123, "xyz", ""},
		{"acct:0:abc", idReal, 0, "abc", ""},
		{"garbage", idInvalid, 0, "", ""},
		{"acct:12x:abc", idInvalid, 0, "", ""},
	}
	for _, c := range cases {
		kind, id, real, key := parseID(c.in)
		if kind != c.kind || id != c.accountID || real != c.realID || key != c.key {
			t.Fatalf("parseID(%q) = (%v,%d,%q,%q), want (%v,%d,%q,%q)",
				c.in, kind, id, real, key, c.kind, c.accountID, c.realID, c.key)
		}
	}
}

// ---------- 用桩驱动验证虚拟合并 ----------

// stubDriver 实现基础 Lister + InfoGetter。
type stubDriver struct {
	name string
	dirs map[string][]domain.FileItem // parentID -> children
}

func (s *stubDriver) Config() driver.Config { return driver.Config{Name: "stub", DefaultRoot: "/"} }
func (s *stubDriver) GetAddition() any      { return nil }
func (s *stubDriver) Init(context.Context) error { return nil }
func (s *stubDriver) Drop(context.Context) error { return nil }
func (s *stubDriver) Ping(context.Context) error { return nil }

func (s *stubDriver) ListFiles(_ context.Context, parentID string) ([]domain.FileItem, error) {
	return s.dirs[parentID], nil
}

func (s *stubDriver) GetFileInfo(_ context.Context, fileID string) (*domain.FileItem, error) {
	for _, items := range s.dirs {
		for _, it := range items {
			if it.ID == fileID {
				c := it
				return &c, nil
			}
		}
	}
	return nil, domain.Errorf(domain.CodeNotFound, "not found")
}

func TestListVirtualMergesByAccount(t *testing.T) {
	accA := &stubDriver{name: "A", dirs: map[string][]domain.FileItem{
		"/": {
			{ID: "folder1", Name: "电影", IsDir: true, ModTime: time.Now()},
		},
		"folder1": {
			{ID: "m1", Name: "同一部.mkv", Size: 10},
			{ID: "m2", Name: "A独有.mkv", Size: 20},
		},
	}}
	accB := &stubDriver{name: "B", dirs: map[string][]domain.FileItem{
		"/": {
			{ID: "folder1", Name: "电影", IsDir: true, ModTime: time.Now()},
		},
		"folder1": {
			{ID: "m1", Name: "同一部.mkv", Size: 99}, // 同名去重，A 先到
			{ID: "m3", Name: "B独有.mkv", Size: 30},
		},
	}}

	d := &Driver{
		order: []string{"电影"},
		byKey: map[string][]aliasTarget{
			"电影": {
				{accountName: "A", segments: []string{"电影"}},
				{accountName: "B", segments: []string{"电影"}},
			},
		},
		resolver: driver.AccountRefResolver{
			ByName: func(_ context.Context, name string) (int64, driver.Driver, error) {
				if name == "A" {
					return 1, accA, nil
				}
				return 2, accB, nil
			},
			ByID: func(_ context.Context, id int64) (driver.Driver, error) {
				if id == 1 {
					return accA, nil
				}
				return accB, nil
			},
		},
	}

	items, err := d.ListFiles(context.Background(), "virt:电影")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("合并后应 3 项（去重 1 项），实际 %d: %+v", len(items), items)
	}
	byName := map[string]domain.FileItem{}
	for _, it := range items {
		byName[it.Name] = it
	}
	if byName["同一部.mkv"].Size != 10 {
		t.Fatalf("同名去重应保留账号 A 的条目，got %+v", byName["同一部.mkv"])
	}
	if byName["B独有.mkv"].ID != "acct:2:m3" {
		t.Fatalf("B 独有条目 ID 应带账号前缀，got %q", byName["B独有.mkv"].ID)
	}
}

func TestResolvePathWalksSegments(t *testing.T) {
	acc := &stubDriver{name: "A", dirs: map[string][]domain.FileItem{
		"/": {
			{ID: "d1", Name: "电影", IsDir: true},
		},
		"d1": {
			{ID: "d2", Name: "悬疑", IsDir: true},
		},
	}}
	id, err := resolvePath(context.Background(), acc, []string{"电影", "悬疑"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "d2" {
		t.Fatalf("resolvePath 应返回 d2，got %q", id)
	}
}

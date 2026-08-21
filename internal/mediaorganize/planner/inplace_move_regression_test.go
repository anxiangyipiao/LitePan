package planner_test

import (
	"context"
	"testing"

	"litepan/internal/domain"
	"litepan/internal/mediaorganize/moplan"
	"litepan/internal/mediaorganize/planner"
)

// TestMovePlanSkipsInDirMoviesInPlace 复现用户报告：原地整理（move，目标根=扫描目录）时，
// 已经位于作品目录内的电影文件（SIVR-498.CD1/CD2、SONE-817-C、STARS-182-C、abc-123）
// 不应再被挪进一个同名新目录，而应全部跳过。
func TestMovePlanSkipsInDirMoviesInPlace(t *testing.T) {
	fs := &mockFS{dirs: map[string][]domain.FileItem{
		"root": {
			{ID: "d1", Name: "SIVR-498.CD1", IsDir: true},
			{ID: "d2", Name: "SIVR-498.CD2", IsDir: true},
			{ID: "d3", Name: "SONE-817-C", IsDir: true},
			{ID: "d4", Name: "STARS-182-C", IsDir: true},
			{ID: "d5", Name: "abc-123", IsDir: true},
		},
		"d1": {{ID: "f1", Name: "SIVR-498.CD1 CD1.mp4"}},
		"d2": {{ID: "f2", Name: "SIVR-498.CD2 CD2.mp4"}},
		"d3": {{ID: "f3", Name: "SONE-817-C.mp4"}},
		"d4": {{ID: "f4", Name: "STARS-182-C.mp4"}},
		"d5": {{ID: "f5", Name: "abc.mp4"}},
	}}
	p := planner.New(
		context.Background(),
		fs,
		1,
		planner.TaskConfig{
			TargetDirectoryID: "root",
			TargetRootID:      "root", // 原地整理：目标根 == 扫描目录
			ActionType:        "move",
			MediaType:         "auto",
			UseTMDB:           false,
			Recursive:         true,
		},
		planner.Settings{"mo_tmdb_api_key": "test-key"},
		"task-test",
		nil,
		func(string) {},
		nil,
		func() error { return nil },
	)
	plan, err := p.Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 0 {
		t.Fatalf("原地整理时已位于目录内的电影不应产生任何动作，实际 actions=%+v", plan.Actions)
	}
	wantSkip := map[string]bool{"f1": true, "f2": true, "f3": true, "f4": true, "f5": true}
	for _, s := range plan.Skipped {
		fid, _ := s["file_id"].(string)
		if !wantSkip[fid] {
			continue
		}
		delete(wantSkip, fid)
		if reason, _ := s["reason"].(string); reason != "已在目录中" {
			t.Fatalf("file %s 跳过原因 = %q，期望 %q", fid, reason, "已在目录中")
		}
	}
	if len(wantSkip) != 0 {
		t.Fatalf("以下文件未被跳过: %v，skipped=%+v", wantSkip, plan.Skipped)
	}
}

// TestMovePlanGroupsScatteredCDPartsIntoOneFolder 复现用户报告：散落在根目录的
// SIVR-498.CD1 / SIVR-498.CD2 分盘文件应合并进同一个目录（SIVR-498），而不是拆成两个目录。
func TestMovePlanGroupsScatteredCDPartsIntoOneFolder(t *testing.T) {
	fs := &mockFS{dirs: map[string][]domain.FileItem{
		"root": {
			{ID: "f1", Name: "SIVR-498.CD1 CD1.mp4"},
			{ID: "f2", Name: "SIVR-498.CD2 CD2.mp4"},
		},
	}}
	p := planner.New(
		context.Background(),
		fs,
		1,
		planner.TaskConfig{
			TargetDirectoryID:   "root",
			TargetRootID:        "root",
			ActionType:          "move",
			MediaType:           "auto",
			UseTMDB:             false,
			Recursive:           true,
			ScatterMoviePerFile: true,
		},
		planner.Settings{"mo_tmdb_api_key": "test-key"},
		"task-test",
		nil,
		func(string) {},
		nil,
		func() error { return nil },
	)
	plan, err := p.Build()
	if err != nil {
		t.Fatal(err)
	}
	workDirs := map[string]bool{}
	fileMoves := map[string]bool{}
	for _, a := range plan.Actions {
		switch a.Kind {
		case moplan.ActionKindEnsureDir:
			workDirs[a.TargetName] = true
		case moplan.ActionKindRelocate:
			if a.SourceID == "f1" || a.SourceID == "f2" {
				fileMoves[a.SourceID] = true
			}
		}
	}
	if len(workDirs) != 1 {
		t.Fatalf("CD1/CD2 应合并为 1 个工作目录，实际 %v (actions=%+v)", workDirs, plan.Actions)
	}
	if !workDirs["SIVR-498"] {
		t.Fatalf("工作目录应为 SIVR-498，实际 %v", workDirs)
	}
	if !fileMoves["f1"] || !fileMoves["f2"] {
		t.Fatalf("两个分盘文件都应移动到合并目录，实际 moves=%v", fileMoves)
	}
}

// TestGroupScatteredJAVCDPartsMergeIntoSingleGroup 在分组层面验证：
// 散落的 SIVR-498.CD1 / SIVR-498.CD2 文件名会归一到同一个番号组（SIVR-498）。
func TestGroupScatteredJAVCDPartsMergeIntoSingleGroup(t *testing.T) {
	p := newTestPlanner(nil, nil, "root")
	entries := []planner.BatchEntryForTest{
		{Item: domain.FileItem{ID: "f1", Name: "SIVR-498.CD1 CD1.mp4"}},
		{Item: domain.FileItem{ID: "f2", Name: "SIVR-498.CD2 CD2.mp4"}},
	}
	groups, pending := planner.GroupEntriesForTestExport(p, entries)
	if len(pending) != 0 {
		t.Fatalf("分组不应产生跳过，实际 %+v", pending)
	}
	if len(groups) != 1 {
		t.Fatalf("CD1/CD2 应合并为 1 组，实际 %d 组", len(groups))
	}
	for key, count := range groups {
		if key.Title != "SIVR-498" {
			t.Fatalf("组标题 = %q，期望 SIVR-498", key.Title)
		}
		if key.DirID != "" {
			t.Fatalf("散落文件组 dirID 应为空，实际 %q", key.DirID)
		}
		if count != 2 {
			t.Fatalf("组内文件数 = %d，期望 2", count)
		}
	}
}

// TestMovePlanRelocationStillMovesInDirJAV 回归保护：跨库搬迁（target_root 指向另一目录）时，
// 已位于作品目录内的 JAV 电影仍应按整体搬迁处理，不能被「已在目录中」规则误跳过。
func TestMovePlanRelocationStillMovesInDirJAV(t *testing.T) {
	fs := &mockFS{dirs: map[string][]domain.FileItem{
		"root": {
			{ID: "d1", Name: "SIVR-498.CD1", IsDir: true},
			{ID: "target", Name: "目标库", IsDir: true},
		},
		"d1":     {{ID: "f1", Name: "SIVR-498.CD1 CD1.mp4"}},
		"target": {},
	}}
	p := planner.New(
		context.Background(),
		fs,
		1,
		planner.TaskConfig{
			TargetDirectoryID: "root",
			TargetRootID:      "target", // 跨库搬迁：目标根 != 扫描目录
			ActionType:        "move",
			MediaType:         "auto",
			UseTMDB:           false,
			Recursive:         true,
		},
		planner.Settings{"mo_tmdb_api_key": "test-key"},
		"task-test",
		nil,
		func(string) {},
		nil,
		func() error { return nil },
	)
	plan, err := p.Build()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range plan.Skipped {
		if fid, _ := s["file_id"].(string); fid == "f1" {
			t.Fatalf("跨库搬迁时 f1 不应被跳过，reason=%v", s["reason"])
		}
	}
	if len(plan.Actions) == 0 {
		t.Fatal("跨库搬迁时已位于目录内的 JAV 电影仍应产生搬迁动作")
	}
}

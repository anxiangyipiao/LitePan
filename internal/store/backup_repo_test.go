package store_test

import (
	"context"
	"testing"
	"time"

	"litepan/internal/domain"
)

func TestBackupJobCRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	id, err := s.BackupJobs.Create(ctx, &domain.BackupJob{
		Name:              "照片备份",
		SourceAccountID:   1,
		SourceParentID:    "dir_src",
		SourceDisplayPath: "/photos",
		TargetAccountID:   3,
		TargetParentID:    "dir_1",
		TargetDisplayPath: "/backup/photos",
		Method:            "sha1",
		ScheduleMode:      domain.BackupScheduleDaily,
		Time:              "03:00",
		Enabled:           true,
		NextRunAt:         time.Now().Add(24 * time.Hour).Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	got, err := s.BackupJobs.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "照片备份" || !got.Enabled || got.Time != "03:00" || got.SourceAccountID != 1 || got.TargetAccountID != 3 {
		t.Fatalf("unexpected job: %+v", got)
	}
	if got.NextRunAt.IsZero() {
		t.Fatal("next_run_at should be populated")
	}

	got.Enabled = false
	got.Method = "md5"
	if err := s.BackupJobs.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := s.BackupJobs.Get(ctx, id)
	if got2.Enabled || got2.Method != "md5" {
		t.Fatalf("update not persisted: %+v", got2)
	}

	lastRun := time.Now().Truncate(time.Second)
	if err := s.BackupJobs.UpdateLastRun(ctx, id, domain.BackupRunSuccess, "备份完成", lastRun); err != nil {
		t.Fatalf("update last run: %v", err)
	}
	got3, _ := s.BackupJobs.Get(ctx, id)
	if got3.LastRunStatus != domain.BackupRunSuccess || !got3.LastRunAt.Equal(lastRun) {
		t.Fatalf("last run not persisted: %+v", got3)
	}

	// 停用后不再出现在 ListEnabled
	enabled, err := s.BackupJobs.ListEnabled(ctx)
	if err != nil || len(enabled) != 0 {
		t.Fatalf("disabled job should not be enabled, got %d, err=%v", len(enabled), err)
	}
	all, err := s.BackupJobs.ListAll(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("list all: %d, %v", len(all), err)
	}
}

func TestBackupRunAndStateLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	jobID, _ := s.BackupJobs.Create(ctx, &domain.BackupJob{Name: "J", SourceAccountID: 2, SourceParentID: "src", TargetAccountID: 1, TargetParentID: "p"})

	_, err := s.BackupRuns.Create(ctx, &domain.BackupRun{
		JobID: jobID, Status: domain.BackupRunRunning, StartedAt: time.Now().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	got, err := s.BackupRuns.ListByJob(ctx, jobID, 10)
	if err != nil || len(got) != 1 || got[0].Status != domain.BackupRunRunning {
		t.Fatalf("list runs: %+v, err=%v", got, err)
	}
	run := got[0]
	run.Status = domain.BackupRunSuccess
	run.Total = 3
	run.Skipped = 2
	run.Uploaded = 1
	if err := s.BackupRuns.Update(ctx, run); err != nil {
		t.Fatalf("update run: %v", err)
	}
	got2, _ := s.BackupRuns.ListByJob(ctx, jobID, 10)
	if got2[0].Status != domain.BackupRunSuccess || got2[0].Uploaded != 1 {
		t.Fatalf("run update not persisted: %+v", got2[0])
	}

	// 文件指纹 upsert 与覆盖
	st := &domain.BackupFileState{
		JobID: jobID, RelPath: "a/b.txt", Size: 100,
		MTime: time.Now().Truncate(time.Second), Hash: "abc", Status: domain.BackupFileUploaded,
	}
	if err := s.BackupFileStates.UpsertByRelPath(ctx, st); err != nil {
		t.Fatalf("upsert state: %v", err)
	}
	st.Size = 120
	st.Hash = "def"
	if err := s.BackupFileStates.UpsertByRelPath(ctx, st); err != nil {
		t.Fatalf("upsert state 2: %v", err)
	}
	states, err := s.BackupFileStates.ListByJob(ctx, jobID)
	if err != nil || len(states) != 1 {
		t.Fatalf("list states: %d, err=%v", len(states), err)
	}
	if states[0].Size != 120 || states[0].Hash != "def" {
		t.Fatalf("state overwrite not persisted: %+v", states[0])
	}

	// 删除任务 → 级联删除 runs 与 states
	if err := s.BackupJobs.Delete(ctx, jobID); err != nil {
		t.Fatalf("delete job: %v", err)
	}
	runsAfter, _ := s.BackupRuns.ListByJob(ctx, jobID, 10)
	if len(runsAfter) != 0 {
		t.Fatalf("runs should cascade-delete, got %d", len(runsAfter))
	}
	statesAfter, _ := s.BackupFileStates.ListByJob(ctx, jobID)
	if len(statesAfter) != 0 {
		t.Fatalf("states should cascade-delete, got %d", len(statesAfter))
	}
}

package backup

import (
	"testing"
	"time"

	"litepan/internal/domain"
)

func TestComputeDailyRunAt(t *testing.T) {
	loc := time.Local
	cases := []struct {
		name string
		base time.Time
		want time.Time
	}{
		{"当天早于触发时间", time.Date(2026, 8, 24, 2, 30, 0, 0, loc), time.Date(2026, 8, 24, 3, 0, 0, 0, loc)},
		{"当天晚于触发时间则顺延次日", time.Date(2026, 8, 24, 10, 0, 0, 0, loc), time.Date(2026, 8, 25, 3, 0, 0, 0, loc)},
		{"恰好等于触发时间则次日", time.Date(2026, 8, 24, 3, 0, 0, 0, loc), time.Date(2026, 8, 25, 3, 0, 0, 0, loc)},
	}
	for _, c := range cases {
		got := computeNextRun(domain.BackupScheduleDaily, map[string]any{"time": "03:00"}, c.base)
		if !got.Equal(c.want) {
			t.Errorf("%s: computeNextRun(daily) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAdvanceDailyRunAt(t *testing.T) {
	loc := time.Local
	got := advanceNextRun(domain.BackupScheduleDaily, map[string]any{"time": "03:00"}, time.Date(2026, 8, 24, 3, 0, 0, 0, loc))
	want := time.Date(2026, 8, 25, 3, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("advanceNextRun(daily) = %v, want %v", got, want)
	}
}

func TestComputeIntervalStartRunAt(t *testing.T) {
	loc := time.Local
	cfg := map[string]any{"start_time": "00:00", "interval_hours": 6}
	cases := []struct {
		name string
		base time.Time
		want time.Time
	}{
		{"早于首次触发时间则当天", time.Date(2026, 8, 23, 23, 0, 0, 0, loc), time.Date(2026, 8, 24, 0, 0, 0, 0, loc)},
		{"晚于首次触发时间则次日", time.Date(2026, 8, 24, 10, 0, 0, 0, loc), time.Date(2026, 8, 25, 0, 0, 0, 0, loc)},
		{"恰好等于触发时间则次日", time.Date(2026, 8, 24, 0, 0, 0, 0, loc), time.Date(2026, 8, 25, 0, 0, 0, 0, loc)},
	}
	for _, c := range cases {
		got := computeNextRun(domain.BackupScheduleInterval, cfg, c.base)
		if !got.Equal(c.want) {
			t.Errorf("%s: computeNextRun(interval) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAdvanceIntervalRunAt(t *testing.T) {
	loc := time.Local
	cfg := map[string]any{"start_time": "00:00", "interval_hours": 2}

	// 同一天内：03:00 + 2h → 05:00
	got := advanceNextRun(domain.BackupScheduleInterval, cfg, time.Date(2026, 8, 24, 3, 0, 0, 0, loc))
	want := time.Date(2026, 8, 24, 5, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("advance interval same-day = %v, want %v", got, want)
	}

	// 跨天：23:30 + 2h → 次日 01:30（跨天）→ 回落到次日 start_time 00:00
	got2 := advanceNextRun(domain.BackupScheduleInterval, cfg, time.Date(2026, 8, 24, 23, 30, 0, 0, loc))
	want2 := time.Date(2026, 8, 25, 0, 0, 0, 0, loc)
	if !got2.Equal(want2) {
		t.Fatalf("advance interval cross-day = %v, want %v", got2, want2)
	}
}

func TestParseClock(t *testing.T) {
	cases := []struct {
		in   string
		h, m int
	}{
		{"03:00", 3, 0},
		{"0:5", 0, 5},
		{"23:59", 23, 59},
		{"25:99", 23, 59}, // 越界收敛
		{"invalid", 0, 0},
		{"12", 0, 0},
	}
	for _, c := range cases {
		h, m := parseClock(c.in)
		if h != c.h || m != c.m {
			t.Errorf("parseClock(%q) = %d:%d, want %d:%d", c.in, h, m, c.h, c.m)
		}
	}
}

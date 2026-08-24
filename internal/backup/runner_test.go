package backup

import (
	"testing"

	"litepan/internal/domain"
)

func TestFinalizeRunSummary(t *testing.T) {
	cases := []struct {
		name                     string
		total, skipped, up, rap, failed int
		wantStatus               string
	}{
		{"空目录", 0, 0, 0, 0, 0, domain.BackupRunSuccess},
		{"源目录不可读(全部失败且 total=0)", 0, 0, 0, 0, 1, domain.BackupRunFailed},
		{"全部成功", 5, 3, 1, 1, 0, domain.BackupRunSuccess},
		{"全部跳过", 5, 5, 0, 0, 0, domain.BackupRunSuccess},
		{"部分失败", 5, 2, 1, 0, 2, domain.BackupRunPartial},
		{"全部失败", 5, 0, 0, 0, 5, domain.BackupRunFailed},
		{"成功含秒传", 5, 0, 1, 4, 0, domain.BackupRunSuccess},
	}
	for _, c := range cases {
		status, msg := finalizeRunSummary(c.total, c.skipped, c.up, c.rap, c.failed)
		if status != c.wantStatus {
			t.Errorf("%s: finalizeRunSummary(%d,%d,%d,%d,%d) status = %q, want %q (msg=%q)",
				c.name, c.total, c.skipped, c.up, c.rap, c.failed, status, c.wantStatus, msg)
		}
		if status == domain.BackupRunFailed && msg == "" {
			t.Errorf("%s: failed run should have non-empty message", c.name)
		}
	}
}

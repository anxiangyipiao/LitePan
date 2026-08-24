package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"litepan/internal/domain"
)

type backupJobRepo struct{ db *DB }

func (r *backupJobRepo) Create(ctx context.Context, job *domain.BackupJob) (int64, error) {
	res, err := r.db.write.ExecContext(ctx,
		`INSERT INTO backup_jobs
		  (name, source_account_id, source_parent_id, source_display_path,
		   target_account_id, target_parent_id, target_display_path, method,
		   schedule_mode, time, start_time, interval_hours, enabled, next_run_at, last_run_at, last_run_status, last_run_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.Name, job.SourceAccountID, job.SourceParentID, job.SourceDisplayPath,
		job.TargetAccountID, job.TargetParentID, job.TargetDisplayPath, job.Method,
		job.ScheduleMode, job.Time, job.StartTime, job.IntervalHours, boolToInt(job.Enabled),
		tsValue(job.NextRunAt), tsValue(job.LastRunAt), job.LastRunStatus, job.LastRunMessage)
	if err != nil {
		return 0, wrapDB(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, wrapDB(err)
	}
	return id, nil
}

func (r *backupJobRepo) Update(ctx context.Context, job *domain.BackupJob) error {
	_, err := r.db.write.ExecContext(ctx,
		`UPDATE backup_jobs
		 SET name=?, source_account_id=?, source_parent_id=?, source_display_path=?,
		     target_account_id=?, target_parent_id=?, target_display_path=?, method=?,
		     schedule_mode=?, time=?, start_time=?, interval_hours=?, enabled=?, next_run_at=?, updated_at=CURRENT_TIMESTAMP
		 WHERE id=?`,
		job.Name, job.SourceAccountID, job.SourceParentID, job.SourceDisplayPath,
		job.TargetAccountID, job.TargetParentID, job.TargetDisplayPath, job.Method,
		job.ScheduleMode, job.Time, job.StartTime, job.IntervalHours, boolToInt(job.Enabled),
		tsValue(job.NextRunAt), job.ID)
	return wrapDB(err)
}

func (r *backupJobRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.write.ExecContext(ctx, `DELETE FROM backup_jobs WHERE id=?`, id)
	return wrapDB(err)
}

func (r *backupJobRepo) Get(ctx context.Context, id int64) (*domain.BackupJob, error) {
	row := r.db.read.QueryRowContext(ctx, selectBackupJobCols+` WHERE id=?`, id)
	return scanBackupJob(row)
}

func (r *backupJobRepo) ListAll(ctx context.Context) ([]*domain.BackupJob, error) {
	rows, err := r.db.read.QueryContext(ctx, selectBackupJobCols+` ORDER BY id DESC`)
	if err != nil {
		return nil, wrapDB(err)
	}
	defer rows.Close()
	var out []*domain.BackupJob
	for rows.Next() {
		job, err := scanBackupJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, wrapDB(rows.Err())
}

func (r *backupJobRepo) ListEnabled(ctx context.Context) ([]*domain.BackupJob, error) {
	rows, err := r.db.read.QueryContext(ctx, selectBackupJobCols+` WHERE enabled=1 ORDER BY id ASC`)
	if err != nil {
		return nil, wrapDB(err)
	}
	defer rows.Close()
	var out []*domain.BackupJob
	for rows.Next() {
		job, err := scanBackupJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, wrapDB(rows.Err())
}

func (r *backupJobRepo) UpdateLastRun(ctx context.Context, id int64, status, message string, lastRunAt time.Time) error {
	_, err := r.db.write.ExecContext(ctx,
		`UPDATE backup_jobs
		 SET last_run_status=?, last_run_message=?, last_run_at=?, updated_at=CURRENT_TIMESTAMP
		 WHERE id=?`,
		status, message, tsValue(lastRunAt), id)
	return wrapDB(err)
}

const selectBackupJobCols = `SELECT id, name, source_account_id, source_parent_id, source_display_path,
  target_account_id, target_parent_id, target_display_path, method,
  schedule_mode, time, start_time, interval_hours, enabled, next_run_at, last_run_at, last_run_status, last_run_message, created_at, updated_at
FROM backup_jobs`

func scanBackupJob(s rowScanner) (*domain.BackupJob, error) {
	var (
		job       domain.BackupJob
		enabled   int
		nextRunAt sql.NullString
		lastRunAt sql.NullString
		createdAt sql.NullString
		updatedAt sql.NullString
	)
	err := s.Scan(
		&job.ID, &job.Name, &job.SourceAccountID, &job.SourceParentID, &job.SourceDisplayPath,
		&job.TargetAccountID, &job.TargetParentID, &job.TargetDisplayPath, &job.Method,
		&job.ScheduleMode, &job.Time, &job.StartTime, &job.IntervalHours, &enabled, &nextRunAt, &lastRunAt,
		&job.LastRunStatus, &job.LastRunMessage, &createdAt, &updatedAt)
	if err != nil {
		return nil, wrapDB(err)
	}
	job.Enabled = enabled != 0
	job.NextRunAt = parseTS(nextRunAt)
	job.LastRunAt = parseTS(lastRunAt)
	job.CreatedAt = parseTS(createdAt)
	job.UpdatedAt = parseTS(updatedAt)
	return &job, nil
}

type backupRunRepo struct{ db *DB }

func (r *backupRunRepo) Create(ctx context.Context, run *domain.BackupRun) (int64, error) {
	res, err := r.db.write.ExecContext(ctx,
		`INSERT INTO backup_runs
		  (job_id, started_at, finished_at, status, total, skipped, uploaded, rapid, failed, message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.JobID, tsValue(run.StartedAt), tsValue(run.FinishedAt), run.Status,
		run.Total, run.Skipped, run.Uploaded, run.Rapid, run.Failed, run.Message)
	if err != nil {
		return 0, wrapDB(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, wrapDB(err)
	}
	return id, nil
}

func (r *backupRunRepo) Update(ctx context.Context, run *domain.BackupRun) error {
	_, err := r.db.write.ExecContext(ctx,
		`UPDATE backup_runs
		 SET finished_at=?, status=?, total=?, skipped=?, uploaded=?, rapid=?, failed=?, message=?, failed_files=?
		 WHERE id=?`,
		tsValue(run.FinishedAt), run.Status, run.Total, run.Skipped, run.Uploaded, run.Rapid, run.Failed,
		run.Message, marshalRunFailures(run.FailedFiles), run.ID)
	return wrapDB(err)
}

func (r *backupRunRepo) ListByJob(ctx context.Context, jobID int64, limit int) ([]*domain.BackupRun, error) {
	query := selectBackupRunCols + ` WHERE job_id=? ORDER BY id DESC`
	args := []any{jobID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := r.db.read.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrapDB(err)
	}
	defer rows.Close()
	var out []*domain.BackupRun
	for rows.Next() {
		run, err := scanBackupRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, wrapDB(rows.Err())
}

func (r *backupRunRepo) DeleteByJob(ctx context.Context, jobID int64) error {
	_, err := r.db.write.ExecContext(ctx, `DELETE FROM backup_runs WHERE job_id=?`, jobID)
	return wrapDB(err)
}

const selectBackupRunCols = `SELECT id, job_id, started_at, finished_at, status, total, skipped, uploaded, rapid, failed, message, failed_files
FROM backup_runs`

func scanBackupRun(s rowScanner) (*domain.BackupRun, error) {
	var (
		run        domain.BackupRun
		startedAt  sql.NullString
		finishedAt sql.NullString
		failedFile sql.NullString
	)
	err := s.Scan(
		&run.ID, &run.JobID, &startedAt, &finishedAt, &run.Status,
		&run.Total, &run.Skipped, &run.Uploaded, &run.Rapid, &run.Failed, &run.Message, &failedFile)
	if err != nil {
		return nil, wrapDB(err)
	}
	run.StartedAt = parseTS(startedAt)
	run.FinishedAt = parseTS(finishedAt)
	run.FailedFiles = unmarshalRunFailures(failedFile)
	return &run, nil
}

func marshalRunFailures(failures []domain.BackupRunFailure) string {
	if len(failures) == 0 {
		return "[]"
	}
	b, err := json.Marshal(failures)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalRunFailures(ns sql.NullString) []domain.BackupRunFailure {
	var out []domain.BackupRunFailure
	if !ns.Valid || strings.TrimSpace(ns.String) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(ns.String), &out)
	return out
}

type backupFileStateRepo struct{ db *DB }

func (r *backupFileStateRepo) UpsertByRelPath(ctx context.Context, st *domain.BackupFileState) error {
	_, err := r.db.write.ExecContext(ctx,
		`INSERT INTO backup_file_states (job_id, rel_path, size, mtime, hash, status, error, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(job_id, rel_path) DO UPDATE SET
		   size=excluded.size, mtime=excluded.mtime, hash=excluded.hash,
		   status=excluded.status, error=excluded.error, updated_at=CURRENT_TIMESTAMP`,
		st.JobID, st.RelPath, st.Size, tsValue(st.MTime), st.Hash, st.Status, st.Error)
	return wrapDB(err)
}

func (r *backupFileStateRepo) ListByJob(ctx context.Context, jobID int64) ([]*domain.BackupFileState, error) {
	rows, err := r.db.read.QueryContext(ctx,
		`SELECT id, job_id, rel_path, size, mtime, hash, status, error, updated_at
		 FROM backup_file_states WHERE job_id=?`, jobID)
	if err != nil {
		return nil, wrapDB(err)
	}
	defer rows.Close()
	var out []*domain.BackupFileState
	for rows.Next() {
		var (
			st       domain.BackupFileState
			mtime    sql.NullString
			updateAt sql.NullString
		)
		if err := rows.Scan(&st.ID, &st.JobID, &st.RelPath, &st.Size, &mtime, &st.Hash, &st.Status, &st.Error, &updateAt); err != nil {
			return nil, wrapDB(err)
		}
		st.MTime = parseTS(mtime)
		st.UpdatedAt = parseTS(updateAt)
		out = append(out, &st)
	}
	return out, wrapDB(rows.Err())
}

func (r *backupFileStateRepo) DeleteByJob(ctx context.Context, jobID int64) error {
	_, err := r.db.write.ExecContext(ctx, `DELETE FROM backup_file_states WHERE job_id=?`, jobID)
	return wrapDB(err)
}

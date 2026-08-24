import { http } from "./client";
import { streamCrossTransferNDJSON } from "./crossTransfer";

export type BackupScheduleMode = "manual" | "daily" | "interval";
export type BackupRunStatus = "running" | "success" | "partial" | "failed";

export interface BackupJob {
  id: number;
  name: string;
  source_path: string;
  target_account_id: number;
  target_parent_id: string;
  target_display_path: string;
  method: "sha1" | "md5";
  schedule_mode: BackupScheduleMode;
  time: string;
  start_time: string;
  interval_hours: number;
  enabled: boolean;
  next_run_at?: string;
  last_run_at?: string;
  last_run_status: string;
  last_run_message: string;
  created_at?: string;
  updated_at?: string;
}

export type BackupJobInput = Omit<
  BackupJob,
  | "id"
  | "next_run_at"
  | "last_run_at"
  | "last_run_status"
  | "last_run_message"
  | "created_at"
  | "updated_at"
>;

export interface BackupRun {
  id: number;
  job_id: number;
  started_at?: string;
  finished_at?: string;
  status: BackupRunStatus;
  total: number;
  skipped: number;
  uploaded: number;
  rapid: number;
  failed: number;
  message: string;
}

export interface BackupStreamEvent {
  event: "start" | "file" | "end" | "error";
  [key: string]: unknown;
}

export function fetchBackupJobs() {
  return http.get<BackupJob[]>("/admin/backup/jobs");
}

export function createBackupJob(body: BackupJobInput) {
  return http.post<BackupJob>("/admin/backup/jobs", body);
}

export function updateBackupJob(id: number, body: BackupJobInput) {
  return http.put<BackupJob>(`/admin/backup/jobs/${id}`, body);
}

export function deleteBackupJob(id: number) {
  return http.del<{ id: number }>(`/admin/backup/jobs/${id}`);
}

export function toggleBackupJob(id: number) {
  return http.post<BackupJob>(`/admin/backup/jobs/${id}/toggle`, {});
}

export function runBackupJob(id: number) {
  return http.post<BackupJob>(`/admin/backup/jobs/${id}/run`, {});
}

export function fetchBackupRuns(jobId: number, limit = 20) {
  return http.get<BackupRun[]>(`/admin/backup/jobs/${jobId}/runs`, { limit });
}

export function streamBackupRun(jobId: number, signal?: AbortSignal) {
  return streamCrossTransferNDJSON<BackupStreamEvent>(
    `/admin/backup/jobs/${jobId}/run/stream`,
    {},
    signal,
  );
}

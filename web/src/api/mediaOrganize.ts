import { http } from "./client";

export interface MediaOrganizeTaskConfig {
  target_directory?: string;
  target_directory_id?: string;
  action_type?: string;
  target_root?: string;
  target_root_id?: string;
  media_type?: string;
  rename_marker?: string;
  use_tmdb?: boolean;
  overwrite_existing?: boolean;
  recursive?: boolean;
  scatter_movie_per_file?: boolean;
  account_id?: string | number;
}

export interface MediaOrganizeTask {
  id: string;
  task_name: string;
  account_id: number;
  config: MediaOrganizeTaskConfig;
  status: string;
  last_run_at?: string;
  last_run_result?: MediaOrganizeRunResult;
  created_at?: string;
  updated_at?: string;
  is_running?: boolean;
}

export interface MediaOrganizeRunResult {
  total?: number;
  renamed?: number;
  moved?: number;
  skipped?: number;
  failed?: number;
  stopped?: boolean;
}

export interface MediaOrganizePlanAction {
  id: string;
  kind: string;
  source_id?: string;
  source_name?: string;
  source_parent_id?: string;
  target_parent_id?: string;
  target_name?: string;
  reason?: string;
  confidence?: number;
  metadata?: Record<string, unknown>;
  status?: string;
  error?: string;
}

export interface MediaOrganizePlan {
  task_id?: string;
  created_at?: string;
  target_root_id?: string;
  target_parent_id?: string;
  actions?: MediaOrganizePlanAction[];
  skipped?: Array<Record<string, unknown>>;
  diagnostics?: Record<string, unknown>;
}

export interface MediaOrganizeProgress {
  stage?: string;
  status?: string;
  scanned_dirs?: number;
  scanned_files?: number;
  groups?: number;
  actions?: number;
  skipped?: number;
  current_dir?: string;
  planned_works?: number;
  max_works?: number;
  quota_reached?: boolean;
}

export interface MediaOrganizeLogEntry {
  time: string;
  message: string;
}

export interface MediaOrganizeSettings {
  proxy_enabled: boolean;
  proxy_url: string;
  proxy_username: string;
  proxy_password: string;
  tmdb_api_key: string;
  tmdb_language: string;
  api_request_interval_ms: number;
  tmdb_request_interval_ms: number;
  file_extensions: string;
  metadata_extensions: string;
  media_tag_order: string[] | string;
  align_media_tags: boolean;
  max_works_per_run: number;
  overwrite_existing: boolean;
}

export type MediaOrganizeTaskInput = {
  task_name: string;
  account_id: number;
  target_directory: string;
  target_directory_id: string;
  action_type: string;
  target_root?: string;
  target_root_id?: string;
  media_type: string;
  rename_marker?: string;
  use_tmdb: boolean;
  overwrite_existing?: boolean;
  recursive?: boolean;
  scatter_movie_per_file?: boolean;
};

export function fetchMediaOrganizeTasks() {
  return http.get<MediaOrganizeTask[]>("/admin/media-organize/tasks");
}

export function createMediaOrganizeTask(input: MediaOrganizeTaskInput) {
  return http.post<MediaOrganizeTask>("/admin/media-organize/tasks", input);
}

export function updateMediaOrganizeTask(id: string, input: Partial<MediaOrganizeTaskInput>) {
  return http.put<MediaOrganizeTask>(`/admin/media-organize/tasks/${id}`, input);
}

export function deleteMediaOrganizeTask(id: string) {
  return http.del<{ id: string; stopping?: boolean }>(`/admin/media-organize/tasks/${id}`);
}

// 计划改为后台生成：接口立即返回 submitted，前端轮询 progress.stage 直到 done/error 后再拉取计划。
export interface MediaOrganizePlanSubmitResult {
  task_id: string;
  submitted: boolean;
}

export function planMediaOrganizeTask(id: string) {
  return http.post<MediaOrganizePlanSubmitResult>(`/admin/media-organize/tasks/${id}/plan`);
}

export function fetchMediaOrganizePlan(id: string) {
  return http.get<MediaOrganizePlan>(`/admin/media-organize/tasks/${id}/plan`);
}

export function applyMediaOrganizeTask(id: string) {
  return http.post<Record<string, unknown>>(`/admin/media-organize/tasks/${id}/apply`);
}

export function stopMediaOrganizeTask(id: string) {
  return http.post<{ stopping: boolean }>(`/admin/media-organize/tasks/${id}/stop`);
}

export function fetchMediaOrganizeLogs(id: string) {
  return http.get<{
    logs: MediaOrganizeLogEntry[];
    status: string;
    last_run_result?: MediaOrganizeRunResult;
  }>(`/admin/media-organize/tasks/${id}/logs`);
}

export function fetchMediaOrganizeProgress(id: string) {
  return http.get<MediaOrganizeProgress>(`/admin/media-organize/tasks/${id}/progress`);
}

export function updateMediaOrganizePlanAction(taskId: string, actionId: string, targetName: string) {
  return http.put<{ action?: MediaOrganizePlanAction; changed?: boolean }>(
    `/admin/media-organize/tasks/${taskId}/plan/actions/${actionId}`,
    { target_name: targetName },
  );
}

export function deleteMediaOrganizePlanAction(taskId: string, actionId: string) {
  return http.del<{ removed?: string }>(`/admin/media-organize/tasks/${taskId}/plan/actions/${actionId}`);
}

export function batchDeleteMediaOrganizePlanActions(taskId: string, actionIds: string[]) {
  return http.post<{ removed?: string[] }>(`/admin/media-organize/tasks/${taskId}/plan/actions/batch-delete`, {
    action_ids: actionIds,
  });
}

export function testMediaOrganizeTmdb(payload?: Partial<MediaOrganizeSettings>) {
  return http.post<{ ok: boolean; language?: string; proxy_used?: boolean }>(
    "/admin/media-organize/test-tmdb",
    payload ?? {},
  );
}

export interface MediaOrganizeTmdbSearchHit {
  id?: number | string;
  title?: string;
  name?: string;
  original_title?: string;
  original_name?: string;
  release_date?: string;
  first_air_date?: string;
  poster_path?: string;
  media_type?: string;
  overview?: string;
}

export function searchMediaOrganizeTmdb(params: {
  query: string;
  year?: number;
  language?: string;
  media_type?: string;
}) {
  return http.get<MediaOrganizeTmdbSearchHit[]>("/admin/media-organize/search-tmdb", {
    query: params.query,
    year: params.year,
    language: params.language,
    media_type: params.media_type ?? "auto",
  });
}

export function fetchMediaOrganizeSettings() {
  return http.get<MediaOrganizeSettings>("/admin/media-organize/settings");
}

export function saveMediaOrganizeSettings(settings: Partial<MediaOrganizeSettings>) {
  return http.put<MediaOrganizeSettings>("/admin/media-organize/settings", settings);
}

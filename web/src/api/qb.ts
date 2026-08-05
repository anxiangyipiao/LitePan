import { http } from "./client";

export interface QBSettings {
  enabled: boolean;
  url: string;
  username: string;
  password: string;
  save_path: string;
}

export interface QBDownloadTask {
  hash: string;
  name: string;
  state: string; // pending | running | seeding | paused | error | finished
  progress: number;
  size: number;
  save_path: string;
  added_on: number;
  error?: string;
}

export function fetchQBSettings() {
  return http.get<QBSettings>("/admin/qb/settings");
}

export function saveQBSettings(settings: Partial<QBSettings>) {
  return http.put<QBSettings>("/admin/qb/settings", settings);
}

export function testQB() {
  return http.post<{ ok: boolean; version?: string; url?: string }>("/admin/qb/test", {});
}

export function addQBDownload(input: { urls: string[]; save_path?: string }) {
  return http.post<{ ok: boolean }>("/admin/qb/add", input);
}

export function fetchQBDownloads() {
  return http.get<QBDownloadTask[]>("/admin/qb/tasks");
}

export function deleteQBDownloads(input: { hashes: string[]; delete_files?: boolean }) {
  return http.post<{ ok: boolean }>("/admin/qb/tasks/delete", input);
}

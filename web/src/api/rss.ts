import { http } from "./client";

export type RSSTargetType = "qb" | "offline";
export type RSSHistoryStatus = "matched" | "pushed" | "queued" | "completed" | "failed" | "skipped";

export interface RSSSubscription {
  id: number;
  name: string;
  feed_url: string;
  enabled: boolean;
  title_keyword: string;
  exclude_keywords: string;
  episode_min: number;
  episode_max: number;
  quality_keyword: string;
  target_type: RSSTargetType;
  qb_save_path: string;
  qb_category: string;
  account_id: number;
  target_parent_id: string;
  target_display_path: string;
  fetch_interval_minutes: number;
  consecutive_failures: number;
  last_fetch_at?: string;
  last_fetch_status: string;
  last_fetch_message: string;
  created_at?: string;
  updated_at?: string;
}

export type RSSSubscriptionInput = Omit<
  RSSSubscription,
  | "id"
  | "consecutive_failures"
  | "last_fetch_at"
  | "last_fetch_status"
  | "last_fetch_message"
  | "created_at"
  | "updated_at"
>;

export interface RSSDownloadHistory {
  id: number;
  subscription_id: number;
  feed_guid: string;
  infohash: string;
  title: string;
  episode: number;
  link: string;
  torrent_url: string;
  target_type: string;
  target_ref: string;
  status: RSSHistoryStatus;
  message: string;
  error: string;
  created_at?: string;
  pushed_at?: string;
}

export interface RSSPreviewItem {
  title: string;
  guid: string;
  link: string;
  pub_date?: string;
  torrent_url: string;
  infohash: string;
  episode: number;
  matched: boolean;
  reason: string;
}

export interface RSSPreviewResult {
  feed_title: string;
  items: RSSPreviewItem[];
  total: number;
  fetched_at?: string;
}

export interface RSSPreviewInput {
  feed_url: string;
  title_keyword: string;
  exclude_keywords: string;
  episode_min: number;
  episode_max: number;
  quality_keyword: string;
  limit: number;
}

export interface RSSFetchNowResult {
  fetched_at?: string;
  items_parsed: number;
  matched: number;
  pushed: number;
  failed: number;
  message: string;
}

export function fetchRSSSubscriptions() {
  return http.get<RSSSubscription[]>("/admin/rss/subscriptions");
}

export function createRSSSubscription(body: RSSSubscriptionInput) {
  return http.post<RSSSubscription>("/admin/rss/subscriptions", body);
}

export function updateRSSSubscription(id: number, body: RSSSubscriptionInput) {
  return http.put<RSSSubscription>(`/admin/rss/subscriptions/${id}`, body);
}

export function deleteRSSSubscription(id: number) {
  return http.del<{ id: number }>(`/admin/rss/subscriptions/${id}`);
}

export function toggleRSSSubscription(id: number) {
  return http.post<RSSSubscription>(`/admin/rss/subscriptions/${id}/toggle`, {});
}

export function fetchRSSSubscriptionNow(id: number) {
  return http.post<RSSFetchNowResult>(`/admin/rss/subscriptions/${id}/fetch`, {});
}

export function previewRSSFeed(body: RSSPreviewInput) {
  return http.post<RSSPreviewResult>("/admin/rss/preview", body);
}

export function fetchRSSHistory(query: { subscription_id?: number; limit?: number; offset?: number }) {
  return http.get<RSSDownloadHistory[]>("/admin/rss/history", query);
}

export function retryRSSHistory(id: number) {
  return http.post<RSSDownloadHistory>(`/admin/rss/history/${id}/retry`, {});
}

export function deleteRSSHistory(id: number) {
  return http.del<{ id: number }>(`/admin/rss/history/${id}`);
}

export function clearRSSHistory(subscriptionId?: number) {
  const query = subscriptionId ? { subscription_id: subscriptionId } : undefined;
  return http.del<{ deleted: number }>("/admin/rss/history", undefined, query);
}
